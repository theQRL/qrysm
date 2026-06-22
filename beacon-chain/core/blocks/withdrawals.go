package blocks

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/encoding/ssz"
)

// ProcessWithdrawals processes the validator withdrawals from the provided execution payload
// into the beacon state.
//
// Spec pseudocode definition:
//
// def process_withdrawals(state: BeaconState, payload: ExecutionPayload) -> None:
//
//	expected_withdrawals = get_expected_withdrawals(state)
//	assert len(payload.withdrawals) == len(expected_withdrawals)
//
//	for expected_withdrawal, withdrawal in zip(expected_withdrawals, payload.withdrawals):
//	    assert withdrawal == expected_withdrawal
//	    decrease_balance(state, withdrawal.validator_index, withdrawal.amount)
//
//	# Update the next withdrawal index if this block contained withdrawals
//	if len(expected_withdrawals) != 0:
//	    latest_withdrawal = expected_withdrawals[-1]
//	    state.next_withdrawal_index = WithdrawalIndex(latest_withdrawal.index + 1)
//
//	# Update the next validator index to start the next withdrawal sweep
//	if len(expected_withdrawals) == MAX_WITHDRAWALS_PER_PAYLOAD:
//	    # Next sweep starts after the latest withdrawal's validator index
//	    next_validator_index = ValidatorIndex((expected_withdrawals[-1].validator_index + 1) % len(state.validators))
//	    state.next_withdrawal_validator_index = next_validator_index
//	else:
//	    # Advance sweep by the max length of the sweep if there was not a full set of withdrawals
//	    next_index = state.next_withdrawal_validator_index + MAX_VALIDATORS_PER_WITHDRAWALS_SWEEP
//	    next_validator_index = ValidatorIndex(next_index % len(state.validators))
//	    state.next_withdrawal_validator_index = next_validator_index
func ProcessWithdrawals(st state.BeaconState, executionData interfaces.ExecutionData) (state.BeaconState, error) {
	expectedWithdrawals, err := st.ExpectedWithdrawals()
	if err != nil {
		return nil, errors.Wrap(err, "could not get expected withdrawals")
	}

	var wdRoot [32]byte
	if executionData.IsBlinded() {
		r, err := executionData.WithdrawalsRoot()
		if err != nil {
			return nil, errors.Wrap(err, "could not get withdrawals root")
		}
		wdRoot = bytesutil.ToBytes32(r)
	} else {
		wds, err := executionData.Withdrawals()
		if err != nil {
			return nil, errors.Wrap(err, "could not get withdrawals")
		}
		wdRoot, err = ssz.WithdrawalSliceRoot(wds, fieldparams.MaxWithdrawalsPerPayload)
		if err != nil {
			return nil, errors.Wrap(err, "could not get withdrawals root")
		}
	}

	expectedRoot, err := ssz.WithdrawalSliceRoot(expectedWithdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, errors.Wrap(err, "could not get expected withdrawals root")
	}
	if expectedRoot != wdRoot {
		return nil, fmt.Errorf("expected withdrawals root %#x, got %#x", expectedRoot, wdRoot)
	}

	for _, withdrawal := range expectedWithdrawals {
		err := helpers.DecreaseBalance(st, withdrawal.ValidatorIndex, withdrawal.Amount)
		if err != nil {
			return nil, errors.Wrap(err, "could not decrease balance")
		}
	}
	if len(expectedWithdrawals) > 0 {
		if err := st.SetNextWithdrawalIndex(expectedWithdrawals[len(expectedWithdrawals)-1].Index + 1); err != nil {
			return nil, errors.Wrap(err, "could not set next withdrawal index")
		}
	}
	var nextValidatorIndex primitives.ValidatorIndex
	if uint64(len(expectedWithdrawals)) < params.BeaconConfig().MaxWithdrawalsPerPayload {
		nextValidatorIndex, err = st.NextWithdrawalValidatorIndex()
		if err != nil {
			return nil, errors.Wrap(err, "could not get next withdrawal validator index")
		}
		nextValidatorIndex += primitives.ValidatorIndex(params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep)
		nextValidatorIndex = nextValidatorIndex % primitives.ValidatorIndex(st.NumValidators())
	} else {
		nextValidatorIndex = expectedWithdrawals[len(expectedWithdrawals)-1].ValidatorIndex + 1
		if nextValidatorIndex == primitives.ValidatorIndex(st.NumValidators()) {
			nextValidatorIndex = 0
		}
	}
	if err := st.SetNextWithdrawalValidatorIndex(nextValidatorIndex); err != nil {
		return nil, errors.Wrap(err, "could not set next withdrawal validator index")
	}
	return st, nil
}
