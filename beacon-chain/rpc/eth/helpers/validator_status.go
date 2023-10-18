package helpers

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/zond/v1"
)

// ValidatorStatus returns a validator's status at the given epoch.
func ValidatorStatus(validator state.ReadOnlyValidator, epoch primitives.Epoch) (zondpb.ValidatorStatus, error) {
	valStatus, err := ValidatorSubStatus(validator, epoch)
	if err != nil {
		return 0, errors.Wrap(err, "could not get sub status")
	}
	switch valStatus {
	case zondpb.ValidatorStatus_PENDING_INITIALIZED, zondpb.ValidatorStatus_PENDING_QUEUED:
		return zondpb.ValidatorStatus_PENDING, nil
	case zondpb.ValidatorStatus_ACTIVE_ONGOING, zondpb.ValidatorStatus_ACTIVE_SLASHED, zondpb.ValidatorStatus_ACTIVE_EXITING:
		return zondpb.ValidatorStatus_ACTIVE, nil
	case zondpb.ValidatorStatus_EXITED_UNSLASHED, zondpb.ValidatorStatus_EXITED_SLASHED:
		return zondpb.ValidatorStatus_EXITED, nil
	case zondpb.ValidatorStatus_WITHDRAWAL_POSSIBLE, zondpb.ValidatorStatus_WITHDRAWAL_DONE:
		return zondpb.ValidatorStatus_WITHDRAWAL, nil
	}
	return 0, errors.New("invalid validator state")
}

// ValidatorSubStatus returns a validator's sub-status at the given epoch.
func ValidatorSubStatus(validator state.ReadOnlyValidator, epoch primitives.Epoch) (zondpb.ValidatorStatus, error) {
	farFutureEpoch := params.BeaconConfig().FarFutureEpoch

	// Pending.
	if validator.ActivationEpoch() > epoch {
		if validator.ActivationEligibilityEpoch() == farFutureEpoch {
			return zondpb.ValidatorStatus_PENDING_INITIALIZED, nil
		} else if validator.ActivationEligibilityEpoch() < farFutureEpoch {
			return zondpb.ValidatorStatus_PENDING_QUEUED, nil
		}
	}

	// Active.
	if validator.ActivationEpoch() <= epoch && epoch < validator.ExitEpoch() {
		if validator.ExitEpoch() == farFutureEpoch {
			return zondpb.ValidatorStatus_ACTIVE_ONGOING, nil
		} else if validator.ExitEpoch() < farFutureEpoch {
			if validator.Slashed() {
				return zondpb.ValidatorStatus_ACTIVE_SLASHED, nil
			}
			return zondpb.ValidatorStatus_ACTIVE_EXITING, nil
		}
	}

	// Exited.
	if validator.ExitEpoch() <= epoch && epoch < validator.WithdrawableEpoch() {
		if validator.Slashed() {
			return zondpb.ValidatorStatus_EXITED_SLASHED, nil
		}
		return zondpb.ValidatorStatus_EXITED_UNSLASHED, nil
	}

	if validator.WithdrawableEpoch() <= epoch {
		if validator.EffectiveBalance() != 0 {
			return zondpb.ValidatorStatus_WITHDRAWAL_POSSIBLE, nil
		} else {
			return zondpb.ValidatorStatus_WITHDRAWAL_DONE, nil
		}
	}

	return 0, errors.New("invalid validator state")
}
