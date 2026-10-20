package altair_test

import (
	"context"
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/container/trie"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/crypto/rand"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProcessDeposits_SameValidatorMultipleDepositsSameBlock(t *testing.T) {
	// Same validator created 3 valid deposits within the same block
	dep, _, err := util.DeterministicDepositsAndKeysSameValidator(3)
	require.NoError(t, err)
	executionData, err := util.DeterministicExecutionData(len(dep))
	require.NoError(t, err)
	registry := []*qrysmpb.Validator{
		{
			PublicKey:           []byte{1},
			WithdrawalRecipient: []byte{1, 2, 3},
		},
	}
	balances := []uint64{0}
	beaconState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators:    registry,
		Balances:      balances,
		ExecutionData: executionData,
		Fork: &qrysmpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		},
	})
	require.NoError(t, err)
	newState, err := altair.ProcessDeposits(context.Background(), beaconState, []*qrysmpb.Deposit{dep[0], dep[1], dep[2]})
	require.NoError(t, err, "Expected block deposits to process correctly")
	require.Equal(t, 2, len(newState.Validators()), "Incorrect validator count")
}

func TestProcessDeposits_MerkleBranchFailsVerification(t *testing.T) {
	deposit := &qrysmpb.Deposit{
		Data: &qrysmpb.Deposit_Data{
			PublicKey:           bytesutil.PadTo([]byte{1, 2, 3}, 2592),
			WithdrawalRecipient: make([]byte, 64),
			Signature:           make([]byte, 4627),

			RandaoCommitment: make([]byte, 32),
		},
	}
	leaf, err := deposit.Data.HashTreeRoot()
	require.NoError(t, err)

	// We then create a merkle branch for the test.
	depositTrie, err := trie.GenerateTrieFromItems([][]byte{leaf[:]}, params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not generate trie")
	proof, err := depositTrie.MerkleProof(0)
	require.NoError(t, err, "Could not generate proof")

	deposit.Proof = proof
	beaconState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		ExecutionData: &qrysmpb.ExecutionData{
			DepositRoot: []byte{0},
			BlockHash:   []byte{1},
		},
	})
	require.NoError(t, err)
	want := "deposit root did not verify"
	_, err = altair.ProcessDeposits(context.Background(), beaconState, []*qrysmpb.Deposit{deposit})
	require.ErrorContains(t, want, err)
}

func TestProcessDeposits_RejectedKeysDoNotAccumulate(t *testing.T) {
	// Keep this test serial: heap measurements cover the whole process. Inputs
	// and temporary states leave scope before each post-GC measurement.
	processRejected := func(count uint64) {
		beaconState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
			ExecutionData: &qrysmpb.ExecutionData{
				DepositRoot:  make([]byte, 32),
				DepositCount: 1,
				BlockHash:    make([]byte, 32),
			},
		})
		require.NoError(t, err)
		deposit := &qrysmpb.Deposit{Data: &qrysmpb.Deposit_Data{
			PublicKey:           make([]byte, field_params.MLDSA87PubkeyLength),
			WithdrawalRecipient: make([]byte, 64),
			Signature:           make([]byte, field_params.MLDSA87SignatureLength),
			RandaoCommitment:    make([]byte, 32),
		}}
		// A fresh prefix prevents keys from a previous run satisfying cache hits.
		_, err = rand.NewGenerator().Read(deposit.Data.PublicKey[8:40])
		require.NoError(t, err)
		for i := range count {
			binary.LittleEndian.PutUint64(deposit.Data.PublicKey[:8], i)
			_, err := altair.ProcessDeposits(context.Background(), beaconState, []*qrysmpb.Deposit{deposit})
			require.ErrorContains(t, "deposit root did not verify", err)
		}
		require.Equal(t, 0, beaconState.NumValidators())
		require.Equal(t, uint64(0), beaconState.ExecutionDepositIndex())
	}
	heapAlloc := func() uint64 {
		runtime.GC()
		runtime.GC()
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		return stats.HeapAlloc
	}

	processRejected(1) // Warm up signature verification and error handling.
	before := heapAlloc()
	const count = 4096
	processRejected(count)
	after := heapAlloc()
	t.Logf("heap after GC: before=%d bytes, after=%d bytes", before, after)
	// The former cache retained over 20 MiB for these keys. Allow ample runtime
	// bookkeeping headroom while rejecting growth proportional to key count.
	const maxRetained = 8 << 20
	if after > before+maxRetained {
		t.Fatalf("%d rejected deposit keys retained %d bytes, limit %d", count, after-before, maxRetained)
	}
}

func TestProcessDeposits_AddsNewValidatorDeposit(t *testing.T) {
	dep, _, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)
	executionData, err := util.DeterministicExecutionData(len(dep))
	require.NoError(t, err)

	registry := []*qrysmpb.Validator{
		{
			PublicKey:           []byte{1},
			WithdrawalRecipient: []byte{1, 2, 3},
		},
	}
	balances := []uint64{0}
	beaconState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators:    registry,
		Balances:      balances,
		ExecutionData: executionData,
		Fork: &qrysmpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		},
	})
	require.NoError(t, err)
	newState, err := altair.ProcessDeposits(context.Background(), beaconState, []*qrysmpb.Deposit{dep[0]})
	require.NoError(t, err, "Expected block deposits to process correctly")
	if newState.Balances()[1] != dep[0].Data.Amount {
		t.Errorf(
			"Expected state validator balances index 0 to equal %d, received %d",
			dep[0].Data.Amount,
			newState.Balances()[1],
		)
	}
}

func TestProcessDeposits_RepeatedDeposit_IncreasesValidatorBalance(t *testing.T) {
	sk, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	deposit := &qrysmpb.Deposit{
		Data: &qrysmpb.Deposit_Data{
			PublicKey:           sk.PublicKey().Marshal(),
			Amount:              1000,
			WithdrawalRecipient: make([]byte, 64),
			Signature:           make([]byte, 4627),

			RandaoCommitment: make([]byte, 32),
		},
	}
	sr, err := signing.ComputeSigningRoot(deposit.Data, bytesutil.ToBytes(3, 32))
	require.NoError(t, err)
	sig, err := sk.Sign(sr[:])
	require.NoError(t, err)
	deposit.Data.Signature = sig.Marshal()
	leaf, err := deposit.Data.HashTreeRoot()
	require.NoError(t, err)

	// We then create a merkle branch for the test.
	depositTrie, err := trie.GenerateTrieFromItems([][]byte{leaf[:]}, params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not generate trie")
	proof, err := depositTrie.MerkleProof(0)
	require.NoError(t, err, "Could not generate proof")

	deposit.Proof = proof
	registry := []*qrysmpb.Validator{
		{
			PublicKey: []byte{1, 2, 3},
		},
		{
			PublicKey:           sk.PublicKey().Marshal(),
			WithdrawalRecipient: []byte{1},
		},
	}
	balances := []uint64{0, 50}
	root, err := depositTrie.HashTreeRoot()
	require.NoError(t, err)
	beaconState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: registry,
		Balances:   balances,
		ExecutionData: &qrysmpb.ExecutionData{
			DepositRoot: root[:],
			BlockHash:   root[:],
		},
	})
	require.NoError(t, err)
	newState, err := altair.ProcessDeposits(context.Background(), beaconState, []*qrysmpb.Deposit{deposit})
	require.NoError(t, err, "Process deposit failed")
	require.Equal(t, uint64(1000+50), newState.Balances()[1], "Expected balance at index 1 to be 1050")
}

func TestProcessDeposit_AddsNewValidatorDeposit(t *testing.T) {
	// Similar to TestProcessDeposits_AddsNewValidatorDeposit except that this test directly calls ProcessDeposit
	dep, _, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)
	executionData, err := util.DeterministicExecutionData(len(dep))
	require.NoError(t, err)

	registry := []*qrysmpb.Validator{
		{
			PublicKey:           []byte{1},
			WithdrawalRecipient: []byte{1, 2, 3},
		},
	}
	balances := []uint64{0}
	beaconState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators:    registry,
		Balances:      balances,
		ExecutionData: executionData,
		Fork: &qrysmpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		},
	})
	require.NoError(t, err)
	newState, err := altair.ProcessDeposit(beaconState, dep[0], true)
	require.NoError(t, err, "Process deposit failed")
	require.Equal(t, 2, len(newState.Validators()), "Expected validator list to have length 2")
	require.Equal(t, 2, len(newState.Balances()), "Expected validator balances list to have length 2")
	if newState.Balances()[1] != dep[0].Data.Amount {
		t.Errorf(
			"Expected state validator balances index 1 to equal %d, received %d",
			dep[0].Data.Amount,
			newState.Balances()[1],
		)
	}
}

func TestProcessDeposit_SkipsInvalidDeposit(t *testing.T) {
	// Same test settings as in TestProcessDeposit_AddsNewValidatorDeposit, except that we use an invalid signature
	dep, _, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)
	dep[0].Data.Signature = make([]byte, field_params.MLDSA87SignatureLength)
	dt, _, err := util.DepositTrieFromDeposits(dep)
	require.NoError(t, err)
	root, err := dt.HashTreeRoot()
	require.NoError(t, err)
	executionData := &qrysmpb.ExecutionData{
		DepositRoot:  root[:],
		DepositCount: 1,
	}
	registry := []*qrysmpb.Validator{
		{
			PublicKey:           []byte{1},
			WithdrawalRecipient: []byte{1, 2, 3},
		},
	}
	balances := []uint64{0}
	beaconState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators:    registry,
		Balances:      balances,
		ExecutionData: executionData,
		Fork: &qrysmpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		},
	})
	require.NoError(t, err)
	newState, err := altair.ProcessDeposit(beaconState, dep[0], true)
	require.NoError(t, err, "Expected invalid block deposit to be ignored without error")

	if newState.ExecutionDepositIndex() != 1 {
		t.Errorf(
			"Expected ExecutionDepositIndex to be increased by 1 after processing an invalid deposit, received change: %v",
			newState.ExecutionDepositIndex(),
		)
	}
	if len(newState.Validators()) != 1 {
		t.Errorf("Expected validator list to have length 1, received: %v", len(newState.Validators()))
	}
	if len(newState.Balances()) != 1 {
		t.Errorf("Expected validator balances list to have length 1, received: %v", len(newState.Balances()))
	}
	if newState.Balances()[0] != 0 {
		t.Errorf("Expected validator balance at index 0 to stay 0, received: %v", newState.Balances()[0])
	}
}
