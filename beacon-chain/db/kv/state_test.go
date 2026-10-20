package kv

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"
	"path"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/features"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	bolt "go.etcd.io/bbolt"
)

func TestStateNil(t *testing.T) {
	db := setupDB(t)
	_, err := db.StateOrError(context.Background(), [32]byte{})
	require.ErrorIs(t, err, ErrNotFoundState)
}

func TestState_CanSaveRetrieve(t *testing.T) {
	db := setupDB(t)

	r := [32]byte{'A'}

	require.Equal(t, false, db.HasState(context.Background(), r))

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(100))

	require.NoError(t, db.SaveState(context.Background(), st, r))
	assert.Equal(t, true, db.HasState(context.Background(), r))

	savedS, err := db.State(context.Background(), r)
	require.NoError(t, err)

	require.DeepSSZEqual(t, st.ToProtoUnsafe(), savedS.ToProtoUnsafe(), "saved state and retrieved state are not matching")

	savedS, err = db.State(context.Background(), [32]byte{'B'})
	require.NoError(t, err)
	assert.Equal(t, state.ReadOnlyBeaconState(nil), savedS, "Unsaved state should've been nil")
}

func TestState_CanSaveRetrieveValidatorEntries(t *testing.T) {
	db := setupDB(t)

	// enable historical state representation flag to test this
	resetCfg := features.InitWithReset(&features.Flags{
		EnableHistoricalSpaceRepresentation: true,
	})
	defer resetCfg()

	r := [32]byte{'A'}

	require.Equal(t, false, db.HasState(context.Background(), r))

	stateValidators := validators(10)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(100))
	require.NoError(t, st.SetValidators(stateValidators))

	ctx := context.Background()
	require.NoError(t, db.SaveState(ctx, st, r))
	assert.Equal(t, true, db.HasState(context.Background(), r))

	savedS, err := db.State(context.Background(), r)
	require.NoError(t, err)

	require.DeepSSZEqual(t, st.ToProtoUnsafe(), savedS.ToProtoUnsafe(), "saved state with validators and retrieved state are not matching")

	// check if the index of the second state is still present.
	err = db.db.Update(func(tx *bolt.Tx) error {
		idxBkt := tx.Bucket(blockRootValidatorHashesBucket)
		data := idxBkt.Get(r[:])
		require.NotEqual(t, 0, len(data))
		return nil
	})
	require.NoError(t, err)

	// check if all the validator entries are still intact in the validator entry bucket.
	err = db.db.Update(func(tx *bolt.Tx) error {
		valBkt := tx.Bucket(stateValidatorsBucket)
		// if any of the original validator entry is not present, then fail the test.
		for _, val := range stateValidators {
			hash, hashErr := val.HashTreeRoot()
			assert.NoError(t, hashErr)
			data := valBkt.Get(hash[:])
			require.NotNil(t, data)
			require.NotEqual(t, 0, len(data))
		}
		return nil
	})
	require.NoError(t, err)
}

func TestState_CanSaveRetrieveValidatorEntriesFromCache(t *testing.T) {
	db := setupDB(t)

	// enable historical state representation flag to test this
	resetCfg := features.InitWithReset(&features.Flags{
		EnableHistoricalSpaceRepresentation: true,
	})
	defer resetCfg()

	r := [32]byte{'A'}

	require.Equal(t, false, db.HasState(context.Background(), r))

	stateValidators := validators(10)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(100))
	require.NoError(t, st.SetValidators(stateValidators))

	ctx := context.Background()
	require.NoError(t, db.SaveState(ctx, st, r))
	assert.Equal(t, true, db.HasState(context.Background(), r))

	// The cache is populated once the transaction has committed, and ristretto
	// applies Set asynchronously: drain its buffer before inspecting the cache.
	db.validatorEntryCache.Wait()

	// check if the state is in cache
	for i := range stateValidators {
		hash, hashErr := stateValidators[i].HashTreeRoot()
		assert.NoError(t, hashErr)

		data, ok := db.validatorEntryCache.Get(string(hash[:]))
		assert.Equal(t, true, ok)
		require.NotNil(t, data)

		valEntry, vType := data.(*qrysmpb.Validator)
		assert.Equal(t, true, vType)
		require.NotNil(t, valEntry)

		require.DeepSSZEqual(t, stateValidators[i], valEntry, "validator entry is not matching")
	}

	// check if all the validator entries are still intact in the validator entry bucket.
	err = db.db.Update(func(tx *bolt.Tx) error {
		valBkt := tx.Bucket(stateValidatorsBucket)
		// if any of the original validator entry is not present, then fail the test.
		for _, val := range stateValidators {
			hash, hashErr := val.HashTreeRoot()
			assert.NoError(t, hashErr)
			data := valBkt.Get(hash[:])
			require.NotNil(t, data)
			require.NotEqual(t, 0, len(data))
		}
		return nil
	})
	require.NoError(t, err)

}

func TestState_CanSaveRetrieveValidatorEntriesWithoutCache(t *testing.T) {
	db := setupDB(t)

	// enable historical state representation flag to test this
	resetCfg := features.InitWithReset(&features.Flags{
		EnableHistoricalSpaceRepresentation: true,
	})
	defer resetCfg()

	r := [32]byte{'A'}

	require.Equal(t, false, db.HasState(context.Background(), r))

	stateValidators := validators(10)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(100))
	require.NoError(t, st.SetValidators(stateValidators))

	ctx := context.Background()
	require.NoError(t, db.SaveState(ctx, st, r))
	assert.Equal(t, true, db.HasState(context.Background(), r))
	db.validatorEntryCache.Clear()

	savedS, err := db.State(context.Background(), r)
	require.NoError(t, err)

	require.DeepSSZEqual(t, st.ToProtoUnsafe(), savedS.ToProtoUnsafe(), "saved state with validators and retrieved state are not matching")

	// check if the index of the second state is still present.
	err = db.db.Update(func(tx *bolt.Tx) error {
		idxBkt := tx.Bucket(blockRootValidatorHashesBucket)
		data := idxBkt.Get(r[:])
		require.NotEqual(t, 0, len(data))
		return nil
	})
	require.NoError(t, err)

	// check if all the validator entries are still intact in the validator entry bucket.
	err = db.db.Update(func(tx *bolt.Tx) error {
		valBkt := tx.Bucket(stateValidatorsBucket)
		// if any of the original validator entry is not present, then fail the test.
		for _, val := range stateValidators {
			hash, hashErr := val.HashTreeRoot()
			assert.NoError(t, hashErr)
			data := valBkt.Get(hash[:])
			require.NotNil(t, data)
			require.NotEqual(t, 0, len(data))
		}
		return nil
	})
	require.NoError(t, err)

}

func TestState_DeleteState(t *testing.T) {
	db := setupDB(t)

	// enable historical state representation flag to test this
	resetCfg := features.InitWithReset(&features.Flags{
		EnableHistoricalSpaceRepresentation: true,
	})
	defer resetCfg()

	r1 := [32]byte{'A'}
	r2 := [32]byte{'B'}

	require.Equal(t, false, db.HasState(context.Background(), r1))
	require.Equal(t, false, db.HasState(context.Background(), r2))

	// create two states with the same set of validators.
	stateValidators := validators(10)
	st1, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st1.SetSlot(100))
	require.NoError(t, st1.SetValidators(stateValidators))

	st2, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st2.SetSlot(101))
	require.NoError(t, st2.SetValidators(stateValidators))

	// save both the states.
	ctx := context.Background()
	require.NoError(t, db.SaveState(ctx, st1, r1))
	require.NoError(t, db.SaveState(ctx, st2, r2))

	// delete the first state.
	var deleteBlockRoots [][32]byte
	deleteBlockRoots = append(deleteBlockRoots, r1)
	require.NoError(t, db.DeleteStates(ctx, deleteBlockRoots))

	// check if the validator entries of this state is removed from cache.
	for _, val := range stateValidators {
		hash, hashErr := val.HashTreeRoot()
		assert.NoError(t, hashErr)
		v, found := db.validatorEntryCache.Get(hash[:])
		require.Equal(t, false, found)
		require.Equal(t, nil, v)
	}

	// check if the index of the first state is deleted.
	err = db.db.Update(func(tx *bolt.Tx) error {
		idxBkt := tx.Bucket(blockRootValidatorHashesBucket)
		data := idxBkt.Get(r1[:])
		require.Equal(t, 0, len(data))
		return nil
	})
	require.NoError(t, err)

	// check if the index of the second state is still present.
	err = db.db.Update(func(tx *bolt.Tx) error {
		idxBkt := tx.Bucket(blockRootValidatorHashesBucket)
		data := idxBkt.Get(r2[:])
		require.NotEqual(t, 0, len(data))
		return nil
	})
	require.NoError(t, err)

	// check if all the validator entries are still intact in the validator entry bucket.
	err = db.db.Update(func(tx *bolt.Tx) error {
		valBkt := tx.Bucket(stateValidatorsBucket)
		// if any of the original validator entry is not present, then fail the test.
		for _, val := range stateValidators {
			hash, hashErr := val.HashTreeRoot()
			assert.NoError(t, hashErr)
			data := valBkt.Get(hash[:])
			require.NotNil(t, data)
			require.NotEqual(t, 0, len(data))
		}
		return nil
	})
	require.NoError(t, err)
}

func TestGenesisState_CanSaveRetrieve(t *testing.T) {
	db := setupDB(t)

	headRoot := [32]byte{'B'}

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(1))
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), headRoot))
	require.NoError(t, db.SaveState(context.Background(), st, headRoot))

	savedGenesisS, err := db.GenesisState(context.Background())
	require.NoError(t, err)
	assert.DeepSSZEqual(t, st.ToProtoUnsafe(), savedGenesisS.ToProtoUnsafe(), "Did not retrieve saved state")
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), [32]byte{'C'}))
}

func TestStore_StatesBatchDelete(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	numBlocks := 100
	totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, numBlocks)
	blockRoots := make([][32]byte, 0)
	evenBlockRoots := make([][32]byte, 0)
	for i := range totalBlocks {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = primitives.Slot(i)
		var err error
		totalBlocks[i], err = blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := totalBlocks[i].Block().HashTreeRoot()
		require.NoError(t, err)
		st, err := util.NewBeaconStateZond()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(primitives.Slot(i)))
		require.NoError(t, db.SaveState(context.Background(), st, r))
		blockRoots = append(blockRoots, r)
		if i%2 == 0 {
			evenBlockRoots = append(evenBlockRoots, r)
		}
	}
	require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
	// We delete all even indexed states.
	require.NoError(t, db.DeleteStates(ctx, evenBlockRoots))
	// When we retrieve the data, only the odd indexed state should remain.
	for _, r := range blockRoots {
		s, err := db.State(context.Background(), r)
		require.NoError(t, err)
		if s == nil {
			continue
		}
		assert.Equal(t, primitives.Slot(1), s.Slot()%2, "State with slot %d should have been deleted", s.Slot())
	}
}

func TestStore_DeleteGenesisState(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	genesisBlockRoot := [32]byte{'A'}
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisBlockRoot))
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(100))
	require.NoError(t, db.SaveState(ctx, st, genesisBlockRoot))
	wantedErr := "cannot delete finalized block or state"
	assert.ErrorContains(t, wantedErr, db.DeleteState(ctx, genesisBlockRoot))
}

func TestStore_DeleteFinalizedState(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	genesis := bytesutil.ToBytes32([]byte{'G', 'E', 'N', 'E', 'S', 'I', 'S'})
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesis))

	blk := util.NewBeaconBlockZond()
	blk.Block.ParentRoot = genesis[:]
	blk.Block.Slot = 100

	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))

	finalizedBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	finalizedState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, finalizedState.SetSlot(100))
	require.NoError(t, db.SaveState(ctx, finalizedState, finalizedBlockRoot))
	finalizedCheckpoint := &qrysmpb.Checkpoint{Root: finalizedBlockRoot[:]}
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, finalizedCheckpoint))
	wantedErr := "cannot delete finalized block or state"
	assert.ErrorContains(t, wantedErr, db.DeleteState(ctx, finalizedBlockRoot))
}

func TestStore_DeleteHeadState(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	genesis := bytesutil.ToBytes32([]byte{'G', 'E', 'N', 'E', 'S', 'I', 'S'})
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesis))

	blk := util.NewBeaconBlockZond()
	blk.Block.ParentRoot = genesis[:]
	blk.Block.Slot = 100
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))

	headBlockRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(100))
	require.NoError(t, db.SaveState(ctx, st, headBlockRoot))
	require.NoError(t, db.SaveHeadBlockRoot(ctx, headBlockRoot))
	require.NoError(t, db.DeleteState(ctx, headBlockRoot)) // Ok to delete head state if it's optimistic.
}

func TestStore_SaveDeleteState_CanGetHighestBelow(t *testing.T) {
	db := setupDB(t)

	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(context.Background(), wsb))
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(1))
	s0 := st.ToProtoUnsafe()
	require.NoError(t, db.SaveState(context.Background(), st, r))

	b.Block.Slot = 100
	r1, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err = blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(context.Background(), wsb))
	st, err = util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(100))
	s1 := st.ToProtoUnsafe()
	require.NoError(t, db.SaveState(context.Background(), st, r1))

	b.Block.Slot = 1000
	r2, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err = blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(context.Background(), wsb))
	st, err = util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(1000))
	s2 := st.ToProtoUnsafe()

	require.NoError(t, db.SaveState(context.Background(), st, r2))

	highest, err := db.HighestSlotStatesBelow(context.Background(), 2)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, highest[0].ToProtoUnsafe(), s0)

	highest, err = db.HighestSlotStatesBelow(context.Background(), 101)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, highest[0].ToProtoUnsafe(), s1)

	highest, err = db.HighestSlotStatesBelow(context.Background(), 1001)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, highest[0].ToProtoUnsafe(), s2)
}

func TestStore_GenesisState_CanGetHighestBelow(t *testing.T) {
	db := setupDB(t)

	genesisState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	genesisRoot := [32]byte{'a'}
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), genesisRoot))
	require.NoError(t, db.SaveState(context.Background(), genesisState, genesisRoot))

	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(context.Background(), wsb))

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(1))
	require.NoError(t, db.SaveState(context.Background(), st, r))

	highest, err := db.HighestSlotStatesBelow(context.Background(), 2)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, highest[0].ToProtoUnsafe(), st.ToProtoUnsafe())

	highest, err = db.HighestSlotStatesBelow(context.Background(), 1)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, highest[0].ToProtoUnsafe(), genesisState.ToProtoUnsafe())
	highest, err = db.HighestSlotStatesBelow(context.Background(), 0)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, highest[0].ToProtoUnsafe(), genesisState.ToProtoUnsafe())
}

func TestStore_CleanUpDirtyStates_AboveThreshold(t *testing.T) {
	db := setupDB(t)

	genesisState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	genesisRoot := [32]byte{'a'}
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), genesisRoot))
	require.NoError(t, db.SaveState(context.Background(), genesisState, genesisRoot))
	require.NoError(t, db.SaveOriginCheckpointBlockRoot(context.Background(), [32]byte{'a'}))

	bRoots := make([][32]byte, 0)
	slotsPerArchivedPoint := primitives.Slot(128)
	prevRoot := genesisRoot
	for i := primitives.Slot(1); i <= slotsPerArchivedPoint; i++ {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = i
		b.Block.ParentRoot = prevRoot[:]
		r, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		require.NoError(t, db.SaveBlock(context.Background(), wsb))
		bRoots = append(bRoots, r)
		prevRoot = r

		st, err := util.NewBeaconStateZond()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(i))
		require.NoError(t, db.SaveState(context.Background(), st, r))
	}

	require.NoError(t, db.SaveFinalizedCheckpoint(context.Background(), &qrysmpb.Checkpoint{
		Root:  bRoots[len(bRoots)-1][:],
		Epoch: primitives.Epoch(slotsPerArchivedPoint / params.BeaconConfig().SlotsPerEpoch),
	}))
	require.NoError(t, db.CleanUpDirtyStates(context.Background(), slotsPerArchivedPoint))

	for i, root := range bRoots {
		if primitives.Slot(i) >= slotsPerArchivedPoint.SubSlot(slotsPerArchivedPoint.Div(3)) {
			require.Equal(t, true, db.HasState(context.Background(), root))
		} else {
			require.Equal(t, false, db.HasState(context.Background(), root))
		}
	}
}

// TestStore_CleanUpDirtyStates_NoOriginRoot exercises the genesis-start
// path (no checkpoint sync, so no origin block root saved). Prior to
// upstream PR 15808, CleanUpDirtyStates returned an error on every call,
// silently aborting state pruning for nodes that never checkpoint-synced.
func TestStore_CleanUpDirtyStates_NoOriginRoot(t *testing.T) {
	db := setupDB(t)
	genesisState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	genesisRoot := [32]byte{'a'}
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), genesisRoot))
	require.NoError(t, db.SaveState(context.Background(), genesisState, genesisRoot))
	// Deliberately do NOT call SaveOriginCheckpointBlockRoot — simulates a
	// node started from genesis rather than via checkpoint sync.

	slotsPerArchivedPoint := primitives.Slot(128)
	bRoots := make([][32]byte, 0)
	prevRoot := genesisRoot
	for i := primitives.Slot(1); i <= slotsPerArchivedPoint; i++ {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = i
		b.Block.ParentRoot = prevRoot[:]
		r, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		require.NoError(t, db.SaveBlock(context.Background(), wsb))
		bRoots = append(bRoots, r)
		prevRoot = r

		st, err := util.NewBeaconStateZond()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(i))
		require.NoError(t, db.SaveState(context.Background(), st, r))
	}

	require.NoError(t, db.SaveFinalizedCheckpoint(context.Background(), &qrysmpb.Checkpoint{
		Root:  bRoots[len(bRoots)-1][:],
		Epoch: primitives.Epoch(slotsPerArchivedPoint / params.BeaconConfig().SlotsPerEpoch),
	}))
	// Must not fail even though origin block root is not set.
	require.NoError(t, db.CleanUpDirtyStates(context.Background(), slotsPerArchivedPoint))

	// Cleanup still works: only the "above threshold" tail of recent states is kept.
	for i, root := range bRoots {
		if primitives.Slot(i) >= slotsPerArchivedPoint.SubSlot(slotsPerArchivedPoint.Div(3)) {
			require.Equal(t, true, db.HasState(context.Background(), root))
		} else {
			require.Equal(t, false, db.HasState(context.Background(), root))
		}
	}
}

// The archive of an archived point with no block at that slot is the state of
// the highest canonical block below it, wherever in the interval that block
// sits. The cleanup must keep it: once the migration has passed the point the
// archive is never written again.
func TestStore_CleanUpDirtyStates_KeepsArchiveOfSkippedPoint(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	genesisState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	genesisRoot := [32]byte{'a'}
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisRoot))
	require.NoError(t, db.SaveState(ctx, genesisState, genesisRoot))

	saveBlockAndState := func(slot primitives.Slot, parent [32]byte) [32]byte {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = slot
		b.Block.ParentRoot = parent[:]
		r, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		require.NoError(t, db.SaveBlock(ctx, wsb))
		st, err := util.NewBeaconStateZond()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(slot))
		require.NoError(t, db.SaveState(ctx, st, r))
		return r
	}
	// Blocks at 1..64 and 130: the archived slot 128 is served by block 64, in
	// the first half of the interval, which the interval rules alone discard.
	slotsPerArchivedPoint := primitives.Slot(128)
	roots := map[primitives.Slot][32]byte{}
	parent := genesisRoot
	for i := primitives.Slot(1); i <= 64; i++ {
		parent = saveBlockAndState(i, parent)
		roots[i] = parent
	}
	fRoot := saveBlockAndState(130, parent)
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{
		Root:  fRoot[:],
		Epoch: primitives.Epoch(primitives.Slot(130) / params.BeaconConfig().SlotsPerEpoch),
	}))

	require.NoError(t, db.CleanUpDirtyStates(ctx, slotsPerArchivedPoint))

	for i := primitives.Slot(1); i < 64; i++ {
		require.Equal(t, false, db.HasState(ctx, roots[i]), "the state at slot %d is not an archive", i)
	}
	require.Equal(t, true, db.HasState(ctx, roots[64]), "the archive of slot 128 (the state of block 64) must be kept")
	require.Equal(t, true, db.HasState(ctx, fRoot))
	require.Equal(t, true, db.HasState(ctx, genesisRoot))
}

func TestStore_CleanUpDirtyStates_Finalized(t *testing.T) {
	db := setupDB(t)

	genesisState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	genesisRoot := [32]byte{'a'}
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), genesisRoot))
	require.NoError(t, db.SaveState(context.Background(), genesisState, genesisRoot))
	require.NoError(t, db.SaveOriginCheckpointBlockRoot(context.Background(), [32]byte{'a'}))

	for i := primitives.Slot(1); i <= params.BeaconConfig().SlotsPerEpoch; i++ {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = i
		r, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		require.NoError(t, db.SaveBlock(context.Background(), wsb))

		st, err := util.NewBeaconStateZond()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(i))
		require.NoError(t, db.SaveState(context.Background(), st, r))
	}

	require.NoError(t, db.SaveFinalizedCheckpoint(context.Background(), &qrysmpb.Checkpoint{Root: genesisRoot[:]}))
	require.NoError(t, db.CleanUpDirtyStates(context.Background(), params.BeaconConfig().SlotsPerEpoch))
	require.Equal(t, true, db.HasState(context.Background(), genesisRoot))
}

func TestStore_CleanUpDirtyStates_OriginRoot(t *testing.T) {
	db := setupDB(t)

	genesisState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	r := [32]byte{'a'}
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), r))
	require.NoError(t, db.SaveState(context.Background(), genesisState, r))

	for i := primitives.Slot(1); i <= params.BeaconConfig().SlotsPerEpoch; i++ {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = i
		r, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		require.NoError(t, db.SaveBlock(context.Background(), wsb))

		st, err := util.NewBeaconStateZond()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(i))
		require.NoError(t, db.SaveState(context.Background(), st, r))
	}

	require.NoError(t, db.SaveOriginCheckpointBlockRoot(context.Background(), r))
	require.NoError(t, db.CleanUpDirtyStates(context.Background(), params.BeaconConfig().SlotsPerEpoch))
	require.Equal(t, true, db.HasState(context.Background(), r))
}

func TestStore_CleanUpDirtyStates_DontDeleteNonFinalized(t *testing.T) {
	db := setupDB(t)

	genesisState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	genesisRoot := [32]byte{'a'}
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), genesisRoot))
	require.NoError(t, db.SaveState(context.Background(), genesisState, genesisRoot))
	require.NoError(t, db.SaveOriginCheckpointBlockRoot(context.Background(), [32]byte{'a'}))

	var unfinalizedRoots [][32]byte
	for i := primitives.Slot(1); i <= params.BeaconConfig().SlotsPerEpoch; i++ {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = i
		r, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		require.NoError(t, db.SaveBlock(context.Background(), wsb))
		unfinalizedRoots = append(unfinalizedRoots, r)

		st, err := util.NewBeaconStateZond()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(i))
		require.NoError(t, db.SaveState(context.Background(), st, r))
	}

	require.NoError(t, db.SaveFinalizedCheckpoint(context.Background(), &qrysmpb.Checkpoint{Root: genesisRoot[:]}))
	require.NoError(t, db.CleanUpDirtyStates(context.Background(), params.BeaconConfig().SlotsPerEpoch))

	for _, rt := range unfinalizedRoots {
		require.Equal(t, true, db.HasState(context.Background(), rt))
	}
}

func validators(limit int) []*qrysmpb.Validator {
	var vals []*qrysmpb.Validator
	for i := range limit {
		pubKey := make([]byte, field_params.MLDSA87PubkeyLength)
		binary.LittleEndian.PutUint64(pubKey, rand.Uint64())
		val := &qrysmpb.Validator{
			PublicKey:                  pubKey,
			WithdrawalRecipient:        bytesutil.ToBytes(rand.Uint64(), field_params.WithdrawalRecipientLength),
			EffectiveBalance:           rand.Uint64(),
			Slashed:                    i%2 != 0,
			ActivationEligibilityEpoch: primitives.Epoch(rand.Uint64()),
			ActivationEpoch:            primitives.Epoch(rand.Uint64()),
			ExitEpoch:                  primitives.Epoch(rand.Uint64()),
			WithdrawableEpoch:          primitives.Epoch(rand.Uint64()),
		}
		vals = append(vals, val)
	}
	return vals
}

func checkStateSaveTime(b *testing.B, saveCount int) {
	db := setupDB(b)
	initialSetOfValidators := validators(100000)

	// construct some states and save to randomize benchmark.
	for range saveCount {
		key := make([]byte, 32)
		_, err := rand.Read(key)
		require.NoError(b, err)
		st, err := util.NewBeaconStateZond()
		require.NoError(b, err)

		// Add some more new validator to the base validator.
		validatosToAddInTest := validators(10000)
		allValidators := append(initialSetOfValidators, validatosToAddInTest...)

		// shuffle validators.
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(allValidators), func(i, j int) { allValidators[i], allValidators[j] = allValidators[j], allValidators[i] })

		require.NoError(b, st.SetValidators(allValidators))
		require.NoError(b, db.SaveState(context.Background(), st, bytesutil.ToBytes32(key)))
	}

	// create a state to save in benchmark
	r := [32]byte{'A'}
	st, err := util.NewBeaconStateZond()
	require.NoError(b, err)
	require.NoError(b, st.SetValidators(initialSetOfValidators))

	b.ReportAllocs()
	for b.Loop() {
		require.NoError(b, db.SaveState(context.Background(), st, r))
	}
}

func checkStateReadTime(b *testing.B, saveCount int) {
	db := setupDB(b)
	initialSetOfValidators := validators(100000)

	// Save a state to read in benchmark
	r := [32]byte{'A'}
	st, err := util.NewBeaconStateZond()
	require.NoError(b, err)
	require.NoError(b, st.SetValidators(initialSetOfValidators))
	require.NoError(b, db.SaveState(context.Background(), st, r))

	// construct some states and save to randomize benchmark.
	for range saveCount {
		key := make([]byte, 32)
		_, err := rand.Read(key)
		require.NoError(b, err)
		st, err = util.NewBeaconStateZond()
		require.NoError(b, err)

		// Add some more new validator to the base validator.
		validatosToAddInTest := validators(10000)
		allValidators := append(initialSetOfValidators, validatosToAddInTest...)

		// shuffle validators.
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(allValidators), func(i, j int) { allValidators[i], allValidators[j] = allValidators[j], allValidators[i] })

		require.NoError(b, st.SetValidators(allValidators))
		require.NoError(b, db.SaveState(context.Background(), st, bytesutil.ToBytes32(key)))
	}

	b.ReportAllocs()
	for b.Loop() {
		_, err := db.State(context.Background(), r)
		require.NoError(b, err)
	}
}

func TestStateZond_CanSaveRetrieveValidatorEntries(t *testing.T) {
	db := setupDB(t)

	// enable historical state representation flag to test this
	resetCfg := features.InitWithReset(&features.Flags{
		EnableHistoricalSpaceRepresentation: true,
	})
	defer resetCfg()

	r := [32]byte{'A'}

	require.Equal(t, false, db.HasState(context.Background(), r))

	stateValidators := validators(10)
	st, _ := util.DeterministicGenesisStateZond(t, 20)
	require.NoError(t, st.SetSlot(100))
	require.NoError(t, st.SetValidators(stateValidators))

	ctx := context.Background()
	require.NoError(t, db.SaveState(ctx, st, r))
	assert.Equal(t, true, db.HasState(context.Background(), r))

	savedS, err := db.State(context.Background(), r)
	require.NoError(t, err)

	require.DeepSSZEqual(t, st.ToProtoUnsafe(), savedS.ToProtoUnsafe(), "saved state with validators and retrieved state are not matching")

	// check if the index of the second state is still present.
	err = db.db.Update(func(tx *bolt.Tx) error {
		idxBkt := tx.Bucket(blockRootValidatorHashesBucket)
		data := idxBkt.Get(r[:])
		require.NotEqual(t, 0, len(data))
		return nil
	})
	require.NoError(t, err)

	// check if all the validator entries are still intact in the validator entry bucket.
	err = db.db.Update(func(tx *bolt.Tx) error {
		valBkt := tx.Bucket(stateValidatorsBucket)
		// if any of the original validator entry is not present, then fail the test.
		for _, val := range stateValidators {
			hash, hashErr := val.HashTreeRoot()
			assert.NoError(t, hashErr)
			data := valBkt.Get(hash[:])
			require.NotNil(t, data)
			require.NotEqual(t, 0, len(data))
		}
		return nil
	})
	require.NoError(t, err)
}

func TestZondState_CanSaveRetrieve(t *testing.T) {
	db := setupDB(t)

	r := [32]byte{'A'}

	require.Equal(t, false, db.HasState(context.Background(), r))

	st, _ := util.DeterministicGenesisStateZond(t, 1)
	require.NoError(t, st.SetSlot(100))

	require.NoError(t, db.SaveState(context.Background(), st, r))
	require.Equal(t, true, db.HasState(context.Background(), r))

	savedS, err := db.State(context.Background(), r)
	require.NoError(t, err)

	require.DeepSSZEqual(t, st.ToProtoUnsafe(), savedS.ToProtoUnsafe())

	savedS, err = db.State(context.Background(), [32]byte{'B'})
	require.NoError(t, err)
	require.Equal(t, state.ReadOnlyBeaconState(nil), savedS, "Unsaved state should've been nil")
}

func TestZondState_CanDelete(t *testing.T) {
	db := setupDB(t)

	r := [32]byte{'A'}

	require.Equal(t, false, db.HasState(context.Background(), r))

	st, _ := util.DeterministicGenesisStateZond(t, 1)
	require.NoError(t, st.SetSlot(100))

	require.NoError(t, db.SaveState(context.Background(), st, r))
	require.Equal(t, true, db.HasState(context.Background(), r))

	require.NoError(t, db.DeleteState(context.Background(), r))
	savedS, err := db.State(context.Background(), r)
	require.NoError(t, err)
	require.Equal(t, state.ReadOnlyBeaconState(nil), savedS, "Unsaved state should've been nil")
}

func BenchmarkState_CheckStateSaveTime_1(b *testing.B)  { checkStateSaveTime(b, 1) }
func BenchmarkState_CheckStateSaveTime_10(b *testing.B) { checkStateSaveTime(b, 10) }

func BenchmarkState_CheckStateReadTime_1(b *testing.B)  { checkStateReadTime(b, 1) }
func BenchmarkState_CheckStateReadTime_10(b *testing.B) { checkStateReadTime(b, 10) }

func TestStore_CleanUpDirtyStates_EveryRootAtSlot(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	genesisState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	genesisRoot := [32]byte{'a'}
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisRoot))
	require.NoError(t, db.SaveState(ctx, genesisState, genesisRoot))

	// Two states at the same slot: the chain forked at slot 5.
	forkState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, forkState.SetSlot(5))
	rA, rB := [32]byte{'b'}, [32]byte{'c'}
	require.NoError(t, db.SaveState(ctx, forkState, rA))
	require.NoError(t, db.SaveState(ctx, forkState, rB))

	fb := util.NewBeaconBlockZond()
	fb.Block.Slot = 64
	fb.Block.ParentRoot = genesisRoot[:]
	wfb, err := blocks.NewSignedBeaconBlock(fb)
	require.NoError(t, err)
	fRoot, err := wfb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wfb))
	finalizedState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, finalizedState.SetSlot(64))
	require.NoError(t, db.SaveState(ctx, finalizedState, fRoot))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 2, Root: fRoot[:]}))

	require.NoError(t, db.CleanUpDirtyStates(ctx, 128))
	require.Equal(t, false, db.HasState(ctx, rA))
	require.Equal(t, false, db.HasState(ctx, rB), "the second state at the slot must be cleaned up too")
	require.Equal(t, true, db.HasState(ctx, fRoot))
	require.Equal(t, true, db.HasState(ctx, genesisRoot))
}

func TestGenesisState_NoNestedReadTransaction(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ConfigName = "no-embedded-genesis"
	params.OverrideBeaconConfig(cfg)
	resetCfg := features.InitWithReset(&features.Flags{EnableHistoricalSpaceRepresentation: false})
	defer resetCfg()

	ctx := context.Background()
	dir := t.TempDir()
	// bbolt's default mmap sizing doubles from 32KB as the file grows, so a
	// small write load produces many remap events.
	boltDB, err := bolt.Open(path.Join(dir, DatabaseFileName), params.BeaconIoConfig().ReadWritePermissions, &bolt.Options{Timeout: time.Second})
	require.NoError(t, err)
	blockCache, err := ristretto.NewCache(&ristretto.Config{NumCounters: 1000, MaxCost: BlockCacheSize, BufferItems: 64})
	require.NoError(t, err)
	valCache, err := ristretto.NewCache(&ristretto.Config{NumCounters: NumOfValidatorEntries, MaxCost: ValidatorEntryMaxCost, BufferItems: 64})
	require.NoError(t, err)
	store := &Store{db: boltDB, databasePath: dir, blockCache: blockCache, validatorEntryCache: valCache, stateSummaryCache: newStateSummaryCache(), ctx: ctx}
	require.NoError(t, boltDB.Update(func(tx *bolt.Tx) error { return createBuckets(tx, Buckets...) }))

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	gRoot := [32]byte{'g'}
	require.NoError(t, store.SaveState(ctx, st, gRoot))
	require.NoError(t, store.SaveGenesisBlockRoot(ctx, gRoot))
	got, err := store.GenesisState(ctx)
	require.NoError(t, err)
	require.Equal(t, false, got == nil || got.IsNil(), "genesis must come from the DB in this test")

	readerDone := make(chan error, 1)
	writerDone := make(chan error, 1)
	go func() {
		for i := 0; i < 1500; i++ {
			if _, err := store.GenesisState(ctx); err != nil {
				readerDone <- err
				return
			}
		}
		readerDone <- nil
	}()
	go func() {
		val := make([]byte, 64*1024)
		for i := 0; i < 200; i++ {
			if _, err := crand.Read(val); err != nil {
				writerDone <- err
				return
			}
			err := boltDB.Update(func(tx *bolt.Tx) error {
				return tx.Bucket(chainMetadataBucket).Put([]byte{'w', byte(i >> 8), byte(i)}, val)
			})
			if err != nil {
				writerDone <- err
				return
			}
		}
		writerDone <- nil
	}()

	deadline := time.After(60 * time.Second)
	for done := 0; done < 2; done++ {
		select {
		case err := <-readerDone:
			require.NoError(t, err)
		case err := <-writerDone:
			require.NoError(t, err)
		case <-deadline:
			t.Fatal("deadlock between GenesisState and a writer that remaps the database")
		}
	}
	require.NoError(t, boltDB.Close())
}

// An interrupted validator-schema migration followed by a restart without the
// flag used to serve the already migrated states without validators. The
// migration now resumes whenever migrated states exist, flag or not.

// Several states can be saved at one slot (a fork). The highest-slot lookup
// must return all of them, not the first root of the slot's packed list.
func TestStore_HighestSlotStatesBelow_ReturnsEveryStateAtTheSlot(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	for _, root := range [][32]byte{{'a'}, {'b'}} {
		st, err := util.NewBeaconStateZond()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(10))
		require.NoError(t, st.SetGenesisValidatorsRoot(root[:])) // tell the two states apart
		require.NoError(t, db.SaveState(ctx, st, root))
	}
	highest, err := db.HighestSlotStatesBelow(ctx, 11)
	require.NoError(t, err)
	require.Equal(t, 2, len(highest), "both states at slot 10 must be returned")
	require.NotEqual(t, highest[0].GenesisValidatorsRoot()[0], highest[1].GenesisValidatorsRoot()[0])
}

// A state saved again under the same root at another slot must not leave its
// old slot-index entry behind: the dirty-state cleanup follows index entries
// and deletes by root, so a stale entry would take the live state with it.
func TestStore_SaveState_ReplacedAtAnotherSlotMovesTheSlotIndex(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	root := [32]byte{'r'}
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(10))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 10, Root: root[:]}))
	require.NoError(t, db.SaveState(ctx, st, root))

	replacement := st.Copy()
	require.NoError(t, replacement.SetSlot(12))
	require.NoError(t, db.SaveState(ctx, replacement, root))

	indexed := func(slot primitives.Slot) [][32]byte {
		var roots [][32]byte
		require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
			v := tx.Bucket(stateSlotIndicesBucket).Get(bytesutil.SlotToBytesBigEndian(slot))
			if len(v) == 0 {
				return nil
			}
			var err error
			roots, err = splitRoots(v)
			return err
		}))
		return roots
	}
	require.Equal(t, 0, len(indexed(10)), "the old slot must no longer index the root")
	require.DeepEqual(t, [][32]byte{root}, indexed(12))
}

// A zero archive interval must be refused instead of dividing by zero.
func TestStore_CleanUpDirtyStates_ZeroIntervalIsAnError(t *testing.T) {
	db := setupDB(t)
	require.ErrorContains(t, "must be positive", db.CleanUpDirtyStates(context.Background(), 0))
}
