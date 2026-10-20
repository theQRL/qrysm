package doublylinkedtree

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/time/slots"
)

// NewSlot expires proposer boost and realizes checkpoint observations from
// earlier epochs when advancing to a new epoch. The first observed slot need
// not be the epoch boundary. Delayed or repeated ticks cannot repeat a completed
// transition, and interrupted transitions remain available for retry.
func (f *ForkChoice) NewSlot(ctx context.Context, slot primitives.Slot) error {
	f.store.expireProposerBoost(slot)

	epoch := slots.ToEpoch(slot)
	if epoch <= f.lastProcessedEpoch {
		return nil
	}
	if f.pendingEpoch == nil || epoch > *f.pendingEpoch {
		f.pendingEpoch = &epoch
	}
	// Older delayed ticks must not consume a newer pending transition.
	if epoch < *f.pendingEpoch {
		return nil
	}

	// Prepare pruning before realizing checkpoints or node epochs, so an
	// interrupted traversal leaves the epoch transition available for retry.
	epoch = *f.pendingEpoch
	_, finalized := f.store.unrealizedCheckpointsBefore(epoch)
	plan, err := f.store.preparePrune(ctx, finalized)
	if err != nil {
		return err
	}
	if err := f.updateUnrealizedCheckpoints(ctx, epoch); err != nil {
		return errors.Wrap(err, "could not update unrealized checkpoints")
	}
	f.store.applyPrune(plan)
	f.lastProcessedEpoch = epoch
	f.pendingEpoch = nil
	return nil
}
