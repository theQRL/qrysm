package kv

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestStore_MetadataSeqNum(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)

	seqNum, err := db.MetadataSeqNum(ctx)
	require.Equal(t, true, errors.Is(err, ErrNotFoundMetadataSeqNum))
	assert.Equal(t, uint64(0), seqNum)

	require.NoError(t, db.SaveMetadataSeqNum(ctx, 42))

	seqNum, err = db.MetadataSeqNum(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(42), seqNum)

	require.NoError(t, db.SaveMetadataSeqNum(ctx, 43))

	seqNum, err = db.MetadataSeqNum(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(43), seqNum)
}
