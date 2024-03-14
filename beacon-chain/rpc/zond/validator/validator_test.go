package validator

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	mockChain "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	builderTest "github.com/theQRL/qrysm/v4/beacon-chain/builder/testing"
	mockSync "github.com/theQRL/qrysm/v4/beacon-chain/sync/initial-sync/testing"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpbalpha "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	zondpbv1 "github.com/theQRL/qrysm/v4/proto/zond/v1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/mock"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestProduceBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.Background()

	t.Run("Capella", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Capella{Capella: &zondpbalpha.BeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlock(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, zondpbv1.Version_CAPELLA, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv1.BeaconBlockContainer_CapellaBlock)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.CapellaBlock.Slot)
	})
	t.Run("Capella blinded", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedCapella{BlindedCapella: &zondpbalpha.BlindedBeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlock(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Capella beacon block is blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Capella{Capella: &zondpbalpha.BeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true},
		}

		_, err := server.ProduceBlock(ctx, &zondpbv1.ProduceBlockRequest{})
		require.ErrorContains(t, "The node is currently optimistic and cannot serve validators", err)
	})
	t.Run("sync not ready", func(t *testing.T) {
		chainService := &mockChain.ChainService{}
		v1Server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}
		_, err := v1Server.ProduceBlock(context.Background(), nil)
		require.ErrorContains(t, "Syncing to latest head", err)
	})
}

func TestProduceBlockSSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.Background()

	t.Run("Capella", func(t *testing.T) {
		b := util.HydrateBeaconBlockCapella(&zondpbalpha.BeaconBlockCapella{})
		b.Slot = 123
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Capella{Capella: b}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedData, err := b.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Capella blinded", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedCapella{BlindedCapella: &zondpbalpha.BlindedBeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Capella beacon block is blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Capella{Capella: &zondpbalpha.BeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true},
		}

		_, err := server.ProduceBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.ErrorContains(t, "The node is currently optimistic and cannot serve validators", err)
	})
	t.Run("sync not ready", func(t *testing.T) {
		chainService := &mockChain.ChainService{}
		v1Server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}
		_, err := v1Server.ProduceBlockSSZ(context.Background(), nil)
		require.ErrorContains(t, "Syncing to latest head", err)
	})
}

func TestProduceBlindedBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.Background()

	t.Run("Capella", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedCapella{BlindedCapella: &zondpbalpha.BlindedBeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlindedBlock(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)

		assert.Equal(t, zondpbv1.Version_CAPELLA, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv1.BlindedBeaconBlockContainer_CapellaBlock)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.CapellaBlock.Slot)
	})
	t.Run("Capella full", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Capella{Capella: &zondpbalpha.BeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlindedBlock(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared beacon block is not blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedCapella{BlindedCapella: &zondpbalpha.BlindedBeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true},
		}

		_, err := server.ProduceBlindedBlock(ctx, &zondpbv1.ProduceBlockRequest{})
		require.ErrorContains(t, "The node is currently optimistic and cannot serve validators", err)
	})
	t.Run("builder not configured", func(t *testing.T) {
		v1Server := &Server{
			BlockBuilder: &builderTest.MockBuilderService{HasConfigured: false},
		}
		_, err := v1Server.ProduceBlindedBlock(context.Background(), nil)
		require.ErrorContains(t, "Block builder not configured", err)
	})
	t.Run("sync not ready", func(t *testing.T) {
		chainService := &mockChain.ChainService{}
		v1Server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
		}
		_, err := v1Server.ProduceBlindedBlock(context.Background(), nil)
		require.ErrorContains(t, "Syncing to latest head", err)
	})
}

func TestProduceBlindedBlockSSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.Background()

	t.Run("Capella", func(t *testing.T) {
		b := util.HydrateBlindedBeaconBlockCapella(&zondpbalpha.BlindedBeaconBlockCapella{})
		b.Slot = 123
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedCapella{BlindedCapella: b}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlindedBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedData, err := b.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Capella full", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Capella{Capella: &zondpbalpha.BeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlindedBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Capella beacon block is not blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedCapella{BlindedCapella: &zondpbalpha.BlindedBeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true},
		}

		_, err := server.ProduceBlindedBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.ErrorContains(t, "The node is currently optimistic and cannot serve validators", err)
	})
	t.Run("builder not configured", func(t *testing.T) {
		v1Server := &Server{
			BlockBuilder: &builderTest.MockBuilderService{HasConfigured: false},
		}
		_, err := v1Server.ProduceBlindedBlockSSZ(context.Background(), nil)
		require.ErrorContains(t, "Block builder not configured", err)
	})
	t.Run("sync not ready", func(t *testing.T) {
		chainService := &mockChain.ChainService{}
		v1Server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
		}
		_, err := v1Server.ProduceBlindedBlockSSZ(context.Background(), nil)
		require.ErrorContains(t, "Syncing to latest head", err)
	})
}
