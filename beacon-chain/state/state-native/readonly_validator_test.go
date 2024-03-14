package state_native_test

import (
	"testing"

	statenative "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestReadOnlyValidator_ReturnsErrorOnNil(t *testing.T) {
	if _, err := statenative.NewValidator(nil); err != statenative.ErrNilWrappedValidator {
		t.Errorf("Wrong error returned. Got %v, wanted %v", err, statenative.ErrNilWrappedValidator)
	}
}

func TestReadOnlyValidator_EffectiveBalance(t *testing.T) {
	bal := uint64(234)
	v, err := statenative.NewValidator(&zondpb.Validator{EffectiveBalance: bal})
	require.NoError(t, err)
	assert.Equal(t, bal, v.EffectiveBalance())
}

func TestReadOnlyValidator_ActivationEligibilityEpoch(t *testing.T) {
	epoch := primitives.Epoch(234)
	v, err := statenative.NewValidator(&zondpb.Validator{ActivationEligibilityEpoch: epoch})
	require.NoError(t, err)
	assert.Equal(t, epoch, v.ActivationEligibilityEpoch())
}

func TestReadOnlyValidator_ActivationEpoch(t *testing.T) {
	epoch := primitives.Epoch(234)
	v, err := statenative.NewValidator(&zondpb.Validator{ActivationEpoch: epoch})
	require.NoError(t, err)
	assert.Equal(t, epoch, v.ActivationEpoch())
}

func TestReadOnlyValidator_WithdrawableEpoch(t *testing.T) {
	epoch := primitives.Epoch(234)
	v, err := statenative.NewValidator(&zondpb.Validator{WithdrawableEpoch: epoch})
	require.NoError(t, err)
	assert.Equal(t, epoch, v.WithdrawableEpoch())
}

func TestReadOnlyValidator_ExitEpoch(t *testing.T) {
	epoch := primitives.Epoch(234)
	v, err := statenative.NewValidator(&zondpb.Validator{ExitEpoch: epoch})
	require.NoError(t, err)
	assert.Equal(t, epoch, v.ExitEpoch())
}

func TestReadOnlyValidator_PublicKey(t *testing.T) {
	key := [field_params.DilithiumPubkeyLength]byte{0xFA, 0xCC}
	v, err := statenative.NewValidator(&zondpb.Validator{PublicKey: key[:]})
	require.NoError(t, err)
	assert.Equal(t, key, v.PublicKey())
}

func TestReadOnlyValidator_WithdrawalCredentials(t *testing.T) {
	creds := []byte{0xFA, 0xCC}
	v, err := statenative.NewValidator(&zondpb.Validator{WithdrawalCredentials: creds})
	require.NoError(t, err)
	assert.DeepEqual(t, creds, v.WithdrawalCredentials())
}

func TestReadOnlyValidator_Slashed(t *testing.T) {
	v, err := statenative.NewValidator(&zondpb.Validator{Slashed: true})
	require.NoError(t, err)
	assert.Equal(t, true, v.Slashed())
}
