package stateutil_test

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/interop"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestState_FieldCount(t *testing.T) {
	count := params.BeaconConfig().BeaconStateCapellaFieldCount
	typ := reflect.TypeOf(zondpb.BeaconStateCapella{})
	numFields := 0
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Name == "state" ||
			typ.Field(i).Name == "sizeCache" ||
			typ.Field(i).Name == "unknownFields" {
			continue
		}
		numFields++
	}
	assert.Equal(t, count, numFields)
}

func BenchmarkHashTreeRoot_Generic_512(b *testing.B) {
	b.StopTimer()
	genesisState := setupGenesisState(b, 512)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := genesisState.HashTreeRoot()
		require.NoError(b, err)
	}
}

func BenchmarkHashTreeRoot_Generic_16384(b *testing.B) {
	b.StopTimer()
	genesisState := setupGenesisState(b, 16384)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := genesisState.HashTreeRoot()
		require.NoError(b, err)
	}
}

func BenchmarkHashTreeRoot_Generic_300000(b *testing.B) {
	b.StopTimer()
	genesisState := setupGenesisState(b, 300000)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, err := genesisState.HashTreeRoot()
		require.NoError(b, err)
	}
}

func setupGenesisState(tb testing.TB, count uint64) *zondpb.BeaconStateCapella {
	genesisState, _, err := interop.GenerateGenesisStateCapella(context.Background(), 0, 1, &enginev1.ExecutionPayloadCapella{}, &zondpb.Eth1Data{})
	require.NoError(tb, err, "Could not generate genesis beacon state")
	for i := uint64(1); i < count; i++ {
		var someRoot [32]byte
		var someKey [field_params.DilithiumPubkeyLength]byte
		copy(someRoot[:], strconv.Itoa(int(i)))
		copy(someKey[:], strconv.Itoa(int(i)))
		genesisState.Validators = append(genesisState.Validators, &zondpb.Validator{
			PublicKey:                  someKey[:],
			WithdrawalCredentials:      someRoot[:],
			EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			Slashed:                    false,
			ActivationEligibilityEpoch: 1,
			ActivationEpoch:            1,
			ExitEpoch:                  1,
			WithdrawableEpoch:          1,
		})
		genesisState.Balances = append(genesisState.Balances, params.BeaconConfig().MaxEffectiveBalance)
	}
	return genesisState
}
