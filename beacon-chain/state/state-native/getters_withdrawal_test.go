package state_native

import (
	"testing"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestNextWithdrawalIndex(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		s := BeaconState{version: version.Zond, nextWithdrawalIndex: 123}
		i, err := s.NextWithdrawalIndex()
		require.NoError(t, err)
		assert.Equal(t, uint64(123), i)
	})
}

func TestNextWithdrawalValidatorIndex(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		s := BeaconState{version: version.Zond, nextWithdrawalValidatorIndex: 123}
		i, err := s.NextWithdrawalValidatorIndex()
		require.NoError(t, err)
		assert.Equal(t, primitives.ValidatorIndex(123), i)
	})
}

func TestHasExecutionWithdrawalCredentials(t *testing.T) {
	creds := []byte{0xFA, 0xCC}
	v := &qrysmpb.Validator{WithdrawalCredentials: creds}
	require.Equal(t, false, hasExecutionWithdrawalCredential(v))
	creds = make([]byte, 64)
	v = &qrysmpb.Validator{WithdrawalCredentials: creds}
	require.Equal(t, true, hasExecutionWithdrawalCredential(v))
	// No Withdrawal cred
	v = &qrysmpb.Validator{}
	require.Equal(t, false, hasExecutionWithdrawalCredential(v))
}

func TestIsFullyWithdrawableValidator(t *testing.T) {
	// Wrong credential length
	creds := []byte{0xFA, 0xCC}
	v := &qrysmpb.Validator{
		WithdrawalCredentials: creds,
		WithdrawableEpoch:     2,
	}
	require.Equal(t, false, isFullyWithdrawableValidator(v, 3))
	// Wrong withdrawable epoch
	creds = make([]byte, 64)
	v = &qrysmpb.Validator{
		WithdrawalCredentials: creds,
		WithdrawableEpoch:     2,
	}
	require.Equal(t, false, isFullyWithdrawableValidator(v, 1))
	// Fully withdrawable
	creds = make([]byte, 64)
	v = &qrysmpb.Validator{
		WithdrawalCredentials: creds,
		WithdrawableEpoch:     2,
	}
	require.Equal(t, true, isFullyWithdrawableValidator(v, 3))
}

func TestExpectedWithdrawals(t *testing.T) {
	t.Run("no withdrawals", func(t *testing.T) {
		s := BeaconState{
			version:    version.Zond,
			validators: make([]*qrysmpb.Validator, 100),
			balances:   make([]uint64, 100),
		}
		for i := range s.validators {
			s.balances[i] = params.BeaconConfig().MaxEffectiveBalance
			val := &qrysmpb.Validator{
				WithdrawalCredentials: make([]byte, 64),
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				WithdrawableEpoch:     primitives.Epoch(1),
			}
			s.validators[i] = val
		}
		expected, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, 0, len(expected))
	})
	t.Run("one fully withdrawable", func(t *testing.T) {
		s := BeaconState{
			version:                      version.Zond,
			validators:                   make([]*qrysmpb.Validator, 100),
			balances:                     make([]uint64, 100),
			nextWithdrawalValidatorIndex: 20,
		}
		for i := range s.validators {
			s.balances[i] = params.BeaconConfig().MaxEffectiveBalance
			val := &qrysmpb.Validator{
				WithdrawalCredentials: make([]byte, 64),
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				WithdrawableEpoch:     primitives.Epoch(1),
			}
			s.validators[i] = val
		}
		s.validators[3].WithdrawableEpoch = primitives.Epoch(0)
		expected, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, 1, len(expected))
		withdrawal := &enginev1.Withdrawal{
			Index:          0,
			ValidatorIndex: 3,
			Address:        s.validators[3].WithdrawalCredentials,
			Amount:         s.balances[3],
		}
		require.DeepEqual(t, withdrawal, expected[0])
	})
	t.Run("one partially withdrawable", func(t *testing.T) {
		s := BeaconState{
			version:    version.Zond,
			validators: make([]*qrysmpb.Validator, 100),
			balances:   make([]uint64, 100),
		}
		for i := range s.validators {
			s.balances[i] = params.BeaconConfig().MaxEffectiveBalance
			val := &qrysmpb.Validator{
				WithdrawalCredentials: make([]byte, 64),
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				WithdrawableEpoch:     primitives.Epoch(1),
			}
			s.validators[i] = val
		}
		s.balances[3] += params.BeaconConfig().MinDepositAmount
		expected, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, 1, len(expected))
		withdrawal := &enginev1.Withdrawal{
			Index:          0,
			ValidatorIndex: 3,
			Address:        s.validators[3].WithdrawalCredentials,
			Amount:         params.BeaconConfig().MinDepositAmount,
		}
		require.DeepEqual(t, withdrawal, expected[0])
	})
	t.Run("one partially and one fully withdrawable", func(t *testing.T) {
		s := BeaconState{
			version:    version.Zond,
			validators: make([]*qrysmpb.Validator, 100),
			balances:   make([]uint64, 100),
		}
		for i := range s.validators {
			s.balances[i] = params.BeaconConfig().MaxEffectiveBalance
			val := &qrysmpb.Validator{
				WithdrawalCredentials: make([]byte, 64),
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				WithdrawableEpoch:     primitives.Epoch(1),
			}
			val.WithdrawalCredentials[31] = byte(i)
			s.validators[i] = val
		}
		s.balances[3] += params.BeaconConfig().MinDepositAmount
		s.validators[7].WithdrawableEpoch = primitives.Epoch(0)
		expected, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, 2, len(expected))

		withdrawalFull := &enginev1.Withdrawal{
			Index:          1,
			ValidatorIndex: 7,
			Address:        s.validators[7].WithdrawalCredentials,
			Amount:         s.balances[7],
		}
		withdrawalPartial := &enginev1.Withdrawal{
			Index:          0,
			ValidatorIndex: 3,
			Address:        s.validators[3].WithdrawalCredentials,
			Amount:         params.BeaconConfig().MinDepositAmount,
		}
		require.DeepEqual(t, withdrawalPartial, expected[0])
		require.DeepEqual(t, withdrawalFull, expected[1])
	})
	t.Run("all partially withdrawable", func(t *testing.T) {
		s := BeaconState{
			version:    version.Zond,
			validators: make([]*qrysmpb.Validator, 100),
			balances:   make([]uint64, 100),
		}
		for i := range s.validators {
			s.balances[i] = params.BeaconConfig().MaxEffectiveBalance + 1
			val := &qrysmpb.Validator{
				WithdrawalCredentials: make([]byte, 64),
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				WithdrawableEpoch:     primitives.Epoch(1),
			}
			s.validators[i] = val
		}
		expected, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, params.BeaconConfig().MaxWithdrawalsPerPayload, uint64(len(expected)))
		withdrawal := &enginev1.Withdrawal{
			Index:          0,
			ValidatorIndex: 0,
			Address:        s.validators[0].WithdrawalCredentials,
			Amount:         1,
		}
		require.DeepEqual(t, withdrawal, expected[0])
	})
	t.Run("all fully withdrawable", func(t *testing.T) {
		s := BeaconState{
			version:    version.Zond,
			validators: make([]*qrysmpb.Validator, 100),
			balances:   make([]uint64, 100),
		}
		for i := range s.validators {
			s.balances[i] = params.BeaconConfig().MaxEffectiveBalance
			val := &qrysmpb.Validator{
				WithdrawalCredentials: make([]byte, 64),
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				WithdrawableEpoch:     primitives.Epoch(0),
			}
			s.validators[i] = val
		}
		expected, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, params.BeaconConfig().MaxWithdrawalsPerPayload, uint64(len(expected)))
		withdrawal := &enginev1.Withdrawal{
			Index:          0,
			ValidatorIndex: 0,
			Address:        s.validators[0].WithdrawalCredentials,
			Amount:         params.BeaconConfig().MaxEffectiveBalance,
		}
		require.DeepEqual(t, withdrawal, expected[0])
	})
	t.Run("all fully and partially withdrawable", func(t *testing.T) {
		s := BeaconState{
			version:    version.Zond,
			validators: make([]*qrysmpb.Validator, 100),
			balances:   make([]uint64, 100),
		}
		for i := range s.validators {
			s.balances[i] = params.BeaconConfig().MaxEffectiveBalance + 1
			val := &qrysmpb.Validator{
				WithdrawalCredentials: make([]byte, 64),
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				WithdrawableEpoch:     primitives.Epoch(0),
			}
			s.validators[i] = val
		}
		expected, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, params.BeaconConfig().MaxWithdrawalsPerPayload, uint64(len(expected)))
		withdrawal := &enginev1.Withdrawal{
			Index:          0,
			ValidatorIndex: 0,
			Address:        s.validators[0].WithdrawalCredentials,
			Amount:         params.BeaconConfig().MaxEffectiveBalance + 1,
		}
		require.DeepEqual(t, withdrawal, expected[0])
	})
	t.Run("one fully withdrawable but zero balance", func(t *testing.T) {
		s := BeaconState{
			version:                      version.Zond,
			validators:                   make([]*qrysmpb.Validator, 100),
			balances:                     make([]uint64, 100),
			nextWithdrawalValidatorIndex: 20,
		}
		for i := range s.validators {
			s.balances[i] = params.BeaconConfig().MaxEffectiveBalance
			val := &qrysmpb.Validator{
				WithdrawalCredentials: make([]byte, 64),
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				WithdrawableEpoch:     primitives.Epoch(1),
			}
			s.validators[i] = val
		}
		s.validators[3].WithdrawableEpoch = primitives.Epoch(0)
		s.balances[3] = 0
		expected, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, 0, len(expected))
	})
	t.Run("one partially withdrawable, one above sweep bound", func(t *testing.T) {
		s := BeaconState{
			version:    version.Zond,
			validators: make([]*qrysmpb.Validator, 100),
			balances:   make([]uint64, 100),
		}
		for i := range s.validators {
			s.balances[i] = params.BeaconConfig().MaxEffectiveBalance
			val := &qrysmpb.Validator{
				WithdrawalCredentials: make([]byte, 64),
				EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
				WithdrawableEpoch:     primitives.Epoch(1),
			}
			s.validators[i] = val
		}
		s.balances[3] += params.BeaconConfig().MinDepositAmount
		s.balances[10] += params.BeaconConfig().MinDepositAmount
		saved := params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep
		params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep = 10
		expected, err := s.ExpectedWithdrawals()
		require.NoError(t, err)
		require.Equal(t, 1, len(expected))
		withdrawal := &enginev1.Withdrawal{
			Index:          0,
			ValidatorIndex: 3,
			Address:        s.validators[3].WithdrawalCredentials,
			Amount:         params.BeaconConfig().MinDepositAmount,
		}
		require.DeepEqual(t, withdrawal, expected[0])
		params.BeaconConfig().MaxValidatorsPerWithdrawalsSweep = saved
	})
}
