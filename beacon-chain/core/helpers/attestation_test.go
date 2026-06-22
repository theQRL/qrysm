package helpers_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	qrysmTime "github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/time/slots"
)

func TestAttestation_IsAggregator(t *testing.T) {
	t.Run("aggregator", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		params.OverrideBeaconConfig(params.MinimalSpecConfig())
		beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
		committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, 0, 0)
		require.NoError(t, err)
		sig := privKeys[0].Sign([]byte{'A'})
		agg, err := helpers.IsAggregator(uint64(len(committee)), sig.Marshal())
		require.NoError(t, err)
		assert.Equal(t, true, agg, "Wanted aggregator true")
	})

	t.Run("not aggregator", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		cfg := params.MinimalSpecConfig().Copy()
		cfg.TargetAggregatorsPerCommittee = 1
		params.OverrideBeaconConfig(cfg)
		beaconState, privKeys := util.DeterministicGenesisStateZond(t, 256)

		committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, 0, 0)
		require.NoError(t, err)
		var sig []byte
		for i := 0; i < 256; i++ {
			candidate := privKeys[0].Sign([]byte{byte(i)}).Marshal()
			agg, err := helpers.IsAggregator(uint64(len(committee)), candidate)
			require.NoError(t, err)
			if !agg {
				sig = candidate
				break
			}
		}
		require.NotEqual(t, 0, len(sig), "could not find a non-aggregator signature")
		agg, err := helpers.IsAggregator(uint64(len(committee)), sig)
		require.NoError(t, err)
		assert.Equal(t, false, agg, "Wanted aggregator false")
	})
}

func TestAttestation_ComputeSubnetForAttestation(t *testing.T) {
	// Create 10 committees
	committeeCount := uint64(10)
	validatorCount := committeeCount * params.BeaconConfig().TargetCommitteeSize
	validators := make([]*qrysmpb.Validator, validatorCount)

	for i := range validators {
		k := make([]byte, field_params.MLDSA87PubkeyLength)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey:             k,
			WithdrawalCredentials: make([]byte, 64),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators:  validators,
		Slot:        200,
		BlockRoots:  make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot),
		StateRoots:  make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot),
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)
	att := &qrysmpb.Attestation{
		AggregationBits: []byte{'A'},
		Data: &qrysmpb.AttestationData{
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
	assert.Equal(t, true, helpers.VerifyCheckpointEpoch(&qrysmpb.Checkpoint{Epoch: 6}, genesis))
	assert.Equal(t, true, helpers.VerifyCheckpointEpoch(&qrysmpb.Checkpoint{Epoch: 5}, genesis))
	assert.Equal(t, false, helpers.VerifyCheckpointEpoch(&qrysmpb.Checkpoint{Epoch: 4}, genesis))
	assert.Equal(t, false, helpers.VerifyCheckpointEpoch(&qrysmpb.Checkpoint{Epoch: 2}, genesis))
}

func TestValidateNilAttestation(t *testing.T) {
	tests := []struct {
		name        string
		attestation *qrysmpb.Attestation
		errString   string
	}{
		{
			name:        "nil attestation",
			attestation: nil,
			errString:   "attestation can't be nil",
		},
		{
			name:        "nil attestation data",
			attestation: &qrysmpb.Attestation{},
			errString:   "attestation's data can't be nil",
		},
		{
			name: "nil attestation source",
			attestation: &qrysmpb.Attestation{
				Data: &qrysmpb.AttestationData{
					Source: nil,
					Target: &qrysmpb.Checkpoint{},
				},
			},
			errString: "attestation's source can't be nil",
		},
		{
			name: "nil attestation target",
			attestation: &qrysmpb.Attestation{
				Data: &qrysmpb.AttestationData{
					Target: nil,
					Source: &qrysmpb.Checkpoint{},
				},
			},
			errString: "attestation's target can't be nil",
		},
		{
			name: "nil attestation bitfield",
			attestation: &qrysmpb.Attestation{
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{},
					Source: &qrysmpb.Checkpoint{},
				},
			},
			errString: "attestation's bitfield can't be nil",
		},
		{
			name: "good attestation",
			attestation: &qrysmpb.Attestation{
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{},
					Source: &qrysmpb.Checkpoint{},
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
		attestation *qrysmpb.Attestation
		errString   string
	}{
		{
			name: "incorrect slot",
			attestation: &qrysmpb.Attestation{
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{Epoch: 1},
					Source: &qrysmpb.Checkpoint{},
				},
				AggregationBits: []byte{},
			},
			errString: "slot 0 does not match target epoch 1",
		},
		{
			name: "good attestation",
			attestation: &qrysmpb.Attestation{
				Data: &qrysmpb.AttestationData{
					Slot:   2 * params.BeaconConfig().SlotsPerEpoch,
					Target: &qrysmpb.Checkpoint{Epoch: 2},
					Source: &qrysmpb.Checkpoint{},
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
