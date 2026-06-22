package state_native_test

import (
	"testing"

	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func BenchmarkParticipationBits(b *testing.B) {
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(b, err)

	max := uint64(16777216)
	for i := uint64(0); i < max-2; i++ {
		require.NoError(b, st.AppendCurrentParticipationBits(byte(1)))
	}

	ref := st.Copy()

	for b.Loop() {
		require.NoError(b, ref.AppendCurrentParticipationBits(byte(2)))
		ref = st.Copy()
	}
}
