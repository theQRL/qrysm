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

// BlockAndCheckpoints supplies a block's checkpoint observations to InsertChain.
type BlockAndCheckpoints struct {
	Block               consensus_blocks.ROBlock
	JustifiedCheckpoint *qrysmpb.Checkpoint
	FinalizedCheckpoint *qrysmpb.Checkpoint
	// Optional checkpoints computed from this block's post-state. Backfilled
	// ancestors without their own state use the realized checkpoints instead.
	UnrealizedJustifiedCheckpoint *qrysmpb.Checkpoint
	UnrealizedFinalizedCheckpoint *qrysmpb.Checkpoint
}

// JustifiedBalances is what fork choice weighs votes and proposer boost with.
// Both come from the justified checkpoint's state advanced to the first slot
// of the checkpoint epoch, as store_target_checkpoint_state does.
type JustifiedBalances struct {
	// Balances is indexed by validator index and holds the effective balance
	// of each active, unslashed validator; every other entry is zero.
	Balances []uint64
	// TotalActiveBalance sums the effective balances of all active
	// validators, slashed or not, as get_total_active_balance does.
	TotalActiveBalance uint64
}
