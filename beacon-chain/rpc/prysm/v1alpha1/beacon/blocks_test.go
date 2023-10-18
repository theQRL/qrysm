package beacon

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/golang/mock/gomock"
	chainMock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/feed"
	blockfeed "github.com/theQRL/qrysm/v4/beacon-chain/core/feed/block"
	statefeed "github.com/theQRL/qrysm/v4/beacon-chain/core/feed/state"
	dbTest "github.com/theQRL/qrysm/v4/beacon-chain/db/testing"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	mockSync "github.com/theQRL/qrysm/v4/beacon-chain/sync/initial-sync/testing"
	"github.com/theQRL/qrysm/v4/config/features"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/mock"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	"github.com/theQRL/qrysm/v4/time/slots"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ensures that if any of the checkpoints are zero-valued, an error will be generated without genesis being present
func TestServer_GetChainHead_NoGenesis(t *testing.T) {
	db := dbTest.SetupDB(t)

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.SetSlot(1))

	genBlock := util.NewBeaconBlock()
	genBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'G'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, genBlock)
	gRoot, err := genBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	cases := []struct {
		name       string
		zeroSetter func(val *zondpb.Checkpoint) error
	}{
		{
			name:       "zero-value prev justified",
			zeroSetter: s.SetPreviousJustifiedCheckpoint,
		},
		{
			name:       "zero-value current justified",
			zeroSetter: s.SetCurrentJustifiedCheckpoint,
		},
		{
			name:       "zero-value finalized",
			zeroSetter: s.SetFinalizedCheckpoint,
		},
	}
	finalized := &zondpb.Checkpoint{Epoch: 1, Root: gRoot[:]}
	prevJustified := &zondpb.Checkpoint{Epoch: 2, Root: gRoot[:]}
	justified := &zondpb.Checkpoint{Epoch: 3, Root: gRoot[:]}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.NoError(t, s.SetPreviousJustifiedCheckpoint(prevJustified))
			require.NoError(t, s.SetCurrentJustifiedCheckpoint(justified))
			require.NoError(t, s.SetFinalizedCheckpoint(finalized))
			require.NoError(t, c.zeroSetter(&zondpb.Checkpoint{Epoch: 0, Root: params.BeaconConfig().ZeroHash[:]}))
		})
		wsb, err := blocks.NewSignedBeaconBlock(genBlock)
		require.NoError(t, err)
		bs := &Server{
			BeaconDB:    db,
			HeadFetcher: &chainMock.ChainService{Block: wsb, State: s},
			FinalizationFetcher: &chainMock.ChainService{
				FinalizedCheckPoint:         s.FinalizedCheckpoint(),
				CurrentJustifiedCheckPoint:  s.CurrentJustifiedCheckpoint(),
				PreviousJustifiedCheckPoint: s.PreviousJustifiedCheckpoint()},
			OptimisticModeFetcher: &chainMock.ChainService{},
		}
		_, err = bs.GetChainHead(context.Background(), nil)
		require.ErrorContains(t, "Could not get genesis block", err)
	}
}

func TestServer_GetChainHead_NoFinalizedBlock(t *testing.T) {
	db := dbTest.SetupDB(t)

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, s.SetSlot(1))
	require.NoError(t, s.SetPreviousJustifiedCheckpoint(&zondpb.Checkpoint{Epoch: 3, Root: bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)}))
	require.NoError(t, s.SetCurrentJustifiedCheckpoint(&zondpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength)}))
	require.NoError(t, s.SetFinalizedCheckpoint(&zondpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'C'}, fieldparams.RootLength)}))

	genBlock := util.NewBeaconBlock()
	genBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'G'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, genBlock)
	gRoot, err := genBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), gRoot))

	wsb, err := blocks.NewSignedBeaconBlock(genBlock)
	require.NoError(t, err)

	bs := &Server{
		BeaconDB:    db,
		HeadFetcher: &chainMock.ChainService{Block: wsb, State: s},
		FinalizationFetcher: &chainMock.ChainService{
			FinalizedCheckPoint:         s.FinalizedCheckpoint(),
			CurrentJustifiedCheckPoint:  s.CurrentJustifiedCheckpoint(),
			PreviousJustifiedCheckPoint: s.PreviousJustifiedCheckpoint()},
		OptimisticModeFetcher: &chainMock.ChainService{},
	}

	_, err = bs.GetChainHead(context.Background(), nil)
	require.ErrorContains(t, "Could not get finalized block", err)
}

func TestServer_GetChainHead_NoHeadBlock(t *testing.T) {
	bs := &Server{
		HeadFetcher:           &chainMock.ChainService{Block: nil},
		OptimisticModeFetcher: &chainMock.ChainService{},
	}
	_, err := bs.GetChainHead(context.Background(), nil)
	assert.ErrorContains(t, "Head block of chain was nil", err)
}

func TestServer_GetChainHead(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())

	db := dbTest.SetupDB(t)
	genBlock := util.NewBeaconBlock()
	genBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'G'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, genBlock)
	gRoot, err := genBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), gRoot))

	finalizedBlock := util.NewBeaconBlock()
	finalizedBlock.Block.Slot = 1
	finalizedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, finalizedBlock)
	fRoot, err := finalizedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot = 2
	justifiedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, justifiedBlock)
	jRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	prevJustifiedBlock := util.NewBeaconBlock()
	prevJustifiedBlock.Block.Slot = 3
	prevJustifiedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'C'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, prevJustifiedBlock)
	pjRoot, err := prevJustifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	s, err := state_native.InitializeFromProtoPhase0(&zondpb.BeaconState{
		Slot:                        1,
		PreviousJustifiedCheckpoint: &zondpb.Checkpoint{Epoch: 3, Root: pjRoot[:]},
		CurrentJustifiedCheckpoint:  &zondpb.Checkpoint{Epoch: 2, Root: jRoot[:]},
		FinalizedCheckpoint:         &zondpb.Checkpoint{Epoch: 1, Root: fRoot[:]},
	})
	require.NoError(t, err)

	b := util.NewBeaconBlock()
	b.Block.Slot, err = slots.EpochStart(s.PreviousJustifiedCheckpoint().Epoch)
	require.NoError(t, err)
	b.Block.Slot++
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	bs := &Server{
		BeaconDB:              db,
		HeadFetcher:           &chainMock.ChainService{Block: wsb, State: s},
		OptimisticModeFetcher: &chainMock.ChainService{},
		FinalizationFetcher: &chainMock.ChainService{
			FinalizedCheckPoint:         s.FinalizedCheckpoint(),
			CurrentJustifiedCheckPoint:  s.CurrentJustifiedCheckpoint(),
			PreviousJustifiedCheckPoint: s.PreviousJustifiedCheckpoint()},
	}

	head, err := bs.GetChainHead(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, primitives.Epoch(3), head.PreviousJustifiedEpoch, "Unexpected PreviousJustifiedEpoch")
	assert.Equal(t, primitives.Epoch(2), head.JustifiedEpoch, "Unexpected JustifiedEpoch")
	assert.Equal(t, primitives.Epoch(1), head.FinalizedEpoch, "Unexpected FinalizedEpoch")
	assert.Equal(t, primitives.Slot(24), head.PreviousJustifiedSlot, "Unexpected PreviousJustifiedSlot")
	assert.Equal(t, primitives.Slot(16), head.JustifiedSlot, "Unexpected JustifiedSlot")
	assert.Equal(t, primitives.Slot(8), head.FinalizedSlot, "Unexpected FinalizedSlot")
	assert.DeepEqual(t, pjRoot[:], head.PreviousJustifiedBlockRoot, "Unexpected PreviousJustifiedBlockRoot")
	assert.DeepEqual(t, jRoot[:], head.JustifiedBlockRoot, "Unexpected JustifiedBlockRoot")
	assert.DeepEqual(t, fRoot[:], head.FinalizedBlockRoot, "Unexpected FinalizedBlockRoot")
	assert.Equal(t, false, head.OptimisticStatus)
}

func TestServer_StreamChainHead_ContextCanceled(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	ctx, cancel := context.WithCancel(ctx)
	chainService := &chainMock.ChainService{}
	server := &Server{
		Ctx:           ctx,
		StateNotifier: chainService.StateNotifier(),
		BeaconDB:      db,
	}

	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconChain_StreamChainHeadServer(ctrl)
	mockStream.EXPECT().Context().Return(ctx)
	go func(tt *testing.T) {
		assert.ErrorContains(tt, "Context canceled", server.StreamChainHead(&emptypb.Empty{}, mockStream))
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestServer_StreamChainHead_OnHeadUpdated(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MainnetConfig())
	db := dbTest.SetupDB(t)
	genBlock := util.NewBeaconBlock()
	genBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'G'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, genBlock)
	gRoot, err := genBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisBlockRoot(context.Background(), gRoot))

	finalizedBlock := util.NewBeaconBlock()
	finalizedBlock.Block.Slot = 32
	finalizedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, finalizedBlock)
	fRoot, err := finalizedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	justifiedBlock := util.NewBeaconBlock()
	justifiedBlock.Block.Slot = 64
	justifiedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, justifiedBlock)
	jRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	prevJustifiedBlock := util.NewBeaconBlock()
	prevJustifiedBlock.Block.Slot = 96
	prevJustifiedBlock.Block.ParentRoot = bytesutil.PadTo([]byte{'C'}, fieldparams.RootLength)
	util.SaveBlock(t, context.Background(), db, prevJustifiedBlock)
	pjRoot, err := prevJustifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	s, err := state_native.InitializeFromProtoPhase0(&zondpb.BeaconState{
		Slot:                        1,
		PreviousJustifiedCheckpoint: &zondpb.Checkpoint{Epoch: 3, Root: pjRoot[:]},
		CurrentJustifiedCheckpoint:  &zondpb.Checkpoint{Epoch: 2, Root: jRoot[:]},
		FinalizedCheckpoint:         &zondpb.Checkpoint{Epoch: 1, Root: fRoot[:]},
	})
	require.NoError(t, err)

	b := util.NewBeaconBlock()
	b.Block.Slot, err = slots.EpochStart(s.PreviousJustifiedCheckpoint().Epoch)
	require.NoError(t, err)

	hRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	chainService := &chainMock.ChainService{}
	ctx := context.Background()
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	server := &Server{
		Ctx:           ctx,
		HeadFetcher:   &chainMock.ChainService{Block: wsb, State: s},
		BeaconDB:      db,
		StateNotifier: chainService.StateNotifier(),
		FinalizationFetcher: &chainMock.ChainService{
			FinalizedCheckPoint:         s.FinalizedCheckpoint(),
			CurrentJustifiedCheckPoint:  s.CurrentJustifiedCheckpoint(),
			PreviousJustifiedCheckPoint: s.PreviousJustifiedCheckpoint()},
		OptimisticModeFetcher: &chainMock.ChainService{},
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconChain_StreamChainHeadServer(ctrl)
	mockStream.EXPECT().Send(
		&zondpb.ChainHead{
			HeadSlot:                   b.Block.Slot,
			HeadEpoch:                  slots.ToEpoch(b.Block.Slot),
			HeadBlockRoot:              hRoot[:],
			FinalizedSlot:              32,
			FinalizedEpoch:             1,
			FinalizedBlockRoot:         fRoot[:],
			JustifiedSlot:              64,
			JustifiedEpoch:             2,
			JustifiedBlockRoot:         jRoot[:],
			PreviousJustifiedSlot:      96,
			PreviousJustifiedEpoch:     3,
			PreviousJustifiedBlockRoot: pjRoot[:],
		},
	).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		assert.NoError(tt, server.StreamChainHead(&emptypb.Empty{}, mockStream), "Could not call RPC method")
	}(t)

	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = server.StateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.BlockProcessed,
			Data: &statefeed.BlockProcessedData{},
		})
	}
	<-exitRoutine
}

func TestServer_StreamBlocksVerified_ContextCanceled(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	chainService := &chainMock.ChainService{}
	ctx, cancel := context.WithCancel(ctx)
	server := &Server{
		Ctx:           ctx,
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
		BeaconDB:      db,
	}

	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconChain_StreamBlocksServer(ctrl)
	mockStream.EXPECT().Context().Return(ctx)
	go func(tt *testing.T) {
		assert.ErrorContains(tt, "Context canceled", server.StreamBlocks(&zondpb.StreamBlocksRequest{
			VerifiedOnly: true,
		}, mockStream))
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestServer_StreamBlocks_ContextCanceled(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	chainService := &chainMock.ChainService{}
	ctx, cancel := context.WithCancel(ctx)
	server := &Server{
		Ctx:           ctx,
		BlockNotifier: chainService.BlockNotifier(),
		HeadFetcher:   chainService,
		BeaconDB:      db,
	}

	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconChain_StreamBlocksServer(ctrl)
	mockStream.EXPECT().Context().Return(ctx)
	go func(tt *testing.T) {
		assert.ErrorContains(tt, "Context canceled", server.StreamBlocks(&zondpb.StreamBlocksRequest{}, mockStream))
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestServer_StreamBlocks_OnHeadUpdated(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())

	ctx := context.Background()
	beaconState, privs := util.DeterministicGenesisState(t, 32)
	b, err := util.GenerateFullBlock(beaconState, privs, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	chainService := &chainMock.ChainService{State: beaconState}
	server := &Server{
		Ctx:           ctx,
		BlockNotifier: chainService.BlockNotifier(),
		HeadFetcher:   chainService,
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconChain_StreamBlocksServer(ctrl)
	mockStream.EXPECT().Send(b).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		assert.NoError(tt, server.StreamBlocks(&zondpb.StreamBlocksRequest{}, mockStream), "Could not call RPC method")
	}(t)

	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		sent = server.BlockNotifier.BlockFeed().Send(&feed.Event{
			Type: blockfeed.ReceivedBlock,
			Data: &blockfeed.ReceivedBlockData{SignedBlock: wsb},
		})
	}
	<-exitRoutine
}

func TestServer_StreamBlocksVerified_OnHeadUpdated(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.BeaconConfig())

	db := dbTest.SetupDB(t)
	ctx := context.Background()
	beaconState, privs := util.DeterministicGenesisState(t, 32)
	b, err := util.GenerateFullBlock(beaconState, privs, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, db, b)
	chainService := &chainMock.ChainService{State: beaconState}
	server := &Server{
		Ctx:           ctx,
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
		BeaconDB:      db,
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconChain_StreamBlocksServer(ctrl)
	mockStream.EXPECT().Send(b).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	mockStream.EXPECT().Context().Return(ctx).AnyTimes()

	go func(tt *testing.T) {
		assert.NoError(tt, server.StreamBlocks(&zondpb.StreamBlocksRequest{
			VerifiedOnly: true,
		}, mockStream), "Could not call RPC method")
	}(t)

	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		sent = server.StateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.BlockProcessed,
			Data: &statefeed.BlockProcessedData{Slot: b.Block.Slot, BlockRoot: r, SignedBlock: wsb},
		})
	}
	<-exitRoutine
}

func TestServer_ListBeaconBlocks_NoResults(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	bs := &Server{
		BeaconDB: db,
	}
	wanted := &zondpb.ListBeaconBlocksResponse{
		BlockContainers: make([]*zondpb.BeaconBlockContainer, 0),
		TotalSize:       int32(0),
		NextPageToken:   strconv.Itoa(0),
	}
	res, err := bs.ListBeaconBlocks(ctx, &zondpb.ListBlocksRequest{
		QueryFilter: &zondpb.ListBlocksRequest_Slot{
			Slot: 0,
		},
	})
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
	res, err = bs.ListBeaconBlocks(ctx, &zondpb.ListBlocksRequest{
		QueryFilter: &zondpb.ListBlocksRequest_Slot{
			Slot: 0,
		},
	})
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
	res, err = bs.ListBeaconBlocks(ctx, &zondpb.ListBlocksRequest{
		QueryFilter: &zondpb.ListBlocksRequest_Root{
			Root: make([]byte, 32),
		},
	})
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
}

func TestServer_ListBeaconBlocks_Genesis(t *testing.T) {
	t.Run("phase 0 block", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		blkContainer := &zondpb.BeaconBlockContainer{
			Block: &zondpb.BeaconBlockContainer_Phase0Block{Phase0Block: blk}}
		wrappedB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBlocksGenesis(t, wrappedB, blkContainer)
	})
	t.Run("altair block", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockAltair()
		blk.Block.ParentRoot = parentRoot[:]
		wrapped, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		blkContainer := &zondpb.BeaconBlockContainer{
			Block: &zondpb.BeaconBlockContainer_AltairBlock{AltairBlock: blk}}
		runListBlocksGenesis(t, wrapped, blkContainer)
	})
	t.Run("bellatrix block", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockBellatrix()
		blk.Block.ParentRoot = parentRoot[:]
		wrapped, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		blinded, err := wrapped.ToBlinded()
		assert.NoError(t, err)
		blindedProto, err := blinded.PbBlindedBellatrixBlock()
		assert.NoError(t, err)
		blkContainer := &zondpb.BeaconBlockContainer{
			Block: &zondpb.BeaconBlockContainer_BlindedBellatrixBlock{BlindedBellatrixBlock: blindedProto}}
		runListBlocksGenesis(t, wrapped, blkContainer)
	})
	t.Run("capella block", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		wrapped, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		blinded, err := wrapped.ToBlinded()
		assert.NoError(t, err)
		blindedProto, err := blinded.PbBlindedCapellaBlock()
		assert.NoError(t, err)
		blkContainer := &zondpb.BeaconBlockContainer{
			Block: &zondpb.BeaconBlockContainer_BlindedCapellaBlock{BlindedCapellaBlock: blindedProto}}
		runListBlocksGenesis(t, wrapped, blkContainer)
	})
}

func runListBlocksGenesis(t *testing.T, blk interfaces.ReadOnlySignedBeaconBlock, blkContainer *zondpb.BeaconBlockContainer) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	bs := &Server{
		BeaconDB: db,
	}

	// Should throw an error if no genesis block is found.
	_, err := bs.ListBeaconBlocks(ctx, &zondpb.ListBlocksRequest{
		QueryFilter: &zondpb.ListBlocksRequest_Genesis{
			Genesis: true,
		},
	})
	require.ErrorContains(t, "Could not find genesis", err)

	root, err := blk.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, blk))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))
	blkContainer.BlockRoot = root[:]
	blkContainer.Canonical = true

	wanted := &zondpb.ListBeaconBlocksResponse{
		BlockContainers: []*zondpb.BeaconBlockContainer{blkContainer},
		NextPageToken:   "0",
		TotalSize:       1,
	}
	res, err := bs.ListBeaconBlocks(ctx, &zondpb.ListBlocksRequest{
		QueryFilter: &zondpb.ListBlocksRequest_Genesis{
			Genesis: true,
		},
	})
	require.NoError(t, err)
	if !proto.Equal(wanted, res) {
		t.Errorf("Wanted %v, received %v", wanted, res)
	}
}

func TestServer_ListBeaconBlocks_Genesis_MultiBlocks(t *testing.T) {
	t.Run("phase 0 block", func(t *testing.T) {
		parentRoot := [32]byte{1, 2, 3}
		blk := util.NewBeaconBlock()
		blk.Block.ParentRoot = parentRoot[:]
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlock()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		genB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksGenesisMultiBlocks(t, genB, blockCreator)
	})
	t.Run("altair block", func(t *testing.T) {
		parentRoot := [32]byte{1, 2, 3}
		blk := util.NewBeaconBlockAltair()
		blk.Block.ParentRoot = parentRoot[:]
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockAltair()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		gBlock, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksGenesisMultiBlocks(t, gBlock, blockCreator)
	})
	t.Run("bellatrix block", func(t *testing.T) {
		parentRoot := [32]byte{1, 2, 3}
		blk := util.NewBeaconBlockBellatrix()
		blk.Block.ParentRoot = parentRoot[:]
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockBellatrix()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		gBlock, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksGenesisMultiBlocks(t, gBlock, blockCreator)
	})
	t.Run("capella block", func(t *testing.T) {
		parentRoot := [32]byte{1, 2, 3}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockCapella()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		gBlock, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksGenesisMultiBlocks(t, gBlock, blockCreator)
	})
}

func runListBeaconBlocksGenesisMultiBlocks(t *testing.T, genBlock interfaces.ReadOnlySignedBeaconBlock,
	blockCreator func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	bs := &Server{
		BeaconDB: db,
	}
	// Should return the proper genesis block if it exists.
	root, err := genBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, genBlock))
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

	count := primitives.Slot(100)
	blks := make([]interfaces.ReadOnlySignedBeaconBlock, count)
	for i := primitives.Slot(0); i < count; i++ {
		blks[i] = blockCreator(i)
	}
	require.NoError(t, db.SaveBlocks(ctx, blks))

	// Should throw an error if more than one blk returned.
	_, err = bs.ListBeaconBlocks(ctx, &zondpb.ListBlocksRequest{
		QueryFilter: &zondpb.ListBlocksRequest_Genesis{
			Genesis: true,
		},
	})
	require.NoError(t, err)
}

func TestServer_ListBeaconBlocks_Pagination(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())
	t.Run("phase 0 block", func(t *testing.T) {
		blk := util.NewBeaconBlock()
		blk.Block.Slot = 300
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlock()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		containerCreator := func(i primitives.Slot, root []byte, canonical bool) *zondpb.BeaconBlockContainer {
			b := util.NewBeaconBlock()
			b.Block.Slot = i
			ctr := &zondpb.BeaconBlockContainer{
				Block: &zondpb.BeaconBlockContainer_Phase0Block{
					Phase0Block: util.HydrateSignedBeaconBlock(b)},
				BlockRoot: root,
				Canonical: canonical}
			return ctr
		}
		wrappedB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksPagination(t, wrappedB, blockCreator, containerCreator)
	})
	t.Run("altair block", func(t *testing.T) {
		blk := util.NewBeaconBlockAltair()
		blk.Block.Slot = 300
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockAltair()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		containerCreator := func(i primitives.Slot, root []byte, canonical bool) *zondpb.BeaconBlockContainer {
			b := util.NewBeaconBlockAltair()
			b.Block.Slot = i
			ctr := &zondpb.BeaconBlockContainer{
				Block: &zondpb.BeaconBlockContainer_AltairBlock{
					AltairBlock: util.HydrateSignedBeaconBlockAltair(b)},
				BlockRoot: root,
				Canonical: canonical}
			return ctr
		}
		orphanedB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksPagination(t, orphanedB, blockCreator, containerCreator)
	})
	t.Run("bellatrix block", func(t *testing.T) {
		resetFn := features.InitWithReset(&features.Flags{
			SaveFullExecutionPayloads: true,
		})
		defer resetFn()
		blk := util.NewBeaconBlockBellatrix()
		blk.Block.Slot = 300
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockBellatrix()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		containerCreator := func(i primitives.Slot, root []byte, canonical bool) *zondpb.BeaconBlockContainer {
			b := util.NewBeaconBlockBellatrix()
			b.Block.Slot = i
			ctr := &zondpb.BeaconBlockContainer{
				Block: &zondpb.BeaconBlockContainer_BellatrixBlock{
					BellatrixBlock: util.HydrateSignedBeaconBlockBellatrix(b)},
				BlockRoot: root,
				Canonical: canonical}
			return ctr
		}
		orphanedB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksPagination(t, orphanedB, blockCreator, containerCreator)
	})
	t.Run("capella block", func(t *testing.T) {
		resetFn := features.InitWithReset(&features.Flags{
			SaveFullExecutionPayloads: true,
		})
		defer resetFn()
		blk := util.NewBeaconBlockCapella()
		blk.Block.Slot = 300
		blockCreator := func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock {
			b := util.NewBeaconBlockCapella()
			b.Block.Slot = i
			wrappedB, err := blocks.NewSignedBeaconBlock(b)
			assert.NoError(t, err)
			return wrappedB
		}
		containerCreator := func(i primitives.Slot, root []byte, canonical bool) *zondpb.BeaconBlockContainer {
			b := util.NewBeaconBlockCapella()
			b.Block.Slot = i
			ctr := &zondpb.BeaconBlockContainer{
				Block: &zondpb.BeaconBlockContainer_CapellaBlock{
					CapellaBlock: util.HydrateSignedBeaconBlockCapella(b)},
				BlockRoot: root,
				Canonical: canonical}
			return ctr
		}
		orphanedB, err := blocks.NewSignedBeaconBlock(blk)
		assert.NoError(t, err)
		runListBeaconBlocksPagination(t, orphanedB, blockCreator, containerCreator)
	})
}

func runListBeaconBlocksPagination(t *testing.T, orphanedBlk interfaces.ReadOnlySignedBeaconBlock,
	blockCreator func(i primitives.Slot) interfaces.ReadOnlySignedBeaconBlock, containerCreator func(i primitives.Slot, root []byte, canonical bool) *zondpb.BeaconBlockContainer) {

	db := dbTest.SetupDB(t)
	chain := &chainMock.ChainService{
		CanonicalRoots: map[[32]byte]bool{},
	}
	ctx := context.Background()

	count := primitives.Slot(100)
	blks := make([]interfaces.ReadOnlySignedBeaconBlock, count)
	blkContainers := make([]*zondpb.BeaconBlockContainer, count)
	for i := primitives.Slot(0); i < count; i++ {
		b := blockCreator(i)
		root, err := b.Block().HashTreeRoot()
		require.NoError(t, err)
		chain.CanonicalRoots[root] = true
		blks[i] = b
		ctr, err := convertToBlockContainer(blks[i], root, true)
		require.NoError(t, err)
		blkContainers[i] = ctr
	}
	require.NoError(t, db.SaveBlocks(ctx, blks))

	orphanedBlkRoot, err := orphanedBlk.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, orphanedBlk))

	bs := &Server{
		BeaconDB:         db,
		CanonicalFetcher: chain,
	}

	root6, err := blks[6].Block().HashTreeRoot()
	require.NoError(t, err)

	tests := []struct {
		req *zondpb.ListBlocksRequest
		res *zondpb.ListBeaconBlocksResponse
	}{
		{req: &zondpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(0),
			QueryFilter: &zondpb.ListBlocksRequest_Slot{Slot: 5},
			PageSize:    3},
			res: &zondpb.ListBeaconBlocksResponse{
				BlockContainers: []*zondpb.BeaconBlockContainer{containerCreator(5, blkContainers[5].BlockRoot, blkContainers[5].Canonical)},
				NextPageToken:   "",
				TotalSize:       1,
			},
		},
		{req: &zondpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(0),
			QueryFilter: &zondpb.ListBlocksRequest_Root{Root: root6[:]},
			PageSize:    3},
			res: &zondpb.ListBeaconBlocksResponse{
				BlockContainers: []*zondpb.BeaconBlockContainer{containerCreator(6, blkContainers[6].BlockRoot, blkContainers[6].Canonical)},
				TotalSize:       1,
				NextPageToken:   strconv.Itoa(0)}},
		{req: &zondpb.ListBlocksRequest{QueryFilter: &zondpb.ListBlocksRequest_Root{Root: root6[:]}},
			res: &zondpb.ListBeaconBlocksResponse{
				BlockContainers: []*zondpb.BeaconBlockContainer{containerCreator(6, blkContainers[6].BlockRoot, blkContainers[6].Canonical)},
				TotalSize:       1, NextPageToken: strconv.Itoa(0)}},
		{req: &zondpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(0),
			QueryFilter: &zondpb.ListBlocksRequest_Epoch{Epoch: 0},
			PageSize:    100},
			res: &zondpb.ListBeaconBlocksResponse{
				BlockContainers: blkContainers[0:params.BeaconConfig().SlotsPerEpoch],
				NextPageToken:   "",
				TotalSize:       int32(params.BeaconConfig().SlotsPerEpoch)}},
		{req: &zondpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(1),
			QueryFilter: &zondpb.ListBlocksRequest_Epoch{Epoch: 5},
			PageSize:    3},
			res: &zondpb.ListBeaconBlocksResponse{
				BlockContainers: blkContainers[43:46],
				NextPageToken:   "2",
				TotalSize:       int32(params.BeaconConfig().SlotsPerEpoch)}},
		{req: &zondpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(1),
			QueryFilter: &zondpb.ListBlocksRequest_Epoch{Epoch: 11},
			PageSize:    7},
			res: &zondpb.ListBeaconBlocksResponse{
				BlockContainers: blkContainers[95:96],
				NextPageToken:   "",
				TotalSize:       int32(params.BeaconConfig().SlotsPerEpoch)}},
		{req: &zondpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(0),
			QueryFilter: &zondpb.ListBlocksRequest_Epoch{Epoch: 12},
			PageSize:    4},
			res: &zondpb.ListBeaconBlocksResponse{
				BlockContainers: blkContainers[96:100],
				NextPageToken:   "",
				TotalSize:       int32(params.BeaconConfig().SlotsPerEpoch / 2)}},
		{req: &zondpb.ListBlocksRequest{
			PageToken:   strconv.Itoa(0),
			QueryFilter: &zondpb.ListBlocksRequest_Slot{Slot: 300},
			PageSize:    3},
			res: &zondpb.ListBeaconBlocksResponse{
				BlockContainers: []*zondpb.BeaconBlockContainer{containerCreator(300, orphanedBlkRoot[:], false)},
				NextPageToken:   "",
				TotalSize:       1}},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("test_%d", i), func(t *testing.T) {
			res, err := bs.ListBeaconBlocks(ctx, test.req)
			require.NoError(t, err)
			require.DeepSSZEqual(t, res, test.res)
		})
	}
}

func TestServer_ConvertToBlockContainer(t *testing.T) {
	b := util.NewBeaconBlockCapella()
	root, err := b.HashTreeRoot()
	require.NoError(t, err)
	wrapped, err := blocks.NewSignedBeaconBlock(b)
	assert.NoError(t, err)
	container, err := convertToBlockContainer(wrapped, root, true)
	require.NoError(t, err)
	require.NotNil(t, container.GetCapellaBlock())

	bb := util.NewBlindedBeaconBlockCapella()
	root, err = b.HashTreeRoot()
	require.NoError(t, err)
	wrapped, err = blocks.NewSignedBeaconBlock(bb)
	assert.NoError(t, err)
	container, err = convertToBlockContainer(wrapped, root, true)
	require.NoError(t, err)
	require.NotNil(t, container.GetBlindedCapellaBlock())
}
