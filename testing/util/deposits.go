package util

import (
	"sync"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/crypto/pqcrypto"
	walletmldsa87 "github.com/theQRL/go-qrllib/wallet/ml_dsa_87"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/container/trie"
	"github.com/theQRL/qrysm/contracts/deposit"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/interop"
	"google.golang.org/protobuf/proto"
)

var lock sync.Mutex

// Caches
var cachedDeposits []*qrysmpb.Deposit
var privKeys []ml_dsa_87.MLDSA87Key
var t *trie.SparseMerkleTrie

// DeterministicDepositsAndKeys returns the entered amount of deposits and secret keys.
// The deposits are configured such that for deposit n the validator
// account is key n and the withdrawal account is key n+1.  As such,
// if all secret keys for n validators are required then numDeposits
// should be n+1.
func DeterministicDepositsAndKeys(numDeposits uint64) ([]*qrysmpb.Deposit, []ml_dsa_87.MLDSA87Key, error) {
	lock.Lock()
	defer lock.Unlock()
	var err error
	if numDeposits < uint64(len(cachedDeposits)) {
		t = nil
		privKeys = []ml_dsa_87.MLDSA87Key{}
		cachedDeposits = []*qrysmpb.Deposit{}
	}

	// Populate trie cache, if not initialized yet.
	if t == nil {
		t, err = trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to create new trie")
		}
	}

	// If more deposits requested than cached, generate more.
	if numDeposits > uint64(len(cachedDeposits)) {
		numExisting := uint64(len(cachedDeposits))
		numRequired := numDeposits - uint64(len(cachedDeposits))
		// Fetch the required number of keys.
		secretKeys, publicKeys, err := interop.DeterministicallyGenerateKeys(numExisting, numRequired+1)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not create deterministic keys: ")
		}
		privKeys = append(privKeys, secretKeys[:len(secretKeys)-1]...)

		// Create the new deposits and add them to the trie.
		for i := range numRequired {
			balance := params.BeaconConfig().MaxEffectiveBalance
			deposit, err := signedDeposit(secretKeys[i], publicKeys[i].Marshal(), balance)
			if err != nil {
				return nil, nil, errors.Wrap(err, "could not create signed deposit")
			}
			cachedDeposits = append(cachedDeposits, deposit)

			hashedDeposit, err := deposit.Data.HashTreeRoot()
			if err != nil {
				return nil, nil, errors.Wrap(err, "could not tree hash deposit data")
			}

			if err = t.Insert(hashedDeposit[:], int(numExisting+i)); err != nil {
				return nil, nil, err
			}
		}
	}

	depositTrie, _, err := DeterministicDepositTrie(int(numDeposits)) // lint:ignore uintcast
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create deposit trie")
	}
	requestedDeposits := make([]*qrysmpb.Deposit, int(numDeposits)) // lint:ignore uintcast -- test code
	for i := range requestedDeposits {
		deposit, ok := proto.Clone(cachedDeposits[i]).(*qrysmpb.Deposit)
		if !ok {
			return nil, nil, errors.New("proto.Clone did not return a deposit proto")
		}
		requestedDeposits[i] = deposit
	}
	for i := range requestedDeposits {
		proof, err := depositTrie.MerkleProof(i)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not create merkle proof")
		}
		requestedDeposits[i].Proof = proof
	}

	requestedKeys := make([]ml_dsa_87.MLDSA87Key, int(numDeposits)) // lint:ignore uintcast -- test code
	copy(requestedKeys, privKeys[0:numDeposits])
	return requestedDeposits, requestedKeys, nil
}

// DepositsWithBalance generates N amount of deposits with the balances taken from the passed in balances array.
// If an empty array is passed,
func DepositsWithBalance(balances []uint64) ([]*qrysmpb.Deposit, *trie.SparseMerkleTrie, error) {
	var err error

	sparseTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create new trie")
	}

	numDeposits := uint64(len(balances))
	numExisting := uint64(len(cachedDeposits))
	numRequired := numDeposits - uint64(len(cachedDeposits))

	var secretKeys []ml_dsa_87.MLDSA87Key
	var publicKeys []ml_dsa_87.PublicKey
	if numExisting >= numDeposits+1 {
		secretKeys = append(secretKeys, privKeys[:numDeposits+1]...)
		publicKeys = publicKeysFromSecrets(secretKeys)
	} else {
		secretKeys = append(secretKeys, privKeys[:numExisting]...)
		publicKeys = publicKeysFromSecrets(secretKeys)
		// Fetch enough keys for all deposits, since this function is uncached.
		newSecretKeys, newPublicKeys, err := interop.DeterministicallyGenerateKeys(numExisting, numRequired+1)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not create deterministic keys: ")
		}
		secretKeys = append(secretKeys, newSecretKeys...)
		publicKeys = append(publicKeys, newPublicKeys...)
	}

	deposits := make([]*qrysmpb.Deposit, numDeposits)
	// Create the new deposits and add them to the trie.
	for i := range numDeposits {
		balance := params.BeaconConfig().MaxEffectiveBalance
		// lint:ignore uintcast -- test code
		if len(balances) == int(numDeposits) {
			balance = balances[i]
		}
		deposit, err := signedDeposit(secretKeys[i], publicKeys[i].Marshal(), balance)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not create signed deposit")
		}
		deposits[i] = deposit

		hashedDeposit, err := deposit.Data.HashTreeRoot()
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not tree hash deposit data")
		}

		// lint:ignore uintcast -- test code
		if err = sparseTrie.Insert(hashedDeposit[:], int(i)); err != nil {
			return nil, nil, err
		}
	}

	depositTrie, _, err := DepositTrieSubset(sparseTrie, int(numDeposits)) // lint:ignore uintcast -- test code
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create deposit trie")
	}
	for i := range deposits {
		proof, err := depositTrie.MerkleProof(i)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not create merkle proof")
		}
		deposits[i].Proof = proof
	}

	return deposits, depositTrie, nil
}

func signedDeposit(
	secretKey ml_dsa_87.MLDSA87Key,
	publicKey []byte,
	balance uint64,
) (*qrysmpb.Deposit, error) {
	d, err := walletmldsa87.NewMLDSA87Descriptor()
	if err != nil {
		return nil, err
	}
	descriptor := d.ToDescriptor()
	withdrawalAddr, err := pqcrypto.PublicKeyAndDescriptorToAddress(publicKey, descriptor)
	if err != nil {
		return nil, err
	}
	withdrawalCreds := deposit.WithdrawalCredentialsAddress(withdrawalAddr)
	depositMessage := &qrysmpb.DepositMessage{
		PublicKey:             publicKey,
		Amount:                balance,
		WithdrawalCredentials: withdrawalCreds,
	}

	domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute domain")
	}
	root, err := depositMessage.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get signing root of deposit data")
	}

	sigRoot, err := (&qrysmpb.SigningData{ObjectRoot: root[:], Domain: domain}).HashTreeRoot()
	if err != nil {
		return nil, err
	}
	depositData := &qrysmpb.Deposit_Data{
		PublicKey:             publicKey,
		Amount:                balance,
		WithdrawalCredentials: withdrawalCreds,
		Signature:             secretKey.Sign(sigRoot[:]).Marshal(),
	}

	deposit := &qrysmpb.Deposit{
		Data: depositData,
	}
	return deposit, nil
}

// DeterministicDepositTrie returns a merkle trie of the requested size from the
// deterministic deposits.
func DeterministicDepositTrie(size int) (*trie.SparseMerkleTrie, [][32]byte, error) {
	if t == nil {
		return nil, [][32]byte{}, errors.New("trie cache is empty, generate deposits at an earlier point")
	}

	return DepositTrieSubset(t, size)
}

// DepositTrieSubset takes in a full tree and the desired size and returns a subset of the deposit trie.
func DepositTrieSubset(sparseTrie *trie.SparseMerkleTrie, size int) (*trie.SparseMerkleTrie, [][32]byte, error) {
	if sparseTrie == nil {
		return nil, [][32]byte{}, errors.New("trie is empty")
	}

	items := sparseTrie.Items()
	if size > len(items) {
		return nil, [][32]byte{}, errors.New("requested a larger tree than amount of deposits")
	}

	items = items[:size]
	depositTrie, err := trie.GenerateTrieFromItems(items, params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		return nil, [][32]byte{}, errors.Wrapf(err, "could not generate trie of %d length", size)
	}

	roots := make([][32]byte, len(items))
	for i, dep := range items {
		roots[i] = bytesutil.ToBytes32(dep)
	}
	return depositTrie, roots, nil
}

// DeterministicExecutionData takes an array of deposits and returns the executionData made from the deposit trie.
func DeterministicExecutionData(size int) (*qrysmpb.ExecutionData, error) {
	depositTrie, _, err := DeterministicDepositTrie(size)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create trie")
	}
	root, err := depositTrie.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute deposit trie root")
	}
	executionData := &qrysmpb.ExecutionData{
		BlockHash:    root[:],
		DepositRoot:  root[:],
		DepositCount: uint64(size),
	}
	return executionData, nil
}

// DepositTrieFromDeposits takes an array of deposits and returns the deposit trie.
func DepositTrieFromDeposits(deposits []*qrysmpb.Deposit) (*trie.SparseMerkleTrie, [][32]byte, error) {
	encodedDeposits := make([][]byte, len(deposits))
	roots := make([][32]byte, len(deposits))
	for i := range encodedDeposits {
		hashedDeposit, err := deposits[i].Data.HashTreeRoot()
		if err != nil {
			return nil, [][32]byte{}, errors.Wrap(err, "could not tree hash deposit data")
		}
		encodedDeposits[i] = hashedDeposit[:]
		roots[i] = hashedDeposit
	}

	depositTrie, err := trie.GenerateTrieFromItems(encodedDeposits, params.BeaconConfig().DepositContractTreeDepth)
	if err != nil {
		return nil, [][32]byte{}, errors.Wrap(err, "Could not generate deposit trie")
	}

	return depositTrie, roots, nil
}

// resetCache clears out the old trie, private keys and deposits.
func resetCache() {
	lock.Lock()
	defer lock.Unlock()
	t = nil
	privKeys = []ml_dsa_87.MLDSA87Key{}
	cachedDeposits = []*qrysmpb.Deposit{}
}

// DeterministicDepositsAndKeysSameValidator returns the entered amount of deposits and secret keys
// of the same validator. This is for negative test cases such as same deposits from same validators in a block don't
// result in duplicated validator indices.
func DeterministicDepositsAndKeysSameValidator(numDeposits uint64) ([]*qrysmpb.Deposit, []ml_dsa_87.MLDSA87Key, error) {
	resetCache()
	lock.Lock()
	defer lock.Unlock()
	var err error

	// Populate trie cache, if not initialized yet.
	if t == nil {
		t, err = trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
		if err != nil {
			return nil, nil, errors.Wrap(err, "failed to create new trie")
		}
	}

	// If more deposits requested than cached, generate more.
	if numDeposits > uint64(len(cachedDeposits)) {
		numExisting := uint64(len(cachedDeposits))
		numRequired := numDeposits - uint64(len(cachedDeposits))
		// Fetch the required number of keys.
		secretKeys, publicKeys, err := interop.DeterministicallyGenerateKeys(numExisting, numRequired+1)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not create deterministic keys: ")
		}
		privKeys = append(privKeys, secretKeys[:len(secretKeys)-1]...)

		d, err := walletmldsa87.NewMLDSA87Descriptor()
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not create deterministic deposit descriptor")
		}
		descriptor := d.ToDescriptor()
		addr1, err := pqcrypto.PublicKeyAndDescriptorToAddress(publicKeys[1].Marshal(), descriptor)
		if err != nil {
			return nil, nil, err
		}

		// Create the new deposits and add them to the trie. Always use the first validator to create deposit
		for i := range numRequired {
			depositMessage := &qrysmpb.DepositMessage{
				PublicKey:             publicKeys[1].Marshal(),
				Amount:                params.BeaconConfig().MaxEffectiveBalance,
				WithdrawalCredentials: deposit.WithdrawalCredentialsAddress(addr1),
			}

			domain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, nil, nil)
			if err != nil {
				return nil, nil, errors.Wrap(err, "could not compute domain")
			}
			root, err := depositMessage.HashTreeRoot()
			if err != nil {
				return nil, nil, errors.Wrap(err, "could not get signing root of deposit data")
			}
			sigRoot, err := (&qrysmpb.SigningData{ObjectRoot: root[:], Domain: domain}).HashTreeRoot()
			if err != nil {
				return nil, nil, errors.Wrap(err, "could not get signing root of deposit data and domain")
			}
			// Always use the same validator to sign
			depositData := &qrysmpb.Deposit_Data{
				PublicKey:             depositMessage.PublicKey,
				Amount:                depositMessage.Amount,
				WithdrawalCredentials: depositMessage.WithdrawalCredentials,
				Signature:             secretKeys[1].Sign(sigRoot[:]).Marshal(),
			}
			deposit := &qrysmpb.Deposit{
				Data: depositData,
			}
			cachedDeposits = append(cachedDeposits, deposit)

			hashedDeposit, err := deposit.Data.HashTreeRoot()
			if err != nil {
				return nil, nil, errors.Wrap(err, "could not tree hash deposit data")
			}

			if err = t.Insert(hashedDeposit[:], int(numExisting+i)); err != nil {
				return nil, nil, err
			}
		}
	}

	// lint:ignore uintcast -- test code
	depositTrie, _, err := DeterministicDepositTrie(int(numDeposits))
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create deposit trie")
	}
	requestedDeposits := cachedDeposits[:numDeposits]
	for i := range requestedDeposits {
		proof, err := depositTrie.MerkleProof(i)
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not create merkle proof")
		}
		requestedDeposits[i].Proof = proof
	}

	return requestedDeposits, privKeys[0:numDeposits], nil
}

func publicKeysFromSecrets(secretKeys []ml_dsa_87.MLDSA87Key) []ml_dsa_87.PublicKey {
	publicKeys := make([]ml_dsa_87.PublicKey, len(secretKeys))
	for i, secretKey := range secretKeys {
		publicKeys[i] = secretKey.PublicKey()
	}
	return publicKeys
}
