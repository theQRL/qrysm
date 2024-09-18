package testing

import (
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zond "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
)

type blockMutator struct {
	Capella func(beaconBlock *zond.SignedBeaconBlockCapella)
}

func (m blockMutator) apply(b interfaces.SignedBeaconBlock) (interfaces.SignedBeaconBlock, error) {
	switch b.Version() {
	case version.Capella:
		bb, err := b.PbCapellaBlock()
		if err != nil {
			return nil, err
		}
		m.Capella(bb)
		return blocks.NewSignedBeaconBlock(bb)
	default:
		return nil, blocks.ErrUnsupportedSignedBeaconBlock
	}
}

// SetBlockStateRoot modifies the block's state root.
func SetBlockStateRoot(b interfaces.SignedBeaconBlock, sr [32]byte) (interfaces.SignedBeaconBlock, error) {
	return blockMutator{
		Capella: func(bb *zond.SignedBeaconBlockCapella) { bb.Block.StateRoot = sr[:] },
	}.apply(b)
}

// SetBlockParentRoot modifies the block's parent root.
func SetBlockParentRoot(b interfaces.SignedBeaconBlock, pr [32]byte) (interfaces.SignedBeaconBlock, error) {
	return blockMutator{
		Capella: func(bb *zond.SignedBeaconBlockCapella) { bb.Block.ParentRoot = pr[:] },
	}.apply(b)
}

// SetBlockSlot modifies the block's slot.
func SetBlockSlot(b interfaces.SignedBeaconBlock, s primitives.Slot) (interfaces.SignedBeaconBlock, error) {
	return blockMutator{
		Capella: func(bb *zond.SignedBeaconBlockCapella) { bb.Block.Slot = s },
	}.apply(b)
}

// SetProposerIndex modifies the block's proposer index.
func SetProposerIndex(b interfaces.SignedBeaconBlock, idx primitives.ValidatorIndex) (interfaces.SignedBeaconBlock, error) {
	return blockMutator{
		Capella: func(bb *zond.SignedBeaconBlockCapella) { bb.Block.ProposerIndex = idx },
	}.apply(b)
}
