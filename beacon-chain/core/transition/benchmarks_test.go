package transition_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	coreState "github.com/theQRL/qrysm/v4/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/benchmark"
	"github.com/theQRL/qrysm/v4/testing/require"
	"google.golang.org/protobuf/proto"
)

var runAmount = 25

func BenchmarkExecuteStateTransition_FullBlock(b *testing.B) {
	undo, err := benchmark.SetBenchmarkConfig()
	require.NoError(b, err)
	defer undo()
	beaconState, err := benchmark.PreGenState1Epoch()
	require.NoError(b, err)
	cleanStates := clonedStates(beaconState)
	block, err := benchmark.PreGenFullBlock()
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wsb, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(b, err)
		_, err = coreState.ExecuteStateTransition(context.Background(), cleanStates[i], wsb)
		require.NoError(b, err)
	}
}

func BenchmarkExecuteStateTransition_WithCache(b *testing.B) {
	undo, err := benchmark.SetBenchmarkConfig()
	require.NoError(b, err)
	defer undo()

	beaconState, err := benchmark.PreGenState1Epoch()
	require.NoError(b, err)
	cleanStates := clonedStates(beaconState)
	block, err := benchmark.PreGenFullBlock()
	require.NoError(b, err)

	// We have to reset slot back to last epoch to hydrate cache. Since
	// some attestations in block are from previous epoch
	currentSlot := beaconState.Slot()
	require.NoError(b, beaconState.SetSlot(beaconState.Slot()-params.BeaconConfig().SlotsPerEpoch))
	require.NoError(b, helpers.UpdateCommitteeCache(context.Background(), beaconState, time.CurrentEpoch(beaconState)))
	require.NoError(b, beaconState.SetSlot(currentSlot))
	// Run the state transition once to populate the cache.
	wsb, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(b, err)
	_, err = coreState.ExecuteStateTransition(context.Background(), beaconState, wsb)
	require.NoError(b, err, "Failed to process block, benchmarks will fail")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wsb, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(b, err)
		_, err = coreState.ExecuteStateTransition(context.Background(), cleanStates[i], wsb)
		require.NoError(b, err, "Failed to process block, benchmarks will fail")
	}
}

func BenchmarkHashTreeRoot_FullState(b *testing.B) {
	beaconState, err := benchmark.PreGenstateFullEpochs()
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := beaconState.HashTreeRoot(context.Background())
		require.NoError(b, err)
	}
}

func BenchmarkHashTreeRootState_FullState(b *testing.B) {
	beaconState, err := benchmark.PreGenstateFullEpochs()
	require.NoError(b, err)

	ctx := context.Background()

	// Hydrate the HashTreeRootState cache.
	_, err = beaconState.HashTreeRoot(ctx)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := beaconState.HashTreeRoot(ctx)
		require.NoError(b, err)
	}
}

func BenchmarkMarshalState_FullState(b *testing.B) {
	beaconState, err := benchmark.PreGenstateFullEpochs()
	require.NoError(b, err)
	natState, err := state_native.ProtobufBeaconStateCapella(beaconState.ToProtoUnsafe())
	require.NoError(b, err)
	b.Run("Proto_Marshal", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := proto.Marshal(natState)
			require.NoError(b, err)
		}
	})

	b.Run("Fast_SSZ_Marshal", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := natState.MarshalSSZ()
			require.NoError(b, err)
		}
	})
}

func BenchmarkUnmarshalState_FullState(b *testing.B) {
	beaconState, err := benchmark.PreGenstateFullEpochs()
	require.NoError(b, err)
	natState, err := state_native.ProtobufBeaconStateCapella(beaconState.ToProtoUnsafe())
	require.NoError(b, err)
	protoObject, err := proto.Marshal(natState)
	require.NoError(b, err)
	sszObject, err := natState.MarshalSSZ()
	require.NoError(b, err)

	b.Run("Proto_Unmarshal", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			require.NoError(b, proto.Unmarshal(protoObject, &zondpb.BeaconStateCapella{}))
		}
	})

	b.Run("Fast_SSZ_Unmarshal", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sszState := &zondpb.BeaconStateCapella{}
			require.NoError(b, sszState.UnmarshalSSZ(sszObject))
		}
	})
}

func clonedStates(beaconState state.BeaconState) []state.BeaconState {
	clonedStates := make([]state.BeaconState, runAmount)
	for i := 0; i < runAmount; i++ {
		clonedStates[i] = beaconState.Copy()
	}
	return clonedStates
}
