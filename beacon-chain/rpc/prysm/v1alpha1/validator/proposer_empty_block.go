package validator

import (
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func getEmptyBlock(slot primitives.Slot) (interfaces.SignedBeaconBlock, error) {
	var sBlk interfaces.SignedBeaconBlock
	var err error
	switch {
	case slots.ToEpoch(slot) < params.BeaconConfig().AltairForkEpoch:
		sBlk, err = blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlock{Block: &zondpb.BeaconBlock{Body: &zondpb.BeaconBlockBody{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	case slots.ToEpoch(slot) < params.BeaconConfig().BellatrixForkEpoch:
		sBlk, err = blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockAltair{Block: &zondpb.BeaconBlockAltair{Body: &zondpb.BeaconBlockBodyAltair{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	case slots.ToEpoch(slot) < params.BeaconConfig().CapellaForkEpoch:
		sBlk, err = blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockBellatrix{Block: &zondpb.BeaconBlockBellatrix{Body: &zondpb.BeaconBlockBodyBellatrix{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	default:
		sBlk, err = blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockCapella{Block: &zondpb.BeaconBlockCapella{Body: &zondpb.BeaconBlockBodyCapella{}}})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not initialize block for proposal: %v", err)
		}
	}
	return sBlk, err
}
