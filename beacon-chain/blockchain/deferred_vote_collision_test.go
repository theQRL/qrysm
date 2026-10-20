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
	"google.golang.org/protobuf/proto"
)

// TestService_DeferredVoteSignatureCollision covers included votes that wait
// for their head block. Branch A includes slot 2, branches B and C skip it, so
// A's slot-19 committee differs from B's while C's matches. Two consensus-valid
// slot-20 blocks carry the same vote for B19 with identical data and bits: A20
// signed by A's committee, invalid in B's target state, and C20 signed by C's
// committee, genuine there. The unverified A20 variant must neither evict the
// C20 variant from the pending pool nor mark its participants seen once it is
// rejected, or B loses its weight and the wrong head is published.
func TestService_DeferredVoteSignatureCollision(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tc := range []struct {
		name                 string
		validFirst, replay   bool
		gossipAfterRejection bool
	}{
		{name: "wrong target signature first"},
		{name: "valid gossip after rejected included vote", gossipAfterRejection: true},
		{name: "valid target signature first control", validFirst: true},
		{name: "wrong target signature first replay control", replay: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 2)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			aState, bState, cState := f.states[2].Copy(), f.states[1].Copy(), f.states[1].Copy()
			var aBranch, bBranch, cBranch []blocks.ROBlock
			var targetState, cTargetState state.BeaconState
			for slot := primitives.Slot(3); slot <= 19; slot++ {
				a, nextA := emptyBranchBlock(t, f, aState, slot, 'a')
				b, nextB := emptyBranchBlock(t, f, bState, slot, 'b')
				c, nextC := emptyBranchBlock(t, f, cState, slot, 'c')
				aBranch, bBranch, cBranch = append(aBranch, a), append(bBranch, b), append(cBranch, c)
				aState, bState, cState = nextA, nextB, nextC
				if slot == 18 {
					targetState = bState.Copy()
					cTargetState = cState.Copy()
				}
			}
			committeeA, err := helpers.BeaconCommitteeFromState(f.ctx, aState, 19, 0)
			require.NoError(t, err)
			committeeB, err := helpers.BeaconCommitteeFromState(f.ctx, targetState, 19, 0)
			require.NoError(t, err)
			committeeC, err := helpers.BeaconCommitteeFromState(f.ctx, cState, 19, 0)
			require.NoError(t, err)
			require.Equal(t, false, slices.Equal(committeeA, committeeB))
			require.Equal(t, true, slices.Equal(committeeC, committeeB))
			atts, err := util.GenerateAttestations(aState.Copy(), f.keys, 1, 20, false)
			require.NoError(t, err)
			require.Equal(t, 1, len(atts))
			bHead, bTarget := bBranch[len(bBranch)-1].Root(), bBranch[len(bBranch)-2].Root()
			atts[0].Data.BeaconBlockRoot, atts[0].Data.Target.Root = bHead[:], bTarget[:]
			signVote := func(st state.BeaconState, committee []primitives.ValidatorIndex) *qrysmpb.Attestation {
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
			bad, good := signVote(aState, committeeA), signVote(cState, committeeC)
			require.DeepEqual(t, bad.AggregationBits, good.AggregationBits)
			require.Equal(t, true, proto.Equal(bad.Data, good.Data))
			_, err = verifiedAttestingIndices(f.ctx, targetState, bad)
			require.NotNil(t, err, "the first variant must be invalid in the voted target state")
			indices, err := verifiedAttestingIndices(f.ctx, targetState, good)
			require.NoError(t, err, "the second variant has genuine target-state signatures")
			var wantWeight uint64
			for _, index := range indices {
				v, err := f.states[0].ValidatorAtIndexReadOnly(primitives.ValidatorIndex(index))
				require.NoError(t, err)
				wantWeight += v.EffectiveBalance()
			}
			containing := func(st state.BeaconState, att *qrysmpb.Attestation) blocks.ROBlock {
				pb, err := util.GenerateFullBlockZond(st.Copy(), f.keys, &util.BlockGenConfig{}, 20)
				require.NoError(t, err)
				pb.Block.Body.Attestations = []*qrysmpb.Attestation{att}
				sig, err := util.BlockSignature(st.Copy(), pb.Block, f.keys)
				require.NoError(t, err)
				pb.Signature = sig.Marshal()
				signed, err := blocks.NewSignedBeaconBlock(pb)
				require.NoError(t, err)
				ro, err := blocks.NewROBlock(signed)
				require.NoError(t, err)
				_, err = transition.ExecuteStateTransition(f.ctx, st.Copy(), ro)
				require.NoError(t, err, "both containing blocks pass the full signed state transition")
				return ro
			}
			a20, c20 := containing(aState, bad), containing(cState, good)
			// Give C one genuine vote from a participant outside B's committee.
			// Missing B votes must then select C, independent of root tie-breaking.
			// A20 never becomes head, isolating deferred rejection from head pruning.
			seedVotes, err := util.GenerateAttestations(cState.Copy(), f.keys, 1, 21, false)
			require.NoError(t, err)
			require.Equal(t, 1, len(seedVotes))
			seed := seedVotes[0]
			seedCommittee, err := helpers.BeaconCommitteeFromState(f.ctx, cState, 20, 0)
			require.NoError(t, err)
			chosen := -1
			for i, index := range seedCommittee {
				if !slices.Contains(committeeB, index) {
					chosen = i
					break
				}
			}
			require.Equal(t, true, chosen >= 0)
			for _, bit := range seed.AggregationBits.BitIndices() {
				seed.AggregationBits.SetBitAt(uint64(bit), bit == chosen)
			}
			seed.Signatures = [][]byte{seed.Signatures[chosen]}
			_, err = verifiedAttestingIndices(f.ctx, cTargetState, seed)
			require.NoError(t, err)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 21, -30)
				fc := f.s.cfg.ForkChoiceStore
				require.NoError(t, fc.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, aBranch))
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, cBranch))
				synctest.Wait()
				require.NoError(t, f.s.cfg.AttPool.SaveForkchoiceAttestation(seed))
				f.s.UpdateHead(f.ctx, 21)
				synctest.Wait()
				require.Equal(t, cBranch[len(cBranch)-1].Root(), f.s.CachedHeadRoot())
				require.Equal(t, false, fc.HasNode(bHead))
				ordered := []blocks.ROBlock{a20, c20}
				if tc.gossipAfterRejection {
					ordered = []blocks.ROBlock{a20}
				}
				if tc.validFirst {
					slices.Reverse(ordered)
				}
				for _, block := range ordered {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{block}))
					synctest.Wait()
				}
				require.Equal(t, false, f.s.CachedHeadRoot() == a20.Root())
				pending := f.s.cfg.AttPool.BlockAttestations()
				validQueued := false
				for _, a := range pending {
					validQueued = validQueued || proto.Equal(a, good)
				}
				require.Equal(t, !tc.gossipAfterRejection, validQueued, "the genuine variant must stay queued beside the unverified one")
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, bBranch))
				synctest.Wait()
				require.NoError(t, f.s.VerifyLmdFfgConsistency(f.ctx, good))
				if !tc.gossipAfterRejection {
					// Duplicate block gossip cannot repair an omitted included vote.
					require.NoError(t, f.s.ReceiveBlock(f.ctx, c20, c20.Root()))
				}
				f.s.UpdateHead(f.ctx, 21)
				synctest.Wait()
				if tc.gossipAfterRejection {
					require.Equal(t, 0, len(f.s.cfg.AttPool.BlockAttestations()), "the invalid included variant has been rejected")
					seen, err := f.s.cfg.AttPool.HasAggregatedAttestation(good)
					require.NoError(t, err)
					require.NoError(t, f.s.cfg.AttPool.SaveAggregatedAttestation(good))
					assert.Equal(t, false, seen, "rejected signatures must not mark genuine target-state participants seen")
					assert.Equal(t, 1, f.s.cfg.AttPool.AggregatedAttestationCount(), "valid gossip should remain available for processing")
					require.NoError(t, f.s.cfg.AttPool.SaveForkchoiceAttestations(f.s.cfg.AttPool.AggregatedAttestations()))
					f.s.UpdateHead(f.ctx, 21)
					synctest.Wait()
				}
				if tc.replay {
					fc.Lock()
					err := f.s.handleBlockAttestations(f.ctx, c20.Block())
					fc.Unlock()
					require.NoError(t, err)
					f.s.UpdateHead(f.ctx, 21)
					synctest.Wait()
				}
				fc.Lock()
				weight, err := fc.Weight(bHead)
				head, headErr := fc.Head(f.ctx)
				fc.Unlock()
				require.NoError(t, err)
				require.NoError(t, headErr)
				assert.Equal(t, wantWeight, weight, "the genuine included vote must survive a colliding unverified variant")
				assert.Equal(t, bHead, head)
				assert.Equal(t, bHead, f.s.CachedHeadRoot())
			})
		})
	}
}
