package forkchoice

import (
	"testing"

	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestBuilderTick(t *testing.T) {
	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
	require.NoError(t, err)
	builder := NewBuilder(t, st, blk)
	builder.Tick(t, 10)

	require.Equal(t, int64(10), builder.lastTick)
}

func TestBuilderInvalidBlock(t *testing.T) {
	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
	require.NoError(t, err)
	builder := NewBuilder(t, st, blk)
	builder.InvalidBlock(t, blk)
}

/*
func TestPoWBlock(t *testing.T) {
	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
	require.NoError(t, err)
	builder := NewBuilder(t, st, blk)
	builder.PoWBlock(&zondpb.PowBlock{BlockHash: []byte{1, 2, 3}})

	require.Equal(t, 1, len(builder.execMock.powBlocks))
}
*/
