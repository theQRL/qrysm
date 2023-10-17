package local

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	common2 "github.com/theQRL/go-qrllib/common"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	keystorev4 "github.com/theQRL/go-zond-wallet-encryptor-keystore"
	"github.com/theQRL/qrysm/v4/async/event"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	validatorpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/v4/runtime/interop"
	"github.com/theQRL/qrysm/v4/validator/accounts/iface"
	"github.com/theQRL/qrysm/v4/validator/accounts/petnames"
	"github.com/theQRL/qrysm/v4/validator/keymanager"
	"go.opencensus.io/trace"
)

var (
	lock               sync.RWMutex
	orderedPublicKeys  = make([][dilithium2.CryptoPublicKeyBytes]byte, 0)
	dilithiumKeysCache = make(map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey)
)

const (
	// KeystoreFileNameFormat exposes the filename the keystore should be formatted in.
	KeystoreFileNameFormat = "keystore-%d.json"
	// AccountsPath where all local keymanager keystores are kept.
	AccountsPath = "accounts"
	// AccountsKeystoreFileName exposes the name of the keystore file.
	AccountsKeystoreFileName = "all-accounts.keystore.json"
)

// Keymanager implementation for local keystores utilizing EIP-2335.
type Keymanager struct {
	wallet              iface.Wallet
	accountsStore       *accountStore
	accountsChangedFeed *event.Feed
}

// SetupConfig includes configuration values for initializing
// a keymanager, such as passwords, the wallet, and more.
type SetupConfig struct {
	Wallet           iface.Wallet
	ListenForChanges bool
}

// Defines a struct containing 1-to-1 corresponding
// private keys and public keys for Ethereum validators.
type accountStore struct {
	Seeds      [][]byte `json:"seeds"`
	PublicKeys [][]byte `json:"public_keys"`
}

// Copy creates a deep copy of accountStore
func (a *accountStore) Copy() *accountStore {
	storeCopy := &accountStore{}
	storeCopy.Seeds = bytesutil.SafeCopy2dBytes(a.Seeds)
	storeCopy.PublicKeys = bytesutil.SafeCopy2dBytes(a.PublicKeys)
	return storeCopy
}

// AccountsKeystoreRepresentation defines an internal Prysm representation
// of validator accounts, encrypted according to the EIP-2334 standard.
type AccountsKeystoreRepresentation struct {
	Crypto  map[string]interface{} `json:"crypto"`
	ID      string                 `json:"uuid"`
	Version uint                   `json:"version"`
	Name    string                 `json:"name"`
}

// ResetCaches for the keymanager.
func ResetCaches() {
	lock.Lock()
	orderedPublicKeys = make([][dilithium2.CryptoPublicKeyBytes]byte, 0)
	dilithiumKeysCache = make(map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey)
	lock.Unlock()
}

// NewKeymanager instantiates a new local keymanager from configuration options.
func NewKeymanager(ctx context.Context, cfg *SetupConfig) (*Keymanager, error) {
	k := &Keymanager{
		wallet:              cfg.Wallet,
		accountsStore:       &accountStore{},
		accountsChangedFeed: new(event.Feed),
	}

	if err := k.initializeAccountKeystore(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to initialize account store")
	}

	if cfg.ListenForChanges {
		// We begin a goroutine to listen for file changes to our
		// all-accounts.keystore.json file in the wallet directory.
		go k.listenForAccountChanges(ctx)
	}
	return k, nil
}

// InteropKeymanagerConfig is used on validator launch to initialize the keymanager.
// InteropKeys are used for testing purposes.
type InteropKeymanagerConfig struct {
	Offset           uint64
	NumValidatorKeys uint64
}

// NewInteropKeymanager instantiates a new imported keymanager with the deterministically generated interop keys.
// InteropKeys are used for testing purposes.
func NewInteropKeymanager(_ context.Context, offset, numValidatorKeys uint64) (*Keymanager, error) {
	k := &Keymanager{
		accountsChangedFeed: new(event.Feed),
	}
	if numValidatorKeys == 0 {
		return k, nil
	}
	secretKeys, publicKeys, err := interop.DeterministicallyGenerateKeys(offset, numValidatorKeys)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate interop keys")
	}
	lock.Lock()
	pubKeys := make([][dilithium2.CryptoPublicKeyBytes]byte, numValidatorKeys)
	for i := uint64(0); i < numValidatorKeys; i++ {
		publicKey := bytesutil.ToBytes2592(publicKeys[i].Marshal())
		pubKeys[i] = publicKey
		dilithiumKeysCache[publicKey] = secretKeys[i]
	}
	orderedPublicKeys = pubKeys
	lock.Unlock()
	return k, nil
}

// SubscribeAccountChanges creates an event subscription for a channel
// to listen for public key changes at runtime, such as when new validator accounts
// are imported into the keymanager while the validator process is running.
func (km *Keymanager) SubscribeAccountChanges(pubKeysChan chan [][dilithium2.CryptoPublicKeyBytes]byte) event.Subscription {
	return km.accountsChangedFeed.Subscribe(pubKeysChan)
}

// ValidatingAccountNames for a local keymanager.
func (_ *Keymanager) ValidatingAccountNames() ([]string, error) {
	lock.RLock()
	names := make([]string, len(orderedPublicKeys))
	for i, pubKey := range orderedPublicKeys {
		names[i] = petnames.DeterministicName(bytesutil.FromBytes2592(pubKey), "-")
	}
	lock.RUnlock()
	return names, nil
}

// Initialize public and secret key caches that are used to speed up the functions
// FetchValidatingPublicKeys and Sign
func (km *Keymanager) initializeKeysCachesFromKeystore() error {
	lock.Lock()
	defer lock.Unlock()
	count := len(km.accountsStore.Seeds)
	orderedPublicKeys = make([][dilithium2.CryptoPublicKeyBytes]byte, count)
	dilithiumKeysCache = make(map[[dilithium2.CryptoPublicKeyBytes]byte]dilithium.DilithiumKey, count)
	for i, publicKey := range km.accountsStore.PublicKeys {
		publicKey2592 := bytesutil.ToBytes2592(publicKey)
		orderedPublicKeys[i] = publicKey2592
		secretKey, err := dilithium.SecretKeyFromBytes(km.accountsStore.Seeds[i])
		if err != nil {
			return errors.Wrap(err, "failed to initialize keys caches from account keystore")
		}
		dilithiumKeysCache[publicKey2592] = secretKey
	}
	return nil
}

// FetchValidatingPublicKeys fetches the list of active public keys from the local account keystores.
func (_ *Keymanager) FetchValidatingPublicKeys(ctx context.Context) ([][dilithium2.CryptoPublicKeyBytes]byte, error) {
	ctx, span := trace.StartSpan(ctx, "keymanager.FetchValidatingPublicKeys")
	defer span.End()

	lock.RLock()
	keys := orderedPublicKeys
	result := make([][dilithium2.CryptoPublicKeyBytes]byte, len(keys))
	copy(result, keys)
	lock.RUnlock()
	return result, nil
}

// FetchValidatingSeeds fetches the list of private keys from the secret keys cache
func (km *Keymanager) FetchValidatingSeeds(ctx context.Context) ([][common2.SeedSize]byte, error) {
	lock.RLock()
	defer lock.RUnlock()
	dilithiumSeed := make([][common2.SeedSize]byte, len(dilithiumKeysCache))
	pubKeys, err := km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve public keys")
	}
	for i, pk := range pubKeys {
		seckey, ok := dilithiumKeysCache[pk]
		if !ok {
			return nil, errors.New("Could not fetch private key")
		}
		dilithiumSeed[i] = bytesutil.ToBytes48(seckey.Marshal())
	}
	return dilithiumSeed, nil
}

// Sign signs a message using a validator key.
func (_ *Keymanager) Sign(ctx context.Context, req *validatorpb.SignRequest) (dilithium.Signature, error) {
	publicKey := req.PublicKey
	if publicKey == nil {
		return nil, errors.New("nil public key in request")
	}
	lock.RLock()
	secretKey, ok := dilithiumKeysCache[bytesutil.ToBytes2592(publicKey)]
	lock.RUnlock()
	if !ok {
		return nil, errors.New("no signing key found in keys cache")
	}
	return secretKey.Sign(req.SigningRoot), nil
}

func (km *Keymanager) initializeAccountKeystore(ctx context.Context) error {
	encoded, err := km.wallet.ReadFileAtPath(ctx, AccountsPath, AccountsKeystoreFileName)
	if err != nil && strings.Contains(err.Error(), "no files found") {
		// If there are no keys to initialize at all, just exit.
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "could not read keystore file for accounts %s", AccountsKeystoreFileName)
	}
	keystoreFile := &AccountsKeystoreRepresentation{}
	if err := json.Unmarshal(encoded, keystoreFile); err != nil {
		return errors.Wrapf(err, "could not decode keystore file for accounts %s", AccountsKeystoreFileName)
	}
	// We extract the validator signing private key from the keystore
	// by utilizing the password and initialize a new BLS secret key from
	// its raw bytes.
	password := km.wallet.Password()
	decryptor := keystorev4.New()
	enc, err := decryptor.Decrypt(keystoreFile.Crypto, password)
	if err != nil && strings.Contains(err.Error(), keymanager.IncorrectPasswordErrMsg) {
		return errors.Wrap(err, "wrong password for wallet entered")
	} else if err != nil {
		return errors.Wrap(err, "could not decrypt keystore")
	}

	store := &accountStore{}
	if err := json.Unmarshal(enc[:], store); err != nil {
		return err
	}
	if len(store.PublicKeys) != len(store.Seeds) {
		return errors.New("unequal number of public keys and private keys")
	}
	if len(store.PublicKeys) == 0 {
		return nil
	}
	km.accountsStore = store
	err = km.initializeKeysCachesFromKeystore()
	if err != nil {
		return errors.Wrap(err, "failed to initialize keys caches")
	}
	return err
}

// CreateAccountsKeystore creates a new keystore holding the provided keys.
func (km *Keymanager) CreateAccountsKeystore(ctx context.Context, seeds,
	publicKeys [][]byte) (*AccountsKeystoreRepresentation, error) {
	if err := km.CreateOrUpdateInMemoryAccountsStore(ctx, seeds, publicKeys); err != nil {
		return nil, err
	}
	return CreateAccountsKeystoreRepresentation(ctx, km.accountsStore, km.wallet.Password())
}

// SaveStoreAndReInitialize saves the store to disk and re-initializes the account keystore from file
func (km *Keymanager) SaveStoreAndReInitialize(ctx context.Context, store *accountStore) error {
	// Save the copy to disk
	accountsKeystore, err := CreateAccountsKeystoreRepresentation(ctx, store, km.wallet.Password())
	if err != nil {
		return err
	}
	encodedAccounts, err := json.MarshalIndent(accountsKeystore, "", "\t")
	if err != nil {
		return err
	}
	if err := km.wallet.WriteFileAtPath(ctx, AccountsPath, AccountsKeystoreFileName, encodedAccounts); err != nil {
		return err
	}

	// Reinitialize account store and cache
	// This will update the in-memory information instead of reading from the file itself for safety concerns
	km.accountsStore = store
	err = km.initializeKeysCachesFromKeystore()
	if err != nil {
		return errors.Wrap(err, "failed to initialize keys caches")
	}
	return err
}

// CreateAccountsKeystoreRepresentation is a pure function that takes an accountStore and wallet password and returns the encrypted formatted json version for local writing.
func CreateAccountsKeystoreRepresentation(
	_ context.Context,
	store *accountStore,
	walletPW string,
) (*AccountsKeystoreRepresentation, error) {
	encryptor := keystorev4.New()
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	encodedStore, err := json.MarshalIndent(store, "", "\t")
	if err != nil {
		return nil, err
	}
	cryptoFields, err := encryptor.Encrypt(encodedStore, walletPW)
	if err != nil {
		return nil, errors.Wrap(err, "could not encrypt accounts")
	}
	return &AccountsKeystoreRepresentation{
		Crypto:  cryptoFields,
		ID:      id.String(),
		Version: encryptor.Version(),
		Name:    encryptor.Name(),
	}, nil
}

// CreateOrUpdateInMemoryAccountsStore will set or update the local accounts store and update the local cache.
// This function DOES NOT save the accounts store to disk.
func (km *Keymanager) CreateOrUpdateInMemoryAccountsStore(_ context.Context, seeds, publicKeys [][]byte) error {
	if len(seeds) != len(publicKeys) {
		return fmt.Errorf(
			"number of private keys and public keys is not equal: %d != %d", len(seeds), len(publicKeys),
		)
	}
	if km.accountsStore == nil {
		km.accountsStore = &accountStore{
			Seeds:      seeds,
			PublicKeys: publicKeys,
		}
	} else {
		updateAccountsStoreKeys(km.accountsStore, seeds, publicKeys)
	}
	err := km.initializeKeysCachesFromKeystore()
	if err != nil {
		return errors.Wrap(err, "failed to initialize keys caches")
	}
	return nil
}

func updateAccountsStoreKeys(store *accountStore, seeds, publicKeys [][]byte) {
	existingPubKeys := make(map[string]bool)
	existingPrivKeys := make(map[string]bool)
	for i := 0; i < len(store.Seeds); i++ {
		existingPrivKeys[string(store.Seeds[i])] = true
		existingPubKeys[string(store.PublicKeys[i])] = true
	}
	// We append to the accounts store keys only
	// if the private/secret key do not already exist, to prevent duplicates.
	for i := 0; i < len(seeds); i++ {
		sk := seeds[i]
		pk := publicKeys[i]
		_, privKeyExists := existingPrivKeys[string(sk)]
		_, pubKeyExists := existingPubKeys[string(pk)]
		if privKeyExists || pubKeyExists {
			continue
		}
		store.PublicKeys = append(store.PublicKeys, pk)
		store.Seeds = append(store.Seeds, sk)
	}
}

func (km *Keymanager) ListKeymanagerAccounts(ctx context.Context, cfg keymanager.ListKeymanagerAccountConfig) error {
	au := aurora.NewAurora(true)
	// We initialize the wallet's keymanager.
	accountNames, err := km.ValidatingAccountNames()
	if err != nil {
		return errors.Wrap(err, "could not fetch account names")
	}
	numAccounts := au.BrightYellow(len(accountNames))
	fmt.Printf("(keymanager kind) %s\n", au.BrightGreen("local wallet").Bold())
	fmt.Println("")
	if len(accountNames) == 1 {
		fmt.Printf("Showing %d validator account\n", numAccounts)
	} else {
		fmt.Printf("Showing %d validator accounts\n", numAccounts)
	}
	fmt.Println(
		au.BrightRed("View the eth1 deposit transaction data for your accounts " +
			"by running `validator accounts list --show-deposit-data`"),
	)

	pubKeys, err := km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return errors.Wrap(err, "could not fetch validating public keys")
	}
	var seeds [][common2.SeedSize]byte
	if cfg.ShowPrivateKeys {
		seeds, err = km.FetchValidatingSeeds(ctx)
		if err != nil {
			return errors.Wrap(err, "could not fetch private keys")
		}
	}
	for i := 0; i < len(accountNames); i++ {
		fmt.Println("")
		fmt.Printf("%s | %s\n", au.BrightBlue(fmt.Sprintf("Account %d", i)).Bold(), au.BrightGreen(accountNames[i]).Bold())
		fmt.Printf("%s %#x\n", au.BrightMagenta("[validating public key]").Bold(), pubKeys[i])
		if cfg.ShowPrivateKeys {
			if len(seeds) > i {
				fmt.Printf("%s %#x\n", au.BrightRed("[validating seeds]").Bold(), seeds[i])
			}
		}
		if !cfg.ShowDepositData {
			continue
		}
		fmt.Printf(
			"%s\n",
			au.BrightRed("If you imported your account coming from the eth2 launchpad, you will find your "+
				"deposit_data.json in the eth2.0-deposit-cli's validator_keys folder"),
		)
		fmt.Println("")
	}
	fmt.Println("")
	return nil
}

func CreatePrintoutOfKeys(keys [][]byte) string {
	var keysStr string
	for i, k := range keys {
		if i == 0 {
			keysStr += fmt.Sprintf("%#x", bytesutil.Trunc(k))
		} else if i == len(keys)-1 {
			keysStr += fmt.Sprintf("%#x", bytesutil.Trunc(k))
		} else {
			keysStr += fmt.Sprintf(",%#x", bytesutil.Trunc(k))
		}
	}
	return keysStr
}
