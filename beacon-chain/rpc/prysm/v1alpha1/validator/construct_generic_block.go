package validator

import (
	"fmt"

	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"google.golang.org/protobuf/proto"
)

// constructGenericBeaconBlock constructs a `GenericBeaconBlock` based on the block version and other parameters.
func (vs *Server) constructGenericBeaconBlock(sBlk interfaces.SignedBeaconBlock, blindBlobs []*zondpb.BlindedBlobSidecar, fullBlobs []*zondpb.BlobSidecar) (*zondpb.GenericBeaconBlock, error) {
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
	case version.Deneb:
		return vs.constructDenebBlock(blockProto, isBlinded, payloadValue, blindBlobs, fullBlobs), nil
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

// Helper functions for constructing blocks for each version
func (vs *Server) constructDenebBlock(blockProto proto.Message, isBlinded bool, payloadValue uint64, blindBlobs []*zondpb.BlindedBlobSidecar, fullBlobs []*zondpb.BlobSidecar) *zondpb.GenericBeaconBlock {
	if isBlinded {
		return &zondpb.GenericBeaconBlock{Block: &zondpb.GenericBeaconBlock_BlindedDeneb{BlindedDeneb: &zondpb.BlindedBeaconBlockAndBlobsDeneb{Block: blockProto.(*zondpb.BlindedBeaconBlockDeneb), Blobs: blindBlobs}}, IsBlinded: true, PayloadValue: payloadValue}
	}
	blockAndBlobs := &zondpb.BeaconBlockAndBlobsDeneb{
		Block: blockProto.(*zondpb.BeaconBlockDeneb),
		Blobs: fullBlobs,
	}
	return &zondpb.GenericBeaconBlock{Block: &zondpb.GenericBeaconBlock_Deneb{Deneb: blockAndBlobs}, IsBlinded: false, PayloadValue: payloadValue}
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
