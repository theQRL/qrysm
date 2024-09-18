package transition

import (
	"github.com/theQRL/qrysm/config/params"
)

// IsValidGenesisState gets called whenever there's a deposit event,
// it checks whether there's enough effective balance to trigger and
// if the minimum genesis time arrived already.
//
// Spec pseudocode definition:
//
//	def is_valid_genesis_state(state: BeaconState) -> bool:
//	   if state.genesis_time < MIN_GENESIS_TIME:
//	       return False
//	   if len(get_active_validator_indices(state, GENESIS_EPOCH)) < MIN_GENESIS_ACTIVE_VALIDATOR_COUNT:
//	       return False
//	   return True
//
// This method has been modified from the spec to allow whole states not to be saved
// but instead only cache the relevant information.
func IsValidGenesisState(chainStartDepositCount, currentTime uint64) bool {
	if currentTime < params.BeaconConfig().MinGenesisTime {
		return false
	}
	if chainStartDepositCount < params.BeaconConfig().MinGenesisActiveValidatorCount {
		return false
	}
	return true
}
