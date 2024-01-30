package transition

import (
	"context"
	"testing"

	fuzz "github.com/google/gofuzz"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestGenesisBeaconState_1000(t *testing.T) {
	SkipSlotCache.Disable()
	defer SkipSlotCache.Enable()
	fuzzer := fuzz.NewWithSeed(0)
	fuzzer.NilChance(0.1)
	deposits := make([]*zondpb.Deposit, 300000)
	var genesisTime uint64
	eth1Data := &zondpb.Eth1Data{}
	for i := 0; i < 1000; i++ {
		fuzzer.Fuzz(&deposits)
		fuzzer.Fuzz(&genesisTime)
		fuzzer.Fuzz(eth1Data)
		gs, err := GenesisBeaconState(context.Background(), deposits, genesisTime, eth1Data)
		if err != nil {
			if gs != nil {
				t.Fatalf("Genesis state should be nil on err. found: %v on error: %v for inputs deposit: %v "+
					"genesis time: %v eth1data: %v", gs, err, deposits, genesisTime, eth1Data)
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
	preState, err := state_native.InitializeFromProtoUnsafePhase0(&zondpb.BeaconState{})
	require.NoError(t, err)
	eth1Data := &zondpb.Eth1Data{}
	for i := 0; i < 1000; i++ {
		fuzzer.Fuzz(&genesisTime)
		fuzzer.Fuzz(eth1Data)
		fuzzer.Fuzz(preState)
		gs, err := OptimizedGenesisBeaconState(genesisTime, preState, eth1Data)
		if err != nil {
			if gs != nil {
				t.Fatalf("Genesis state should be nil on err. found: %v on error: %v for inputs genesis time: %v "+
					"pre state: %v eth1data: %v", gs, err, genesisTime, preState, eth1Data)
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
	for i := 0; i < 100000; i++ {
		fuzzer.Fuzz(&chainStartDepositCount)
		fuzzer.Fuzz(&currentTime)
		IsValidGenesisState(chainStartDepositCount, currentTime)
	}
}
