package state_native_test

import (
	"testing"

	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func BenchmarkAppendHistoricalSummaries(b *testing.B) {
	st, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{})
	require.NoError(b, err)

	max := params.BeaconConfig().HistoricalRootsLimit
	if max < 2 {
		b.Fatalf("HistoricalRootsLimit is less than 2: %d", max)
	}

	for i := uint64(0); i < max-2; i++ {
		err := st.AppendHistoricalSummaries(&zondpb.HistoricalSummary{})
		require.NoError(b, err)
	}

	ref := st.Copy()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := ref.AppendHistoricalSummaries(&zondpb.HistoricalSummary{})
		require.NoError(b, err)
		ref = st.Copy()
	}
}
