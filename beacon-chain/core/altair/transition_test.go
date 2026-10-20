package altair_test

import (
	"context"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/epoch"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProcessEpoch_CanProcessZond(t *testing.T) {
	st, _ := util.DeterministicGenesisStateZond(t, testValidatorSetSize)
	require.NoError(t, st.SetSlot(10*params.BeaconConfig().SlotsPerEpoch))
	newState, err := altair.ProcessEpoch(context.Background(), st)
	require.NoError(t, err)
	require.Equal(t, uint64(0), newState.Slashings()[2], "Unexpected slashed balance")

	b := st.Balances()
	require.Equal(t, testValidatorSetSize, uint64(len(b)))
	require.Equal(t, uint64(39999263173438), b[0])

	s, err := st.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, testValidatorSetSize, uint64(len(s)))

	p, err := st.PreviousEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, testValidatorSetSize, uint64(len(p)))

	p, err = st.CurrentEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, testValidatorSetSize, uint64(len(p)))

	sc, err := st.CurrentSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))

	sc, err = st.NextSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))
}

func TestProcessEpoch_RewardOrderAfterSyncPenalties(t *testing.T) {
	ctx := context.Background()
	cfg := params.BeaconConfig()
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	st, _ := util.DeterministicGenesisStateZond(t, 16)
	validator, err := st.ValidatorAtIndex(0)
	require.NoError(t, err)
	validator.EffectiveBalance = cfg.EffectiveBalanceIncrement
	validator.ExitEpoch = 8
	validator.WithdrawableEpoch = 8 + cfg.MinValidatorWithdrawabilityDelay
	require.NoError(t, st.UpdateValidatorAtIndex(0, validator))
	// Start at the downward hysteresis boundary. Sync penalties drain the
	// actual balance during the epoch while effective balance stays positive.
	downwardThreshold := cfg.EffectiveBalanceIncrement / cfg.HysteresisQuotient * cfg.HysteresisDownwardMultiplier
	require.NoError(t, st.UpdateBalancesAtIndex(0, cfg.EffectiveBalanceIncrement-downwardThreshold))
	st, err = epoch.ProcessEffectiveBalanceUpdates(st)
	require.NoError(t, err)
	validator, err = st.ValidatorAtIndex(0)
	require.NoError(t, err)
	require.Equal(t, cfg.EffectiveBalanceIncrement, validator.EffectiveBalance)
	// Explicitly install a repeated-member committee to exercise the balance
	// floor; this fixture does not assert reachability from a chain history.
	committee := &qrysmpb.SyncCommittee{Pubkeys: make([][]byte, cfg.SyncCommitteeSize)}
	for i := range committee.Pubkeys {
		committee.Pubkeys[i] = validator.PublicKey
	}
	require.NoError(t, st.SetCurrentSyncCommittee(committee))
	flags := make([]byte, st.NumValidators())
	for i := range flags {
		flags[i] = 1 << cfg.TimelyTargetFlagIndex
	}
	require.NoError(t, st.SetPreviousParticipationBits(flags))
	require.NoError(t, st.SetPreviousJustifiedCheckpoint(&qrysmpb.Checkpoint{Epoch: 4, Root: make([]byte, 32)}))
	require.NoError(t, st.SetCurrentJustifiedCheckpoint(&qrysmpb.Checkpoint{Epoch: 5, Root: make([]byte, 32)}))
	require.NoError(t, st.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{Epoch: 4, Root: make([]byte, 32)}))
	require.NoError(t, st.SetJustificationBits(bitfield.Bitvector4{3}))
	aggregate := &qrysmpb.SyncAggregate{SyncCommitteeBits: bitfield.NewBitvector128()}
	for slot := 6 * cfg.SlotsPerEpoch; slot < 7*cfg.SlotsPerEpoch; slot++ {
		require.NoError(t, st.SetSlot(slot))
		st, _, err = altair.ProcessSyncAggregate(ctx, st, aggregate)
		require.NoError(t, err)
	}
	require.Equal(t, uint64(0), st.Balances()[0])
	validator, err = st.ValidatorAtIndex(0)
	require.NoError(t, err)
	require.Equal(t, cfg.EffectiveBalanceIncrement, validator.EffectiveBalance)
	baseReward, err := altair.BaseRewardPerIncrement(15*cfg.MaxEffectiveBalance + cfg.EffectiveBalanceIncrement)
	require.NoError(t, err)
	targetReward := baseReward * cfg.TimelyTargetWeight / cfg.WeightDenominator
	require.NotEqual(t, uint64(0), targetReward)
	post, err := altair.ProcessEpoch(ctx, st)
	require.NoError(t, err)
	// The source penalty floors at zero before the target reward arrives.
	require.Equal(t, targetReward, post.Balances()[0])
}
