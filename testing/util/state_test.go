package util

import (
	"context"
	"testing"

	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestNewBeaconStateCapella(t *testing.T) {
	st, err := NewBeaconStateCapella()
	require.NoError(t, err)
	b, err := st.MarshalSSZ()
	require.NoError(t, err)
	got := &zondpb.BeaconStateCapella{}
	require.NoError(t, got.UnmarshalSSZ(b))
	assert.DeepEqual(t, st.ToProtoUnsafe(), got)
}

func TestNewBeaconState_HashTreeRoot(t *testing.T) {
	st, err := NewBeaconStateCapella()
	require.NoError(t, err)
	_, err = st.HashTreeRoot(context.Background())
	require.NoError(t, err)
}
