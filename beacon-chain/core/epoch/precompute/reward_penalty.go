package precompute

// EligibleForRewards for validator.
//
// Spec code:
// if is_active_validator(v, previous_epoch) or (v.slashed and previous_epoch + 1 < v.withdrawable_epoch)
func EligibleForRewards(v *Validator) bool {
	return v.IsActivePrevEpoch || (v.IsSlashed && !v.IsWithdrawableCurrentEpoch)
}
