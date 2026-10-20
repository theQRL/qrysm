package client

import (
	"context"
	"testing"
	"time"

	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeClientConn struct {
	host           string
	invokeErr      error
	invokeCalls    int
	invokeFn       func(method string, reply interface{}) error
	newStreamErr   error
	newStreamCalls int
	closed         bool
}

func (f *fakeClientConn) Invoke(_ context.Context, method string, _ interface{}, reply interface{}, _ ...grpc.CallOption) error {
	f.invokeCalls++
	if f.invokeFn != nil {
		return f.invokeFn(method, reply)
	}
	return f.invokeErr
}

func (f *fakeClientConn) NewStream(_ context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	f.newStreamCalls++
	if f.newStreamErr != nil {
		return nil, f.newStreamErr
	}
	return &fakeClientStream{}, nil
}

func (f *fakeClientConn) Close() error {
	f.closed = true
	return nil
}

type fakeClientStream struct{}

func (*fakeClientStream) Header() (metadata.MD, error) { return nil, nil }
func (*fakeClientStream) Trailer() metadata.MD         { return nil }
func (*fakeClientStream) CloseSend() error             { return nil }
func (*fakeClientStream) Context() context.Context     { return context.Background() }
func (*fakeClientStream) SendMsg(interface{}) error    { return nil }
func (*fakeClientStream) RecvMsg(interface{}) error    { return nil }

type fakeNodeClient struct {
	getSyncStatusCalls int
	syncStatus         *qrysmpb.SyncStatus
	err                error
}

func (f *fakeNodeClient) GetSyncStatus(_ context.Context, _ *emptypb.Empty) (*qrysmpb.SyncStatus, error) {
	f.getSyncStatusCalls++
	return f.syncStatus, f.err
}

func (*fakeNodeClient) GetGenesis(context.Context, *emptypb.Empty) (*qrysmpb.Genesis, error) {
	return nil, nil
}

func (*fakeNodeClient) GetVersion(context.Context, *emptypb.Empty) (*qrysmpb.Version, error) {
	return nil, nil
}

func (*fakeNodeClient) ListPeers(context.Context, *emptypb.Empty) (*qrysmpb.Peers, error) {
	return nil, nil
}

func TestGrpcConnectionProvider_InvokeFailsOverToHealthyHost(t *testing.T) {
	primary := &fakeClientConn{
		host:      "primary:4000",
		invokeErr: status.Error(codes.Unavailable, "primary unavailable"),
	}
	secondary := &fakeClientConn{host: "secondary:4000"}

	provider := &grpcConnectionProvider{
		endpoints:   []string{"primary:4000", "secondary:4000"},
		currentHost: "primary:4000",
		currentIdx:  0,
		conn:        primary,
		dial: func(_ context.Context, host string) (closableClientConn, error) {
			if host == secondary.host {
				return secondary, nil
			}
			return primary, nil
		},
		checkSyncStatus: func(_ context.Context, conn grpc.ClientConnInterface) (bool, error) {
			switch conn.(*fakeClientConn).host {
			case primary.host:
				return false, status.Error(codes.Unavailable, "primary unavailable")
			case secondary.host:
				return false, nil
			default:
				return false, nil
			}
		},
		healthCheckTimeout: time.Second,
	}

	err := provider.Invoke(context.Background(), "/qrysm.Node/GetSyncStatus", &emptypb.Empty{}, &emptypb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, "secondary:4000", provider.CurrentHost())
	assert.Equal(t, 1, secondary.invokeCalls)
	assert.Equal(t, true, primary.closed)
}

func TestWaitForSync_SwitchesToHealthyHost(t *testing.T) {
	primary := &fakeClientConn{host: "primary:4000"}
	secondary := &fakeClientConn{host: "secondary:4000"}

	provider := &grpcConnectionProvider{
		endpoints:   []string{"primary:4000", "secondary:4000"},
		currentHost: "primary:4000",
		currentIdx:  0,
		conn:        primary,
		dial: func(_ context.Context, host string) (closableClientConn, error) {
			if host == secondary.host {
				return secondary, nil
			}
			return primary, nil
		},
		checkSyncStatus: func(_ context.Context, conn grpc.ClientConnInterface) (bool, error) {
			return conn.(*fakeClientConn).host == primary.host, nil
		},
		healthCheckTimeout: time.Second,
	}

	node := &fakeNodeClient{
		syncStatus: &qrysmpb.SyncStatus{Syncing: true},
	}
	v := &validator{
		grpcHealthTracker: provider,
		node:              node,
	}

	require.NoError(t, v.WaitForSync(context.Background()))
	assert.Equal(t, "secondary:4000", provider.CurrentHost())
	assert.Equal(t, 0, node.getSyncStatusCalls)
}
