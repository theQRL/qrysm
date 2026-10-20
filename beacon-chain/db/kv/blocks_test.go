package kv

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/beacon-chain/db/filters"
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
	"google.golang.org/protobuf/proto"
)

var blockTests = []struct {
	name     string
	newBlock func(primitives.Slot, []byte) (interfaces.ReadOnlySignedBeaconBlock, error)
}{
	{
		name: "zond",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBeaconBlockZond()
			b.Block.Slot = slot
			if root != nil {
				b.Block.ParentRoot = root
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
	{
		name: "zond blind",
		newBlock: func(slot primitives.Slot, root []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
			b := util.NewBlindedBeaconBlockZond()
			b.Block.Slot = slot
			if root != nil {
				b.Block.ParentRoot = root
			}
			return blocks.NewSignedBeaconBlock(b)
		},
	},
}

func TestStore_SaveBackfillBlockRoot(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()

	_, err := db.BackfillBlockRoot(ctx)
	require.ErrorIs(t, err, ErrNotFoundBackfillBlockRoot)

	var expected [32]byte
	copy(expected[:], []byte{0x23})
	err = db.SaveBackfillBlockRoot(ctx, expected)
	require.NoError(t, err)
	actual, err := db.BackfillBlockRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, expected, actual)

}

func TestStore_SaveBlock_NoDuplicates(t *testing.T) {
	// A cache too small for a block: every lookup goes to the database. The
	// size is a package variable read by every store created afterwards, so it
	// is restored (it used to be left at 256 bytes, which silently disabled the
	// block cache for the rest of the package: the library discards items
	// larger than the cache without reporting them).
	originalBlockCacheSize := BlockCacheSize
	t.Cleanup(func() { BlockCacheSize = originalBlockCacheSize })
	BlockCacheSize = 1
	slot := primitives.Slot(20)
	ctx := context.Background()

	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)

			// First we save a previous block to ensure the cache max size is reached.
			prevBlock, err := tt.newBlock(slot-1, bytesutil.PadTo([]byte{1, 2, 3}, 32))
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, prevBlock))

			blk, err := tt.newBlock(slot, bytesutil.PadTo([]byte{1, 2, 3}, 32))
			require.NoError(t, err)

			// Even with a full cache, saving new blocks should not cause
			// duplicated blocks in the DB.
			for range 100 {
				require.NoError(t, db.SaveBlock(ctx, blk))
			}

			f := filters.NewFilter().SetStartSlot(slot).SetEndSlot(slot)
			retrieved, _, err := db.Blocks(ctx, f)
			require.NoError(t, err)
			assert.Equal(t, 1, len(retrieved))
		})
	}
}

func TestStore_BlocksCRUD(t *testing.T) {
	ctx := context.Background()

	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)

			blk, err := tt.newBlock(primitives.Slot(20), bytesutil.PadTo([]byte{1, 2, 3}, 32))
			require.NoError(t, err)
			blockRoot, err := blk.Block().HashTreeRoot()
			require.NoError(t, err)

			retrievedBlock, err := db.Block(ctx, blockRoot)
			require.NoError(t, err)
			assert.DeepEqual(t, nil, retrievedBlock, "Expected nil block")

			require.NoError(t, db.SaveBlock(ctx, blk))
			assert.Equal(t, true, db.HasBlock(ctx, blockRoot), "Expected block to exist in the db")
			retrievedBlock, err = db.Block(ctx, blockRoot)
			require.NoError(t, err)
			wanted := retrievedBlock
			if _, err := retrievedBlock.PbZondBlock(); err == nil {
				wanted, err = retrievedBlock.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)
			retrievedPb, err := retrievedBlock.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, retrievedPb), "Wanted: %v, received: %v", wanted, retrievedBlock)
		})
	}
}

func TestStore_BlocksHandleZeroCase(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := context.Background()
			numBlocks := 10
			totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, numBlocks)
			for i := range totalBlocks {
				b, err := tt.newBlock(primitives.Slot(i), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				totalBlocks[i] = b
				_, err = totalBlocks[i].Block().HashTreeRoot()
				require.NoError(t, err)
			}
			require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
			zeroFilter := filters.NewFilter().SetStartSlot(0).SetEndSlot(0)
			retrieved, _, err := db.Blocks(ctx, zeroFilter)
			require.NoError(t, err)
			assert.Equal(t, 1, len(retrieved), "Unexpected number of blocks received, expected one")
		})
	}
}

func TestStore_BlocksHandleInvalidEndSlot(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := context.Background()
			numBlocks := 10
			totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, numBlocks)
			// Save blocks from slot 1 onwards.
			for i := range totalBlocks {
				b, err := tt.newBlock(primitives.Slot(i+1), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				totalBlocks[i] = b
				_, err = totalBlocks[i].Block().HashTreeRoot()
				require.NoError(t, err)
			}
			require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
			badFilter := filters.NewFilter().SetStartSlot(5).SetEndSlot(1)
			_, _, err := db.Blocks(ctx, badFilter)
			require.ErrorContains(t, errInvalidSlotRange.Error(), err)

			goodFilter := filters.NewFilter().SetStartSlot(0).SetEndSlot(1)
			requested, _, err := db.Blocks(ctx, goodFilter)
			require.NoError(t, err)
			assert.Equal(t, 1, len(requested), "Unexpected number of blocks received, only expected two")
		})
	}
}

func TestStore_DeleteBlock(t *testing.T) {
	slotsPerEpoch := uint64(params.BeaconConfig().SlotsPerEpoch)
	db := setupDB(t)
	ctx := context.Background()

	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisBlockRoot))
	blks := makeBlocksZond(t, 0, slotsPerEpoch*4, genesisBlockRoot)
	require.NoError(t, db.SaveBlocks(ctx, blks))
	ss := make([]*qrysmpb.StateSummary, len(blks))
	for i, blk := range blks {
		r, err := blk.Block().HashTreeRoot()
		require.NoError(t, err)
		ss[i] = &qrysmpb.StateSummary{
			Slot: blk.Block().Slot(),
			Root: r[:],
		}
	}
	require.NoError(t, db.SaveStateSummaries(ctx, ss))

	root, err := blks[slotsPerEpoch].Block().HashTreeRoot()
	require.NoError(t, err)
	cp := &qrysmpb.Checkpoint{
		Epoch: 1,
		Root:  root[:],
	}
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, cp))

	root2, err := blks[4*slotsPerEpoch-2].Block().HashTreeRoot()
	require.NoError(t, err)
	b, err := db.Block(ctx, root2)
	require.NoError(t, err)
	require.NotNil(t, b)
	require.NoError(t, db.DeleteBlock(ctx, root2))
	st, err = db.State(ctx, root2)
	require.NoError(t, err)
	require.Equal(t, st, nil)

	b, err = db.Block(ctx, root2)
	require.NoError(t, err)
	require.Equal(t, b, nil)
	require.Equal(t, false, db.HasStateSummary(ctx, root2))

	require.ErrorIs(t, db.DeleteBlock(ctx, root), ErrDeleteJustifiedAndFinalized)
}

func TestStore_DeleteJustifiedBlock(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	cp := &qrysmpb.Checkpoint{
		Root: root[:],
	}
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	blk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, blk))
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveJustifiedCheckpoint(ctx, cp))
	require.ErrorIs(t, db.DeleteBlock(ctx, root), ErrDeleteJustifiedAndFinalized)
}

func TestStore_DeleteFinalizedBlock(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	b := util.NewBeaconBlockZond()
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	cp := &qrysmpb.Checkpoint{
		Root: root[:],
	}
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	blk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, blk))
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, cp))
	require.ErrorIs(t, db.DeleteBlock(ctx, root), ErrDeleteJustifiedAndFinalized)
}
func TestStore_GenesisBlock(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	genesisBlock := util.NewBeaconBlockZond()
	genesisBlock.Block.ParentRoot = bytesutil.PadTo([]byte{1, 2, 3}, 32)
	blockRoot, err := genesisBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, blockRoot))
	wsb, err := blocks.NewSignedBeaconBlock(genesisBlock)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))
	retrievedBlock, err := db.GenesisBlock(ctx)
	require.NoError(t, err)
	retrievedBlockPb, err := retrievedBlock.Proto()
	require.NoError(t, err)
	wanted, err := wsb.ToBlinded()
	require.NoError(t, err)
	wantedPb, err := wanted.Proto()
	require.NoError(t, err)
	assert.Equal(t, true, proto.Equal(wantedPb, retrievedBlockPb), "Wanted: %v, received: %v", genesisBlock, retrievedBlock)
}

func TestStore_BlocksCRUD_NoCache(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := context.Background()
			blk, err := tt.newBlock(primitives.Slot(20), bytesutil.PadTo([]byte{1, 2, 3}, 32))
			require.NoError(t, err)
			blockRoot, err := blk.Block().HashTreeRoot()
			require.NoError(t, err)
			retrievedBlock, err := db.Block(ctx, blockRoot)
			require.NoError(t, err)
			require.DeepEqual(t, nil, retrievedBlock, "Expected nil block")
			require.NoError(t, db.SaveBlock(ctx, blk))
			db.blockCache.Del(string(blockRoot[:]))
			assert.Equal(t, true, db.HasBlock(ctx, blockRoot), "Expected block to exist in the db")
			retrievedBlock, err = db.Block(ctx, blockRoot)
			require.NoError(t, err)

			wanted := blk
			if _, err := blk.PbZondBlock(); err == nil {
				wanted, err = blk.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)
			retrievedPb, err := retrievedBlock.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, retrievedPb), "Wanted: %v, received: %v", wanted, retrievedBlock)
		})
	}
}

func TestStore_Blocks_FiltersCorrectly(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			b4, err := tt.newBlock(primitives.Slot(4), bytesutil.PadTo([]byte("parent"), 32))
			require.NoError(t, err)
			b5, err := tt.newBlock(primitives.Slot(5), bytesutil.PadTo([]byte("parent2"), 32))
			require.NoError(t, err)
			b6, err := tt.newBlock(primitives.Slot(6), bytesutil.PadTo([]byte("parent2"), 32))
			require.NoError(t, err)
			b7, err := tt.newBlock(primitives.Slot(7), bytesutil.PadTo([]byte("parent3"), 32))
			require.NoError(t, err)
			b8, err := tt.newBlock(primitives.Slot(8), bytesutil.PadTo([]byte("parent4"), 32))
			require.NoError(t, err)
			blocks := []interfaces.ReadOnlySignedBeaconBlock{
				b4,
				b5,
				b6,
				b7,
				b8,
			}
			ctx := context.Background()
			require.NoError(t, db.SaveBlocks(ctx, blocks))

			tests := []struct {
				filter            *filters.QueryFilter
				expectedNumBlocks int
			}{
				{
					filter:            filters.NewFilter().SetParentRoot(bytesutil.PadTo([]byte("parent2"), 32)),
					expectedNumBlocks: 2,
				},
				{
					// No block meets the criteria below.
					filter:            filters.NewFilter().SetParentRoot(bytesutil.PadTo([]byte{3, 4, 5}, 32)),
					expectedNumBlocks: 0,
				},
				{
					// Block slot range filter criteria.
					filter:            filters.NewFilter().SetStartSlot(5).SetEndSlot(7),
					expectedNumBlocks: 3,
				},
				{
					filter:            filters.NewFilter().SetStartSlot(7).SetEndSlot(7),
					expectedNumBlocks: 1,
				},
				{
					filter:            filters.NewFilter().SetStartSlot(4).SetEndSlot(8),
					expectedNumBlocks: 5,
				},
				{
					filter:            filters.NewFilter().SetStartSlot(4).SetEndSlot(5),
					expectedNumBlocks: 2,
				},
				{
					filter:            filters.NewFilter().SetStartSlot(5).SetEndSlot(9),
					expectedNumBlocks: 4,
				},
				{
					filter:            filters.NewFilter().SetEndSlot(7),
					expectedNumBlocks: 4,
				},
				{
					filter:            filters.NewFilter().SetEndSlot(8),
					expectedNumBlocks: 5,
				},
				{
					filter:            filters.NewFilter().SetStartSlot(5).SetEndSlot(10),
					expectedNumBlocks: 4,
				},
				{
					// Composite filter criteria.
					filter: filters.NewFilter().
						SetParentRoot(bytesutil.PadTo([]byte("parent2"), 32)).
						SetStartSlot(6).
						SetEndSlot(8),
					expectedNumBlocks: 1,
				},
				{
					// A slot filter that no block satisfies must not fall back
					// to the parent-root matches alone (slots 5 and 6).
					filter: filters.NewFilter().
						SetParentRoot(bytesutil.PadTo([]byte("parent2"), 32)).
						SetStartSlot(20).
						SetEndSlot(20),
					expectedNumBlocks: 0,
				},
				{
					filter: filters.NewFilter().
						SetParentRoot(bytesutil.PadTo([]byte("parent2"), 32)).
						SetStartSlot(6).
						SetEndSlot(6),
					expectedNumBlocks: 1,
				},
			}
			for _, tt2 := range tests {
				retrievedBlocks, _, err := db.Blocks(ctx, tt2.filter)
				require.NoError(t, err)
				assert.Equal(t, tt2.expectedNumBlocks, len(retrievedBlocks), "Unexpected number of blocks")
			}
		})
	}
}

func TestStore_Blocks_VerifyBlockRoots(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := setupDB(t)
			b1, err := tt.newBlock(primitives.Slot(1), nil)
			require.NoError(t, err)
			r1, err := b1.Block().HashTreeRoot()
			require.NoError(t, err)
			b2, err := tt.newBlock(primitives.Slot(2), nil)
			require.NoError(t, err)
			r2, err := b2.Block().HashTreeRoot()
			require.NoError(t, err)

			require.NoError(t, db.SaveBlock(ctx, b1))
			require.NoError(t, db.SaveBlock(ctx, b2))

			filter := filters.NewFilter().SetStartSlot(b1.Block().Slot()).SetEndSlot(b2.Block().Slot())
			roots, err := db.BlockRoots(ctx, filter)
			require.NoError(t, err)

			assert.DeepEqual(t, [][32]byte{r1, r2}, roots)
		})
	}
}

func TestStore_Blocks_Retrieve_SlotRange(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, 500)
			for i := range 500 {
				b, err := tt.newBlock(primitives.Slot(i), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				totalBlocks[i] = b
			}
			ctx := context.Background()
			require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
			retrieved, _, err := db.Blocks(ctx, filters.NewFilter().SetStartSlot(100).SetEndSlot(399))
			require.NoError(t, err)
			assert.Equal(t, 300, len(retrieved))
		})
	}
}

func TestStore_Blocks_Retrieve_Epoch(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			slots := params.BeaconConfig().SlotsPerEpoch.Mul(7)
			totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, slots)
			for i := range slots {
				b, err := tt.newBlock(i, bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				totalBlocks[i] = b
			}
			ctx := context.Background()
			require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
			retrieved, _, err := db.Blocks(ctx, filters.NewFilter().SetStartEpoch(5).SetEndEpoch(6))
			require.NoError(t, err)
			want := params.BeaconConfig().SlotsPerEpoch.Mul(2)
			assert.Equal(t, uint64(want), uint64(len(retrieved)))
			retrieved, _, err = db.Blocks(ctx, filters.NewFilter().SetStartEpoch(0).SetEndEpoch(0))
			require.NoError(t, err)
			want = params.BeaconConfig().SlotsPerEpoch
			assert.Equal(t, uint64(want), uint64(len(retrieved)))
		})
	}
}

func TestStore_Blocks_Retrieve_SlotRangeWithStep(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			totalBlocks := make([]interfaces.ReadOnlySignedBeaconBlock, 500)
			for i := range 500 {
				b, err := tt.newBlock(primitives.Slot(i), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				totalBlocks[i] = b
			}
			const step = 2
			ctx := context.Background()
			require.NoError(t, db.SaveBlocks(ctx, totalBlocks))
			retrieved, _, err := db.Blocks(ctx, filters.NewFilter().SetStartSlot(100).SetEndSlot(399).SetSlotStep(step))
			require.NoError(t, err)
			assert.Equal(t, 150, len(retrieved))
			for _, b := range retrieved {
				assert.Equal(t, primitives.Slot(0), (b.Block().Slot()-100)%step, "Unexpected block slot %d", b.Block().Slot())
			}
		})
	}
}

func TestStore_SaveBlock_CanGetHighestAt(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := context.Background()

			block1, err := tt.newBlock(primitives.Slot(1), nil)
			require.NoError(t, err)
			block2, err := tt.newBlock(primitives.Slot(10), nil)
			require.NoError(t, err)
			block3, err := tt.newBlock(primitives.Slot(100), nil)
			require.NoError(t, err)

			require.NoError(t, db.SaveBlock(ctx, block1))
			require.NoError(t, db.SaveBlock(ctx, block2))
			require.NoError(t, db.SaveBlock(ctx, block3))

			_, roots, err := db.HighestRootsBelowSlot(ctx, 2)
			require.NoError(t, err)
			assert.Equal(t, false, len(roots) <= 0, "Got empty highest at slice")
			require.Equal(t, 1, len(roots))
			root := roots[0]
			b, err := db.Block(ctx, root)
			require.NoError(t, err)
			wanted := block1
			if _, err := block1.PbZondBlock(); err == nil {
				wanted, err = wanted.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)
			bPb, err := b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, bPb), "Wanted: %v, received: %v", wanted, b)

			_, roots, err = db.HighestRootsBelowSlot(ctx, 11)
			require.NoError(t, err)
			assert.Equal(t, false, len(roots) <= 0, "Got empty highest at slice")
			require.Equal(t, 1, len(roots))
			root = roots[0]
			b, err = db.Block(ctx, root)
			require.NoError(t, err)
			wanted2 := block2
			if _, err := block2.PbZondBlock(); err == nil {
				wanted2, err = block2.ToBlinded()
				require.NoError(t, err)
			}
			wanted2Pb, err := wanted2.Proto()
			require.NoError(t, err)
			bPb, err = b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wanted2Pb, bPb), "Wanted: %v, received: %v", wanted2, b)

			_, roots, err = db.HighestRootsBelowSlot(ctx, 101)
			require.NoError(t, err)
			assert.Equal(t, false, len(roots) <= 0, "Got empty highest at slice")
			require.Equal(t, 1, len(roots))
			root = roots[0]
			b, err = db.Block(ctx, root)
			require.NoError(t, err)
			wanted = block3
			if _, err := block3.PbZondBlock(); err == nil {
				wanted, err = wanted.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err = wanted.Proto()
			require.NoError(t, err)
			bPb, err = b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, bPb), "Wanted: %v, received: %v", wanted, b)
		})
	}
}

func TestStore_GenesisBlock_CanGetHighestAt(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := context.Background()

			genesisBlock, err := tt.newBlock(primitives.Slot(0), nil)
			require.NoError(t, err)
			genesisRoot, err := genesisBlock.Block().HashTreeRoot()
			require.NoError(t, err)
			require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisRoot))
			require.NoError(t, db.SaveBlock(ctx, genesisBlock))
			block1, err := tt.newBlock(primitives.Slot(1), nil)
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, block1))

			_, roots, err := db.HighestRootsBelowSlot(ctx, 2)
			require.NoError(t, err)
			require.Equal(t, 1, len(roots))
			root := roots[0]
			b, err := db.Block(ctx, root)
			require.NoError(t, err)
			wanted := block1
			if _, err := block1.PbZondBlock(); err == nil {
				wanted, err = block1.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)
			bPb, err := b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, bPb), "Wanted: %v, received: %v", wanted, b)

			_, roots, err = db.HighestRootsBelowSlot(ctx, 1)
			require.NoError(t, err)
			require.Equal(t, 1, len(roots))
			root = roots[0]
			b, err = db.Block(ctx, root)
			require.NoError(t, err)
			wanted = genesisBlock
			if _, err := genesisBlock.PbZondBlock(); err == nil {
				wanted, err = genesisBlock.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err = wanted.Proto()
			require.NoError(t, err)
			bPb, err = b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, bPb), "Wanted: %v, received: %v", wanted, b)

			_, roots, err = db.HighestRootsBelowSlot(ctx, 0)
			require.NoError(t, err)
			require.Equal(t, 1, len(roots))
			root = roots[0]
			b, err = db.Block(ctx, root)
			require.NoError(t, err)
			wanted = genesisBlock
			if _, err := genesisBlock.PbZondBlock(); err == nil {
				wanted, err = genesisBlock.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err = wanted.Proto()
			require.NoError(t, err)
			bPb, err = b.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, bPb), "Wanted: %v, received: %v", wanted, b)
		})
	}
}

func TestStore_SaveBlocks_HasCachedBlocks(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := context.Background()

			b := make([]interfaces.ReadOnlySignedBeaconBlock, 500)
			for i := range 500 {
				blk, err := tt.newBlock(primitives.Slot(i), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				b[i] = blk
			}

			require.NoError(t, db.SaveBlock(ctx, b[0]))
			require.NoError(t, db.SaveBlocks(ctx, b))
			f := filters.NewFilter().SetStartSlot(0).SetEndSlot(500)

			blks, _, err := db.Blocks(ctx, f)
			require.NoError(t, err)
			assert.Equal(t, 500, len(blks), "Did not get wanted blocks")
		})
	}
}

func TestStore_SaveBlocks_HasRootsMatched(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := context.Background()

			b := make([]interfaces.ReadOnlySignedBeaconBlock, 500)
			for i := range 500 {
				blk, err := tt.newBlock(primitives.Slot(i), bytesutil.PadTo([]byte("parent"), 32))
				require.NoError(t, err)
				b[i] = blk
			}

			require.NoError(t, db.SaveBlocks(ctx, b))
			f := filters.NewFilter().SetStartSlot(0).SetEndSlot(500)

			blks, roots, err := db.Blocks(ctx, f)
			require.NoError(t, err)
			assert.Equal(t, 500, len(blks), "Did not get wanted blocks")

			for i, blk := range blks {
				rt, err := blk.Block().HashTreeRoot()
				require.NoError(t, err)
				assert.Equal(t, roots[i], rt, "mismatch of block roots")
			}
		})
	}
}

func TestStore_BlocksBySlot_BlockRootsBySlot(t *testing.T) {
	for _, tt := range blockTests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupDB(t)
			ctx := context.Background()

			b1, err := tt.newBlock(primitives.Slot(20), nil)
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, b1))
			b2, err := tt.newBlock(primitives.Slot(100), bytesutil.PadTo([]byte("parent1"), 32))
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, b2))
			b3, err := tt.newBlock(primitives.Slot(100), bytesutil.PadTo([]byte("parent2"), 32))
			require.NoError(t, err)
			require.NoError(t, db.SaveBlock(ctx, b3))

			r1, err := b1.Block().HashTreeRoot()
			require.NoError(t, err)
			r2, err := b2.Block().HashTreeRoot()
			require.NoError(t, err)
			r3, err := b3.Block().HashTreeRoot()
			require.NoError(t, err)

			retrievedBlocks, err := db.BlocksBySlot(ctx, 1)
			require.NoError(t, err)
			assert.Equal(t, 0, len(retrievedBlocks), "Unexpected number of blocks received, expected none")
			retrievedBlocks, err = db.BlocksBySlot(ctx, 20)
			require.NoError(t, err)

			wanted := b1
			if _, err := b1.PbZondBlock(); err == nil {
				wanted, err = b1.ToBlinded()
				require.NoError(t, err)
			}
			retrieved0Pb, err := retrievedBlocks[0].Proto()
			require.NoError(t, err)
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(retrieved0Pb, wantedPb), "Wanted: %v, received: %v", retrievedBlocks[0], wanted)
			assert.Equal(t, true, len(retrievedBlocks) > 0, "Expected to have blocks")
			retrievedBlocks, err = db.BlocksBySlot(ctx, 100)
			require.NoError(t, err)
			if len(retrievedBlocks) != 2 {
				t.Fatalf("Expected 2 blocks, received %d blocks", len(retrievedBlocks))
			}
			wanted = b2
			if _, err := b2.PbZondBlock(); err == nil {
				wanted, err = b2.ToBlinded()
				require.NoError(t, err)
			}
			retrieved0Pb, err = retrievedBlocks[0].Proto()
			require.NoError(t, err)
			wantedPb, err = wanted.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(wantedPb, retrieved0Pb), "Wanted: %v, received: %v", retrievedBlocks[0], wanted)
			wanted = b3
			if _, err := b3.PbZondBlock(); err == nil {
				wanted, err = b3.ToBlinded()
				require.NoError(t, err)
			}
			retrieved1Pb, err := retrievedBlocks[1].Proto()
			require.NoError(t, err)
			wantedPb, err = wanted.Proto()
			require.NoError(t, err)
			assert.Equal(t, true, proto.Equal(retrieved1Pb, wantedPb), "Wanted: %v, received: %v", retrievedBlocks[1], wanted)
			assert.Equal(t, true, len(retrievedBlocks) > 0, "Expected to have blocks")

			hasBlockRoots, retrievedBlockRoots, err := db.BlockRootsBySlot(ctx, 1)
			require.NoError(t, err)
			assert.DeepEqual(t, [][32]byte{}, retrievedBlockRoots)
			assert.Equal(t, false, hasBlockRoots, "Expected no block roots")
			hasBlockRoots, retrievedBlockRoots, err = db.BlockRootsBySlot(ctx, 20)
			require.NoError(t, err)
			assert.DeepEqual(t, [][32]byte{r1}, retrievedBlockRoots)
			assert.Equal(t, true, hasBlockRoots, "Expected no block roots")
			hasBlockRoots, retrievedBlockRoots, err = db.BlockRootsBySlot(ctx, 100)
			require.NoError(t, err)
			assert.DeepEqual(t, [][32]byte{r2, r3}, retrievedBlockRoots)
			assert.Equal(t, true, hasBlockRoots, "Expected no block roots")
		})
	}
}

func TestStore_FeeRecipientByValidatorID(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	ids := []primitives.ValidatorIndex{0, 0, 0}
	feeRecipients := []common.Address{{}, {}, {}, {}}
	require.ErrorContains(t, "validatorIDs and feeRecipients must be the same length", db.SaveFeeRecipientsByValidatorIDs(ctx, ids, feeRecipients))

	ids = []primitives.ValidatorIndex{0, 1, 2}
	feeRecipients = []common.Address{{'a'}, {'b'}, {'c'}}
	require.NoError(t, db.SaveFeeRecipientsByValidatorIDs(ctx, ids, feeRecipients))
	f, err := db.FeeRecipientByValidatorID(ctx, 0)
	require.NoError(t, err)
	require.Equal(t, common.Address{'a'}, f)
	f, err = db.FeeRecipientByValidatorID(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, common.Address{'b'}, f)
	f, err = db.FeeRecipientByValidatorID(ctx, 2)
	require.NoError(t, err)
	require.Equal(t, common.Address{'c'}, f)
	_, err = db.FeeRecipientByValidatorID(ctx, 3)
	want := errors.Wrap(ErrNotFoundFeeRecipient, "validator id 3")
	require.Equal(t, want.Error(), err.Error())

	regs := []*qrysmpb.ValidatorRegistrationV1{
		{
			FeeRecipient: bytesutil.PadTo([]byte("a"), 64),
			GasLimit:     1,
			Timestamp:    2,
			Pubkey:       bytesutil.PadTo([]byte("b"), field_params.MLDSA87PubkeyLength),
		}}
	require.NoError(t, db.SaveRegistrationsByValidatorIDs(ctx, []primitives.ValidatorIndex{3}, regs))
	f, err = db.FeeRecipientByValidatorID(ctx, 3)
	require.NoError(t, err)
	require.Equal(t, common.Address{'a'}, f)

	_, err = db.FeeRecipientByValidatorID(ctx, 4)
	want = errors.Wrap(ErrNotFoundFeeRecipient, "validator id 4")
	require.Equal(t, want.Error(), err.Error())
}

func TestStore_RegistrationsByValidatorID(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	ids := []primitives.ValidatorIndex{0, 0, 0}
	regs := []*qrysmpb.ValidatorRegistrationV1{{}, {}, {}, {}}
	require.ErrorContains(t, "ids and registrations must be the same length", db.SaveRegistrationsByValidatorIDs(ctx, ids, regs))
	timestamp := time.Now().Unix()
	ids = []primitives.ValidatorIndex{0, 1, 2}
	regs = []*qrysmpb.ValidatorRegistrationV1{
		{
			FeeRecipient: bytesutil.PadTo([]byte("a"), 64),
			GasLimit:     1,
			Timestamp:    uint64(timestamp),
			Pubkey:       bytesutil.PadTo([]byte("b"), field_params.MLDSA87PubkeyLength),
		},
		{
			FeeRecipient: bytesutil.PadTo([]byte("c"), 64),
			GasLimit:     3,
			Timestamp:    uint64(timestamp),
			Pubkey:       bytesutil.PadTo([]byte("d"), field_params.MLDSA87PubkeyLength),
		},
		{
			FeeRecipient: bytesutil.PadTo([]byte("e"), 64),
			GasLimit:     5,
			Timestamp:    uint64(timestamp),
			Pubkey:       bytesutil.PadTo([]byte("f"), field_params.MLDSA87PubkeyLength),
		},
	}
	require.NoError(t, db.SaveRegistrationsByValidatorIDs(ctx, ids, regs))
	f, err := db.RegistrationByValidatorID(ctx, 0)
	require.NoError(t, err)
	require.DeepEqual(t, &qrysmpb.ValidatorRegistrationV1{
		FeeRecipient: bytesutil.PadTo([]byte("a"), 64),
		GasLimit:     1,
		Timestamp:    uint64(timestamp),
		Pubkey:       bytesutil.PadTo([]byte("b"), field_params.MLDSA87PubkeyLength),
	}, f)
	f, err = db.RegistrationByValidatorID(ctx, 1)
	require.NoError(t, err)
	require.DeepEqual(t, &qrysmpb.ValidatorRegistrationV1{
		FeeRecipient: bytesutil.PadTo([]byte("c"), 64),
		GasLimit:     3,
		Timestamp:    uint64(timestamp),
		Pubkey:       bytesutil.PadTo([]byte("d"), field_params.MLDSA87PubkeyLength),
	}, f)
	f, err = db.RegistrationByValidatorID(ctx, 2)
	require.NoError(t, err)
	require.DeepEqual(t, &qrysmpb.ValidatorRegistrationV1{
		FeeRecipient: bytesutil.PadTo([]byte("e"), 64),
		GasLimit:     5,
		Timestamp:    uint64(timestamp),
		Pubkey:       bytesutil.PadTo([]byte("f"), field_params.MLDSA87PubkeyLength),
	}, f)
	_, err = db.RegistrationByValidatorID(ctx, 3)
	want := errors.Wrap(ErrNotFoundFeeRecipient, "validator id 3")
	require.Equal(t, want.Error(), err.Error())
}

func TestStore_DeleteBlock_FinalizedKeepsStateAndSummary(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	mk := func(slot primitives.Slot, parent [32]byte) [32]byte {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = slot
		b.Block.ParentRoot = parent[:]
		w, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := w.Block().HashTreeRoot()
		require.NoError(t, err)
		require.NoError(t, db.SaveBlock(ctx, w))
		return r
	}
	gRoot := mk(0, [32]byte{})
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, gRoot))
	r1 := mk(1, gRoot)
	r2 := mk(2, r1)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(1))
	require.NoError(t, db.SaveState(ctx, st, r1))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 1, Root: r1[:]}))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 2, Root: r2[:]}))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 0, Root: r2[:]}))
	require.Equal(t, true, db.IsFinalizedBlock(ctx, r1))

	// r1 is finalized but not the checkpoint root, so DeleteState alone would not refuse it.
	require.ErrorIs(t, db.DeleteBlock(ctx, r1), ErrDeleteJustifiedAndFinalized)
	require.Equal(t, true, db.HasBlock(ctx, r1))
	require.Equal(t, true, db.HasState(ctx, r1), "state of a finalized block must survive a refused deletion")
	require.Equal(t, true, db.HasStateSummary(ctx, r1), "summary of a finalized block must survive a refused deletion")
}

func TestDeleteBlock_RemovesStateAndSummary(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 7
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err := wsb.Block().HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(7))
	require.NoError(t, db.SaveBlock(ctx, wsb))
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 7, Root: root[:]}))

	require.NoError(t, db.DeleteBlock(ctx, root))
	require.Equal(t, false, db.HasBlock(ctx, root))
	require.Equal(t, false, db.HasState(ctx, root))
	require.Equal(t, false, db.HasStateSummary(ctx, root))
	has, _, err := db.BlockRootsBySlot(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, false, has)
}

// pauseMarker is a cache value whose cost callback blocks the cache's apply
// goroutine until released, and reports when the goroutine reached it. Queued
// ahead of an insert, it pins that goroutine at a chosen point of the set
// buffer, which makes the race between a queued insert and a later delete
// deterministic.
type pauseMarker struct {
	entered chan struct{}
	release chan struct{}
	once    *sync.Once
}

// pausableBlockCache is a block cache with the store's sizing whose apply
// goroutine honours pauseMarker values (cost 0 routes a value to the callback).
func pausableBlockCache(t *testing.T) *ristretto.Cache {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1000,
		MaxCost:     128 << 20, // the store's default; not the package variable, which another test shrinks
		BufferItems: 64,
		Metrics:     true,
		Cost: func(v interface{}) int64 {
			if m, ok := v.(pauseMarker); ok {
				m.once.Do(func() { close(m.entered) })
				<-m.release
			}
			return 1
		},
	})
	require.NoError(t, err)
	return cache
}

// newPause queues a pause marker and returns its release function and the
// channel closed once the apply goroutine is blocked at it.
func newPause(t *testing.T, cache *ristretto.Cache, key string) (release func(), entered <-chan struct{}) {
	m := pauseMarker{entered: make(chan struct{}), release: make(chan struct{}), once: &sync.Once{}}
	require.Equal(t, true, cache.Set(key, m, 0), "the pause marker must be queued")
	var once sync.Once
	release = func() { once.Do(func() { close(m.release) }) }
	t.Cleanup(release)
	return release, m.entered
}

// awaitPause fails the test when the apply goroutine does not reach the pause.
func awaitPause(t *testing.T, entered <-chan struct{}, name string) {
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatalf("the cache's apply goroutine did not reach the pause %q", name)
	}
}

// The cache applies inserts from a buffer in the background. A block deleted
// while the insert queued by its save is still unapplied is served by the
// cache after DeleteBlock returned, until the delete the cache queued behind
// the insert lands. In that window the database must still be what counts:
// HasBlock reports the block gone and a re-save writes it instead of being
// skipped as already stored. The apply goroutine is pinned before the insert
// and again between the insert and the delete, so the window is held open
// deterministically.
func TestDeleteBlock_DatabaseDecidesWhileTheCacheServesTheDeletedBlock(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	db.blockCache = pausableBlockCache(t)
	onDisk := func(root [32]byte) bool {
		var exists bool
		require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
			exists = tx.Bucket(blocksBucket).Get(root[:]) != nil
			return nil
		}))
		return exists
	}
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 7
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err := wsb.Block().HashTreeRoot()
	require.NoError(t, err)

	releaseFirst, firstEntered := newPause(t, db.blockCache, "pause-before-insert")
	awaitPause(t, firstEntered, "pause-before-insert") // the apply goroutine is pinned
	require.NoError(t, db.SaveBlock(ctx, wsb))         // the insert queues behind the pin
	releaseSecond, secondEntered := newPause(t, db.blockCache, "pause-after-insert")
	require.NoError(t, db.DeleteBlock(ctx, root)) // removal misses the unapplied insert; delete queued behind the second pause
	require.Equal(t, false, onDisk(root))

	// Let the queued insert land; the apply goroutine then stops at the second
	// pause, with the cache serving the deleted block.
	releaseFirst()
	awaitPause(t, secondEntered, "pause-after-insert")
	v, ok := db.blockCache.Get(string(root[:]))
	if !(ok && v != nil) {
		t.Logf("cache metrics: %s", db.blockCache.Metrics.String())
	}
	require.Equal(t, true, ok && v != nil, "the queued insert was not applied")

	require.Equal(t, false, db.HasBlock(ctx, root), "HasBlock must follow the database, not the cache")
	require.NoError(t, db.SaveBlock(ctx, wsb))
	require.Equal(t, true, onDisk(root), "the re-save must reach the database despite the cache hit")

	// Once the queued delete and the re-save's set land, the store serves the
	// block from the database. (The cache itself may miss: the re-save's set
	// found the stale entry and became an in-place update, which the queued
	// delete then removed; a miss falls through to the database.)
	releaseSecond()
	db.blockCache.Wait()
	require.Equal(t, true, db.HasBlock(ctx, root))
	stored, err := db.Block(ctx, root)
	require.NoError(t, err)
	require.NoError(t, blocks.BeaconBlockIsNil(stored))
	storedRoot, err := stored.Block().HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, root, storedRoot)
}

// A cache entry without a stored block (the cache is filled asynchronously)
// must not make SaveBlock skip the write.
func TestSaveBlock_CacheHitWithoutStoredBlockWrites(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 7
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err := wsb.Block().HashTreeRoot()
	require.NoError(t, err)
	db.blockCache.Set(string(root[:]), wsb, 1)
	db.blockCache.Wait()

	require.NoError(t, db.SaveBlock(ctx, wsb))
	var exists bool
	require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
		exists = tx.Bucket(blocksBucket).Get(root[:]) != nil
		return nil
	}))
	require.Equal(t, true, exists)
}
