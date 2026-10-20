package epoch_test

import (
	"context"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/epoch"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/beacon-chain/state/stateutil"
	"github.com/theQRL/qrysm/config/features"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/protobuf/proto"
)

func TestUnslashedAttestingIndices_CanSortAndFilter(t *testing.T) {
	// Generate 2 attestations.
	atts := make([]*qrysmpb.PendingAttestation, 2)
	for i := range atts {
		atts[i] = &qrysmpb.PendingAttestation{
			Data: &qrysmpb.AttestationData{Source: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				Target: &qrysmpb.Checkpoint{Epoch: 0, Root: make([]byte, fieldparams.RootLength)},
			},
			AggregationBits: bitfield.Bitlist{0xFF},
		}
	}

	// Generate validators and state for the 2 attestations.
	validatorCount := 1000
	validators := make([]*qrysmpb.Validator, validatorCount)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			ExitEpoch: params.BeaconConfig().FarFutureEpoch,
		}
	}
	base := &qrysmpb.BeaconStateZond{
		Validators:  validators,
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	}
	beaconState, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)

	indices, err := epoch.UnslashedAttestingIndices(context.Background(), beaconState, atts)
	require.NoError(t, err)
	for i := 0; i < len(indices)-1; i++ {
		if indices[i] >= indices[i+1] {
			t.Error("sorted indices not sorted or duplicated")
		}
	}

	// Verify the slashed validator is filtered.
	slashedValidator := indices[0]
	validators = beaconState.Validators()
	validators[slashedValidator].Slashed = true
	require.NoError(t, beaconState.SetValidators(validators))
	indices, err = epoch.UnslashedAttestingIndices(context.Background(), beaconState, atts)
	require.NoError(t, err)
	for i := range indices {
		assert.NotEqual(t, slashedValidator, indices[i], "Slashed validator %d is not filtered", slashedValidator)
	}
}

func TestUnslashedAttestingIndices_DuplicatedAttestations(t *testing.T) {
	// Generate 5 of the same attestations.
	atts := make([]*qrysmpb.PendingAttestation, 5)
	for i := range atts {
		atts[i] = &qrysmpb.PendingAttestation{
			Data: &qrysmpb.AttestationData{Source: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				Target: &qrysmpb.Checkpoint{Epoch: 0}},
			AggregationBits: bitfield.Bitlist{0xFF},
		}
	}

	// Generate validators and state for the 5 attestations.
	validatorCount := 1000
	validators := make([]*qrysmpb.Validator, validatorCount)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			ExitEpoch: params.BeaconConfig().FarFutureEpoch,
		}
	}
	base := &qrysmpb.BeaconStateZond{
		Validators:  validators,
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	}
	beaconState, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)

	indices, err := epoch.UnslashedAttestingIndices(context.Background(), beaconState, atts)
	require.NoError(t, err)

	for i := 0; i < len(indices)-1; i++ {
		if indices[i] >= indices[i+1] {
			t.Error("sorted indices not sorted or duplicated")
		}
	}
}

func TestUnslashedAttestingIndices_AllSlashingPatterns(t *testing.T) {
	ctx := context.Background()
	cfg := params.BeaconConfig()
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%t", experimental), func(t *testing.T) {
			flags := *features.Get()
			flags.EnableExperimentalState = experimental
			t.Cleanup(features.InitWithReset(&flags))
			helpers.ClearCache()
			t.Cleanup(helpers.ClearCache)

			validators := make([]*qrysmpb.Validator, int(cfg.SlotsPerEpoch)*4)
			for i := range validators {
				validators[i] = &qrysmpb.Validator{
					ExitEpoch:        cfg.FarFutureEpoch,
					EffectiveBalance: cfg.MaxEffectiveBalance - uint64(i%4)*cfg.EffectiveBalanceIncrement,
				}
			}
			base, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
				Validators:  validators,
				RandaoMixes: make([][]byte, cfg.EpochsPerHistoricalVector),
			})
			require.NoError(t, err)
			committee, err := helpers.BeaconCommitteeFromState(ctx, base, 0, 0)
			require.NoError(t, err)
			require.Equal(t, 4, len(committee))
			sorted := slices.Clone(committee)
			slices.Sort(sorted)
			att := &qrysmpb.PendingAttestation{
				Data:            &qrysmpb.AttestationData{},
				AggregationBits: bitfield.Bitlist{0x1f}, // All four committee members attest.
			}
			// Duplicate attestations must not duplicate indices or effective balance.
			atts := []*qrysmpb.PendingAttestation{att, att}

			for mask := uint8(0); mask < 16; mask++ {
				t.Run(fmt.Sprintf("slashed=%04b", mask), func(t *testing.T) {
					st := base.Copy()
					want := make([]primitives.ValidatorIndex, 0, len(sorted))
					var wantBalance uint64
					for i, index := range sorted {
						v, err := st.ValidatorAtIndex(index)
						require.NoError(t, err)
						if mask&(1<<i) != 0 {
							v.Slashed = true
							require.NoError(t, st.UpdateValidatorAtIndex(index, v))
						} else {
							want = append(want, index)
							wantBalance += v.EffectiveBalance
						}
					}
					got, err := epoch.UnslashedAttestingIndices(ctx, st, atts)
					require.NoError(t, err)
					assert.DeepEqual(t, want, got)
					balance, err := epoch.AttestingBalance(ctx, st, atts)
					require.NoError(t, err)
					assert.Equal(t, max(wantBalance, cfg.EffectiveBalanceIncrement), balance)
				})
			}
			t.Run("no attestations", func(t *testing.T) {
				got, err := epoch.UnslashedAttestingIndices(ctx, base, nil)
				require.NoError(t, err)
				require.Equal(t, 0, len(got))
				balance, err := epoch.AttestingBalance(ctx, base, nil)
				require.NoError(t, err)
				require.Equal(t, cfg.EffectiveBalanceIncrement, balance)
			})
		})
	}
}

func TestAttestingBalance_CorrectBalance(t *testing.T) {
	helpers.ClearCache()
	// Generate 2 attestations.
	atts := make([]*qrysmpb.PendingAttestation, 2)
	for i := range atts {
		atts[i] = &qrysmpb.PendingAttestation{
			Data: &qrysmpb.AttestationData{
				Target: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				Source: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				Slot:   primitives.Slot(i),
			},
		}
	}

	// Generate validators with balances and state for the 2 attestations.
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
		Slot:        2,
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),

		Validators: validators,
		Balances:   balances,
	}
	beaconState, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)

	expectedParticipants := make(map[primitives.ValidatorIndex]struct{})
	for i := range atts {
		committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, atts[i].Data.Slot, atts[i].Data.CommitteeIndex)
		require.NoError(t, err)

		aggBits := bitfield.NewBitlist(uint64(len(committee)))
		for j, idx := range committee {
			aggBits.SetBitAt(uint64(j), true)
			expectedParticipants[idx] = struct{}{}
		}
		atts[i].AggregationBits = aggBits
	}

	balance, err := epoch.AttestingBalance(context.Background(), beaconState, atts)
	require.NoError(t, err)
	var wanted uint64
	for idx := range expectedParticipants {
		wanted += balances[idx]
	}
	assert.Equal(t, wanted, balance)
}

func TestProcessSlashings_NotSlashed(t *testing.T) {
	base := &qrysmpb.BeaconStateZond{
		Slot:       0,
		Validators: []*qrysmpb.Validator{{Slashed: true}},
		Balances:   []uint64{params.BeaconConfig().MaxEffectiveBalance},
		Slashings:  []uint64{0, 1e9},
	}
	s, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)
	newState, err := epoch.ProcessSlashings(s, params.BeaconConfig().ProportionalSlashingMultiplier)
	require.NoError(t, err)
	wanted := params.BeaconConfig().MaxEffectiveBalance
	assert.Equal(t, wanted, newState.Balances()[0], "Unexpected slashed balance")
}

func TestProcessSlashings_WindowMidpoint(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig()
	slashedEpoch := cfg.EpochsPerSlashingsVector + 7
	for _, delay := range []primitives.Epoch{0, cfg.MinValidatorWithdrawabilityDelay} {
		t.Run(fmt.Sprintf("withdrawal_delay_%d", delay), func(t *testing.T) {
			helpers.ClearCache()
			registry := make([]*qrysmpb.Validator, 9)
			balances := make([]uint64, len(registry))
			for i := range registry {
				registry[i] = &qrysmpb.Validator{EffectiveBalance: cfg.MaxEffectiveBalance, ExitEpoch: cfg.FarFutureEpoch}
				balances[i] = cfg.MaxEffectiveBalance
			}
			registry[0].Slashed = true
			registry[0].ExitEpoch = helpers.ActivationExitEpoch(slashedEpoch)
			registry[0].WithdrawableEpoch = slashedEpoch + cfg.EpochsPerSlashingsVector + delay
			slashings := make([]uint64, cfg.EpochsPerSlashingsVector)
			slashings[slashedEpoch%cfg.EpochsPerSlashingsVector] = cfg.MaxEffectiveBalance
			st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
				Validators: registry, Balances: balances, Slashings: slashings,
			})
			require.NoError(t, err)
			assessmentEpoch := registry[0].WithdrawableEpoch - cfg.EpochsPerSlashingsVector/2
			// Eight unslashed validators remain active; the correlation penalty is 3/8
			// of the slashed validator's effective balance, rounded to the increment.
			penalty := (cfg.MaxEffectiveBalance / cfg.EffectiveBalanceIncrement) * cfg.ProportionalSlashingMultiplier / 8 * cfg.EffectiveBalanceIncrement
			for _, epochToCheck := range []primitives.Epoch{assessmentEpoch - 1, assessmentEpoch, assessmentEpoch + 1} {
				require.NoError(t, st.SetSlot(primitives.Slot(epochToCheck)*cfg.SlotsPerEpoch))
				_, err := epoch.ProcessSlashings(st, cfg.ProportionalSlashingMultiplier)
				require.NoError(t, err)
				want := cfg.MaxEffectiveBalance
				if epochToCheck >= assessmentEpoch {
					want -= penalty
				}
				require.Equal(t, want, st.Balances()[0], "correlation penalty at epoch %d", epochToCheck)
			}
		})
	}
}

func TestProcessSlashingsReset_WindowRollover(t *testing.T) {
	cfg := params.BeaconConfig()
	want := make([]uint64, cfg.EpochsPerSlashingsVector)
	for i := range want {
		want[i] = uint64(i + 1)
	}
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Slashings: append([]uint64(nil), want...)})
	require.NoError(t, err)
	for _, currentEpoch := range []primitives.Epoch{
		cfg.EpochsPerSlashingsVector - 2,
		cfg.EpochsPerSlashingsVector - 1,
		cfg.EpochsPerSlashingsVector,
		2*cfg.EpochsPerSlashingsVector - 1,
	} {
		require.NoError(t, st.SetSlot(primitives.Slot(currentEpoch)*cfg.SlotsPerEpoch))
		_, err := epoch.ProcessSlashingsReset(st)
		require.NoError(t, err)
		want[(currentEpoch+1)%cfg.EpochsPerSlashingsVector] = 0
		require.DeepEqual(t, want, st.Slashings(), "only the next epoch's ring-buffer entry should be cleared")
	}
}

func TestProcessSlashings_SlashedLess(t *testing.T) {
	tests := []struct {
		state *qrysmpb.BeaconStateZond
		want  uint64
	}{
		{
			state: &qrysmpb.BeaconStateZond{
				Validators: []*qrysmpb.Validator{
					{Slashed: true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance}},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
				Slashings: []uint64{0, 1e9},
			},
			// penalty    = validator balance / increment * (2*total_penalties) / total_balance * increment
			// 1000000000 = (32 * 1e9)        / (1 * 1e9) * (1*1e9)             / (32*1e9)      * (1 * 1e9)
			want: uint64(39997000000000),
		},
		{
			state: &qrysmpb.BeaconStateZond{
				Validators: []*qrysmpb.Validator{
					{Slashed: true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
				},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
				Slashings: []uint64{0, 1e9},
			},
			// penalty    = validator balance / increment * (2*total_penalties) / total_balance * increment
			// 500000000 = (32 * 1e9)        / (1 * 1e9) * (1*1e9)             / (32*1e9)      * (1 * 1e9)
			want: 39999000000000,
		},
		{
			state: &qrysmpb.BeaconStateZond{
				Validators: []*qrysmpb.Validator{
					{Slashed: true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance},
				},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance, params.BeaconConfig().MaxEffectiveBalance},
				Slashings: []uint64{0, 2 * 1e9},
			},
			// penalty    = validator balance / increment * (3*total_penalties) / total_balance * increment
			// 1000000000 = (32 * 1e9)        / (1 * 1e9) * (1*2e9)             / (64*1e9)      * (1 * 1e9)
			want: 39997000000000,
		},
		{
			state: &qrysmpb.BeaconStateZond{
				Validators: []*qrysmpb.Validator{
					{Slashed: true,
						WithdrawableEpoch: params.BeaconConfig().EpochsPerSlashingsVector / 2,
						EffectiveBalance:  params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement},
					{ExitEpoch: params.BeaconConfig().FarFutureEpoch, EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement}},
				Balances:  []uint64{params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement, params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement},
				Slashings: []uint64{0, 1e9},
			},
			// penalty    = validator balance           / increment * (3*total_penalties) / total_balance        * increment
			// 2000000000 = (32  * 1e9 - 1*1e9)         / (1 * 1e9) * (2*1e9)             / (31*1e9)             * (1 * 1e9)
			want: 39996000000000,
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			original := proto.Clone(tt.state)
			s, err := state_native.InitializeFromProtoZond(tt.state)
			require.NoError(t, err)
			helpers.ClearCache()
			newState, err := epoch.ProcessSlashings(s, params.BeaconConfig().ProportionalSlashingMultiplier)
			require.NoError(t, err)
			assert.Equal(t, tt.want, newState.Balances()[0], "ProcessSlashings({%v}) = newState; newState.Balances[0] = %d", original, newState.Balances()[0])
		})
	}
}

func TestProcessRegistryUpdates_NoRotation(t *testing.T) {
	base := &qrysmpb.BeaconStateZond{
		Slot: 5 * params.BeaconConfig().SlotsPerEpoch,
		Validators: []*qrysmpb.Validator{
			{ExitEpoch: params.BeaconConfig().MaxSeedLookahead},
			{ExitEpoch: params.BeaconConfig().MaxSeedLookahead},
		},
		Balances: []uint64{
			params.BeaconConfig().MaxEffectiveBalance,
			params.BeaconConfig().MaxEffectiveBalance,
		},
		FinalizedCheckpoint: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}
	beaconState, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)
	newState, err := epoch.ProcessRegistryUpdates(context.Background(), beaconState)
	require.NoError(t, err)
	for i, validator := range newState.Validators() {
		assert.Equal(t, params.BeaconConfig().MaxSeedLookahead, validator.ExitEpoch, "Could not update registry %d", i)
	}
}

func TestProcessRegistryUpdates_EligibleToActivate(t *testing.T) {
	base := &qrysmpb.BeaconStateZond{
		Slot:                5 * params.BeaconConfig().SlotsPerEpoch,
		FinalizedCheckpoint: &qrysmpb.Checkpoint{Epoch: 6, Root: make([]byte, fieldparams.RootLength)},
	}
	limit := helpers.ValidatorActivationChurnLimit(0)
	for i := uint64(0); i < limit+10; i++ {
		base.Validators = append(base.Validators, &qrysmpb.Validator{
			ActivationEligibilityEpoch: params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			ActivationEpoch:            params.BeaconConfig().FarFutureEpoch,
		})
	}
	beaconState, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)
	currentEpoch := time.CurrentEpoch(beaconState)
	newState, err := epoch.ProcessRegistryUpdates(context.Background(), beaconState)
	require.NoError(t, err)
	for i, validator := range newState.Validators() {
		assert.Equal(t, currentEpoch+1, validator.ActivationEligibilityEpoch, "Could not update registry %d, unexpected activation eligibility epoch", i)
		if uint64(i) < limit && validator.ActivationEpoch != helpers.ActivationExitEpoch(currentEpoch) {
			t.Errorf("Could not update registry %d, validators failed to activate: wanted activation epoch %d, got %d",
				i, helpers.ActivationExitEpoch(currentEpoch), validator.ActivationEpoch)
		}
		if uint64(i) >= limit && validator.ActivationEpoch != params.BeaconConfig().FarFutureEpoch {
			t.Errorf("Could not update registry %d, validators should not have been activated, wanted activation epoch: %d, got %d",
				i, params.BeaconConfig().FarFutureEpoch, validator.ActivationEpoch)
		}
	}
}

func TestProcessRegistryUpdates_ActivationCompletes(t *testing.T) {
	base := &qrysmpb.BeaconStateZond{
		Slot: 5 * params.BeaconConfig().SlotsPerEpoch,
		Validators: []*qrysmpb.Validator{
			{ExitEpoch: params.BeaconConfig().MaxSeedLookahead,
				ActivationEpoch: 5 + params.BeaconConfig().MaxSeedLookahead + 1},
			{ExitEpoch: params.BeaconConfig().MaxSeedLookahead,
				ActivationEpoch: 5 + params.BeaconConfig().MaxSeedLookahead + 1},
		},
		FinalizedCheckpoint: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}
	beaconState, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)
	newState, err := epoch.ProcessRegistryUpdates(context.Background(), beaconState)
	require.NoError(t, err)
	for i, validator := range newState.Validators() {
		assert.Equal(t, params.BeaconConfig().MaxSeedLookahead, validator.ExitEpoch, "Could not update registry %d, unexpected exit slot", i)
	}
}

func TestProcessRegistryUpdates_EnforcesActiveValidatorCapacity(t *testing.T) {
	for _, name := range []string{"default committees", "two committees with unsafe gap"} {
		t.Run(name, func(t *testing.T) {
			cfg := params.MainnetConfig().Copy()
			if fieldparams.Preset == "minimal" {
				cfg = params.MinimalSpecConfig().Copy()
			}
			wantCapacity := uint64(cfg.SlotsPerEpoch) * cfg.MaxCommitteesPerSlot * cfg.MaxValidatorsPerCommittee
			if name == "two committees with unsafe gap" {
				cfg.MaxCommitteesPerSlot = 2
				cfg.TargetCommitteeSize = cfg.MaxValidatorsPerCommittee
				wantCapacity = uint64(cfg.SlotsPerEpoch) * cfg.MaxValidatorsPerCommittee
			}
			testRegistryActiveValidatorCapacity(t, cfg, wantCapacity)
		})
	}
}

func testRegistryActiveValidatorCapacity(t *testing.T, cfg *params.BeaconChainConfig, wantCapacity uint64) {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(cfg)

	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)

	maxActiveValidators, err := cfg.MaxActiveValidators()
	require.NoError(t, err)
	require.Equal(t, wantCapacity, maxActiveValidators)

	const currentEpoch = primitives.Epoch(5)
	st := buildState(t, cfg.SlotsPerEpoch.Mul(uint64(currentEpoch)), maxActiveValidators-1)
	require.NoError(t, st.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{Epoch: currentEpoch, Root: make([]byte, fieldparams.RootLength)}))

	validators := st.Validators()
	balances := st.Balances()
	for range 2 {
		validators = append(validators, &qrysmpb.Validator{
			ActivationEligibilityEpoch: 0,
			ActivationEpoch:            cfg.FarFutureEpoch,
			ExitEpoch:                  cfg.FarFutureEpoch,
			WithdrawableEpoch:          cfg.FarFutureEpoch,
			EffectiveBalance:           cfg.MaxEffectiveBalance,
		})
		balances = append(balances, cfg.MaxEffectiveBalance)
	}
	require.NoError(t, st.SetValidators(validators))
	require.NoError(t, st.SetBalances(balances))

	firstActivationEpoch := helpers.ActivationExitEpoch(currentEpoch)
	st, err = epoch.ProcessRegistryUpdates(context.Background(), st)
	require.NoError(t, err)
	validators = st.Validators()
	require.Equal(t, firstActivationEpoch, validators[maxActiveValidators-1].ActivationEpoch)
	require.Equal(t, cfg.FarFutureEpoch, validators[maxActiveValidators].ActivationEpoch)

	// The first queued validator is now active at the next transition's target
	// epoch. The second must remain queued, rather than being scheduled past
	// the committee/SSZ capacity.
	nextEpoch := currentEpoch + 1
	require.NoError(t, st.SetSlot(cfg.SlotsPerEpoch.Mul(uint64(nextEpoch))))
	st, err = epoch.ProcessRegistryUpdates(context.Background(), st)
	require.NoError(t, err)
	validators = st.Validators()
	require.Equal(t, firstActivationEpoch, validators[maxActiveValidators-1].ActivationEpoch)
	require.Equal(t, cfg.FarFutureEpoch, validators[maxActiveValidators].ActivationEpoch)

	activeAtNextActivationEpoch := uint64(0)
	for _, validator := range validators {
		if helpers.IsActiveValidator(validator, helpers.ActivationExitEpoch(nextEpoch)) {
			activeAtNextActivationEpoch++
		}
	}
	require.Equal(t, maxActiveValidators, activeAtNextActivationEpoch)
}

func TestProcessRegistryUpdates_ValidatorsEjected(t *testing.T) {
	base := &qrysmpb.BeaconStateZond{
		Slot: 0,
		Validators: []*qrysmpb.Validator{
			{
				ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
				EffectiveBalance: params.BeaconConfig().EjectionBalance - 1,
			},
			{
				ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
				EffectiveBalance: params.BeaconConfig().EjectionBalance - 1,
			},
		},
		FinalizedCheckpoint: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}
	beaconState, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)
	newState, err := epoch.ProcessRegistryUpdates(context.Background(), beaconState)
	require.NoError(t, err)
	for i, validator := range newState.Validators() {
		assert.Equal(t, params.BeaconConfig().MaxSeedLookahead+1, validator.ExitEpoch, "Could not update registry %d, unexpected exit slot", i)
	}
}

func TestProcessRegistryUpdates_CanExits(t *testing.T) {
	e := primitives.Epoch(5)
	exitEpoch := helpers.ActivationExitEpoch(e)
	minWithdrawalDelay := params.BeaconConfig().MinValidatorWithdrawabilityDelay
	base := &qrysmpb.BeaconStateZond{
		Slot: params.BeaconConfig().SlotsPerEpoch.Mul(uint64(e)),
		Validators: []*qrysmpb.Validator{
			{
				ExitEpoch:         exitEpoch,
				WithdrawableEpoch: exitEpoch + minWithdrawalDelay},
			{
				ExitEpoch:         exitEpoch,
				WithdrawableEpoch: exitEpoch + minWithdrawalDelay},
		},
		FinalizedCheckpoint: &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}
	beaconState, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)
	newState, err := epoch.ProcessRegistryUpdates(context.Background(), beaconState)
	require.NoError(t, err)
	for i, validator := range newState.Validators() {
		assert.Equal(t, exitEpoch, validator.ExitEpoch, "Could not update registry %d, unexpected exit slot", i)
	}
}

func buildState(t testing.TB, slot primitives.Slot, validatorCount uint64) state.BeaconState {
	validators := make([]*qrysmpb.Validator, validatorCount)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		}
	}
	validatorBalances := make([]uint64, len(validators))
	for i := range validatorBalances {
		validatorBalances[i] = params.BeaconConfig().MaxEffectiveBalance
	}
	latestActiveIndexRoots := make(
		[][]byte,
		params.BeaconConfig().EpochsPerHistoricalVector,
	)
	for i := range latestActiveIndexRoots {
		latestActiveIndexRoots[i] = params.BeaconConfig().ZeroHash[:]
	}
	latestRandaoMixes := make(
		[][]byte,
		params.BeaconConfig().EpochsPerHistoricalVector,
	)
	for i := range latestRandaoMixes {
		latestRandaoMixes[i] = params.BeaconConfig().ZeroHash[:]
	}
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	if err := s.SetSlot(slot); err != nil {
		t.Error(err)
	}
	if err := s.SetBalances(validatorBalances); err != nil {
		t.Error(err)
	}
	if err := s.SetValidators(validators); err != nil {
		t.Error(err)
	}
	return s
}

func TestProcessSlashings_BadValue(t *testing.T) {
	base := &qrysmpb.BeaconStateZond{
		Slot:       0,
		Validators: []*qrysmpb.Validator{{Slashed: true}},
		Balances:   []uint64{params.BeaconConfig().MaxEffectiveBalance},
		Slashings:  []uint64{math.MaxUint64, 1e9},
	}
	s, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)
	_, err = epoch.ProcessSlashings(s, params.BeaconConfig().ProportionalSlashingMultiplier)
	require.ErrorContains(t, "addition overflows", err)
}

// TestProcessSlashings_OverflowUnderQRLConstants is a regression test for the
// uint64 overflow in the correlation (proportional) slashing penalty under QRL's
// balance constants. The spec factors EFFECTIVE_BALANCE_INCREMENT out of the
// penalty numerator "to avoid uint64 overflow", which is sufficient on Ethereum
// where effective_balance/increment = 32. On QRL,
// MaxEffectiveBalance/EffectiveBalanceIncrement = 40000, so
//
//	penalty_numerator = effective_balance/increment * adjusted_total_slashing
//
// overflows uint64 once adjusted_total_slashing exceeds ~461,168 QRL. Before the
// fix the penalty here wrapped to 75 QRL; ProcessSlashings now computes it in
// 128-bit arithmetic and must produce the spec-correct 1920 QRL.
func TestProcessSlashings_OverflowUnderQRLConstants(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	helpers.ClearCache()
	cfg := params.BeaconConfig()

	// Test premise: QRL's EB/increment ratio is 40000 (Ethereum's is 32).
	require.Equal(t, uint64(40000), cfg.MaxEffectiveBalance/cfg.EffectiveBalanceIncrement)

	// 250 active validators of 40,000 QRL each => 10,000,000 QRL total active
	// balance. validator[0] is the one slashed and penalized this epoch.
	const numValidators = 250
	vals := make([]*qrysmpb.Validator, numValidators)
	bals := make([]uint64, numValidators)
	for i := range vals {
		vals[i] = &qrysmpb.Validator{
			ExitEpoch:        cfg.FarFutureEpoch,
			EffectiveBalance: cfg.MaxEffectiveBalance,
		}
		bals[i] = cfg.MaxEffectiveBalance
	}
	vals[0].Slashed = true
	vals[0].WithdrawableEpoch = cfg.EpochsPerSlashingsVector / 2 // currentEpoch(0) + vector/2

	// Slashings vector sums to 160,000 QRL (e.g. four 40,000-QRL validators
	// slashed within EPOCHS_PER_SLASHINGS_VECTOR). With
	// ProportionalSlashingMultiplier = 3, adjusted_total_slashing = 480,000 QRL,
	// so penalty_numerator = 40000 * 480,000e9 = 1.92e19, which is > 2^64.
	base := &qrysmpb.BeaconStateZond{
		Slot:       0,
		Validators: vals,
		Balances:   bals,
		Slashings:  []uint64{160000 * cfg.EffectiveBalanceIncrement},
	}
	s, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)

	newState, err := epoch.ProcessSlashings(s, cfg.ProportionalSlashingMultiplier)
	require.NoError(t, err)

	start := cfg.MaxEffectiveBalance // 40,000 QRL

	// Spec-correct penalty, computed without overflow:
	//   (effective_balance/increment * adjusted) / total_balance * increment
	// = (40000 * 480,000e9) / 10,000,000e9 * 1e9 = 1920e9  (1,920 QRL)
	// Before the fix the numerator 40000 * 480,000e9 = 1.92e19 wrapped uint64 and
	// produced a 75 QRL penalty.
	correctPenalty := uint64(1920) * cfg.EffectiveBalanceIncrement
	got := newState.Balances()[0]
	require.Equal(t, start-correctPenalty, got,
		"correlation slashing penalty must be computed without uint64 overflow (1920 QRL)")
}

func TestProcessHistoricalDataUpdate(t *testing.T) {
	tests := []struct {
		name     string
		st       func() state.BeaconState
		verifier func(state.BeaconState)
	}{
		{
			name: "no change",
			st: func() state.BeaconState {
				st, _ := util.DeterministicGenesisStateZond(t, 1)
				return st
			},
			verifier: func(st state.BeaconState) {
				roots, err := st.HistoricalRoots()
				require.NoError(t, err)
				require.Equal(t, 0, len(roots))
			},
		},
		{
			name: "after zond can process and get historical summary",
			st: func() state.BeaconState {
				st, _ := util.DeterministicGenesisStateZond(t, 1)
				st, err := transition.ProcessSlots(context.Background(), st, params.BeaconConfig().SlotsPerHistoricalRoot-1)
				require.NoError(t, err)
				return st
			},
			verifier: func(st state.BeaconState) {
				summaries, err := st.HistoricalSummaries()
				require.NoError(t, err)
				require.Equal(t, 1, len(summaries))

				br, err := stateutil.ArraysRoot(st.BlockRoots(), fieldparams.BlockRootsLength)
				require.NoError(t, err)
				sr, err := stateutil.ArraysRoot(st.StateRoots(), fieldparams.StateRootsLength)
				require.NoError(t, err)
				b := &qrysmpb.HistoricalSummary{
					BlockSummaryRoot: br[:],
					StateSummaryRoot: sr[:],
				}
				require.DeepEqual(t, b, summaries[0])
				hrs, err := st.HistoricalRoots()
				require.NoError(t, err)
				require.DeepEqual(t, hrs, [][]byte{})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := epoch.ProcessHistoricalDataUpdate(tt.st())
			require.NoError(t, err)
			tt.verifier(got)
		})
	}
}
