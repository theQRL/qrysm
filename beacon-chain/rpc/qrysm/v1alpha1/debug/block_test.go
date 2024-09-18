package debug

import (
	"context"
	"testing"

	dbTest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestServer_GetBlock(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()

	b := util.NewBeaconBlockCapella()
	b.Block.Slot = 100
	util.SaveBlock(t, ctx, db, b)
	blockRoot, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	bs := &Server{
		BeaconDB: db,
	}
	res, err := bs.GetBlock(ctx, &zondpb.BlockRequestByRoot{
		BlockRoot: blockRoot[:],
	})
	require.NoError(t, err)

	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	wsbBlinded, err := wsb.ToBlinded()
	require.NoError(t, err)

	wanted, err := wsbBlinded.MarshalSSZ()
	require.NoError(t, err)
	assert.DeepEqual(t, wanted, res.Encoded)

	// Checking for nil block.
	blockRoot = [32]byte{}
	res, err = bs.GetBlock(ctx, &zondpb.BlockRequestByRoot{
		BlockRoot: blockRoot[:],
	})
	require.NoError(t, err)
	assert.DeepEqual(t, []byte{}, res.Encoded)
}
