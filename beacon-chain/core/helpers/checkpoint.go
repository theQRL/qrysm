package helpers

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
)

// ValidateCheckpointActiveValidatorCount checks committee capacity at the state's
// epoch and at every future activation/exit already scheduled in its registry.
// Scan directly, without the committee cache, because the checkpoint has not yet
// been validated. This does not replace trusting the checkpoint's source.
func ValidateCheckpointActiveValidatorCount(st state.ReadOnlyBeaconState) error {
	if st == nil || st.IsNil() {
		return errors.New("nil checkpoint state")
	}
	cfg := params.BeaconConfig()
	capacity, err := cfg.MaxActiveValidators()
	if err != nil {
		return errors.Wrap(err, "could not determine active validator capacity")
	}
	currentEpoch := primitives.Epoch(st.Slot() / cfg.SlotsPerEpoch)
	overflow, err := findActiveValidatorCapacityOverflow(st, currentEpoch, capacity)
	if err != nil {
		return errors.Wrap(err, "could not count checkpoint active validators")
	}
	if overflow != nil {
		return fmt.Errorf("checkpoint active validator count %d at epoch %d exceeds committee capacity %d", overflow.count, overflow.epoch, capacity)
	}
	return nil
}
