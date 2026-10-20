package helpers

import (
	"slices"

	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
)

type activeValidatorCapacityOverflow struct {
	count uint64
	epoch primitives.Epoch
}

// findActiveValidatorCapacityOverflow returns the first epoch at or after
// fromEpoch whose scheduled active set exceeds capacity, or nil if none does.
// Callers validate the state and config and provide context-specific errors.
// Scan the registry directly so unvalidated states cannot reuse committee caches.
func findActiveValidatorCapacityOverflow(st state.ReadOnlyBeaconState, fromEpoch primitives.Epoch, capacity uint64) (*activeValidatorCapacityOverflow, error) {
	farFutureEpoch := params.BeaconConfig().FarFutureEpoch
	type countChange struct {
		activations uint64
		exits       uint64
	}
	changes := make(map[primitives.Epoch]countChange)
	var activeCount uint64
	if err := st.ReadFromEveryValidator(func(_ int, validator state.ReadOnlyValidator) error {
		activation, exit := validator.ActivationEpoch(), validator.ExitEpoch()
		if activation == farFutureEpoch || exit <= fromEpoch || activation >= exit {
			return nil
		}
		if activation <= fromEpoch {
			activeCount++
		} else {
			change := changes[activation]
			change.activations++
			changes[activation] = change
		}
		if exit != farFutureEpoch {
			change := changes[exit]
			change.exits++
			changes[exit] = change
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if activeCount > capacity {
		return &activeValidatorCapacityOverflow{count: activeCount, epoch: fromEpoch}, nil
	}

	// Counts only change at scheduled events. Sorting these epochs avoids walking
	// a potentially unbounded epoch range supplied by an imported state.
	epochs := make([]primitives.Epoch, 0, len(changes))
	for epoch := range changes {
		epochs = append(epochs, epoch)
	}
	slices.Sort(epochs)
	for _, epoch := range epochs {
		change := changes[epoch]
		// Exits at this epoch free capacity for activations at the same epoch:
		// a validator is active exactly when activation <= epoch < exit.
		activeCount = activeCount - change.exits + change.activations
		if activeCount > capacity {
			return &activeValidatorCapacityOverflow{count: activeCount, epoch: epoch}, nil
		}
	}
	return nil, nil
}
