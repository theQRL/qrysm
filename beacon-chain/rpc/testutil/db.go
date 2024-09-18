package testutil

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpbalpha "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func FillDBWithBlocks(ctx context.Context, t *testing.T, beaconDB db.Database) (*zondpbalpha.SignedBeaconBlockCapella, []*zondpbalpha.BeaconBlockContainer) {
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
