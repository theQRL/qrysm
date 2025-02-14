package execution

import (
	"context"
	"fmt"
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	testing2 "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/container/trie"
	"github.com/theQRL/qrysm/crypto/dilithium"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

const pubKeyErr = "could not convert bytes to public key"

func TestDepositContractAddress_EmptyAddress(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.DepositContractAddress = ""
	params.OverrideBeaconConfig(config)

	_, err := DepositContractAddress()
	assert.ErrorContains(t, "valid deposit contract is required", err)
}

func TestDepositContractAddress_NotHexAddress(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.DepositContractAddress = "abc?!"
	params.OverrideBeaconConfig(config)

	_, err := DepositContractAddress()
	assert.ErrorContains(t, "invalid deposit contract address given", err)
}

func TestDepositContractAddress_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	addr, err := DepositContractAddress()
	require.NoError(t, err)
	assert.Equal(t, params.BeaconConfig().DepositContractAddress, addr)
}

func TestProcessDeposit_OK(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	server, endpoint, err := testing2.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "Unable to setup web3 ETH1.0 chain service")

	web3Service = setDefaultMocks(web3Service)

	deposits, _, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)

	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)

	err = web3Service.processDeposit(context.Background(), eth1Data, deposits[0])
	require.NoError(t, err, "could not process deposit")

	valcount, err := helpers.ActiveValidatorCount(context.Background(), web3Service.preGenesisState, 0)
	require.NoError(t, err)
	require.Equal(t, 1, int(valcount), "Did not get correct active validator count")
}

func TestProcessDeposit_InvalidMerkleBranch(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	server, endpoint, err := testing2.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)

	deposits, _, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)

	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)

	deposits[0].Proof = [][]byte{{'f', 'a', 'k', 'e'}}

	err = web3Service.processDeposit(context.Background(), eth1Data, deposits[0])
	require.NotNil(t, err, "No errors, when an error was expected")

	want := "deposit merkle branch of deposit root did not verify for root"

	assert.ErrorContains(t, want, err)
}

func TestProcessDeposit_UnableToVerify(t *testing.T) {
	hook := logTest.NewGlobal()
	beaconDB := testDB.SetupDB(t)
	server, endpoint, err := testing2.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)

	deposits, keys, err := util.DeterministicDepositsAndKeys(1)
	require.NoError(t, err)
	sig := keys[0].Sign([]byte{'F', 'A', 'K', 'E'})
	deposits[0].Data.Signature = sig.Marshal()

	generatedTrie, _, err := util.DepositTrieFromDeposits(deposits)
	require.NoError(t, err)
	root, err := generatedTrie.HashTreeRoot()
	require.NoError(t, err)
	eth1Data := &zondpb.Eth1Data{
		DepositCount: 1,
		DepositRoot:  root[:],
	}
	proof, err := generatedTrie.MerkleProof(0)
	require.NoError(t, err)
	deposits[0].Proof = proof
	err = web3Service.processDeposit(context.Background(), eth1Data, deposits[0])
	require.NoError(t, err)
	want := "signature did not verify"

	require.LogsContain(t, hook, want)

}

func TestProcessDeposit_IncompleteDeposit(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	server, endpoint, err := testing2.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)
	require.NoError(t, web3Service.preGenesisState.SetValidators([]*zondpb.Validator{}))

	deposit := &zondpb.Deposit{
		Data: &zondpb.Deposit_Data{
			Amount:                params.BeaconConfig().EffectiveBalanceIncrement, // incomplete deposit
			WithdrawalCredentials: bytesutil.PadTo([]byte("testing"), 32),
			Signature:             bytesutil.PadTo([]byte("test"), field_params.DilithiumSignatureLength),
		},
	}

	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	deposit.Data.PublicKey = priv.PublicKey().Marshal()
	d, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	require.NoError(t, err)
	signedRoot, err := signing.ComputeSigningRoot(deposit.Data, d)
	require.NoError(t, err)

	sig := priv.Sign(signedRoot[:])
	deposit.Data.Signature = sig.Marshal()

	generatedTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err)
	root, err := generatedTrie.HashTreeRoot()
	require.NoError(t, err)
	eth1Data := &zondpb.Eth1Data{
		DepositCount: 1,
		DepositRoot:  root[:],
	}
	proof, err := generatedTrie.MerkleProof(0)
	require.NoError(t, err)
	dataRoot, err := deposit.Data.HashTreeRoot()
	require.NoError(t, err)
	deposit.Proof = proof

	factor := params.BeaconConfig().MaxEffectiveBalance / params.BeaconConfig().EffectiveBalanceIncrement
	// deposit till 31e9
	for i := 0; i < int(factor-1); i++ {
		assert.NoError(t, generatedTrie.Insert(dataRoot[:], i))

		trieRoot, err := generatedTrie.HashTreeRoot()
		require.NoError(t, err)
		eth1Data.DepositRoot = trieRoot[:]
		eth1Data.DepositCount = uint64(i + 1)

		deposit.Proof, err = generatedTrie.MerkleProof(i)
		require.NoError(t, err)
		err = web3Service.processDeposit(context.Background(), eth1Data, deposit)
		require.NoError(t, err, fmt.Sprintf("Could not process deposit at %d", i))

		valcount, err := helpers.ActiveValidatorCount(context.Background(), web3Service.preGenesisState, 0)
		require.NoError(t, err)
		require.Equal(t, 0, int(valcount), "Did not get correct active validator count")
	}
}

func TestProcessDeposit_AllDepositedSuccessfully(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	server, endpoint, err := testing2.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 ETH1.0 chain service")
	web3Service = setDefaultMocks(web3Service)

	deposits, keys, err := util.DeterministicDepositsAndKeys(10)
	require.NoError(t, err)
	eth1Data, err := util.DeterministicEth1Data(len(deposits))
	require.NoError(t, err)

	for i := range keys {
		eth1Data.DepositCount = uint64(i + 1)
		err = web3Service.processDeposit(context.Background(), eth1Data, deposits[i])
		require.NoError(t, err, fmt.Sprintf("Could not process deposit at %d", i))

		valCount, err := helpers.ActiveValidatorCount(context.Background(), web3Service.preGenesisState, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(i+1), valCount, "Did not get correct active validator count")

		val, err := web3Service.preGenesisState.ValidatorAtIndex(primitives.ValidatorIndex(i))
		require.NoError(t, err)
		assert.Equal(t, params.BeaconConfig().MaxEffectiveBalance, val.EffectiveBalance)
	}
}
