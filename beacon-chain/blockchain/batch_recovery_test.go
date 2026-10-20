package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/verification"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type retainedBatchPrefixStore struct {
	forkchoice.ForkChoicer
	failure error
	cancel  context.CancelFunc
}

func (f *retainedBatchPrefixStore) InsertChain(ctx context.Context, chain []*forktypes.BlockAndCheckpoints) error {
	if err := f.ForkChoicer.InsertChain(ctx, chain[:1]); err != nil {
		return err
	}
	if f.cancel != nil {
		f.cancel()
		return ctx.Err()
	}
	return f.failure
}

func TestReceiveBlockBatch_FailedPrefixHeadRecovery(t *testing.T) {
	for _, mode := range []string{"partial insertion", "cancelled insertion", "invalid execution tail"} {
		t.Run(mode, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 4)
			fc := f.s.cfg.ForkChoiceStore
			failure := errors.New("temporary insertion failure")
			if mode == "invalid execution tail" {
				payload, err := f.blks[2].Block().Body().Execution()
				require.NoError(t, err)
				f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
				f.engine.ForkChoiceUpdatedResp = payload.BlockHash()
				f.engine.OverrideValidHash = bytesutil.ToBytes32(payload.BlockHash())
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 5, 0)
				ctx, cancel := context.WithCancel(f.ctx)
				defer cancel()
				if mode != "invalid execution tail" {
					store := &retainedBatchPrefixStore{ForkChoicer: fc, failure: failure}
					if mode == "cancelled insertion" {
						store.cancel = cancel
						failure = context.Canceled
					}
					f.s.cfg.ForkChoiceStore = store
				}
				events := make(chan *feed.Event, 32)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				err := f.s.ReceiveBlockBatch(ctx, f.blks[2:])
				retained := f.blks[2].Root()
				if mode == "invalid execution tail" {
					require.Equal(t, true, IsInvalidBlock(err))
					require.Equal(t, f.blks[3].Root(), InvalidBlockRoot(err))
					head, headErr := f.s.HeadRoot(f.ctx)
					require.NoError(t, headErr)
					require.Equal(t, retained, bytesutil.ToBytes32(head), "FCU must recover the retained head immediately")
					require.Equal(t, false, f.s.cfg.BeaconDB.HasBlock(f.ctx, f.blks[3].Root()))
				} else {
					require.ErrorIs(t, err, failure)
				}
				synctest.Wait()
				f.s.cfg.ForkChoiceStore = fc
				f.engine.ErrForkchoiceUpdated = nil
				require.Equal(t, true, fc.HasNode(retained))
				require.Equal(t, false, fc.HasNode(f.blks[3].Root()))
				require.Equal(t, true, f.s.cfg.BeaconDB.HasBlock(f.ctx, retained), "retained blocks must be available to state regeneration")
				require.NoError(t, f.s.NewSlot(f.ctx, 5))
				f.s.UpdateHead(f.ctx, 5)
				synctest.Wait()
				head, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				assert.Equal(t, retained, bytesutil.ToBytes32(head))
				assert.Equal(t, retained, f.s.CachedHeadRoot())
				counts := processedBlockCounts(events)
				assert.Equal(t, 1, counts[retained])
				assert.Equal(t, 0, counts[f.blks[3].Root()])
			})
		})
	}
}

type batchBlockSaveFailureDB struct {
	db.HeadAccessDatabase
	failure error
	cancel  context.CancelFunc
}

func (d *batchBlockSaveFailureDB) SaveBlocks(ctx context.Context, blks []interfaces.ReadOnlySignedBeaconBlock) error {
	if d.cancel != nil {
		d.cancel()
		d.cancel = nil
		return ctx.Err()
	}
	if d.failure != nil {
		err := d.failure
		d.failure = nil
		return err
	}
	return d.HeadAccessDatabase.SaveBlocks(ctx, blks)
}

func TestReceiveBlockBatch_SaveFailureBeforeInsertion(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "write failure", true: "cancelled write"}[cancelled], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 4)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 5, 0)
				ctx, cancel := context.WithCancel(f.ctx)
				defer cancel()
				failure := errors.New("temporary block write failure")
				d := &batchBlockSaveFailureDB{HeadAccessDatabase: f.s.cfg.BeaconDB, failure: failure}
				if cancelled {
					d.failure, d.cancel = nil, cancel
					failure = context.Canceled
				}
				f.s.cfg.BeaconDB = d
				err := f.s.ReceiveBlockBatch(ctx, f.blks[2:])
				require.ErrorIs(t, err, failure)
				require.Equal(t, false, errors.Is(err, verification.ErrInvalid))
				for _, b := range f.blks[2:] {
					require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(b.Root()), "failed persistence must precede insertion")
				}
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				synctest.Wait()
				require.Equal(t, primitives.Slot(4), f.s.HeadSlot())
				st, err := f.s.cfg.StateGen.StateByRoot(f.ctx, f.blks[2].Root())
				require.NoError(t, err)
				root, err := st.HashTreeRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, f.blks[2].Block().StateRoot(), root)
			})
		})
	}
}

func TestReceiveBlockBatch_RetainedPrefixExecutionValidity(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		for _, validSlot := range []int{3, 4} {
			name := map[bool]string{false: "insertion failure", true: "cancelled insertion"}[cancelled] + map[int]string{3: "/valid retained block", 4: "/valid uninserted descendant"}[validSlot]
			t.Run(name, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 4)
				fc := f.s.cfg.ForkChoiceStore
				payload, err := f.blks[validSlot-1].Block().Body().Execution()
				require.NoError(t, err)
				f.s.cfg.ExecutionEngineCaller = &batchMixedValidationEngine{EngineClient: f.engine, validHash: bytesutil.ToBytes32(payload.BlockHash())}
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 5, 0)
					ctx, cancel := context.WithCancel(f.ctx)
					defer cancel()
					failure := errors.New("temporary insertion failure")
					store := &retainedBatchPrefixStore{ForkChoicer: fc, failure: failure}
					if cancelled {
						store.cancel = cancel
						failure = context.Canceled
					}
					f.s.cfg.ForkChoiceStore = store
					events := make(chan *feed.Event, 16)
					sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
					defer sub.Unsubscribe()
					require.ErrorIs(t, f.s.ReceiveBlockBatch(ctx, f.blks[2:]), failure)
					synctest.Wait()
					optimistic, err := fc.IsOptimistic(f.blks[2].Root())
					require.NoError(t, err)
					require.Equal(t, false, optimistic, "the retained prefix must keep its VALID payload verdict")
					requireBlockEventOptimism(t, events, f.blks[2:3], 3)
					f.s.cfg.ForkChoiceStore = fc
					f.s.cfg.ExecutionEngineCaller = f.engine
					// The retry skips C3 and receives only SYNCING responses.
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
					synctest.Wait()
					optimistic, err = fc.IsOptimistic(f.blks[2].Root())
					require.NoError(t, err)
					require.Equal(t, false, optimistic)
					requireBlockEventOptimism(t, events, f.blks[3:], 3)
				})
			})
		}
	}
}

func TestReceiveBlockBatch_FinalizedRetry(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, operation := range []string{"finality write", "validation write"} {
		for _, suffix := range []bool{false, true} {
			name := operation + map[bool]string{false: "/known batch", true: "/new suffix"}[suffix]
			t.Run(name, func(t *testing.T) {
				count := 24
				if suffix {
					count++
				}
				f := newBatchExecutionFixture(t, count)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				failure := errors.New("temporary checkpoint write failure")
				d := &finalizationRetryDB{HeadAccessDatabase: f.s.cfg.BeaconDB, operation: operation, failures: 2, failure: failure}
				f.s.cfg.BeaconDB = d
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, int64(count), 0)
					events := make(chan *feed.Event, 64)
					sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
					defer sub.Unsubscribe()
					require.ErrorIs(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:24]), failure)
					synctest.Wait()
					require.Equal(t, primitives.Epoch(2), f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
					require.Equal(t, primitives.Slot(2), f.s.HeadSlot())
					require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(f.blks[2].Root()), "the accepted prefix was pruned by finality")
					// The stale HeadSlot lets this prefix through initial sync's
					// duplicate filter. Retry local work even if it fails again.
					err := f.s.ReceiveBlockBatch(f.ctx, f.blks[2:])
					require.ErrorIs(t, err, failure)
					require.Equal(t, false, errors.Is(err, verification.ErrInvalid))
					synctest.Wait()
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
					synctest.Wait()
					require.Equal(t, primitives.Slot(count), f.s.HeadSlot())
					cp, err := d.FinalizedCheckpoint(f.ctx)
					require.NoError(t, err)
					validated, err := d.LastValidatedCheckpoint(f.ctx)
					require.NoError(t, err)
					require.Equal(t, primitives.Epoch(2), cp.Epoch)
					require.DeepSSZEqual(t, cp, validated)
					counts := processedBlockCounts(events)
					for _, b := range f.blks[2:] {
						assert.Equal(t, 1, counts[b.Root()], "a retry must not repeat accepted block notifications")
					}
				})
			})
		}
	}
}

func TestReceiveBlockBatch_FinalizedRetryRejectsInvalidBlocks(t *testing.T) {
	setupEpochTransitionTest(t)
	f := newBatchExecutionFixture(t, 25)
	f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
	orphan, _ := emptyBranchBlock(t, f, f.states[2], 3, 'x')
	bad, err := f.blks[24].Copy()
	require.NoError(t, err)
	pb, err := bad.PbZondBlock()
	require.NoError(t, err)
	pb.Signature[0] ^= 0xff
	bad, err = blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)
	badSuffix, err := blocks.NewROBlock(bad)
	require.NoError(t, err)
	synctest.Test(t, func(t *testing.T) {
		t.Cleanup(synctest.Wait)
		driftGenesisTime(f.s, 25, 0)
		require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:24]))
		synctest.Wait()
		for _, tc := range []struct {
			name string
			blks []blocks.ROBlock
		}{
			{name: "conflicting finalized block", blks: []blocks.ROBlock{orphan}},
			{name: "disconnected prefix before known blocks", blks: append([]blocks.ROBlock{orphan}, f.blks[3:24]...)},
			{name: "invalid new suffix", blks: append(append([]blocks.ROBlock(nil), f.blks[2:24]...), badSuffix)},
		} {
			err := f.s.ReceiveBlockBatch(f.ctx, tc.blks)
			require.Equal(t, true, errors.Is(err, verification.ErrInvalid), tc.name)
			require.Equal(t, primitives.Slot(24), f.s.HeadSlot(), tc.name)
			require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(orphan.Root()), tc.name)
			require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(badSuffix.Root()), tc.name)
		}
	})
}
