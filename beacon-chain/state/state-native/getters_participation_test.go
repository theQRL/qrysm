package state_native

import (
	"testing"

	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestState_UnrealizedCheckpointBalances(t *testing.T) {
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	balances := make([]uint64, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		}
		balances[i] = params.BeaconConfig().MaxEffectiveBalance
	}
	base := &qrysmpb.BeaconStateZond{
		Slot:        130,
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),

		Validators:                 validators,
		CurrentEpochParticipation:  make([]byte, params.BeaconConfig().MinGenesisActiveValidatorCount),
		PreviousEpochParticipation: make([]byte, params.BeaconConfig().MinGenesisActiveValidatorCount),
		Balances:                   balances,
	}
	state, err := InitializeFromProtoZond(base)
	require.NoError(t, err)

	// No one voted in the last two epochs
	allActive := params.BeaconConfig().MinGenesisActiveValidatorCount * params.BeaconConfig().MaxEffectiveBalance
	active, previous, current, err := state.UnrealizedCheckpointBalances()
	require.NoError(t, err)
	require.Equal(t, allActive, active)
	require.Equal(t, params.BeaconConfig().EffectiveBalanceIncrement, current)
	require.Equal(t, params.BeaconConfig().EffectiveBalanceIncrement, previous)

	// Add some votes in the last two epochs:
	base.CurrentEpochParticipation[0] = 0xFF
	base.PreviousEpochParticipation[0] = 0xFF
	base.PreviousEpochParticipation[1] = 0xFF

	state, err = InitializeFromProtoZond(base)
	require.NoError(t, err)
	active, previous, current, err = state.UnrealizedCheckpointBalances()
	require.NoError(t, err)
	require.Equal(t, allActive, active)
	require.Equal(t, params.BeaconConfig().MaxEffectiveBalance, current)
	require.Equal(t, 2*params.BeaconConfig().MaxEffectiveBalance, previous)

	// Slash some validators
	validators[0].Slashed = true
	state, err = InitializeFromProtoZond(base)
	require.NoError(t, err)
	active, previous, current, err = state.UnrealizedCheckpointBalances()
	require.NoError(t, err)
	require.Equal(t, allActive, active)
	require.Equal(t, params.BeaconConfig().EffectiveBalanceIncrement, current)
	require.Equal(t, params.BeaconConfig().MaxEffectiveBalance, previous)

}
