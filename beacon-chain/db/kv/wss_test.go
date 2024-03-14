package kv

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/state/genesis"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestSaveOrigin(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	// Embedded Genesis works with Mainnet config
	params.OverrideBeaconConfig(params.MainnetConfig().Copy())

	ctx := context.Background()
	db := setupDB(t)

	st, err := genesis.State(params.MainnetName)
	require.NoError(t, err)

	sb, err := st.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, db.LoadGenesis(ctx, sb))

	// this is necessary for mainnet, because LoadGenesis is short-circuited by the embedded state,
	// so the genesis root key is never written to the db.
	require.NoError(t, db.EnsureEmbeddedGenesis(ctx))

	cst, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	cb := util.NewBeaconBlockCapella()
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, db.SaveOrigin(ctx, csb, cbb))

	broot, err := scb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, true, db.IsFinalizedBlock(ctx, broot))
}
