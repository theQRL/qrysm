package lookup

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/go-zond/common/hexutil"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	dbtesting "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/beacon-chain/rpc/testutil"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpbalpha "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/protobuf/proto"
)

func TestGetBlock(t *testing.T) {
	beaconDB := dbtesting.SetupDB(t)
	ctx := context.Background()

	genBlk, blkContainers := testutil.FillDBWithBlocks(ctx, t, beaconDB)
	canonicalRoots := make(map[[32]byte]bool)

	for _, bContr := range blkContainers {
		canonicalRoots[bytesutil.ToBytes32(bContr.BlockRoot)] = true
	}
	headBlock := blkContainers[len(blkContainers)-1]
	nextSlot := headBlock.GetCapellaBlock().Block.Slot + 1

	b2 := util.NewBeaconBlockCapella()
	b2.Block.Slot = 30
	b2.Block.ParentRoot = bytesutil.PadTo([]byte{1}, 32)
	util.SaveBlock(t, ctx, beaconDB, b2)
	b3 := util.NewBeaconBlockCapella()
	b3.Block.Slot = 30
	b3.Block.ParentRoot = bytesutil.PadTo([]byte{4}, 32)
	util.SaveBlock(t, ctx, beaconDB, b3)
	b4 := util.NewBeaconBlockCapella()
	b4.Block.Slot = nextSlot
	b4.Block.ParentRoot = bytesutil.PadTo([]byte{8}, 32)
	util.SaveBlock(t, ctx, beaconDB, b4)

	wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*zondpbalpha.BeaconBlockContainer_CapellaBlock).CapellaBlock)
	require.NoError(t, err)

	fetcher := &BeaconDbBlocker{
		BeaconDB: beaconDB,
		ChainInfoFetcher: &mock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &zondpbalpha.Checkpoint{Root: blkContainers[64].BlockRoot},
			CanonicalRoots:      canonicalRoots,
		},
	}

	root, err := genBlk.Block.HashTreeRoot()
	require.NoError(t, err)

	tests := []struct {
		name    string
		blockID []byte
		want    *zondpbalpha.SignedBeaconBlockCapella
		wantErr bool
	}{
		{
			name:    "slot",
			blockID: []byte("30"),
			want:    blkContainers[30].Block.(*zondpbalpha.BeaconBlockContainer_CapellaBlock).CapellaBlock,
		},
		{
			name:    "bad formatting",
			blockID: []byte("3bad0"),
			wantErr: true,
		},
		{
			name:    "canonical",
			blockID: []byte("30"),
			want:    blkContainers[30].Block.(*zondpbalpha.BeaconBlockContainer_CapellaBlock).CapellaBlock,
		},
		{
			name:    "non canonical",
			blockID: []byte(fmt.Sprintf("%d", nextSlot)),
			want:    nil,
		},
		{
			name:    "head",
			blockID: []byte("head"),
			want:    headBlock.Block.(*zondpbalpha.BeaconBlockContainer_CapellaBlock).CapellaBlock,
		},
		{
			name:    "finalized",
			blockID: []byte("finalized"),
			want:    blkContainers[64].Block.(*zondpbalpha.BeaconBlockContainer_CapellaBlock).CapellaBlock,
		},
		{
			name:    "genesis",
			blockID: []byte("genesis"),
			want:    genBlk,
		},
		{
			name:    "genesis root",
			blockID: root[:],
			want:    genBlk,
		},
		{
			name:    "root",
			blockID: blkContainers[20].BlockRoot,
			want:    blkContainers[20].Block.(*zondpbalpha.BeaconBlockContainer_CapellaBlock).CapellaBlock,
		},
		{
			name:    "non-existent root",
			blockID: bytesutil.PadTo([]byte("hi there"), 32),
			want:    nil,
		},
		{
			name:    "hex",
			blockID: []byte(hexutil.Encode(blkContainers[20].BlockRoot)),
			want:    blkContainers[20].Block.(*zondpbalpha.BeaconBlockContainer_CapellaBlock).CapellaBlock,
		},
		{
			name:    "no block",
			blockID: []byte("105"),
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := fetcher.Block(ctx, tt.blockID)
			if tt.wantErr {
				assert.NotEqual(t, err, nil, "no error has been returned")
				return
			}
			if tt.want == nil {
				assert.Equal(t, nil, result)
				return
			}
			require.NoError(t, err)

			wsb, err := blocks.NewSignedBeaconBlock(tt.want)
			require.NoError(t, err)

			var wanted interfaces.ReadOnlySignedBeaconBlock = wsb
			if tt.name != "head" {
				wanted, err = wsb.ToBlinded()
				require.NoError(t, err)
			}
			wantedPb, err := wanted.Proto()
			require.NoError(t, err)

			resultPb, err := result.Proto()
			require.NoError(t, err)

			require.Equal(t, true, proto.Equal(wantedPb, resultPb), "Wanted: %v, received: %v", tt.want, result)
		})
	}
}
