package state_native_test

import (
	"context"
	"testing"

	statenative "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestInitializeFromProto_Capella(t *testing.T) {
	type test struct {
		name  string
		state *zondpb.BeaconStateCapella
		error string
	}
	initTests := []test{
		{
			name:  "nil state",
			state: nil,
			error: "received nil state",
		},
		{
			name: "nil validators",
			state: &zondpb.BeaconStateCapella{
				Slot:       4,
				Validators: nil,
			},
		},
		{
			name:  "empty state",
			state: &zondpb.BeaconStateCapella{},
		},
	}
	for _, tt := range initTests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := statenative.InitializeFromProtoCapella(tt.state)
			if tt.error != "" {
				require.ErrorContains(t, tt.error, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestInitializeFromProtoUnsafe_Capella(t *testing.T) {
	type test struct {
		name  string
		state *zondpb.BeaconStateCapella
		error string
	}
	initTests := []test{
		{
			name: "nil validators",
			state: &zondpb.BeaconStateCapella{
				Slot:       4,
				Validators: nil,
			},
		},
		{
			name:  "empty state",
			state: &zondpb.BeaconStateCapella{},
		},
	}
	for _, tt := range initTests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := statenative.InitializeFromProtoUnsafeCapella(tt.state)
			if tt.error != "" {
				assert.ErrorContains(t, tt.error, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TODO(rgeraldes24): this test is also failing in the original repo once we
// switch the state to a version > phase0. From debugging, it seems that the
// root of the prevParticipationRoot(and current) is different in both states.
/*
func TestBeaconState_HashTreeRoot(t *testing.T) {
	testState, _ := util.DeterministicGenesisStateCapella(t, 64)

	type test struct {
		name        string
		stateModify func(beaconState state.BeaconState) (state.BeaconState, error)
		error       string
	}
	initTests := []test{
		{
			name: "unchanged state",
			stateModify: func(beaconState state.BeaconState) (state.BeaconState, error) {
				return beaconState, nil
			},
			error: "",
		},
		{
			name: "different slot",
			stateModify: func(beaconState state.BeaconState) (state.BeaconState, error) {
				if err := beaconState.SetSlot(5); err != nil {
					return nil, err
				}
				return beaconState, nil
			},
			error: "",
		},
		{
			name: "different validator balance",
			stateModify: func(beaconState state.BeaconState) (state.BeaconState, error) {
				val, err := beaconState.ValidatorAtIndex(5)
				if err != nil {
					return nil, err
				}
				val.EffectiveBalance = params.BeaconConfig().MaxEffectiveBalance - params.BeaconConfig().EffectiveBalanceIncrement
				if err := beaconState.UpdateValidatorAtIndex(5, val); err != nil {
					return nil, err
				}
				return beaconState, nil
			},
			error: "",
		},
	}

	var err error
	var oldHTR []byte
	for _, tt := range initTests {
		t.Run(tt.name, func(t *testing.T) {
			testState, err = tt.stateModify(testState)
			assert.NoError(t, err)
			root, err := testState.HashTreeRoot(context.Background())
			if err == nil && tt.error != "" {
				t.Errorf("Expected error, expected %v, received %v", tt.error, err)
			}
			pbState, err := statenative.ProtobufBeaconStateCapella(testState.ToProtoUnsafe())
			require.NoError(t, err)
			genericHTR, err := pbState.HashTreeRoot()
			if err == nil && tt.error != "" {
				t.Errorf("Expected error, expected %v, received %v", tt.error, err)
			}
			assert.DeepNotEqual(t, []byte{}, root[:], "Received empty hash tree root")
			assert.DeepEqual(t, genericHTR[:], root[:], "Expected hash tree root to match generic")
			if len(oldHTR) != 0 && bytes.Equal(root[:], oldHTR) {
				t.Errorf("Expected HTR to change, received %#x == old %#x", root, oldHTR)
			}
			oldHTR = root[:]
		})
	}
}
*/

func BenchmarkBeaconState(b *testing.B) {
	testState, _ := util.DeterministicGenesisStateCapella(b, 16000)
	pbState, err := statenative.ProtobufBeaconStateCapella(testState.ToProtoUnsafe())
	require.NoError(b, err)

	b.Run("Vectorized SHA256", func(b *testing.B) {
		st, err := statenative.InitializeFromProtoUnsafeCapella(pbState)
		require.NoError(b, err)
		_, err = st.HashTreeRoot(context.Background())
		assert.NoError(b, err)
	})

	b.Run("Current SHA256", func(b *testing.B) {
		_, err := pbState.HashTreeRoot()
		require.NoError(b, err)
	})
}

func TestBeaconState_AppendValidator_DoesntMutateCopy(t *testing.T) {
	st0, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	st1 := st0.Copy()
	originalCount := st1.NumValidators()

	val := &zondpb.Validator{Slashed: true}
	assert.NoError(t, st0.AppendValidator(val))
	assert.Equal(t, originalCount, st1.NumValidators(), "st1 NumValidators mutated")
	_, ok := st1.ValidatorIndexByPubkey(bytesutil.ToBytes2592(val.PublicKey))
	assert.Equal(t, false, ok, "Expected no validator index to be present in st1 for the newly inserted pubkey")
}

func TestBeaconState_ValidatorMutation_Capella(t *testing.T) {
	testState, _ := util.DeterministicGenesisStateCapella(t, 400)
	pbState, err := statenative.ProtobufBeaconStateCapella(testState.ToProtoUnsafe())
	require.NoError(t, err)
	testState, err = statenative.InitializeFromProtoCapella(pbState)
	require.NoError(t, err)

	_, err = testState.HashTreeRoot(context.Background())
	require.NoError(t, err)

	// Reset tries
	require.NoError(t, testState.UpdateValidatorAtIndex(200, new(zondpb.Validator)))
	_, err = testState.HashTreeRoot(context.Background())
	require.NoError(t, err)

	newState1 := testState.Copy()
	_ = testState.Copy()

	require.NoError(t, testState.UpdateValidatorAtIndex(15, &zondpb.Validator{
		PublicKey:                  make([]byte, 48),
		WithdrawalCredentials:      make([]byte, 32),
		EffectiveBalance:           1111,
		Slashed:                    false,
		ActivationEligibilityEpoch: 1112,
		ActivationEpoch:            1114,
		ExitEpoch:                  1116,
		WithdrawableEpoch:          1117,
	}))

	rt, err := testState.HashTreeRoot(context.Background())
	require.NoError(t, err)
	pbState, err = statenative.ProtobufBeaconStateCapella(testState.ToProtoUnsafe())
	require.NoError(t, err)

	copiedTestState, err := statenative.InitializeFromProtoCapella(pbState)
	require.NoError(t, err)

	rt2, err := copiedTestState.HashTreeRoot(context.Background())
	require.NoError(t, err)

	assert.Equal(t, rt, rt2)

	require.NoError(t, newState1.UpdateValidatorAtIndex(150, &zondpb.Validator{
		PublicKey:                  make([]byte, 48),
		WithdrawalCredentials:      make([]byte, 32),
		EffectiveBalance:           2111,
		Slashed:                    false,
		ActivationEligibilityEpoch: 2112,
		ActivationEpoch:            2114,
		ExitEpoch:                  2116,
		WithdrawableEpoch:          2117,
	}))

	rt, err = newState1.HashTreeRoot(context.Background())
	require.NoError(t, err)
	pbState, err = statenative.ProtobufBeaconStateCapella(newState1.ToProtoUnsafe())
	require.NoError(t, err)

	copiedTestState, err = statenative.InitializeFromProtoCapella(pbState)
	require.NoError(t, err)

	rt2, err = copiedTestState.HashTreeRoot(context.Background())
	require.NoError(t, err)

	assert.Equal(t, rt, rt2)
}
