package local

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/monitoring/progress"
	keystorev1 "github.com/theQRL/qrysm/pkg/go-qrl-wallet-encryptor-keystore"
	qrlpbservice "github.com/theQRL/qrysm/proto/qrl/service"
	"github.com/theQRL/qrysm/validator/keymanager"
)

// ImportKeystores into the local keymanager from an external source.
// 1) Copy the in memory keystore
// 2) Update copied keystore with new keys
// 3) Save the copy to disk
// 4) Reinitialize account store and updating the keymanager
// 5) Return Statuses
func (km *Keymanager) ImportKeystores(
	ctx context.Context,
	keystores []*keymanager.Keystore,
	passwords []string,
) ([]*qrlpbservice.ImportedKeystoreStatus, error) {
	if len(passwords) == 0 {
		return nil, ErrNoPasswords
	}
	if len(passwords) != len(keystores) {
		return nil, ErrMismatchedNumPasswords
	}
	enc := keystorev1.New()
	bar := progress.InitializeProgressBar(len(keystores), "Importing accounts...")
	keys := map[string]string{}
	statuses := make([]*qrlpbservice.ImportedKeystoreStatus, len(keystores))
	var err error
	// 1) Copy the in memory keystore
	storeCopy := km.accountsStore.Copy()
	importedKeys := make([][]byte, 0)
	existingPubKeys := make(map[string]bool)
	for i := 0; i < len(storeCopy.Seeds); i++ {
		existingPubKeys[string(storeCopy.PublicKeys[i])] = true
	}
	for i := range keystores {
		var seedBytes []byte
		var pubKeyBytes []byte
		seedBytes, pubKeyBytes, _, err = km.attemptDecryptKeystore(enc, keystores[i], passwords[i])
		if err != nil {
			statuses[i] = &qrlpbservice.ImportedKeystoreStatus{
				Status:  qrlpbservice.ImportedKeystoreStatus_ERROR,
				Message: err.Error(),
			}
			continue
		}
		if err := bar.Add(1); err != nil {
			log.Error(err)
		}
		// if key exists prior to being added then output log that duplicate key was found
		_, isDuplicateInArray := keys[string(pubKeyBytes)]
		_, isDuplicateInExisting := existingPubKeys[string(pubKeyBytes)]
		if isDuplicateInArray || isDuplicateInExisting {
			log.Warnf("Duplicate key in import will be ignored: %#x", pubKeyBytes)
			statuses[i] = &qrlpbservice.ImportedKeystoreStatus{
				Status: qrlpbservice.ImportedKeystoreStatus_DUPLICATE,
			}
			continue
		}

		keys[string(pubKeyBytes)] = string(seedBytes)
		importedKeys = append(importedKeys, pubKeyBytes)
		statuses[i] = &qrlpbservice.ImportedKeystoreStatus{
			Status: qrlpbservice.ImportedKeystoreStatus_IMPORTED,
		}
	}
	if len(importedKeys) == 0 {
		log.Warn("No keys were imported")
		return statuses, nil
	}
	// 2) Update copied keystore with new keys,clear duplicates in existing set
	// duplicates,errored ones are already skipped
	for pubKey, privKey := range keys {
		storeCopy.PublicKeys = append(storeCopy.PublicKeys, []byte(pubKey))
		storeCopy.Seeds = append(storeCopy.Seeds, []byte(privKey))
	}
	//3 & 4) save to disk and re-initializes keystore
	if err := km.SaveStoreAndReInitialize(ctx, storeCopy); err != nil {
		return nil, err
	}

	log.WithFields(logrus.Fields{
		"publicKeys": CreatePrintoutOfKeys(importedKeys),
	}).Info("Successfully imported validator key(s)")

	// 5) Return Statuses
	return statuses, nil
}

// ImportKeypairs directly into the keymanager.
func (km *Keymanager) ImportKeypairs(ctx context.Context, privKeys, pubKeys [][]byte) error {
	if len(privKeys) != len(pubKeys) {
		return fmt.Errorf(
			"number of private keys and public keys is not equal: %d != %d", len(privKeys), len(pubKeys),
		)
	}
	// 1) Copy the in memory keystore
	storeCopy := km.accountsStore.Copy()

	// 2) Update store and remove duplicates
	updateAccountsStoreKeys(storeCopy, privKeys, pubKeys)

	// 3 & 4) save to disk and re-initializes keystore
	if err := km.SaveStoreAndReInitialize(ctx, storeCopy); err != nil {
		return err
	}
	// 5) verify if store was not updated
	if len(km.accountsStore.PublicKeys) < len(storeCopy.PublicKeys) {
		return fmt.Errorf("keys were not imported successfully, expected %d got %d", len(storeCopy.PublicKeys), len(km.accountsStore.PublicKeys))
	}
	return nil
}

// Retrieves the private key and public key from an EIP-2335 keystore file
// by decrypting using a specified password. If the password fails,
// it prompts the user for the correct password until it confirms.
func (*Keymanager) attemptDecryptKeystore(
	enc *keystorev1.Encryptor, keystore *keymanager.Keystore, password string,
) ([]byte, []byte, string, error) {
	// Attempt to decrypt the keystore with the specifies password.
	var seedBytes []byte
	var err error
	seedBytes, err = enc.Decrypt(keystore.Crypto, password)
	doesNotDecrypt := err != nil && strings.Contains(err.Error(), keymanager.IncorrectPasswordErrMsg)
	if doesNotDecrypt {
		return nil, nil, "", fmt.Errorf(
			"incorrect password for key %s %v",
			keystore.Pubkey[:12], err,
		)
	}
	if err != nil && !strings.Contains(err.Error(), keymanager.IncorrectPasswordErrMsg) {
		return nil, nil, "", errors.Wrap(err, "could not decrypt keystore")
	}
	var pubKeyBytes []byte
	// Attempt to use the pubkey present in the keystore itself as a field. If unavailable,
	// then utilize the public key directly from the private key.
	if keystore.Pubkey != "" {
		pubKeyBytes, err = hex.DecodeString(strings.TrimPrefix(keystore.Pubkey, "0x"))
		if err != nil {
			return nil, nil, "", errors.Wrap(err, "could not decode pubkey from keystore")
		}
	} else {
		privKey, err := ml_dsa_87.SecretKeyFromSeed(seedBytes)
		if err != nil {
			return nil, nil, "", errors.Wrap(err, "could not initialize private key from bytes")
		}
		pubKeyBytes = privKey.PublicKey().Marshal()
	}
	return seedBytes, pubKeyBytes, password, nil
}
