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

func TestValidateCheckpointActiveValidatorCount(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	if fieldparams.Preset == "minimal" {
		params.OverrideBeaconConfig(params.MinimalSpecConfig().Copy())
	}
	cfg := params.BeaconConfig()
	capacity, err := cfg.MaxActiveValidators()
	require.NoError(t, err)
	const currentEpoch = primitives.Epoch(10)
	farFuture := cfg.FarFutureEpoch
	type validatorGroup struct {
		count      uint64
		activation primitives.Epoch
		exit       primitives.Epoch
		slashed    bool
	}
	for _, tc := range []struct {
		name      string
		groups    []validatorGroup
		wantEpoch primitives.Epoch // Zero means no capacity error.
	}{
		{name: "empty registry"},
		{
			name:   "below capacity",
			groups: []validatorGroup{{count: capacity - 1, activation: 5, exit: farFuture}},
		},
		{
			name:   "at capacity activated after genesis",
			groups: []validatorGroup{{count: capacity, activation: currentEpoch, exit: farFuture}},
		},
		{
			name:      "above capacity activated after genesis",
			groups:    []validatorGroup{{count: capacity + 1, activation: currentEpoch, exit: farFuture}},
			wantEpoch: currentEpoch,
		},
		{
			name: "slashed validators still consume capacity until exit",
			groups: []validatorGroup{
				{count: capacity, activation: 5, exit: farFuture},
				{count: 1, activation: 5, exit: currentEpoch + 1, slashed: true},
			},
			wantEpoch: currentEpoch,
		},
		{
			name: "inactive and exited records do not consume capacity",
			groups: []validatorGroup{
				{count: capacity, activation: 5, exit: farFuture},
				{count: 2, activation: farFuture, exit: farFuture},
				{count: 2, activation: 0, exit: currentEpoch},
				{count: 2, activation: currentEpoch + 1, exit: currentEpoch + 1},
			},
		},
		{
			name: "scheduled activations reach capacity",
			groups: []validatorGroup{
				{count: capacity - 2, activation: 5, exit: farFuture},
				{count: 2, activation: currentEpoch + 1, exit: farFuture},
			},
		},
		{
			name: "scheduled activations exceed capacity",
			groups: []validatorGroup{
				{count: capacity - 1, activation: 5, exit: farFuture},
				{count: 2, activation: currentEpoch + 1, exit: farFuture},
			},
			wantEpoch: currentEpoch + 1,
		},
		{
			name: "same epoch exits free capacity regardless of registry order",
			groups: []validatorGroup{
				{count: 2, activation: currentEpoch + 1, exit: farFuture},
				{count: capacity - 2, activation: 5, exit: farFuture},
				{count: 2, activation: 5, exit: currentEpoch + 1},
			},
		},
		{
			name: "earlier exits free capacity for later activations",
			groups: []validatorGroup{
				{count: 2, activation: currentEpoch + 2, exit: farFuture},
				{count: capacity - 2, activation: 5, exit: farFuture},
				{count: 2, activation: 5, exit: currentEpoch + 1},
			},
		},
		{
			name: "transient overflow before a later exit is rejected",
			groups: []validatorGroup{
				{count: capacity - 1, activation: 5, exit: farFuture},
				{count: 1, activation: 5, exit: currentEpoch + 2},
				{count: 1, activation: currentEpoch + 1, exit: farFuture},
			},
			wantEpoch: currentEpoch + 1,
		},
		{
			name: "all scheduled epochs are checked in order",
			groups: []validatorGroup{
				{count: 1, activation: currentEpoch + 3, exit: currentEpoch + 4},
				{count: 1, activation: currentEpoch + 2, exit: currentEpoch + 3},
				{count: capacity, activation: currentEpoch + 1, exit: currentEpoch + 3},
			},
			wantEpoch: currentEpoch + 2,
		},
		{
			name: "nonoverlapping future sets may exceed total registry capacity",
			groups: []validatorGroup{
				{count: capacity, activation: currentEpoch + 2, exit: farFuture},
				{count: capacity, activation: currentEpoch + 1, exit: currentEpoch + 2},
			},
		},
		{
			name: "distant scheduled activations cannot bypass validation",
			groups: []validatorGroup{
				{count: capacity, activation: 5, exit: farFuture},
				{count: 1, activation: farFuture - 1, exit: farFuture},
			},
			wantEpoch: farFuture - 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			validators := make([]*qrysmpb.Validator, 0)
			for _, group := range tc.groups {
				for i := uint64(0); i < group.count; i++ {
					validators = append(validators, &qrysmpb.Validator{
						ActivationEpoch: group.activation,
						ExitEpoch:       group.exit,
						Slashed:         group.slashed,
					})
				}
			}
			st, err := state_native.InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{
				Slot:       primitives.Slot(currentEpoch)*cfg.SlotsPerEpoch + cfg.SlotsPerEpoch - 1,
				Validators: validators,
			})
			require.NoError(t, err)
			err = helpers.ValidateCheckpointActiveValidatorCount(st)
			if tc.wantEpoch != 0 {
				require.ErrorContains(t, fmt.Sprintf("checkpoint active validator count %d at epoch %d exceeds committee capacity %d", capacity+1, tc.wantEpoch, capacity), err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	t.Run("nil state", func(t *testing.T) {
		require.ErrorContains(t, "nil checkpoint state", helpers.ValidateCheckpointActiveValidatorCount(nil))
	})
	t.Run("nil validator registry", func(t *testing.T) {
		st, err := state_native.InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
		require.NoError(t, err)
		require.ErrorContains(t, "could not count checkpoint active validators: state has nil validator slice", helpers.ValidateCheckpointActiveValidatorCount(st))
	})
	t.Run("invalid capacity config", func(t *testing.T) {
		params.SetupTestConfigCleanup(t)
		invalid := cfg.Copy()
		invalid.SlotsPerEpoch = 0
		params.OverrideBeaconConfig(invalid)
		st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
		require.NoError(t, err)
		require.ErrorContains(t, "SLOTS_PER_EPOCH must be non-zero", helpers.ValidateCheckpointActiveValidatorCount(st))
	})
}
