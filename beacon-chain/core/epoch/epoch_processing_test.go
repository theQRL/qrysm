package epoch_test

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/epoch"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/beacon-chain/state/stateutil"
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
