package altair

import (
	"context"
	"math"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/epoch/precompute"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestInitializeEpochValidators_Ok(t *testing.T) {
	ffe := params.BeaconConfig().FarFutureEpoch
	s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: params.BeaconConfig().SlotsPerEpoch,
		// Validator 0 is slashed
		// Validator 1 is withdrawable
		// Validator 2 is active prev epoch and current epoch
		// Validator 3 is active prev epoch
		Validators: []*qrysmpb.Validator{
			{Slashed: true, WithdrawableEpoch: ffe, EffectiveBalance: 100},
			{EffectiveBalance: 100},
			{WithdrawableEpoch: ffe, ExitEpoch: ffe, EffectiveBalance: 100},
			{WithdrawableEpoch: ffe, ExitEpoch: 1, EffectiveBalance: 100},
		},
		InactivityScores: []uint64{0, 1, 2, 3},
	})
	require.NoError(t, err)
	v, b, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)
	assert.DeepEqual(t, &precompute.Validator{
		IsSlashed:                    true,
		CurrentEpochEffectiveBalance: 100,
		InactivityScore:              0,
	}, v[0], "Incorrect validator 0 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: 100,
		InactivityScore:              1,
	}, v[1], "Incorrect validator 1 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsActivePrevEpoch:            true,
		IsActiveCurrentEpoch:         true,
		CurrentEpochEffectiveBalance: 100,
		InactivityScore:              2,
	}, v[2], "Incorrect validator 2 status")
	assert.DeepEqual(t, &precompute.Validator{
		IsActivePrevEpoch:            true,
		CurrentEpochEffectiveBalance: 100,
		InactivityScore:              3,
	}, v[3], "Incorrect validator 3 status")

	wantedBalances := &precompute.Balance{
		ActiveCurrentEpoch: 100,
		ActivePrevEpoch:    200,
	}
	assert.DeepEqual(t, wantedBalances, b, "Incorrect wanted balance")
}

func TestInitializeEpochValidators_Overflow(t *testing.T) {
	ffe := params.BeaconConfig().FarFutureEpoch
	s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: params.BeaconConfig().SlotsPerEpoch,
		Validators: []*qrysmpb.Validator{
			{WithdrawableEpoch: ffe, ExitEpoch: ffe, EffectiveBalance: math.MaxUint64},
			{WithdrawableEpoch: ffe, ExitEpoch: ffe, EffectiveBalance: math.MaxUint64},
		},
		InactivityScores: []uint64{0, 1},
	})
	require.NoError(t, err)
	_, _, err = InitializePrecomputeValidators(context.Background(), s)
	require.ErrorContains(t, "could not read every validator: addition overflows", err)
}

func TestInitializeEpochValidators_BadState(t *testing.T) {
	s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators:       []*qrysmpb.Validator{{}},
		InactivityScores: []uint64{},
	})
	require.NoError(t, err)
	_, _, err = InitializePrecomputeValidators(context.Background(), s)
	require.ErrorContains(t, "num of validators is different than num of inactivity scores", err)
}

func TestProcessEpochParticipation(t *testing.T) {
	s, err := testStateZond()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(context.Background(), s, balance, validators)
	require.NoError(t, err)
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
	}, validators[0])
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		IsCurrentEpochAttester:       true,
		IsPrevEpochAttester:          true,
		IsPrevEpochSourceAttester:    true,
	}, validators[1])
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		IsCurrentEpochAttester:       true,
		IsPrevEpochAttester:          true,
		IsPrevEpochSourceAttester:    true,
		IsCurrentEpochTargetAttester: true,
		IsPrevEpochTargetAttester:    true,
	}, validators[2])
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		IsCurrentEpochAttester:       true,
		IsPrevEpochAttester:          true,
		IsPrevEpochSourceAttester:    true,
		IsCurrentEpochTargetAttester: true,
		IsPrevEpochTargetAttester:    true,
		IsPrevEpochHeadAttester:      true,
	}, validators[3])
	require.Equal(t, params.BeaconConfig().MaxEffectiveBalance*3, balance.PrevEpochAttested)
	require.Equal(t, balance.CurrentEpochTargetAttested, params.BeaconConfig().MaxEffectiveBalance*2)
	require.Equal(t, balance.PrevEpochTargetAttested, params.BeaconConfig().MaxEffectiveBalance*2)
	require.Equal(t, balance.PrevEpochHeadAttested, params.BeaconConfig().MaxEffectiveBalance*1)
}

func TestProcessEpochParticipation_InactiveValidator(t *testing.T) {
	generateParticipation := func(flags ...uint8) byte {
		b := byte(0)
		var err error
		for _, flag := range flags {
			b, err = AddValidatorFlag(b, flag)
			require.NoError(t, err)
		}
		return b
	}
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: 2 * params.BeaconConfig().SlotsPerEpoch,
		Validators: []*qrysmpb.Validator{
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},                                                  // Inactive
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: 2},                                    // Inactive current epoch, active previous epoch
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch}, // Active
		},
		CurrentEpochParticipation: []byte{
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex),
		},
		PreviousEpochParticipation: []byte{
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex),
		},
		InactivityScores: []uint64{0, 0, 0},
	})
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(context.Background(), st)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(context.Background(), st, balance, validators)
	require.NoError(t, err)
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         false,
		IsActivePrevEpoch:            false,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
	}, validators[0])
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         false,
		IsActivePrevEpoch:            true,
		IsPrevEpochAttester:          true,
		IsPrevEpochSourceAttester:    true,
		IsPrevEpochTargetAttester:    true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
	}, validators[1])
	require.DeepEqual(t, &precompute.Validator{
		IsActiveCurrentEpoch:         true,
		IsActivePrevEpoch:            true,
		IsWithdrawableCurrentEpoch:   true,
		CurrentEpochEffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		IsCurrentEpochAttester:       true,
		IsPrevEpochAttester:          true,
		IsPrevEpochSourceAttester:    true,
		IsCurrentEpochTargetAttester: true,
		IsPrevEpochTargetAttester:    true,
		IsPrevEpochHeadAttester:      true,
	}, validators[2])
	require.Equal(t, balance.PrevEpochAttested, 2*params.BeaconConfig().MaxEffectiveBalance)
	require.Equal(t, balance.CurrentEpochTargetAttested, params.BeaconConfig().MaxEffectiveBalance)
	require.Equal(t, balance.PrevEpochTargetAttested, 2*params.BeaconConfig().MaxEffectiveBalance)
	require.Equal(t, balance.PrevEpochHeadAttested, params.BeaconConfig().MaxEffectiveBalance)
}

func TestAttestationsDelta(t *testing.T) {
	s, err := testStateZond()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(context.Background(), s, balance, validators)
	require.NoError(t, err)
	deltas, err := AttestationsDelta(s, balance, validators)
	require.NoError(t, err)

	rewards := make([]uint64, len(deltas))
	penalties := make([]uint64, len(deltas))
	for i, d := range deltas {
		rewards[i] = d.HeadReward + d.SourceReward + d.TargetReward
		penalties[i] = d.SourcePenalty + d.TargetPenalty
	}

	// Reward amount should increase as validator index increases due to setup.
	for i := 1; i < len(rewards); i++ {
		require.Equal(t, true, rewards[i] > rewards[i-1])
	}

	// Penalty amount should decrease as validator index increases due to setup.
	for i := 1; i < len(penalties); i++ {
		require.Equal(t, true, penalties[i] <= penalties[i-1])
	}

	// First index should have 0 reward.
	require.Equal(t, uint64(0), rewards[0])
	// Last index should have 0 penalty.
	require.Equal(t, uint64(0), penalties[len(penalties)-1])

	want := []uint64{0, 1062521250, 2378023750, 2732197500}
	require.DeepEqual(t, want, rewards)
	want = []uint64{4047700000, 2631005000, 0, 0}
	require.DeepEqual(t, want, penalties)
}

func TestProcessRewardsAndPenaltiesPrecompute_RewardDenominator(t *testing.T) {
	const validatorCount = 128
	activeIncrements := validatorCount * (params.BeaconConfig().MaxEffectiveBalance / params.BeaconConfig().EffectiveBalanceIncrement)
	for _, tc := range []struct {
		name        string
		denominator uint64
		want        string
	}{
		{"default denominator", 64, ""},
		{"largest safe product", math.MaxUint64 / activeIncrements, ""},
		{"overflow to zero", 1 << 51, "could not compute attestation reward denominator: multiplication overflows"},
		{"overflow to nonzero", 1<<51 + 1, "could not compute attestation reward denominator: multiplication overflows"},
		{"zero denominator", 0, "attestation reward denominator must be non-zero"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			cfg := params.BeaconConfig().Copy()
			cfg.WeightDenominator = tc.denominator
			if tc.denominator > 0 {
				cfg.ProposerWeight = tc.denominator - 1
			}
			cfg.TimelySourceWeight = 1
			cfg.TimelyTargetWeight = 0
			cfg.TimelyHeadWeight = 0
			cfg.SyncRewardWeight = 0
			// Bypass config validation to exercise the arithmetic guard itself.
			params.OverrideBeaconConfig(cfg)

			validators := make([]*qrysmpb.Validator, validatorCount)
			balances := make([]uint64, validatorCount)
			participation := make([]byte, validatorCount)
			for i := range validators {
				validators[i] = &qrysmpb.Validator{EffectiveBalance: cfg.MaxEffectiveBalance, ExitEpoch: cfg.FarFutureEpoch}
				balances[i] = cfg.MaxEffectiveBalance
				participation[i] = 1<<cfg.TimelySourceFlagIndex | 1<<cfg.TimelyTargetFlagIndex | 1<<cfg.TimelyHeadFlagIndex
			}
			s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
				Slot:                       2 * cfg.SlotsPerEpoch,
				Validators:                 validators,
				Balances:                   balances,
				CurrentEpochParticipation:  participation,
				PreviousEpochParticipation: participation,
				InactivityScores:           make([]uint64, validatorCount),
			})
			require.NoError(t, err)
			vals, bal, err := InitializePrecomputeValidators(context.Background(), s)
			require.NoError(t, err)
			vals, bal, err = ProcessEpochParticipation(context.Background(), s, bal, vals)
			require.NoError(t, err)
			_, err = ProcessRewardsAndPenaltiesPrecompute(s, bal, vals)
			if tc.want != "" {
				require.ErrorContains(t, tc.want, err)
				require.DeepEqual(t, balances, s.Balances(), "failed reward processing must preserve balances")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestProcessRewardsAndPenaltiesPrecompute_Ok(t *testing.T) {
	s, err := testStateZond()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(context.Background(), s, balance, validators)
	require.NoError(t, err)
	s, err = ProcessRewardsAndPenaltiesPrecompute(s, balance, validators)
	require.NoError(t, err)

	balances := s.Balances()
	// Reward amount should increase as validator index increases due to setup.
	for i := 1; i < len(balances); i++ {
		require.Equal(t, true, balances[i] >= balances[i-1])
	}

	wanted := make([]uint64, s.NumValidators())
	deltas, err := AttestationsDelta(s, balance, validators)
	require.NoError(t, err)

	rewards := make([]uint64, len(deltas))
	penalties := make([]uint64, len(deltas))
	for i, d := range deltas {
		rewards[i] = d.HeadReward + d.SourceReward + d.TargetReward
		penalties[i] = d.SourcePenalty + d.TargetPenalty
	}
	for i := range rewards {
		wanted[i] += rewards[i]
	}
	for i := range penalties {
		if wanted[i] > penalties[i] {
			wanted[i] -= penalties[i]
		} else {
			wanted[i] = 0
		}
	}
	require.DeepEqual(t, wanted, balances)
}

func TestProcessRewardsAndPenaltiesPrecompute_RewardOrder(t *testing.T) {
	cfg := params.BeaconConfig()
	baseReward, err := BaseRewardPerIncrement(cfg.EffectiveBalanceIncrement)
	require.NoError(t, err)
	source := byte(1 << cfg.TimelySourceFlagIndex)
	target := byte(1 << cfg.TimelyTargetFlagIndex)
	head := byte(1 << cfg.TimelyHeadFlagIndex)
	sourceDelta := baseReward * cfg.TimelySourceWeight / cfg.WeightDenominator
	targetDelta := baseReward * cfg.TimelyTargetWeight / cfg.WeightDenominator
	headReward := baseReward * cfg.TimelyHeadWeight / cfg.WeightDenominator
	const inactivityScore = uint64(64)
	inactivityPenalty := cfg.EffectiveBalanceIncrement * inactivityScore / (cfg.InactivityScoreBias * cfg.InactivityPenaltyQuotient)

	for _, tt := range []struct {
		name            string
		balance         uint64
		inactivityScore uint64
		want            uint64
		flags           byte
		slashed         bool
		leak            bool
	}{
		{name: "target_only_zero_balance", flags: target, want: targetDelta},
		{name: "target_only_one_shor", flags: target, balance: 1, want: targetDelta},
		{name: "target_only_below_penalty", flags: target, balance: sourceDelta - 1, want: targetDelta},
		{name: "target_only_at_penalty", flags: target, balance: sourceDelta, want: targetDelta},
		{name: "target_only_above_penalty", flags: target, balance: sourceDelta + 1, want: targetDelta + 1},
		{name: "target_only_sufficient_balance", flags: target, balance: cfg.EffectiveBalanceIncrement, want: cfg.EffectiveBalanceIncrement - sourceDelta + targetDelta},
		{name: "no_participation", want: 0},
		{name: "source_only", flags: source, want: 0},
		{name: "source_and_target", flags: source | target, want: sourceDelta + targetDelta},
		{name: "all_flags", flags: source | target | head, want: sourceDelta + targetDelta + headReward},
		{name: "slashed_target_voter", flags: target, slashed: true, want: 0},
		{name: "target_only_in_leak", flags: target, leak: true, want: 0},
		{name: "all_flags_in_leak", flags: source | target | head, leak: true, want: 0},
		{
			name:            "source_reward_then_target_and_inactivity_penalties",
			flags:           source,
			balance:         cfg.EffectiveBalanceIncrement,
			inactivityScore: inactivityScore,
			want:            cfg.EffectiveBalanceIncrement + sourceDelta - targetDelta - inactivityPenalty,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			slot := cfg.SlotsPerEpoch
			if tt.leak {
				slot = primitives.Slot(cfg.MinEpochsToInactivityPenalty+2) * cfg.SlotsPerEpoch
			}
			st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
				Slot: slot,
				Validators: []*qrysmpb.Validator{{
					EffectiveBalance:  cfg.EffectiveBalanceIncrement,
					ExitEpoch:         cfg.FarFutureEpoch,
					WithdrawableEpoch: cfg.FarFutureEpoch,
					Slashed:           tt.slashed,
				}},
				Balances:                   []uint64{tt.balance},
				InactivityScores:           []uint64{tt.inactivityScore},
				CurrentEpochParticipation:  []byte{0},
				PreviousEpochParticipation: []byte{tt.flags},
				FinalizedCheckpoint:        &qrysmpb.Checkpoint{},
			})
			require.NoError(t, err)
			vals, totals, err := InitializePrecomputeValidators(context.Background(), st)
			require.NoError(t, err)
			vals, totals, err = ProcessEpochParticipation(context.Background(), st, totals, vals)
			require.NoError(t, err)
			post, err := ProcessRewardsAndPenaltiesPrecompute(st, totals, vals)
			require.NoError(t, err)
			require.Equal(t, tt.want, post.Balances()[0])
			require.Equal(t, tt.balance, vals[0].BeforeEpochTransitionBalance)
			require.Equal(t, tt.want, vals[0].AfterEpochTransitionBalance)
		})
	}
}

func TestProcessRewardsAndPenaltiesPrecompute_InactivityLeak(t *testing.T) {
	s, err := testStateZond()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(context.Background(), s, balance, validators)
	require.NoError(t, err)
	sCopy := s.Copy()
	s, err = ProcessRewardsAndPenaltiesPrecompute(s, balance, validators)
	require.NoError(t, err)

	// Copied state where finality happened long ago
	require.NoError(t, sCopy.SetSlot(params.BeaconConfig().SlotsPerEpoch*1000))
	sCopy, err = ProcessRewardsAndPenaltiesPrecompute(sCopy, balance, validators)
	require.NoError(t, err)

	balances := s.Balances()
	inactivityBalances := sCopy.Balances()
	// Balances decreased to 0 due to inactivity
	require.Equal(t, uint64(2378023750), balances[2])
	require.Equal(t, uint64(2732197500), balances[3])
	require.Equal(t, uint64(0), inactivityBalances[2])
	require.Equal(t, uint64(0), inactivityBalances[3])
}

func TestProcessInactivityScores_CanProcessInactivityLeak(t *testing.T) {
	s, err := testStateZond()
	require.NoError(t, err)
	defaultScore := uint64(5)
	require.NoError(t, s.SetInactivityScores([]uint64{defaultScore, defaultScore, defaultScore, defaultScore}))
	require.NoError(t, s.SetSlot(params.BeaconConfig().SlotsPerEpoch*primitives.Slot(params.BeaconConfig().MinEpochsToInactivityPenalty+2)))
	validators, balance, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)
	validators, _, err = ProcessEpochParticipation(context.Background(), s, balance, validators)
	require.NoError(t, err)
	s, _, err = ProcessInactivityScores(context.Background(), s, validators)
	require.NoError(t, err)
	inactivityScores, err := s.InactivityScores()
	require.NoError(t, err)
	// V0 and V1 didn't vote head. V2 and V3 did.
	require.Equal(t, defaultScore+params.BeaconConfig().InactivityScoreBias, inactivityScores[0])
	require.Equal(t, defaultScore+params.BeaconConfig().InactivityScoreBias, inactivityScores[1])
	require.Equal(t, defaultScore-1, inactivityScores[2])
	require.Equal(t, defaultScore-1, inactivityScores[3])
}

func TestProcessInactivityScores_GenesisEpoch(t *testing.T) {
	s, err := testStateZond()
	require.NoError(t, err)
	defaultScore := uint64(10)
	require.NoError(t, s.SetInactivityScores([]uint64{defaultScore, defaultScore, defaultScore, defaultScore}))
	require.NoError(t, s.SetSlot(params.BeaconConfig().GenesisSlot))
	validators, balance, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)
	validators, _, err = ProcessEpochParticipation(context.Background(), s, balance, validators)
	require.NoError(t, err)
	s, _, err = ProcessInactivityScores(context.Background(), s, validators)
	require.NoError(t, err)
	inactivityScores, err := s.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, defaultScore, inactivityScores[0])
	require.Equal(t, defaultScore, inactivityScores[1])
	require.Equal(t, defaultScore, inactivityScores[2])
	require.Equal(t, defaultScore, inactivityScores[3])
}

func TestProcessInactivityScores_CanProcessNonInactivityLeak(t *testing.T) {
	s, err := testStateZond()
	require.NoError(t, err)
	defaultScore := uint64(5)
	require.NoError(t, s.SetInactivityScores([]uint64{defaultScore, defaultScore, defaultScore, defaultScore}))
	validators, balance, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)
	validators, _, err = ProcessEpochParticipation(context.Background(), s, balance, validators)
	require.NoError(t, err)
	s, _, err = ProcessInactivityScores(context.Background(), s, validators)
	require.NoError(t, err)
	inactivityScores, err := s.InactivityScores()
	require.NoError(t, err)

	require.Equal(t, uint64(0), inactivityScores[0])
	require.Equal(t, uint64(0), inactivityScores[1])
	require.Equal(t, uint64(0), inactivityScores[2])
	require.Equal(t, uint64(0), inactivityScores[3])
}

func TestProcessRewardsAndPenaltiesPrecompute_GenesisEpoch(t *testing.T) {
	s, err := testStateZond()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)
	validators, balance, err = ProcessEpochParticipation(context.Background(), s, balance, validators)
	require.NoError(t, err)
	require.NoError(t, s.SetSlot(0))
	s, err = ProcessRewardsAndPenaltiesPrecompute(s, balance, validators)
	require.NoError(t, err)

	balances := s.Balances()
	// Nothing should happen at genesis epoch
	require.Equal(t, uint64(0), balances[0])
	for i := 1; i < len(balances); i++ {
		require.Equal(t, true, balances[i] == balances[i-1])
	}
}

func TestProcessRewardsAndPenaltiesPrecompute_BadState(t *testing.T) {
	s, err := testStateZond()
	require.NoError(t, err)
	validators, balance, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)
	_, balance, err = ProcessEpochParticipation(context.Background(), s, balance, validators)
	require.NoError(t, err)
	_, err = ProcessRewardsAndPenaltiesPrecompute(s, balance, []*precompute.Validator{})
	require.ErrorContains(t, "validator registries not the same length as state's validator registries", err)
}

func TestProcessInactivityScores_NonEligibleValidator(t *testing.T) {
	s, err := testStateZond()
	require.NoError(t, err)
	defaultScore := uint64(5)
	require.NoError(t, s.SetInactivityScores([]uint64{defaultScore, defaultScore, defaultScore, defaultScore}))
	validators, balance, err := InitializePrecomputeValidators(context.Background(), s)
	require.NoError(t, err)

	// v0 is eligible (not active previous epoch, slashed and not withdrawable)
	validators[0].IsActivePrevEpoch = false
	validators[0].IsSlashed = true
	validators[0].IsWithdrawableCurrentEpoch = false

	// v1 is not eligible (not active previous epoch, not slashed and not withdrawable)
	validators[1].IsActivePrevEpoch = false
	validators[1].IsSlashed = false
	validators[1].IsWithdrawableCurrentEpoch = false

	// v2 is not eligible (not active previous epoch, slashed and withdrawable)
	validators[2].IsActivePrevEpoch = false
	validators[2].IsSlashed = true
	validators[2].IsWithdrawableCurrentEpoch = true

	// v3 is eligible (active previous epoch)
	validators[3].IsActivePrevEpoch = true

	validators, _, err = ProcessEpochParticipation(context.Background(), s, balance, validators)
	require.NoError(t, err)
	s, _, err = ProcessInactivityScores(context.Background(), s, validators)
	require.NoError(t, err)
	inactivityScores, err := s.InactivityScores()
	require.NoError(t, err)

	require.Equal(t, uint64(0), inactivityScores[0])
	require.Equal(t, defaultScore, inactivityScores[1]) // Should remain unchanged
	require.Equal(t, defaultScore, inactivityScores[2]) // Should remain unchanged
	require.Equal(t, uint64(0), inactivityScores[3])
}

func testStateZond() (state.BeaconState, error) {
	generateParticipation := func(flags ...uint8) byte {
		b := byte(0)
		var err error
		for _, flag := range flags {
			b, err = AddValidatorFlag(b, flag)
			if err != nil {
				return 0
			}
		}
		return b
	}
	return state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: 2 * params.BeaconConfig().SlotsPerEpoch,
		Validators: []*qrysmpb.Validator{
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
			{EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance, ExitEpoch: params.BeaconConfig().FarFutureEpoch},
		},
		CurrentEpochParticipation: []byte{
			0,
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex),
		},
		PreviousEpochParticipation: []byte{
			0,
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex),
			generateParticipation(params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex),
		},
		InactivityScores: []uint64{0, 0, 0, 0},
		Balances:         []uint64{0, 0, 0, 0},
	})
}
