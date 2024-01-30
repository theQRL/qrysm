package local

import (
	"bytes"
	"context"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpbservice "github.com/theQRL/qrysm/v4/proto/zond/service"
)

// DeleteKeystores takes in public keys and removes the accounts from the wallet.
// This includes their disk keystore and cached keystore, but maintains the slashing
// protection history in the database.
// 1) Copy the in memory keystore
// 2) Delete the keys from copied in memory keystore
// 3) Save the copy to disk
// 4) Reinitialize account store and updating the keymanager
// 5) Return API response
func (km *Keymanager) DeleteKeystores(
	ctx context.Context, publicKeys [][]byte,
) ([]*zondpbservice.DeletedKeystoreStatus, error) {
	// Check for duplicate keys and filter them out.
	trackedPublicKeys := make(map[[dilithium.CryptoPublicKeyBytes]byte]bool)
	statuses := make([]*zondpbservice.DeletedKeystoreStatus, 0, len(publicKeys))
	deletedKeys := make([][]byte, 0, len(publicKeys))
	// 1) Copy the in memory keystore
	storeCopy := km.accountsStore.Copy()
	for _, publicKey := range publicKeys {
		// Check if the key in the request is a duplicate or not found
		if _, ok := trackedPublicKeys[bytesutil.ToBytes2592(publicKey)]; ok {
			statuses = append(statuses, &zondpbservice.DeletedKeystoreStatus{
				Status: zondpbservice.DeletedKeystoreStatus_NOT_ACTIVE,
			})
			continue
		}
		var index int
		var found bool
		for j, pubKey := range storeCopy.PublicKeys {
			if bytes.Equal(pubKey, publicKey) {
				index = j
				found = true
				break
			}
		}
		if !found {
			statuses = append(statuses, &zondpbservice.DeletedKeystoreStatus{
				Status: zondpbservice.DeletedKeystoreStatus_NOT_FOUND,
			})
			continue
		}
		// 2) Delete the keys from copied in memory keystore
		deletedPublicKey := storeCopy.PublicKeys[index]
		deletedKeys = append(deletedKeys, deletedPublicKey)
		storeCopy.Seeds = append(storeCopy.Seeds[:index], storeCopy.Seeds[index+1:]...)
		storeCopy.PublicKeys = append(storeCopy.PublicKeys[:index], storeCopy.PublicKeys[index+1:]...)
		statuses = append(statuses, &zondpbservice.DeletedKeystoreStatus{
			Status: zondpbservice.DeletedKeystoreStatus_DELETED,
		})
		trackedPublicKeys[bytesutil.ToBytes2592(publicKey)] = true
	}
	if len(deletedKeys) == 0 {
		return statuses, nil
	}
	// 3 & 4) save to disk and re-initializes keystore
	if err := km.SaveStoreAndReInitialize(ctx, storeCopy); err != nil {
		return nil, err
	}

	log.WithFields(logrus.Fields{
		"publicKeys": CreatePrintoutOfKeys(deletedKeys),
	}).Info("Successfully deleted validator key(s)")
	// 5) Return API response
	return statuses, nil
}
