package testing

import (
	"testing"

	"github.com/cyyber/qrysm/v4/beacon-chain/state"
	"github.com/cyyber/qrysm/v4/testing/assert"
	"github.com/cyyber/qrysm/v4/testing/require"
)

type getState func() (state.BeaconState, error)

func VerifyBeaconStateValidatorAtIndexReadOnlyHandlesNilSlice(t *testing.T, factory getState) {
	st, err := factory()
	require.NoError(t, err)

	_, err = st.ValidatorAtIndexReadOnly(0)
	assert.Equal(t, state.ErrNilValidatorsInState, err)
}
