package stategen

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"go.opencensus.io/trace"
)

// MigrateToCold advances the finalized info in between the cold and hot state sections.
// It moves the recent finalized states from the hot section to the cold section and
// only preserves the ones that are on archived point.
func (s *State) MigrateToCold(ctx context.Context, fRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "stateGen.MigrateToCold")
	defer span.End()

	// When migrating states we choose to acquire the migration lock before
	// proceeding. This is to prevent multiple migration routines from overwriting each
	// other.
	s.migrationLock.Lock()
	defer s.migrationLock.Unlock()

	s.finalizedInfo.lock.RLock()
	oldFSlot := s.finalizedInfo.slot
	s.finalizedInfo.lock.RUnlock()

	fBlock, err := s.beaconDB.Block(ctx, fRoot)
	if err != nil {
		return err
	}
	if err := blocks.BeaconBlockIsNil(fBlock); err != nil {
		return errors.Wrapf(err, "finalized block %#x not found", fRoot)
	}
	fSlot := fBlock.Block().Slot()
	if oldFSlot > fSlot {
		return nil
	}

	// Start at previous finalized slot, stop at current finalized slot (it will be handled in the next migration).
	// If the slot is on archived point, save the state of that slot to the DB.
	for slot := oldFSlot; slot < fSlot; slot++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if slot%s.slotsPerArchivedPoint == 0 && slot != 0 {
			cached, exists, err := s.epochBoundaryStateCache.getBySlot(slot)
			if err != nil {
				return fmt.Errorf("could not get epoch boundary state for slot %d", slot)
			}
			// The cache is populated for every epoch boundary block that gets processed and is
			// only ever evicted by size or when a block turns out to be invalid, never when a
			// block loses fork choice. Its slot index is also first-write-wins, so a reorged
			// sibling processed ahead of the canonical block at the same slot keeps the slot
			// key. Either way the cached root can be non-canonical, so it has to be checked
			// before its state is archived.
			if exists && !s.beaconDB.IsFinalizedBlock(ctx, cached.root) {
				log.WithFields(logrus.Fields{
					"slot": slot,
					"root": fmt.Sprintf("%#x", cached.root),
				}).Debug("Ignoring non-canonical epoch boundary state while migrating to cold")
				exists = false
			}

			var aRoot [32]byte
			var aState state.BeaconState

			// When the epoch boundary state is not in cache (skipped slot, cache
			// eviction, or a non-canonical entry holding the slot key), regenerate
			// the state which represents the epoch boundary from the highest
			// canonical block at or below the boundary slot. The slot itself must be
			// considered: after a restart the boundary cache is empty, and when the
			// boundary coincides with the finalized slot the migration resumed from
			// (the floor below), the finalized block is the block to archive. Looking
			// only strictly below it trips the floor check and, because the finalized
			// info is then never advanced, stalls every later migration.
			if exists {
				aRoot = cached.root
				aState = cached.state
			} else {
				aRoot, err = s.canonicalRootAtOrBelow(ctx, slot, oldFSlot)
				if err != nil {
					return err
				}
				// There's no need to generate the state if the state already exists in the DB.
				// We can skip saving the state.
				if !s.beaconDB.HasState(ctx, aRoot) {
					aState, err = s.StateByRoot(ctx, aRoot)
					if err != nil {
						return err
					}
				}
			}

			// The existence check and the removal from the hot-state list form one
			// critical section with DisableSaveHotStateToDB, which deletes the
			// listed states under the same lock. Checked outside it, the state
			// could be deleted between the check and the removal: the archived
			// point would be skipped here while its state is gone.
			s.saveHotStateDB.lock.Lock()
			if s.beaconDB.HasState(ctx, aRoot) {
				// If you are migrating a state and its already part of the hot state cache saved to the db,
				// you can just remove it from the hot state cache as it becomes redundant.
				// Remove every entry for the root: a leftover one would have the
				// hot-state cleanup delete the archived state later.
				roots := s.saveHotStateDB.blockRootsOfSavedStates
				kept := roots[:0]
				for _, r := range roots {
					if r != aRoot {
						kept = append(kept, r)
					}
				}
				s.saveHotStateDB.blockRootsOfSavedStates = kept
				s.saveHotStateDB.lock.Unlock()
				continue
			}
			s.saveHotStateDB.lock.Unlock()

			// aState is only generated when the first HasState check above missed. A
			// concurrent DeleteStates (e.g. DisableSaveHotStateToDB on the block-import
			// path) can remove aRoot's state between that check and the HasState check
			// just above, so we reach here with aState still nil. Regenerate it rather
			// than passing a nil state to SaveState, which would panic this background
			// migration goroutine (it has no panic recovery).
			if aState == nil || aState.IsNil() {
				aState, err = s.StateByRoot(ctx, aRoot)
				if err != nil {
					return err
				}
			}
			if err := s.beaconDB.SaveState(ctx, aState, aRoot); err != nil {
				return err
			}
			log.WithFields(
				logrus.Fields{
					"slot": aState.Slot(),
					"root": hex.EncodeToString(bytesutil.Trunc(aRoot[:])),
				}).Info("Saved state in DB")
		}
	}

	// Every archived point below the finalized slot is on disk now. Record the
	// cursor so a restart resumes here rather than at whatever archived state
	// happens to be the highest one, which can hide an unfinished range below it.
	if err := s.beaconDB.SaveStateMigrationCursor(ctx, fSlot); err != nil {
		return errors.Wrap(err, "could not save the state migration cursor")
	}

	// Update finalized info in memory. The slot is the migration cursor and
	// advances whatever the caches hold: left behind, the next migration would
	// redo the whole range, and an older queued migration (above the stale
	// cursor) would run and record its lower slot. The finalized root and state
	// are replaced together, when the state is at hand.
	var fState state.BeaconState
	fInfo, ok, err := s.epochBoundaryStateCache.getByBlockRoot(fRoot)
	if err != nil {
		return err
	}
	if ok {
		fState = fInfo.state
	} else if cached := s.hotStateCache.getWithoutCopy(fRoot); cached != nil && !cached.IsNil() {
		fState = cached
	}
	s.advanceFinalizedInfo(fSlot, fRoot, fState)

	return nil
}

// canonicalRootAtOrBelow returns the root of the highest canonical block at or below the given slot,
// skipping slots that hold only blocks which lost fork choice. Those are never deleted from the slot index.
// Canonicality comes from the finalized index, which is decisive below the finalized checkpoint slot.
// floor is the migration cursor, a slot whose archive is known to exist. A canonical block below the
// floor is a valid answer only when its state is on disk (the archive of a skipped boundary slot is the
// state of the highest canonical block below that slot); otherwise resolving past the floor means the
// finalized index is missing entries, and that is an error rather than a walk to genesis.
func (s *State) canonicalRootAtOrBelow(ctx context.Context, slot, floor primitives.Slot) ([32]byte, error) {
	// HighestRootsBelowSlot reports a strictly lower slot, so next decreases every round.
	for next := slot + 1; ; {
		high, roots, err := s.beaconDB.HighestRootsBelowSlot(ctx, next)
		if err != nil {
			return [32]byte{}, err
		}
		canonical := make([][32]byte, 0, 1)
		for _, r := range roots {
			if s.beaconDB.IsFinalizedBlock(ctx, r) {
				canonical = append(canonical, r)
			}
		}
		switch len(canonical) {
		case 1:
			if high < floor && !s.beaconDB.HasState(ctx, canonical[0]) {
				return [32]byte{}, errUnknownBlock
			}
			return canonical[0], nil
		case 0:
			if high == 0 {
				return [32]byte{}, errUnknownBlock
			}
			next = high
		default:
			return [32]byte{}, errUnknownBlock
		}
	}
}
