package kv

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestArchivedPointIndexRoot_CanSaveRetrieve(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	i1 := primitives.Slot(100)
	r1 := [32]byte{'A'}

	received := db.ArchivedPointRoot(ctx, i1)
	require.NotEqual(t, r1, received, "Should not have been saved")
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(i1))
	require.NoError(t, db.SaveState(ctx, st, r1))
	received = db.ArchivedPointRoot(ctx, i1)
	assert.Equal(t, r1, received, "Should have been saved")
}

func TestLastArchivedPoint_CanRetrieve(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	i, err := db.LastArchivedSlot(ctx)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(0), i, "Did not get correct index")

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	assert.NoError(t, db.SaveState(ctx, st, [32]byte{'A'}))
	assert.Equal(t, [32]byte{'A'}, db.LastArchivedRoot(ctx), "Did not get wanted root")

	assert.NoError(t, st.SetSlot(2))
	assert.NoError(t, db.SaveState(ctx, st, [32]byte{'B'}))
	assert.Equal(t, [32]byte{'B'}, db.LastArchivedRoot(ctx))

	assert.NoError(t, st.SetSlot(3))
	assert.NoError(t, db.SaveState(ctx, st, [32]byte{'C'}))

	i, err = db.LastArchivedSlot(ctx)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(3), i, "Did not get correct index")
}

func TestArchivedPointRoot_PrefersFinalizedRootAtSlot(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	genesisRoot := [32]byte{'g'}
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesisRoot))
	slot := primitives.Slot(128)
	// The non-canonical sibling is saved first, so it comes first in the packed index value.
	sibling := [32]byte{'s'}
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(slot))
	require.NoError(t, db.SaveState(ctx, st, sibling))
	b := util.NewBeaconBlockZond()
	b.Block.Slot = slot
	b.Block.ParentRoot = genesisRoot[:]
	wb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	canonical, err := wb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wb))
	require.NoError(t, db.SaveState(ctx, st, canonical))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 4, Root: canonical[:]}))

	assert.Equal(t, canonical, db.ArchivedPointRoot(ctx, slot))
	assert.Equal(t, canonical, db.LastArchivedRoot(ctx))
}
