package kv

import (
	"context"
	"testing"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

var genesisBlockRoot = bytesutil.ToBytes32([]byte{'G', 'E', 'N', 'E', 'S', 'I', 'S'})

func TestStore_IsFinalizedBlock(t *testing.T) {
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	db := setupDB(t)
	ctx := context.Background()

	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisBlockRoot))

	blks := makeBlocksZond(t, 0, slotsPerEpoch*3, genesisBlockRoot)
	require.NoError(t, db.SaveBlocks(ctx, blks))

	root, err := blks[slotsPerEpoch].Block().HashTreeRoot()
	require.NoError(t, err)

	cp := &qrysmpb.Checkpoint{
		Epoch: 1,
		Root:  root[:],
	}

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	// a state is required to save checkpoint
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, cp))

	// blks[i] is at slot i+1. All blocks of epochs 0 and 1 (slots 1 to
	// 2*slotsPerEpoch-1) should be in the finalized index.
	for i := uint64(0); i < slotsPerEpoch*2-1; i++ {
		root, err := blks[i].Block().HashTreeRoot()
		require.NoError(t, err)
		assert.Equal(t, true, db.IsFinalizedBlock(ctx, root), "Block at index %d was not considered finalized in the index", i)
	}
	// Blocks of epoch 2 and later are not finalized in any sense; in
	// particular the epoch after the checkpoint epoch must not be marked.
	for i := slotsPerEpoch*2 - 1; i < uint64(len(blks)); i++ {
		root, err := blks[i].Block().HashTreeRoot()
		require.NoError(t, err)
		assert.Equal(t, false, db.IsFinalizedBlock(ctx, root), "Block at index %d was considered finalized in the index, but should not have", i)
	}
}

func TestStore_IsFinalizedBlockGenesis(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = 0
	root, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))
	assert.Equal(t, true, db.IsFinalizedBlock(ctx, root), "Finalized genesis block doesn't exist in db")
}

// This test scenario is to test a specific edge case where the finalized block root is not part of
// the finalized and canonical chain.
//
// Example:
// 0    1  2  3   4     5   6     slot
// a <- b <-- d <- e <- f <- g    roots
//
//	^- c
//
// Imagine that epochs are 2 slots and that epoch 1, 2, and 3 are finalized. Checkpoint roots would
// be c, e, and g. In this scenario, c was a finalized checkpoint root but no block built upon it so
// it should not be considered "final and canonical" in the view at slot 6.
func TestStore_IsFinalized_ForkEdgeCase(t *testing.T) {
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	blocks0 := makeBlocksZond(t, slotsPerEpoch*0, slotsPerEpoch, genesisBlockRoot)
	blocks1 := append(
		makeBlocksZond(t, slotsPerEpoch*1, 1, bytesutil.ToBytes32(sszRootOrDie(t, blocks0[len(blocks0)-1]))), // No block builds off of the first block in epoch.
		makeBlocksZond(t, slotsPerEpoch*1+1, slotsPerEpoch-1, bytesutil.ToBytes32(sszRootOrDie(t, blocks0[len(blocks0)-1])))...,
	)
	blocks2 := makeBlocksZond(t, slotsPerEpoch*2, slotsPerEpoch, bytesutil.ToBytes32(sszRootOrDie(t, blocks1[len(blocks1)-1])))

	db := setupDB(t)
	ctx := context.Background()

	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisBlockRoot))
	require.NoError(t, db.SaveBlocks(ctx, blocks0))
	require.NoError(t, db.SaveBlocks(ctx, blocks1))
	require.NoError(t, db.SaveBlocks(ctx, blocks2))

	// First checkpoint
	checkpoint1 := &qrysmpb.Checkpoint{
		Root:  sszRootOrDie(t, blocks1[0]),
		Epoch: 1,
	}

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	// A state is required to save checkpoint
	require.NoError(t, db.SaveState(ctx, st, bytesutil.ToBytes32(checkpoint1.Root)))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, checkpoint1))
	// All blocks in blocks0 and blocks1 should be finalized and canonical,
	// except the last block of blocks1 (slot 2*slotsPerEpoch), which is in the
	// epoch after the checkpoint epoch and therefore not finalized yet.
	for i, block := range append(blocks0, blocks1[:len(blocks1)-1]...) {
		root := sszRootOrDie(t, block)
		assert.Equal(t, true, db.IsFinalizedBlock(ctx, bytesutil.ToBytes32(root)), "%d - Expected block %#x to be finalized", i, root)
	}
	assert.Equal(t, false, db.IsFinalizedBlock(ctx, bytesutil.ToBytes32(sszRootOrDie(t, blocks1[len(blocks1)-1]))), "a block of the epoch after the checkpoint epoch must not be finalized")

	// Second checkpoint
	checkpoint2 := &qrysmpb.Checkpoint{
		Root:  sszRootOrDie(t, blocks2[0]),
		Epoch: 2,
	}
	// A state is required to save checkpoint
	require.NoError(t, db.SaveState(ctx, st, bytesutil.ToBytes32(checkpoint2.Root)))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, checkpoint2))
	// All blocks in blocks0 and blocks2 should be finalized and canonical,
	// except the last block of blocks2 (slot 3*slotsPerEpoch, epoch 3).
	for i, block := range append(blocks0, blocks2[:len(blocks2)-1]...) {
		root := sszRootOrDie(t, block)
		assert.Equal(t, true, db.IsFinalizedBlock(ctx, bytesutil.ToBytes32(root)), "%d - Expected block %#x to be finalized", i, root)
	}
	assert.Equal(t, false, db.IsFinalizedBlock(ctx, bytesutil.ToBytes32(sszRootOrDie(t, blocks2[len(blocks2)-1]))), "a block of the epoch after the checkpoint epoch must not be finalized")
	// All blocks in blocks1 should be finalized and canonical, except blocks1[0].
	for i, block := range blocks1 {
		root := sszRootOrDie(t, block)
		if db.IsFinalizedBlock(ctx, bytesutil.ToBytes32(root)) == (i == 0) {
			t.Errorf("Expected db.IsFinalizedBlock(ctx, blocks1[%d]) to be %v", i, i != 0)
		}
	}
}

func TestStore_IsFinalizedChildBlock(t *testing.T) {
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	ctx := context.Background()

	eval := func(t testing.TB, ctx context.Context, db *Store, blks []interfaces.ReadOnlySignedBeaconBlock) {
		require.NoError(t, db.SaveBlocks(ctx, blks))
		root, err := blks[slotsPerEpoch].Block().HashTreeRoot()
		require.NoError(t, err)

		cp := &qrysmpb.Checkpoint{
			Epoch: 1,
			Root:  root[:],
		}

		st, err := util.NewBeaconStateZond()
		require.NoError(t, err)
		// a state is required to save checkpoint
		require.NoError(t, db.SaveState(ctx, st, root))
		require.NoError(t, db.SaveFinalizedCheckpoint(ctx, cp))

		// All blocks up to slotsPerEpoch should have a finalized child block.
		for i := range slotsPerEpoch {
			root, err := blks[i].Block().HashTreeRoot()
			require.NoError(t, err)
			assert.Equal(t, true, db.IsFinalizedBlock(ctx, root), "Block at index %d was not considered finalized in the index", i)
			blk, err := db.FinalizedChildBlock(ctx, root)
			assert.NoError(t, err)
			if blk == nil {
				t.Error("Child block doesn't exist for valid finalized block.")
			}
		}
	}

	setup := func(t testing.TB) *Store {
		db := setupDB(t)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisBlockRoot))

		return db
	}

	t.Run("zond", func(t *testing.T) {
		db := setup(t)

		blks := makeBlocksZond(t, 0, slotsPerEpoch*3, genesisBlockRoot)
		eval(t, ctx, db, blks)
	})
}

func sszRootOrDie(t *testing.T, block interfaces.ReadOnlySignedBeaconBlock) []byte {
	root, err := block.Block().HashTreeRoot()
	require.NoError(t, err)
	return root[:]
}

func makeBlocksZond(t *testing.T, i, n uint64, previousRoot [32]byte) []interfaces.ReadOnlySignedBeaconBlock {
	blocks := make([]*qrysmpb.SignedBeaconBlockZond, n)
	ifaceBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, n)
	for j := i; j < n+i; j++ {
		parentRoot := make([]byte, fieldparams.RootLength)
		copy(parentRoot, previousRoot[:])
		blocks[j-i] = util.NewBeaconBlockZond()
		blocks[j-i].Block.Slot = primitives.Slot(j + 1)
		blocks[j-i].Block.ParentRoot = parentRoot
		var err error
		previousRoot, err = blocks[j-i].Block.HashTreeRoot()
		require.NoError(t, err)
		ifaceBlocks[j-i], err = consensusblocks.NewSignedBeaconBlock(blocks[j-i])
		require.NoError(t, err)
	}
	return ifaceBlocks
}

// The finalized index covers the checkpoint epoch, not the epoch after it:
// those blocks are not finalized in any sense, and an orphan among them must
// stay deletable by invalid-block cleanup.
func TestStore_IsFinalizedBlock_ExcludesEpochAfterCheckpoint(t *testing.T) {
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	db := setupDB(t)
	ctx := context.Background()
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisBlockRoot))
	blks := makeBlocksZond(t, 0, slotsPerEpoch*3, genesisBlockRoot)
	require.NoError(t, db.SaveBlocks(ctx, blks))

	// An orphan in epoch 2, sibling of the canonical block at its slot.
	orphanIdx := 2*slotsPerEpoch + 5
	parentRoot, err := blks[orphanIdx-1].Block().HashTreeRoot()
	require.NoError(t, err)
	orphan := util.NewBeaconBlockZond()
	orphan.Block.Slot = blks[orphanIdx].Block().Slot()
	orphan.Block.ParentRoot = parentRoot[:]
	orphan.Block.Body.Graffiti = bytesutil.PadTo([]byte("orphan"), 32)
	orphanRoot, err := orphan.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, db, orphan)

	// Finalize epoch 1 at the block of its first slot (blks[i] is at slot i+1).
	cpRoot, err := blks[slotsPerEpoch-1].Block().HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, st, cpRoot))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 1, Root: cpRoot[:]}))

	canonicalRoot, err := blks[orphanIdx].Block().HashTreeRoot()
	require.NoError(t, err)
	assert.Equal(t, false, db.IsFinalizedBlock(ctx, canonicalRoot), "epoch 2 is after the finalized epoch")
	assert.Equal(t, false, db.IsFinalizedBlock(ctx, orphanRoot), "an orphan in the epoch after finalization is not finalized")
	require.NoError(t, db.DeleteBlock(ctx, orphanRoot))
	assert.Equal(t, false, db.HasBlock(ctx, orphanRoot))
}

// Blocks of the finalized epoch after the checkpoint root only carry a sentinel
// in the index: they are reported finalized, but may still be invalidated or
// lose fork choice, so DeleteBlock must remove them (and their sentinel) while
// still refusing the finalized canonical chain.
func TestStore_DeleteBlock_FinalizedEpochSentinelIsDeletable(t *testing.T) {
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	db := setupDB(t)
	ctx := context.Background()
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisBlockRoot))
	blks := makeBlocksZond(t, 0, slotsPerEpoch*2, genesisBlockRoot)
	require.NoError(t, db.SaveBlocks(ctx, blks))

	// Finalize epoch 1 at the block of its first slot (blks[i] is at slot i+1).
	cpIdx := slotsPerEpoch - 1
	cpRoot, err := blks[cpIdx].Block().HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, st, cpRoot))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 1, Root: cpRoot[:]}))

	laterIdx := cpIdx + 5
	laterRoot, err := blks[laterIdx].Block().HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, true, db.IsFinalizedBlock(ctx, laterRoot), "a block of the finalized epoch is reported finalized")
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: blks[laterIdx].Block().Slot(), Root: laterRoot[:]}))
	require.NoError(t, db.DeleteBlock(ctx, laterRoot))
	assert.Equal(t, false, db.HasBlock(ctx, laterRoot))
	assert.Equal(t, false, db.HasStateSummary(ctx, laterRoot))
	assert.Equal(t, false, db.IsFinalizedBlock(ctx, laterRoot), "the sentinel must go with the block")

	// Canonical finalized ancestors and the checkpoint root stay protected.
	for _, idx := range []uint64{3, cpIdx} {
		r, err := blks[idx].Block().HashTreeRoot()
		require.NoError(t, err)
		require.ErrorIs(t, db.DeleteBlock(ctx, r), ErrDeleteJustifiedAndFinalized)
		assert.Equal(t, true, db.HasBlock(ctx, r))
	}
}
