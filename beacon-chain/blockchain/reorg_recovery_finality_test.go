package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/db"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

type reorgRecoveryReadDB struct {
	db.HeadAccessDatabase
	root      [32]byte
	err       error
	remaining int
	failures  int
}

func (d *reorgRecoveryReadDB) Block(ctx context.Context, root [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	if root == d.root && d.remaining > 0 {
		d.remaining--
		d.failures++
		return nil, d.err
	}
	return d.HeadAccessDatabase.Block(ctx, root)
}

func TestService_ReorgRecoveryAfterPruning(t *testing.T) {
	setupEpochTransitionTest(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ShardCommitteePeriod = 0
	params.OverrideBeaconConfig(cfg)
	for _, tc := range []struct {
		name        string
		parent      int
		readFailure error
	}{
		{name: "non-genesis control", parent: 11},
		{name: "fork at genesis", parent: 0},
		{name: "temporary read failure", parent: 11, readFailure: errors.New("temporary block read failure")},
		{name: "canceled read", parent: 11, readFailure: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 24)
			f.s.cfg.SlashingPool = slashings.NewPool()
			f.s.cfg.ExitPool = voluntaryexits.NewPool()
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			pre := f.states[tc.parent]
			ps, err := util.GenerateProposerSlashingForValidator(pre, f.keys[0], 0)
			require.NoError(t, err)
			as, err := util.GenerateAttesterSlashingForValidator(pre, f.keys[1], 1)
			require.NoError(t, err)
			exit, err := util.GenerateVoluntaryExits(pre, f.keys[2], 2)
			require.NoError(t, err)
			pb, err := util.GenerateFullBlockZond(pre.Copy(), f.keys, &util.BlockGenConfig{}, 21)
			require.NoError(t, err)
			pb.Block.Body.ProposerSlashings = []*qrysmpb.ProposerSlashing{ps}
			pb.Block.Body.AttesterSlashings = []*qrysmpb.AttesterSlashing{as}
			pb.Block.Body.VoluntaryExits = []*qrysmpb.SignedVoluntaryExit{exit}
			sig, err := util.BlockSignature(pre.Copy(), pb.Block, f.keys)
			require.NoError(t, err)
			pb.Signature = sig.Marshal()
			signed, err := blocks.NewSignedBeaconBlock(pb)
			require.NoError(t, err)
			orphan, err := blocks.NewROBlock(signed)
			require.NoError(t, err)
			fresh := slashings.NewPool()
			require.NoError(t, fresh.InsertProposerSlashing(f.ctx, f.states[23], ps))
			require.NoError(t, fresh.InsertAttesterSlashing(f.ctx, f.states[23], as))
			require.NoError(t, f.s.cfg.SlashingPool.InsertProposerSlashing(f.ctx, pre, ps))
			require.NoError(t, f.s.cfg.SlashingPool.InsertAttesterSlashing(f.ctx, pre, as))
			f.s.cfg.ExitPool.InsertVoluntaryExit(exit)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 21, 0)
				// Load genesis balances so the timely orphan receives proposer
				// boost instead of relying on the ordering of zero-weight roots.
				f.s.cfg.ForkChoiceStore.Lock()
				require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
				f.s.cfg.ForkChoiceStore.Unlock()
				if tc.parent > 2 {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:tc.parent]))
					synctest.Wait()
				}
				if tc.parent == 0 {
					// The orphan is a sibling of the fixture's first block under
					// genesis. The fixture never loads justified balances, so
					// both would weigh zero and the head would follow root
					// order. Vote for the orphan so it is the head on import.
					voteForRoot(t, f, orphan.Root(), slots.ToEpoch(orphan.Block().Slot()))
				}
				require.NoError(t, f.s.ReceiveBlock(f.ctx, orphan, orphan.Root()))
				synctest.Wait()
				published, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, orphan.Root(), bytesutil.ToBytes32(published))
				require.Equal(t, 0, len(f.s.cfg.SlashingPool.PendingProposerSlashings(f.ctx, f.states[23], true)))
				require.Equal(t, 0, len(f.s.cfg.SlashingPool.PendingAttesterSlashings(f.ctx, f.states[23], true)))
				exits, err := f.s.cfg.ExitPool.PendingExits()
				require.NoError(t, err)
				require.Equal(t, 0, len(exits))
				d := &reorgRecoveryReadDB{HeadAccessDatabase: f.s.cfg.BeaconDB, root: orphan.Root()}
				if tc.readFailure != nil {
					d.err, d.remaining = tc.readFailure, 1
					f.s.cfg.BeaconDB = d
				}
				events := make(chan *feed.Event, 64)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				assertUnpublished := func() {
					t.Helper()
					published, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					require.Equal(t, orphan.Root(), bytesutil.ToBytes32(published))
					persisted, err := d.HeadBlockRoot()
					require.NoError(t, err)
					require.Equal(t, orphan.Root(), persisted)
					for len(events) > 0 {
						typ := (<-events).Type
						assert.NotEqual(t, statefeed.NewHead, typ)
						assert.NotEqual(t, statefeed.Reorg, typ)
					}
				}
				driftGenesisTime(f.s, 24, -30)
				err = f.s.ReceiveBlockBatch(f.ctx, f.blks[max(2, tc.parent):23])
				synctest.Wait()
				if tc.readFailure != nil {
					require.ErrorIs(t, err, tc.readFailure)
					require.Equal(t, 1, d.failures)
					assertUnpublished()
					// Fail just the first lookup in saveHead. A second lookup
					// would succeed, but must not publish an incorrect reorg depth.
					d.remaining = 1
					f.s.UpdateHead(f.ctx, 24)
					synctest.Wait()
					require.Equal(t, 2, d.failures)
					assertUnpublished()
					// Once storage recovers, the unchanged selected head still
					// retries recovery and publishes exactly once.
					f.s.UpdateHead(f.ctx, 24)
					synctest.Wait()
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, primitives.Epoch(2), f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(orphan.Root()))
				require.Equal(t, true, f.s.cfg.BeaconDB.HasBlock(f.ctx, orphan.Root()))
				published, err = f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, f.blks[22].Root(), bytesutil.ToBytes32(published))
				st, err := f.s.HeadState(f.ctx)
				require.NoError(t, err)
				psCount := len(f.s.cfg.SlashingPool.PendingProposerSlashings(f.ctx, st, true))
				asCount := len(f.s.cfg.SlashingPool.PendingAttesterSlashings(f.ctx, st, true))
				exits, err = f.s.cfg.ExitPool.ExitsForInclusion(st, st.Slot())
				require.NoError(t, err)
				assert.Equal(t, 1, psCount)
				assert.Equal(t, 1, asCount)
				assert.Equal(t, 1, len(exits))
				if tc.parent == 0 {
					f.s.cfg.ForkChoiceStore.Lock()
					root, slot, err := f.s.commonAncestorForReorg(f.ctx, orphan.Root(), f.blks[22].Root())
					f.s.cfg.ForkChoiceStore.Unlock()
					assert.NoError(t, err)
					assert.Equal(t, f.s.originBlockRoot, root)
					assert.Equal(t, primitives.Slot(0), slot)
				}
				if tc.readFailure != nil {
					heads, reorgs := 0, 0
					for len(events) > 0 {
						ev := <-events
						switch ev.Type {
						case statefeed.NewHead:
							heads++
						case statefeed.Reorg:
							reorgs++
							assert.Equal(t, uint64(12), ev.Data.(*qrlpb.EventChainReorg).Depth)
						}
					}
					require.Equal(t, 1, heads)
					require.Equal(t, 1, reorgs)
				}
			})
		})
	}
}

// voteForRoot loads the justified balances, which the batch fixture never
// does, and casts every validator's vote for root at the given target epoch.
// The votes weigh in once the block is inserted, so it outweighs any sibling
// regardless of proposer boost or root order.
func voteForRoot(t *testing.T, f *batchExecutionFixture, root [32]byte, epoch primitives.Epoch) {
	t.Helper()
	fc := f.s.cfg.ForkChoiceStore
	indices := make([]uint64, len(f.keys))
	for i := range indices {
		indices[i] = uint64(i)
	}
	fc.Lock()
	err := fc.UpdateJustifiedCheckpoint(f.ctx, fc.JustifiedCheckpoint())
	if err == nil {
		fc.ProcessAttestation(f.ctx, indices, root, epoch)
	}
	fc.Unlock()
	require.NoError(t, err)
}
