// Package state contains types for state operation-specific events fired
// during the runtime of a beacon node such as state initialization and state
// updates.
package state

import (
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
)

const (
	// BlockProcessed is sent after a block has been processed and updated the state database.
	BlockProcessed = iota + 1
	// Reorg is an event sent when the new head is not a descendant of the previous head.
	Reorg
	// FinalizedCheckpoint event.
	FinalizedCheckpoint
	// NewHead of the chain event.
	NewHead
	// MissedSlot is sent when we need to notify users that a slot was missed.
	MissedSlot
)

// BlockProcessedData is the data sent with BlockProcessed events.
type BlockProcessedData struct {
	// Slot is the slot of the processed block.
	Slot primitives.Slot
	// BlockRoot of the processed block.
	BlockRoot [32]byte
	// SignedBlock is the physical processed block.
	SignedBlock interfaces.ReadOnlySignedBeaconBlock
	// Verified is true if the block's ML-DSA-87 contents have been verified.
	Verified bool
	// Optimistic is true if the block is optimistic.
	Optimistic bool
}
