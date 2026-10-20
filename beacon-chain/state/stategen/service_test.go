package stategen

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/db"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestResume(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	b := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, service.beaconDB, b)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, root))
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, root))
	require.NoError(t, service.beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: root[:]}))

	resumeState, err := service.Resume(ctx, beaconState)
	require.NoError(t, err)
	require.DeepSSZEqual(t, beaconState.ToProtoUnsafe(), resumeState.ToProtoUnsafe())
	// The migration cursor resumes from the last archived point in the DB; there
	// is none above genesis here.
	assert.Equal(t, primitives.Slot(0), service.finalizedInfo.slot, "Did not get wanted migration cursor")
	assert.Equal(t, service.finalizedInfo.root, root, "Did not get wanted root")
	assert.NotNil(t, service.finalizedState(), "Wanted a non nil finalized state")
}

// The root comparison and the state copy must happen atomically: a mismatched
// root returns nil (so latestAncestor falls through to the other lookup paths)
// rather than the finalized state of a different root. Backport of upstream
// PR #16881.
func TestFinalizedStateIfRoot(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	// Nothing cached yet: any root yields nil instead of a nil-state panic.
	assert.Equal(t, nil, service.finalizedStateIfRoot([32]byte{'a'}))

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	fRoot := [32]byte{'f'}
	service.SaveFinalizedState(0, fRoot, beaconState)

	got := service.finalizedStateIfRoot(fRoot)
	require.NotNil(t, got, "Wanted the finalized state for the matching root")
	require.DeepSSZEqual(t, beaconState.ToProtoUnsafe(), got.ToProtoUnsafe())
	assert.Equal(t, nil, service.finalizedStateIfRoot([32]byte{'o'}), "Wanted nil for a non-finalized root")

	// After the finalized info advances, the old root no longer matches.
	newState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, newState.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	newRoot := [32]byte{'n'}
	service.SaveFinalizedState(params.BeaconConfig().SlotsPerEpoch, newRoot, newState)
	assert.Equal(t, nil, service.finalizedStateIfRoot(fRoot), "Wanted nil for the stale finalized root")
	require.NotNil(t, service.finalizedStateIfRoot(newRoot))
}

// MigrateToCold runs in the background and its cursor is kept in memory only,
// so archived points between the last completed migration and a restart can be
// missing. Resume must start the cursor at the last archived point in the DB
// rather than at the finalized slot, so the next migration fills the gap.
func TestResume_MigrationCursorFromLastArchivedPoint(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 4

	base, _ := util.DeterministicGenesisStateZond(t, 8)
	genesisRoot := [32]byte{'g'}
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))
	require.NoError(t, beaconDB.SaveState(ctx, base, genesisRoot))
	// Archived points 4 and 8 were written for canonical blocks. The chain has
	// no block at 12, so the archive of point 12 is the state of block 8, which
	// is on disk: every point is present and the cursor is the highest one.
	parent := genesisRoot
	for _, slot := range []primitives.Slot{4, 8} {
		parent = saveCanonicalArchivedPoint(t, ctx, beaconDB, base, slot, parent)
	}
	fRoot, fState := saveFinalizedBlock(t, ctx, beaconDB, base, 14, parent)

	_, err := service.Resume(ctx, fState)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(12), service.finalizedInfo.slot, "cursor must resume at the highest present archived point")
	assert.Equal(t, fRoot, service.finalizedInfo.root)
	assert.Equal(t, primitives.Slot(14), service.finalizedInfo.state.Slot())
}

// A state at an archived slot that belongs to a block which lost fork choice
// must not become the migration cursor: the canonical chain may have no block
// at that slot and MigrateToCold never looks below its cursor, so every later
// migration would fail with an unknown block.
func TestResume_MigrationCursorSkipsNonCanonicalArchivedPoint(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 4

	base, _ := util.DeterministicGenesisStateZond(t, 8)
	genesisRoot := [32]byte{'g'}
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))
	require.NoError(t, beaconDB.SaveState(ctx, base, genesisRoot))
	parent := genesisRoot
	for _, slot := range []primitives.Slot{4, 8} {
		parent = saveCanonicalArchivedPoint(t, ctx, beaconDB, base, slot, parent)
	}
	// A block at the archived slot 12 that lost fork choice: its hot state was
	// saved to the DB during non-finality and survives the dirty-state cleanup.
	orphan := util.NewBeaconBlockZond()
	orphan.Block.Slot = 12
	orphan.Block.ParentRoot = parent[:]
	orphan.Block.Body.Graffiti = []byte("orphan")
	orphan.Block.Body.Graffiti = append(orphan.Block.Body.Graffiti, make([]byte, 32-len(orphan.Block.Body.Graffiti))...)
	wOrphan := util.SaveBlock(t, ctx, beaconDB, orphan)
	orphanRoot, err := wOrphan.Block().HashTreeRoot()
	require.NoError(t, err)
	orphanState := base.Copy()
	require.NoError(t, orphanState.SetSlot(12))
	require.NoError(t, beaconDB.SaveState(ctx, orphanState, orphanRoot))
	// The canonical chain skipped slot 12: the finalized block, in the next
	// epoch and off an archive boundary, descends from 8. (Blocks of the
	// finalized epoch itself carry a "status pending" marker in the index;
	// archived points lie below it.)
	fRoot, fState := saveFinalizedBlock(t, ctx, beaconDB, base, params.BeaconConfig().SlotsPerEpoch+2, parent)

	_, err = service.Resume(ctx, fState)
	require.NoError(t, err)
	// Every point up to the finalized epoch is served by the state of block 8;
	// the orphan at 12 plays no part. The migration from that cursor completes
	// even though no canonical block sits at the cursor slot itself.
	assert.Equal(t, primitives.Slot(128), service.finalizedInfo.slot, "cursor must ignore the non-canonical archived point")
	require.NoError(t, service.MigrateToCold(ctx, fRoot))
}

// saveCanonicalArchivedPoint saves a block at slot with the given parent and
// its state, and returns the block root.
func saveCanonicalArchivedPoint(t *testing.T, ctx context.Context, beaconDB db.Database, base state.BeaconState, slot primitives.Slot, parent [32]byte) [32]byte {
	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = slot
	blk.Block.ParentRoot = parent[:]
	wb := util.SaveBlock(t, ctx, beaconDB, blk)
	root, err := wb.Block().HashTreeRoot()
	require.NoError(t, err)
	st := base.Copy()
	require.NoError(t, st.SetSlot(slot))
	require.NoError(t, beaconDB.SaveState(ctx, st, root))
	return root
}

// saveFinalizedBlock saves a block at slot with the given parent, its state,
// and records it as the finalized checkpoint, which indexes its ancestry as
// finalized. It returns the block root and state.
func saveFinalizedBlock(t *testing.T, ctx context.Context, beaconDB db.Database, base state.BeaconState, slot primitives.Slot, parent [32]byte) ([32]byte, state.BeaconState) {
	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = slot
	blk.Block.ParentRoot = parent[:]
	wb := util.SaveBlock(t, ctx, beaconDB, blk)
	fRoot, err := wb.Block().HashTreeRoot()
	require.NoError(t, err)
	fState := base.Copy()
	require.NoError(t, fState.SetSlot(slot))
	require.NoError(t, beaconDB.SaveState(ctx, fState, fRoot))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: slots.ToEpoch(slot), Root: fRoot[:]}))
	return fRoot, fState
}

// The migration cursor recorded by the last completed migration takes
// precedence over the highest archived state: a finalized state forced to disk
// at an archived slot (shutdown) or a hot state saved there can sit above an
// unfinished range, which the scan alone would never fill.
func TestResume_MigrationCursorPrefersRecordedProgress(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 4

	base, _ := util.DeterministicGenesisStateZond(t, 8)
	genesisRoot := [32]byte{'g'}
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))
	require.NoError(t, beaconDB.SaveState(ctx, base, genesisRoot))
	// Points 4 and 12 are on disk; the migration that would have handled the
	// archived slot 8 (served by the canonical block at 7) never completed.
	// The state at 12 is the finalized state itself.
	parent := saveCanonicalArchivedPoint(t, ctx, beaconDB, base, 4, genesisRoot)
	parent = saveCanonicalArchivedPoint(t, ctx, beaconDB, base, 7, parent)
	fRoot, fState := saveFinalizedBlock(t, ctx, beaconDB, base, 12, parent)
	require.NoError(t, beaconDB.SaveStateMigrationCursor(ctx, 4))

	_, err := service.Resume(ctx, fState)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(4), service.finalizedInfo.slot, "cursor must resume from the recorded progress")

	// The migration walks the gap and moves the recorded cursor forward.
	require.NoError(t, service.MigrateToCold(ctx, fRoot))
	cursor, found, err := beaconDB.StateMigrationCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, true, found)
	assert.Equal(t, primitives.Slot(12), cursor)

	// Without a usable recorded cursor the scan alone decides.
	service2 := New(beaconDB, doublylinkedtree.New())
	service2.slotsPerArchivedPoint = 4
	require.NoError(t, beaconDB.SaveStateMigrationCursor(ctx, 1<<40))
	_, err = service2.Resume(ctx, fState)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(12), service2.finalizedInfo.slot)
}

// A database written before the migration cursor existed can hold an archived
// state above an unfinished range. Without a recorded cursor the migration
// resumes at the last archived point whose archive is on disk before the gap,
// so the gap is filled and the first migration step can be reconstructed from
// that archive.
func TestResume_NoRecordedCursorResumesBeforeFirstMissingPoint(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 4

	beaconState, pks := util.DeterministicGenesisStateZond(t, 32)
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	// Real blocks at 4 (archived), 7 (state missing: the archive of point 8 was
	// never written) and 12 (the finalized state, forced to disk at shutdown).
	roots := map[primitives.Slot][32]byte{}
	states := map[primitives.Slot]state.BeaconState{}
	for _, slot := range []primitives.Slot{4, 7, 12} {
		b, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), slot)
		require.NoError(t, err)
		wb, err := consensusblocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		beaconState, err = executeStateTransitionStateGen(ctx, beaconState, wb)
		require.NoError(t, err)
		r, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, beaconDB, b)
		require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: slot, Root: r[:]}))
		roots[slot], states[slot] = r, beaconState.Copy()
	}
	require.NoError(t, beaconDB.SaveState(ctx, states[4], roots[4]))
	require.NoError(t, beaconDB.SaveState(ctx, states[12], roots[12]))
	fRoot := roots[12]
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: fRoot[:]}))

	_, err = service.Resume(ctx, states[12])
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(4), service.finalizedInfo.slot, "cursor must resume before the missing archived point")

	// The migration from there rebuilds the missing archive and records its progress.
	require.NoError(t, service.MigrateToCold(ctx, fRoot))
	require.Equal(t, true, beaconDB.HasState(ctx, roots[7]), "the archive of point 8 (the state of block 7) was not rebuilt")
	cursor, found, err := beaconDB.StateMigrationCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, true, found)
	assert.Equal(t, primitives.Slot(12), cursor)

	// An archive lost below the recorded cursor (an older release's cleanup
	// removed skipped-slot archives) is found by the scan and rebuilt.
	require.NoError(t, beaconDB.DeleteState(ctx, roots[7]))
	service2 := New(beaconDB, doublylinkedtree.New())
	service2.slotsPerArchivedPoint = 4
	_, err = service2.Resume(ctx, states[12])
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(4), service2.finalizedInfo.slot, "the recorded cursor must not hide a missing archive")
	require.NoError(t, service2.MigrateToCold(ctx, fRoot))
	require.Equal(t, true, beaconDB.HasState(ctx, roots[7]), "the archive of point 8 was not rebuilt after its loss")
}

// On a checkpoint-synced node the migration cannot go below the origin: without
// a recorded cursor it resumes at the origin's slot and the first migration
// after that completes even though no block sits at the archived slot itself.
func TestResume_NoRecordedCursorOnCheckpointSyncedNode(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 4

	base, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, beaconDB.SaveGenesisData(ctx, base))
	genesisRoot, err := beaconDB.GenesisBlockRoot(ctx)
	require.NoError(t, err)

	// The origin block at slot 5, between the archived slots 4 and 8.
	originSlot := primitives.Slot(5)
	originState := base.Copy()
	require.NoError(t, originState.SetSlot(originSlot))
	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = originSlot
	blk.Block.ParentRoot = genesisRoot[:]
	bodyRoot, err := blk.Block.Body.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, originState.SetLatestBlockHeader(&qrysmpb.BeaconBlockHeader{
		Slot:          originSlot,
		ProposerIndex: blk.Block.ProposerIndex,
		ParentRoot:    genesisRoot[:],
		StateRoot:     make([]byte, 32),
		BodyRoot:      bodyRoot[:],
	}))
	stateRoot, err := originState.HashTreeRoot(ctx)
	require.NoError(t, err)
	blk.Block.StateRoot = stateRoot[:]
	originRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	sb, err := originState.MarshalSSZ()
	require.NoError(t, err)
	wb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	bb, err := wb.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveOrigin(ctx, sb, bb))

	_, err = service.Resume(ctx, originState)
	require.NoError(t, err)
	assert.Equal(t, originSlot, service.finalizedInfo.slot, "cursor must not go below the origin")

	// Finality moves to slot 9: the archived slot 8 is served by the origin's state.
	fRoot, _ := saveFinalizedBlock(t, ctx, beaconDB, base, 9, originRoot)
	require.NoError(t, service.MigrateToCold(ctx, fRoot))
	assert.Equal(t, primitives.Slot(9), service.finalizedInfo.slot, "the in-memory cursor advances without a cache entry")
	cursor, found, err := beaconDB.StateMigrationCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, true, found)
	assert.Equal(t, primitives.Slot(9), cursor)
}
