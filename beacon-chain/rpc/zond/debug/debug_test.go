package debug

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	blockchainmock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	dbTest "github.com/theQRL/qrysm/v4/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/v4/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/theQRL/qrysm/v4/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/testutil"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpbv1 "github.com/theQRL/qrysm/v4/proto/zond/v1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestGetBeaconState(t *testing.T) {
	ctx := context.Background()
	db := dbTest.SetupDB(t)

	t.Run("Capella", func(t *testing.T) {
		fakeState, _ := util.DeterministicGenesisStateCapella(t, 1)
		server := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           &blockchainmock.ChainService{},
			OptimisticModeFetcher: &blockchainmock.ChainService{},
			FinalizationFetcher:   &blockchainmock.ChainService{},
			BeaconDB:              db,
		}
		resp, err := server.GetBeaconState(context.Background(), &zondpbv1.BeaconStateRequest{
			StateId: []byte("head"),
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, zondpbv1.Version_CAPELLA, resp.Version)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		fakeState, _ := util.DeterministicGenesisStateCapella(t, 1)
		server := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           &blockchainmock.ChainService{},
			OptimisticModeFetcher: &blockchainmock.ChainService{Optimistic: true},
			FinalizationFetcher:   &blockchainmock.ChainService{},
			BeaconDB:              db,
		}
		resp, err := server.GetBeaconState(context.Background(), &zondpbv1.BeaconStateRequest{
			StateId: []byte("head"),
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		fakeState, _ := util.DeterministicGenesisStateCapella(t, 1)
		headerRoot, err := fakeState.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService := &blockchainmock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		server := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}
		resp, err := server.GetBeaconState(context.Background(), &zondpbv1.BeaconStateRequest{
			StateId: []byte("head"),
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, true, resp.Finalized)
	})
}

func TestGetBeaconStateSSZ(t *testing.T) {
	t.Run("Capella", func(t *testing.T) {
		fakeState, _ := util.DeterministicGenesisStateCapella(t, 1)
		sszState, err := fakeState.MarshalSSZ()
		require.NoError(t, err)

		server := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
		}
		resp, err := server.GetBeaconStateSSZ(context.Background(), &zondpbv1.BeaconStateRequest{
			StateId: make([]byte, 0),
		})
		require.NoError(t, err)
		assert.NotNil(t, resp)

		assert.DeepEqual(t, sszState, resp.Data)
		assert.Equal(t, zondpbv1.Version_CAPELLA, resp.Version)
	})
}

func TestListForkChoiceHeads(t *testing.T) {
	ctx := context.Background()

	expectedSlotsAndRoots := []struct {
		Slot primitives.Slot
		Root [32]byte
	}{{
		Slot: 0,
		Root: bytesutil.ToBytes32(bytesutil.PadTo([]byte("foo"), 32)),
	}, {
		Slot: 1,
		Root: bytesutil.ToBytes32(bytesutil.PadTo([]byte("bar"), 32)),
	}}

	chainService := &blockchainmock.ChainService{}
	server := &Server{
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
	}
	resp, err := server.ListForkChoiceHeads(ctx, &emptypb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, 2, len(resp.Data))
	for _, sr := range expectedSlotsAndRoots {
		found := false
		for _, h := range resp.Data {
			if h.Slot == sr.Slot {
				found = true
				assert.DeepEqual(t, sr.Root[:], h.Root)
			}
			assert.Equal(t, false, h.ExecutionOptimistic)
		}
		assert.Equal(t, true, found, "Expected head not found")
	}

	t.Run("optimistic head", func(t *testing.T) {
		chainService := &blockchainmock.ChainService{
			Optimistic:      true,
			OptimisticRoots: make(map[[32]byte]bool),
		}
		for _, sr := range expectedSlotsAndRoots {
			chainService.OptimisticRoots[sr.Root] = true
		}
		server := &Server{
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}
		resp, err := server.ListForkChoiceHeads(ctx, &emptypb.Empty{})
		require.NoError(t, err)
		assert.Equal(t, 2, len(resp.Data))
		for _, sr := range expectedSlotsAndRoots {
			found := false
			for _, h := range resp.Data {
				if h.Slot == sr.Slot {
					found = true
					assert.DeepEqual(t, sr.Root[:], h.Root)
				}
				assert.Equal(t, true, h.ExecutionOptimistic)
			}
			assert.Equal(t, true, found, "Expected head not found")
		}
	})
}

func TestServer_GetForkChoice(t *testing.T) {
	store := doublylinkedtree.New()
	fRoot := [32]byte{'a'}
	fc := &forkchoicetypes.Checkpoint{Epoch: 2, Root: fRoot}
	require.NoError(t, store.UpdateFinalizedCheckpoint(fc))
	bs := &Server{ForkchoiceFetcher: &blockchainmock.ChainService{ForkChoiceStore: store}}
	res, err := bs.GetForkChoice(context.Background(), &empty.Empty{})
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(2), res.FinalizedCheckpoint.Epoch, "Did not get wanted finalized epoch")
}
