package sync

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/testing/require"
)

func TestValidateSignatures(t *testing.T) {
	key, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	otherKey, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	sig, err := key.Sign(make([]byte, 32))
	require.NoError(t, err)
	badSig, err := otherKey.Sign(make([]byte, 32))
	require.NoError(t, err)
	validSet := &ml_dsa_87.SignatureBatch{
		Messages:     [][32]byte{{}},
		PublicKeys:   [][]ml_dsa_87.PublicKey{{key.PublicKey()}},
		Signatures:   [][][]byte{{sig.Marshal()}},
		Descriptions: []string{signing.UnknownSignature},
	}
	invalidSet := &ml_dsa_87.SignatureBatch{
		Messages:     [][32]byte{{}},
		PublicKeys:   [][]ml_dsa_87.PublicKey{{key.PublicKey()}},
		Signatures:   [][][]byte{{badSig.Marshal()}},
		Descriptions: []string{signing.UnknownSignature},
	}
	missingDescriptions := validSet.Copy()
	missingDescriptions.Descriptions = nil
	missingSignatures := validSet.Copy()
	missingSignatures.Signatures = nil
	nilPublicKey := validSet.Copy()
	nilPublicKey.PublicKeys[0][0] = nil
	tests := []struct {
		name          string
		message       string
		set           *ml_dsa_87.SignatureBatch
		preFilledSets []*ml_dsa_87.SignatureBatch
		want          pubsub.ValidationResult
	}{
		{
			name:    "empty queue",
			message: "random",
			set:     validSet,
			want:    pubsub.ValidationAccept,
		},
		{
			name:    "invalid set",
			message: "random",
			set:     invalidSet,
			want:    pubsub.ValidationReject,
		},
		{
			name:          "invalid set in routine with valid set",
			message:       "random",
			set:           validSet,
			preFilledSets: []*ml_dsa_87.SignatureBatch{invalidSet},
			want:          pubsub.ValidationAccept,
		},
		{
			name:          "valid set in routine with invalid set",
			message:       "random",
			set:           invalidSet,
			preFilledSets: []*ml_dsa_87.SignatureBatch{validSet},
			want:          pubsub.ValidationReject,
		},
		{
			name:    "missing descriptions do not affect verification",
			message: "random",
			set:     missingDescriptions,
			want:    pubsub.ValidationAccept,
		},
		{
			name: "empty set",
			set:  ml_dsa_87.NewSet(),
			want: pubsub.ValidationReject,
		},
		{
			name: "nil set",
			want: pubsub.ValidationReject,
		},
		{
			name:    "missing signature groups",
			message: "random",
			set:     missingSignatures,
			want:    pubsub.ValidationReject,
		},
		{
			name:    "nil public key",
			message: "random",
			set:     nilPublicKey,
			want:    pubsub.ValidationReject,
		},
		{
			name:          "malformed set in routine with valid set",
			message:       "random",
			set:           validSet,
			preFilledSets: []*ml_dsa_87.SignatureBatch{missingSignatures},
			want:          pubsub.ValidationAccept,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newSignatureVerifierService(t)
			startSignatureTestWorkers(t, svc, 2, (*ml_dsa_87.SignatureBatch).VerifySequential)
			for _, st := range tt.preFilledSets {
				svc.signatureChan <- &signatureVerifier{ctx: svc.ctx, set: st.Copy(), resChan: make(chan error, 1)}
			}
			got, err := svc.validateSignatures(t.Context(), tt.message, tt.set)
			if got != tt.want {
				t.Errorf("validateSignatures() = %v, want %v", got, tt.want)
			}
			if err != nil && tt.want == pubsub.ValidationAccept {
				t.Errorf("Wanted no error but received: %v", err)
			}
		})
	}
}

func TestSignatureVerifier_IndependentResults(t *testing.T) {
	svc := newSignatureVerifierService(t)
	var calls [verifierLimit]atomic.Int32
	startSignatureTestWorkers(t, svc, 4, func(set *ml_dsa_87.SignatureBatch, ctx context.Context) (bool, error) {
		calls[set.Messages[0][0]].Add(1)
		return set.VerifySequential(ctx)
	})
	results := make([]<-chan signatureValidationResult, verifierLimit)
	for i := range results {
		set := newSignatureTestSet(t, byte(i))
		if i == 0 {
			set.Signatures[0][0][0] ^= 1
		}
		results[i] = validateSignaturesInBackground(svc, t.Context(), set)
	}
	for i, result := range results {
		got := awaitSignatureResult(t, result)
		if i == 0 {
			require.Equal(t, pubsub.ValidationReject, got.result)
			require.NotNil(t, got.err)
		} else {
			require.Equal(t, pubsub.ValidationAccept, got.result)
			require.NoError(t, got.err)
		}
		require.Equal(t, int32(1), calls[i].Load(), "each request must be verified only once")
	}
}

func TestValidateSignatures_DoesNotRetryFailedVerification(t *testing.T) {
	svc := newSignatureVerifierService(t)
	startSignatureTestWorkers(t, svc, 1, func(*ml_dsa_87.SignatureBatch, context.Context) (bool, error) {
		return false, nil
	})
	// The signature itself is valid. Retrying it outside the workers would turn
	// their failed verdict into an acceptance and bypass the concurrency limit.
	result, err := svc.validateSignatures(t.Context(), "test", newSignatureTestSet(t, 0))
	require.Equal(t, pubsub.ValidationReject, result)
	require.ErrorContains(t, "signature verification failed", err)
}

func TestSignatureVerifier_WorkerLimit(t *testing.T) {
	svc := newSignatureVerifierService(t)
	const workers = 2
	started := make(chan struct{}, verifierLimit)
	release := make(chan struct{})
	var active, peak atomic.Int32
	startSignatureTestWorkers(t, svc, workers, func(set *ml_dsa_87.SignatureBatch, ctx context.Context) (bool, error) {
		count := active.Add(1)
		defer active.Add(-1)
		for old := peak.Load(); count > old; old = peak.Load() {
			if peak.CompareAndSwap(old, count) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
			return set.VerifySequential(ctx)
		case <-ctx.Done():
			return false, ctx.Err()
		}
	})
	results := make([]<-chan signatureValidationResult, 10)
	set := newSignatureTestSet(t, 0)
	for i := range results {
		results[i] = validateSignaturesInBackground(svc, t.Context(), set)
	}
	for range workers {
		awaitSignatureResult(t, started)
	}
	close(release)
	for _, result := range results {
		got := awaitSignatureResult(t, result)
		require.NoError(t, got.err)
		require.Equal(t, pubsub.ValidationAccept, got.result)
	}
	require.Equal(t, int32(workers), peak.Load())
}

func TestValidateSignatures_Cancellation(t *testing.T) {
	set := newSignatureTestSet(t, 0)
	for _, stage := range []string{"before enqueue", "full queue", "waiting for result", "verifying"} {
		for _, shutdown := range []bool{false, true} {
			name := stage + "/caller"
			if shutdown {
				name = stage + "/shutdown"
			}
			t.Run(name, func(t *testing.T) {
				svc := newSignatureVerifierService(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				cancelRequest := cancel
				if shutdown {
					cancelRequest = svc.cancel
				}
				switch stage {
				case "before enqueue":
					cancelRequest()
				case "full queue":
					svc.signatureChan = make(chan *signatureVerifier)
					// No receiver is running, so enqueueing must wait for cancellation.
					timer := time.AfterFunc(20*time.Millisecond, cancelRequest)
					defer timer.Stop()
				}
				result := validateSignaturesInBackground(svc, ctx, set)
				if stage == "waiting for result" {
					request := awaitSignatureResult(t, svc.signatureChan)
					cancelRequest()
					got := awaitSignatureResult(t, result)
					require.Equal(t, pubsub.ValidationIgnore, got.result)
					require.ErrorIs(t, got.err, context.Canceled)
					// A worker can deliver its result after the caller has left.
					select {
					case request.resChan <- nil:
					default:
						t.Fatal("abandoned request would block a worker")
					}
					return
				}
				if stage == "verifying" {
					started := make(chan struct{}, 1)
					startSignatureTestWorkers(t, svc, 1, func(_ *ml_dsa_87.SignatureBatch, ctx context.Context) (bool, error) {
						started <- struct{}{}
						<-ctx.Done()
						return false, ctx.Err()
					})
					awaitSignatureResult(t, started)
					cancelRequest()
				}
				got := awaitSignatureResult(t, result)
				require.Equal(t, pubsub.ValidationIgnore, got.result)
				require.ErrorIs(t, got.err, context.Canceled)
			})
		}
	}
}

func TestSignatureVerifier_SkipsCanceledRequests(t *testing.T) {
	svc := newSignatureVerifierService(t)
	set := newSignatureTestSet(t, 0)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	abandoned := &signatureVerifier{ctx: ctx, set: set, resChan: make(chan error, 1)}
	svc.signatureChan <- abandoned
	var calls atomic.Int32
	startSignatureTestWorkers(t, svc, 1, func(set *ml_dsa_87.SignatureBatch, ctx context.Context) (bool, error) {
		calls.Add(1)
		return set.VerifySequential(ctx)
	})
	result, err := svc.validateSignatures(t.Context(), "test", set)
	require.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, result)
	require.ErrorIs(t, awaitSignatureResult(t, abandoned.resChan), context.Canceled)
	require.Equal(t, int32(1), calls.Load())
}

func TestValidateSignatures_Deadline(t *testing.T) {
	svc := newSignatureVerifierService(t)
	set := newSignatureTestSet(t, 0)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	result, err := svc.validateSignatures(ctx, "test", set)
	require.Equal(t, pubsub.ValidationIgnore, result)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

type signatureValidationResult struct {
	result pubsub.ValidationResult
	err    error
}

func validateSignaturesInBackground(svc *Service, ctx context.Context, set *ml_dsa_87.SignatureBatch) <-chan signatureValidationResult {
	result := make(chan signatureValidationResult, 1)
	go func() {
		validation, err := svc.validateSignatures(ctx, "test", set)
		result <- signatureValidationResult{result: validation, err: err}
	}()
	return result
}

func newSignatureVerifierService(t *testing.T) *Service {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	return &Service{ctx: ctx, cancel: cancel, signatureChan: make(chan *signatureVerifier, verifierLimit)}
}

func startSignatureTestWorkers(t *testing.T, svc *Service, count int, verify func(*ml_dsa_87.SignatureBatch, context.Context) (bool, error)) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		svc.runSignatureWorkers(count, verify)
		close(done)
	}()
	t.Cleanup(func() {
		svc.cancel()
		awaitSignatureResult(t, done)
	})
}

func newSignatureTestSet(t *testing.T, id byte) *ml_dsa_87.SignatureBatch {
	t.Helper()
	key, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	msg := [32]byte{id}
	sig, err := key.Sign(msg[:])
	require.NoError(t, err)
	return &ml_dsa_87.SignatureBatch{
		Messages: [][32]byte{msg}, Signatures: [][][]byte{{sig.Marshal()}},
		PublicKeys: [][]ml_dsa_87.PublicKey{{key.PublicKey()}},
	}
}

func awaitSignatureResult[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("signature verifier did not finish")
		var zero T
		return zero
	}
}
