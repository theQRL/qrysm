package local

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	keystorev4 "github.com/theQRL/go-zond-wallet-encryptor-keystore"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/dilithium"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	validatorpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	mock "github.com/theQRL/qrysm/validator/accounts/testing"
	"github.com/theQRL/qrysm/validator/keymanager"
)

func TestLocalKeymanager_FetchValidatingPublicKeys(t *testing.T) {
	wallet := &mock.Wallet{
		Files:          make(map[string]map[string][]byte),
		WalletPassword: password,
	}
	dr := &Keymanager{
		wallet:        wallet,
		accountsStore: &accountStore{},
	}
	// First, generate accounts and their keystore.json files.
	ctx := context.Background()
	numAccounts := 10
	wantedPubKeys := make([][field_params.DilithiumPubkeyLength]byte, 0)
	for i := 0; i < numAccounts; i++ {
		privKey, err := dilithium.RandKey()
		require.NoError(t, err)
		pubKey := bytesutil.ToBytes2592(privKey.PublicKey().Marshal())
		wantedPubKeys = append(wantedPubKeys, pubKey)
		dr.accountsStore.PublicKeys = append(dr.accountsStore.PublicKeys, pubKey[:])
		dr.accountsStore.Seeds = append(dr.accountsStore.Seeds, privKey.Marshal())
	}
	require.NoError(t, dr.initializeKeysCachesFromKeystore())
	publicKeys, err := dr.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	assert.Equal(t, numAccounts, len(publicKeys))
	// FetchValidatingPublicKeys is also used in generating the output of account list
	// therefore the results must be in the same order as the order in which the accounts were derived
	for i, key := range wantedPubKeys {
		assert.Equal(t, key, publicKeys[i])
	}
}

func TestLocalKeymanager_FetchValidatingSeeds(t *testing.T) {
	wallet := &mock.Wallet{
		Files:          make(map[string]map[string][]byte),
		WalletPassword: password,
	}
	dr := &Keymanager{
		wallet:        wallet,
		accountsStore: &accountStore{},
	}
	// First, generate accounts and their keystore.json files.
	ctx := context.Background()
	numAccounts := 10
	wantedSeeds := make([][48]byte, numAccounts)
	for i := 0; i < numAccounts; i++ {
		seed, err := dilithium.RandKey()
		require.NoError(t, err)
		seedData := seed.Marshal()
		pubKey := bytesutil.ToBytes2592(seed.PublicKey().Marshal())
		wantedSeeds[i] = bytesutil.ToBytes48(seedData)
		dr.accountsStore.PublicKeys = append(dr.accountsStore.PublicKeys, pubKey[:])
		dr.accountsStore.Seeds = append(dr.accountsStore.Seeds, seedData)
	}
	require.NoError(t, dr.initializeKeysCachesFromKeystore())
	seeds, err := dr.FetchValidatingSeeds(ctx)
	require.NoError(t, err)
	assert.Equal(t, numAccounts, len(seeds))
	// FetchValidatingSeeds is also used in generating the output of account list
	// therefore the results must be in the same order as the order in which the accounts were created
	for i, key := range wantedSeeds {
		assert.Equal(t, key, seeds[i])
	}
}

func TestLocalKeymanager_Sign(t *testing.T) {
	wallet := &mock.Wallet{
		Files:            make(map[string]map[string][]byte),
		AccountPasswords: make(map[string]string),
		WalletPassword:   password,
	}
	dr := &Keymanager{
		wallet:        wallet,
		accountsStore: &accountStore{},
	}

	// First, generate accounts and their keystore.json files.
	ctx := context.Background()
	numAccounts := 10
	keystores := make([]*keymanager.Keystore, numAccounts)
	passwords := make([]string, numAccounts)
	for i := 0; i < numAccounts; i++ {
		keystores[i] = createRandomKeystore(t, password)
		passwords[i] = password
	}
	_, err := dr.ImportKeystores(ctx, keystores, passwords)
	require.NoError(t, err)

	var encodedKeystore []byte
	for k, v := range wallet.Files[AccountsPath] {
		if strings.Contains(k, "keystore") {
			encodedKeystore = v
		}
	}
	keystoreFile := &keymanager.Keystore{}
	require.NoError(t, json.Unmarshal(encodedKeystore, keystoreFile))

	// We extract the validator signing private key from the keystore
	// by utilizing the password and initialize a new Dilithium secret key from
	// its raw bytes.
	decryptor := keystorev4.New()
	enc, err := decryptor.Decrypt(keystoreFile.Crypto, dr.wallet.Password())
	require.NoError(t, err)
	store := &accountStore{}
	require.NoError(t, json.Unmarshal(enc, store))
	require.Equal(t, len(store.PublicKeys), len(store.Seeds))
	require.NotEqual(t, 0, len(store.PublicKeys))
	dr.accountsStore = store
	require.NoError(t, dr.initializeKeysCachesFromKeystore())
	publicKeys, err := dr.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	require.Equal(t, len(publicKeys), len(store.PublicKeys))

	// We prepare naive data to sign.
	data := []byte("hello world")
	signRequest := &validatorpb.SignRequest{
		PublicKey:   publicKeys[0][:],
		SigningRoot: data,
	}
	sig, err := dr.Sign(ctx, signRequest)
	require.NoError(t, err)
	pubKey, err := dilithium.PublicKeyFromBytes(publicKeys[0][:])
	require.NoError(t, err)
	wrongPubKey, err := dilithium.PublicKeyFromBytes(publicKeys[1][:])
	require.NoError(t, err)
	if !sig.Verify(pubKey, data) {
		t.Fatalf("Expected sig to verify for pubkey %#x and data %v", pubKey.Marshal(), data)
	}
	if sig.Verify(wrongPubKey, data) {
		t.Fatalf("Expected sig not to verify for pubkey %#x and data %v", wrongPubKey.Marshal(), data)
	}
}

func TestLocalKeymanager_Sign_NoPublicKeySpecified(t *testing.T) {
	req := &validatorpb.SignRequest{
		PublicKey: nil,
	}
	dr := &Keymanager{}
	_, err := dr.Sign(context.Background(), req)
	assert.ErrorContains(t, "nil public key", err)
}

func TestLocalKeymanager_Sign_NoPublicKeyInCache(t *testing.T) {
	req := &validatorpb.SignRequest{
		PublicKey: []byte("hello world"),
	}
	dilithiumKeysCache = make(map[[field_params.DilithiumPubkeyLength]byte]dilithium.DilithiumKey)
	dr := &Keymanager{}
	_, err := dr.Sign(context.Background(), req)
	assert.ErrorContains(t, "no signing key found in keys cache", err)
}
