package validator

import (
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func getEmptyBlock(slot primitives.Slot) (interfaces.SignedBeaconBlock, error) {
	var sBlk interfaces.SignedBeaconBlock
	var err error
	switch {
	default:
		sBlk, err = blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockCapella{Block: &zondpb.BeaconBlockCapella{Body: &zondpb.BeaconBlockBodyCapella{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	}
	return sBlk, err
}
