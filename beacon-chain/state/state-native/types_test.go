package state_native_test

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	log "github.com/sirupsen/logrus"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	statenative "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/interop"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"google.golang.org/protobuf/proto"
)

func TestBeaconState_ProtoBeaconStateCompatibility(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx := context.Background()
	genesis := setupGenesisState(t, 64)
	customState, err := statenative.InitializeFromProtoPhase0(genesis)
	require.NoError(t, err)
	cloned, ok := proto.Clone(genesis).(*zondpb.BeaconState)
	assert.Equal(t, true, ok, "Object is not of type *zondpb.BeaconState")
	custom := customState.ToProto()
	assert.DeepSSZEqual(t, cloned, custom)

	r1, err := customState.HashTreeRoot(ctx)
	require.NoError(t, err)
	beaconState, err := statenative.InitializeFromProtoPhase0(genesis)
	require.NoError(t, err)
	r2, err := beaconState.HashTreeRoot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, r1, r2, "Mismatched roots")

	// We then write to the state and compare hash tree roots again.
	balances := genesis.Balances
	balances[0] = 3823
	require.NoError(t, customState.SetBalances(balances))
	r1, err = customState.HashTreeRoot(ctx)
	require.NoError(t, err)
	genesis.Balances = balances
	beaconState, err = statenative.InitializeFromProtoPhase0(genesis)
	require.NoError(t, err)
	r2, err = beaconState.HashTreeRoot(context.Background())
	require.NoError(t, err)
	assert.Equal(t, r1, r2, "Mismatched roots")
}

func setupGenesisState(tb testing.TB, count uint64) *zondpb.BeaconState {
	genesisState, _, err := interop.GenerateGenesisState(context.Background(), 0, count)
	require.NoError(tb, err, "Could not generate genesis beacon state")
	for i := uint64(1); i < count; i++ {
		var someRoot [32]byte
		var someKey [dilithium2.CryptoPublicKeyBytes]byte
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

func BenchmarkCloneValidators_Proto(b *testing.B) {
	b.StopTimer()
	validators := make([]*zondpb.Validator, 16384)
	somePubKey := [dilithium2.CryptoPublicKeyBytes]byte{1, 2, 3}
	someRoot := [32]byte{3, 4, 5}
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			PublicKey:                  somePubKey[:],
			WithdrawalCredentials:      someRoot[:],
			EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			Slashed:                    false,
			ActivationEligibilityEpoch: params.BeaconConfig().FarFutureEpoch,
			ActivationEpoch:            3,
			ExitEpoch:                  4,
			WithdrawableEpoch:          5,
		}
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		cloneValidatorsWithProto(validators)
	}
}

func BenchmarkCloneValidators_Manual(b *testing.B) {
	b.StopTimer()
	validators := make([]*zondpb.Validator, 16384)
	somePubKey := [dilithium2.CryptoPublicKeyBytes]byte{1, 2, 3}
	someRoot := [32]byte{3, 4, 5}
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			PublicKey:                  somePubKey[:],
			WithdrawalCredentials:      someRoot[:],
			EffectiveBalance:           params.BeaconConfig().MaxEffectiveBalance,
			Slashed:                    false,
			ActivationEligibilityEpoch: params.BeaconConfig().FarFutureEpoch,
			ActivationEpoch:            3,
			ExitEpoch:                  4,
			WithdrawableEpoch:          5,
		}
	}
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		cloneValidatorsManually(validators)
	}
}

func BenchmarkStateClone_Proto(b *testing.B) {
	b.StopTimer()
	params.SetupTestConfigCleanup(b)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())
	genesis := setupGenesisState(b, 64)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_, ok := proto.Clone(genesis).(*zondpb.BeaconState)
		assert.Equal(b, true, ok, "Entity is not of type *zondpb.BeaconState")
	}
}

func BenchmarkStateClone_Manual(b *testing.B) {
	b.StopTimer()
	params.SetupTestConfigCleanup(b)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())
	genesis := setupGenesisState(b, 64)
	st, err := statenative.InitializeFromProtoPhase0(genesis)
	require.NoError(b, err)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_ = st.ToProto()
	}
}

func cloneValidatorsWithProto(vals []*zondpb.Validator) []*zondpb.Validator {
	var ok bool
	res := make([]*zondpb.Validator, len(vals))
	for i := 0; i < len(res); i++ {
		res[i], ok = proto.Clone(vals[i]).(*zondpb.Validator)
		if !ok {
			log.Debug("Entity is not of type *zondpb.Validator")
		}
	}
	return res
}

func cloneValidatorsManually(vals []*zondpb.Validator) []*zondpb.Validator {
	res := make([]*zondpb.Validator, len(vals))
	for i := 0; i < len(res); i++ {
		val := vals[i]
		res[i] = &zondpb.Validator{
			PublicKey:                  val.PublicKey,
			WithdrawalCredentials:      val.WithdrawalCredentials,
			EffectiveBalance:           val.EffectiveBalance,
			Slashed:                    val.Slashed,
			ActivationEligibilityEpoch: val.ActivationEligibilityEpoch,
			ActivationEpoch:            val.ActivationEpoch,
			ExitEpoch:                  val.ExitEpoch,
			WithdrawableEpoch:          val.WithdrawableEpoch,
		}
	}
	return res
}

func TestBeaconState_ImmutabilityWithSharedResources(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())
	genesis := setupGenesisState(t, 64)
	a, err := statenative.InitializeFromProtoPhase0(genesis)
	require.NoError(t, err)
	b := a.Copy()

	// Randao mixes
	require.DeepEqual(t, a.RandaoMixes(), b.RandaoMixes(), "Test precondition failed, fields are not equal")
	require.NoError(t, a.UpdateRandaoMixesAtIndex(1, bytesutil.ToBytes32([]byte("foo"))))
	if reflect.DeepEqual(a.RandaoMixes(), b.RandaoMixes()) {
		t.Error("Expect a.RandaoMixes() to be different from b.RandaoMixes()")
	}

	// Validators
	require.DeepEqual(t, a.Validators(), b.Validators(), "Test precondition failed, fields are not equal")
	require.NoError(t, a.UpdateValidatorAtIndex(1, &zondpb.Validator{Slashed: true}))
	if reflect.DeepEqual(a.Validators(), b.Validators()) {
		t.Error("Expect a.Validators() to be different from b.Validators()")
	}

	// State Roots
	require.DeepEqual(t, a.StateRoots(), b.StateRoots(), "Test precondition failed, fields are not equal")
	require.NoError(t, a.UpdateStateRootAtIndex(1, bytesutil.ToBytes32([]byte("foo"))))
	if reflect.DeepEqual(a.StateRoots(), b.StateRoots()) {
		t.Fatal("Expected a.StateRoots() to be different from b.StateRoots()")
	}

	// Block Roots
	require.DeepEqual(t, a.BlockRoots(), b.BlockRoots(), "Test precondition failed, fields are not equal")
	require.NoError(t, a.UpdateBlockRootAtIndex(1, bytesutil.ToBytes32([]byte("foo"))))
	if reflect.DeepEqual(a.BlockRoots(), b.BlockRoots()) {
		t.Fatal("Expected a.BlockRoots() to be different from b.BlockRoots()")
	}
}

func TestForkManualCopy_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())
	genesis := setupGenesisState(t, 64)
	a, err := statenative.InitializeFromProtoPhase0(genesis)
	require.NoError(t, err)
	wantedFork := &zondpb.Fork{
		PreviousVersion: []byte{'a', 'b', 'c'},
		CurrentVersion:  []byte{'d', 'e', 'f'},
		Epoch:           0,
	}
	require.NoError(t, a.SetFork(wantedFork))

	pbState, err := statenative.ProtobufBeaconStatePhase0(a.ToProtoUnsafe())
	require.NoError(t, err)
	require.DeepEqual(t, pbState.Fork, wantedFork)
}
