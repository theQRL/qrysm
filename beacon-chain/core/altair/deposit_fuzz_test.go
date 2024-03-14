package altair_test

import (
	"context"
	"testing"

	fuzz "github.com/google/gofuzz"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/altair"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestFuzzProcessDeposits_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	deposits := make([]*zondpb.Deposit, 100)
	ctx := context.Background()
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		for i := range deposits {
			fuzzer.Fuzz(deposits[i])
		}
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		r, err := altair.ProcessDeposits(ctx, s, deposits)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, deposits)
		}
	}
}

func TestFuzzProcessDeposit_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	state := &zondpb.BeaconStateCapella{}
	deposit := &zondpb.Deposit{}

	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(state)
		fuzzer.Fuzz(deposit)
		s, err := state_native.InitializeFromProtoUnsafeCapella(state)
		require.NoError(t, err)
		r, err := altair.ProcessDeposit(s, deposit, true)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, state, deposit)
		}
	}
}
