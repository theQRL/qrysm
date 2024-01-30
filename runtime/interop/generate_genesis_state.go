// Package interop contains deterministic utilities for generating
// genesis states and keys.
package interop

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/async"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	coreState "github.com/theQRL/qrysm/v4/beacon-chain/core/transition"
	statenative "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/container/trie"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/crypto/hash"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/time"
)

var (
	// This is the recommended mock eth1 block hash according to the Ethereum consensus interop guidelines.
	// https://github.com/ethereum/eth2.0-pm/blob/a085c9870f3956d6228ed2a40cd37f0c6580ecd7/interop/mocked_start/README.md
	mockEth1BlockHash = []byte{66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66}
)

// GenerateGenesisState deterministically given a genesis time and number of validators.
// If a genesis time of 0 is supplied it is set to the current time.
func GenerateGenesisState(ctx context.Context, genesisTime, numValidators uint64) (*zondpb.BeaconState, []*zondpb.Deposit, error) {
	privKeys, pubKeys, err := DeterministicallyGenerateKeys(0 /*startIndex*/, numValidators)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not deterministically generate keys for %d validators", numValidators)
	}
	depositDataItems, depositDataRoots, err := DepositDataFromKeys(privKeys, pubKeys)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate deposit data from keys")
	}
	return GenerateGenesisStateFromDepositData(ctx, genesisTime, depositDataItems, depositDataRoots)
}

// GenerateGenesisStateFromDepositData creates a genesis state given a list of
// deposit data items and their corresponding roots.
func GenerateGenesisStateFromDepositData(
	ctx context.Context, genesisTime uint64, depositData []*zondpb.Deposit_Data, depositDataRoots [][]byte,
) (*zondpb.BeaconState, []*zondpb.Deposit, error) {
	t, err := trie.GenerateTrieFromItems(depositDataRoots, params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate Merkle trie for deposit proofs")
	}
	deposits, err := GenerateDepositsFromData(depositData, t)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate deposits from the deposit data provided")
	}
	root, err := t.HashTreeRoot()
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not hash tree root of deposit trie")
	}
	if genesisTime == 0 {
		genesisTime = uint64(time.Now().Unix())
	}
	beaconState, err := coreState.GenesisBeaconState(ctx, deposits, genesisTime, &zondpb.Eth1Data{
		DepositRoot:  root[:],
		DepositCount: uint64(len(deposits)),
		BlockHash:    mockEth1BlockHash,
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate genesis state")
	}

	pbState, err := statenative.ProtobufBeaconStatePhase0(beaconState.ToProtoUnsafe())
	if err != nil {
		return nil, nil, err
	}
	return pbState, deposits, nil
}

// GenerateDepositsFromData a list of deposit items by creating proofs for each of them from a sparse Merkle trie.
func GenerateDepositsFromData(depositDataItems []*zondpb.Deposit_Data, trie *trie.SparseMerkleTrie) ([]*zondpb.Deposit, error) {
	deposits := make([]*zondpb.Deposit, len(depositDataItems))
	results, err := async.Scatter(len(depositDataItems), func(offset int, entries int, _ *sync.RWMutex) (interface{}, error) {
		return generateDepositsFromData(depositDataItems[offset:offset+entries], offset, trie)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate deposits from data")
	}
	for _, result := range results {
		if depositExtent, ok := result.Extent.([]*zondpb.Deposit); ok {
			copy(deposits[result.Offset:], depositExtent)
		} else {
			return nil, errors.New("extent not of expected type")
		}
	}
	return deposits, nil
}

// generateDepositsFromData a list of deposit items by creating proofs for each of them from a sparse Merkle trie.
func generateDepositsFromData(depositDataItems []*zondpb.Deposit_Data, offset int, trie *trie.SparseMerkleTrie) ([]*zondpb.Deposit, error) {
	deposits := make([]*zondpb.Deposit, len(depositDataItems))
	for i, item := range depositDataItems {
		proof, err := trie.MerkleProof(i + offset)
		if err != nil {
			return nil, errors.Wrapf(err, "could not generate proof for deposit %d", i+offset)
		}
		deposits[i] = &zondpb.Deposit{
			Proof: proof,
			Data:  item,
		}
	}
	return deposits, nil
}

// DepositDataFromKeys generates a list of deposit data items from a set of BLS validator keys.
func DepositDataFromKeys(privKeys []dilithium.DilithiumKey, pubKeys []dilithium.PublicKey) ([]*zondpb.Deposit_Data, [][]byte, error) {
	type depositData struct {
		items []*zondpb.Deposit_Data
		roots [][]byte
	}
	depositDataItems := make([]*zondpb.Deposit_Data, len(privKeys))
	depositDataRoots := make([][]byte, len(privKeys))
	results, err := async.Scatter(len(privKeys), func(offset int, entries int, _ *sync.RWMutex) (interface{}, error) {
		items, roots, err := depositDataFromKeys(privKeys[offset:offset+entries], pubKeys[offset:offset+entries], 0)
		return &depositData{items: items, roots: roots}, err
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to generate deposit data from keys")
	}
	for _, result := range results {
		if depositDataExtent, ok := result.Extent.(*depositData); ok {
			copy(depositDataItems[result.Offset:], depositDataExtent.items)
			copy(depositDataRoots[result.Offset:], depositDataExtent.roots)
		} else {
			return nil, nil, errors.New("extent not of expected type")
		}
	}
	return depositDataItems, depositDataRoots, nil
}

// DepositDataFromKeysWithExecCreds generates a list of deposit data items from a set of BLS validator keys.
func DepositDataFromKeysWithExecCreds(privKeys []dilithium.DilithiumKey, pubKeys []dilithium.PublicKey, numOfCreds uint64) ([]*zondpb.Deposit_Data, [][]byte, error) {
	return depositDataFromKeys(privKeys, pubKeys, numOfCreds)
}

func depositDataFromKeys(privKeys []dilithium.DilithiumKey, pubKeys []dilithium.PublicKey, numOfCreds uint64) ([]*zondpb.Deposit_Data, [][]byte, error) {
	dataRoots := make([][]byte, len(privKeys))
	depositDataItems := make([]*zondpb.Deposit_Data, len(privKeys))
	for i := 0; i < len(privKeys); i++ {
		withCred := uint64(i) < numOfCreds
		data, err := createDepositData(privKeys[i], pubKeys[i], withCred)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "could not create deposit data for key: %#x", privKeys[i].Marshal())
		}
		h, err := data.HashTreeRoot()
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not hash tree root deposit data item")
		}
		dataRoots[i] = h[:]
		depositDataItems[i] = data
	}
	return depositDataItems, dataRoots, nil
}

// Generates a deposit data item from BLS keys and signs the hash tree root of the data.
func createDepositData(privKey dilithium.DilithiumKey, pubKey dilithium.PublicKey, withExecCreds bool) (*zondpb.Deposit_Data, error) {
	depositMessage := &zondpb.DepositMessage{
		PublicKey:             pubKey.Marshal(),
		WithdrawalCredentials: withdrawalCredentialsHash(pubKey.Marshal()),
		Amount:                params.BeaconConfig().MaxEffectiveBalance,
	}
	if withExecCreds {
		newCredentials := make([]byte, 12)
		newCredentials[0] = params.BeaconConfig().ETH1AddressWithdrawalPrefixByte
		execAddr := bytesutil.ToBytes20(pubKey.Marshal())
		depositMessage.WithdrawalCredentials = append(newCredentials, execAddr[:]...)
	}
	sr, err := depositMessage.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		return nil, err
	}
	root, err := (&zondpb.SigningData{ObjectRoot: sr[:], Domain: domain}).HashTreeRoot()
	if err != nil {
		return nil, err
	}
	di := &zondpb.Deposit_Data{
		PublicKey:             depositMessage.PublicKey,
		WithdrawalCredentials: depositMessage.WithdrawalCredentials,
		Amount:                depositMessage.Amount,
		Signature:             privKey.Sign(root[:]).Marshal(),
	}
	return di, nil
}

// withdrawalCredentialsHash forms a 32 byte hash of the withdrawal public
// address.
//
// The specification is as follows:
//
//	withdrawal_credentials[:1] == BLS_WITHDRAWAL_PREFIX_BYTE
//	withdrawal_credentials[1:] == hash(withdrawal_pubkey)[1:]
//
// where withdrawal_credentials is of type bytes32.
func withdrawalCredentialsHash(pubKey []byte) []byte {
	h := hash.Hash(pubKey)
	return append([]byte{blsWithdrawalPrefixByte}, h[1:]...)[:32]
}
