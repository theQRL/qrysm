package derived

import (
	"context"
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	common2 "github.com/theQRL/go-qrllib/common"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/async/event"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	zondpbservice "github.com/theQRL/qrysm/v4/proto/zond/service"
	validatorpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/v4/validator/accounts/iface"
	"github.com/theQRL/qrysm/v4/validator/keymanager"
	"github.com/theQRL/qrysm/v4/validator/keymanager/local"
	util "github.com/wealdtech/go-eth2-util"
)

const (
	// DerivationPathFormat describes the structure of how keys are derived from a master key.
	DerivationPathFormat = "m / purpose / coin_type / account_index / withdrawal_key / validating_key"
	// ValidatingKeyDerivationPathTemplate defining the hierarchical path for validating
	// keys for Prysm Ethereum validators. According to EIP-2334, the format is as follows:
	// m / purpose / coin_type / account_index / withdrawal_key / validating_key
	ValidatingKeyDerivationPathTemplate = "m/12381/3600/%d/0/0"
)

// SetupConfig includes configuration values for initializing
// a keymanager, such as passwords, the wallet, and more.
type SetupConfig struct {
	Wallet           iface.Wallet
	ListenForChanges bool
}

// Keymanager implementation for derived, HD keymanager using EIP-2333 and EIP-2334.
type Keymanager struct {
	localKM *local.Keymanager
}

// NewKeymanager instantiates a new derived keymanager from configuration options.
func NewKeymanager(
	ctx context.Context,
	cfg *SetupConfig,
) (*Keymanager, error) {
	localKM, err := local.NewKeymanager(ctx, &local.SetupConfig{
		Wallet:           cfg.Wallet,
		ListenForChanges: cfg.ListenForChanges,
	})
	if err != nil {
		return nil, err
	}
	return &Keymanager{
		localKM: localKM,
	}, nil
}

// RecoverAccountsFromMnemonic given a mnemonic phrase, is able to regenerate N accounts
// from a derived seed, encrypt them according to the EIP-2334 JSON standard, and write them
// to disk. Then, the mnemonic is never stored nor used by the validator.
func (km *Keymanager) RecoverAccountsFromMnemonic(
	ctx context.Context, mnemonic, mnemonicLanguage, mnemonicPassphrase string, numAccounts int,
) error {
	seed, err := seedFromMnemonic(mnemonic, mnemonicLanguage, mnemonicPassphrase)
	if err != nil {
		return errors.Wrap(err, "could not initialize new wallet seed file")
	}
	privKeys := make([][]byte, numAccounts)
	pubKeys := make([][]byte, numAccounts)
	for i := 0; i < numAccounts; i++ {
		privKey, err := util.PrivateKeyFromSeedAndPath(
			seed, fmt.Sprintf(ValidatingKeyDerivationPathTemplate, i),
		)
		if err != nil {
			return err
		}
		privKeys[i] = privKey.Marshal()
		pubKeys[i] = privKey.PublicKey().Marshal()
	}
	return km.localKM.ImportKeypairs(ctx, privKeys, pubKeys)
}

// ExtractKeystores retrieves the secret keys for specified public keys
// in the function input, encrypts them using the specified password,
// and returns their respective EIP-2335 keystores.
func (km *Keymanager) ExtractKeystores(
	ctx context.Context, publicKeys []dilithium.PublicKey, password string,
) ([]*keymanager.Keystore, error) {
	return km.localKM.ExtractKeystores(ctx, publicKeys, password)
}

// ValidatingAccountNames for the derived keymanager.
func (km *Keymanager) ValidatingAccountNames(_ context.Context) ([]string, error) {
	return km.localKM.ValidatingAccountNames()
}

// Sign signs a message using a validator key.
func (km *Keymanager) Sign(ctx context.Context, req *validatorpb.SignRequest) (dilithium.Signature, error) {
	return km.localKM.Sign(ctx, req)
}

// FetchValidatingPublicKeys fetches the list of validating public keys from the keymanager.
func (km *Keymanager) FetchValidatingPublicKeys(ctx context.Context) ([][dilithium2.CryptoPublicKeyBytes]byte, error) {
	return km.localKM.FetchValidatingPublicKeys(ctx)
}

// FetchValidatingSeeds fetches the list of validating private keys from the keymanager.
func (km *Keymanager) FetchValidatingSeeds(ctx context.Context) ([][common2.SeedSize]byte, error) {
	return km.localKM.FetchValidatingSeeds(ctx)
}

// ImportKeystores for a derived keymanager.
func (km *Keymanager) ImportKeystores(
	ctx context.Context, keystores []*keymanager.Keystore, passwords []string,
) ([]*zondpbservice.ImportedKeystoreStatus, error) {
	return km.localKM.ImportKeystores(ctx, keystores, passwords)
}

// DeleteKeystores for a derived keymanager.
func (km *Keymanager) DeleteKeystores(
	ctx context.Context, publicKeys [][]byte,
) ([]*zondpbservice.DeletedKeystoreStatus, error) {
	return km.localKM.DeleteKeystores(ctx, publicKeys)
}

// SubscribeAccountChanges creates an event subscription for a channel
// to listen for public key changes at runtime, such as when new validator accounts
// are imported into the keymanager while the validator process is running.
func (km *Keymanager) SubscribeAccountChanges(pubKeysChan chan [][dilithium2.CryptoPublicKeyBytes]byte) event.Subscription {
	return km.localKM.SubscribeAccountChanges(pubKeysChan)
}

func (km *Keymanager) ListKeymanagerAccounts(ctx context.Context, cfg keymanager.ListKeymanagerAccountConfig) error {
	au := aurora.NewAurora(true)
	fmt.Printf("(keymanager kind) %s\n", au.BrightGreen("derived, (HD) hierarchical-deterministic").Bold())
	fmt.Printf("(derivation format) %s\n", au.BrightGreen(DerivationPathFormat).Bold())
	validatingPubKeys, err := km.FetchValidatingPublicKeys(ctx)
	if err != nil {
		return errors.Wrap(err, "could not fetch validating public keys")
	}
	var validatingPrivateKeys [][common2.SeedSize]byte
	if cfg.ShowPrivateKeys {
		validatingPrivateKeys, err = km.FetchValidatingSeeds(ctx)
		if err != nil {
			return errors.Wrap(err, "could not fetch validating private keys")
		}
	}
	accountNames, err := km.ValidatingAccountNames(ctx)
	if err != nil {
		return err
	}
	if len(accountNames) == 1 {
		fmt.Print("Showing 1 validator account\n")
	} else if len(accountNames) == 0 {
		fmt.Print("No accounts found\n")
		return nil
	} else {
		fmt.Printf("Showing %d validator accounts\n", len(accountNames))
	}
	for i := 0; i < len(accountNames); i++ {
		fmt.Println("")
		validatingKeyPath := fmt.Sprintf(ValidatingKeyDerivationPathTemplate, i)

		// Retrieve the withdrawal key account metadata.
		fmt.Printf("%s | %s\n", au.BrightBlue(fmt.Sprintf("Account %d", i)).Bold(), au.BrightGreen(accountNames[i]).Bold())
		// Retrieve the validating key account metadata.
		fmt.Printf("%s %#x\n", au.BrightCyan("[validating public key]").Bold(), validatingPubKeys[i])
		if cfg.ShowPrivateKeys && validatingPrivateKeys != nil {
			fmt.Printf("%s %#x\n", au.BrightRed("[validating private key]").Bold(), validatingPrivateKeys[i])
		}
		fmt.Printf("%s %s\n", au.BrightCyan("[derivation path]").Bold(), validatingKeyPath)
		fmt.Println(" ")
	}
	return nil
}
