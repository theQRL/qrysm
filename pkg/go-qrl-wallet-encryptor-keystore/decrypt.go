package keystorev1

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"golang.org/x/crypto/argon2"
)

// Decrypt decrypts the data provided, returning the secret.
func (e *Encryptor) Decrypt(input map[string]any, passphrase string) ([]byte, error) {
	if input == nil {
		return nil, errors.New("no data supplied")
	}
	// Marshal the map and unmarshal it back in to a keystore format so we can work with it.
	data, err := json.Marshal(input)
	if err != nil {
		return nil, errors.New("failed to parse keystore")
	}

	ks := &keystoreV1{}
	err = json.Unmarshal(data, &ks)
	if err != nil {
		return nil, errors.New("failed to parse keystore")
	}

	if ks.Cipher == nil {
		return nil, errors.New("no cipher")
	}

	normedPassphrase := []byte(normPassphrase(passphrase))
	res, err := decryptNorm(ks, normedPassphrase)
	if err != nil {
		// There is an alternate method to generate a normalised
		// passphrase that can produce different results.  To allow
		// decryption of data that may have been encrypted with the
		// alternate method we attempt to decrypt using that method
		// given the failure of the standard normalised method.
		normedPassphrase = []byte(altNormPassphrase(passphrase))

		res, err = decryptNorm(ks, normedPassphrase)
		if err != nil {
			// No luck either way.
			return nil, err
		}
	}

	return res, nil
}

func decryptNorm(ks *keystoreV1, normedPassphrase []byte) ([]byte, error) {
	decryptionKey, err := obtainDecryptionKey(ks, normedPassphrase)
	if err != nil {
		return nil, err
	}

	cipherMsg, err := hex.DecodeString(ks.Cipher.Message)
	if err != nil {
		return nil, errors.New("invalid cipher message")
	}

	// Decrypt.
	var res []byte
	switch ks.Cipher.Function {
	case cipherAes256Gcm:
		aesCipher, err := aes.NewCipher(decryptionKey)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create AES cipher")
		}

		iv, err := hex.DecodeString(ks.Cipher.Params.IV)
		if err != nil {
			return nil, errors.Wrap(err, "invalid IV")
		}

		block, err := cipher.NewGCM(aesCipher)
		if err != nil {
			return nil, errors.Wrap(err, "invalid cipher")
		}
		res, err = block.Open(nil, iv, cipherMsg, nil)
		if err != nil {
			return nil, errors.Wrap(err, "failed to decrypt and authenticate ciphertext")
		}
	default:
		return nil, fmt.Errorf("unsupported cipher %q", ks.Cipher.Function)
	}

	return res, nil
}

func obtainDecryptionKey(ks *keystoreV1, normedPassphrase []byte) ([]byte, error) {
	// Decryption key.
	var decryptionKey []byte
	if ks.KDF == nil {
		decryptionKey = normedPassphrase
	} else {
		kdfParams := ks.KDF.Params
		salt, err := hex.DecodeString(kdfParams.Salt)
		if err != nil {
			return nil, errors.New("invalid KDF salt")
		}
		switch ks.KDF.Function {
		case algoArgon2id:
			decryptionKey = argon2.IDKey(
				normedPassphrase,
				salt,
				uint32(kdfParams.T),
				uint32(kdfParams.M),
				uint8(kdfParams.P),
				uint32(kdfParams.DKLen),
			)
		default:
			return nil, fmt.Errorf("unsupported KDF %q", ks.KDF.Function)
		}
	}

	return decryptionKey, nil
}
