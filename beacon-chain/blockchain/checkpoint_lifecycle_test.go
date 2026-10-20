package blockchain

import (
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func restartFromPersistedFinality(t *testing.T, f *batchExecutionFixture) {
	t.Helper()
	cp, err := f.s.cfg.BeaconDB.FinalizedCheckpoint(f.ctx)
	require.NoError(t, err)
	root := f.s.ensureRootNotZeros(bytesutil.ToBytes32(cp.Root))
	b, err := f.s.cfg.BeaconDB.Block(f.ctx, root)
	require.NoError(t, err)
	f.s.head = nil
	f.s.cfg.ForkChoiceStore = doublylinkedtree.New()
	f.s.cfg.StateGen = stategen.New(f.s.cfg.BeaconDB, f.s.cfg.ForkChoiceStore)
	f.s.cfg.ForkChoiceStore.SetBalancesByRooter(f.s.cfg.StateGen.BalancesByCheckpoint)
	require.NoError(t, f.s.setupForkchoice(f.states[b.Block().Slot()].Copy()))
}

func TestService_LaterExecutionValidation(t *testing.T) {
	setupEpochTransitionTest(t)
	reset := features.InitWithReset(&features.Flags{})
	t.Cleanup(reset)
	for _, mode := range []string{"batch", "gossip", "forkchoice update"} {
		for _, initiallyValid := range []bool{true, false} {
			t.Run(mode+map[bool]string{true: "/already validated control", false: "/optimistic then validated"}[initiallyValid], func(t *testing.T) {
				f := newBatchExecutionFixture(t, 25)
				if initiallyValid {
					f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				}
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 24, 0)
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:24]))
					synctest.Wait()
					before := *f.s.cfg.ForkChoiceStore.FinalizedCheckpoint()
					require.Equal(t, primitives.Epoch(2), before.Epoch)
					events := make(chan *feed.Event, 16)
					sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
					defer sub.Unsubscribe()
					// Only newPayload returns VALID during block import. The
					// FCU-only case tests validation without importing a block.
					f.engine.ErrNewPayload = nil
					f.engine.ErrForkchoiceUpdated = execution.ErrAcceptedSyncingPayloadStatus
					driftGenesisTime(f.s, 25, 0)
					if mode == "batch" {
						require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[24:]))
					} else if mode == "gossip" {
						require.NoError(t, f.s.ReceiveBlock(f.ctx, f.blks[24], f.blks[24].Root()))
					} else {
						f.engine.ErrForkchoiceUpdated = nil
						f.s.cfg.ForkChoiceStore.Lock()
						_, err := f.s.notifyForkchoiceUpdate(f.ctx, &notifyForkchoiceUpdateArg{
							headState: f.states[24].Copy(),
							headBlock: f.blks[23].Block(),
							headRoot:  f.blks[23].Root(),
						})
						f.s.cfg.ForkChoiceStore.Unlock()
						require.NoError(t, err)
					}
					synctest.Wait()
					require.Equal(t, before, *f.s.cfg.ForkChoiceStore.FinalizedCheckpoint())
					optimistic, err := f.s.cfg.ForkChoiceStore.IsOptimistic(before.Root)
					require.NoError(t, err)
					require.Equal(t, false, optimistic)
					validated, err := f.s.cfg.BeaconDB.LastValidatedCheckpoint(f.ctx)
					require.NoError(t, err)
					historical, err := f.s.IsOptimisticForRoot(f.ctx, f.blks[10].Root())
					require.NoError(t, err)
					assert.Equal(t, before.Epoch, validated.Epoch)
					assert.Equal(t, before.Root, bytesutil.ToBytes32(validated.Root))
					assert.Equal(t, false, historical)
					for len(events) > 0 {
						assert.NotEqual(t, statefeed.FinalizedCheckpoint, (<-events).Type, "validation alone must not emit a finalized event")
					}
					restartFromPersistedFinality(t, f)
					optimistic, err = f.s.cfg.ForkChoiceStore.IsOptimistic(before.Root)
					require.NoError(t, err)
					assert.Equal(t, false, optimistic)
				})
			})
		}
	}
}

func TestService_TickCheckpointPersistence(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tc := range []struct {
		name                                                      string
		tickFirst, headFirst, importNextBlock, batch, unsavedTick bool
		tickOffset                                                primitives.Slot
	}{
		{name: "block before tick control", importNextBlock: true},
		{name: "head before epoch tick", headFirst: true},
		{name: "first tick after epoch boundary", tickOffset: 1},
		{name: "tick before gossip block", tickFirst: true, importNextBlock: true},
		{name: "gossip repairs unsaved tick", tickFirst: true, importNextBlock: true, unsavedTick: true},
		{name: "tick without next block", tickFirst: true},
		{name: "tick before batch control", tickFirst: true, importNextBlock: true, batch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 24)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			// This valid branch skips the block at finalized slot 12.
			pb, err := util.GenerateFullBlockZond(f.states[11].Copy(), f.keys, &util.BlockGenConfig{}, 13)
			require.NoError(t, err)
			signed, err := blocks.NewSignedBeaconBlock(pb)
			require.NoError(t, err)
			conflicting, err := blocks.NewROBlock(signed)
			require.NoError(t, err)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 23, 0)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:23]))
				synctest.Wait()
				require.Equal(t, primitives.Epoch(0), f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				events := make(chan *feed.Event, 16)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				driftGenesisTime(f.s, 24, 0)
				if tc.headFirst {
					f.s.UpdateHead(f.ctx, 24)
					require.Equal(t, primitives.Epoch(3), f.s.cfg.ForkChoiceStore.JustifiedCheckpoint().Epoch)
					require.Equal(t, primitives.Epoch(2), f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				}
				if tc.tickFirst {
					if tc.unsavedTick {
						// Model an earlier tick whose DB write did not complete.
						f.s.cfg.ForkChoiceStore.Lock()
						err := f.s.cfg.ForkChoiceStore.NewSlot(f.ctx, 24)
						f.s.cfg.ForkChoiceStore.Unlock()
						require.NoError(t, err)
					} else {
						require.NoError(t, f.s.NewSlot(f.ctx, 24))
					}
					f.s.UpdateHead(f.ctx, 24)
				}
				if tc.importNextBlock {
					if tc.batch {
						require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[23:]))
					} else {
						require.NoError(t, f.s.ReceiveBlock(f.ctx, f.blks[23], f.blks[23].Root()))
					}
				}
				if !tc.tickFirst {
					driftGenesisTime(f.s, int64(24+tc.tickOffset), 0)
					require.NoError(t, f.s.NewSlot(f.ctx, 24+tc.tickOffset))
				}
				synctest.Wait()
				jc, fc := *f.s.cfg.ForkChoiceStore.JustifiedCheckpoint(), *f.s.cfg.ForkChoiceStore.FinalizedCheckpoint()
				require.Equal(t, primitives.Epoch(3), jc.Epoch)
				require.Equal(t, primitives.Epoch(2), fc.Epoch)
				savedJ, err := f.s.cfg.BeaconDB.JustifiedCheckpoint(f.ctx)
				require.NoError(t, err)
				savedF, err := f.s.cfg.BeaconDB.FinalizedCheckpoint(f.ctx)
				require.NoError(t, err)
				assert.Equal(t, jc.Epoch, savedJ.Epoch)
				assert.Equal(t, fc.Epoch, savedF.Epoch)
				assert.Equal(t, jc.Root, bytesutil.ToBytes32(savedJ.Root))
				assert.Equal(t, fc.Root, bytesutil.ToBytes32(savedF.Root))
				// Repeated ticks must not repeat finalization notifications.
				require.NoError(t, f.s.NewSlot(f.ctx, 24))
				require.NoError(t, f.s.NewSlot(f.ctx, 25))
				synctest.Wait()
				finalizedEvents := 0
				for len(events) > 0 {
					if ev := <-events; ev.Type == statefeed.FinalizedCheckpoint {
						finalizedEvents++
					}
				}
				assert.Equal(t, 1, finalizedEvents)
				err = f.s.ReceiveBlock(f.ctx, conflicting, conflicting.Root())
				require.NotNil(t, err, "must reject a block branching before finalized slot 12")
				synctest.Wait()
				restartFromPersistedFinality(t, f)
				assert.Equal(t, fc.Epoch, f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				err = f.s.ReceiveBlock(f.ctx, conflicting, conflicting.Root())
				assert.NotNil(t, err, "restart must retain the finalized ancestry restriction")
			})
		})
	}
}
