package blockchain

import (
	"slices"
	"testing"
	"testing/synctest"

	coreblocks "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func emptyBranchBlock(t *testing.T, f *batchExecutionFixture, pre state.BeaconState, slot primitives.Slot, graffiti byte) (blocks.ROBlock, state.BeaconState) {
	t.Helper()
	pb, err := util.GenerateFullBlockZond(pre.Copy(), f.keys, &util.BlockGenConfig{}, slot)
	require.NoError(t, err)
	pb.Block.Body.Graffiti[0] = graffiti
	sig, err := util.BlockSignature(pre.Copy(), pb.Block, f.keys)
	require.NoError(t, err)
	pb.Signature = sig.Marshal()
	signed, err := blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)
	ro, err := blocks.NewROBlock(signed)
	require.NoError(t, err)
	post, err := transition.ExecuteStateTransition(f.ctx, pre.Copy(), ro)
	require.NoError(t, err)
	return ro, post
}

func TestService_IncludedAttestationTargetSignatures(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, mode := range []string{"gossip", "batch", "deferred"} {
		for _, changed := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/same shuffling control", true: "/different shuffling"}[changed], func(t *testing.T) {
				f := newBatchExecutionFixture(t, 2)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				aState, bState := f.states[2].Copy(), f.states[2].Copy()
				if changed {
					bState = f.states[1].Copy()
				}
				var aBranch, bBranch []blocks.ROBlock
				var targetState state.BeaconState
				for slot := primitives.Slot(3); slot <= 19; slot++ {
					a, nextA := emptyBranchBlock(t, f, aState, slot, 'a')
					b, nextB := emptyBranchBlock(t, f, bState, slot, 'b')
					aBranch, bBranch = append(aBranch, a), append(bBranch, b)
					aState, bState = nextA, nextB
					if slot == 18 {
						targetState = bState.Copy()
					}
				}
				committeeA, err := helpers.BeaconCommitteeFromState(f.ctx, aState, 19, 0)
				require.NoError(t, err)
				committeeB, err := helpers.BeaconCommitteeFromState(f.ctx, targetState, 19, 0)
				require.NoError(t, err)
				require.Equal(t, !changed, slices.Equal(committeeA, committeeB))
				atts, err := util.GenerateAttestations(aState.Copy(), f.keys, 1, 20, false)
				require.NoError(t, err)
				require.Equal(t, 1, len(atts))
				att := atts[0]
				require.Equal(t, primitives.Slot(19), att.Data.Slot)
				bHead, bTarget := bBranch[len(bBranch)-1].Root(), bBranch[len(bBranch)-2].Root()
				att.Data.BeaconBlockRoot, att.Data.Target.Root = bHead[:], bTarget[:]
				domain, err := signing.Domain(aState.Fork(), att.Data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester, aState.GenesisValidatorsRoot())
				require.NoError(t, err)
				signingRoot, err := signing.ComputeSigningRoot(att.Data, domain)
				require.NoError(t, err)
				att.Signatures = nil
				for i, index := range committeeA {
					if att.AggregationBits.BitAt(uint64(i)) {
						sig, err := f.keys[index].Sign(signingRoot[:])
						require.NoError(t, err)
						att.Signatures = append(att.Signatures, sig.Marshal())
					}
				}
				targetErr := coreblocks.VerifyAttestationSignatures(f.ctx, targetState, att)
				if changed {
					require.NotNil(t, targetErr)
				} else {
					require.NoError(t, targetErr)
				}
				pb, err := util.GenerateFullBlockZond(aState.Copy(), f.keys, &util.BlockGenConfig{}, 20)
				require.NoError(t, err)
				pb.Block.Body.Attestations = atts
				sig, err := util.BlockSignature(aState.Copy(), pb.Block, f.keys)
				require.NoError(t, err)
				pb.Signature = sig.Marshal()
				signed, err := blocks.NewSignedBeaconBlock(pb)
				require.NoError(t, err)
				containing, err := blocks.NewROBlock(signed)
				require.NoError(t, err)
				_, err = transition.ExecuteStateTransition(f.ctx, aState.Copy(), containing)
				require.NoError(t, err, "containing block passes full state transition and signature checks")
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 21, -30)
					require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, aBranch))
					synctest.Wait()
					if mode != "deferred" {
						require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, bBranch))
						synctest.Wait()
						require.NoError(t, f.s.VerifyLmdFfgConsistency(f.ctx, att))
					}
					ownCommittee, err := helpers.BeaconCommitteeFromState(f.ctx, aState, 20, 0)
					require.NoError(t, err)
					var voter uint64
					found := false
					for _, index := range ownCommittee {
						if !slices.Contains(committeeA, index) && !slices.Contains(committeeB, index) {
							voter, found = uint64(index), true
							break
						}
					}
					require.Equal(t, true, found)
					aHead := aBranch[len(aBranch)-1].Root()
					f.s.cfg.ForkChoiceStore.ProcessAttestation(f.ctx, []uint64{voter}, aHead, 3)
					f.s.UpdateHead(f.ctx, 21)
					synctest.Wait()
					require.Equal(t, aHead, f.s.CachedHeadRoot())
					if mode == "batch" {
						require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{containing}))
					} else {
						require.NoError(t, f.s.ReceiveBlock(f.ctx, containing, containing.Root()))
					}
					synctest.Wait()
					if mode == "deferred" {
						pending := f.s.cfg.AttPool.BlockAttestations()
						require.Equal(t, 1, len(pending))
						require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, bBranch))
						synctest.Wait()
						require.NoError(t, f.s.VerifyLmdFfgConsistency(f.ctx, att))
						f.s.UpdateHead(f.ctx, 21)
						synctest.Wait()
						require.Equal(t, 0, len(f.s.cfg.AttPool.BlockAttestations()))
					}
					weight, err := f.s.cfg.ForkChoiceStore.Weight(bHead)
					require.NoError(t, err)
					head, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					if changed {
						assert.Equal(t, uint64(0), weight, "attestation invalid in its target state must not receive weight")
						assert.Equal(t, containing.Root(), bytesutil.ToBytes32(head))
					} else {
						require.Equal(t, true, weight > 0)
						require.Equal(t, bHead, bytesutil.ToBytes32(head))
					}
				})
			})
		}
	}
}

func TestService_BatchAttestationSkippedTargetSlot(t *testing.T) {
	setupEpochTransitionTest(t)
	f := newBatchExecutionFixture(t, 2)
	f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
	st := f.states[2].Copy()
	var branch []blocks.ROBlock
	for _, slot := range []primitives.Slot{3, 4, 5, 7} {
		b, next := emptyBranchBlock(t, f, st, slot, 'a')
		branch, st = append(branch, b), next
	}
	pb, err := util.GenerateFullBlockZond(st.Copy(), f.keys, util.DefaultBlockGenConfig(), 8)
	require.NoError(t, err)
	signed, err := blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)
	containing, err := blocks.NewROBlock(signed)
	require.NoError(t, err)
	att := containing.Block().Body().Attestations()[0]
	targetRoot := branch[2].Root() // Slot 5, since the epoch starts at missing slot 6.
	require.Equal(t, targetRoot, bytesutil.ToBytes32(att.Data.Target.Root))
	require.Equal(t, primitives.Epoch(1), att.Data.Target.Epoch)
	synctest.Test(t, func(t *testing.T) {
		t.Cleanup(synctest.Wait)
		driftGenesisTime(f.s, 9, -30)
		require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
		require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, append(branch, containing)))
		synctest.Wait()
		// The target is neither an epoch-boundary block nor the batch tip,
		// so its state must be regenerated from the cached initial-sync blocks.
		require.Equal(t, true, f.s.cfg.BeaconDB.HasBlock(f.ctx, targetRoot))
		weight, err := f.s.cfg.ForkChoiceStore.Weight(branch[3].Root())
		require.NoError(t, err)
		require.Equal(t, true, weight > 0, "valid votes at skipped epoch boundaries must count")
		require.Equal(t, 0, len(f.s.cfg.AttPool.BlockAttestations()))
		require.Equal(t, containing.Root(), f.s.CachedHeadRoot())
	})
}
