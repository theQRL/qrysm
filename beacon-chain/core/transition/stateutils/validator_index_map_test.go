package stateutils_test

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/transition/stateutils"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestValidatorIndexMap_OK(t *testing.T) {
	base := &zondpb.BeaconStateCapella{
		Validators: []*zondpb.Validator{
			{
				PublicKey: []byte("zero"),
			},
			{
				PublicKey: []byte("one"),
			},
		},
	}
	state, err := state_native.InitializeFromProtoCapella(base)
	require.NoError(t, err)

	tests := []struct {
		key [field_params.DilithiumPubkeyLength]byte
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
