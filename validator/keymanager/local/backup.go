package local

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	keystorev1 "github.com/theQRL/qrysm/pkg/go-qrl-wallet-encryptor-keystore"
	"github.com/theQRL/qrysm/validator/keymanager"
)

// ExtractKeystores retrieves the secret keys for specified public keys
// in the function input, encrypts them using the specified password,
// and returns their respective EIP-2335 keystores.
func (*Keymanager) ExtractKeystores(
	_ context.Context, publicKeys []ml_dsa_87.PublicKey, password string,
) ([]*keymanager.Keystore, error) {
	lock.Lock()
	defer lock.Unlock()
	encryptor := keystorev1.New()
	keystores := make([]*keymanager.Keystore, len(publicKeys))
	for i, pk := range publicKeys {
		pubKeyBytes := pk.Marshal()
		secretKey, ok := mlDSA87KeysCache[bytesutil.ToBytes2592(pubKeyBytes)]
		if !ok {
			return nil, fmt.Errorf(
				"secret key for public key %#x not found in cache",
				pubKeyBytes,
			)
		}
		cryptoFields, err := encryptor.Encrypt(secretKey.Marshal(), password)
		if err != nil {
			return nil, errors.Wrapf(
				err,
				"could not encrypt secret key for public key %#x",
				pubKeyBytes,
			)
		}
		id, err := uuid.NewRandom()
		if err != nil {
			return nil, err
		}
		keystores[i] = &keymanager.Keystore{
			Crypto:      cryptoFields,
			ID:          id.String(),
			Pubkey:      fmt.Sprintf("%x", pubKeyBytes),
			Version:     encryptor.Version(),
			Description: encryptor.Name(),
		}
	}
	return keystores, nil
}
