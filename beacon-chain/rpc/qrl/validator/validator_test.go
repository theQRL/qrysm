package validator

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	mockChain "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	builderTest "github.com/theQRL/qrysm/beacon-chain/builder/testing"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/mock"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProduceBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.Background()

	t.Run("Zond", func(t *testing.T) {
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_Zond{Zond: &qrysmpb.BeaconBlockZond{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlock(ctx, &qrlpb.ProduceBlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, qrlpb.Version_ZOND, resp.Version)
		containerBlock, ok := resp.Data.Block.(*qrlpb.BeaconBlockContainer_ZondBlock)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.ZondBlock.Slot)
	})
	t.Run("Zond blinded", func(t *testing.T) {
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_BlindedZond{BlindedZond: &qrysmpb.BlindedBeaconBlockZond{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlock(ctx, &qrlpb.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Zond beacon block is blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_Zond{Zond: &qrysmpb.BeaconBlockZond{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true},
		}

		_, err := server.ProduceBlock(ctx, &qrlpb.ProduceBlockRequest{})
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

	t.Run("Zond", func(t *testing.T) {
		b := util.HydrateBeaconBlockZond(&qrysmpb.BeaconBlockZond{})
		b.Slot = 123
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_Zond{Zond: b}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlockSSZ(ctx, &qrlpb.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedData, err := b.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Zond blinded", func(t *testing.T) {
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_BlindedZond{BlindedZond: &qrysmpb.BlindedBeaconBlockZond{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlockSSZ(ctx, &qrlpb.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Zond beacon block is blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_Zond{Zond: &qrysmpb.BeaconBlockZond{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true},
		}

		_, err := server.ProduceBlockSSZ(ctx, &qrlpb.ProduceBlockRequest{})
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

	t.Run("Zond", func(t *testing.T) {
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_BlindedZond{BlindedZond: &qrysmpb.BlindedBeaconBlockZond{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlindedBlock(ctx, &qrlpb.ProduceBlockRequest{})
		require.NoError(t, err)

		assert.Equal(t, qrlpb.Version_ZOND, resp.Version)
		containerBlock, ok := resp.Data.Block.(*qrlpb.BlindedBeaconBlockContainer_ZondBlock)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.ZondBlock.Slot)
	})
	t.Run("Zond full", func(t *testing.T) {
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_Zond{Zond: &qrysmpb.BeaconBlockZond{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlindedBlock(ctx, &qrlpb.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared beacon block is not blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_BlindedZond{BlindedZond: &qrysmpb.BlindedBeaconBlockZond{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true},
		}

		_, err := server.ProduceBlindedBlock(ctx, &qrlpb.ProduceBlockRequest{})
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

	t.Run("Zond", func(t *testing.T) {
		b := util.HydrateBlindedBeaconBlockZond(&qrysmpb.BlindedBeaconBlockZond{})
		b.Slot = 123
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_BlindedZond{BlindedZond: b}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlindedBlockSSZ(ctx, &qrlpb.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedData, err := b.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Zond full", func(t *testing.T) {
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_Zond{Zond: &qrysmpb.BeaconBlockZond{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlindedBlockSSZ(ctx, &qrlpb.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Zond beacon block is not blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_BlindedZond{BlindedZond: &qrysmpb.BlindedBeaconBlockZond{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true},
		}

		_, err := server.ProduceBlindedBlockSSZ(ctx, &qrlpb.ProduceBlockRequest{})
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
