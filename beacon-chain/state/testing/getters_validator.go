package testing

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type getState func() (state.BeaconState, error)

func VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t *testing.T, factory getState) {
	st, err := factory()
	require.NoError(t, err)

	_, err = st.ValidatorAtIndexReadOnly(0)
	assert.Equal(t, state.ErrNilValidatorsInState, err)
}
