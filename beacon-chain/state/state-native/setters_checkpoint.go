package state_native

import (
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/v4/beacon-chain/state/state-native/types"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

// SetJustificationBits for the beacon state.
func (b *BeaconState) SetJustificationBits(val bitfield.Bitvector4) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.justificationBits = val
	b.markFieldAsDirty(types.JustificationBits)
	return nil
}

// SetPreviousJustifiedCheckpoint for the beacon state.
func (b *BeaconState) SetPreviousJustifiedCheckpoint(val *zondpb.Checkpoint) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.previousJustifiedCheckpoint = val
	b.markFieldAsDirty(types.PreviousJustifiedCheckpoint)
	return nil
}

// SetCurrentJustifiedCheckpoint for the beacon state.
func (b *BeaconState) SetCurrentJustifiedCheckpoint(val *zondpb.Checkpoint) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.currentJustifiedCheckpoint = val
	b.markFieldAsDirty(types.CurrentJustifiedCheckpoint)
	return nil
}

// SetFinalizedCheckpoint for the beacon state.
func (b *BeaconState) SetFinalizedCheckpoint(val *zondpb.Checkpoint) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.finalizedCheckpoint = val
	b.markFieldAsDirty(types.FinalizedCheckpoint)
	return nil
}
