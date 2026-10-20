package validators

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/time/slots"
)

func TestHasVoted_OK(t *testing.T) {
	// Setting bitlist to 11111111.
	pendingAttestation := &qrysmpb.Attestation{
		AggregationBits: []byte{0xFF, 0x01},
	}

	for i := uint64(0); i < pendingAttestation.AggregationBits.Len(); i++ {
		assert.Equal(t, true, pendingAttestation.AggregationBits.BitAt(i), "Validator voted but received didn't vote")
	}

	// Setting bit field to 10101010.
	pendingAttestation = &qrysmpb.Attestation{
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
	base := &qrysmpb.BeaconStateZond{Validators: []*qrysmpb.Validator{{
		ExitEpoch: exitEpoch},
	}}
	state, err := state_native.InitializeFromProtoZond(base)
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
	base := &qrysmpb.BeaconStateZond{Validators: []*qrysmpb.Validator{
		{ExitEpoch: exitedEpoch},
		{ExitEpoch: exitedEpoch + 1},
		{ExitEpoch: exitedEpoch + 2},
		{ExitEpoch: params.BeaconConfig().FarFutureEpoch},
	}}
	state, err := state_native.InitializeFromProtoZond(base)
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
	base := &qrysmpb.BeaconStateZond{Validators: []*qrysmpb.Validator{
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
	state, err := state_native.InitializeFromProtoZond(base)
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
	base := &qrysmpb.BeaconStateZond{Validators: []*qrysmpb.Validator{
		{ExitEpoch: params.BeaconConfig().FarFutureEpoch - 1},
		{EffectiveBalance: params.BeaconConfig().EjectionBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
	}}
	state, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)
	_, _, err = InitiateValidatorExit(context.Background(), state, 1, params.BeaconConfig().FarFutureEpoch-1, 1)
	require.ErrorContains(t, "addition overflows", err)
}

func TestSlashValidator_OK(t *testing.T) {
	validatorCount := 100
	registry := make([]*qrysmpb.Validator, 0, validatorCount)
	balances := make([]uint64, 0, validatorCount)
	for range validatorCount {
		registry = append(registry, &qrysmpb.Validator{
			ActivationEpoch:  0,
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		})
		balances = append(balances, params.BeaconConfig().MaxEffectiveBalance)
	}

	base := &qrysmpb.BeaconStateZond{
		Validators:  registry,
		Slashings:   make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector),
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		Balances:    balances,
	}
	state, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)

	slashedIdx := primitives.ValidatorIndex(3)

	proposer, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err, "Could not get proposer")
	proposerBal, err := state.BalanceAtIndex(proposer)
	require.NoError(t, err)
	cfg := params.BeaconConfig()
	slashedState, err := SlashValidator(context.Background(), state, slashedIdx, cfg.MinSlashingPenaltyQuotient, cfg.ProposerRewardQuotient)
	require.NoError(t, err, "Could not slash validator")
	require.Equal(t, true, slashedState.Version() == version.Zond)

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

func TestSlashValidator_SlashingWindow(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	// A nonzero epoch beyond a full window catches slot/epoch indexing mistakes.
	currentEpoch := cfg.EpochsPerSlashingsVector + 7
	for _, scenario := range []string{"normal exit", "congested exit queue", "already exiting later"} {
		t.Run(scenario, func(t *testing.T) {
			helpers.ClearCache()
			registry := make([]*qrysmpb.Validator, cfg.MinPerEpochChurnLimit+2)
			balances := make([]uint64, len(registry))
			for i := range registry {
				registry[i] = &qrysmpb.Validator{
					EffectiveBalance:  cfg.MaxEffectiveBalance,
					ExitEpoch:         cfg.FarFutureEpoch,
					WithdrawableEpoch: cfg.FarFutureEpoch,
				}
				balances[i] = cfg.MaxEffectiveBalance
			}
			st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
				Slot:        primitives.Slot(currentEpoch) * cfg.SlotsPerEpoch,
				Validators:  registry,
				Balances:    balances,
				Slashings:   make([]uint64, cfg.EpochsPerSlashingsVector),
				RandaoMixes: make([][]byte, cfg.EpochsPerHistoricalVector),
			})
			require.NoError(t, err)
			proposer, err := helpers.BeaconProposerIndex(context.Background(), st)
			require.NoError(t, err)
			slashedIdx := (proposer + 1) % primitives.ValidatorIndex(len(registry))
			wantExit := helpers.ActivationExitEpoch(currentEpoch)
			wantWithdrawable := currentEpoch + cfg.EpochsPerSlashingsVector
			switch scenario {
			case "congested exit queue":
				queueEpoch := currentEpoch + cfg.EpochsPerSlashingsVector
				queued := uint64(0)
				for i := range registry {
					idx := primitives.ValidatorIndex(i)
					if idx == slashedIdx || queued == cfg.MinPerEpochChurnLimit {
						continue
					}
					v, err := st.ValidatorAtIndex(idx)
					require.NoError(t, err)
					v.ExitEpoch = queueEpoch
					v.WithdrawableEpoch = queueEpoch + cfg.MinValidatorWithdrawabilityDelay
					require.NoError(t, st.UpdateValidatorAtIndex(idx, v))
					queued++
				}
				wantExit = queueEpoch + 1
				wantWithdrawable = wantExit + cfg.MinValidatorWithdrawabilityDelay
			case "already exiting later":
				wantExit = currentEpoch + cfg.EpochsPerSlashingsVector + 20
				wantWithdrawable = wantExit + cfg.MinValidatorWithdrawabilityDelay
				v, err := st.ValidatorAtIndex(slashedIdx)
				require.NoError(t, err)
				v.ExitEpoch = wantExit
				v.WithdrawableEpoch = wantWithdrawable
				require.NoError(t, st.UpdateValidatorAtIndex(slashedIdx, v))
			}
			_, err = SlashValidator(context.Background(), st, slashedIdx, cfg.MinSlashingPenaltyQuotient, cfg.ProposerRewardQuotient)
			require.NoError(t, err)
			v, err := st.ValidatorAtIndex(slashedIdx)
			require.NoError(t, err)
			require.Equal(t, true, v.Slashed)
			require.Equal(t, wantExit, v.ExitEpoch)
			require.Equal(t, wantWithdrawable, v.WithdrawableEpoch)
			balance, err := st.BalanceAtIndex(slashedIdx)
			require.NoError(t, err)
			require.Equal(t, cfg.MaxEffectiveBalance-cfg.MaxEffectiveBalance/cfg.MinSlashingPenaltyQuotient, balance)
			wantSlashings := make([]uint64, cfg.EpochsPerSlashingsVector)
			wantSlashings[currentEpoch%cfg.EpochsPerSlashingsVector] = cfg.MaxEffectiveBalance
			require.DeepEqual(t, wantSlashings, st.Slashings())
		})
	}
}

func TestActivatedValidatorIndices(t *testing.T) {
	far := params.BeaconConfig().FarFutureEpoch
	registry := []*qrysmpb.Validator{
		{ActivationEpoch: 0, ExitEpoch: 1},
		{ActivationEpoch: 0, ExitEpoch: far},
		{ActivationEpoch: 5, ExitEpoch: far},
		{ActivationEpoch: 5, ExitEpoch: 8},
		{ActivationEpoch: helpers.ActivationExitEpoch(10), ExitEpoch: far},
		{ActivationEpoch: far, ExitEpoch: far},
	}
	tests := []struct {
		epoch  primitives.Epoch
		wanted []primitives.ValidatorIndex
	}{
		{epoch: 0, wanted: []primitives.ValidatorIndex{0, 1}},
		// Validators that are merely active in the epoch are not activations.
		{epoch: 3, wanted: []primitives.ValidatorIndex{}},
		{epoch: 5, wanted: []primitives.ValidatorIndex{2, 3}},
		{epoch: 6, wanted: []primitives.ValidatorIndex{}},
		{epoch: helpers.ActivationExitEpoch(10), wanted: []primitives.ValidatorIndex{4}},
	}
	for _, tt := range tests {
		slot, err := slots.EpochStart(tt.epoch)
		require.NoError(t, err)
		s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Slot: slot, Validators: registry})
		require.NoError(t, err)
		activatedIndices := ActivatedValidatorIndices(time.CurrentEpoch(s), s.Validators())
		assert.DeepEqual(t, tt.wanted, activatedIndices, "epoch %d", tt.epoch)
	}
}

func TestNewlySlashedValidatorIndices(t *testing.T) {
	cfg := params.BeaconConfig()
	far := cfg.FarFutureEpoch
	vector := cfg.EpochsPerSlashingsVector
	before := []*qrysmpb.Validator{
		{Slashed: true, WithdrawableEpoch: vector},
		{Slashed: false, WithdrawableEpoch: far},
		{Slashed: false, WithdrawableEpoch: far},
		// An exit scheduled far ahead keeps its later withdrawable epoch when slashed.
		{Slashed: false, ExitEpoch: 2 * vector, WithdrawableEpoch: 2*vector + cfg.MinValidatorWithdrawabilityDelay},
		{Slashed: false, WithdrawableEpoch: far},
	}
	after := []*qrysmpb.Validator{
		{Slashed: true, WithdrawableEpoch: vector},
		{Slashed: true, WithdrawableEpoch: vector + 3},
		{Slashed: false, WithdrawableEpoch: far},
		{Slashed: true, ExitEpoch: 2 * vector, WithdrawableEpoch: 2*vector + cfg.MinValidatorWithdrawabilityDelay},
		{Slashed: false, WithdrawableEpoch: far},
		// Deposited and slashed within the same epoch.
		{Slashed: true, WithdrawableEpoch: vector + 3},
		{Slashed: false, WithdrawableEpoch: far},
	}
	assert.DeepEqual(t, []primitives.ValidatorIndex{1, 3, 5}, NewlySlashedValidatorIndices(before, after))
	assert.DeepEqual(t, []primitives.ValidatorIndex{}, NewlySlashedValidatorIndices(after, after))
	assert.DeepEqual(t, []primitives.ValidatorIndex{0, 1, 3, 5}, NewlySlashedValidatorIndices(nil, after))
}

func TestExitedValidatorIndices(t *testing.T) {
	tests := []struct {
		state  *qrysmpb.BeaconStateZond
		wanted []primitives.ValidatorIndex
	}{
		{
			state: &qrysmpb.BeaconStateZond{
				Validators: []*qrysmpb.Validator{
					{
						EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:        0,
					},
					{
						EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:        10,
					},
					{
						EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
						ExitEpoch:        0,
					},
				},
			},
			wanted: []primitives.ValidatorIndex{0, 2},
		},
		{
			state: &qrysmpb.BeaconStateZond{
				Validators: []*qrysmpb.Validator{
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
			state: &qrysmpb.BeaconStateZond{
				Validators: []*qrysmpb.Validator{
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
		exitedIndices, err := ExitedValidatorIndices(0, tt.state.Validators)
		require.NoError(t, err)
		assert.DeepEqual(t, tt.wanted, exitedIndices)
	}
}

func TestValidatorMaxExitEpochAndChurn(t *testing.T) {
	tests := []struct {
		state       *qrysmpb.BeaconStateZond
		wantedEpoch primitives.Epoch
		wantedChurn uint64
	}{
		{
			state: &qrysmpb.BeaconStateZond{
				Validators: []*qrysmpb.Validator{
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
			state: &qrysmpb.BeaconStateZond{
				Validators: []*qrysmpb.Validator{
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
			state: &qrysmpb.BeaconStateZond{
				Validators: []*qrysmpb.Validator{
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
		s, err := state_native.InitializeFromProtoZond(tt.state)
		require.NoError(t, err)
		epoch, churn := ValidatorsMaxExitEpochAndChurn(s)
		require.Equal(t, tt.wantedEpoch, epoch)
		require.Equal(t, tt.wantedChurn, churn)
	}
}
