package validator

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	mockChain "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	builderTest "github.com/theQRL/qrysm/v4/beacon-chain/builder/testing"
	mockSync "github.com/theQRL/qrysm/v4/beacon-chain/sync/initial-sync/testing"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/proto/migration"
	zondpbalpha "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	zondpbv1 "github.com/theQRL/qrysm/v4/proto/zond/v1"
	zondpbv2 "github.com/theQRL/qrysm/v4/proto/zond/v2"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/mock"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestProduceBlockV2(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.Background()

	t.Run("Phase 0", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Phase0{Phase0: &zondpbalpha.BeaconBlock{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
		}

		resp, err := server.ProduceBlockV2(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)

		assert.Equal(t, zondpbv2.Version_PHASE0, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv2.BeaconBlockContainerV2_Phase0Block)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.Phase0Block.Slot)
	})
	t.Run("Altair", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Altair{Altair: &zondpbalpha.BeaconBlockAltair{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
		}

		resp, err := server.ProduceBlockV2(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)

		assert.Equal(t, zondpbv2.Version_ALTAIR, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv2.BeaconBlockContainerV2_AltairBlock)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.AltairBlock.Slot)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Bellatrix{Bellatrix: &zondpbalpha.BeaconBlockBellatrix{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlockV2(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)

		assert.Equal(t, zondpbv2.Version_BELLATRIX, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv2.BeaconBlockContainerV2_BellatrixBlock)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.BellatrixBlock.Slot)
	})
	t.Run("Bellatrix blinded", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: &zondpbalpha.BlindedBeaconBlockBellatrix{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlockV2(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Bellatrix beacon block is blinded", err)
	})
	t.Run("Capella", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Capella{Capella: &zondpbalpha.BeaconBlockCapella{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlockV2(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, zondpbv2.Version_CAPELLA, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv2.BeaconBlockContainerV2_CapellaBlock)
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

		_, err := server.ProduceBlockV2(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Capella beacon block is blinded", err)
	})
	t.Run("Deneb", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{
			Block: &zondpbalpha.GenericBeaconBlock_Deneb{
				Deneb: &zondpbalpha.BeaconBlockAndBlobsDeneb{
					Block: &zondpbalpha.BeaconBlockDeneb{Slot: 123},
					Blobs: []*zondpbalpha.BlobSidecar{{Slot: 123}},
				}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlockV2(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)

		assert.Equal(t, zondpbv2.Version_DENEB, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv2.BeaconBlockContainerV2_DenebContents)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.DenebContents.Block.Slot)
		require.Equal(t, 1, len(containerBlock.DenebContents.BlobSidecars))
		assert.Equal(t, primitives.Slot(123), containerBlock.DenebContents.BlobSidecars[0].Slot)
	})
	t.Run("Deneb blinded", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{
			Block: &zondpbalpha.GenericBeaconBlock_BlindedDeneb{
				BlindedDeneb: &zondpbalpha.BlindedBeaconBlockAndBlobsDeneb{
					Block: &zondpbalpha.BlindedBeaconBlockDeneb{Slot: 123},
					Blobs: []*zondpbalpha.BlindedBlobSidecar{{Slot: 123}},
				}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlockV2(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Deneb beacon block contents are blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Bellatrix{Bellatrix: &zondpbalpha.BeaconBlockBellatrix{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true},
		}

		_, err := server.ProduceBlockV2(ctx, &zondpbv1.ProduceBlockRequest{})
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
		_, err := v1Server.ProduceBlockV2(context.Background(), nil)
		require.ErrorContains(t, "Syncing to latest head", err)
	})
}

func TestProduceBlockV2SSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.Background()

	t.Run("Phase 0", func(t *testing.T) {
		b := util.HydrateBeaconBlock(&zondpbalpha.BeaconBlock{})
		b.Slot = 123
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Phase0{Phase0: b}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
		}

		resp, err := server.ProduceBlockV2SSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedData, err := b.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Altair", func(t *testing.T) {
		b := util.HydrateBeaconBlockAltair(&zondpbalpha.BeaconBlockAltair{})
		b.Slot = 123
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Altair{Altair: b}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
		}

		resp, err := server.ProduceBlockV2SSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedData, err := b.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		b := util.HydrateBeaconBlockBellatrix(&zondpbalpha.BeaconBlockBellatrix{})
		b.Slot = 123
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Bellatrix{Bellatrix: b}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlockV2SSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedData, err := b.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Bellatrix blinded", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: &zondpbalpha.BlindedBeaconBlockBellatrix{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlockV2SSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Bellatrix beacon block is blinded", err)
	})
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

		resp, err := server.ProduceBlockV2SSZ(ctx, &zondpbv1.ProduceBlockRequest{})
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

		_, err := server.ProduceBlockV2SSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Capella beacon block is blinded", err)
	})
	t.Run("Deneb", func(t *testing.T) {
		b, err := util.NewBeaconBlockContentsDeneb(fieldparams.MaxBlobsPerBlock)
		require.NoError(t, err)
		b.SignedBlock.Message.Slot = 123
		blk, err := migration.V2BeaconBlockDenebToV1Alpha1(b.SignedBlock.Message)
		require.NoError(t, err)
		signedBlobs := migration.SignedBlobsToV1Alpha1SignedBlobs(b.SignedBlobSidecars)
		blobs := make([]*zondpbalpha.BlobSidecar, len(signedBlobs))
		v2blobs := make([]*zondpbv2.BlobSidecar, len(signedBlobs))
		for i := range signedBlobs {
			blobs[i] = signedBlobs[i].Message
			v2blobs[i] = b.SignedBlobSidecars[i].Message
		}

		blkContents := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Deneb{
			Deneb: &zondpbalpha.BeaconBlockAndBlobsDeneb{
				Block: blk,
				Blobs: blobs,
			},
		}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blkContents, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlockV2SSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedObject := &zondpbv2.BeaconBlockContentsDeneb{
			Block:        b.SignedBlock.Message,
			BlobSidecars: v2blobs,
		}
		expectedData, err := expectedObject.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Deneb blinded", func(t *testing.T) {
		b, err := util.NewBlindedBeaconBlockContentsDeneb(fieldparams.MaxBlobsPerBlock)
		require.NoError(t, err)
		blk, err := migration.BlindedDenebToV1Alpha1SignedBlock(b.SignedBlindedBlock)
		require.NoError(t, err)
		signedBlobs := migration.SignedBlindedBlobsToV1Alpha1SignedBlindedBlobs(b.SignedBlindedBlobSidecars)
		blobs := make([]*zondpbalpha.BlindedBlobSidecar, len(signedBlobs))
		v2blobs := make([]*zondpbv2.BlindedBlobSidecar, len(signedBlobs))
		for i := range signedBlobs {
			blobs[i] = signedBlobs[i].Message
			v2blobs[i] = b.SignedBlindedBlobSidecars[i].Message
		}
		genericBlock := &zondpbalpha.GenericBeaconBlock{
			Block: &zondpbalpha.GenericBeaconBlock_BlindedDeneb{
				BlindedDeneb: &zondpbalpha.BlindedBeaconBlockAndBlobsDeneb{
					Block: blk.Message,
					Blobs: blobs,
				},
			},
		}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(genericBlock, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err = server.ProduceBlockV2SSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Deneb beacon blockcontent is blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Bellatrix{Bellatrix: &zondpbalpha.BeaconBlockBellatrix{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: true},
		}

		_, err := server.ProduceBlockV2SSZ(ctx, &zondpbv1.ProduceBlockRequest{})
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
		_, err := v1Server.ProduceBlockV2SSZ(context.Background(), nil)
		require.ErrorContains(t, "Syncing to latest head", err)
	})
}

func TestProduceBlindedBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.Background()

	t.Run("Phase 0", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Phase0{Phase0: &zondpbalpha.BeaconBlock{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
			BlockBuilder:   &builderTest.MockBuilderService{HasConfigured: true},
		}

		resp, err := server.ProduceBlindedBlock(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)

		assert.Equal(t, zondpbv2.Version_PHASE0, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv2.BlindedBeaconBlockContainer_Phase0Block)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.Phase0Block.Slot)
	})
	t.Run("Altair", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Altair{Altair: &zondpbalpha.BeaconBlockAltair{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
			BlockBuilder:   &builderTest.MockBuilderService{HasConfigured: true},
		}

		resp, err := server.ProduceBlindedBlock(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)

		assert.Equal(t, zondpbv2.Version_ALTAIR, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv2.BlindedBeaconBlockContainer_AltairBlock)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.AltairBlock.Slot)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: &zondpbalpha.BlindedBeaconBlockBellatrix{Slot: 123}}}
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

		assert.Equal(t, zondpbv2.Version_BELLATRIX, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv2.BlindedBeaconBlockContainer_BellatrixBlock)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.BellatrixBlock.Slot)
	})
	t.Run("Bellatrix full", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Bellatrix{Bellatrix: &zondpbalpha.BeaconBlockBellatrix{Slot: 123}}}
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

		assert.Equal(t, zondpbv2.Version_CAPELLA, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv2.BlindedBeaconBlockContainer_CapellaBlock)
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
	t.Run("Deneb", func(t *testing.T) {
		b, err := util.NewBlindedBeaconBlockContentsDeneb(fieldparams.MaxBlobsPerBlock)
		require.NoError(t, err)
		b.SignedBlindedBlock.Message.Slot = 123
		blk, err := migration.BlindedDenebToV1Alpha1SignedBlock(b.SignedBlindedBlock)
		require.NoError(t, err)
		signedBlobs := migration.SignedBlindedBlobsToV1Alpha1SignedBlindedBlobs(b.SignedBlindedBlobSidecars)
		blobs := make([]*zondpbalpha.BlindedBlobSidecar, len(signedBlobs))
		for i := range signedBlobs {
			blobs[i] = signedBlobs[i].Message
		}
		genericBlock := &zondpbalpha.GenericBeaconBlock{
			Block: &zondpbalpha.GenericBeaconBlock_BlindedDeneb{
				BlindedDeneb: &zondpbalpha.BlindedBeaconBlockAndBlobsDeneb{
					Block: blk.Message,
					Blobs: blobs,
				},
			},
		}

		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(genericBlock, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlindedBlock(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)

		assert.Equal(t, zondpbv2.Version_DENEB, resp.Version)
		containerBlock, ok := resp.Data.Block.(*zondpbv2.BlindedBeaconBlockContainer_DenebContents)
		require.Equal(t, true, ok)
		assert.Equal(t, primitives.Slot(123), containerBlock.DenebContents.BlindedBlock.Slot)
		assert.Equal(t, fieldparams.MaxBlobsPerBlock, len(containerBlock.DenebContents.BlindedBlobSidecars))
	})
	t.Run("Deneb full", func(t *testing.T) {
		b, err := util.NewBeaconBlockContentsDeneb(fieldparams.MaxBlobsPerBlock)
		require.NoError(t, err)
		b.SignedBlock.Message.Slot = 123
		blk, err := migration.V2BeaconBlockDenebToV1Alpha1(b.SignedBlock.Message)
		require.NoError(t, err)
		signedBlobs := migration.SignedBlobsToV1Alpha1SignedBlobs(b.SignedBlobSidecars)
		blobs := make([]*zondpbalpha.BlobSidecar, len(signedBlobs))
		for i := range signedBlobs {
			blobs[i] = signedBlobs[i].Message
		}
		blkContents := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Deneb{
			Deneb: &zondpbalpha.BeaconBlockAndBlobsDeneb{
				Block: blk,
				Blobs: blobs,
			},
		}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blkContents, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err = server.ProduceBlindedBlock(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Deneb beacon block contents are not blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: &zondpbalpha.BlindedBeaconBlockBellatrix{Slot: 123}}}
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

	t.Run("Phase 0", func(t *testing.T) {
		b := util.HydrateBeaconBlock(&zondpbalpha.BeaconBlock{})
		b.Slot = 123
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Phase0{Phase0: b}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
			BlockBuilder:   &builderTest.MockBuilderService{HasConfigured: true},
		}

		resp, err := server.ProduceBlindedBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedData, err := b.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Altair", func(t *testing.T) {
		b := util.HydrateBeaconBlockAltair(&zondpbalpha.BeaconBlockAltair{})
		b.Slot = 123
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Altair{Altair: b}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
			BlockBuilder:   &builderTest.MockBuilderService{HasConfigured: true},
		}

		resp, err := server.ProduceBlindedBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedData, err := b.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		b := util.HydrateBlindedBeaconBlockBellatrix(&zondpbalpha.BlindedBeaconBlockBellatrix{})
		b.Slot = 123
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: b}}
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
	t.Run("Bellatrix full", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Bellatrix{Bellatrix: &zondpbalpha.BeaconBlockBellatrix{Slot: 123}}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blk, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err := server.ProduceBlindedBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Bellatrix beacon block is not blinded", err)
	})
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
	t.Run("Deneb", func(t *testing.T) {
		b, err := util.NewBlindedBeaconBlockContentsDeneb(fieldparams.MaxBlobsPerBlock)
		require.NoError(t, err)
		b.SignedBlindedBlock.Message.Slot = 123
		blk, err := migration.BlindedDenebToV1Alpha1SignedBlock(b.SignedBlindedBlock)
		require.NoError(t, err)
		signedBlobs := migration.SignedBlindedBlobsToV1Alpha1SignedBlindedBlobs(b.SignedBlindedBlobSidecars)
		blobs := make([]*zondpbalpha.BlindedBlobSidecar, len(signedBlobs))
		v2blobs := make([]*zondpbv2.BlindedBlobSidecar, len(signedBlobs))
		for i := range signedBlobs {
			blobs[i] = signedBlobs[i].Message
			v2blobs[i] = b.SignedBlindedBlobSidecars[i].Message
		}
		genericBlock := &zondpbalpha.GenericBeaconBlock{
			Block: &zondpbalpha.GenericBeaconBlock_BlindedDeneb{
				BlindedDeneb: &zondpbalpha.BlindedBeaconBlockAndBlobsDeneb{
					Block: blk.Message,
					Blobs: blobs,
				},
			},
		}

		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(genericBlock, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		resp, err := server.ProduceBlindedBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		require.NoError(t, err)
		expectedObject := &zondpbv2.BlindedBeaconBlockContentsDeneb{
			BlindedBlock:        b.SignedBlindedBlock.Message,
			BlindedBlobSidecars: v2blobs,
		}
		expectedData, err := expectedObject.MarshalSSZ()
		assert.NoError(t, err)
		assert.DeepEqual(t, expectedData, resp.Data)
	})
	t.Run("Deneb full", func(t *testing.T) {
		b, err := util.NewBeaconBlockContentsDeneb(fieldparams.MaxBlobsPerBlock)
		require.NoError(t, err)
		b.SignedBlock.Message.Slot = 123
		blk, err := migration.V2BeaconBlockDenebToV1Alpha1(b.SignedBlock.Message)
		require.NoError(t, err)
		signedBlobs := migration.SignedBlobsToV1Alpha1SignedBlobs(b.SignedBlobSidecars)
		blobs := make([]*zondpbalpha.BlobSidecar, len(signedBlobs))
		for i := range signedBlobs {
			blobs[i] = signedBlobs[i].Message
		}
		blkContents := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_Deneb{
			Deneb: &zondpbalpha.BeaconBlockAndBlobsDeneb{
				Block: blk,
				Blobs: blobs,
			},
		}}
		v1alpha1Server := mock.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(blkContents, nil)
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			BlockBuilder:          &builderTest.MockBuilderService{HasConfigured: true},
			OptimisticModeFetcher: &mockChain.ChainService{Optimistic: false},
		}

		_, err = server.ProduceBlindedBlockSSZ(ctx, &zondpbv1.ProduceBlockRequest{})
		assert.ErrorContains(t, "Prepared Deneb beacon block content is not blinded", err)
	})
	t.Run("optimistic", func(t *testing.T) {
		blk := &zondpbalpha.GenericBeaconBlock{Block: &zondpbalpha.GenericBeaconBlock_BlindedBellatrix{BlindedBellatrix: &zondpbalpha.BlindedBeaconBlockBellatrix{Slot: 123}}}
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
