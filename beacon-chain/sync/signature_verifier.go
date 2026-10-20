package sync

import (
	"context"
	"runtime"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/monitoring/tracing"
	"go.opencensus.io/trace"
)

// Maximum number of requests waiting for a signature worker.
const verifierLimit = 50

type signatureVerifier struct {
	ctx     context.Context
	set     *ml_dsa_87.SignatureBatch
	resChan chan error
}

// Verify gossip requests independently. Each worker checks signatures serially,
// so this pool bounds concurrent signature checks across all queued requests.
func (s *Service) verifierRoutine() {
	s.runSignatureWorkers(max(runtime.GOMAXPROCS(0)-1, 1), (*ml_dsa_87.SignatureBatch).VerifySequential)
}

func (s *Service) runSignatureWorkers(count int, verify func(*ml_dsa_87.SignatureBatch, context.Context) (bool, error)) {
	var workers sync.WaitGroup
	for range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-s.ctx.Done():
					return
				case request := <-s.signatureChan:
					if s.ctx.Err() != nil {
						return
					}
					err := request.ctx.Err()
					if err == nil {
						var valid bool
						valid, err = verify(request.set, request.ctx)
						if err == nil && !valid {
							err = errors.New("signature verification failed")
						}
					}
					// One buffered result lets a worker finish even if the caller
					// has already returned because of cancellation.
					request.resChan <- err
				}
			}
		}()
	}
	workers.Wait()
}

func (s *Service) validateSignatures(ctx context.Context, message string, set *ml_dsa_87.SignatureBatch) (pubsub.ValidationResult, error) {
	ctx, span := trace.StartSpan(ctx, "sync.validateSignatures")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return pubsub.ValidationIgnore, err
	}
	if err := s.ctx.Err(); err != nil {
		return pubsub.ValidationIgnore, err
	}
	if set == nil {
		return pubsub.ValidationReject, errors.New("nil signature set")
	}

	// Both the caller's deadline and service shutdown cancel queued and active
	// verification. Returning also cancels work the caller no longer needs.
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	request := &signatureVerifier{ctx: ctx, set: set.Copy(), resChan: make(chan error, 1)}
	select {
	case <-ctx.Done():
		return pubsub.ValidationIgnore, ctx.Err()
	case <-s.ctx.Done():
		return pubsub.ValidationIgnore, s.ctx.Err()
	case s.signatureChan <- request:
	}
	select {
	case <-ctx.Done():
		return pubsub.ValidationIgnore, ctx.Err()
	case <-s.ctx.Done():
		return pubsub.ValidationIgnore, s.ctx.Err()
	case err := <-request.resChan:
		if ctx.Err() != nil {
			return pubsub.ValidationIgnore, ctx.Err()
		}
		if s.ctx.Err() != nil {
			return pubsub.ValidationIgnore, s.ctx.Err()
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return pubsub.ValidationIgnore, err
			}
			err = errors.Wrapf(err, "Could not verify %s", message)
			tracing.AnnotateError(span, err)
			return pubsub.ValidationReject, err
		}
		return pubsub.ValidationAccept, nil
	}
}
