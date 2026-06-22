package util

import (
	"context"
	"testing"

	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestNewBeaconStateZond(t *testing.T) {
	st, err := NewBeaconStateZond()
	require.NoError(t, err)
	b, err := st.MarshalSSZ()
	require.NoError(t, err)
	got := &qrysmpb.BeaconStateZond{}
	require.NoError(t, got.UnmarshalSSZ(b))
	assert.DeepEqual(t, st.ToProtoUnsafe(), got)
}

func TestNewBeaconState_HashTreeRoot(t *testing.T) {
	st, err := NewBeaconStateZond()
	require.NoError(t, err)
	_, err = st.HashTreeRoot(context.Background())
	require.NoError(t, err)
}
