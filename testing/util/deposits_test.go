package util

import (
	"bytes"
	"testing"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func assertDepositShape(t *testing.T, deposit *qrysmpb.Deposit, depositDataRoot [32]byte) {
	t.Helper()
	if len(deposit.Data.PublicKey) != fieldparams.MLDSA87PubkeyLength {
		t.Fatalf("incorrect public key length, wanted %d but received %d", fieldparams.MLDSA87PubkeyLength, len(deposit.Data.PublicKey))
	}
	if len(deposit.Data.WithdrawalCredentials) != fieldparams.WithdrawalCredentialsLength {
		t.Fatalf("incorrect withdrawal credentials length, wanted %d but received %d", fieldparams.WithdrawalCredentialsLength, len(deposit.Data.WithdrawalCredentials))
	}
	if len(deposit.Data.Signature) != fieldparams.MLDSA87SignatureLength {
		t.Fatalf("incorrect signature length, wanted %d but received %d", fieldparams.MLDSA87SignatureLength, len(deposit.Data.Signature))
	}
	if depositDataRoot == [32]byte{} {
		t.Fatal("expected non-zero deposit data root")
	}
}

func TestSetupInitialDeposits_1024Entries(t *testing.T) {
	entries := 1024
	resetCache()
	deposits, privKeys, err := DeterministicDepositsAndKeys(uint64(entries))
	require.NoError(t, err)
	_, depositDataRoots, err := DeterministicDepositTrie(len(deposits))
	require.NoError(t, err)

	if len(deposits) != entries {
		t.Fatalf("incorrect number of deposits returned, wanted %d but received %d", entries, len(deposits))
	}
	if len(privKeys) != entries {
		t.Fatalf("incorrect number of private keys returned, wanted %d but received %d", entries, len(privKeys))
	}
	assertDepositShape(t, deposits[0], depositDataRoots[0])
	assertDepositShape(t, deposits[1023], depositDataRoots[1023])
}

func TestDepositsWithBalance_MatchesDeterministic(t *testing.T) {
	entries := 64
	resetCache()
	balances := make([]uint64, entries)
	for i := range entries {
		balances[i] = params.BeaconConfig().MaxEffectiveBalance
	}
	deposits, depositTrie, err := DepositsWithBalance(balances)
	require.NoError(t, err)
	_, depositDataRoots, err := DepositTrieSubset(depositTrie, entries)
	require.NoError(t, err)

	determDeposits, _, err := DeterministicDepositsAndKeys(uint64(entries))
	require.NoError(t, err)

	for i := range entries {
		if !bytes.Equal(deposits[i].Data.PublicKey, determDeposits[i].Data.PublicKey) {
			t.Errorf("Expected deposit public key %d to match", i)
		}
		if deposits[i].Data.Amount != determDeposits[i].Data.Amount {
			t.Errorf("Expected deposit amount %d to match", i)
		}
		if !bytes.Equal(deposits[i].Data.WithdrawalCredentials, determDeposits[i].Data.WithdrawalCredentials) {
			t.Errorf("Expected deposit withdrawal credentials %d to match", i)
		}
		depositDataRoot, err := deposits[i].Data.HashTreeRoot()
		require.NoError(t, err)
		if !bytes.Equal(depositDataRoots[i][:], depositDataRoot[:]) {
			t.Errorf("Expected deposit root %d to match deposit data root", i)
		}
	}
}

func TestDepositsWithBalance_MatchesDeterministic_Cached(t *testing.T) {
	entries := 32
	resetCache()
	// Cache half of the deposit cache.
	_, _, err := DeterministicDepositsAndKeys(uint64(entries))
	require.NoError(t, err)
	_, _, err = DeterministicDepositTrie(entries)
	require.NoError(t, err)

	// Generate balanced deposits with half cache.
	entries = 64
	balances := make([]uint64, entries)
	for i := range entries {
		balances[i] = params.BeaconConfig().MaxEffectiveBalance
	}
	deposits, depositTrie, err := DepositsWithBalance(balances)
	require.NoError(t, err)
	_, depositDataRoots, err := DepositTrieSubset(depositTrie, entries)
	require.NoError(t, err)

	// Get 64 standard deposits.
	determDeposits, _, err := DeterministicDepositsAndKeys(uint64(entries))
	require.NoError(t, err)

	for i := range entries {
		if !bytes.Equal(deposits[i].Data.PublicKey, determDeposits[i].Data.PublicKey) {
			t.Errorf("Expected deposit public key %d to match", i)
		}
		if deposits[i].Data.Amount != determDeposits[i].Data.Amount {
			t.Errorf("Expected deposit amount %d to match", i)
		}
		if !bytes.Equal(deposits[i].Data.WithdrawalCredentials, determDeposits[i].Data.WithdrawalCredentials) {
			t.Errorf("Expected deposit withdrawal credentials %d to match", i)
		}
		depositDataRoot, err := deposits[i].Data.HashTreeRoot()
		require.NoError(t, err)
		if !bytes.Equal(depositDataRoots[i][:], depositDataRoot[:]) {
			t.Errorf("Expected deposit root %d to match deposit data root", i)
		}
	}
}

func TestSetupInitialDeposits_1024Entries_PartialDeposits(t *testing.T) {
	entries := 1024
	resetCache()
	balances := make([]uint64, entries)
	for i := range entries {
		balances[i] = params.BeaconConfig().MaxEffectiveBalance / 2
	}
	deposits, depositTrie, err := DepositsWithBalance(balances)
	require.NoError(t, err)
	_, depositDataRoots, err := DepositTrieSubset(depositTrie, entries)
	require.NoError(t, err)

	if len(deposits) != entries {
		t.Fatalf("incorrect number of deposits returned, wanted %d but received %d", entries, len(deposits))
	}
	assertDepositShape(t, deposits[0], depositDataRoots[0])
	assertDepositShape(t, deposits[1023], depositDataRoots[1023])
}

func TestDepositTrieFromDeposits(t *testing.T) {
	deposits, _, err := DeterministicDepositsAndKeys(100)
	require.NoError(t, err)
	executionData, err := DeterministicExecutionData(len(deposits))
	require.NoError(t, err)

	depositTrie, _, err := DepositTrieFromDeposits(deposits)
	require.NoError(t, err)

	root, err := depositTrie.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(root[:], executionData.DepositRoot) {
		t.Fatal("expected deposit trie root to equal executionData deposit root")
	}
}
