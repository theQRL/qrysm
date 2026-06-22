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
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/grpc"
)

func fillDBTestBlocks(ctx context.Context, t *testing.T, beaconDB db.Database) (*qrysmpb.SignedBeaconBlockZond, []*qrysmpb.BeaconBlockContainer) {
	parentRoot := [32]byte{1, 2, 3}
	genBlk := util.NewBeaconBlockZond()
	genBlk.Block.ParentRoot = parentRoot[:]
	root, err := genBlk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, genBlk)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, root))

	count := primitives.Slot(100)
	blks := make([]interfaces.ReadOnlySignedBeaconBlock, count)
	blkContainers := make([]*qrysmpb.BeaconBlockContainer, count)
	for i := range count {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = i
		b.Block.ParentRoot = bytesutil.PadTo([]byte{uint8(i)}, 32)
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		blks[i], err = blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		blkContainers[i] = &qrysmpb.BeaconBlockContainer{
			Block:     &qrysmpb.BeaconBlockContainer_ZondBlock{ZondBlock: b},
			BlockRoot: root[:],
		}
	}
	require.NoError(t, beaconDB.SaveBlocks(ctx, blks))
	headRoot := bytesutil.ToBytes32(blkContainers[len(blks)-1].BlockRoot)
	summary := &qrysmpb.StateSummary{
		Root: headRoot[:],
		Slot: blkContainers[len(blks)-1].Block.(*qrysmpb.BeaconBlockContainer_ZondBlock).ZondBlock.Block.Slot,
	}
	require.NoError(t, beaconDB.SaveStateSummary(ctx, summary))
	require.NoError(t, beaconDB.SaveHeadBlockRoot(ctx, headRoot))
	return genBlk, blkContainers
}

func TestServer_GetBlock(t *testing.T) {
	stream := &runtime.ServerTransportStream{}
	ctx := grpc.NewContextWithServerTransportStream(context.Background(), stream)
	t.Run("Zond", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
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

		blk, err := bs.GetBlock(ctx, &qrlpb.BlockRequest{})
		require.NoError(t, err)

		v1Block, err := migration.V1Alpha1BeaconBlockZondToV1(b.Block)
		require.NoError(t, err)
		bellatrixBlock, ok := blk.Data.Message.(*qrlpb.SignedBeaconBlockContainer_ZondBlock)
		require.Equal(t, true, ok)
		assert.DeepEqual(t, v1Block, bellatrixBlock.ZondBlock)
		assert.Equal(t, qrlpb.Version_ZOND, blk.Version)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
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

		blk, err := bs.GetBlock(ctx, &qrlpb.BlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, true, blk.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
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

			header, err := bs.GetBlock(ctx, &qrlpb.BlockRequest{BlockId: r[:]})
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

			resp, err := bs.GetBlock(ctx, &qrlpb.BlockRequest{BlockId: r[:]})
			require.NoError(t, err)
			assert.Equal(t, false, resp.Finalized)
		})
	})
}

func TestServer_GetBlockSSZ(t *testing.T) {
	ctx := context.Background()

	t.Run("Zond", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
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

		resp, err := bs.GetBlockSSZ(ctx, &qrlpb.BlockRequest{})
		require.NoError(t, err)
		assert.NotNil(t, resp)
		sszBlock, err := b.MarshalSSZ()
		require.NoError(t, err)
		assert.DeepEqual(t, sszBlock, resp.Data)
		assert.Equal(t, qrlpb.Version_ZOND, resp.Version)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
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

		resp, err := bs.GetBlockSSZ(ctx, &qrlpb.BlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
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

			header, err := bs.GetBlockSSZ(ctx, &qrlpb.BlockRequest{BlockId: r[:]})
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

			resp, err := bs.GetBlockSSZ(ctx, &qrlpb.BlockRequest{BlockId: r[:]})
			require.NoError(t, err)
			assert.Equal(t, false, resp.Finalized)
		})
	})
}

func TestServer_ListBlockAttestations(t *testing.T) {
	ctx := context.Background()

	t.Run("Zond", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
		b.Block.Body.Attestations = []*qrysmpb.Attestation{
			{
				AggregationBits: bitfield.Bitlist{0x00},
				Data: &qrysmpb.AttestationData{
					Slot:            123,
					CommitteeIndex:  123,
					BeaconBlockRoot: bytesutil.PadTo([]byte("root1"), 32),
					Source: &qrysmpb.Checkpoint{
						Epoch: 123,
						Root:  bytesutil.PadTo([]byte("root1"), 32),
					},
					Target: &qrysmpb.Checkpoint{
						Epoch: 123,
						Root:  bytesutil.PadTo([]byte("root1"), 32),
					},
				},
				Signatures: [][]byte{bytesutil.PadTo([]byte("sig1"), field_params.MLDSA87SignatureLength)},
			},
			{
				AggregationBits: bitfield.Bitlist{0x01},
				Data: &qrysmpb.AttestationData{
					Slot:            456,
					CommitteeIndex:  456,
					BeaconBlockRoot: bytesutil.PadTo([]byte("root2"), 32),
					Source: &qrysmpb.Checkpoint{
						Epoch: 456,
						Root:  bytesutil.PadTo([]byte("root2"), 32),
					},
					Target: &qrysmpb.Checkpoint{
						Epoch: 456,
						Root:  bytesutil.PadTo([]byte("root2"), 32),
					},
				},
				Signatures: [][]byte{bytesutil.PadTo([]byte("sig2"), field_params.MLDSA87SignatureLength)},
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

		resp, err := bs.ListBlockAttestations(ctx, &qrlpb.BlockRequest{})
		require.NoError(t, err)

		v1Block, err := migration.V1Alpha1BeaconBlockZondToV1(b.Block)
		require.NoError(t, err)
		assert.DeepEqual(t, v1Block.Body.Attestations, resp.Data)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
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

		resp, err := bs.ListBlockAttestations(ctx, &qrlpb.BlockRequest{})
		require.NoError(t, err)
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		b := util.NewBeaconBlockZond()
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

			resp, err := bs.ListBlockAttestations(ctx, &qrlpb.BlockRequest{BlockId: r[:]})
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

			resp, err := bs.ListBlockAttestations(ctx, &qrlpb.BlockRequest{BlockId: r[:]})
			require.NoError(t, err)
			assert.Equal(t, false, resp.Finalized)
		})
	})
}
