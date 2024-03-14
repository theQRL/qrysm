package state_native

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	mathutil "github.com/theQRL/qrysm/v4/math"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/time/slots"
)

const ETH1AddressOffset = 12

// NextWithdrawalIndex returns the index that will be assigned to the next withdrawal.
func (b *BeaconState) NextWithdrawalIndex() (uint64, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.nextWithdrawalIndex, nil
}

// NextWithdrawalValidatorIndex returns the index of the validator which is
// next in line for a withdrawal.
func (b *BeaconState) NextWithdrawalValidatorIndex() (primitives.ValidatorIndex, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.nextWithdrawalValidatorIndex, nil
}

// ExpectedWithdrawals returns the withdrawals that a proposer will need to pack in the next block
// applied to the current state. It is also used by validators to check that the execution payload carried
// the right number of withdrawals
func (b *BeaconState) ExpectedWithdrawals() ([]*enginev1.Withdrawal, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	withdrawals := make([]*enginev1.Withdrawal, 0, params.BeaconConfig().MaxWithdrawalsPerPayload)
	validatorIndex := b.nextWithdrawalValidatorIndex
	withdrawalIndex := b.nextWithdrawalIndex
	epoch := slots.ToEpoch(b.slot)

	validatorsLen := b.validatorsLen()
	bound := mathutil.Min(uint64(validatorsLen), params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep)
	for i := uint64(0); i < bound; i++ {
		val, err := b.validatorAtIndex(validatorIndex)
		if err != nil {
			return nil, errors.Wrapf(err, "could not retrieve validator at index %d", validatorIndex)
		}
		balance, err := b.balanceAtIndex(validatorIndex)
		if err != nil {
			return nil, errors.Wrapf(err, "could not retrieve balance at index %d", validatorIndex)
		}
		if balance > 0 && isFullyWithdrawableValidator(val, epoch) {
			withdrawals = append(withdrawals, &enginev1.Withdrawal{
				Index:          withdrawalIndex,
				ValidatorIndex: validatorIndex,
				Address:        bytesutil.SafeCopyBytes(val.WithdrawalCredentials[ETH1AddressOffset:]),
				Amount:         balance,
			})
			withdrawalIndex++
		} else if isPartiallyWithdrawableValidator(val, balance) {
			withdrawals = append(withdrawals, &enginev1.Withdrawal{
				Index:          withdrawalIndex,
				ValidatorIndex: validatorIndex,
				Address:        bytesutil.SafeCopyBytes(val.WithdrawalCredentials[ETH1AddressOffset:]),
				Amount:         balance - params.BeaconConfig().MaxEffectiveBalance,
			})
			withdrawalIndex++
		}
		if uint64(len(withdrawals)) == params.BeaconConfig().MaxWithdrawalsPerPayload {
			break
		}
		validatorIndex += 1
		if uint64(validatorIndex) == uint64(validatorsLen) {
			validatorIndex = 0
		}
	}
	return withdrawals, nil
}

// hasETH1WithdrawalCredential returns whether the validator has an ETH1
// Withdrawal prefix. It assumes that the caller has a lock on the state
func hasETH1WithdrawalCredential(val *zondpb.Validator) bool {
	if val == nil {
		return false
	}
	cred := val.WithdrawalCredentials
	return len(cred) > 0 && cred[0] == params.BeaconConfig().ZondAddressWithdrawalPrefixByte
}

// isFullyWithdrawableValidator returns whether the validator is able to perform a full
// withdrawal. This differ from the spec helper in that the balance > 0 is not
// checked. This function assumes that the caller holds a lock on the state
func isFullyWithdrawableValidator(val *zondpb.Validator, epoch primitives.Epoch) bool {
	if val == nil {
		return false
	}
	return hasETH1WithdrawalCredential(val) && val.WithdrawableEpoch <= epoch
}

// isPartiallyWithdrawable returns whether the validator is able to perform a
// partial withdrawal. This function assumes that the caller has a lock on the state
func isPartiallyWithdrawableValidator(val *zondpb.Validator, balance uint64) bool {
	if val == nil {
		return false
	}
	hasMaxBalance := val.EffectiveBalance == params.BeaconConfig().MaxEffectiveBalance
	hasExcessBalance := balance > params.BeaconConfig().MaxEffectiveBalance
	return hasETH1WithdrawalCredential(val) && hasExcessBalance && hasMaxBalance
}
