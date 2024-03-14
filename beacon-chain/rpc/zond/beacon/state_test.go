package beacon

import (
	"context"
	"testing"

	chainMock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	dbTest "github.com/theQRL/qrysm/v4/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/testutil"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zond "github.com/theQRL/qrysm/v4/proto/zond/v1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestGetStateRoot(t *testing.T) {
	ctx := context.Background()
	fakeState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	stateRoot, err := fakeState.HashTreeRoot(ctx)
	require.NoError(t, err)
	db := dbTest.SetupDB(t)

	chainService := &chainMock.ChainService{}
	server := &Server{
		Stater: &testutil.MockStater{
			BeaconStateRoot: stateRoot[:],
			BeaconState:     fakeState,
		},
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
		BeaconDB:              db,
	}

	resp, err := server.GetStateRoot(context.Background(), &zond.StateRequest{
		StateId: []byte("head"),
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.DeepEqual(t, stateRoot[:], resp.Data.Root)

	t.Run("execution optimistic", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		chainService := &chainMock.ChainService{Optimistic: true}
		server := &Server{
			Stater: &testutil.MockStater{
				BeaconStateRoot: stateRoot[:],
				BeaconState:     fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}
		resp, err := server.GetStateRoot(context.Background(), &zond.StateRequest{
			StateId: []byte("head"),
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.DeepEqual(t, true, resp.ExecutionOptimistic)
	})

	t.Run("finalized", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		headerRoot, err := fakeState.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		server := &Server{
			Stater: &testutil.MockStater{
				BeaconStateRoot: stateRoot[:],
				BeaconState:     fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}
		resp, err := server.GetStateRoot(context.Background(), &zond.StateRequest{
			StateId: []byte("head"),
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.DeepEqual(t, true, resp.Finalized)
	})
}

func TestGetRandao(t *testing.T) {
	mixCurrent := bytesutil.ToBytes32([]byte("current"))
	mixOld := bytesutil.ToBytes32([]byte("old"))
	epochCurrent := primitives.Epoch(100000)
	epochOld := 100000 - params.BeaconConfig().EpochsPerHistoricalVector + 1

	ctx := context.Background()
	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	// Set slot to epoch 100000
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch*100000))
	require.NoError(t, st.UpdateRandaoMixesAtIndex(uint64(epochCurrent%params.BeaconConfig().EpochsPerHistoricalVector), mixCurrent))
	require.NoError(t, st.UpdateRandaoMixesAtIndex(uint64(epochOld%params.BeaconConfig().EpochsPerHistoricalVector), mixOld))

	headEpoch := primitives.Epoch(1)
	headSt, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, headSt.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	headRandao := bytesutil.ToBytes32([]byte("head"))
	require.NoError(t, headSt.UpdateRandaoMixesAtIndex(uint64(headEpoch), headRandao))

	db := dbTest.SetupDB(t)
	chainService := &chainMock.ChainService{}
	server := &Server{
		Stater: &testutil.MockStater{
			BeaconState: st,
		},
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
		BeaconDB:              db,
	}

	t.Run("no epoch requested", func(t *testing.T) {
		resp, err := server.GetRandao(ctx, &zond.RandaoRequest{StateId: []byte("head")})
		require.NoError(t, err)
		assert.DeepEqual(t, mixCurrent[:], resp.Data.Randao)
	})
	t.Run("current epoch requested", func(t *testing.T) {
		resp, err := server.GetRandao(ctx, &zond.RandaoRequest{StateId: []byte("head"), Epoch: &epochCurrent})
		require.NoError(t, err)
		assert.DeepEqual(t, mixCurrent[:], resp.Data.Randao)
	})
	t.Run("old epoch requested", func(t *testing.T) {
		resp, err := server.GetRandao(ctx, &zond.RandaoRequest{StateId: []byte("head"), Epoch: &epochOld})
		require.NoError(t, err)
		assert.DeepEqual(t, mixOld[:], resp.Data.Randao)
	})
	t.Run("head state below `EpochsPerHistoricalVector`", func(t *testing.T) {
		server.Stater = &testutil.MockStater{
			BeaconState: headSt,
		}
		resp, err := server.GetRandao(ctx, &zond.RandaoRequest{StateId: []byte("head")})
		require.NoError(t, err)
		assert.DeepEqual(t, headRandao[:], resp.Data.Randao)
	})
	t.Run("epoch too old", func(t *testing.T) {
		epochTooOld := primitives.Epoch(100000 - st.RandaoMixesLength())
		_, err := server.GetRandao(ctx, &zond.RandaoRequest{StateId: make([]byte, 0), Epoch: &epochTooOld})
		require.ErrorContains(t, "Epoch is out of range for the randao mixes of the state", err)
	})
	t.Run("epoch in the future", func(t *testing.T) {
		futureEpoch := primitives.Epoch(100000 + 1)
		_, err := server.GetRandao(ctx, &zond.RandaoRequest{StateId: make([]byte, 0), Epoch: &futureEpoch})
		require.ErrorContains(t, "Epoch is out of range for the randao mixes of the state", err)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		chainService := &chainMock.ChainService{Optimistic: true}
		server := &Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}
		resp, err := server.GetRandao(context.Background(), &zond.RandaoRequest{
			StateId: []byte("head"),
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.DeepEqual(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		headerRoot, err := headSt.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		server := &Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}
		resp, err := server.GetRandao(context.Background(), &zond.RandaoRequest{
			StateId: []byte("head"),
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.DeepEqual(t, true, resp.Finalized)
	})
}
