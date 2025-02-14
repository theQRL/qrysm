package util

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/testing/require"
)

func TestDeterministicGenesisState_100Validators(t *testing.T) {
	validatorCount := uint64(100)
	beaconState, privKeys := DeterministicGenesisStateCapella(t, validatorCount)
	activeValidators, err := helpers.ActiveValidatorCount(context.Background(), beaconState, 0)
	require.NoError(t, err)

	// lint:ignore uintcast -- test code
	if len(privKeys) != int(validatorCount) {
		t.Fatalf("expected amount of private keys %d to match requested amount of validators %d", len(privKeys), validatorCount)
	}
	if activeValidators != validatorCount {
		t.Fatalf("expected validators in state %d to match requested amount %d", activeValidators, validatorCount)
	}
}

func TestDeterministicGenesisStateCapella(t *testing.T) {
	st, k := DeterministicGenesisStateCapella(t, params.BeaconConfig().MaxCommitteesPerSlot)
	require.Equal(t, params.BeaconConfig().MaxCommitteesPerSlot, uint64(len(k)))
	require.Equal(t, params.BeaconConfig().MaxCommitteesPerSlot, uint64(st.NumValidators()))
}

func TestGenesisBeaconStateCapella(t *testing.T) {
	ctx := context.Background()
	deposits, _, err := DeterministicDepositsAndKeys(params.BeaconConfig().MaxCommitteesPerSlot)
	require.NoError(t, err)
	eth1Data, err := DeterministicEth1Data(len(deposits))
	require.NoError(t, err)
	gt := uint64(10000)
	st, err := GenesisBeaconStateCapella(ctx, deposits, gt, eth1Data)
	require.NoError(t, err)
	require.Equal(t, gt, st.GenesisTime())
	require.Equal(t, params.BeaconConfig().MaxCommitteesPerSlot, uint64(st.NumValidators()))
}
