package blockchain

import (
	"bytes"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestService_DeferredIncludedHistoricalVotes(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tc := range []struct {
		name                      string
		deferred, ageWhilePending bool
		gossipImport, mixedVotes  bool
		currentSlot               int64
	}{
		{name: "known historical target control", currentSlot: 18},
		{name: "deferred current target control", deferred: true, currentSlot: 9},
		{name: "deferred historical target", deferred: true, currentSlot: 18},
		{name: "cross epoch while pending", deferred: true, ageWhilePending: true, currentSlot: 9},
		{name: "gossip import with deferred historical target", deferred: true, gossipImport: true, currentSlot: 18},
		{name: "gossip participants sharing included data", deferred: true, mixedVotes: true, currentSlot: 18},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 2)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			a6, a6State := emptyBranchBlock(t, f, f.states[2], 6, 'a')
			b6, b6State := emptyBranchBlock(t, f, f.states[2], 6, 'b')
			aRoot, bRoot := a6.Root(), b6.Root()
			if bytes.Compare(aRoot[:], bRoot[:]) < 0 {
				a6, b6, a6State, b6State = b6, a6, b6State, a6State
			}
			a7, a7State := emptyBranchBlock(t, f, a6State, 7, 'a')
			b7, _ := emptyBranchBlock(t, f, b6State, 7, 'b')
			atts, err := util.GenerateAttestations(a7State.Copy(), f.keys, 1, 8, false)
			require.NoError(t, err)
			require.Equal(t, 1, len(atts))
			att := atts[0]
			bHead, bTarget := b7.Root(), b6.Root()
			att.Data.BeaconBlockRoot, att.Data.Target.Root = bHead[:], bTarget[:]
			committee, err := helpers.BeaconCommitteeFromState(f.ctx, a7State, att.Data.Slot, att.Data.CommitteeIndex)
			require.NoError(t, err)
			domain, err := signing.Domain(a7State.Fork(), att.Data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester, a7State.GenesisValidatorsRoot())
			require.NoError(t, err)
			signingRoot, err := signing.ComputeSigningRoot(att.Data, domain)
			require.NoError(t, err)
			att.Signatures = nil
			for i, index := range committee {
				if att.AggregationBits.BitAt(uint64(i)) {
					sig, err := f.keys[index].Sign(signingRoot[:])
					require.NoError(t, err)
					att.Signatures = append(att.Signatures, sig.Marshal())
				}
			}
			gossipVote := qrysmpb.CopyAttestation(att)
			if tc.mixedVotes {
				// Only one participant is included. The remaining correctly
				// signed gossip participants must still expire at epoch 3.
				for _, bit := range att.AggregationBits.BitIndices()[1:] {
					att.AggregationBits.SetBitAt(uint64(bit), false)
				}
				att.Signatures = att.Signatures[:1]
			}
			indices, err := verifiedAttestingIndices(f.ctx, b6State, att)
			require.NoError(t, err, "vote is signed by the target-state committee")
			var wantWeight uint64
			for _, index := range indices {
				v, err := f.states[0].ValidatorAtIndexReadOnly(primitives.ValidatorIndex(index))
				require.NoError(t, err)
				wantWeight += v.EffectiveBalance()
			}
			pb, err := util.GenerateFullBlockZond(a7State.Copy(), f.keys, &util.BlockGenConfig{}, 8)
			require.NoError(t, err)
			pb.Block.Body.Attestations = atts
			sig, err := util.BlockSignature(a7State.Copy(), pb.Block, f.keys)
			require.NoError(t, err)
			pb.Signature = sig.Marshal()
			signed, err := blocks.NewSignedBeaconBlock(pb)
			require.NoError(t, err)
			a8, err := blocks.NewROBlock(signed)
			require.NoError(t, err)
			_, err = transition.ExecuteStateTransition(f.ctx, a7State.Copy(), a8)
			require.NoError(t, err, "containing block passes the full signed state transition")
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, tc.currentSlot, -30)
				fc := f.s.cfg.ForkChoiceStore
				require.NoError(t, fc.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{a6, a7}))
				synctest.Wait()
				if !tc.deferred {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{b6, b7}))
					synctest.Wait()
				}
				require.Equal(t, a7.Root(), f.s.CachedHeadRoot(), "A wins only the root tie-break")
				if tc.gossipImport {
					require.NoError(t, f.s.ReceiveBlock(f.ctx, a8, a8.Root()))
				} else {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{a8}))
				}
				synctest.Wait()
				if tc.deferred {
					pending := f.s.cfg.AttPool.BlockAttestations()
					require.Equal(t, 1, len(pending), "the included vote waits for B")
					f.s.UpdateHead(f.ctx, f.s.CurrentSlot())
					synctest.Wait()
					require.Equal(t, 1, len(f.s.cfg.AttPool.BlockAttestations()), "missing dependencies retain included votes even after the gossip window")
					if tc.ageWhilePending {
						driftGenesisTime(f.s, 18, -30)
					}
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{b6, b7}))
					synctest.Wait()
					require.NoError(t, f.s.saveInitSyncBlocks(f.ctx, true))
					require.NoError(t, f.s.VerifyLmdFfgConsistency(f.ctx, att))
					if tc.mixedVotes {
						require.NoError(t, f.s.cfg.AttPool.SaveForkchoiceAttestation(gossipVote))
					}
				}
				f.s.UpdateHead(f.ctx, f.s.CurrentSlot())
				synctest.Wait()
				fc.Lock()
				weight, err := fc.Weight(b7.Root())
				head, headErr := fc.Head(f.ctx)
				fc.Unlock()
				require.NoError(t, err)
				require.NoError(t, headErr)
				require.Equal(t, 0, len(f.s.cfg.AttPool.BlockAttestations()), "processed votes leave the included queue")
				require.Equal(t, 0, f.s.cfg.AttPool.ForkchoiceAttestationCount())
				assert.Equal(t, wantWeight, weight, "included votes remain applicable after gossip age expires")
				assert.Equal(t, b7.Root(), head, "B has the only included vote")
				assert.Equal(t, b7.Root(), f.s.CachedHeadRoot(), "publish the branch selected by the included vote")
			})
		})
	}
}
