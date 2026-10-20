package stategen

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/db"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestStateByRoot_GenesisState(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	b := util.NewBeaconBlockZond()
	bRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, bRoot))
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, bRoot))
	loadedState, err := service.StateByRoot(ctx, params.BeaconConfig().ZeroHash) // Zero hash is genesis state root.
	require.NoError(t, err)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())
}

func TestStateByRoot_ColdState(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.finalizedInfo.slot = 2
	service.slotsPerArchivedPoint = 1

	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	util.SaveBlock(t, ctx, beaconDB, b)
	bRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(1))
	val, err := beaconState.ValidatorAtIndex(0)
	require.NoError(t, err)
	val.Slashed = true
	require.NoError(t, beaconState.UpdateValidatorAtIndex(0, val))
	roval, err := beaconState.ValidatorAtIndexReadOnly(0)
	require.NoError(t, err)
	require.Equal(t, true, roval.Slashed())

	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, bRoot))
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, bRoot))
	loadedState, err := service.StateByRoot(ctx, bRoot)
	require.NoError(t, err)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())
}

func TestBalancesByCheckpoint(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.finalizedInfo.slot = 2
	service.slotsPerArchivedPoint = 1
	cfg := params.BeaconConfig()

	// The checkpoint block is the last slot of epoch 0.
	b := util.NewBeaconBlockZond()
	b.Block.Slot = cfg.SlotsPerEpoch - 1
	util.SaveBlock(t, ctx, beaconDB, b)
	bRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(cfg.SlotsPerEpoch-1))
	update := func(idx primitives.ValidatorIndex, mutate func(v *qrysmpb.Validator)) {
		val, err := beaconState.ValidatorAtIndex(idx)
		require.NoError(t, err)
		mutate(val)
		require.NoError(t, beaconState.UpdateValidatorAtIndex(idx, val))
	}
	// Validator 0 is slashed, 1 activates at epoch 1 and 2 exits at epoch 1.
	update(0, func(v *qrysmpb.Validator) { v.Slashed = true })
	update(1, func(v *qrysmpb.Validator) { v.ActivationEpoch = 1 })
	update(2, func(v *qrysmpb.Validator) {
		v.ExitEpoch = 1
		v.WithdrawableEpoch = 1 + cfg.MinValidatorWithdrawabilityDelay
	})
	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, bRoot))

	maxBalance := cfg.MaxEffectiveBalance
	for _, tt := range []struct {
		name     string
		epoch    primitives.Epoch
		balances map[int]uint64
		total    uint64
	}{
		{
			// The block's own epoch: validator 1 is not yet active and 2 still is.
			name:     "checkpoint in the block's epoch",
			epoch:    0,
			balances: map[int]uint64{0: 0, 1: 0, 2: maxBalance},
			total:    31 * maxBalance,
		},
		{
			// Epoch 1 has no block at its first slot, so the checkpoint state is
			// the block's state advanced across the boundary.
			name:     "checkpoint after an empty boundary slot",
			epoch:    1,
			balances: map[int]uint64{0: 0, 1: maxBalance, 2: 0},
			total:    31 * maxBalance,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.BalancesByCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: tt.epoch, Root: bRoot})
			require.NoError(t, err)
			require.Equal(t, 32, len(got.Balances))
			for i, balance := range got.Balances {
				want := maxBalance
				if w, ok := tt.balances[i]; ok {
					want = w
				}
				require.Equal(t, want, balance, "validator %d", i)
			}
			// The slashed validator is excluded from vote weights but still
			// counts towards the total active balance.
			require.Equal(t, tt.total, got.TotalActiveBalance)
		})
	}
	_, err = service.BalancesByCheckpoint(ctx, nil)
	require.ErrorContains(t, "nil checkpoint", err)
}

func TestStateByRootIfCachedNoCopy_HotState(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	r := [32]byte{'A'}
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: r[:]}))
	service.hotStateCache.put(r, beaconState)

	loadedState := service.StateByRootIfCachedNoCopy(r)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())
}

func TestStateByRootIfCachedNoCopy_ColdState(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	service.finalizedInfo.slot = 2
	service.slotsPerArchivedPoint = 1

	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	util.SaveBlock(t, ctx, beaconDB, b)
	bRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(1))
	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, bRoot))
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, bRoot))
	loadedState := service.StateByRootIfCachedNoCopy(bRoot)
	require.NoError(t, err)
	require.Equal(t, loadedState, nil)
}

func TestStateByRoot_HotStateUsingEpochBoundaryCacheNoReplay(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(10))
	blk := util.NewBeaconBlockZond()
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: blkRoot[:]}))
	require.NoError(t, service.epochBoundaryStateCache.put(blkRoot, beaconState))
	loadedState, err := service.StateByRoot(ctx, blkRoot)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(10), loadedState.Slot(), "Did not correctly load state")
}

func TestStateByRoot_HotStateUsingEpochBoundaryCacheWithReplay(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	blk := util.NewBeaconBlockZond()
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.epochBoundaryStateCache.put(blkRoot, beaconState))
	targetSlot := primitives.Slot(10)
	targetBlock := util.NewBeaconBlockZond()
	targetBlock.Block.Slot = 11
	targetBlock.Block.ParentRoot = blkRoot[:]
	targetBlock.Block.ProposerIndex = 8
	util.SaveBlock(t, ctx, service.beaconDB, targetBlock)
	targetRoot, err := targetBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: targetSlot, Root: targetRoot[:]}))
	loadedState, err := service.StateByRoot(ctx, targetRoot)
	require.NoError(t, err)
	assert.Equal(t, targetSlot, loadedState.Slot(), "Did not correctly load state")
}

func TestStateByRoot_RejectsAncestorAfterTarget(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name        string
		initialSync bool
	}{
		{name: "regular getter"},
		{name: "initial sync getter", initialSync: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beaconDB := testDB.SetupDB(t)
			service := New(beaconDB, doublylinkedtree.New())

			ancestorState, _ := util.DeterministicGenesisStateZond(t, 32)
			require.NoError(t, ancestorState.SetSlot(11))
			ancestorBlock := util.NewBeaconBlockZond()
			ancestorRoot, err := ancestorBlock.Block.HashTreeRoot()
			require.NoError(t, err)
			require.NoError(t, service.epochBoundaryStateCache.put(ancestorRoot, ancestorState))

			targetBlock := util.NewBeaconBlockZond()
			targetBlock.Block.Slot = 12
			targetBlock.Block.ParentRoot = ancestorRoot[:]
			targetRoot, err := targetBlock.Block.HashTreeRoot()
			require.NoError(t, err)
			util.SaveBlock(t, ctx, beaconDB, targetBlock)
			require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 10, Root: targetRoot[:]}))

			if tt.initialSync {
				_, err = service.StateByRootInitialSync(ctx, targetRoot)
			} else {
				_, err = service.loadStateByRoot(ctx, targetRoot)
			}
			require.ErrorIs(t, err, ErrReplayTargetSlotExceeded)
		})
	}
}

func TestStateByRoot_HotStateCached(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	r := [32]byte{'A'}
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: r[:]}))
	service.hotStateCache.put(r, beaconState)

	loadedState, err := service.StateByRoot(ctx, r)
	require.NoError(t, err)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())
}

func TestDeleteStateFromCaches(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	r := [32]byte{'A'}

	require.Equal(t, false, service.hotStateCache.has(r))
	_, has, err := service.epochBoundaryStateCache.getByBlockRoot(r)
	require.NoError(t, err)
	require.Equal(t, false, has)

	service.hotStateCache.put(r, beaconState)
	require.NoError(t, service.epochBoundaryStateCache.put(r, beaconState))

	require.Equal(t, true, service.hotStateCache.has(r))
	_, has, err = service.epochBoundaryStateCache.getByBlockRoot(r)
	require.NoError(t, err)
	require.Equal(t, true, has)

	require.NoError(t, service.DeleteStateFromCaches(ctx, r))

	require.Equal(t, false, service.hotStateCache.has(r))
	_, has, err = service.epochBoundaryStateCache.getByBlockRoot(r)
	require.NoError(t, err)
	require.Equal(t, false, has)
}

func TestStateByRoot_StateByRootInitialSync(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())
	b := util.NewBeaconBlockZond()
	bRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, service.beaconDB.SaveState(ctx, beaconState, bRoot))
	util.SaveBlock(t, ctx, service.beaconDB, b)
	require.NoError(t, service.beaconDB.SaveGenesisBlockRoot(ctx, bRoot))
	loadedState, err := service.StateByRootInitialSync(ctx, params.BeaconConfig().ZeroHash) // Zero hash is genesis state root.
	require.NoError(t, err)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())
}

func TestStateByRootInitialSync_UseEpochStateCache(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	targetSlot := primitives.Slot(10)
	require.NoError(t, beaconState.SetSlot(targetSlot))
	blk := util.NewBeaconBlockZond()
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.epochBoundaryStateCache.put(blkRoot, beaconState))
	loadedState, err := service.StateByRootInitialSync(ctx, blkRoot)
	require.NoError(t, err)
	assert.Equal(t, targetSlot, loadedState.Slot(), "Did not correctly load state")
}

func TestStateByRootInitialSync_UseCache(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	r := [32]byte{'A'}
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: r[:]}))
	service.hotStateCache.put(r, beaconState)

	loadedState, err := service.StateByRootInitialSync(ctx, r)
	require.NoError(t, err)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())
	if service.hotStateCache.has(r) {
		t.Error("Hot state cache was not invalidated")
	}
}

func TestStateByRootInitialSync_CanProcessUpTo(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	blk := util.NewBeaconBlockZond()
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.epochBoundaryStateCache.put(blkRoot, beaconState))
	targetSlot := primitives.Slot(10)
	targetBlk := util.NewBeaconBlockZond()
	targetBlk.Block.Slot = 11
	targetBlk.Block.ParentRoot = blkRoot[:]
	targetRoot, err := targetBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.beaconDB, targetBlk)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: targetSlot, Root: targetRoot[:]}))

	loadedState, err := service.StateByRootInitialSync(ctx, targetRoot)
	require.NoError(t, err)
	assert.Equal(t, targetSlot, loadedState.Slot(), "Did not correctly load state")
}

func TestLoadeStateByRoot_Cached(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	r := [32]byte{'A'}
	service.hotStateCache.put(r, beaconState)

	// This tests where hot state was already cached.
	loadedState, err := service.loadStateByRoot(ctx, r)
	require.NoError(t, err)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())
}

func TestLoadeStateByRoot_FinalizedState(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	genesisStateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis := blocks.NewGenesisBlock(genesisStateRoot[:])
	util.SaveBlock(t, ctx, beaconDB, genesis)
	gRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 0, Root: gRoot[:]}))

	service.finalizedInfo.state = beaconState
	service.finalizedInfo.slot = beaconState.Slot()
	service.finalizedInfo.root = gRoot

	// This tests where hot state was already cached.
	loadedState, err := service.loadStateByRoot(ctx, gRoot)
	require.NoError(t, err)
	require.DeepSSZEqual(t, loadedState.ToProtoUnsafe(), beaconState.ToProtoUnsafe())
}

func TestLoadeStateByRoot_EpochBoundaryStateCanProcess(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	gBlk := util.NewBeaconBlockZond()
	gBlkRoot, err := gBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.epochBoundaryStateCache.put(gBlkRoot, beaconState))

	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = 11
	blk.Block.ProposerIndex = 8
	blk.Block.ParentRoot = gBlkRoot[:]
	util.SaveBlock(t, ctx, service.beaconDB, blk)
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 10, Root: blkRoot[:]}))

	// This tests where hot state was not cached and needs processing.
	loadedState, err := service.loadStateByRoot(ctx, blkRoot)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(10), loadedState.Slot(), "Did not correctly load state")
}

func TestLoadeStateByRoot_FromDBBoundaryCase(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	gBlk := util.NewBeaconBlockZond()
	gBlkRoot, err := gBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.epochBoundaryStateCache.put(gBlkRoot, beaconState))

	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = 11
	blk.Block.ProposerIndex = 8
	blk.Block.ParentRoot = gBlkRoot[:]
	util.SaveBlock(t, ctx, service.beaconDB, blk)
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 10, Root: blkRoot[:]}))

	// This tests where hot state was not cached and needs processing.
	loadedState, err := service.loadStateByRoot(ctx, blkRoot)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(10), loadedState.Slot(), "Did not correctly load state")
}

func TestLastAncestorState_CanGetUsingDB(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	b0 := util.NewBeaconBlockZond()
	b0.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	r0, err := b0.Block.HashTreeRoot()
	require.NoError(t, err)
	b1 := util.NewBeaconBlockZond()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = bytesutil.PadTo(r0[:], 32)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	b2 := util.NewBeaconBlockZond()
	b2.Block.Slot = 2
	b2.Block.ParentRoot = bytesutil.PadTo(r1[:], 32)
	r2, err := b2.Block.HashTreeRoot()
	require.NoError(t, err)
	b3 := util.NewBeaconBlockZond()
	b3.Block.Slot = 3
	b3.Block.ParentRoot = bytesutil.PadTo(r2[:], 32)
	r3, err := b3.Block.HashTreeRoot()
	require.NoError(t, err)

	b1State, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, b1State.SetSlot(1))

	util.SaveBlock(t, ctx, service.beaconDB, b0)
	util.SaveBlock(t, ctx, service.beaconDB, b1)
	util.SaveBlock(t, ctx, service.beaconDB, b2)
	util.SaveBlock(t, ctx, service.beaconDB, b3)
	require.NoError(t, service.beaconDB.SaveState(ctx, b1State, r1))

	lastState, _, err := service.latestAncestorAndBlockRootsForSlot(ctx, r3, 3)
	require.NoError(t, err)
	assert.Equal(t, b1State.Slot(), lastState.Slot(), "Did not get wanted state")
}

func TestLastAncestorState_CanGetUsingCache(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	b0 := util.NewBeaconBlockZond()
	b0.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	r0, err := b0.Block.HashTreeRoot()
	require.NoError(t, err)
	b1 := util.NewBeaconBlockZond()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = bytesutil.PadTo(r0[:], 32)
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	b2 := util.NewBeaconBlockZond()
	b2.Block.Slot = 2
	b2.Block.ParentRoot = bytesutil.PadTo(r1[:], 32)
	r2, err := b2.Block.HashTreeRoot()
	require.NoError(t, err)
	b3 := util.NewBeaconBlockZond()
	b3.Block.Slot = 3
	b3.Block.ParentRoot = bytesutil.PadTo(r2[:], 32)
	r3, err := b3.Block.HashTreeRoot()
	require.NoError(t, err)

	b1State, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, b1State.SetSlot(1))

	util.SaveBlock(t, ctx, service.beaconDB, b0)
	util.SaveBlock(t, ctx, service.beaconDB, b1)
	util.SaveBlock(t, ctx, service.beaconDB, b2)
	util.SaveBlock(t, ctx, service.beaconDB, b3)
	service.hotStateCache.put(r1, b1State)

	lastState, _, err := service.latestAncestorAndBlockRootsForSlot(ctx, r3, 3)
	require.NoError(t, err)
	assert.Equal(t, b1State.Slot(), lastState.Slot(), "Did not get wanted state")
}

func TestState_HasState(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	rHit1 := [32]byte{1}
	rHit2 := [32]byte{2}
	rMiss := [32]byte{3}
	service.hotStateCache.put(rHit1, s)
	require.NoError(t, service.epochBoundaryStateCache.put(rHit2, s))

	b := util.NewBeaconBlockZond()
	rHit3, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.beaconDB.SaveState(ctx, s, rHit3))
	tt := []struct {
		root [32]byte
		want bool
	}{
		{rHit1, true},
		{rHit2, true},
		{rMiss, false},
		{rHit3, true},
	}
	for _, tc := range tt {
		got, err := service.HasState(ctx, tc.root)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}

func TestState_HasStateInCache(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	rHit1 := [32]byte{1}
	rHit2 := [32]byte{2}
	rMiss := [32]byte{3}
	service.hotStateCache.put(rHit1, s)
	require.NoError(t, service.epochBoundaryStateCache.put(rHit2, s))

	tt := []struct {
		root [32]byte
		want bool
	}{
		{rHit1, true},
		{rHit2, true},
		{rMiss, false},
	}
	for _, tc := range tt {
		got, err := service.hasStateInCache(ctx, tc.root)
		require.NoError(t, err)
		require.Equal(t, tc.want, got)
	}
}

// stateRemovedAfterCheckDB removes one state (and optionally its block) the
// first time that state is read, reproducing a deletion that lands between the
// loader's HasState check and its State read. The database then reports a nil
// state without an error.
type stateRemovedAfterCheckDB struct {
	db.NoHeadAccessDatabase
	root        [32]byte
	deleteBlock bool
	reads       int
}

func (d *stateRemovedAfterCheckDB) State(ctx context.Context, root [32]byte) (state.BeaconState, error) {
	if root == d.root {
		d.reads++
		if d.reads == 1 {
			var err error
			if d.deleteBlock {
				err = d.NoHeadAccessDatabase.DeleteBlock(ctx, root)
			} else {
				err = d.NoHeadAccessDatabase.DeleteState(ctx, root)
			}
			if err != nil {
				return nil, err
			}
		}
	}
	return d.NoHeadAccessDatabase.State(ctx, root)
}

func TestLoadStateByRoot_StateRemovedBetweenCheckAndRead(t *testing.T) {
	for _, tc := range []struct {
		name        string
		deleteBlock bool
	}{
		{name: "state removed, block kept: regenerated by replay"},
		{name: "block removed too: error, never a nil state", deleteBlock: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			beaconDB := testDB.SetupDB(t)
			beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
			gBlk := util.NewBeaconBlockZond()
			gBlkRoot, err := gBlk.Block.HashTreeRoot()
			require.NoError(t, err)

			blk := util.NewBeaconBlockZond()
			blk.Block.Slot = 11
			blk.Block.ProposerIndex = 8
			blk.Block.ParentRoot = gBlkRoot[:]
			util.SaveBlock(t, ctx, beaconDB, blk)
			blkRoot, err := blk.Block.HashTreeRoot()
			require.NoError(t, err)
			require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 10, Root: blkRoot[:]}))
			saved := beaconState.Copy()
			require.NoError(t, saved.SetSlot(10))
			require.NoError(t, beaconDB.SaveState(ctx, saved, blkRoot))

			racy := &stateRemovedAfterCheckDB{NoHeadAccessDatabase: beaconDB, root: blkRoot, deleteBlock: tc.deleteBlock}
			service := New(racy, doublylinkedtree.New())
			require.NoError(t, service.epochBoundaryStateCache.put(gBlkRoot, beaconState))
			require.Equal(t, true, racy.HasState(ctx, blkRoot))

			loaded, err := service.loadStateByRoot(ctx, blkRoot)
			require.Equal(t, 1, racy.reads, "the loader must take the database short circuit")
			require.Equal(t, false, racy.HasState(ctx, blkRoot), "the read must have observed the removed state")
			if tc.deleteBlock {
				require.ErrorContains(t, "could not get state summary", err)
				require.Equal(t, true, loaded == nil, "an error must never come with a state")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, loaded)
			assert.Equal(t, primitives.Slot(10), loaded.Slot(), "state must be regenerated from the retained block")
		})
	}
}

func TestLastAncestorState_StateRemovedBetweenCheckAndRead(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)

	b0 := util.NewBeaconBlockZond()
	b0.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	r0, err := b0.Block.HashTreeRoot()
	require.NoError(t, err)
	b1 := util.NewBeaconBlockZond()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = r0[:]
	r1, err := b1.Block.HashTreeRoot()
	require.NoError(t, err)
	b2 := util.NewBeaconBlockZond()
	b2.Block.Slot = 2
	b2.Block.ParentRoot = r1[:]
	r2, err := b2.Block.HashTreeRoot()
	require.NoError(t, err)
	b3 := util.NewBeaconBlockZond()
	b3.Block.Slot = 3
	b3.Block.ParentRoot = r2[:]
	r3, err := b3.Block.HashTreeRoot()
	require.NoError(t, err)

	b0State, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	b1State, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, b1State.SetSlot(1))

	util.SaveBlock(t, ctx, beaconDB, b0)
	util.SaveBlock(t, ctx, beaconDB, b1)
	util.SaveBlock(t, ctx, beaconDB, b2)
	util.SaveBlock(t, ctx, beaconDB, b3)
	require.NoError(t, beaconDB.SaveState(ctx, b0State, r0))
	require.NoError(t, beaconDB.SaveState(ctx, b1State, r1))

	// b1's state disappears between the existence check and the read. The walk
	// must continue to b0's state and schedule b1 for replay, rather than
	// return the nil state.
	racy := &stateRemovedAfterCheckDB{NoHeadAccessDatabase: beaconDB, root: r1}
	service := New(racy, doublylinkedtree.New())
	lastState, roots, err := service.latestAncestorAndBlockRootsForSlot(ctx, r3, 3)
	require.NoError(t, err)
	require.Equal(t, 1, racy.reads)
	require.NotNil(t, lastState)
	assert.Equal(t, primitives.Slot(0), lastState.Slot(), "the walk must fall back to the older ancestor state")
	require.DeepEqual(t, [][32]byte{r1, r2, r3}, roots)
}

// The replayer used by the slot-based APIs looks states up through the
// combined cache. The finalized state held in memory must be part of it, so a
// replay for a slot after finalization starts at the finalized checkpoint rather
// than at the previous archived point.
func TestCombinedCache_ServesFinalizedState(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	service := New(beaconDB, doublylinkedtree.New())

	st, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, st.SetSlot(96))
	root := [32]byte{'f'}
	service.SaveFinalizedState(96, root, st)

	cache := service.CombinedCache()
	got, err := cache.ByBlockRoot(root)
	require.NoError(t, err)
	require.Equal(t, st.Slot(), got.Slot())

	_, err = cache.ByBlockRoot([32]byte{'x'})
	require.ErrorIs(t, err, ErrNotInCache)
}
