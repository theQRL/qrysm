package validator

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestServer_SetSyncAggregate_EmptyCase(t *testing.T) {
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
	require.NoError(t, err)
	s := &Server{} // Sever is not initialized with sync committee pool.
	s.setSyncAggregate(context.Background(), b)
	agg, err := b.Block().Body().SyncAggregate()
	require.NoError(t, err)

	want := &zondpb.SyncAggregate{
		SyncCommitteeBits:       make([]byte, params.BeaconConfig().SyncCommitteeSize/8),
		SyncCommitteeSignatures: [][]byte{},
	}
	require.DeepEqual(t, want, agg)
}
