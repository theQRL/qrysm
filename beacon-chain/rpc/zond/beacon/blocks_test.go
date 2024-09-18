package beacon

import (
	"context"
	"testing"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/theQRL/go-bitfield"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/rpc/testutil"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/proto/migration"
	zondpbalpha "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	zondpbv1 "github.com/theQRL/qrysm/proto/zond/v1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/grpc"
)

func fillDBTestBlocks(ctx context.Context, t *testing.T, beaconDB db.Database) (*zondpbalpha.SignedBeaconBlockCapella, []*zondpbalpha.BeaconBlockContainer) {
	parentRoot := [32]byte{1, 2, 3}
	genBlk := util.NewBeaconBlockCapella()
	genBlk.Block.ParentRoot = parentRoot[:]
	root, err := genBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, genBlk)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, root))

	count := primitives.Slot(100)
	blks := make([]interfaces.ReadOnlySignedBeaconBlock, count)
	blkContainers := make([]*zondpbalpha.BeaconBlockContainer, count)
	for i := primitives.Slot(0); i < count; i++ {
		b := util.NewBeaconBlockCapella()
		b.Block.Slot = i
		b.Block.ParentRoot = bytesutil.PadTo([]byte{uint8(i)}, 32)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		blks[i], err = blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		blkContainers[i] = &zondpbalpha.BeaconBlockContainer{
			Block:     &zondpbalpha.BeaconBlockContainer_CapellaBlock{CapellaBlock: b},
			BlockRoot: root[:],
		}
	}
	require.NoError(t, beaconDB.SaveBlocks(ctx, blks))
	headRoot := bytesutil.ToBytes32(blkContainers[len(blks)-1].BlockRoot)
	summary := &zondpbalpha.StateSummary{
		Root: headRoot[:],
		Slot: blkContainers[len(blks)-1].Block.(*zondpbalpha.BeaconBlockContainer_CapellaBlock).CapellaBlock.Block.Slot,
	}
	require.NoError(t, beaconDB.SaveStateSummary(ctx, summary))
	require.NoError(t, beaconDB.SaveHeadBlockRoot(ctx, headRoot))
	return genBlk, blkContainers
}

func TestServer_GetBlock(t *testing.T) {
	stream := &runtime.ServerTransportStream{}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)
	t.Run("Capella", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &mock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		bs := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		blk, err := bs.GetBlock(ctx, &zondpbv1.BlockRequest{})
		require.NoError(t, err)

		v1Block, err := migration.V1Alpha1BeaconBlockCapellaToV1(b.Block)
		require.NoError(t, err)
		bellatrixBlock, ok := blk.Data.Message.(*zondpbv1.SignedBeaconBlockContainer_CapellaBlock)
		require.Equal(t, true, ok)
		assert.DeepEqual(t, v1Block, bellatrixBlock.CapellaBlock)
		assert.Equal(t, zondpbv1.Version_CAPELLA, blk.Version)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &mock.ChainService{
			OptimisticRoots: map[[32]byte]bool{r: true},
			FinalizedRoots:  map[[32]byte]bool{},
		}
		bs := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		blk, err := bs.GetBlock(ctx, &zondpbv1.BlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, true, blk.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}

		t.Run("true", func(t *testing.T) {
			mockChainService := &mock.ChainService{FinalizedRoots: map[[32]byte]bool{r: true}}
			bs := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			header, err := bs.GetBlock(ctx, &zondpbv1.BlockRequest{BlockId: r[:]})
			require.NoError(t, err)
			assert.Equal(t, true, header.Finalized)
		})
		t.Run("false", func(t *testing.T) {
			mockChainService := &mock.ChainService{FinalizedRoots: map[[32]byte]bool{r: false}}
			bs := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			resp, err := bs.GetBlock(ctx, &zondpbv1.BlockRequest{BlockId: r[:]})
			require.NoError(t, err)
			assert.Equal(t, false, resp.Finalized)
		})
	})
}

func TestServer_GetBlockSSZ(t *testing.T) {
	ctx := context.Background()

	t.Run("Capella", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		b.Block.Slot = 123
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)

		mockChainService := &mock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		bs := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
		}

		resp, err := bs.GetBlockSSZ(ctx, &zondpbv1.BlockRequest{})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		sszBlock, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszBlock, resp.Data)
		assert.Equal(t, zondpbv1.Version_CAPELLA, resp.Version)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)

		mockChainService := &mock.ChainService{
			OptimisticRoots: map[[32]byte]bool{r: true},
			FinalizedRoots:  map[[32]byte]bool{},
		}
		bs := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               &testutil.MockBlocker{BlockToReturn: sb},
		}

		resp, err := bs.GetBlockSSZ(ctx, &zondpbv1.BlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}

		t.Run("true", func(t *testing.T) {
			mockChainService := &mock.ChainService{FinalizedRoots: map[[32]byte]bool{r: true}}
			bs := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			header, err := bs.GetBlockSSZ(ctx, &zondpbv1.BlockRequest{BlockId: r[:]})
			require.NoError(t, err)
			assert.Equal(t, true, header.Finalized)
		})
		t.Run("false", func(t *testing.T) {
			mockChainService := &mock.ChainService{FinalizedRoots: map[[32]byte]bool{r: false}}
			bs := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			resp, err := bs.GetBlockSSZ(ctx, &zondpbv1.BlockRequest{BlockId: r[:]})
			require.NoError(t, err)
			assert.Equal(t, false, resp.Finalized)
		})
	})
}

func TestServer_ListBlockAttestations(t *testing.T) {
	ctx := context.Background()

	t.Run("Capella", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		b.Block.Body.Attestations = []*zondpbalpha.Attestation{
			{
				AggregationBits: bitfield.Bitlist{0x00},
				Data: &zondpbalpha.AttestationData{
					Slot:            123,
					CommitteeIndex:  123,
					BeaconBlockRoot: bytesutil.PadTo([]byte("root1"), 32),
					Source: &zondpbalpha.Checkpoint{
						Epoch: 123,
						Root:  bytesutil.PadTo([]byte("root1"), 32),
					},
					Target: &zondpbalpha.Checkpoint{
						Epoch: 123,
						Root:  bytesutil.PadTo([]byte("root1"), 32),
					},
				},
				Signatures: [][]byte{bytesutil.PadTo([]byte("sig1"), field_params.DilithiumSignatureLength)},
			},
			{
				AggregationBits: bitfield.Bitlist{0x01},
				Data: &zondpbalpha.AttestationData{
					Slot:            456,
					CommitteeIndex:  456,
					BeaconBlockRoot: bytesutil.PadTo([]byte("root2"), 32),
					Source: &zondpbalpha.Checkpoint{
						Epoch: 456,
						Root:  bytesutil.PadTo([]byte("root2"), 32),
					},
					Target: &zondpbalpha.Checkpoint{
						Epoch: 456,
						Root:  bytesutil.PadTo([]byte("root2"), 32),
					},
				},
				Signatures: [][]byte{bytesutil.PadTo([]byte("sig2"), field_params.DilithiumSignatureLength)},
			},
		}
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &mock.ChainService{
			FinalizedRoots: map[[32]byte]bool{},
		}
		bs := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		resp, err := bs.ListBlockAttestations(ctx, &zondpbv1.BlockRequest{})
		require.NoError(t, err)

		v1Block, err := migration.V1Alpha1BeaconBlockCapellaToV1(b.Block)
		require.NoError(t, err)
		assert.DeepEqual(t, v1Block.Body.Attestations, resp.Data)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
		mockChainService := &mock.ChainService{
			OptimisticRoots: map[[32]byte]bool{r: true},
			FinalizedRoots:  map[[32]byte]bool{},
		}
		bs := &Server{
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		resp, err := bs.ListBlockAttestations(ctx, &zondpbv1.BlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		b := util.NewBeaconBlockCapella()
		sb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)
		mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}

		t.Run("true", func(t *testing.T) {
			mockChainService := &mock.ChainService{FinalizedRoots: map[[32]byte]bool{r: true}}
			bs := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			resp, err := bs.ListBlockAttestations(ctx, &zondpbv1.BlockRequest{BlockId: r[:]})
			require.NoError(t, err)
			assert.Equal(t, true, resp.Finalized)
		})
		t.Run("false", func(t *testing.T) {
			mockChainService := &mock.ChainService{FinalizedRoots: map[[32]byte]bool{r: false}}
			bs := &Server{
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			resp, err := bs.ListBlockAttestations(ctx, &zondpbv1.BlockRequest{BlockId: r[:]})
			require.NoError(t, err)
			assert.Equal(t, false, resp.Finalized)
		})
	})
}
