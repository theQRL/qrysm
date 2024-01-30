package testing

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

// NewSignedBeaconBlockFromGeneric creates a signed beacon block
// from a protobuf generic signed beacon block.
func NewSignedBeaconBlockFromGeneric(gb *zond.GenericSignedBeaconBlock) (interfaces.ReadOnlySignedBeaconBlock, error) {
	if gb == nil {
		return nil, blocks.ErrNilObject
	}
	switch bb := gb.Block.(type) {
	case *zond.GenericSignedBeaconBlock_Phase0:
		return blocks.NewSignedBeaconBlock(bb.Phase0)
	case *zond.GenericSignedBeaconBlock_Altair:
		return blocks.NewSignedBeaconBlock(bb.Altair)
	case *zond.GenericSignedBeaconBlock_Bellatrix:
		return blocks.NewSignedBeaconBlock(bb.Bellatrix)
	case *zond.GenericSignedBeaconBlock_BlindedBellatrix:
		return blocks.NewSignedBeaconBlock(bb.BlindedBellatrix)
	case *zond.GenericSignedBeaconBlock_Capella:
		return blocks.NewSignedBeaconBlock(bb.Capella)
	case *zond.GenericSignedBeaconBlock_BlindedCapella:
		return blocks.NewSignedBeaconBlock(bb.BlindedCapella)
	case *zond.GenericSignedBeaconBlock_Deneb:
		return blocks.NewSignedBeaconBlock(bb.Deneb.Block)
	case *zond.GenericSignedBeaconBlock_BlindedDeneb:
		return blocks.NewSignedBeaconBlock(bb.BlindedDeneb.SignedBlindedBlock)
	// Generic Signed Beacon Block Deneb can't be used here as it is not a block, but block content with blobs
	default:
		return nil, errors.Wrapf(blocks.ErrUnsupportedSignedBeaconBlock, "unable to create block from type %T", gb)
	}
}
