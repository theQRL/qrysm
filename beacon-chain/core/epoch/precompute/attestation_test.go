package precompute_test

import (
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/core/epoch/precompute"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/testing/assert"
)

func TestUpdateBalance(t *testing.T) {
	vp := []*precompute.Validator{
		{IsCurrentEpochAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsCurrentEpochTargetAttester: true, IsCurrentEpochAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsCurrentEpochTargetAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsPrevEpochAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsPrevEpochAttester: true, IsPrevEpochTargetAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsPrevEpochHeadAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsPrevEpochAttester: true, IsPrevEpochHeadAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsSlashed: true, IsCurrentEpochAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
	}
	wantedPBal := &precompute.Balance{
		ActiveCurrentEpoch:         params.BeaconConfig().EffectiveBalanceIncrement,
		ActivePrevEpoch:            params.BeaconConfig().EffectiveBalanceIncrement,
		CurrentEpochAttested:       200 * params.BeaconConfig().EffectiveBalanceIncrement,
		CurrentEpochTargetAttested: 200 * params.BeaconConfig().EffectiveBalanceIncrement,
		PrevEpochAttested:          params.BeaconConfig().EffectiveBalanceIncrement,
		PrevEpochTargetAttested:    100 * params.BeaconConfig().EffectiveBalanceIncrement,
		PrevEpochHeadAttested:      200 * params.BeaconConfig().EffectiveBalanceIncrement,
	}
	pBal := precompute.UpdateBalance(vp, &precompute.Balance{})
	assert.DeepEqual(t, wantedPBal, pBal, "Incorrect balance calculations")
}

func TestUpdateBalanceDifferentVersions(t *testing.T) {
	vp := []*precompute.Validator{
		{IsCurrentEpochAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsCurrentEpochTargetAttester: true, IsCurrentEpochAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsCurrentEpochTargetAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsPrevEpochAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsPrevEpochAttester: true, IsPrevEpochTargetAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsPrevEpochHeadAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsPrevEpochAttester: true, IsPrevEpochHeadAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
		{IsSlashed: true, IsCurrentEpochAttester: true, CurrentEpochEffectiveBalance: 100 * params.BeaconConfig().EffectiveBalanceIncrement},
	}
	wantedPBal := &precompute.Balance{
		ActiveCurrentEpoch:         params.BeaconConfig().EffectiveBalanceIncrement,
		ActivePrevEpoch:            params.BeaconConfig().EffectiveBalanceIncrement,
		CurrentEpochAttested:       200 * params.BeaconConfig().EffectiveBalanceIncrement,
		CurrentEpochTargetAttested: 200 * params.BeaconConfig().EffectiveBalanceIncrement,
		PrevEpochAttested:          params.BeaconConfig().EffectiveBalanceIncrement,
		PrevEpochTargetAttested:    100 * params.BeaconConfig().EffectiveBalanceIncrement,
		PrevEpochHeadAttested:      200 * params.BeaconConfig().EffectiveBalanceIncrement,
	}
	pBal := precompute.UpdateBalance(vp, &precompute.Balance{})
	assert.DeepEqual(t, wantedPBal, pBal, "Incorrect balance calculations")

	pBal = precompute.UpdateBalance(vp, &precompute.Balance{})
	assert.DeepEqual(t, wantedPBal, pBal, "Incorrect balance calculations")
}

func TestEnsureBalancesLowerBound(t *testing.T) {
	b := &precompute.Balance{}
	b = precompute.EnsureBalancesLowerBound(b)
	balanceIncrement := params.BeaconConfig().EffectiveBalanceIncrement
	assert.Equal(t, balanceIncrement, b.ActiveCurrentEpoch, "Did not get wanted active current balance")
	assert.Equal(t, balanceIncrement, b.ActivePrevEpoch, "Did not get wanted active previous balance")
	assert.Equal(t, balanceIncrement, b.CurrentEpochAttested, "Did not get wanted current attested balance")
	assert.Equal(t, balanceIncrement, b.CurrentEpochTargetAttested, "Did not get wanted target attested balance")
	assert.Equal(t, balanceIncrement, b.PrevEpochAttested, "Did not get wanted prev attested balance")
	assert.Equal(t, balanceIncrement, b.PrevEpochTargetAttested, "Did not get wanted prev target attested balance")
	assert.Equal(t, balanceIncrement, b.PrevEpochHeadAttested, "Did not get wanted prev head attested balance")
}
