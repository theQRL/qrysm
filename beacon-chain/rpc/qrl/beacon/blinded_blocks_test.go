package beacon

import (
	"context"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/rpc/testutil"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/proto/migration"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/grpc"
)

func TestServer_GetBlindedBlock(t *testing.T) {
	stream := &runtime.ServerTransportStream{}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)

	t.Run("Zond", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockZond()
		blk, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		mockChainService := &mock.ChainService{}
		bs := &Server{
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: blk},
			OptimisticModeFetcher: mockChainService,
		}

		expected, err := migration.V1Alpha1BeaconBlockBlindedZondToV1Blinded(b.Block)
		require.NoError(t, err)
		resp, err := bs.GetBlindedBlock(ctx, &qrlpb.BlockRequest{})
		require.NoError(t, err)
		zondBlock, ok := resp.Data.Message.(*qrlpb.SignedBlindedBeaconBlockContainer_ZondBlock)
		require.Equal(t, true, ok)
		assert.DeepEqual(t, expected, zondBlock.ZondBlock)
		assert.Equal(t, qrlpb.Version_ZOND, resp.Version)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockZond()
		blk, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := blk.Block().HashTreeRoot()
		require.NoError(t, err)

		mockChainService := &mock.ChainService{
			OptimisticRoots: map[[32]byte]bool{r: true},
		}
		bs := &Server{
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: blk},
			OptimisticModeFetcher: mockChainService,
		}

		resp, err := bs.GetBlindedBlock(ctx, &qrlpb.BlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
		blk, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := blk.Block().HashTreeRoot()
		require.NoError(t, err)

		mockChainService := &mock.ChainService{
			FinalizedRoots: map[[32]byte]bool{root: true},
		}
		bs := &Server{
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: blk},
			OptimisticModeFetcher: mockChainService,
		}

		resp, err := bs.GetBlindedBlock(ctx, &qrlpb.BlockRequest{BlockId: root[:]})
		require.NoError(t, err)
		assert.Equal(t, true, resp.Finalized)
	})
	t.Run("not finalized", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
		blk, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := blk.Block().HashTreeRoot()
		require.NoError(t, err)

		mockChainService := &mock.ChainService{
			FinalizedRoots: map[[32]byte]bool{root: false},
		}
		bs := &Server{
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: blk},
			OptimisticModeFetcher: mockChainService,
		}

		resp, err := bs.GetBlindedBlock(ctx, &qrlpb.BlockRequest{BlockId: root[:]})
		require.NoError(t, err)
		assert.Equal(t, false, resp.Finalized)
	})
}

func TestServer_GetBlindedBlockSSZ(t *testing.T) {
	ctx := context.Background()

	t.Run("Zond", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockZond()
		blk, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		mockChainService := &mock.ChainService{}
		bs := &Server{
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: blk},
			OptimisticModeFetcher: mockChainService,
		}

		expected, err := blk.MarshalSSZ()
		require.NoError(t, err)
		resp, err := bs.GetBlindedBlockSSZ(ctx, &qrlpb.BlockRequest{})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.DeepEqual(t, expected, resp.Data)
		assert.Equal(t, qrlpb.Version_ZOND, resp.Version)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		b := util.NewBlindedBeaconBlockZond()
		blk, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := blk.Block().HashTreeRoot()
		require.NoError(t, err)

		mockChainService := &mock.ChainService{
			OptimisticRoots: map[[32]byte]bool{r: true},
		}
		bs := &Server{
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: blk},
			OptimisticModeFetcher: mockChainService,
		}

		resp, err := bs.GetBlindedBlockSSZ(ctx, &qrlpb.BlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
		blk, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := blk.Block().HashTreeRoot()
		require.NoError(t, err)

		mockChainService := &mock.ChainService{
			FinalizedRoots: map[[32]byte]bool{root: true},
		}
		bs := &Server{
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: blk},
			OptimisticModeFetcher: mockChainService,
		}

		resp, err := bs.GetBlindedBlockSSZ(ctx, &qrlpb.BlockRequest{BlockId: root[:]})
		require.NoError(t, err)
		assert.Equal(t, true, resp.Finalized)
	})
	t.Run("not finalized", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
		blk, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		root, err := blk.Block().HashTreeRoot()
		require.NoError(t, err)

		mockChainService := &mock.ChainService{
			FinalizedRoots: map[[32]byte]bool{root: false},
		}
		bs := &Server{
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: blk},
			OptimisticModeFetcher: mockChainService,
		}

		resp, err := bs.GetBlindedBlockSSZ(ctx, &qrlpb.BlockRequest{BlockId: root[:]})
		require.NoError(t, err)
		assert.Equal(t, false, resp.Finalized)
	})
}
