package local

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/async/event"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	keymock "github.com/theQRL/qrysm/crypto/ml_dsa_87/common/mock"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	keystorev1 "github.com/theQRL/qrysm/pkg/go-qrl-wallet-encryptor-keystore"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
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
	wantedPubKeys := make([][field_params.MLDSA87PubkeyLength]byte, 0)
	for range numAccounts {
		privKey, err := ml_dsa_87.RandKey()
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

// Regression: when the validator client starts with a wallet that has no
// accounts file yet, no file watcher is running (listenForAccountChanges
// returns immediately for a missing file). The first keymanager-API import
// creates the file and must notify accountsChangedFeed subscribers itself,
// otherwise the running validator client never learns about the imported
// keys. Writes to an already existing file are reported by the file watcher
// and must not be reported a second time here.
func TestLocalKeymanager_SaveStoreAndReInitialize_NotifiesOnFirstAccountsFileWrite(t *testing.T) {
	ctx := context.Background()
	wallet := &mock.Wallet{
		Files:          make(map[string]map[string][]byte),
		WalletPassword: password,
	}
	dr := &Keymanager{
		wallet:              wallet,
		accountsStore:       &accountStore{},
		accountsChangedFeed: new(event.Feed),
	}
	accountsChangedChan := make(chan [][field_params.MLDSA87PubkeyLength]byte, 1)
	sub := dr.SubscribeAccountChanges(accountsChangedChan)
	defer sub.Unsubscribe()

	newStore := func(t *testing.T, numKeys int) (*accountStore, [][field_params.MLDSA87PubkeyLength]byte) {
		t.Helper()
		store := &accountStore{}
		pubKeys := make([][field_params.MLDSA87PubkeyLength]byte, 0, numKeys)
		for range numKeys {
			privKey, err := ml_dsa_87.RandKey()
			require.NoError(t, err)
			pubKey := bytesutil.ToBytes2592(privKey.PublicKey().Marshal())
			pubKeys = append(pubKeys, pubKey)
			store.PublicKeys = append(store.PublicKeys, pubKey[:])
			store.Seeds = append(store.Seeds, privKey.Marshal())
		}
		return store, pubKeys
	}

	// First write creates the accounts file: subscribers must be notified
	// with the full new key set and the caches must hold the keys.
	store, wantKeys := newStore(t, 2)
	require.NoError(t, dr.SaveStoreAndReInitialize(ctx, store))
	select {
	case got := <-accountsChangedChan:
		require.DeepEqual(t, wantKeys, got)
	default:
		t.Fatal("expected accounts changed notification after the accounts file was first created")
	}
	pubKeys, err := dr.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	require.DeepEqual(t, wantKeys, pubKeys)

	// Second write updates the existing file: the file watcher is responsible
	// for notifying subscribers, so no direct notification is sent.
	store, wantKeys = newStore(t, 3)
	require.NoError(t, dr.SaveStoreAndReInitialize(ctx, store))
	select {
	case <-accountsChangedChan:
		t.Fatal("unexpected direct notification for a write to an existing accounts file")
	default:
	}
	pubKeys, err = dr.FetchValidatingPublicKeys(ctx)
	require.NoError(t, err)
	require.DeepEqual(t, wantKeys, pubKeys)
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
	for i := range numAccounts {
		seed, err := ml_dsa_87.RandKey()
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

// newKeymanagerWithRandomAccounts imports numAccounts freshly generated
// keystores into a keymanager and initializes its key caches from them,
// returning the keymanager and the validating public keys it holds.
func newKeymanagerWithRandomAccounts(t *testing.T, numAccounts int) (*Keymanager, [][field_params.MLDSA87PubkeyLength]byte) {
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
	keystores := make([]*keymanager.Keystore, numAccounts)
	passwords := make([]string, numAccounts)
	for i := range numAccounts {
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
	// by utilizing the password and initialize a new ML-DSA-87 secret key from
	// its raw bytes.
	decryptor := keystorev1.New()
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
	return dr, publicKeys
}

func TestLocalKeymanager_Sign(t *testing.T) {
	ctx := context.Background()
	dr, publicKeys := newKeymanagerWithRandomAccounts(t, 10)

	// We prepare naive data to sign.
	data := []byte("hello world")
	signRequest := &validatorpb.SignRequest{
		PublicKey:   publicKeys[0][:],
		SigningRoot: data,
	}
	sig, err := dr.Sign(ctx, signRequest)
	require.NoError(t, err)
	pubKey, err := ml_dsa_87.PublicKeyFromBytes(publicKeys[0][:])
	require.NoError(t, err)
	wrongPubKey, err := ml_dsa_87.PublicKeyFromBytes(publicKeys[1][:])
	require.NoError(t, err)
	if !sig.Verify(pubKey, data) {
		t.Fatalf("Expected sig to verify for pubkey %#x and data %v", pubKey.Marshal(), data)
	}
	if sig.Verify(wrongPubKey, data) {
		t.Fatalf("Expected sig not to verify for pubkey %#x and data %v", wrongPubKey.Marshal(), data)
	}
}

// Regression test for the qrysm-specific fix of hedged ML-DSA-87 signing
// breaking aggregator selection: signatures the protocol hashes as
// pseudo-random values (selection proofs, RANDAO reveal) must be reproducible,
// while everything else keeps the (randomized) hedged mode.
func TestLocalKeymanager_Sign_DeterministicForSelectionProofsAndRandao(t *testing.T) {
	ctx := context.Background()
	dr, publicKeys := newKeymanagerWithRandomAccounts(t, 1)
	pubKey, err := ml_dsa_87.PublicKeyFromBytes(publicKeys[0][:])
	require.NoError(t, err)
	root := bytesutil.PadTo([]byte("signing root"), 32)

	signTwice := func(t *testing.T, newReq func() *validatorpb.SignRequest) ([]byte, []byte) {
		first, err := dr.Sign(ctx, newReq())
		require.NoError(t, err)
		second, err := dr.Sign(ctx, newReq())
		require.NoError(t, err)
		require.Equal(t, true, first.Verify(pubKey, root))
		require.Equal(t, true, second.Verify(pubKey, root))
		return first.Marshal(), second.Marshal()
	}

	deterministic := map[string]func() *validatorpb.SignRequest{
		"selection proof (slot)": func() *validatorpb.SignRequest {
			return &validatorpb.SignRequest{PublicKey: publicKeys[0][:], SigningRoot: root, Object: &validatorpb.SignRequest_Slot{Slot: 7}}
		},
		"sync committee selection proof": func() *validatorpb.SignRequest {
			return &validatorpb.SignRequest{PublicKey: publicKeys[0][:], SigningRoot: root, Object: &validatorpb.SignRequest_SyncAggregatorSelectionData{
				SyncAggregatorSelectionData: &qrysmpb.SyncAggregatorSelectionData{Slot: 7, SubcommitteeIndex: 1},
			}}
		},
		"randao reveal (epoch)": func() *validatorpb.SignRequest {
			return &validatorpb.SignRequest{PublicKey: publicKeys[0][:], SigningRoot: root, Object: &validatorpb.SignRequest_Epoch{Epoch: 3}}
		},
	}
	for name, newReq := range deterministic {
		t.Run(name, func(t *testing.T) {
			first, second := signTwice(t, newReq)
			require.DeepEqual(t, first, second, "%s must be signed deterministically", name)
		})
	}

	hedged := map[string]func() *validatorpb.SignRequest{
		"no object": func() *validatorpb.SignRequest {
			return &validatorpb.SignRequest{PublicKey: publicKeys[0][:], SigningRoot: root}
		},
		"attestation data": func() *validatorpb.SignRequest {
			return &validatorpb.SignRequest{PublicKey: publicKeys[0][:], SigningRoot: root, Object: &validatorpb.SignRequest_AttestationData{
				AttestationData: &qrysmpb.AttestationData{},
			}}
		},
	}
	for name, newReq := range hedged {
		t.Run(name, func(t *testing.T) {
			first, second := signTwice(t, newReq)
			require.DeepNotEqual(t, first, second, "%s must keep hedged (randomized) signing", name)
		})
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
	mlDSA87KeysCache = make(map[[field_params.MLDSA87PubkeyLength]byte]ml_dsa_87.MLDSA87Key)
	dr := &Keymanager{}
	_, err := dr.Sign(context.Background(), req)
	assert.ErrorContains(t, "no signing key found in keys cache", err)
}

func TestCreatePrintoutOfKeys(t *testing.T) {
	mk := func(b byte) []byte {
		k := make([]byte, 48)
		for i := range k {
			k[i] = b
		}
		return k
	}
	tests := []struct {
		name string
		keys [][]byte
		want string
	}{
		{name: "empty", keys: nil, want: ""},
		{name: "one key", keys: [][]byte{mk(0x01)}, want: "0x010101010101"},
		{name: "two keys", keys: [][]byte{mk(0x01), mk(0x02)}, want: "0x010101010101,0x020202020202"},
		{name: "three keys", keys: [][]byte{mk(0x01), mk(0x02), mk(0x03)}, want: "0x010101010101,0x020202020202,0x030303030303"},
		{name: "four keys", keys: [][]byte{mk(0x01), mk(0x02), mk(0x03), mk(0x04)}, want: "0x010101010101,0x020202020202,0x030303030303,0x040404040404"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CreatePrintoutOfKeys(tt.keys))
		})
	}
}

// Regression test: the hedged ML-DSA-87 signing scheme can fail (e.g. on
// entropy exhaustion). The keymanager must surface that as an error instead of
// returning a nil signature with a nil error, which callers would dereference
// and crash the whole validator client.
func TestLocalKeymanager_Sign_PropagatesSigningError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	pubKey := make([]byte, field_params.MLDSA87PubkeyLength)
	pubKey[0] = 0xAA
	signingErr := errors.New("entropy exhausted")
	secretKey := keymock.NewMockSecretKey(ctrl)
	secretKey.EXPECT().Sign(gomock.Any()).Return(nil, signingErr)

	lock.Lock()
	mlDSA87KeysCache[bytesutil.ToBytes2592(pubKey)] = secretKey
	lock.Unlock()
	defer func() {
		lock.Lock()
		delete(mlDSA87KeysCache, bytesutil.ToBytes2592(pubKey))
		lock.Unlock()
	}()

	km := &Keymanager{}
	sig, err := km.Sign(context.Background(), &validatorpb.SignRequest{
		PublicKey:   pubKey,
		SigningRoot: make([]byte, 32),
	})
	require.ErrorContains(t, "entropy exhausted", err)
	require.Equal(t, true, sig == nil)
}
