package validators

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestHasVoted_OK(t *testing.T) {
	// Setting bitlist to 11111111.
	pendingAttestation := &zondpb.Attestation{
		AggregationBits: []byte{0xFF, 0x01},
	}

	for i := uint64(0); i < pendingAttestation.AggregationBits.Len(); i++ {
		assert.Equal(t, true, pendingAttestation.AggregationBits.BitAt(i), "Validator voted but received didn't vote")
	}

	// Setting bit field to 10101010.
	pendingAttestation = &zondpb.Attestation{
		AggregationBits: []byte{0xAA, 0x1},
	}

	for i := uint64(0); i < pendingAttestation.AggregationBits.Len(); i++ {
		voted := pendingAttestation.AggregationBits.BitAt(i)
		if i%2 == 0 && voted {
			t.Error("validator didn't vote but received voted")
		}
		if i%2 == 1 && !voted {
			t.Error("validator voted but received didn't vote")
		}
	}
}

func TestInitiateValidatorExit_AlreadyExited(t *testing.T) {
	exitEpoch := primitives.Epoch(199)
	base := &zondpb.BeaconStateCapella{Validators: []*zondpb.Validator{{
		ExitEpoch: exitEpoch},
	}}
	state, err := state_native.InitializeFromProtoCapella(base)
	require.NoError(t, err)
	newState, epoch, err := InitiateValidatorExit(context.Background(), state, 0, 199, 1)
	require.ErrorIs(t, err, ValidatorAlreadyExitedErr)
	require.Equal(t, exitEpoch, epoch)
	v, err := newState.ValidatorAtIndex(0)
	require.NoError(t, err)
	assert.Equal(t, exitEpoch, v.ExitEpoch, "Already exited")
}

func TestInitiateValidatorExit_ProperExit(t *testing.T) {
	exitedEpoch := primitives.Epoch(100)
	idx := primitives.ValidatorIndex(3)
	base := &zondpb.BeaconStateCapella{Validators: []*zondpb.Validator{
		{ExitEpoch: exitedEpoch},
		{ExitEpoch: exitedEpoch + 1},
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: params.BeaconConfig().FarFutureEpoch},
	}}
	state, err := state_native.InitializeFromProtoCapella(base)
	require.NoError(t, err)
	newState, epoch, err := InitiateValidatorExit(context.Background(), state, idx, exitedEpoch+2, 1)
	require.NoError(t, err)
	require.Equal(t, exitedEpoch+2, epoch)
	v, err := newState.ValidatorAtIndex(idx)
	require.NoError(t, err)
	assert.Equal(t, exitedEpoch+2, v.ExitEpoch, "Exit epoch was not the highest")
}

func TestInitiateValidatorExit_ChurnOverflow(t *testing.T) {
	exitedEpoch := primitives.Epoch(100)
	idx := primitives.ValidatorIndex(10)
	base := &zondpb.BeaconStateCapella{Validators: []*zondpb.Validator{
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: exitedEpoch + 2}, // overflow here
		{ExitEpoch: params.BeaconConfig().FarFutureEpoch},
	}}
	state, err := state_native.InitializeFromProtoCapella(base)
	require.NoError(t, err)
	newState, epoch, err := InitiateValidatorExit(context.Background(), state, idx, exitedEpoch+2, 10)
	require.NoError(t, err)
	require.Equal(t, exitedEpoch+3, epoch)

	// Because of exit queue overflow,
	// validator who init exited has to wait one more epoch.
	v, err := newState.ValidatorAtIndex(0)
	require.NoError(t, err)
	wantedEpoch := v.ExitEpoch + 1

	v, err = newState.ValidatorAtIndex(idx)
	require.NoError(t, err)
	assert.Equal(t, wantedEpoch, v.ExitEpoch, "Exit epoch did not cover overflow case")
}

func TestInitiateValidatorExit_WithdrawalOverflows(t *testing.T) {
	base := &zondpb.BeaconStateCapella{Validators: []*zondpb.Validator{
		{ExitEpoch: params.BeaconConfig().FarFutureEpoch - 1},
		{EffectiveBalance: params.BeaconConfig().EjectionBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
	}}
	state, err := state_native.InitializeFromProtoCapella(base)
	require.NoError(t, err)
	_, _, err = InitiateValidatorExit(context.Background(), state, 1, params.BeaconConfig().FarFutureEpoch-1, 1)
	require.ErrorContains(t, "addition overflows", err)
}

func TestSlashValidator_OK(t *testing.T) {
	validatorCount := 100
	registry := make([]*zondpb.Validator, 0, validatorCount)
	balances := make([]uint64, 0, validatorCount)
	for i := 0; i < validatorCount; i++ {
		registry = append(registry, &zondpb.Validator{
			ActivationEpoch:  0,
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		})
		balances = append(balances, params.BeaconConfig().MaxEffectiveBalance)
	}

	base := &zondpb.BeaconStateCapella{
		Validators:  registry,
		Slashings:   make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector),
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		Balances:    balances,
	}
	state, err := state_native.InitializeFromProtoCapella(base)
	require.NoError(t, err)

	slashedIdx := primitives.ValidatorIndex(3)

	proposer, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err, "Could not get proposer")
	proposerBal, err := state.BalanceAtIndex(proposer)
	require.NoError(t, err)
	cfg := params.BeaconConfig()
	slashedState, err := SlashValidator(context.Background(), state, slashedIdx, cfg.MinSlashingPenaltyQuotient, cfg.ProposerRewardQuotient)
	require.NoError(t, err, "Could not slash validator")
	require.Equal(t, true, slashedState.Version() == version.Capella)

	v, err := state.ValidatorAtIndex(slashedIdx)
	require.NoError(t, err)
	assert.Equal(t, true, v.Slashed, "Validator not slashed despite supposed to being slashed")
	assert.Equal(t, time.CurrentEpoch(state)+params.BeaconConfig().EpochsPerSlashingsVector, v.WithdrawableEpoch, "Withdrawable epoch not the expected value")

	maxBalance := params.BeaconConfig().MaxEffectiveBalance
	slashedBalance := state.Slashings()[state.Slot().Mod(uint64(params.BeaconConfig().EpochsPerSlashingsVector))]
	assert.Equal(t, maxBalance, slashedBalance, "Slashed balance isn't the expected amount")

	whistleblowerReward := slashedBalance / params.BeaconConfig().WhistleBlowerRewardQuotient
	bal, err := state.BalanceAtIndex(proposer)
	require.NoError(t, err)
	// The proposer is the whistleblower in phase 0.
	assert.Equal(t, proposerBal+whistleblowerReward, bal, "Did not get expected balance for proposer")
	bal, err = state.BalanceAtIndex(slashedIdx)
	require.NoError(t, err)
	v, err = state.ValidatorAtIndex(slashedIdx)
	require.NoError(t, err)
	assert.Equal(t, maxBalance-(v.EffectiveBalance/params.BeaconConfig().MinSlashingPenaltyQuotient), bal, "Did not get expected balance for slashed validator")
}

func TestActivatedValidatorIndices(t *testing.T) {
	tests := []struct {
		state  *zondpb.BeaconStateCapella
		wanted []primitives.ValidatorIndex
	}{
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						ActivationEpoch: 0,
						ExitEpoch:       1,
					},
					{
						ActivationEpoch: 0,
						ExitEpoch:       1,
					},
					{
						ActivationEpoch: 5,
					},
					{
						ActivationEpoch: 0,
						ExitEpoch:       1,
					},
				},
			},
			wanted: []primitives.ValidatorIndex{0, 1, 3},
		},
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						ActivationEpoch: helpers.ActivationExitEpoch(10),
					},
				},
			},
			wanted: []primitives.ValidatorIndex{},
		},
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						ActivationEpoch: 0,
						ExitEpoch:       1,
					},
				},
			},
			wanted: []primitives.ValidatorIndex{0},
		},
	}
	for _, tt := range tests {
		s, err := state_native.InitializeFromProtoCapella(tt.state)
		require.NoError(t, err)
		activatedIndices := ActivatedValidatorIndices(time.CurrentEpoch(s), tt.state.Validators)
		assert.DeepEqual(t, tt.wanted, activatedIndices)
	}
}

func TestSlashedValidatorIndices(t *testing.T) {
	tests := []struct {
		state  *zondpb.BeaconStateCapella
		wanted []primitives.ValidatorIndex
	}{
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector,
						Slashed:           true,
					},
					{
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector,
						Slashed:           false,
					},
					{
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector,
						Slashed:           true,
					},
				},
			},
			wanted: []primitives.ValidatorIndex{0, 2},
		},
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector,
					},
				},
			},
			wanted: []primitives.ValidatorIndex{},
		},
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector,
						Slashed:           true,
					},
				},
			},
			wanted: []primitives.ValidatorIndex{0},
		},
	}
	for _, tt := range tests {
		s, err := state_native.InitializeFromProtoCapella(tt.state)
		require.NoError(t, err)
		slashedIndices := SlashedValidatorIndices(time.CurrentEpoch(s), tt.state.Validators)
		assert.DeepEqual(t, tt.wanted, slashedIndices)
	}
}

func TestExitedValidatorIndices(t *testing.T) {
	tests := []struct {
		state  *zondpb.BeaconStateCapella
		wanted []primitives.ValidatorIndex
	}{
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         0,
						WithdrawableEpoch: params.BeaconConfig().MinValidatorWithdrawabilityDelay,
					},
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         0,
						WithdrawableEpoch: 10,
					},
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         0,
						WithdrawableEpoch: params.BeaconConfig().MinValidatorWithdrawabilityDelay,
					},
				},
			},
			wanted: []primitives.ValidatorIndex{0, 2},
		},
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         params.BeaconConfig().FarFutureEpoch,
						WithdrawableEpoch: params.BeaconConfig().MinValidatorWithdrawabilityDelay,
					},
				},
			},
			wanted: []primitives.ValidatorIndex{},
		},
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         0,
						WithdrawableEpoch: params.BeaconConfig().MinValidatorWithdrawabilityDelay,
					},
				},
			},
			wanted: []primitives.ValidatorIndex{0},
		},
	}
	for _, tt := range tests {
		s, err := state_native.InitializeFromProtoCapella(tt.state)
		require.NoError(t, err)
		activeCount, err := helpers.ActiveValidatorCount(context.Background(), s, time.PrevEpoch(s))
		require.NoError(t, err)
		exitedIndices, err := ExitedValidatorIndices(0, tt.state.Validators, activeCount)
		require.NoError(t, err)
		assert.DeepEqual(t, tt.wanted, exitedIndices)
	}
}

func TestValidatorMaxExitEpochAndChurn(t *testing.T) {
	tests := []struct {
		state       *zondpb.BeaconStateCapella
		wantedEpoch primitives.Epoch
		wantedChurn uint64
	}{
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         0,
						WithdrawableEpoch: params.BeaconConfig().MinValidatorWithdrawabilityDelay,
					},
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         0,
						WithdrawableEpoch: 10,
					},
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         0,
						WithdrawableEpoch: params.BeaconConfig().MinValidatorWithdrawabilityDelay,
					},
				},
			},
			wantedEpoch: 0,
			wantedChurn: 3,
		},
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         params.BeaconConfig().FarFutureEpoch,
						WithdrawableEpoch: params.BeaconConfig().MinValidatorWithdrawabilityDelay,
					},
				},
			},
			wantedEpoch: 0,
			wantedChurn: 0,
		},
		{
			state: &zondpb.BeaconStateCapella{
				Validators: []*zondpb.Validator{
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         1,
						WithdrawableEpoch: params.BeaconConfig().MinValidatorWithdrawabilityDelay,
					},
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         0,
						WithdrawableEpoch: 10,
					},
					{
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:         1,
						WithdrawableEpoch: params.BeaconConfig().MinValidatorWithdrawabilityDelay,
					},
				},
			},
			wantedEpoch: 1,
			wantedChurn: 2,
		},
	}
	for _, tt := range tests {
		s, err := state_native.InitializeFromProtoCapella(tt.state)
		require.NoError(t, err)
		epoch, churn := ValidatorsMaxExitEpochAndChurn(s)
		require.Equal(t, tt.wantedEpoch, epoch)
		require.Equal(t, tt.wantedChurn, churn)
	}
}
