package helpers_test

import (
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestValidateGenesisActiveValidatorCount(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	if fieldparams.Preset == "minimal" {
		params.OverrideBeaconConfig(params.MinimalSpecConfig().Copy())
	}
	cfg := params.BeaconConfig()
	capacity, err := cfg.MaxActiveValidators()
	require.NoError(t, err)

	for _, tc := range []struct {
		name     string
		active   uint64
		inactive uint64
		exited   uint64
		wantErr  bool
	}{
		{name: "below capacity", active: capacity - 1},
		{name: "at capacity", active: capacity},
		{name: "above capacity", active: capacity + 1, wantErr: true},
		{name: "inactive and exited records do not consume capacity", active: capacity, inactive: 2, exited: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validators := make([]*qrysmpb.Validator, tc.active+tc.inactive+tc.exited)
			for i := range validators {
				val := &qrysmpb.Validator{ActivationEpoch: cfg.GenesisEpoch, ExitEpoch: cfg.FarFutureEpoch}
				if uint64(i) >= tc.active+tc.inactive {
					val.ExitEpoch = cfg.GenesisEpoch
				} else if uint64(i) >= tc.active {
					val.ActivationEpoch = cfg.FarFutureEpoch
				}
				validators[i] = val
			}
			st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Validators: validators})
			require.NoError(t, err)
			err = helpers.ValidateGenesisActiveValidatorCount(st)
			if tc.wantErr {
				require.ErrorContains(t, fmt.Sprintf("genesis active validator count %d exceeds committee capacity %d", tc.active, capacity), err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	t.Run("nil state", func(t *testing.T) {
		require.ErrorContains(t, "nil genesis state", helpers.ValidateGenesisActiveValidatorCount(nil))
	})
}

func TestValidateActiveValidatorCount_RejectsUnsafeCommitteeConfig(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.MaxValidatorsPerCommittee = 64
	params.OverrideBeaconConfig(cfg)
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	// These entry points are also used by genesis/checkpoint imports, which
	// must reject an unsafe bound even if the caller skipped config validation.
	require.ErrorContains(t, "SSZ attestation limit (32)", helpers.ValidateGenesisActiveValidatorCount(st))
	require.ErrorContains(t, "SSZ attestation limit (32)", helpers.ValidateCheckpointActiveValidatorCount(st))
}

func TestValidateGenesisActiveValidatorCount_ScheduledActivations(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	if fieldparams.Preset == "minimal" {
		params.OverrideBeaconConfig(params.MinimalSpecConfig().Copy())
	}
	cfg := params.BeaconConfig()
	capacity, err := cfg.MaxActiveValidators()
	require.NoError(t, err)

	for _, tc := range []struct {
		name            string
		active          uint64
		scheduled       uint64
		activationEpoch primitives.Epoch
		exitEpoch       primitives.Epoch // Optional exit for the first active validator.
		slot            primitives.Slot
		wantErr         bool
	}{
		{name: "scheduled activations reach capacity", active: capacity - 1, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 1},
		{name: "scheduled activations exceed capacity", active: capacity, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 1, wantErr: true},
		{name: "all validators activate after genesis", scheduled: capacity + 1, activationEpoch: cfg.GenesisEpoch + 1, wantErr: true},
		{name: "same epoch replacement at capacity", active: capacity, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 1, exitEpoch: cfg.GenesisEpoch + 1},
		{name: "earlier exit frees capacity", active: capacity, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 2, exitEpoch: cfg.GenesisEpoch + 1},
		{name: "overflow before later exit", active: capacity, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 1, exitEpoch: cfg.GenesisEpoch + 2, wantErr: true},
		{name: "distant scheduled activation", active: capacity, scheduled: 1, activationEpoch: cfg.FarFutureEpoch - 1, wantErr: true},
		// The genesis helper must start at GenesisEpoch, not at the supplied
		// state's slot, even when that would hide an earlier capacity overflow.
		{name: "later state slot cannot hide overflow", active: capacity, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 1, exitEpoch: cfg.GenesisEpoch + 2, slot: 3 * cfg.SlotsPerEpoch, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validators := make([]*qrysmpb.Validator, tc.active+tc.scheduled)
			for i := range validators {
				validator := &qrysmpb.Validator{ActivationEpoch: cfg.GenesisEpoch, ExitEpoch: cfg.FarFutureEpoch}
				if uint64(i) >= tc.active {
					validator.ActivationEpoch = tc.activationEpoch
				}
				if i == 0 && tc.exitEpoch != 0 {
					validator.ExitEpoch = tc.exitEpoch
				}
				validators[i] = validator
			}
			st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Slot: tc.slot, Validators: validators})
			require.NoError(t, err)
			err = helpers.ValidateGenesisActiveValidatorCount(st)
			if tc.wantErr {
				require.ErrorContains(t, fmt.Sprintf("genesis active validator count %d at epoch %d exceeds committee capacity %d", capacity+1, tc.activationEpoch, capacity), err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
