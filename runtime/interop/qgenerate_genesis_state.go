package interop

import (
	"context"
	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v4/async"
	"github.com/prysmaticlabs/prysm/v4/beacon-chain/core/signing"
	coreState "github.com/prysmaticlabs/prysm/v4/beacon-chain/core/transition"
	statenative "github.com/prysmaticlabs/prysm/v4/beacon-chain/state/state-native"
	"github.com/prysmaticlabs/prysm/v4/config/params"
	"github.com/prysmaticlabs/prysm/v4/container/trie"
	"github.com/prysmaticlabs/prysm/v4/crypto/dilithium"
	"github.com/prysmaticlabs/prysm/v4/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v4/time"
	"sync"
)

func QGenerateGenesisState(ctx context.Context, genesisTime, numValidators uint64) (*ethpb.BeaconState, []*ethpb.Deposit, error) {
	dilithiumKeys, pubKeys, err := QDeterministicallyGenerateKeys(0 /*startIndex*/, numValidators)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "could not deterministically generate keys for %d validators", numValidators)
	}
	depositDataItems, depositDataRoots, err := QDepositDataFromKeys(dilithiumKeys, pubKeys)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate deposit data from keys")
	}
	return QGenerateGenesisStateFromDepositData(ctx, genesisTime, depositDataItems, depositDataRoots)
}

// QGenerateGenesisStateFromDepositData creates a genesis state given a list of
// deposit data items and their corresponding roots.
func QGenerateGenesisStateFromDepositData(
	ctx context.Context, genesisTime uint64, depositData []*ethpb.Deposit_Data, depositDataRoots [][]byte,
) (*ethpb.BeaconState, []*ethpb.Deposit, error) {
	t, err := trie.GenerateTrieFromItems(depositDataRoots, params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not generate Merkle trie for deposit proofs")
	}
	deposits, err := QGenerateDepositsFromData(depositData, t)
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
	beaconState, err := coreState.GenesisBeaconState(ctx, deposits, genesisTime, &ethpb.Eth1Data{
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

// QGenerateDepositsFromData a list of deposit items by creating proofs for each of them from a sparse Merkle trie.
func QGenerateDepositsFromData(depositDataItems []*ethpb.Deposit_Data, trie *trie.SparseMerkleTrie) ([]*ethpb.Deposit, error) {
	deposits := make([]*ethpb.Deposit, len(depositDataItems))
	results, err := async.Scatter(len(depositDataItems), func(offset int, entries int, _ *sync.RWMutex) (interface{}, error) {
		return qgenerateDepositsFromData(depositDataItems[offset:offset+entries], offset, trie)
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate deposits from data")
	}
	for _, result := range results {
		if depositExtent, ok := result.Extent.([]*ethpb.Deposit); ok {
			copy(deposits[result.Offset:], depositExtent)
		} else {
			return nil, errors.New("extent not of expected type")
		}
	}
	return deposits, nil
}

// qgenerateDepositsFromData a list of deposit items by creating proofs for each of them from a sparse Merkle trie.
func qgenerateDepositsFromData(depositDataItems []*ethpb.Deposit_Data, offset int, trie *trie.SparseMerkleTrie) ([]*ethpb.Deposit, error) {
	deposits := make([]*ethpb.Deposit, len(depositDataItems))
	for i, item := range depositDataItems {
		proof, err := trie.MerkleProof(i + offset)
		if err != nil {
			return nil, errors.Wrapf(err, "could not generate proof for deposit %d", i+offset)
		}
		deposits[i] = &ethpb.Deposit{
			Proof: proof,
			Data:  item,
		}
	}
	return deposits, nil
}

// QDepositDataFromKeys generates a list of deposit data items from a set of BLS validator keys.
func QDepositDataFromKeys(dilithiumKeys []dilithium.DilithiumKey, pubKeys []dilithium.PublicKey) ([]*ethpb.Deposit_Data, [][]byte, error) {
	type depositData struct {
		items []*ethpb.Deposit_Data
		roots [][]byte
	}
	depositDataItems := make([]*ethpb.Deposit_Data, len(dilithiumKeys))
	depositDataRoots := make([][]byte, len(dilithiumKeys))
	results, err := async.Scatter(len(dilithiumKeys), func(offset int, entries int, _ *sync.RWMutex) (interface{}, error) {
		items, roots, err := qdepositDataFromKeys(dilithiumKeys[offset:offset+entries], pubKeys[offset:offset+entries], 0)
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

func qdepositDataFromKeys(dilithiumKeys []dilithium.DilithiumKey, pubKeys []dilithium.PublicKey, numOfCreds uint64) ([]*ethpb.Deposit_Data, [][]byte, error) {
	dataRoots := make([][]byte, len(dilithiumKeys))
	depositDataItems := make([]*ethpb.Deposit_Data, len(dilithiumKeys))
	for i := 0; i < len(dilithiumKeys); i++ {
		withCred := uint64(i) < numOfCreds
		data, err := qcreateDepositData(dilithiumKeys[i], pubKeys[i], withCred)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "could not create deposit data for key: %#x", dilithiumKeys[i].Marshal())
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

func qcreateDepositData(dilithiumKey dilithium.DilithiumKey, pubKey dilithium.PublicKey, withExecCreds bool) (*ethpb.Deposit_Data, error) {
	depositMessage := &ethpb.DepositMessage{
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
	root, err := (&ethpb.SigningData{ObjectRoot: sr[:], Domain: domain}).HashTreeRoot()
	if err != nil {
		return nil, err
	}
	di := &ethpb.Deposit_Data{
		PublicKey:             depositMessage.PublicKey,
		WithdrawalCredentials: depositMessage.WithdrawalCredentials,
		Amount:                depositMessage.Amount,
		Signature:             dilithiumKey.Sign(root[:]).Marshal(),
	}
	return di, nil
}
