package state_native_test

import (
	"testing"

	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func BenchmarkAppendBalance(b *testing.B) {
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(b, err)

	max := uint64(16777216)
	for i := uint64(0); i < max-2; i++ {
		require.NoError(b, st.AppendBalance(i))
	}

	ref := st.Copy()

	for i := 0; b.Loop(); i++ {
		require.NoError(b, ref.AppendBalance(uint64(i)))
		ref = st.Copy()
	}
}

func BenchmarkAppendInactivityScore(b *testing.B) {
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(b, err)

	max := uint64(16777216)
	for i := uint64(0); i < max-2; i++ {
		require.NoError(b, st.AppendInactivityScore(i))
	}

	ref := st.Copy()

	for i := 0; b.Loop(); i++ {
		require.NoError(b, ref.AppendInactivityScore(uint64(i)))
		ref = st.Copy()
	}
}
