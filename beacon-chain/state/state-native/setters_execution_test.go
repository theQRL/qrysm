package state_native_test

import (
	"testing"

	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func BenchmarkAppendExecutionDataVotes(b *testing.B) {
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(b, err)

	max := params.BeaconConfig().ExecutionDataVotesLength()

	if max < 2 {
		b.Fatalf("ExecutionDataVotesLength is less than 2")
	}

	for i := uint64(0); i < max-2; i++ {
		err := st.AppendExecutionDataVotes(&qrysmpb.ExecutionData{
			DepositCount: i,
			DepositRoot:  make([]byte, 64),
			BlockHash:    make([]byte, 64),
		})
		require.NoError(b, err)
	}

	ref := st.Copy()

	for i := 0; b.Loop(); i++ {
		err := ref.AppendExecutionDataVotes(&qrysmpb.ExecutionData{DepositCount: uint64(i)})
		require.NoError(b, err)
		ref = st.Copy()
	}
}
