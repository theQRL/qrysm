package helpers_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	qrysmTime "github.com/theQRL/qrysm/v4/time"
	"github.com/theQRL/qrysm/v4/time/slots"
)

func TestAttestation_IsAggregator(t *testing.T) {
	t.Run("aggregator", func(t *testing.T) {
		beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 100)
		committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, 0, 0)
		require.NoError(t, err)
		sig := privKeys[0].Sign([]byte{'A'})
		agg, err := helpers.IsAggregator(uint64(len(committee)), sig.Marshal())
		require.NoError(t, err)
		assert.Equal(t, true, agg, "Wanted aggregator true")
	})

	t.Run("not aggregator", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		params.OverrideBeaconConfig(params.MinimalSpecConfig())
		beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 2048)

		committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, 0, 0)
		require.NoError(t, err)
		sig := privKeys[0].Sign([]byte{'B'})
		agg, err := helpers.IsAggregator(uint64(len(committee)), sig.Marshal())
		require.NoError(t, err)
		assert.Equal(t, false, agg, "Wanted aggregator false")
	})
}

func TestAttestation_ComputeSubnetForAttestation(t *testing.T) {
	// Create 10 committees
	committeeCount := uint64(10)
	validatorCount := committeeCount * params.BeaconConfig().TargetCommitteeSize
	validators := make([]*zondpb.Validator, validatorCount)

	for i := 0; i < len(validators); i++ {
		k := make([]byte, field_params.DilithiumPubkeyLength)
		copy(k, strconv.Itoa(i))
		validators[i] = &zondpb.Validator{
			PublicKey:             k,
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
	}

	state, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Validators:  validators,
		Slot:        200,
		BlockRoots:  make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot),
		StateRoots:  make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot),
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)
	att := &zondpb.Attestation{
		AggregationBits: []byte{'A'},
		Data: &zondpb.AttestationData{
			Slot:            130,
			CommitteeIndex:  4,
			BeaconBlockRoot: []byte{'C'},
			Source:          nil,
			Target:          nil,
		},
		Signatures: [][]byte{{'B'}},
	}
	valCount, err := helpers.ActiveValidatorCount(context.Background(), state, slots.ToEpoch(att.Data.Slot))
	require.NoError(t, err)
	sub := helpers.ComputeSubnetForAttestation(valCount, att)
	assert.Equal(t, uint64(6), sub, "Did not get correct subnet for attestation")
}

func Test_ValidateAttestationTime(t *testing.T) {
	cfg := params.BeaconConfig().Copy()
	params.OverrideBeaconConfig(cfg)
	params.SetupTestConfigCleanup(t)

	if params.BeaconNetworkConfig().MaximumGossipClockDisparity < 200*time.Millisecond {
		t.Fatal("This test expects the maximum clock disparity to be at least 200ms")
	}

	type args struct {
		attSlot     primitives.Slot
		genesisTime time.Time
	}
	tests := []struct {
		name      string
		args      args
		wantedErr string
	}{
		{
			name: "attestation.slot == current_slot",
			args: args{
				attSlot:     15,
				genesisTime: qrysmTime.Now().Add(-15 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
			},
		},
		{
			name: "attestation.slot == current_slot, received in middle of slot",
			args: args{
				attSlot: 15,
				genesisTime: qrysmTime.Now().Add(
					-15 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second,
				).Add(-(time.Duration(params.BeaconConfig().SecondsPerSlot/2) * time.Second)),
			},
		},
		{
			name: "attestation.slot == current_slot, received 200ms early",
			args: args{
				attSlot: 16,
				genesisTime: qrysmTime.Now().Add(
					-16 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second,
				).Add(-200 * time.Millisecond),
			},
		},
		{
			name: "attestation.slot > current_slot",
			args: args{
				attSlot:     16,
				genesisTime: qrysmTime.Now().Add(-15 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
			},
			wantedErr: "not within attestation propagation range",
		},
		{
			name: "attestation.slot < current_slot-ATTESTATION_PROPAGATION_SLOT_RANGE",
			args: args{
				attSlot:     100 - params.BeaconNetworkConfig().AttestationPropagationSlotRange - 1,
				genesisTime: qrysmTime.Now().Add(-100 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
			},
			wantedErr: "not within attestation propagation range",
		},
		{
			name: "attestation.slot = current_slot-ATTESTATION_PROPAGATION_SLOT_RANGE",
			args: args{
				attSlot:     100 - params.BeaconNetworkConfig().AttestationPropagationSlotRange,
				genesisTime: qrysmTime.Now().Add(-100 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
			},
		},
		{
			name: "attestation.slot = current_slot-ATTESTATION_PROPAGATION_SLOT_RANGE, received 200ms late",
			args: args{
				attSlot: 100 - params.BeaconNetworkConfig().AttestationPropagationSlotRange,
				genesisTime: qrysmTime.Now().Add(
					-100 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second,
				).Add(200 * time.Millisecond),
			},
		},
		{
			name: "attestation.slot is well beyond current slot",
			args: args{
				attSlot:     1 << 32,
				genesisTime: qrysmTime.Now().Add(-15 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second),
			},
			wantedErr: "which exceeds max allowed value relative to the local clock",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := helpers.ValidateAttestationTime(tt.args.attSlot, tt.args.genesisTime,
				params.BeaconNetworkConfig().MaximumGossipClockDisparity)
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVerifyCheckpointEpoch_Ok(t *testing.T) {
	// Genesis was 6 epochs ago exactly.
	offset := params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot * 6)
	genesis := time.Now().Add(-1 * time.Second * time.Duration(offset))
	assert.Equal(t, true, helpers.VerifyCheckpointEpoch(&zondpb.Checkpoint{Epoch: 6}, genesis))
	assert.Equal(t, true, helpers.VerifyCheckpointEpoch(&zondpb.Checkpoint{Epoch: 5}, genesis))
	assert.Equal(t, false, helpers.VerifyCheckpointEpoch(&zondpb.Checkpoint{Epoch: 4}, genesis))
	assert.Equal(t, false, helpers.VerifyCheckpointEpoch(&zondpb.Checkpoint{Epoch: 2}, genesis))
}

func TestValidateNilAttestation(t *testing.T) {
	tests := []struct {
		name        string
		attestation *zondpb.Attestation
		errString   string
	}{
		{
			name:        "nil attestation",
			attestation: nil,
			errString:   "attestation can't be nil",
		},
		{
			name:        "nil attestation data",
			attestation: &zondpb.Attestation{},
			errString:   "attestation's data can't be nil",
		},
		{
			name: "nil attestation source",
			attestation: &zondpb.Attestation{
				Data: &zondpb.AttestationData{
					Source: nil,
					Target: &zondpb.Checkpoint{},
				},
			},
			errString: "attestation's source can't be nil",
		},
		{
			name: "nil attestation target",
			attestation: &zondpb.Attestation{
				Data: &zondpb.AttestationData{
					Target: nil,
					Source: &zondpb.Checkpoint{},
				},
			},
			errString: "attestation's target can't be nil",
		},
		{
			name: "nil attestation bitfield",
			attestation: &zondpb.Attestation{
				Data: &zondpb.AttestationData{
					Target: &zondpb.Checkpoint{},
					Source: &zondpb.Checkpoint{},
				},
			},
			errString: "attestation's bitfield can't be nil",
		},
		{
			name: "good attestation",
			attestation: &zondpb.Attestation{
				Data: &zondpb.AttestationData{
					Target: &zondpb.Checkpoint{},
					Source: &zondpb.Checkpoint{},
				},
				AggregationBits: []byte{},
			},
			errString: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.errString != "" {
				require.ErrorContains(t, tt.errString, helpers.ValidateNilAttestation(tt.attestation))
			} else {
				require.NoError(t, helpers.ValidateNilAttestation(tt.attestation))
			}
		})
	}
}

func TestValidateSlotTargetEpoch(t *testing.T) {
	tests := []struct {
		name        string
		attestation *zondpb.Attestation
		errString   string
	}{
		{
			name: "incorrect slot",
			attestation: &zondpb.Attestation{
				Data: &zondpb.AttestationData{
					Target: &zondpb.Checkpoint{Epoch: 1},
					Source: &zondpb.Checkpoint{},
				},
				AggregationBits: []byte{},
			},
			errString: "slot 0 does not match target epoch 1",
		},
		{
			name: "good attestation",
			attestation: &zondpb.Attestation{
				Data: &zondpb.AttestationData{
					Slot:   2 * params.BeaconConfig().SlotsPerEpoch,
					Target: &zondpb.Checkpoint{Epoch: 2},
					Source: &zondpb.Checkpoint{},
				},
				AggregationBits: []byte{},
			},
			errString: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.errString != "" {
				require.ErrorContains(t, tt.errString, helpers.ValidateSlotTargetEpoch(tt.attestation.Data))
			} else {
				require.NoError(t, helpers.ValidateSlotTargetEpoch(tt.attestation.Data))
			}
		})
	}
}
