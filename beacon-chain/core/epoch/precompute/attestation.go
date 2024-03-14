package precompute

import (
	"github.com/theQRL/qrysm/v4/config/params"
)

// UpdateBalance updates pre computed balance store.
func UpdateBalance(vp []*Validator, bBal *Balance) *Balance {
	for _, v := range vp {
		if !v.IsSlashed {
			if v.IsCurrentEpochAttester {
				bBal.CurrentEpochAttested += v.CurrentEpochEffectiveBalance
			}
			if v.IsCurrentEpochTargetAttester {
				bBal.CurrentEpochTargetAttested += v.CurrentEpochEffectiveBalance
			}
			if v.IsPrevEpochSourceAttester {
				bBal.PrevEpochAttested += v.CurrentEpochEffectiveBalance
			}
			if v.IsPrevEpochTargetAttester {
				bBal.PrevEpochTargetAttested += v.CurrentEpochEffectiveBalance
			}
			if v.IsPrevEpochHeadAttester {
				bBal.PrevEpochHeadAttested += v.CurrentEpochEffectiveBalance
			}
		}
	}

	return EnsureBalancesLowerBound(bBal)
}

// EnsureBalancesLowerBound ensures all the balances such as active current epoch, active previous epoch and more
// have EffectiveBalanceIncrement(1 eth) as a lower bound.
func EnsureBalancesLowerBound(bBal *Balance) *Balance {
	ebi := params.BeaconConfig().EffectiveBalanceIncrement
	if ebi > bBal.ActiveCurrentEpoch {
		bBal.ActiveCurrentEpoch = ebi
	}
	if ebi > bBal.ActivePrevEpoch {
		bBal.ActivePrevEpoch = ebi
	}
	if ebi > bBal.CurrentEpochAttested {
		bBal.CurrentEpochAttested = ebi
	}
	if ebi > bBal.CurrentEpochTargetAttested {
		bBal.CurrentEpochTargetAttested = ebi
	}
	if ebi > bBal.PrevEpochAttested {
		bBal.PrevEpochAttested = ebi
	}
	if ebi > bBal.PrevEpochTargetAttested {
		bBal.PrevEpochTargetAttested = ebi
	}
	if ebi > bBal.PrevEpochHeadAttested {
		bBal.PrevEpochHeadAttested = ebi
	}
	return bBal
}
