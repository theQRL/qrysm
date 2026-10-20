package doublylinkedtree

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/epoch/precompute"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
)

func (s *Store) setUnrealizedJustifiedEpoch(root [32]byte, epoch primitives.Epoch) error {
	node, ok := s.nodeByRoot[root]
	if !ok || node == nil {
		return errors.Wrap(ErrNilNode, "could not set unrealized justified epoch")
	}
	if epoch < node.unrealizedJustifiedEpoch {
		return errInvalidUnrealizedJustifiedEpoch
	}
	node.unrealizedJustifiedEpoch = epoch
	return nil
}

func (s *Store) setUnrealizedFinalizedEpoch(root [32]byte, epoch primitives.Epoch) error {
	node, ok := s.nodeByRoot[root]
	if !ok || node == nil {
		return errors.Wrap(ErrNilNode, "could not set unrealized finalized epoch")
	}
	if epoch < node.unrealizedFinalizedEpoch {
		return errInvalidUnrealizedFinalizedEpoch
	}
	node.unrealizedFinalizedEpoch = epoch
	return nil
}

// updateUnrealizedCheckpoints "realizes" the unrealized justified and finalized
// epochs stored within nodes. It should be called at the beginning of each epoch.
func (f *ForkChoice) updateUnrealizedCheckpoints(ctx context.Context, epoch primitives.Epoch) error {
	jc, fc := f.store.unrealizedCheckpointsBefore(epoch)
	if jc.Epoch > f.store.justifiedCheckpoint.Epoch {
		if err := f.UpdateJustifiedCheckpoint(ctx, jc); err != nil {
			return err
		}
	}
	f.store.finalizedCheckpoint = fc
	for _, node := range f.store.nodeByRoot {
		if slots.ToEpoch(node.slot) < epoch {
			node.justifiedEpoch = node.unrealizedJustifiedEpoch
			node.finalizedEpoch = node.unrealizedFinalizedEpoch
		}
	}
	return nil
}

// unrealizedCheckpointsBefore selects observations from earlier epochs only.
// A block can acquire the store lock before its epoch's slot tick, so the global
// unrealized checkpoints may already include observations from that new epoch.
// Keep those candidates pending for the next epoch, including on repeated ticks.
func (s *Store) unrealizedCheckpointsBefore(epoch primitives.Epoch) (*forkchoicetypes.Checkpoint, *forkchoicetypes.Checkpoint) {
	jc, fc := s.justifiedCheckpoint, s.finalizedCheckpoint
	for _, node := range s.nodeByRoot {
		if slots.ToEpoch(node.slot) >= epoch {
			continue
		}
		// Node metadata can lead the cached candidates during batch imports.
		// Only promote epochs that have also been accepted by the store.
		if node.unrealizedJustifiedEpoch > s.justifiedCheckpoint.Epoch &&
			node.unrealizedJustifiedEpoch <= s.unrealizedJustifiedCheckpoint.Epoch {
			jc = preferUnrealizedCheckpoint(jc, s.unrealizedJustifiedCheckpoint, node.unrealizedJustifiedEpoch, node.unrealizedJustifiedRoot)
		}
		if node.unrealizedFinalizedEpoch > s.finalizedCheckpoint.Epoch &&
			node.unrealizedFinalizedEpoch <= s.unrealizedFinalizedCheckpoint.Epoch {
			fc = preferUnrealizedCheckpoint(fc, s.unrealizedFinalizedCheckpoint, node.unrealizedFinalizedEpoch, node.unrealizedFinalizedRoot)
		}
	}
	return jc, fc
}

func (s *Store) pullTips(state state.BeaconState, node *Node, jc, fc *qrysmpb.Checkpoint) (*qrysmpb.Checkpoint, *qrysmpb.Checkpoint) {
	if node.parent == nil { // Nothing to do if the parent is nil.
		node.unrealizedJustifiedRoot = bytesutil.ToBytes32(jc.Root)
		node.unrealizedFinalizedRoot = bytesutil.ToBytes32(fc.Root)
		return jc, fc
	}
	currentEpoch := slots.ToEpoch(slots.CurrentSlot(s.genesisTime))
	stateEpoch := slots.ToEpoch(state.Slot())
	// Always compute the checkpoints from the state, as the spec's
	// compute_pulled_up_tip does. Inheriting the parent's values is not exact
	// in either direction: with fewer than two thirds of the epoch's slots
	// elapsed, uneven effective balances can still reach the two-thirds
	// quorum, because committees are equal by validator count rather than by
	// balance; and once the parent has justified the current epoch, a late
	// previous-epoch attestation in the child can still justify the previous
	// epoch and thereby finalize one the parent did not.
	uj, uf, err := precompute.UnrealizedCheckpoints(state)
	if err != nil {
		log.WithError(err).Debug("could not compute unrealized checkpoints")
		uj, uf = jc, fc
	}

	s.advanceUnrealizedCheckpoints(uj, uf)

	// Update node's checkpoints.
	node.unrealizedJustifiedEpoch, node.unrealizedFinalizedEpoch = uj.Epoch, uf.Epoch
	node.unrealizedJustifiedRoot = bytesutil.ToBytes32(uj.Root)
	node.unrealizedFinalizedRoot = bytesutil.ToBytes32(uf.Root)
	if stateEpoch < currentEpoch {
		jc, fc = uj, uf
		node.justifiedEpoch = uj.Epoch
		node.finalizedEpoch = uf.Epoch
	}
	return jc, fc
}

// advanceUnrealizedCheckpoints advances each checkpoint independently. A block
// that improves finalization can have an older justification than another branch.
func (s *Store) advanceUnrealizedCheckpoints(jc, fc *qrysmpb.Checkpoint) {
	if jc.Epoch > s.unrealizedJustifiedCheckpoint.Epoch {
		s.unrealizedJustifiedCheckpoint = &forkchoicetypes.Checkpoint{
			Epoch: jc.Epoch, Root: bytesutil.ToBytes32(jc.Root),
		}
	}
	if fc.Epoch > s.unrealizedFinalizedCheckpoint.Epoch {
		s.unrealizedFinalizedCheckpoint = &forkchoicetypes.Checkpoint{
			Epoch: fc.Epoch, Root: bytesutil.ToBytes32(fc.Root),
		}
	}
}

// recomputeUnrealizedCheckpoints discards checkpoint observations from a
// removed subtree. Realized checkpoints are never rolled back, even if an
// execution-invalid branch contained them; that case remains optimistic.
func (s *Store) recomputeUnrealizedCheckpoints() {
	jc, fc := s.justifiedCheckpoint, s.finalizedCheckpoint
	for _, node := range s.nodeByRoot {
		if node.unrealizedJustifiedEpoch > s.justifiedCheckpoint.Epoch {
			jc = s.survivingUnrealizedCheckpoint(jc, s.unrealizedJustifiedCheckpoint, node.unrealizedJustifiedEpoch, node.unrealizedJustifiedRoot)
		}
		if node.unrealizedFinalizedEpoch > s.finalizedCheckpoint.Epoch {
			fc = s.survivingUnrealizedCheckpoint(fc, s.unrealizedFinalizedCheckpoint, node.unrealizedFinalizedEpoch, node.unrealizedFinalizedRoot)
		}
	}
	s.unrealizedJustifiedCheckpoint, s.unrealizedFinalizedCheckpoint = jc, fc
}

func (s *Store) survivingUnrealizedCheckpoint(best, previous *forkchoicetypes.Checkpoint, epoch primitives.Epoch, root [32]byte) *forkchoicetypes.Checkpoint {
	if s.nodeByRoot[root] == nil {
		return best
	}
	return preferUnrealizedCheckpoint(best, previous, epoch, root)
}

func preferUnrealizedCheckpoint(best, previous *forkchoicetypes.Checkpoint, epoch primitives.Epoch, root [32]byte) *forkchoicetypes.Checkpoint {
	if epoch < best.Epoch {
		return best
	}
	// Preserve the previous choice when surviving observations still support
	// it. Otherwise break equal-epoch ties deterministically, independently
	// of map iteration order.
	if epoch == best.Epoch && (best.Root == previous.Root || (root != previous.Root && bytes.Compare(root[:], best.Root[:]) <= 0)) {
		return best
	}
	return &forkchoicetypes.Checkpoint{Epoch: epoch, Root: root}
}
