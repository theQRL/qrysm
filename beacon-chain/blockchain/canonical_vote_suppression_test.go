package blockchain

import (
	"slices"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

// TestService_CanonicalVoteSuppression covers the two pool paths that used to
// treat inclusion in a block as proof of valid target-state signatures. Branch
// A includes slot 2, branches B and C skip it, so A's slot-19 committee differs
// from B's. A20 is consensus-valid and carries a vote for B19 signed by A's
// committee, which fails in B's target state. When A20 becomes the head, pool
// pruning must not mark those participants seen; when A20 is orphaned, reorg
// recovery must not return the vote to the proposal pool. Either would make
// the genuine B-committee signatures for the same data and bits look seen,
// leaving B without its weight and publishing the wrong head.
func TestService_CanonicalVoteSuppression(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tc := range []struct {
		name                                         string
		deferred, batch, noncanonical, replay, reorg bool
	}{
		{name: "known target gossip import"},
		{name: "deferred target gossip import", deferred: true},
		{name: "known target batch import", batch: true},
		{name: "deferred target batch import", deferred: true, batch: true},
		{name: "noncanonical containing block control", deferred: true, noncanonical: true},
		{name: "direct verified vote replay control", deferred: true, replay: true},
		{name: "reorg recovers invalid target signatures after batch import", deferred: true, batch: true, reorg: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 2)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			aState, bState, cState := f.states[2].Copy(), f.states[1].Copy(), f.states[1].Copy()
			var aBranch, bBranch, cBranch []blocks.ROBlock
			var aTarget, bTarget, cTarget state.BeaconState
			for slot := primitives.Slot(3); slot <= 19; slot++ {
				a, nextA := emptyBranchBlock(t, f, aState, slot, 'a')
				b, nextB := emptyBranchBlock(t, f, bState, slot, 'b')
				c, nextC := emptyBranchBlock(t, f, cState, slot, 'c')
				aBranch, bBranch, cBranch = append(aBranch, a), append(bBranch, b), append(cBranch, c)
				aState, bState, cState = nextA, nextB, nextC
				if slot == 18 {
					aTarget, bTarget, cTarget = aState.Copy(), bState.Copy(), cState.Copy()
				}
			}
			aCommittee, err := helpers.BeaconCommitteeFromState(f.ctx, aTarget, 19, 0)
			require.NoError(t, err)
			bCommittee, err := helpers.BeaconCommitteeFromState(f.ctx, bTarget, 19, 0)
			require.NoError(t, err)
			require.Equal(t, false, slices.Equal(aCommittee, bCommittee))
			atts, err := util.GenerateAttestations(aState.Copy(), f.keys, 1, 20, false)
			require.NoError(t, err)
			require.Equal(t, 1, len(atts))
			bHead, bTargetRoot := bBranch[len(bBranch)-1].Root(), bBranch[len(bBranch)-2].Root()
			atts[0].Data.BeaconBlockRoot, atts[0].Data.Target.Root = bHead[:], bTargetRoot[:]
			sign := func(st state.BeaconState, committee []primitives.ValidatorIndex) *qrysmpb.Attestation {
				att := qrysmpb.CopyAttestation(atts[0])
				domain, err := signing.Domain(st.Fork(), att.Data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
				require.NoError(t, err)
				root, err := signing.ComputeSigningRoot(att.Data, domain)
				require.NoError(t, err)
				att.Signatures = nil
				for i, index := range committee {
					if att.AggregationBits.BitAt(uint64(i)) {
						sig, err := f.keys[index].Sign(root[:])
						require.NoError(t, err)
						att.Signatures = append(att.Signatures, sig.Marshal())
					}
				}
				return att
			}
			bad, good := sign(aTarget, aCommittee), sign(bTarget, bCommittee)
			_, err = verifiedAttestingIndices(f.ctx, bTarget, bad)
			require.NotNil(t, err)
			indices, err := verifiedAttestingIndices(f.ctx, bTarget, good)
			require.NoError(t, err)
			var wantWeight uint64
			for _, index := range indices {
				v, err := f.states[0].ValidatorAtIndexReadOnly(primitives.ValidatorIndex(index))
				require.NoError(t, err)
				wantWeight += v.EffectiveBalance()
			}
			pb, err := util.GenerateFullBlockZond(aState.Copy(), f.keys, &util.BlockGenConfig{}, 20)
			require.NoError(t, err)
			pb.Block.Body.Attestations = []*qrysmpb.Attestation{bad}
			sig, err := util.BlockSignature(aState.Copy(), pb.Block, f.keys)
			require.NoError(t, err)
			pb.Signature = sig.Marshal()
			signed, err := blocks.NewSignedBeaconBlock(pb)
			require.NoError(t, err)
			a20, err := blocks.NewROBlock(signed)
			require.NoError(t, err)
			_, err = transition.ExecuteStateTransition(f.ctx, aState.Copy(), a20)
			require.NoError(t, err, "the containing block passes its full signed state transition")

			// A genuine independent vote keeps the intended competing branch
			// ahead when B's signatures are suppressed, without root tie-breaking.
			seedVote := func(st, target state.BeaconState, count int, exclude []primitives.ValidatorIndex) (*qrysmpb.Attestation, []primitives.ValidatorIndex) {
				votes, err := util.GenerateAttestations(st.Copy(), f.keys, 1, 21, false)
				require.NoError(t, err)
				require.Equal(t, 1, len(votes))
				att := votes[0]
				committee, err := helpers.BeaconCommitteeFromState(f.ctx, target, 20, 0)
				require.NoError(t, err)
				var chosen []primitives.ValidatorIndex
				signatures := att.Signatures
				att.Signatures = nil
				for _, bit := range att.AggregationBits.BitIndices() {
					index := committee[bit]
					keep := len(chosen) < count && !slices.Contains(bCommittee, index) && !slices.Contains(exclude, index)
					att.AggregationBits.SetBitAt(uint64(bit), keep)
					if keep {
						chosen = append(chosen, index)
						att.Signatures = append(att.Signatures, signatures[bit])
					}
				}
				require.Equal(t, count, len(chosen))
				_, err = verifiedAttestingIndices(f.ctx, target, att)
				require.NoError(t, err)
				return att, chosen
			}
			seedState, seedTarget, seedRoot := aState, aTarget, aBranch[len(aBranch)-1].Root()
			if tc.noncanonical {
				seedState, seedTarget, seedRoot = cState, cTarget, cBranch[len(cBranch)-1].Root()
			}
			seed, seedIndices := seedVote(seedState, seedTarget, 1, nil)

			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 21, -30)
				fc := f.s.cfg.ForkChoiceStore
				require.NoError(t, fc.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, aBranch))
				if tc.noncanonical || tc.reorg {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, cBranch))
				}
				synctest.Wait()
				require.NoError(t, f.s.cfg.AttPool.SaveForkchoiceAttestation(seed))
				f.s.UpdateHead(f.ctx, 21)
				synctest.Wait()
				require.Equal(t, seedRoot, f.s.CachedHeadRoot())
				if !tc.deferred {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, bBranch))
					synctest.Wait()
				}
				if tc.batch {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{a20}))
				} else {
					require.NoError(t, f.s.ReceiveBlock(f.ctx, a20, a20.Root()))
				}
				synctest.Wait()
				require.Equal(t, !tc.noncanonical, f.s.CachedHeadRoot() == a20.Root())
				if tc.deferred {
					require.Equal(t, 1, len(f.s.cfg.AttPool.BlockAttestations()))
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, bBranch))
					synctest.Wait()
				}
				f.s.UpdateHead(f.ctx, 21)
				synctest.Wait()
				require.Equal(t, 0, len(f.s.cfg.AttPool.BlockAttestations()), "the invalid included variant is no longer pending")
				require.NoError(t, f.s.VerifyLmdFfgConsistency(f.ctx, good))
				if tc.reorg {
					seen, err := f.s.cfg.AttPool.HasAggregatedAttestation(good)
					require.NoError(t, err)
					require.Equal(t, false, seen, "the batch import leaves genuine gossip unseen before the reorg")
					require.Equal(t, 0, f.s.cfg.AttPool.AggregatedAttestationCount())
					reorgSeed, _ := seedVote(cState, cTarget, 2, seedIndices)
					require.NoError(t, f.s.cfg.AttPool.SaveForkchoiceAttestation(reorgSeed))
					f.s.UpdateHead(f.ctx, 21)
					synctest.Wait()
					require.Equal(t, cBranch[len(cBranch)-1].Root(), f.s.CachedHeadRoot())
					require.Equal(t, 0, f.s.cfg.AttPool.AggregatedAttestationCount(), "an orphaned vote invalid in its target state is not a proposal candidate")
				}
				seen, err := f.s.cfg.AttPool.HasAggregatedAttestation(good)
				require.NoError(t, err)
				require.NoError(t, f.s.cfg.AttPool.SaveAggregatedAttestation(good))
				gossip := f.s.cfg.AttPool.AggregatedAttestations()
				validCount := 0
				for _, att := range gossip {
					if _, err := verifiedAttestingIndices(f.ctx, bTarget, att); err == nil {
						validCount++
					}
				}
				if !tc.replay {
					assert.Equal(t, false, seen, "a head block or orphaned proposal candidate is not proof of valid target signatures")
					assert.Equal(t, 1, validCount, "the genuine target signatures must survive")
				}
				require.NoError(t, f.s.cfg.AttPool.SaveForkchoiceAttestations(gossip))
				if tc.replay {
					require.NoError(t, f.s.cfg.AttPool.SaveForkchoiceAttestation(good))
				}
				f.s.UpdateHead(f.ctx, 21)
				synctest.Wait()
				fc.Lock()
				weight, err := fc.Weight(bHead)
				head, headErr := fc.Head(f.ctx)
				fc.Unlock()
				require.NoError(t, err)
				require.NoError(t, headErr)
				assert.Equal(t, wantWeight, weight)
				assert.Equal(t, bHead, head)
				assert.Equal(t, bHead, f.s.CachedHeadRoot())
			})
		})
	}
}
