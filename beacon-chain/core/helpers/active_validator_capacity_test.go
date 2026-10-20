package helpers_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestActiveValidatorCapacity_CommitteeScaling(t *testing.T) {
	for _, tc := range []struct {
		name           string
		target         uint64
		capacityFactor uint64
	}{
		{name: "unsafe gap", target: 32, capacityFactor: 1},
		{name: "safe scaling", target: 16, capacityFactor: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			cfg := params.MainnetConfig().Copy()
			if fieldparams.Preset == "minimal" {
				cfg = params.MinimalSpecConfig().Copy()
			}
			cfg.MaxCommitteesPerSlot = 2
			cfg.TargetCommitteeSize = tc.target
			params.OverrideBeaconConfig(cfg)
			capacity := uint64(cfg.SlotsPerEpoch) * cfg.MaxValidatorsPerCommittee * tc.capacityFactor
			got, err := cfg.MaxActiveValidators()
			require.NoError(t, err)
			require.Equal(t, capacity, got)

			counts := []uint64{capacity - 1, capacity, capacity + 1}
			if tc.capacityFactor == 1 {
				// This larger set fits individually, but exits can put it into
				// the unsafe gap, so imports must still reject it.
				counts = append(counts, 2*capacity)
			}
			for _, count := range counts {
				t.Run(fmt.Sprintf("validators_%d", count), func(t *testing.T) {
					helpers.ClearCache()
					t.Cleanup(helpers.ClearCache)
					validators := make([]*qrysmpb.Validator, count)
					indices := make([]primitives.ValidatorIndex, count)
					for i := range validators {
						validators[i] = &qrysmpb.Validator{ActivationEpoch: cfg.GenesisEpoch, ExitEpoch: cfg.FarFutureEpoch}
						indices[i] = primitives.ValidatorIndex(i)
					}
					st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Validators: validators})
					require.NoError(t, err)
					genesisErr := helpers.ValidateGenesisActiveValidatorCount(st)
					require.NoError(t, st.SetSlot(10*cfg.SlotsPerEpoch))
					checkpointErr := helpers.ValidateCheckpointActiveValidatorCount(st)
					if count > capacity {
						wantErr := fmt.Sprintf("exceeds committee capacity %d", capacity)
						require.ErrorContains(t, wantErr, genesisErr)
						require.ErrorContains(t, wantErr, checkpointErr)
					} else {
						require.NoError(t, genesisErr)
						require.NoError(t, checkpointErr)
					}

					// Exercise the real committee selection and SSZ codec. The
					// last committee is one of the largest in the epoch.
					committeeIndex := primitives.CommitteeIndex(helpers.SlotCommitteeCount(count) - 1)
					committee, err := helpers.BeaconCommittee(context.Background(), indices, [32]byte{}, cfg.SlotsPerEpoch-1, committeeIndex)
					require.NoError(t, err)
					wantMembers := cfg.MaxValidatorsPerCommittee
					if count == capacity+1 {
						wantMembers++
					}
					require.Equal(t, wantMembers, uint64(len(committee)))
					bits := bitfield.NewBitlist(uint64(len(committee)))
					bits.SetBitAt(0, true)
					att := util.HydrateAttestation(&qrysmpb.Attestation{AggregationBits: bits})
					encoded, err := att.MarshalSSZ()
					if wantMembers > fieldparams.MaxValidatorsPerCommittee {
						require.ErrorContains(t, "too many bits", err)
						return
					}
					require.NoError(t, err)

					decoded := new(qrysmpb.Attestation)
					require.NoError(t, decoded.UnmarshalSSZ(encoded))
					require.DeepEqual(t, att, decoded)
				})
			}
		})
	}
}
