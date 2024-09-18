package local

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	keystorev4 "github.com/theQRL/go-zond-wallet-encryptor-keystore"
	"github.com/theQRL/qrysm/async/event"
	"github.com/theQRL/qrysm/crypto/dilithium"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	mock "github.com/theQRL/qrysm/validator/accounts/testing"
)

func TestLocalKeymanager_reloadAccountsFromKeystore_MismatchedNumKeys(t *testing.T) {
	password := "Passw03rdz293**%#2"
	wallet := &mock.Wallet{
		Files:            make(map[string]map[string][]byte),
		AccountPasswords: make(map[string]string),
		WalletPassword:   password,
	}
	dr := &Keymanager{
		wallet: wallet,
	}
	accountsStore := &accountStore{
		Seeds:      [][]byte{[]byte("hello")},
		PublicKeys: [][]byte{[]byte("hi"), []byte("world")},
	}
	encodedStore, err := json.MarshalIndent(accountsStore, "", "\t")
	require.NoError(t, err)
	encryptor := keystorev4.New()
	cryptoFields, err := encryptor.Encrypt(encodedStore, dr.wallet.Password())
	require.NoError(t, err)
	id, err := uuid.NewRandom()
	require.NoError(t, err)
	keystore := &AccountsKeystoreRepresentation{
		Crypto:  cryptoFields,
		ID:      id.String(),
		Version: encryptor.Version(),
		Name:    encryptor.Name(),
	}
	err = dr.reloadAccountsFromKeystore(keystore)
	assert.ErrorContains(t, "do not match", err)
}

func TestLocalKeymanager_reloadAccountsFromKeystore(t *testing.T) {
	password := "Passw03rdz293**%#2"
	wallet := &mock.Wallet{
		Files:            make(map[string]map[string][]byte),
		AccountPasswords: make(map[string]string),
		WalletPassword:   password,
	}
	dr := &Keymanager{
		wallet:              wallet,
		accountsChangedFeed: new(event.Feed),
	}

	numAccounts := 20
	privKeys := make([][]byte, numAccounts)
	pubKeys := make([][]byte, numAccounts)
	for i := 0; i < numAccounts; i++ {
		privKey, err := dilithium.RandKey()
		require.NoError(t, err)
		privKeys[i] = privKey.Marshal()
		pubKeys[i] = privKey.PublicKey().Marshal()
	}

	accountsStore, err := dr.CreateAccountsKeystore(context.Background(), privKeys, pubKeys)
	require.NoError(t, err)
	require.NoError(t, dr.reloadAccountsFromKeystore(accountsStore))

	// Check that the public keys were added to the public keys cache.
	for i, keyBytes := range pubKeys {
		require.Equal(t, bytesutil.ToBytes2592(keyBytes), orderedPublicKeys[i])
	}

	// Check that the secret keys were added to the secret keys cache.
	lock.RLock()
	defer lock.RUnlock()
	for i, keyBytes := range privKeys {
		privKey, ok := dilithiumKeysCache[bytesutil.ToBytes2592(pubKeys[i])]
		require.Equal(t, true, ok)
		require.Equal(t, bytesutil.ToBytes2592(keyBytes), bytesutil.ToBytes2592(privKey.Marshal()))
	}

	// Check the key was added to the global accounts store.
	require.Equal(t, numAccounts, len(dr.accountsStore.PublicKeys))
	require.Equal(t, numAccounts, len(dr.accountsStore.Seeds))
	assert.DeepEqual(t, dr.accountsStore.PublicKeys[0], pubKeys[0])
}
