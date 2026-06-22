package transition

import (
	"context"
	"testing"

	fuzz "github.com/google/gofuzz"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestGenesisBeaconState_1000(t *testing.T) {
	SkipSlotCache.Disable()
	defer SkipSlotCache.Enable()
	fuzzer := fuzz.NewWithSeed(0)
	fuzzer.NilChance(0.1)
	deposits := make([]*qrysmpb.Deposit, 300000)
	var genesisTime uint64
	executionData := &qrysmpb.ExecutionData{}
	for range 1000 {
		fuzzer.Fuzz(&deposits)
		fuzzer.Fuzz(&genesisTime)
		fuzzer.Fuzz(executionData)
		gs, err := GenesisBeaconStateZond(context.Background(), deposits, genesisTime, executionData, &enginev1.ExecutionPayloadZond{})
		if err != nil {
			if gs != nil {
				t.Fatalf("Genesis state should be nil on err. found: %v on error: %v for inputs deposit: %v "+
					"genesis time: %v executionData: %v", gs, err, deposits, genesisTime, executionData)
			}
		}
	}
}

func TestOptimizedGenesisBeaconState_1000(t *testing.T) {
	SkipSlotCache.Disable()
	defer SkipSlotCache.Enable()
	fuzzer := fuzz.NewWithSeed(0)
	fuzzer.NilChance(0.1)
	var genesisTime uint64
	preState, err := state_native.InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	executionData := &qrysmpb.ExecutionData{}
	for range 1000 {
		fuzzer.Fuzz(&genesisTime)
		fuzzer.Fuzz(executionData)
		fuzzer.Fuzz(preState)
		gs, err := OptimizedGenesisBeaconStateZond(genesisTime, preState, executionData, &enginev1.ExecutionPayloadZond{})
		if err != nil {
			if gs != nil {
				t.Fatalf("Genesis state should be nil on err. found: %v on error: %v for inputs genesis time: %v "+
					"pre state: %v executionData: %v", gs, err, genesisTime, preState, executionData)
			}
		}
	}
}

func TestIsValidGenesisState_100000(_ *testing.T) {
	SkipSlotCache.Disable()
	defer SkipSlotCache.Enable()
	fuzzer := fuzz.NewWithSeed(0)
	fuzzer.NilChance(0.1)
	var chainStartDepositCount, currentTime uint64
	for range 100000 {
		fuzzer.Fuzz(&chainStartDepositCount)
		fuzzer.Fuzz(&currentTime)
		IsValidGenesisState(chainStartDepositCount, currentTime)
	}
}
