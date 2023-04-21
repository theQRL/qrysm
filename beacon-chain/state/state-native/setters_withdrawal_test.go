package state_native

import (
	"testing"

	"github.com/cyyber/qrysm/v4/beacon-chain/state/state-native/types"
	"github.com/cyyber/qrysm/v4/consensus-types/primitives"
	"github.com/cyyber/qrysm/v4/runtime/version"
	"github.com/cyyber/qrysm/v4/testing/require"
)

func TestSetNextWithdrawalIndex(t *testing.T) {
	s := BeaconState{
		version:             version.Capella,
		nextWithdrawalIndex: 3,
		dirtyFields:         make(map[types.FieldIndex]bool),
	}
	require.NoError(t, s.SetNextWithdrawalIndex(5))
	require.Equal(t, uint64(5), s.nextWithdrawalIndex)
	require.Equal(t, true, s.dirtyFields[types.NextWithdrawalIndex])
}

func TestSetNextWithdrawalValidatorIndex(t *testing.T) {
	s := BeaconState{
		version:                      version.Capella,
		nextWithdrawalValidatorIndex: 3,
		dirtyFields:                  make(map[types.FieldIndex]bool),
	}
	require.NoError(t, s.SetNextWithdrawalValidatorIndex(5))
	require.Equal(t, primitives.ValidatorIndex(5), s.nextWithdrawalValidatorIndex)
	require.Equal(t, true, s.dirtyFields[types.NextWithdrawalValidatorIndex])
}
