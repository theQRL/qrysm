package stateutils_test

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v4/beacon-chain/core/transition/stateutils"
	state_native "github.com/prysmaticlabs/prysm/v4/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v4/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v4/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v4/testing/assert"
	"github.com/prysmaticlabs/prysm/v4/testing/require"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
)

func TestValidatorIndexMap_OK(t *testing.T) {
	base := &ethpb.BeaconState{
		Validators: []*ethpb.Validator{
			{
				PublicKey: []byte("zero"),
			},
			{
				PublicKey: []byte("one"),
			},
		},
	}
	state, err := state_native.InitializeFromProtoPhase0(base)
	require.NoError(t, err)

	tests := []struct {
		key [dilithium2.CryptoPublicKeyBytes]byte
		val primitives.ValidatorIndex
		ok  bool
	}{
		{
			key: bytesutil.ToBytes2592([]byte("zero")),
			val: 0,
			ok:  true,
		}, {
			key: bytesutil.ToBytes2592([]byte("one")),
			val: 1,
			ok:  true,
		}, {
			key: bytesutil.ToBytes2592([]byte("no")),
			val: 0,
			ok:  false,
		},
	}

	m := stateutils.ValidatorIndexMap(state.Validators())
	for _, tt := range tests {
		result, ok := m[tt.key]
		assert.Equal(t, tt.val, result)
		assert.Equal(t, tt.ok, ok)
	}
}
