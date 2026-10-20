package stategen

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
)

// SaveState saves the state in the cache and/or DB.
func (s *State) SaveState(ctx context.Context, blockRoot [32]byte, st state.BeaconState) error {
	ctx, span := trace.StartSpan(ctx, "stateGen.SaveState")
	defer span.End()

	return s.saveStateByRoot(ctx, blockRoot, st)
}

// ForceCheckpoint initiates a cold state save of the given block root's state. This method does not update the
// "last archived state" but simply saves the specified state from the root argument into the DB.
//
// The name "Checkpoint" isn't referring to checkpoint in the sense of our consensus type, but checkpoint for our historical states.
func (s *State) ForceCheckpoint(ctx context.Context, blockRoot []byte) error {
	ctx, span := trace.StartSpan(ctx, "stateGen.ForceCheckpoint")
	defer span.End()

	root32 := bytesutil.ToBytes32(blockRoot)
	// Before the first finalized checkpoint, the finalized root is zero hash.
	// Return early if there hasn't been a finalized checkpoint.
	if root32 == params.BeaconConfig().ZeroHash {
		return nil
	}

	fs, err := s.loadStateByRoot(ctx, root32)
	if err != nil {
		return err
	}

	return s.beaconDB.SaveState(ctx, fs, root32)
}

// This saves a post beacon state. On the epoch boundary,
// it saves a full state. On an intermediate slot, it saves a back pointer to the
// nearest epoch boundary state.
func (s *State) saveStateByRoot(ctx context.Context, blockRoot [32]byte, st state.BeaconState) error {
	ctx, span := trace.StartSpan(ctx, "stateGen.saveStateByRoot")
	defer span.End()

	// Duration can't be 0 to prevent panic for division.
	duration := uint64(math.Max(float64(s.saveHotStateDB.duration), 1))

	s.saveHotStateDB.lock.Lock()
	if s.saveHotStateDB.enabled && st.Slot().Mod(duration) == 0 {
		if err := s.beaconDB.SaveState(ctx, st, blockRoot); err != nil {
			s.saveHotStateDB.lock.Unlock()
			return err
		}
		// The same root can be saved more than once (a batch saves its boundary
		// states and its last state, which may coincide). The list must hold it
		// once: the migration removes a root it archived, and a leftover entry
		// would have the hot-state cleanup delete the archived state.
		if !slices.Contains(s.saveHotStateDB.blockRootsOfSavedStates, blockRoot) {
			s.saveHotStateDB.blockRootsOfSavedStates = append(s.saveHotStateDB.blockRootsOfSavedStates, blockRoot)
		}

		log.WithFields(logrus.Fields{
			"slot":                   st.Slot(),
			"totalHotStateSavedInDB": len(s.saveHotStateDB.blockRootsOfSavedStates),
		}).Info("Saving hot state to DB")
	}
	s.saveHotStateDB.lock.Unlock()

	// If the hot state is already in cache, one can be sure the state was processed and in the DB.
	if s.hotStateCache.has(blockRoot) {
		return nil
	}

	// Only on an epoch boundary slot, save epoch boundary state in epoch boundary root state cache.
	if slots.IsEpochStart(st.Slot()) {
		if err := s.epochBoundaryStateCache.put(blockRoot, st); err != nil {
			return err
		}
	} else {
		// Always check that the correct epoch boundary states have been saved
		// for the current epoch.
		epochStart, err := slots.EpochStart(slots.ToEpoch(st.Slot()))
		if err != nil {
			return err
		}
		bRoot, err := helpers.BlockRootAtSlot(st, epochStart)
		if err != nil {
			return err
		}
		_, ok, err := s.epochBoundaryStateCache.getByBlockRoot([32]byte(bRoot))
		if err != nil {
			return err
		}

		// We would only recover the boundary states under this condition:
		//
		// 1) Would indicate that the epoch boundary was skipped due to a missed slot, we
		// then recover by saving the state at that particular slot here.
		if !ok {
			// Only recover the state if it is in our hot state cache, otherwise we
			// simply skip this step. A single lookup: a concurrent put can evict
			// the entry between a has() and a get(), and the nil state would then
			// be handed to the boundary cache and dereferenced.
			if hState := s.hotStateCache.get([32]byte(bRoot)); hState != nil {
				log.WithFields(logrus.Fields{
					"slot": epochStart,
					"root": fmt.Sprintf("%#x", bRoot),
				}).Debug("Recovering state for epoch boundary cache")

				if err := s.epochBoundaryStateCache.put([32]byte(bRoot), hState); err != nil {
					return err
				}
			}
		}
	}

	// On an intermediate slot, save state summary.
	if err := s.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{
		Slot: st.Slot(),
		Root: blockRoot[:],
	}); err != nil {
		return err
	}

	// Store the copied state in the hot state cache.
	s.hotStateCache.put(blockRoot, st)

	return nil
}

// EnableSaveHotStateToDB enters the mode that saves hot beacon state to the DB.
// This usually gets triggered when there's long duration since finality.
func (s *State) EnableSaveHotStateToDB(_ context.Context) {
	s.saveHotStateDB.lock.Lock()
	defer s.saveHotStateDB.lock.Unlock()
	if s.saveHotStateDB.enabled {
		return
	}

	s.saveHotStateDB.enabled = true

	log.WithFields(logrus.Fields{
		"enabled":       s.saveHotStateDB.enabled,
		"slotsInterval": s.saveHotStateDB.duration,
	}).Warn("Entering mode to save hot states in DB")
}

// DisableSaveHotStateToDB exits the mode that saves beacon state to DB for the hot states.
// This usually gets triggered once there's finality after long duration since finality.
func (s *State) DisableSaveHotStateToDB(ctx context.Context) error {
	s.saveHotStateDB.lock.Lock()
	defer s.saveHotStateDB.lock.Unlock()
	if !s.saveHotStateDB.enabled && len(s.saveHotStateDB.blockRootsOfSavedStates) == 0 {
		return nil
	}

	if s.saveHotStateDB.enabled {
		log.WithFields(logrus.Fields{
			"enabled":          s.saveHotStateDB.enabled,
			"deletedHotStates": len(s.saveHotStateDB.blockRootsOfSavedStates),
		}).Warn("Exiting mode to save hot states in DB")
		s.saveHotStateDB.enabled = false
	}

	// Delete the previously saved states one by one. The database refuses to
	// delete the state of the current justified or finalized checkpoint root,
	// which a saved hot state can well be: keep such a root, and any root whose
	// deletion failed, so a later call removes it once finality has moved on,
	// instead of abandoning the rest of the list.
	kept := make([][32]byte, 0)
	var firstErr error
	for i, r := range s.saveHotStateDB.blockRootsOfSavedStates {
		if err := ctx.Err(); err != nil {
			kept = append(kept, s.saveHotStateDB.blockRootsOfSavedStates[i:]...)
			if firstErr == nil {
				firstErr = err
			}
			break
		}
		err := s.beaconDB.DeleteState(ctx, r)
		if err == nil {
			continue
		}
		kept = append(kept, r)
		if errors.Is(err, db.ErrDeleteJustifiedAndFinalized) {
			continue
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	s.saveHotStateDB.blockRootsOfSavedStates = kept

	return firstErr
}
