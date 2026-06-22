package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/async"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
)

// getRecentPreState returns a pre-state usable for processing attestations in
// the head or the immediately following epoch without going through the
// state-regeneration path. Returns nil if the checkpoint is not eligible for
// the fast path; the caller must then fall back to the regen path.
func (s *Service) getRecentPreState(ctx context.Context, c *qrysmpb.Checkpoint) state.ReadOnlyBeaconState {
	headEpoch := slots.ToEpoch(s.HeadSlot())
	if c.Epoch < headEpoch {
		return nil
	}
	if !s.cfg.ForkChoiceStore.IsCanonical([32]byte(c.Root)) {
		return nil
	}
	// Only use head state if the head's chain has the same target root for
	// c.Epoch as the checkpoint being processed. Without this check, on a
	// reorg boundary the head may be canonical but its target root for the
	// checkpoint epoch can differ from c.Root, which would yield the wrong
	// shuffling for this attestation.
	headRoot, err := s.HeadRoot(ctx)
	if err != nil {
		return nil
	}
	headTarget, err := s.cfg.ForkChoiceStore.TargetRootForEpoch([32]byte(headRoot), c.Epoch)
	if err != nil {
		return nil
	}
	if !bytes.Equal(c.Root, headTarget[:]) {
		return nil
	}
	if c.Epoch == headEpoch {
		// The TargetRootForEpoch check above already guarantees the head's
		// target for this epoch matches c.Root, so the head state's
		// shuffling is valid regardless of how old c.Root's slot is. The
		// previous targetSlot+1 < headEpoch bound rejected canonical
		// attestations with older target blocks (sparse chains) and forced
		// the slow regen path unnecessarily.
		st, err := s.HeadStateReadOnly(ctx)
		if err != nil {
			return nil
		}
		return st
	}
	// c.Epoch > headEpoch: a late-imported / next-epoch attestation. Avoid the
	// full regen by advancing the head state using the next-slot cache.
	slot, err := slots.EpochStart(c.Epoch)
	if err != nil {
		return nil
	}
	epochKey := strconv.FormatUint(uint64(c.Epoch), 10 /* base 10 */)
	lock := async.NewMultilock(string(c.Root) + epochKey)
	lock.Lock()
	defer lock.Unlock()
	cachedState, err := s.checkpointStateCache.StateByCheckpoint(c)
	if err != nil {
		return nil
	}
	if cachedState != nil && !cachedState.IsNil() {
		return cachedState
	}
	st, err := s.HeadState(ctx)
	if err != nil {
		return nil
	}
	st, err = transition.ProcessSlotsUsingNextSlotCache(ctx, st, c.Root, slot)
	if err != nil {
		return nil
	}
	if err := s.checkpointStateCache.AddCheckpointState(c, st); err != nil {
		// A cache-add failure doesn't invalidate the state we just computed;
		// log and return it so the caller doesn't fall back to the slow regen
		// path. The next call will simply recompute / re-cache.
		log.WithError(err).Warn("could not save checkpoint state to cache")
	}
	return st
}

// getAttPreState retrieves the att pre state by either from the cache or the DB.
func (s *Service) getAttPreState(ctx context.Context, c *qrysmpb.Checkpoint) (state.ReadOnlyBeaconState, error) {
	// If the attestation is recent and canonical we can use the head state to compute the shuffling.
	if st := s.getRecentPreState(ctx, c); st != nil {
		return st, nil
	}
	// Use a multilock to allow scoped holding of a mutex by a checkpoint root + epoch
	// allowing us to behave smarter in terms of how this function is used concurrently.
	epochKey := strconv.FormatUint(uint64(c.Epoch), 10 /* base 10 */)
	lock := async.NewMultilock(string(c.Root) + epochKey)
	lock.Lock()
	defer lock.Unlock()
	cachedState, err := s.checkpointStateCache.StateByCheckpoint(c)
	if err != nil {
		return nil, errors.Wrap(err, "could not get cached checkpoint state")
	}
	if cachedState != nil && !cachedState.IsNil() {
		return cachedState, nil
	}
	// Try the next slot cache for the early epoch calls, this should mostly have been covered already
	// but is cheap
	slot, err := slots.EpochStart(c.Epoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute epoch start")
	}
	cachedState = transition.NextSlotState(c.Root, slot)
	if cachedState != nil && !cachedState.IsNil() {
		if cachedState.Slot() != slot {
			cachedState, err = transition.ProcessSlots(ctx, cachedState, slot)
			if err != nil {
				return nil, errors.Wrap(err, "could not process slots")
			}
		}
		if err := s.checkpointStateCache.AddCheckpointState(c, cachedState); err != nil {
			return nil, errors.Wrap(err, "could not save checkpoint state to cache")
		}
		return cachedState, nil
	}

	// Do not process attestations for old non viable checkpoints otherwise
	ok, err := s.cfg.ForkChoiceStore.IsViableForCheckpoint(&forkchoicetypes.Checkpoint{Root: [32]byte(c.Root), Epoch: c.Epoch})
	if err != nil {
		return nil, errors.Wrap(err, "could not check checkpoint condition in forkchoice")
	}
	if !ok {
		return nil, errors.Wrap(ErrNotCheckpoint, fmt.Sprintf("epoch %d root %#x", c.Epoch, c.Root))
	}

	// Fallback to state regeneration.
	baseState, err := s.cfg.StateGen.StateByRoot(ctx, bytesutil.ToBytes32(c.Root))
	if err != nil {
		return nil, errors.Wrapf(err, "could not get pre state for epoch %d", c.Epoch)
	}

	epochStartSlot, err := slots.EpochStart(c.Epoch)
	if err != nil {
		return nil, err
	}
	baseState, err = transition.ProcessSlotsIfPossible(ctx, baseState, epochStartSlot)
	if err != nil {
		return nil, errors.Wrapf(err, "could not process slots up to epoch %d", c.Epoch)
	}

	// Sharing the same state across caches is perfectly fine here, the fetching
	// of attestation prestate is by far the most accessed state fetching pattern in
	// the beacon node. An extra state instance cached isn't an issue in the bigger
	// picture.
	if err := s.checkpointStateCache.AddCheckpointState(c, baseState); err != nil {
		return nil, errors.Wrap(err, "could not save checkpoint state to cache")
	}
	return baseState, nil
}

// verifyAttTargetEpoch validates attestation is from the current or previous epoch.
func verifyAttTargetEpoch(_ context.Context, genesisTime, nowTime uint64, c *qrysmpb.Checkpoint) error {
	currentSlot := primitives.Slot((nowTime - genesisTime) / params.BeaconConfig().SecondsPerSlot)
	currentEpoch := slots.ToEpoch(currentSlot)
	var prevEpoch primitives.Epoch
	// Prevents previous epoch under flow
	if currentEpoch > 1 {
		prevEpoch = currentEpoch - 1
	}
	if c.Epoch != prevEpoch && c.Epoch != currentEpoch {
		return fmt.Errorf("target epoch %d does not match current epoch %d or prev epoch %d", c.Epoch, currentEpoch, prevEpoch)
	}
	return nil
}

// verifyBeaconBlock verifies beacon head block is known and not from the future.
func (s *Service) verifyBeaconBlock(ctx context.Context, data *qrysmpb.AttestationData) error {
	r := bytesutil.ToBytes32(data.BeaconBlockRoot)
	b, err := s.getBlock(ctx, r)
	if err != nil {
		return err
	}
	if err := blocks.BeaconBlockIsNil(b); err != nil {
		return err
	}
	if b.Block().Slot() > data.Slot {
		return fmt.Errorf("could not process attestation for future block, block.Slot=%d > attestation.Data.Slot=%d", b.Block().Slot(), data.Slot)
	}
	return nil
}
