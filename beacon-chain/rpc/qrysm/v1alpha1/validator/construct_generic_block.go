package validator

import (
	"fmt"

	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"google.golang.org/protobuf/proto"
)

// constructGenericBeaconBlock constructs a `GenericBeaconBlock` based on the block version and other parameters.
func (vs *Server) constructGenericBeaconBlock(sBlk interfaces.SignedBeaconBlock) (*zondpb.GenericBeaconBlock, error) {
	if sBlk == nil || sBlk.Block() == nil {
		return nil, fmt.Errorf("block cannot be nil")
	}

	blockProto, err := sBlk.Block().Proto()
	if err != nil {
		return nil, err
	}

	isBlinded := sBlk.IsBlinded()
	payloadValue := sBlk.ValueInGwei()

	switch sBlk.Version() {
	case version.Capella:
		return vs.constructCapellaBlock(blockProto, isBlinded, payloadValue), nil
	case version.Bellatrix:
		return vs.constructBellatrixBlock(blockProto, isBlinded, payloadValue), nil
	case version.Altair:
		return vs.constructAltairBlock(blockProto), nil
	case version.Phase0:
		return vs.constructPhase0Block(blockProto), nil
	default:
		return nil, fmt.Errorf("unknown block version: %d", sBlk.Version())
	}
}

func (vs *Server) constructCapellaBlock(pb proto.Message, isBlinded bool, payloadValue uint64) *zondpb.GenericBeaconBlock {
	if isBlinded {
		return &zondpb.GenericBeaconBlock{Block: &zondpb.GenericBeaconBlock_BlindedCapella{BlindedCapella: pb.(*zondpb.BlindedBeaconBlockCapella)}, IsBlinded: true, PayloadValue: payloadValue}
	}
	return &zondpb.GenericBeaconBlock{Block: &zondpb.GenericBeaconBlock_Capella{Capella: pb.(*zondpb.BeaconBlockCapella)}, IsBlinded: false, PayloadValue: payloadValue}
}

func (vs *Server) constructBellatrixBlock(pb proto.Message, isBlinded bool, payloadValue uint64) *zondpb.GenericBeaconBlock {
	if isBlinded {
		return &zondpb.GenericBeaconBlock{Block: &zondpb.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: pb.(*zondpb.BlindedBeaconBlockBellatrix)}, IsBlinded: true, PayloadValue: payloadValue}
	}
	return &zondpb.GenericBeaconBlock{Block: &zondpb.GenericBeaconBlock_Bellatrix{Bellatrix: pb.(*zondpb.BeaconBlockBellatrix)}, IsBlinded: false, PayloadValue: payloadValue}
}

func (vs *Server) constructAltairBlock(pb proto.Message) *zondpb.GenericBeaconBlock {
	return &zondpb.GenericBeaconBlock{Block: &zondpb.GenericBeaconBlock_Altair{Altair: pb.(*zondpb.BeaconBlockAltair)}}
}

func (vs *Server) constructPhase0Block(pb proto.Message) *zondpb.GenericBeaconBlock {
	return &zondpb.GenericBeaconBlock{Block: &zondpb.GenericBeaconBlock_Phase0{Phase0: pb.(*zondpb.BeaconBlock)}}
}
