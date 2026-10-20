package stategen

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"go.opencensus.io/trace"
)

func WithCache(c CachedGetter) CanonicalHistoryOption {
	return func(h *CanonicalHistory) {
		h.cache = c
	}
}

type CanonicalHistoryOption func(*CanonicalHistory)

func NewCanonicalHistory(h HistoryAccessor, cc CanonicalChecker, cs CurrentSlotter, opts ...CanonicalHistoryOption) *CanonicalHistory {
	ch := &CanonicalHistory{
		h:  h,
		cc: cc,
		cs: cs,
	}
	for _, o := range opts {
		o(ch)
	}
	return ch
}

type CanonicalHistory struct {
	h     HistoryAccessor
	cc    CanonicalChecker
	cs    CurrentSlotter
	cache CachedGetter
}

// Block retrieves a block by root for the streamed replay path.
func (c *CanonicalHistory) Block(ctx context.Context, blockRoot [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	return c.h.Block(ctx, blockRoot)
}

func (c *CanonicalHistory) ReplayerForSlot(target primitives.Slot) Replayer {
	return &stateReplayer{chainer: c, method: forSlot, target: target}
}

func (c *CanonicalHistory) BlockRootForSlot(ctx context.Context, target primitives.Slot) ([32]byte, error) {
	if currentSlot := c.cs.CurrentSlot(); target > currentSlot {
		return [32]byte{}, errors.Wrap(ErrFutureSlotRequested, fmt.Sprintf("requested=%d, current=%d", target, currentSlot))
	}
	covered, err := c.slotCovered(ctx, target)
	if err != nil {
		return [32]byte{}, err
	}
	if !covered {
		// Without this check the search below finds no canonical block, falls
		// through to genesis, and the replay fabricates a state from the genesis
		// state and empty slots.
		return [32]byte{}, errors.Wrapf(ErrNoDataForSlot, "slot %d not in db due to checkpoint sync", target)
	}

	slotAbove := target + 1
	// don't bother searching for candidate roots when we know the target slot is genesis
	for slotAbove > 1 {
		if ctx.Err() != nil {
			return [32]byte{}, errors.Wrap(ctx.Err(), "context canceled during canonicalBlockForSlot")
		}
		slot, roots, err := c.h.HighestRootsBelowSlot(ctx, slotAbove)
		if err != nil {
			return [32]byte{}, errors.Wrapf(err, "error finding highest block w/ slot < %d", slotAbove)
		}
		if len(roots) == 0 {
			return [32]byte{}, errors.Wrap(ErrNoBlocksBelowSlot, fmt.Sprintf("slot=%d", slotAbove))
		}
		r, err := c.bestForSlot(ctx, roots)
		if err == nil {
			// we found a valid, canonical block!
			return r, nil
		}

		// we found a block, but it wasn't considered canonical - keep looking
		if errors.Is(err, ErrNoCanonicalBlockForSlot) {
			// break once we've seen slot 0 (and prevent underflow)
			if slot == params.BeaconConfig().GenesisSlot {
				break
			}
			slotAbove = slot
			continue
		}
		return [32]byte{}, err
	}

	return c.h.GenesisBlockRoot(ctx)
}

// slotCovered reports whether the node holds canonical history for the slot. A
// node started from a checkpoint has no blocks between the backfill position
// and the origin block, so no state can be replayed for a slot in that gap.
// This mirrors backfill.Status.SlotCovered using the persisted roots. Note for a
// future backfill implementation: blocks it stores must also be canonical for
// the CanonicalChecker (the finalized index) before slots at or below the
// backfill position may be served from them.
func (c *CanonicalHistory) slotCovered(ctx context.Context, slot primitives.Slot) (bool, error) {
	originRoot, err := c.h.OriginCheckpointBlockRoot(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNotFoundOriginBlockRoot) {
			return true, nil // synced from genesis
		}
		return false, errors.Wrap(err, "could not read the origin checkpoint root")
	}
	originSlot, err := c.slotOfBlock(ctx, originRoot)
	if err != nil {
		return false, errors.Wrap(err, "could not read the origin checkpoint block")
	}
	if slot >= originSlot {
		return true, nil
	}
	backfillRoot, err := c.h.BackfillBlockRoot(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNotFoundBackfillBlockRoot) {
			return false, nil // nothing backfilled yet
		}
		return false, errors.Wrap(err, "could not read the backfill block root")
	}
	backfillSlot, err := c.slotOfBlock(ctx, backfillRoot)
	if err != nil {
		return false, errors.Wrap(err, "could not read the backfill block")
	}
	return slot <= backfillSlot, nil
}

func (c *CanonicalHistory) slotOfBlock(ctx context.Context, root [32]byte) (primitives.Slot, error) {
	b, err := c.h.Block(ctx, root)
	if err != nil {
		return 0, err
	}
	if err := blocks.BeaconBlockIsNil(b); err != nil {
		return 0, errors.Wrapf(db.ErrNotFound, "block %#x", root)
	}
	return b.Block().Slot(), nil
}

// bestForSlot encapsulates several messy realities of the underlying db code, looping through multiple blocks,
// performing null/validity checks, and using CanonicalChecker to only pick canonical blocks.
func (c *CanonicalHistory) bestForSlot(ctx context.Context, roots [][32]byte) ([32]byte, error) {
	for _, root := range roots {
		canon, err := c.cc.IsCanonical(ctx, root)
		if err != nil {
			return [32]byte{}, errors.Wrap(err, "replayer could not check if block is canonical")
		}
		if canon {
			return root, nil
		}
	}
	return [32]byte{}, errors.Wrap(ErrNoCanonicalBlockForSlot, "no good block for slot")
}

// ChainForSlot creates a value that satisfies the Replayer interface via db queries
// and the stategen transition helper methods. This implementation uses the following algorithm:
// - find the highest canonical block <= the target slot
// - starting with this block, recursively search backwards for a stored state, and retain intervening block roots
func (c *CanonicalHistory) chainForSlot(ctx context.Context, target primitives.Slot) (state.BeaconState, [][32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "canonicalChainer.chainForSlot")
	defer span.End()
	r, err := c.BlockRootForSlot(ctx, target)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "no canonical block root found below slot=%d", target)
	}
	s, descendants, err := c.ancestorChain(ctx, r, target)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to query for ancestor and descendant block roots")
	}

	return s, descendants, nil
}

func (c *CanonicalHistory) getState(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error) {
	if c.cache != nil {
		st, err := c.cache.ByBlockRoot(blockRoot)
		if err == nil {
			return st, nil
		}
		if !errors.Is(err, ErrNotInCache) {
			return nil, errors.Wrap(err, "error reading from state cache during state replay")
		}
	}
	return c.h.StateOrError(ctx, blockRoot)
}

// ancestorChain works backwards through the chain lineage, accumulating block roots and checking for a saved state.
// If it finds a saved state that the tail block was descended from, it returns this state and
// all roots in the lineage, including the tail block. Roots are returned in ascending order.
// Note that this function assumes that the tail is a canonical block, and therefore assumes that
// all ancestors are also canonical. A saved state at a later slot than its block (the block's
// post-state advanced through empty slots, which is how a checkpoint state may have been
// provided) is used as long as it does not lie beyond the replay target.
func (c *CanonicalHistory) ancestorChain(ctx context.Context, tailRoot [32]byte, target primitives.Slot) (state.BeaconState, [][32]byte, error) {
	ctx, span := trace.StartSpan(ctx, "canonicalChainer.ancestorChain")
	defer span.End()
	chain := make([][32]byte, 0)
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, errors.Wrap(err, "context canceled while finding ancestor block roots")
		}
		tail, err := c.h.Block(ctx, tailRoot)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "db error when retrieving block by root=%#x", tailRoot)
		}
		if err := blocks.BeaconBlockIsNil(tail); err != nil {
			return nil, nil, errors.Wrapf(db.ErrNotFound, "unable to retrieve block by root=%#x", tailRoot)
		}
		b := tail.Block()
		st, err := c.getState(ctx, tailRoot)
		// err == nil, we've got a real state - the job is done!
		// Note: in cases where there are skipped slots we could find a state that is a descendant
		// of the block we are searching for. Such a state is only usable when it does not lie
		// beyond the replay target; otherwise we keep working backwards.
		if err == nil && st.Slot() >= b.Slot() && st.Slot() <= target {
			// we found the state by the root of the head, meaning it has already been applied.
			// we only want to return the roots descended from it.
			reverseBlockRoots(chain)
			return st, chain, nil
		}
		// ErrNotFoundState errors are fine, but other errors mean something is wrong with the db
		if err != nil && !errors.Is(err, db.ErrNotFoundState) {
			return nil, nil, errors.Wrapf(err, "error querying database for state w/ block root = %#x", tailRoot)
		}
		chain = append(chain, tailRoot)
		tailRoot = b.ParentRoot()
	}
}
