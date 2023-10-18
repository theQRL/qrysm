package validator

import (
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

func (vs *Server) getExits(head state.BeaconState, slot primitives.Slot) []*zondpb.SignedVoluntaryExit {
	exits, err := vs.ExitPool.ExitsForInclusion(head, slot)
	if err != nil {
		log.WithError(err).Error("Could not get exits")
		return []*zondpb.SignedVoluntaryExit{}
	}
	return exits
}
