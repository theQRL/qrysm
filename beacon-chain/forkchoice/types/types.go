package types

import (
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	consensus_blocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// Checkpoint is an array version of qrysmpb.Checkpoint. It is used internally in
// forkchoice, while the slice version is used in the interface to legacy code
// in other packages
type Checkpoint struct {
	Epoch primitives.Epoch
	Root  [fieldparams.RootLength]byte
}

// BlockAndCheckpoints to call the InsertOptimisticChain function
type BlockAndCheckpoints struct {
	Block               consensus_blocks.ROBlock
	JustifiedCheckpoint *qrysmpb.Checkpoint
	FinalizedCheckpoint *qrysmpb.Checkpoint
}
