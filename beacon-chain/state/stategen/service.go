// Package stategen defines functions to regenerate beacon chain states
// by replaying blocks from a stored state checkpoint, useful for
// optimization and reducing a beacon node's resource consumption.
package stategen

import (
	"context"
	stderrors "errors"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/sync/backfill"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"go.opencensus.io/trace"
)

var defaultHotStateDBInterval primitives.Slot = 128

var populatePubkeyCacheOnce sync.Once

// StateManager represents a management object that handles the internal
// logic of maintaining both hot and cold states in DB.
type StateManager interface {
	Resume(ctx context.Context, fState state.BeaconState) (state.BeaconState, error)
	DisableSaveHotStateToDB(ctx context.Context) error
	EnableSaveHotStateToDB(_ context.Context)
	HasState(ctx context.Context, blockRoot [32]byte) (bool, error)
	DeleteStateFromCaches(ctx context.Context, blockRoot [32]byte) error
	ForceCheckpoint(ctx context.Context, root []byte) error
	SaveState(ctx context.Context, blockRoot [32]byte, st state.BeaconState) error
	SaveFinalizedState(fSlot primitives.Slot, fRoot [32]byte, fState state.BeaconState)
	MigrateToCold(ctx context.Context, fRoot [32]byte) error
	StateByRoot(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error)
	BalancesByCheckpoint(context.Context, *forkchoicetypes.Checkpoint) (*forkchoicetypes.JustifiedBalances, error)
	StateByRootIfCached(blockRoot [32]byte) state.BeaconState
	StateByRootIfCachedNoCopy(blockRoot [32]byte) state.BeaconState
	StateByRootInitialSync(ctx context.Context, blockRoot [32]byte) (state.BeaconState, error)
}

// State is a concrete implementation of StateManager.
type State struct {
	beaconDB                db.NoHeadAccessDatabase
	slotsPerArchivedPoint   primitives.Slot
	hotStateCache           *hotStateCache
	finalizedInfo           *finalizedInfo
	epochBoundaryStateCache *epochBoundaryState
	saveHotStateDB          *saveHotStateDbConfig
	backfillStatus          *backfill.Status
	migrationLock           *sync.Mutex
	fc                      forkchoice.ForkChoicer
}

// This tracks the config in the event of long non-finality,
// how often does the node save hot states to db? what are
// the saved hot states in db?... etc
type saveHotStateDbConfig struct {
	enabled                 bool
	lock                    sync.Mutex
	duration                primitives.Slot
	blockRootsOfSavedStates [][32]byte
}

// This tracks the finalized point. It's also the point where slot and the block root of
// cold and hot sections of the DB splits: slot is the cold-state migration cursor,
// MigrateToCold archives the points from it up to the next finalized slot.
type finalizedInfo struct {
	slot  primitives.Slot
	root  [32]byte
	state state.BeaconState
	lock  sync.RWMutex
}

// StateGenOption is a functional option for controlling the initialization of a *State value
type StateGenOption func(*State)

func WithBackfillStatus(bfs *backfill.Status) StateGenOption {
	return func(sg *State) {
		sg.backfillStatus = bfs
	}
}

// New returns a new state management object.
func New(beaconDB db.NoHeadAccessDatabase, fc forkchoice.ForkChoicer, opts ...StateGenOption) *State {
	s := &State{
		beaconDB:                beaconDB,
		hotStateCache:           newHotStateCache(),
		finalizedInfo:           &finalizedInfo{slot: 0, root: params.BeaconConfig().ZeroHash},
		slotsPerArchivedPoint:   params.BeaconConfig().SlotsPerArchivedPoint,
		epochBoundaryStateCache: newBoundaryStateCache(),
		saveHotStateDB: &saveHotStateDbConfig{
			duration: defaultHotStateDBInterval,
		},
		migrationLock: new(sync.Mutex),
		fc:            fc,
	}
	for _, o := range opts {
		o(s)
	}
	fc.Lock()
	defer fc.Unlock()
	fc.SetBalancesByRooter(s.BalancesByCheckpoint)
	return s
}

// Resume resumes a new state management object from previously saved finalized checkpoint in DB.
func (s *State) Resume(ctx context.Context, fState state.BeaconState) (state.BeaconState, error) {
	ctx, span := trace.StartSpan(ctx, "stateGen.Resume")
	defer span.End()

	c, err := s.beaconDB.FinalizedCheckpoint(ctx)
	if err != nil {
		return nil, err
	}
	fRoot := bytesutil.ToBytes32(c.Root)
	// Resume as genesis state if last finalized root is zero hashes.
	if fRoot == params.BeaconConfig().ZeroHash {
		st, err := s.beaconDB.GenesisState(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "could not get genesis state")
		}
		// Save genesis state in the hot state cache.
		gbr, err := s.beaconDB.GenesisBlockRoot(ctx)
		if err != nil {
			return nil, stderrors.Join(ErrNoGenesisBlock, err)
		}
		return st, s.SaveState(ctx, gbr, st)
	}

	if fState == nil || fState.IsNil() {
		return nil, errors.New("finalized state is nil")
	}

	go func() {
		if err := s.beaconDB.CleanUpDirtyStates(ctx, s.slotsPerArchivedPoint); err != nil {
			log.WithError(err).Error("Could not clean up dirty states")
		}
	}()

	// The migration cursor lives only in memory and MigrateToCold runs in the
	// background, so a shutdown or crash can lose the archived points between
	// the last completed migration and the finalized slot restarted from. Resume
	// the cursor from the last archived point that is actually in the DB; the
	// migration skips points whose state already exists, so this is cheap.
	cursor, err := s.migrationCursor(ctx, fState.Slot())
	if err != nil {
		return nil, errors.Wrap(err, "could not find the migration cursor")
	}
	if cursor < fState.Slot() {
		log.WithField("finalizedSlot", fState.Slot()).WithField("resumeFrom", cursor).
			Info("Resuming cold state migration from the last archived point")
	}
	// Update the fields under the lock rather than replacing the struct: other
	// services may already be serving requests that read the finalized info.
	s.finalizedInfo.lock.Lock()
	s.finalizedInfo.slot = cursor
	s.finalizedInfo.root = fRoot
	s.finalizedInfo.state = fState.Copy()
	s.finalizedInfo.lock.Unlock()

	// Pre-populate the pubkey cache with the validator public keys from the finalized state.
	// This process takes about 30 seconds on mainnet with 450,000 validators.
	go populatePubkeyCacheOnce.Do(func() {
		log.Debug("Populating pubkey cache")
		start := time.Now()
		if err := fState.ReadFromEveryValidator(func(_ int, val state.ReadOnlyValidator) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			pub := val.PublicKey()
			_, err := ml_dsa_87.PublicKeyFromBytes(pub[:])
			return err
		}); err != nil {
			log.WithError(err).Error("Failed to populate pubkey cache")
		}
		log.WithField("duration", time.Since(start)).Debug("Done populating pubkey cache")
	})

	return fState, nil
}

// migrationCursor returns the slot the cold state migration resumes from: the
// last archived point whose archive is on disk before the first missing one
// (so a gap is filled rather than hidden and the first migration step can be
// reconstructed from the archive at the cursor), capped by the cursor recorded
// by the last completed migration when there is one at or below the finalized
// slot. The archived states on disk are the ground truth: an archive can be
// lost below the recorded cursor (an older release's cleanup removed the
// archive of a skipped archived slot), and the scan is cheap, one lookup per
// archived point.
func (s *State) migrationCursor(ctx context.Context, finalizedSlot primitives.Slot) (primitives.Slot, error) {
	scanned, err := s.lastReconstructableArchivedPoint(ctx, finalizedSlot)
	if err != nil {
		return 0, err
	}
	cursor, found, err := s.beaconDB.StateMigrationCursor(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "could not read the state migration cursor")
	}
	if found && cursor <= finalizedSlot && cursor < scanned {
		return cursor, nil
	}
	if found && cursor <= finalizedSlot && scanned < cursor {
		log.WithField("recorded", cursor).WithField("resumeFrom", scanned).
			Warn("An archived state below the recorded migration cursor is missing; resuming below it to rebuild it")
	}
	return scanned, nil
}

// lastReconstructableArchivedPoint returns the highest archived point at or
// below slot such that every archived point from the lower bound up to it has
// its archive on disk (the archive of a point is the state of the highest
// canonical block at or below it). The lower bound is the checkpoint origin
// slot on a checkpoint-synced node, since no point below the origin can be
// migrated, and genesis otherwise; it is returned when no point qualifies.
func (s *State) lastReconstructableArchivedPoint(ctx context.Context, slot primitives.Slot) (primitives.Slot, error) {
	interval := s.slotsPerArchivedPoint
	if interval == 0 {
		return slot, nil
	}
	lower, err := s.migrationLowerBound(ctx)
	if err != nil {
		return 0, err
	}
	cursor := lower
	// Point 0 is genesis, which the migration never archives.
	first := lower
	if rem := lower % interval; rem != 0 {
		first = lower - rem + interval
	}
	if first == 0 {
		first = interval
	}
	for point := first; point <= slot; point += interval {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		root, err := s.canonicalRootAtOrBelow(ctx, point, lower)
		if err != nil || !s.beaconDB.HasState(ctx, root) {
			log.WithField("slot", point).WithField("resumeFrom", cursor).
				Info("Archived point missing; resuming cold state migration from the last archived point on disk")
			return cursor, nil
		}
		cursor = point
	}
	return cursor, nil
}

// migrationLowerBound returns the slot below which no state can be archived:
// the checkpoint origin's slot on a checkpoint-synced node, otherwise genesis.
func (s *State) migrationLowerBound(ctx context.Context) (primitives.Slot, error) {
	originRoot, err := s.beaconDB.OriginCheckpointBlockRoot(ctx)
	if err != nil {
		if errors.Is(err, db.ErrNotFoundOriginBlockRoot) {
			return 0, nil
		}
		return 0, errors.Wrap(err, "could not read the origin checkpoint root")
	}
	blk, err := s.beaconDB.Block(ctx, originRoot)
	if err != nil {
		return 0, errors.Wrap(err, "could not read the origin checkpoint block")
	}
	if blk == nil || blk.IsNil() {
		return 0, errors.New("origin checkpoint block not found")
	}
	return blk.Block().Slot(), nil
}

// advanceFinalizedInfo moves the migration cursor to fSlot, never back, and
// records fRoot and fState as the finalized state when fState is given.
func (s *State) advanceFinalizedInfo(fSlot primitives.Slot, fRoot [32]byte, fState state.BeaconState) {
	s.finalizedInfo.lock.Lock()
	defer s.finalizedInfo.lock.Unlock()
	if fSlot < s.finalizedInfo.slot {
		return
	}
	s.finalizedInfo.slot = fSlot
	if fState != nil && !fState.IsNil() {
		s.finalizedInfo.root = fRoot
		s.finalizedInfo.state = fState.Copy()
	}
}

// SaveFinalizedState saves the finalized slot, root and state into memory to be used by state gen service.
// This used for migration at the correct start slot and used for hot state play back to ensure
// lower bound to start is always at the last finalized state.
func (s *State) SaveFinalizedState(fSlot primitives.Slot, fRoot [32]byte, fState state.BeaconState) {
	s.finalizedInfo.lock.Lock()
	defer s.finalizedInfo.lock.Unlock()
	s.finalizedInfo.root = fRoot
	s.finalizedInfo.state = fState.Copy()
	s.finalizedInfo.slot = fSlot
}

// Returns true if input root equals to cached finalized root.
func (s *State) isFinalizedRoot(r [32]byte) bool {
	s.finalizedInfo.lock.RLock()
	defer s.finalizedInfo.lock.RUnlock()
	return r == s.finalizedInfo.root
}

// Returns the cached and copied finalized state.
func (s *State) finalizedState() state.BeaconState {
	s.finalizedInfo.lock.RLock()
	defer s.finalizedInfo.lock.RUnlock()
	return s.finalizedInfo.state.Copy()
}

// finalizedStateIfRoot returns a copy of the cached finalized state only if
// the cached finalized root matches r at the moment of the read. The root
// comparison and the state copy happen under a single lock acquisition so a
// concurrent SaveFinalizedState cannot swap the finalized info in between and
// make us return a state belonging to a different root than requested.
func (s *State) finalizedStateIfRoot(r [32]byte) state.BeaconState {
	s.finalizedInfo.lock.RLock()
	defer s.finalizedInfo.lock.RUnlock()
	if r != s.finalizedInfo.root || s.finalizedInfo.state == nil {
		return nil
	}
	return s.finalizedInfo.state.Copy()
}
