package validator

import (
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	ethpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
)

// Sets the dilithium to exec data for a block.
func (vs *Server) setDilithiumToExecData(blk interfaces.SignedBeaconBlock, headState state.BeaconState) {
	if blk.Version() < version.Capella {
		return
	}
	if err := blk.SetDilithiumToExecutionChanges([]*ethpb.SignedDilithiumToExecutionChange{}); err != nil {
		log.WithError(err).Error("Could not set dilithium to execution data in block")
		return
	}
	changes, err := vs.DilithiumChangesPool.DilithiumToExecChangesForInclusion(headState)
	if err != nil {
		log.WithError(err).Error("Could not get dilithium to execution changes")
		return
	} else {
		if err := blk.SetDilithiumToExecutionChanges(changes); err != nil {
			log.WithError(err).Error("Could not set dilithium to execution changes")
			return
		}
	}
}
