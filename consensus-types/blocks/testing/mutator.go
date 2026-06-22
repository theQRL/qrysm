package testing

import (
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
)

type blockMutator struct {
	Zond func(beaconBlock *qrysmpb.SignedBeaconBlockZond)
}

func (m blockMutator) apply(b interfaces.SignedBeaconBlock) (interfaces.SignedBeaconBlock, error) {
	switch b.Version() {
	case version.Zond:
		bb, err := b.PbZondBlock()
		if err != nil {
			return nil, err
		}
		m.Zond(bb)
		return blocks.NewSignedBeaconBlock(bb)
	default:
		return nil, blocks.ErrUnsupportedSignedBeaconBlock
	}
}

// SetBlockStateRoot modifies the block's state root.
func SetBlockStateRoot(b interfaces.SignedBeaconBlock, sr [32]byte) (interfaces.SignedBeaconBlock, error) {
	return blockMutator{
		Zond: func(bb *qrysmpb.SignedBeaconBlockZond) { bb.Block.StateRoot = sr[:] },
	}.apply(b)
}

// SetBlockParentRoot modifies the block's parent root.
func SetBlockParentRoot(b interfaces.SignedBeaconBlock, pr [32]byte) (interfaces.SignedBeaconBlock, error) {
	return blockMutator{
		Zond: func(bb *qrysmpb.SignedBeaconBlockZond) { bb.Block.ParentRoot = pr[:] },
	}.apply(b)
}

// SetBlockSlot modifies the block's slot.
func SetBlockSlot(b interfaces.SignedBeaconBlock, s primitives.Slot) (interfaces.SignedBeaconBlock, error) {
	return blockMutator{
		Zond: func(bb *qrysmpb.SignedBeaconBlockZond) { bb.Block.Slot = s },
	}.apply(b)
}

// SetProposerIndex modifies the block's proposer index.
func SetProposerIndex(b interfaces.SignedBeaconBlock, idx primitives.ValidatorIndex) (interfaces.SignedBeaconBlock, error) {
	return blockMutator{
		Zond: func(bb *qrysmpb.SignedBeaconBlockZond) { bb.Block.ProposerIndex = idx },
	}.apply(b)
}
