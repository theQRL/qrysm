package blockchain

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/db"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

type reorgPoolReadDB struct {
	db.HeadAccessDatabase
	root [32]byte
	hook func()
}

func (d *reorgPoolReadDB) SaveHeadBlockRoot(ctx context.Context, root [32]byte) error {
	if root == d.root && d.hook != nil {
		hook := d.hook
		d.hook = nil
		hook()
	}
	return d.HeadAccessDatabase.SaveHeadBlockRoot(ctx, root)
}

func TestService_ReorgSlashingRecoveryWindows(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ShardCommitteePeriod = 0
	params.OverrideBeaconConfig(cfg)
	for _, mode := range []string{"healthy", "batch replacement", "epoch old block", "older ancestor", "read during persistence", "old snapshot after publication", "batch with old snapshot"} {
		t.Run(mode, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 2)
			f.s.cfg.SlashingPool = slashings.NewPool()
			f.s.cfg.ExitPool = voluntaryexits.NewPool()
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			pre := f.states[2]
			ps, err := util.GenerateProposerSlashingForValidator(pre, f.keys[0], 0)
			require.NoError(t, err)
			as, err := util.GenerateAttesterSlashingForValidator(pre, f.keys[1], 1)
			require.NoError(t, err)
			exit, err := util.GenerateVoluntaryExits(pre, f.keys[2], 2)
			require.NoError(t, err)
			pb, err := util.GenerateFullBlockZond(pre.Copy(), f.keys, &util.BlockGenConfig{}, 3)
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
			orphanState, err := transition.ExecuteStateTransition(f.ctx, pre.Copy(), orphan)
			require.NoError(t, err)
			oldHead := orphan
			if mode == "older ancestor" {
				oldHead, _ = emptyBranchBlock(t, f, orphanState, 4, 'o')
			}
			replacementSlot := primitives.Slot(4)
			if mode == "epoch old block" || mode == "older ancestor" {
				replacementSlot = oldHead.Block().Slot() + params.BeaconConfig().SlotsPerEpoch
			}
			replacement, replacementState := emptyBranchBlock(t, f, pre, replacementSlot, 'r')
			// Both proofs remain eligible even for the older-branch cases.
			fresh := slashings.NewPool()
			require.NoError(t, fresh.InsertProposerSlashing(f.ctx, replacementState, ps))
			require.NoError(t, fresh.InsertAttesterSlashing(f.ctx, replacementState, as))
			require.NoError(t, f.s.cfg.SlashingPool.InsertProposerSlashing(f.ctx, pre, ps))
			require.NoError(t, f.s.cfg.SlashingPool.InsertAttesterSlashing(f.ctx, pre, as))
			f.s.cfg.ExitPool.InsertVoluntaryExit(exit)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 3, 0)
				f.s.cfg.ForkChoiceStore.Lock()
				require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
				f.s.cfg.ForkChoiceStore.Unlock()
				require.NoError(t, f.s.ReceiveBlock(f.ctx, orphan, orphan.Root()))
				if mode == "older ancestor" {
					driftGenesisTime(f.s, 4, 0)
					require.NoError(t, f.s.ReceiveBlock(f.ctx, oldHead, oldHead.Root()))
				}
				synctest.Wait()
				oldRoot, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, oldHead.Root(), bytesutil.ToBytes32(oldRoot))
				require.Equal(t, 0, len(f.s.cfg.SlashingPool.PendingProposerSlashings(f.ctx, replacementState, true)))
				require.Equal(t, 0, len(f.s.cfg.SlashingPool.PendingAttesterSlashings(f.ctx, replacementState, true)))
				exits, err := f.s.cfg.ExitPool.PendingExits()
				require.NoError(t, err)
				require.Equal(t, 0, len(exits))
				var readPools func()
				if mode == "read during persistence" || mode == "old snapshot after publication" || mode == "batch with old snapshot" {
					oldState, err := f.s.HeadStateReadOnly(f.ctx)
					require.NoError(t, err)
					read, done := make(chan struct{}), make(chan struct{})
					go func() {
						defer close(done)
						<-read
						snapshot := oldState
						if mode == "read during persistence" {
							// Pool-list RPCs fetch head state without the forkchoice lock.
							var err error
							snapshot, err = f.s.HeadStateReadOnly(f.ctx)
							require.NoError(t, err)
						}
						require.Equal(t, primitives.Slot(3), snapshot.Slot())
						require.Equal(t, 0, len(f.s.cfg.SlashingPool.PendingProposerSlashings(f.ctx, snapshot, true)))
						require.Equal(t, 0, len(f.s.cfg.SlashingPool.PendingAttesterSlashings(f.ctx, snapshot, true)))
						// A proposal can retain this state across publication too.
						exits, err := f.s.cfg.ExitPool.ExitsForInclusion(snapshot, replacementSlot)
						require.NoError(t, err)
						require.Equal(t, 0, len(exits))
					}()
					readCompleted := false
					readPools = func() {
						if !readCompleted {
							close(read)
							<-done
							readCompleted = true
						}
					}
					defer readPools()
					if mode == "read during persistence" {
						f.s.cfg.BeaconDB = &reorgPoolReadDB{HeadAccessDatabase: f.s.cfg.BeaconDB, root: replacement.Root(), hook: readPools}
					}
				}
				driftGenesisTime(f.s, int64(replacementSlot), 0)
				if mode == "batch replacement" || mode == "batch with old snapshot" {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{replacement}))
				} else {
					require.NoError(t, f.s.ReceiveBlock(f.ctx, replacement, replacement.Root()))
				}
				if readPools != nil {
					readPools()
				}
				synctest.Wait()
				f.s.UpdateHead(f.ctx, replacementSlot)
				synctest.Wait()
				root, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, replacement.Root(), bytesutil.ToBytes32(root))
				st, err := f.s.HeadStateReadOnly(f.ctx)
				require.NoError(t, err)
				require.Equal(t, 1, len(f.s.cfg.SlashingPool.PendingProposerSlashings(f.ctx, st, true)))
				require.Equal(t, 1, len(f.s.cfg.SlashingPool.PendingAttesterSlashings(f.ctx, st, true)))
				exits, err = f.s.cfg.ExitPool.ExitsForInclusion(st, replacementSlot)
				require.NoError(t, err)
				require.Equal(t, 1, len(exits))
			})
		})
	}
}
