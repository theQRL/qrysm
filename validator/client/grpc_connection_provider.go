package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const grpcHealthCheckTimeout = 5 * time.Second

type closableClientConn interface {
	grpc.ClientConnInterface
	Close() error
}

type grpcHealthTracker interface {
	EnsureHealthy(context.Context) error
	CurrentHost() string
}

type grpcConnectionProvider struct {
	mu          sync.RWMutex
	endpoints   []string
	currentHost string
	currentIdx  int
	conn        closableClientConn

	dial               func(context.Context, string) (closableClientConn, error)
	checkSyncStatus    func(context.Context, grpc.ClientConnInterface) (bool, error)
	healthCheckTimeout time.Duration
}

func newGrpcConnectionProvider(
	ctx context.Context,
	endpoint string,
	dialOpts []grpc.DialOption,
) (*grpcConnectionProvider, error) {
	endpoints := splitGRPCEndpoints(endpoint)
	provider := &grpcConnectionProvider{
		endpoints: endpoints,
		dial: func(ctx context.Context, host string) (closableClientConn, error) {
			return grpc.DialContext(ctx, host, dialOpts...)
		},
		checkSyncStatus: func(ctx context.Context, conn grpc.ClientConnInterface) (bool, error) {
			resp, err := qrysmpb.NewNodeClient(conn).GetSyncStatus(ctx, &emptypb.Empty{})
			if err != nil {
				return false, err
			}
			return resp.Syncing, nil
		},
		healthCheckTimeout: grpcHealthCheckTimeout,
	}

	for i, host := range endpoints {
		conn, err := provider.dial(ctx, host)
		if err != nil {
			if i == len(endpoints)-1 {
				return nil, err
			}
			continue
		}
		provider.conn = conn
		provider.currentHost = host
		provider.currentIdx = i
		return provider, nil
	}

	return nil, fmt.Errorf("could not dial any beacon RPC endpoint from %q", endpoint)
}

func splitGRPCEndpoints(endpoint string) []string {
	parts := strings.Split(endpoint, ",")
	endpoints := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		endpoints = append(endpoints, trimmed)
	}
	if len(endpoints) == 0 {
		return []string{endpoint}
	}
	return endpoints
}

func (p *grpcConnectionProvider) CurrentHost() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentHost
}

func (p *grpcConnectionProvider) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	conn := p.currentConn()
	if conn == nil {
		return errors.New("no beacon RPC connection")
	}
	err := conn.Invoke(ctx, method, args, reply, opts...)
	if !shouldFailoverGRPCError(err) {
		return err
	}
	if failoverErr := p.EnsureHealthy(ctx); failoverErr != nil {
		return err
	}
	conn = p.currentConn()
	if conn == nil {
		return err
	}
	return conn.Invoke(ctx, method, args, reply, opts...)
}

func (p *grpcConnectionProvider) NewStream(
	ctx context.Context,
	desc *grpc.StreamDesc,
	method string,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	conn := p.currentConn()
	if conn == nil {
		return nil, errors.New("no beacon RPC connection")
	}
	stream, err := conn.NewStream(ctx, desc, method, opts...)
	if !shouldFailoverGRPCError(err) {
		return stream, err
	}
	if failoverErr := p.EnsureHealthy(ctx); failoverErr != nil {
		return nil, err
	}
	conn = p.currentConn()
	if conn == nil {
		return nil, err
	}
	return conn.NewStream(ctx, desc, method, opts...)
}

func (p *grpcConnectionProvider) Close() error {
	p.mu.Lock()
	conn := p.conn
	p.conn = nil
	p.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

func (p *grpcConnectionProvider) EnsureHealthy(ctx context.Context) error {
	currentConn, currentHost, currentIdx := p.snapshot()
	if currentConn != nil {
		healthy, err := p.isHealthyConn(ctx, currentConn)
		if err == nil && healthy {
			return nil
		}
	}

	var errs []string
	for attempt := 1; attempt < len(p.endpoints); attempt++ {
		idx := (currentIdx + attempt) % len(p.endpoints)
		host := p.endpoints[idx]
		conn, err := p.dial(ctx, host)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", host, err))
			continue
		}
		healthy, err := p.isHealthyConn(ctx, conn)
		if err != nil {
			_ = conn.Close()
			errs = append(errs, fmt.Sprintf("%s: %v", host, err))
			continue
		}
		if !healthy {
			_ = conn.Close()
			errs = append(errs, fmt.Sprintf("%s: beacon node still syncing", host))
			continue
		}
		p.swapConnection(idx, host, conn)
		log.WithFields(logrus.Fields{
			"previousHost": currentHost,
			"newHost":      host,
		}).Info("Switched gRPC endpoint")
		return nil
	}

	if currentHost == "" {
		return fmt.Errorf("could not find healthy beacon RPC endpoint: %s", strings.Join(errs, "; "))
	}

	conn, err := p.dial(ctx, currentHost)
	if err == nil {
		healthy, healthErr := p.isHealthyConn(ctx, conn)
		if healthErr == nil && healthy {
			p.swapConnection(currentIdx, currentHost, conn)
			log.WithField("host", currentHost).Info("Reconnected to healthy beacon node")
			return nil
		}
		if healthErr != nil {
			err = healthErr
		} else {
			err = errors.New("beacon node still syncing")
		}
		_ = conn.Close()
	}
	errs = append(errs, fmt.Sprintf("%s: %v", currentHost, err))
	return fmt.Errorf("could not find healthy beacon RPC endpoint: %s", strings.Join(errs, "; "))
}

func (p *grpcConnectionProvider) snapshot() (closableClientConn, string, int) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.conn, p.currentHost, p.currentIdx
}

func (p *grpcConnectionProvider) currentConn() closableClientConn {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.conn
}

func (p *grpcConnectionProvider) swapConnection(idx int, host string, conn closableClientConn) {
	p.mu.Lock()
	oldConn := p.conn
	p.conn = conn
	p.currentIdx = idx
	p.currentHost = host
	p.mu.Unlock()
	if oldConn != nil && oldConn != conn {
		_ = oldConn.Close()
	}
}

func (p *grpcConnectionProvider) isHealthyConn(ctx context.Context, conn grpc.ClientConnInterface) (bool, error) {
	timeout := p.healthCheckTimeout
	if timeout <= 0 {
		timeout = grpcHealthCheckTimeout
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	syncing, err := p.checkSyncStatus(checkCtx, conn)
	if err != nil {
		return false, err
	}
	return !syncing, nil
}

func shouldFailoverGRPCError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	st, ok := status.FromError(err)
	return ok && st.Code() == codes.Unavailable
}
