package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type finalizationRetryDB struct {
	db.HeadAccessDatabase
	operation string
	failures  int
	failure   error
	cancel    context.CancelFunc
}

func (d *finalizationRetryDB) fail(ctx context.Context, operation string) error {
	if d.operation != operation || d.failures == 0 {
		return nil
	}
	d.failures--
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
		return ctx.Err()
	}
	return d.failure
}

func (d *finalizationRetryDB) SaveFinalizedCheckpoint(ctx context.Context, cp *qrysmpb.Checkpoint) error {
	if err := d.fail(ctx, "finality write"); err != nil {
		return err
	}
	return d.HeadAccessDatabase.SaveFinalizedCheckpoint(ctx, cp)
}

func (d *finalizationRetryDB) LastValidatedCheckpoint(ctx context.Context) (*qrysmpb.Checkpoint, error) {
	if err := d.fail(ctx, "validation read"); err != nil {
		return nil, err
	}
	return d.HeadAccessDatabase.LastValidatedCheckpoint(ctx)
}

func (d *finalizationRetryDB) SaveLastValidatedCheckpoint(ctx context.Context, cp *qrysmpb.Checkpoint) error {
	if err := d.fail(ctx, "validation write"); err != nil {
		return err
	}
	return d.HeadAccessDatabase.SaveLastValidatedCheckpoint(ctx, cp)
}

func (d *finalizationRetryDB) Block(ctx context.Context, root [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	// Make a request-bound notification reliably fail after cancellation,
	// even when the underlying DB could serve this particular block from cache.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return d.HeadAccessDatabase.Block(ctx, root)
}

func TestService_FinalizationSideEffectsOnValidationFailure(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, gossip := range []bool{false, true} {
		for _, tc := range []struct {
			name, operation string
			cancel          bool
		}{
			{name: "healthy control"},
			{name: "finality write fails", operation: "finality write"},
			{name: "validation read fails", operation: "validation read"},
			{name: "validation write fails", operation: "validation write"},
			{name: "validation write cancelled", operation: "validation write", cancel: true},
		} {
			t.Run(map[bool]string{false: "tick/", true: "gossip/"}[gossip]+tc.name, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 24)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 23, 0)
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:23]))
					synctest.Wait()
					events := make(chan *feed.Event, 16)
					sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
					defer sub.Unsubscribe()
					writeErr := errors.New("temporary checkpoint database failure")
					d := &finalizationRetryDB{HeadAccessDatabase: f.s.cfg.BeaconDB, operation: tc.operation, failure: writeErr}
					if tc.operation != "" {
						d.failures = 2
					}
					request, cancel := context.WithCancel(f.ctx)
					defer cancel()
					if tc.cancel {
						d.cancel = cancel
					}
					f.s.cfg.BeaconDB = d
					driftGenesisTime(f.s, 24, 0)
					var err error
					if gossip {
						err = f.s.ReceiveBlock(request, f.blks[23], f.blks[23].Root())
					} else {
						err = f.s.NewSlot(request, 24)
					}
					switch {
					case tc.cancel:
						require.ErrorIs(t, err, context.Canceled)
					case tc.operation != "":
						require.ErrorIs(t, err, writeErr)
					default:
						require.NoError(t, err)
					}
					synctest.Wait()
					count := 0
					drainFinalizedEvents := func() {
						for len(events) > 0 {
							event := <-events
							if event.Type != statefeed.FinalizedCheckpoint {
								continue
							}
							count++
							finalized := event.Data.(*qrlpb.EventFinalizedCheckpoint)
							assert.Equal(t, primitives.Epoch(2), finalized.Epoch)
							assert.Equal(t, f.blks[11].Root(), bytesutil.ToBytes32(finalized.Block))
							assert.Equal(t, false, finalized.ExecutionOptimistic)
						}
					}
					drainFinalizedEvents()
					wantEvents := 1
					wantEpoch := primitives.Epoch(2)
					if tc.operation == "finality write" {
						wantEvents, wantEpoch = 0, 0
					}
					assert.Equal(t, wantEvents, count, "notify durable finality even while its validation marker is pending")
					saved, err := d.FinalizedCheckpoint(f.ctx)
					require.NoError(t, err)
					assert.Equal(t, wantEpoch, saved.Epoch)
					for slot := primitives.Slot(25); slot <= 27; slot++ {
						driftGenesisTime(f.s, int64(slot), 0)
						err := f.s.NewSlot(f.ctx, slot)
						if slot == 25 && tc.operation != "" {
							require.ErrorIs(t, err, writeErr)
						} else {
							require.NoError(t, err)
						}
						synctest.Wait()
						drainFinalizedEvents()
					}
					saved, err = d.FinalizedCheckpoint(f.ctx)
					require.NoError(t, err)
					validated, err := d.LastValidatedCheckpoint(f.ctx)
					require.NoError(t, err)
					assert.Equal(t, primitives.Epoch(2), saved.Epoch)
					assert.DeepSSZEqual(t, saved, validated)
					assert.Equal(t, 1, count, "successful retries must not repeat finalization notifications")
				})
			})
		}
	}
}
