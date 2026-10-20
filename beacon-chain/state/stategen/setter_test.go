package stategen

import (
	"context"
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestSaveState_HotStateCanBeSaved(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	// This goes to hot section, verify it can save on epoch boundary.
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))

	r := [32]byte{'a'}
	require.NoError(t, service.SaveState(ctx, r, beaconState))

	// Should save both state and state summary.
	_, ok, err := service.epochBoundaryStateCache.getByBlockRoot(r)
	require.NoError(t, err)
	assert.Equal(t, true, ok, "Should have saved the state")
	assert.Equal(t, true, service.beaconDB.HasStateSummary(ctx, r), "Should have saved the state summary")
}

func TestSaveState_HotStateCached(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.slotsPerArchivedPoint = 1
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))

	// Cache the state prior.
	r := [32]byte{'a'}
	service.hotStateCache.put(r, beaconState)
	require.NoError(t, service.SaveState(ctx, r, beaconState))

	// Should not save the state and state summary.
	assert.Equal(t, false, service.beaconDB.HasState(ctx, r), "Should not have saved the state")
	assert.Equal(t, false, service.beaconDB.HasStateSummary(ctx, r), "Should have saved the state summary")
	require.LogsDoNotContain(t, hook, "Saved full state on epoch boundary")
}

func TestState_ForceCheckpoint_SavesStateToDatabase(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	svc := New(beaconDB, doublylinkedtree.New())
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))

	r := [32]byte{'a'}
	svc.hotStateCache.put(r, beaconState)

	require.Equal(t, false, beaconDB.HasState(ctx, r), "Database has state stored already")
	assert.NoError(t, svc.ForceCheckpoint(ctx, r[:]))
	assert.Equal(t, true, beaconDB.HasState(ctx, r), "Did not save checkpoint to database")

	// Should not panic with genesis finalized root.
	assert.NoError(t, svc.ForceCheckpoint(ctx, params.BeaconConfig().ZeroHash[:]))
}

func TestSaveState_Alreadyhas(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	r := [32]byte{'A'}

	// Pre cache the hot state.
	service.hotStateCache.put(r, beaconState)
	require.NoError(t, service.saveStateByRoot(ctx, r, beaconState))

	// Should not save the state and state summary.
	assert.Equal(t, false, service.beaconDB.HasState(ctx, r), "Should not have saved the state")
	assert.Equal(t, false, service.beaconDB.HasStateSummary(ctx, r), "Should have saved the state summary")
	require.LogsDoNotContain(t, hook, "Saved full state on epoch boundary")
}

func TestSaveState_CanSaveOnEpochBoundary(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	r := [32]byte{'A'}

	require.NoError(t, service.saveStateByRoot(ctx, r, beaconState))

	// Should save both state and state summary.
	_, ok, err := service.epochBoundaryStateCache.getByBlockRoot(r)
	require.NoError(t, err)
	require.Equal(t, true, ok, "Did not save epoch boundary state")
	assert.Equal(t, true, service.beaconDB.HasStateSummary(ctx, r), "Should have saved the state summary")
	// Should have not been saved in DB.
	require.Equal(t, false, beaconDB.HasState(ctx, r))
}

func TestSaveState_NoSaveNotEpochBoundary(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch-1))
	r := [32]byte{'A'}
	b := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	require.NoError(t, service.SaveState(ctx, r, beaconState))

	// Should only save state summary.
	assert.Equal(t, false, service.beaconDB.HasState(ctx, r), "Should not have saved the state")
	assert.Equal(t, true, service.beaconDB.HasStateSummary(ctx, r), "Should have saved the state summary")
	require.LogsDoNotContain(t, hook, "Saved full state on epoch boundary")
	// Should have not been saved in DB.
	require.Equal(t, false, beaconDB.HasState(ctx, r))
}

func TestSaveState_RecoverForEpochBoundary(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch-1))
	r := [32]byte{'A'}
	boundaryRoot := [32]byte{'B'}
	require.NoError(t, beaconState.UpdateBlockRootAtIndex(0, boundaryRoot))

	b := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, beaconDB, b)
	gRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, gRoot))
	// Save boundary state to the hot state cache.
	boundaryState, _ := util.DeterministicGenesisStateZond(t, 32)
	service.hotStateCache.put(boundaryRoot, boundaryState)
	require.NoError(t, service.SaveState(ctx, r, beaconState))

	rInfo, ok, err := service.epochBoundaryStateCache.getByBlockRoot(boundaryRoot)
	assert.NoError(t, err)
	assert.Equal(t, true, ok, "state does not exist in cache")
	assert.Equal(t, rInfo.root, boundaryRoot, "incorrect root of root state info")
	assert.Equal(t, rInfo.state.Slot(), primitives.Slot(0), "incorrect slot of state")
}

func TestSaveState_CanSaveHotStateToDB(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.EnableSaveHotStateToDB(ctx)
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(defaultHotStateDBInterval))

	r := [32]byte{'A'}
	require.NoError(t, service.saveStateByRoot(ctx, r, beaconState))

	require.LogsContain(t, hook, "Saving hot state to DB")
	// Should have saved in DB.
	require.Equal(t, true, beaconDB.HasState(ctx, r))
}

func TestEnableSaveHotStateToDB_Enabled(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	service.EnableSaveHotStateToDB(ctx)
	require.LogsContain(t, hook, "Entering mode to save hot states in DB")
	require.Equal(t, true, service.saveHotStateDB.enabled)
}

func TestEnableSaveHotStateToDB_AlreadyEnabled(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.saveHotStateDB.enabled = true
	service.EnableSaveHotStateToDB(ctx)
	require.LogsDoNotContain(t, hook, "Entering mode to save hot states in DB")
	require.Equal(t, true, service.saveHotStateDB.enabled)
}

func TestEnableSaveHotStateToDB_Disabled(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.saveHotStateDB.enabled = true
	b := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, beaconDB, b)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	service.saveHotStateDB.blockRootsOfSavedStates = [][32]byte{r}
	require.NoError(t, service.DisableSaveHotStateToDB(ctx))
	require.LogsContain(t, hook, "Exiting mode to save hot states in DB")
	require.Equal(t, false, service.saveHotStateDB.enabled)
	require.Equal(t, 0, len(service.saveHotStateDB.blockRootsOfSavedStates))
}

func TestEnableSaveHotStateToDB_AlreadyDisabled(t *testing.T) {
	hook := logTest.NewGlobal()
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	require.NoError(t, service.DisableSaveHotStateToDB(ctx))
	require.LogsDoNotContain(t, hook, "Exiting mode to save hot states in DB")
	require.Equal(t, false, service.saveHotStateDB.enabled)
}

func TestDisableSaveHotStateToDB_KeepsProtectedRootForLater(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	base, _ := util.DeterministicGenesisStateZond(t, 8)
	service.EnableSaveHotStateToDB(ctx)
	r1, r2, r3 := [32]byte{'1'}, [32]byte{'2'}, [32]byte{'3'}
	for i, r := range [][32]byte{r1, r2, r3} {
		st := base.Copy()
		require.NoError(t, st.SetSlot(primitives.Slot(128*(i+1))))
		require.NoError(t, service.SaveState(ctx, r, st))
		require.Equal(t, true, beaconDB.HasState(ctx, r))
	}
	// Finality resumes with the justified checkpoint at a block whose hot state was saved.
	require.NoError(t, beaconDB.SaveJustifiedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 8, Root: r2[:]}))
	require.NoError(t, service.DisableSaveHotStateToDB(ctx))
	require.Equal(t, false, beaconDB.HasState(ctx, r1))
	require.Equal(t, true, beaconDB.HasState(ctx, r2), "the justified checkpoint state must stay")
	require.Equal(t, false, beaconDB.HasState(ctx, r3), "states after the protected root must still be deleted")
	require.DeepEqual(t, [][32]byte{r2}, service.saveHotStateDB.blockRootsOfSavedStates)
	require.Equal(t, false, service.saveHotStateDB.enabled)

	// Once finality has moved past it, a later call removes the remaining state.
	other := [32]byte{'o'}
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 1024, Root: other[:]}))
	require.NoError(t, beaconDB.SaveJustifiedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 32, Root: other[:]}))
	require.NoError(t, service.DisableSaveHotStateToDB(ctx))
	require.Equal(t, false, beaconDB.HasState(ctx, r2))
	require.Equal(t, 0, len(service.saveHotStateDB.blockRootsOfSavedStates))
}

// Saving the same hot state twice (a batch saves its boundary states and its
// last state, which may coincide) must record its root once: the migration
// removes the root it archived, and a leftover entry would have the hot-state
// cleanup delete the archived state.
func TestSaveState_HotStateRootRecordedOnce(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	service.EnableSaveHotStateToDB(ctx)
	service.saveHotStateDB.duration = 2

	beaconState, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, beaconState.SetSlot(4))
	r := [32]byte{'r'}
	require.NoError(t, service.SaveState(ctx, r, beaconState))
	// The second save of the same root (hot cache cleared in between, as a
	// cache eviction would do) must not add a second entry.
	service.hotStateCache.delete(r)
	require.NoError(t, service.SaveState(ctx, r, beaconState))
	require.DeepEqual(t, [][32]byte{r}, service.saveHotStateDB.blockRootsOfSavedStates)
}
