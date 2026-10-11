package stategen

import (
	"context"
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/state"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestMigrateToCold_CanSaveFinalizedInfo(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	br, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.epochBoundaryStateCache.put(br, beaconState))
	require.NoError(t, service.MigrateToCold(ctx, br))

	wanted := &finalizedInfo{state: beaconState, root: br, slot: 1}
	assert.DeepEqual(t, wanted.root, service.finalizedInfo.root)
	assert.Equal(t, wanted.slot, service.finalizedInfo.slot)
	expectedHTR, err := wanted.state.HashTreeRoot(ctx)
	require.NoError(t, err)
	actualHTR, err := service.finalizedInfo.state.HashTreeRoot(ctx)
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, actualHTR)
}

func TestMigrateToCold_HappyPath(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	stateSlot := primitives.Slot(1)
	require.NoError(t, beaconState.SetSlot(stateSlot))
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 2
	fRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.epochBoundaryStateCache.put(fRoot, beaconState))
	// The migration resolves canonicality through the finalized index, which the
	// blockchain service populates before calling MigrateToCold.
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, [32]byte{}))
	require.NoError(t, service.beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: fRoot[:]}))
	require.NoError(t, service.MigrateToCold(ctx, fRoot))

	gotState, err := service.beaconDB.State(ctx, fRoot)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, beaconState.ToProtoUnsafe(), gotState.ToProtoUnsafe(), "Did not save state")
	gotRoot := service.beaconDB.ArchivedPointRoot(ctx, stateSlot/service.slotsPerArchivedPoint)
	assert.Equal(t, fRoot, gotRoot, "Did not save archived root")
	lastIndex, err := service.beaconDB.LastArchivedSlot(ctx)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(1), lastIndex, "Did not save last archived index")

	require.LogsContain(t, hook, "Saved state in DB")
}

func TestMigrateToCold_RegeneratePath(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, pks := util.DeterministicGenesisStateZond(t, 32)
	genState := beaconState.Copy()
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.NoError(t, beaconDB.SaveState(ctx, genState, gRoot))
	assert.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	b1, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	wB1, err := consensusblocks.NewSignedBeaconBlock(b1)
	require.NoError(t, err)
	beaconState, err = executeStateTransitionStateGen(ctx, beaconState, wB1)
	require.NoError(t, err)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b1)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 1, Root: r1[:]}))

	b4, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), 4)
	require.NoError(t, err)
	r4, err := b4.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b4)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 4, Root: r4[:]}))
	service.finalizedInfo = &finalizedInfo{
		slot:  0,
		root:  genesisStateRoot,
		state: genState,
	}
	// The migration resolves canonicality through the finalized index, which the
	// blockchain service populates before calling MigrateToCold.
	require.NoError(t, service.beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: r4[:]}))

	require.NoError(t, service.MigrateToCold(ctx, r4))

	s1, err := service.beaconDB.State(ctx, r1)
	require.NoError(t, err)
	assert.Equal(t, s1.Slot(), primitives.Slot(1), "Did not save state")
	gotRoot := service.beaconDB.ArchivedPointRoot(ctx, 1/service.slotsPerArchivedPoint)
	assert.Equal(t, r1, gotRoot, "Did not save archived root")
	lastIndex, err := service.beaconDB.LastArchivedSlot(ctx)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(1), lastIndex, "Did not save last archived index")

	require.LogsContain(t, hook, "Saved state in DB")
}

func TestMigrateToCold_StateExistsInDB(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	stateSlot := primitives.Slot(1)
	require.NoError(t, beaconState.SetSlot(stateSlot))
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 2
	fRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.epochBoundaryStateCache.put(fRoot, beaconState))
	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, fRoot))
	// The migration resolves canonicality through the finalized index, which the
	// blockchain service populates before calling MigrateToCold.
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, [32]byte{}))
	require.NoError(t, service.beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: fRoot[:]}))

	service.saveHotStateDB.blockRootsOfSavedStates = [][32]byte{{1}, {2}, {3}, {4}, fRoot}
	require.NoError(t, service.MigrateToCold(ctx, fRoot))
	assert.DeepEqual(t, [][32]byte{{1}, {2}, {3}, {4}}, service.saveHotStateDB.blockRootsOfSavedStates)
	assert.LogsDoNotContain(t, hook, "Saved state in DB")
}

func TestMigrateToCold_ParallelCalls(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, pks := util.DeterministicGenesisStateZond(t, 32)
	genState := beaconState.Copy()
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.NoError(t, beaconDB.SaveState(ctx, beaconState, gRoot))
	assert.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	b1, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	wB1, err := consensusblocks.NewSignedBeaconBlock(b1)
	require.NoError(t, err)
	beaconState, err = executeStateTransitionStateGen(ctx, beaconState, wB1)
	assert.NoError(t, err)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b1)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 1, Root: r1[:]}))

	b4, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), 4)
	require.NoError(t, err)
	wB4, err := consensusblocks.NewSignedBeaconBlock(b4)
	require.NoError(t, err)
	beaconState, err = executeStateTransitionStateGen(ctx, beaconState, wB4)
	assert.NoError(t, err)
	r4, err := b4.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b4)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 4, Root: r4[:]}))

	b7, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), 7)
	require.NoError(t, err)
	wB7, err := consensusblocks.NewSignedBeaconBlock(b7)
	require.NoError(t, err)
	beaconState, err = executeStateTransitionStateGen(ctx, beaconState, wB7)
	assert.NoError(t, err)
	r7, err := b7.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, b7)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 7, Root: r7[:]}))

	// The migration resolves canonicality through the finalized index, which the
	// blockchain service populates before calling MigrateToCold.
	require.NoError(t, service.beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: r7[:]}))

	service.finalizedInfo = &finalizedInfo{
		slot:  0,
		root:  genesisStateRoot,
		state: genState,
	}
	service.saveHotStateDB.blockRootsOfSavedStates = [][32]byte{r1, r4, r7}

	// Run the migration routines concurrently for 2 different finalized roots.
	// Join the goroutine before asserting: it must not report its result, or
	// run against the closed database, after the test has completed.
	parallelErr := make(chan error, 1)
	go func() {
		parallelErr <- service.MigrateToCold(ctx, r4)
	}()

	require.NoError(t, service.MigrateToCold(ctx, r7))
	require.NoError(t, <-parallelErr)

	s1, err := service.beaconDB.State(ctx, r1)
	require.NoError(t, err)
	assert.Equal(t, s1.Slot(), primitives.Slot(1), "Did not save state")
	s4, err := service.beaconDB.State(ctx, r4)
	require.NoError(t, err)
	assert.Equal(t, s4.Slot(), primitives.Slot(4), "Did not save state")

	gotRoot := service.beaconDB.ArchivedPointRoot(ctx, 1/service.slotsPerArchivedPoint)
	assert.Equal(t, r1, gotRoot, "Did not save archived root")
	gotRoot = service.beaconDB.ArchivedPointRoot(ctx, 4)
	assert.Equal(t, r4, gotRoot, "Did not save archived root")
	lastIndex, err := service.beaconDB.LastArchivedSlot(ctx)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(4), lastIndex, "Did not save last archived index")
	assert.DeepEqual(t, [][32]byte{r7}, service.saveHotStateDB.blockRootsOfSavedStates, "Did not remove all saved hot state roots")
	require.LogsContain(t, hook, "Saved state in DB")
}

// Regression test for the qrysm port of upstream PR #17371: at an archived
// point, the migration must not archive the state of a block that lost fork
// choice. A stale epoch-boundary cache entry keyed by a reorged block holds the
// slot key; the migration must archive the canonical block at the boundary slot
// itself, not the reorged entry and not an older block. Two equivocating blocks
// at the same slot used to abort the migration with errUnknownBlock forever.
func TestMigrateToCold_SkipsReorgedBoundaryBlocks(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 3
	beaconState, pks := util.DeterministicGenesisStateZond(t, 32)
	genState := beaconState.Copy()
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveState(ctx, genState, gRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	// Canonical block at slot 1.
	b1, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	wB1, err := consensusblocks.NewSignedBeaconBlock(b1)
	require.NoError(t, err)
	beaconState, err = executeStateTransitionStateGen(ctx, beaconState, wB1)
	require.NoError(t, err)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b1)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 1, Root: r1[:]}))

	// Two equivocating children of slot 1 at slot 2; both lose fork choice and
	// are never deleted from the DB.
	bO1, err := util.GenerateFullBlockZond(beaconState.Copy(), pks, util.DefaultBlockGenConfig(), 2)
	require.NoError(t, err)
	rO1, err := bO1.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, bO1)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 2, Root: rO1[:]}))
	bO2, err := util.GenerateFullBlockZond(beaconState.Copy(), pks, util.DefaultBlockGenConfig(), 2)
	require.NoError(t, err)
	graffiti := make([]byte, 32)
	graffiti[0] = 'o'
	bO2.Block.Body.Graffiti = graffiti
	rO2, err := bO2.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NotEqual(t, rO1, rO2, "the equivocating blocks must differ for this test to mean anything")
	util.SaveBlock(t, ctx, beaconDB, bO2)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 2, Root: rO2[:]}))

	// The canonical chain skips slot 2: slot 3 builds on slot 1.
	b3, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), 3)
	require.NoError(t, err)
	require.DeepEqual(t, r1[:], b3.Block.ParentRoot, "slot 3 must build on slot 1, orphaning slot 2")
	wB3, err := consensusblocks.NewSignedBeaconBlock(b3)
	require.NoError(t, err)
	beaconState, err = executeStateTransitionStateGen(ctx, beaconState, wB3)
	require.NoError(t, err)
	r3, err := b3.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b3)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 3, Root: r3[:]}))

	// The new finalized block at slot 4.
	b4, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), 4)
	require.NoError(t, err)
	r4, err := b4.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b4)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 4, Root: r4[:]}))

	// The blockchain service saves the finalized checkpoint before calling
	// MigrateToCold, so the index holds 4 -> 3 -> 1 -> genesis, never the orphans.
	// The checkpoint epoch is 1: the finalized index is canonical-exact only for
	// epochs older than the checkpoint epoch (blocks in the checkpoint epoch
	// itself are all indexed as finalized-but-not-canonical), and the migration
	// processes slots from the previous finalized epoch, which is older.
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: r4[:], Epoch: 1}))
	require.Equal(t, false, beaconDB.IsFinalizedBlock(ctx, rO1))
	require.Equal(t, false, beaconDB.IsFinalizedBlock(ctx, rO2))

	// A stale epoch-boundary cache entry at the archived slot, keyed by a
	// reorged block: processed first, it holds the slot key until evicted.
	orphanCacheState := genState.Copy()
	require.NoError(t, orphanCacheState.SetSlot(3))
	require.NoError(t, service.epochBoundaryStateCache.put(rO1, orphanCacheState))

	service.finalizedInfo = &finalizedInfo{slot: 0, root: gRoot, state: genState}
	require.NoError(t, service.MigrateToCold(ctx, r4))

	// The archived state at the boundary belongs to the canonical block at the
	// boundary slot itself, not to either reorged block and not to an older
	// block hidden behind the stale cache entry.
	s3, err := beaconDB.State(ctx, r3)
	require.NoError(t, err)
	require.NotNil(t, s3)
	assert.Equal(t, primitives.Slot(3), s3.Slot(), "Did not archive the canonical state")
	assert.Equal(t, false, beaconDB.HasState(ctx, r1), "Archived an older block instead of the canonical boundary block")
	assert.Equal(t, false, beaconDB.HasState(ctx, rO1), "Archived a reorged block's state")
	assert.Equal(t, false, beaconDB.HasState(ctx, rO2), "Archived a reorged block's state")
}

func TestMigrateToCold_UnknownFinalizedBlock(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	require.ErrorContains(t, "not found", service.MigrateToCold(ctx, [32]byte{'z'}))
}

// When the canonical chain skips the archived slot, the migration resolves the
// highest canonical block below it, skipping orphans (here two equivocating
// blocks) that sit at the highest populated slot in between.
func TestMigrateToCold_SkipsOrphansBelowSkippedBoundary(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 3
	beaconState, pks := util.DeterministicGenesisStateZond(t, 32)
	genState := beaconState.Copy()
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveState(ctx, genState, gRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	// Canonical block at slot 1.
	b1, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	wB1, err := consensusblocks.NewSignedBeaconBlock(b1)
	require.NoError(t, err)
	beaconState, err = executeStateTransitionStateGen(ctx, beaconState, wB1)
	require.NoError(t, err)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b1)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 1, Root: r1[:]}))

	// Two equivocating children of slot 1 at slot 2; both lose fork choice.
	bO1, err := util.GenerateFullBlockZond(beaconState.Copy(), pks, util.DefaultBlockGenConfig(), 2)
	require.NoError(t, err)
	rO1, err := bO1.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, bO1)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 2, Root: rO1[:]}))
	bO2, err := util.GenerateFullBlockZond(beaconState.Copy(), pks, util.DefaultBlockGenConfig(), 2)
	require.NoError(t, err)
	graffiti := make([]byte, 32)
	graffiti[0] = 'o'
	bO2.Block.Body.Graffiti = graffiti
	rO2, err := bO2.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NotEqual(t, rO1, rO2, "the equivocating blocks must differ for this test to mean anything")
	util.SaveBlock(t, ctx, beaconDB, bO2)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 2, Root: rO2[:]}))

	// The canonical chain skips slots 2 and 3 (the archived point): slot 4 builds on slot 1.
	b4, err := util.GenerateFullBlockZond(beaconState, pks, util.DefaultBlockGenConfig(), 4)
	require.NoError(t, err)
	require.DeepEqual(t, r1[:], b4.Block.ParentRoot, "slot 4 must build on slot 1, orphaning slot 2")
	r4, err := b4.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b4)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 4, Root: r4[:]}))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: r4[:], Epoch: 1}))

	service.finalizedInfo = &finalizedInfo{slot: 0, root: gRoot, state: genState}
	require.NoError(t, service.MigrateToCold(ctx, r4))

	s1, err := beaconDB.State(ctx, r1)
	require.NoError(t, err)
	require.NotNil(t, s1)
	assert.Equal(t, primitives.Slot(1), s1.Slot(), "Did not archive the canonical state")
	assert.Equal(t, false, beaconDB.HasState(ctx, rO1), "Archived a reorged block's state")
	assert.Equal(t, false, beaconDB.HasState(ctx, rO2), "Archived a reorged block's state")
}

// After a restart whose finalized block sits exactly on an archived point, the
// migration must handle that point (its state is already in the DB) and move
// the finalized info on. Looking only strictly below the archived slot tripped
// the floor check with errUnknownBlock, and since the finalized info was then
// never advanced, every later migration failed the same way until a restart.
func TestMigrateToCold_RestartWithFinalizedBlockAtArchivedPoint(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 4

	beaconState, pks := util.DeterministicGenesisStateZond(t, 32)
	genState := beaconState.Copy()
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveState(ctx, genState, gRoot))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))

	roots := make(map[primitives.Slot][32]byte)
	states := make(map[primitives.Slot]state.BeaconState)
	for slot := primitives.Slot(1); slot <= 8; slot++ {
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
		roots[slot] = r
		states[slot] = beaconState.Copy()
	}

	// Finality reached the block at slot 4 (an archived point) and the node was
	// restarted there: the finalized state is in the DB (ForceCheckpoint at
	// shutdown), finalizedInfo is what Resume sets, the boundary cache is empty.
	r4, r8 := roots[4], roots[8]
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: r4[:]}))
	require.NoError(t, beaconDB.SaveState(ctx, states[4], r4))
	service.finalizedInfo = &finalizedInfo{slot: 4, root: r4, state: states[4]}

	// Finality moves on to slot 8 after the restart.
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: r8[:]}))
	require.NoError(t, service.epochBoundaryStateCache.put(r8, states[8]))

	require.NoError(t, service.MigrateToCold(ctx, r8), "archived point 4 already holds the finalized state; the migration must not fail")
	assert.Equal(t, primitives.Slot(8), service.finalizedInfo.slot)
	assert.Equal(t, r8, service.finalizedInfo.root)
	assert.Equal(t, true, beaconDB.HasState(ctx, r4))
}

// An archived root present more than once in the hot-state list (as older
// releases could record it) is removed entirely by the migration, so the
// hot-state cleanup can never delete the archived state through a leftover entry.
func TestMigrateToCold_RemovesEveryHotStateEntryOfArchivedRoot(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(1))
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	util.SaveBlock(t, ctx, beaconDB, b)
	bRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveState(ctx, beaconState, bRoot))
	// The finalized index walk stops at the genesis root (the blocks' zero parent).
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, [32]byte{}))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: bRoot[:]}))
	fb := util.NewBeaconBlockZond()
	fb.Block.Slot = 2
	fb.Block.ParentRoot = bRoot[:]
	util.SaveBlock(t, ctx, beaconDB, fb)
	fRoot, err := fb.Block.HashTreeRoot()
	require.NoError(t, err)
	fState := beaconState.Copy()
	require.NoError(t, fState.SetSlot(2))
	require.NoError(t, beaconDB.SaveState(ctx, fState, fRoot))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: fRoot[:]}))

	service.finalizedInfo.slot = 1
	service.saveHotStateDB.blockRootsOfSavedStates = [][32]byte{bRoot, {9}, bRoot}
	require.NoError(t, service.MigrateToCold(ctx, fRoot))
	require.DeepEqual(t, [][32]byte{{9}}, service.saveHotStateDB.blockRootsOfSavedStates)
	require.Equal(t, true, beaconDB.HasState(ctx, bRoot))
	// The migration recorded where it stopped.
	cursor, found, err := beaconDB.StateMigrationCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, true, found)
	require.Equal(t, primitives.Slot(2), cursor)
}

// Migrations run in the background and complete in any order. The in-memory
// cursor advances even when the finalized state is in no cache, so an older
// migration finishing later is a no-op instead of redoing the range and moving
// the recorded cursor back.
func TestMigrateToCold_OlderMigrationDoesNotRegressProgress(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 4

	base, _ := util.DeterministicGenesisStateZond(t, 8)
	genesisRoot := [32]byte{'g'}
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, genesisRoot))
	require.NoError(t, beaconDB.SaveState(ctx, base, genesisRoot))
	older, _ := saveFinalizedBlock(t, ctx, beaconDB, base, 8, genesisRoot)
	newer, _ := saveFinalizedBlock(t, ctx, beaconDB, base, 12, older)

	require.NoError(t, service.MigrateToCold(ctx, newer))
	require.Equal(t, primitives.Slot(12), service.finalizedInfo.slot, "the cursor must advance without a boundary cache entry")
	cursor, _, err := beaconDB.StateMigrationCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(12), cursor)

	// The older migration, queued before the newer one, completes afterwards.
	require.NoError(t, service.MigrateToCold(ctx, older))
	require.Equal(t, primitives.Slot(12), service.finalizedInfo.slot)
	cursor, _, err = beaconDB.StateMigrationCursor(ctx)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(12), cursor, "an older migration must not move the recorded cursor back")
}
