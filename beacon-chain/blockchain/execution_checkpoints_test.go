package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/verification"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	payloadattribute "github.com/theQRL/qrysm/consensus-types/payload-attribute"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type recordedExecutionCheckpoint struct{ head, safe, finalized [32]byte }
type checkpointRecordingEngine struct {
	*mockExecution.EngineClient
	checkpoints []recordedExecutionCheckpoint
}

func (e *checkpointRecordingEngine) ForkchoiceUpdated(ctx context.Context, fcs *enginev1.ForkchoiceState, attr payloadattribute.Attributer) (*enginev1.PayloadIDBytes, []byte, error) {
	e.checkpoints = append(e.checkpoints, recordedExecutionCheckpoint{
		head: bytesutil.ToBytes32(fcs.HeadBlockHash), safe: bytesutil.ToBytes32(fcs.SafeBlockHash), finalized: bytesutil.ToBytes32(fcs.FinalizedBlockHash),
	})
	return e.EngineClient.ForkchoiceUpdated(ctx, fcs, attr)
}

type interruptedValidForkchoiceEngine struct {
	*mockExecution.EngineClient
	hash      [32]byte
	interrupt func()
	calls     int
}

func (e *interruptedValidForkchoiceEngine) ForkchoiceUpdated(ctx context.Context, fcs *enginev1.ForkchoiceState, attr payloadattribute.Attributer) (*enginev1.PayloadIDBytes, []byte, error) {
	pid, lvh, err := e.EngineClient.ForkchoiceUpdated(ctx, fcs, attr)
	if bytesutil.ToBytes32(fcs.HeadBlockHash) == e.hash {
		e.calls++
		if err == nil && e.interrupt != nil {
			interrupt := e.interrupt
			e.interrupt = nil
			interrupt()
		}
	}
	return pid, lvh, err
}

func TestReceiveBlockBatch_RetainsValidForkchoiceOnCancellation(t *testing.T) {
	for _, mode := range []string{"healthy", "cancelled", "deadline exceeded"} {
		t.Run(mode, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 3)
			incoming := f.blks[2]
			alternate, _ := emptyBranchBlock(t, f, f.states[1], 4, 'd')
			payload, err := incoming.Block().Body().Execution()
			require.NoError(t, err)
			// NewPayload stays SYNCING; only FCU validates this branch.
			f.engine.ErrForkchoiceUpdated = nil
			engine := &interruptedValidForkchoiceEngine{EngineClient: f.engine, hash: bytesutil.ToBytes32(payload.BlockHash())}
			f.s.cfg.ExecutionEngineCaller = engine
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 4, 0)
				ctx, cancel := context.WithCancel(f.ctx)
				defer cancel()
				var wantErr error
				switch mode {
				case "cancelled":
					engine.interrupt = cancel
					wantErr = context.Canceled
				case "deadline exceeded":
					var cancelDeadline context.CancelFunc
					ctx, cancelDeadline = context.WithTimeout(ctx, time.Second)
					defer cancelDeadline()
					engine.interrupt = func() { <-ctx.Done() }
					wantErr = context.DeadlineExceeded
				}
				events := make(chan *feed.Event, 32)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				err = f.s.ReceiveBlockBatch(ctx, []blocks.ROBlock{incoming})
				wantHead := incoming.Root()
				if wantErr == nil {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, wantErr)
					assert.Equal(t, false, errors.Is(err, verification.ErrInvalid))
					wantHead = f.blks[1].Root()
				}
				published, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, wantHead, bytesutil.ToBytes32(published), "cancellation still stops head publication")
				synctest.Wait()
				require.Equal(t, 1, engine.calls)
				for _, b := range f.blks[1:] {
					optimistic, err := f.s.IsOptimisticForRoot(f.ctx, b.Root())
					require.NoError(t, err)
					assert.Equal(t, false, optimistic, "retain VALID for the block and its ancestors")
				}
				optimistic, err := f.s.IsOptimistic(f.ctx)
				require.NoError(t, err)
				assert.Equal(t, false, optimistic, "refresh cached head optimism even when head publication stops")
				requireBlockEventOptimism(t, events, []blocks.ROBlock{incoming}, incoming.Block().Slot())
				// A sibling becomes head before another FCU can repair C3.
				// Duplicate gossip and later head updates must preserve C3's VALID.
				driftGenesisTime(f.s, int64(params.BeaconConfig().SlotsPerEpoch)+1, 0)
				voteForRoot(t, f, alternate.Root(), 1)
				f.engine.ErrNewPayload = nil
				require.NoError(t, f.s.ReceiveBlock(f.ctx, alternate, alternate.Root()))
				synctest.Wait()
				require.Equal(t, alternate.Root(), f.s.CachedHeadRoot())
				for i := 0; i < 2; i++ {
					require.NoError(t, f.s.ReceiveBlock(f.ctx, incoming, incoming.Root()))
					f.s.UpdateHead(f.ctx, f.s.CurrentSlot())
					synctest.Wait()
				}
				optimistic, err = f.s.IsOptimisticForRoot(f.ctx, incoming.Root())
				require.NoError(t, err)
				assert.Equal(t, false, optimistic)
				require.Equal(t, 1, engine.calls, "head updates on another branch cannot repair this verdict")
			})
		})
	}
}

type failedForkchoiceValidationStore struct {
	forkchoice.ForkChoicer
	failure error
}

func (s *failedForkchoiceValidationStore) SetOptimisticToValid(ctx context.Context, root [32]byte) error {
	if s.failure != nil {
		err := s.failure
		s.failure = nil
		return err
	}
	return s.ForkChoicer.SetOptimisticToValid(ctx, root)
}

func TestNotifyForkchoiceUpdate_ValidationFailureRetries(t *testing.T) {
	f := newBatchExecutionFixture(t, 3)
	f.engine.ErrForkchoiceUpdated = nil
	engine := &checkpointRecordingEngine{EngineClient: f.engine}
	f.s.cfg.ExecutionEngineCaller = engine
	failure := errors.New("temporary forkchoice validation failure")
	f.s.cfg.ForkChoiceStore = &failedForkchoiceValidationStore{ForkChoicer: f.s.cfg.ForkChoiceStore, failure: failure}
	synctest.Test(t, func(t *testing.T) {
		t.Cleanup(synctest.Wait)
		driftGenesisTime(f.s, 4, 0)
		incoming := f.blks[2]
		err := f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{incoming})
		require.ErrorIs(t, err, failure)
		assert.Equal(t, false, errors.Is(err, verification.ErrInvalid))
		published, err := f.s.HeadRoot(f.ctx)
		require.NoError(t, err)
		require.Equal(t, f.blks[1].Root(), bytesutil.ToBytes32(published))
		require.Equal(t, true, f.s.lastForkchoiceUpdate == nil, "failed validation must not acknowledge the FCU")
		require.Equal(t, true, f.s.cfg.ForkChoiceStore.HasNode(incoming.Root()))
		require.Equal(t, 1, len(engine.checkpoints))
		// The accepted batch can retry the failed local update without replay.
		require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{incoming}))
		synctest.Wait()
		require.Equal(t, incoming.Root(), f.s.CachedHeadRoot())
		optimistic, err := f.s.IsOptimisticForRoot(f.ctx, incoming.Root())
		require.NoError(t, err)
		assert.Equal(t, false, optimistic)
		require.Equal(t, 2, len(engine.checkpoints))
		f.s.UpdateHead(f.ctx, 4)
		require.Equal(t, 2, len(engine.checkpoints), "deduplicate once validation succeeds")
	})
}

func TestService_UnchangedOptimisticHeadRetry(t *testing.T) {
	reset := features.InitWithReset(&features.Flags{})
	t.Cleanup(reset)
	f := newBatchExecutionFixture(t, 2)
	engine := &checkpointRecordingEngine{EngineClient: f.engine}
	f.s.cfg.ExecutionEngineCaller = engine
	optimistic, err := f.s.IsOptimistic(f.ctx)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)
	synctest.Test(t, func(t *testing.T) {
		t.Cleanup(synctest.Wait)
		events := make(chan *feed.Event, 16)
		sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
		defer sub.Unsubscribe()
		// A status-only update must not prune operations or republish the head.
		require.NoError(t, f.s.cfg.AttPool.SaveUnaggregatedAttestation(f.blks[1].Block().Body().Attestations()[0]))
		for slot := primitives.Slot(3); slot <= 6; slot++ {
			if slot == 5 {
				// Execution finishes syncing without any new beacon blocks.
				f.engine.ErrForkchoiceUpdated = nil
			}
			driftGenesisTime(f.s, int64(slot), 0)
			require.NoError(t, f.s.NewSlot(f.ctx, slot))
			f.s.UpdateHead(f.ctx, slot)
			synctest.Wait()
			require.Equal(t, min(int(slot)-2, 3), len(engine.checkpoints), "retry SYNCING, then deduplicate VALID")
			optimistic, err := f.s.IsOptimistic(f.ctx)
			require.NoError(t, err)
			assert.Equal(t, slot < 5, optimistic)
			assert.Equal(t, f.blks[1].Root(), f.s.CachedHeadRoot())
			driftGenesisTime(f.s, int64(slot), -20)
			f.s.lateBlockTasks(f.ctx)
			synctest.Wait()
		}
		assert.Equal(t, 1, f.s.cfg.AttPool.UnaggregatedAttestationCount())
		for len(events) > 0 {
			typ := (<-events).Type
			assert.NotEqual(t, statefeed.NewHead, typ)
			assert.NotEqual(t, statefeed.Reorg, typ)
		}
	})
}

func TestService_TickExecutionFinality(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tc := range []struct {
		name      string
		nextBlock bool
		engineErr error
	}{
		{name: "unchanged head"},
		{name: "syncing response", engineErr: execution.ErrAcceptedSyncingPayloadStatus},
		{name: "retry failed notification", engineErr: errors.New("engine unavailable")},
		{name: "next head control", nextBlock: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 24)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			engine := &checkpointRecordingEngine{EngineClient: f.engine}
			f.s.cfg.ExecutionEngineCaller = engine
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 23, 0)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:23]))
				synctest.Wait()
				require.Equal(t, 1, len(engine.checkpoints))
				before := engine.checkpoints[0]
				require.Equal(t, primitives.Epoch(0), f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				oldHead, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				events := make(chan *feed.Event, 16)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				f.engine.ErrForkchoiceUpdated = tc.engineErr
				driftGenesisTime(f.s, 24, 0)
				require.NoError(t, f.s.NewSlot(f.ctx, 24))
				f.s.UpdateHead(f.ctx, 24)
				synctest.Wait()
				require.Equal(t, 2, len(engine.checkpoints), "checkpoint change must notify execution immediately")
				calls := 2
				if tc.engineErr != nil {
					f.engine.ErrForkchoiceUpdated = nil
					f.s.UpdateHead(f.ctx, 24)
					synctest.Wait()
					calls++
					require.Equal(t, calls, len(engine.checkpoints), "pending validation must be retried")
				}
				require.Equal(t, primitives.Epoch(2), f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				currentHead, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.DeepEqual(t, oldHead, currentHead)
				finalizedPayload, err := f.blks[11].Block().Body().Execution()
				require.NoError(t, err)
				want := bytesutil.ToBytes32(finalizedPayload.BlockHash())
				require.NotEqual(t, before.finalized, want)
				last := engine.checkpoints[len(engine.checkpoints)-1]
				assert.Equal(t, before.head, last.head)
				assert.Equal(t, want, last.finalized)
				for len(events) > 0 {
					typ := (<-events).Type
					assert.NotEqual(t, statefeed.NewHead, typ, "checkpoint-only update must not publish a head event")
					assert.NotEqual(t, statefeed.Reorg, typ)
				}
				// Repeated calls with validated checkpoints must be deduplicated.
				f.s.UpdateHead(f.ctx, 24)
				driftGenesisTime(f.s, 25, 0)
				require.NoError(t, f.s.NewSlot(f.ctx, 25))
				f.s.UpdateHead(f.ctx, 25)
				synctest.Wait()
				require.Equal(t, calls, len(engine.checkpoints))
				if tc.nextBlock {
					require.NoError(t, f.s.ReceiveBlock(f.ctx, f.blks[23], f.blks[23].Root()))
					synctest.Wait()
					require.Equal(t, calls+1, len(engine.checkpoints))
					assert.Equal(t, want, engine.checkpoints[calls].finalized)
				}
			})
		})
	}
}

type executionSafeHashStore struct {
	forkchoice.ForkChoicer
	safeHash [32]byte
}

func (s *executionSafeHashStore) UnrealizedJustifiedPayloadBlockHash() [32]byte {
	return s.safeHash
}

func TestService_ExecutionSafeHashUpdate(t *testing.T) {
	f := newBatchExecutionFixture(t, 2)
	f.engine.ErrForkchoiceUpdated = nil
	engine := &checkpointRecordingEngine{EngineClient: f.engine}
	f.s.cfg.ExecutionEngineCaller = engine
	payload, err := f.blks[0].Block().Body().Execution()
	require.NoError(t, err)
	store := &executionSafeHashStore{ForkChoicer: f.s.cfg.ForkChoiceStore, safeHash: bytesutil.ToBytes32(payload.BlockHash())}
	f.s.cfg.ForkChoiceStore = store
	synctest.Test(t, func(t *testing.T) {
		t.Cleanup(synctest.Wait)
		driftGenesisTime(f.s, 3, 0)
		// Retain an attestation included in the existing head to detect any
		// accidental pruning during a checkpoint-only notification.
		require.NoError(t, f.s.cfg.AttPool.SaveUnaggregatedAttestation(f.blks[1].Block().Body().Attestations()[0]))
		f.s.UpdateHead(f.ctx, 3)
		synctest.Wait()
		require.Equal(t, 1, len(engine.checkpoints))
		require.Equal(t, store.safeHash, engine.checkpoints[0].safe)
		require.Equal(t, [32]byte{}, engine.checkpoints[0].finalized)
		require.Equal(t, f.blks[1].Root(), f.s.CachedHeadRoot())
		require.Equal(t, 1, f.s.cfg.AttPool.UnaggregatedAttestationCount())
		f.s.UpdateHead(f.ctx, 3)
		synctest.Wait()
		require.Equal(t, 1, len(engine.checkpoints))

		// An RPC error leaves the engine's state uncertain. If a safe candidate
		// is discarded afterwards, resend the previously accepted checkpoint
		// too: the failed request may already have changed execution's view.
		acceptedSafe := store.safeHash
		store.safeHash = [32]byte{}
		f.engine.ErrForkchoiceUpdated = errors.New("engine response lost")
		f.s.UpdateHead(f.ctx, 3)
		synctest.Wait()
		require.Equal(t, 2, len(engine.checkpoints))
		store.safeHash = acceptedSafe
		f.engine.ErrForkchoiceUpdated = nil
		f.s.UpdateHead(f.ctx, 3)
		synctest.Wait()
		require.Equal(t, 3, len(engine.checkpoints))
		require.Equal(t, acceptedSafe, engine.checkpoints[2].safe)
		f.s.UpdateHead(f.ctx, 3)
		synctest.Wait()
		require.Equal(t, 3, len(engine.checkpoints))
	})
}

func TestService_ExecutionUpdateAfterProposal(t *testing.T) {
	for _, reorgDuringRPC := range []bool{false, true} {
		for _, rpcFailure := range []bool{false, true} {
			name := map[bool]string{false: "proposal before reorg", true: "proposal finishes after reorg"}[reorgDuringRPC]
			name += map[bool]string{false: "/VALID", true: "/response lost"}[rpcFailure]
			t.Run(name, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 2)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				replacement, _ := emptyBranchBlock(t, f, f.states[1].Copy(), 3, 'r')
				oldPayload, err := f.blks[1].Block().Body().Execution()
				require.NoError(t, err)
				newPayload, err := replacement.Block().Body().Execution()
				require.NoError(t, err)
				require.NotEqual(t, bytesutil.ToBytes32(oldPayload.BlockHash()), bytesutil.ToBytes32(newPayload.BlockHash()))
				engine := &checkpointRecordingEngine{EngineClient: f.engine}
				f.s.cfg.ExecutionEngineCaller = engine
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 3, 0)
					f.s.cfg.ForkChoiceStore.Lock()
					err := f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot})
					f.s.cfg.ForkChoiceStore.Unlock()
					require.NoError(t, err)
					f.s.UpdateHead(f.ctx, 3)
					require.Equal(t, 1, len(engine.checkpoints))
					f.s.InvalidateForkchoiceUpdate()
					reorg := func() {
						require.NoError(t, f.s.ReceiveBlock(f.ctx, replacement, replacement.Root()))
						require.Equal(t, replacement.Root(), f.s.CachedHeadRoot())
					}
					if reorgDuringRPC {
						reorg()
					}
					if rpcFailure {
						f.engine.ErrForkchoiceUpdated = errors.New("proposal FCU response lost")
					}
					// The proposer uses the same engine independently of head
					// updates. Its stale parent can be accepted after the reorg.
					_, _, err = engine.ForkchoiceUpdated(f.ctx, &enginev1.ForkchoiceState{HeadBlockHash: oldPayload.BlockHash()}, nil)
					if rpcFailure {
						require.ErrorIs(t, err, f.engine.ErrForkchoiceUpdated)
					} else {
						require.NoError(t, err)
					}
					f.s.InvalidateForkchoiceUpdate()
					f.engine.ErrForkchoiceUpdated = nil
					if !reorgDuringRPC {
						reorg()
					}
					before := len(engine.checkpoints)
					f.s.UpdateHead(f.ctx, 3)
					wantCalls := before
					if reorgDuringRPC {
						wantCalls++
					}
					require.Equal(t, wantCalls, len(engine.checkpoints), "reconcile after a late proposal response")
					require.Equal(t, bytesutil.ToBytes32(newPayload.BlockHash()), engine.checkpoints[wantCalls-1].head)
					published, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					require.Equal(t, replacement.Root(), bytesutil.ToBytes32(published))
					f.s.UpdateHead(f.ctx, 3)
					require.Equal(t, wantCalls, len(engine.checkpoints), "deduplicate again after successful reconciliation")
				})
			})
		}
	}
}
