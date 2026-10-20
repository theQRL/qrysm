package validator

import (
	"context"
	"slices"
	"testing"

	"github.com/theQRL/go-bitfield"
	coreblocks "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProposer_PackAttestationsAfterReorg(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 6
	cfg.TargetCommitteeSize = 128 // One committee per slot with 64 validators.
	cfg.SlotsPerHistoricalRoot = fieldparams.BlockRootsLength
	cfg.EpochsPerHistoricalVector = fieldparams.RandaoMixesLength
	cfg.EpochsPerSlashingsVector = fieldparams.SlashingsLength
	cfg.SyncCommitteeSize = fieldparams.SyncCommitteeLength
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	transition.SkipSlotCache.Disable()
	t.Cleanup(transition.SkipSlotCache.Enable)
	ctx := context.Background()
	genesis, keys := util.DeterministicGenesisStateZond(t, 64)

	// Every branch block and the resulting proposal must pass full signature
	// validation, which proposal state-root calculation alone does not perform.
	buildBlock := func(t *testing.T, pre state.BeaconState, slot primitives.Slot, graffiti byte, atts []*qrysmpb.Attestation) state.BeaconState {
		t.Helper()
		pb, err := util.GenerateFullBlockZond(pre.Copy(), keys, &util.BlockGenConfig{}, slot)
		require.NoError(t, err)
		pb.Block.Body.Graffiti[0] = graffiti
		pb.Block.Body.Attestations = atts
		sig, err := util.BlockSignature(pre.Copy(), pb.Block, keys)
		require.NoError(t, err)
		pb.Signature = sig.Marshal()
		signed, err := blocks.NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		post, err := transition.ExecuteStateTransition(ctx, pre.Copy(), signed)
		require.NoError(t, err, "block must pass full consensus validation")
		return post
	}
	state1 := buildBlock(t, genesis, 1, 0, nil)
	state2 := buildBlock(t, state1, 2, 0, nil)

	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "same committee", true: "changed committee"}[changed], func(t *testing.T) {
			oldState, replacementState := state2.Copy(), state2.Copy()
			if changed {
				// Skipping slot 2 changes the RANDAO history and the committee
				// for slot 19 without changing the justified checkpoint.
				replacementState = state1.Copy()
			}
			for slot := primitives.Slot(3); slot <= 19; slot++ {
				oldState = buildBlock(t, oldState, slot, 'a', nil)
				replacementState = buildBlock(t, replacementState, slot, 'b', nil)
			}
			committeeA, err := helpers.BeaconCommitteeFromState(ctx, oldState, 19, 0)
			require.NoError(t, err)
			committeeB, err := helpers.BeaconCommitteeFromState(ctx, replacementState, 19, 0)
			require.NoError(t, err)
			require.Equal(t, !changed, slices.Equal(committeeA, committeeB))
			oldAtts, err := util.GenerateAttestations(oldState.Copy(), keys, 1, 20, false)
			require.NoError(t, err)
			require.Equal(t, 1, len(oldAtts))
			replacementState = buildBlock(t, replacementState, 20, 'b', nil)
			replacementState = buildBlock(t, replacementState, 21, 'b', nil)
			proposalState, err := transition.ProcessSlots(ctx, replacementState.Copy(), 22)
			require.NoError(t, err)
			newAtts, err := util.GenerateAttestations(replacementState.Copy(), keys, 1, 22, false)
			require.NoError(t, err)
			require.Equal(t, 1, len(newAtts))

			for _, aggregated := range []bool{false, true} {
				t.Run(map[bool]string{false: "unaggregated", true: "aggregated"}[aggregated], func(t *testing.T) {
					oldAtt := qrysmpb.CopyAttestation(oldAtts[0])
					if !aggregated {
						position := 0
						if changed {
							for committeeA[position] == committeeB[position] {
								position++
							}
						}
						bits := bitfield.NewBitlist(oldAtt.AggregationBits.Len())
						bits.SetBitAt(uint64(position), true)
						oldAtt.AggregationBits = bits
						oldAtt.Signatures = oldAtt.Signatures[position : position+1]
					}
					orphanState := buildBlock(t, oldState, 20, 'a', []*qrysmpb.Attestation{oldAtt})
					pool := attestations.NewPool()
					if aggregated {
						require.NoError(t, pool.SaveAggregatedAttestation(oldAtt))
						require.NoError(t, pool.DeleteAggregatedAttestation(oldAtt))
					} else {
						require.NoError(t, pool.SaveUnaggregatedAttestation(oldAtt))
						require.NoError(t, pool.DeleteUnaggregatedAttestation(oldAtt))
					}
					require.Equal(t, 0, pool.AggregatedAttestationCount()+pool.UnaggregatedAttestationCount())
					require.NoError(t, pool.RecoverAttestation(oldAtt))
					vs := &Server{AttPool: pool}
					// Passing validation on one proposal branch must not bypass
					// signature validation when the selected parent changes.
					packed, err := vs.packAttestations(ctx, orphanState)
					require.NoError(t, err)
					require.DeepEqual(t, []*qrysmpb.Attestation{oldAtt}, packed)
					require.NoError(t, pool.SaveAggregatedAttestation(newAtts[0]))
					require.NoError(t, coreblocks.VerifyAttestationNoVerifySignatures(ctx, proposalState, oldAtt))
					sigErr := coreblocks.VerifyAttestationSignatures(ctx, proposalState, oldAtt)
					if changed {
						require.NotNil(t, sigErr, "the old branch's signatures do not authenticate the new committee")
					} else {
						require.NoError(t, sigErr)
					}
					packed, err = vs.packAttestations(ctx, proposalState)
					require.NoError(t, err)
					want := []*qrysmpb.Attestation{newAtts[0]}
					if !changed {
						want = append(want, oldAtt)
					}
					require.Equal(t, len(want), len(packed), "include only votes valid in the selected proposal state")
					require.DeepEqual(t, want, packed, "include only votes valid in the selected proposal state")
					require.Equal(t, len(want), pool.AggregatedAttestationCount()+pool.UnaggregatedAttestationCount(), "remove invalid votes from the proposal pool")
					for _, att := range packed {
						require.NoError(t, coreblocks.VerifyAttestationSignatures(ctx, proposalState, att))
					}
					buildBlock(t, replacementState, 22, 'b', packed)
				})
			}
		})
	}
}
