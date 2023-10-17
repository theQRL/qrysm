package keyhandling

import (
	"crypto/sha256"

	"github.com/theQRL/qrysm/v4/cmd/staking-deposit-cli/misc"
)

type KeystoreCrypto struct {
	KDF      *KeystoreModule `json:"kdf"`
	Checksum *KeystoreModule `json:"checksum"`
	Cipher   *KeystoreModule `json:"cipher"`
}

func CheckSumDecryptionKeyAndMessage(partialDecryptionKey, cipherText []uint8) [32]byte {
	var keyAndCipherText []byte
	keyAndCipherText = append(keyAndCipherText, partialDecryptionKey...)
	keyAndCipherText = append(keyAndCipherText, cipherText...)
	return sha256.Sum256(keyAndCipherText)
}

func NewKeystoreCrypto(salt, aesIV, cipherText, partialDecryptionKey []uint8) *KeystoreCrypto {
	checksum := CheckSumDecryptionKeyAndMessage(partialDecryptionKey, cipherText)

	return &KeystoreCrypto{
		KDF: &KeystoreModule{
			Function: "custom",
			Params:   map[string]interface{}{"salt": misc.EncodeHex(salt)},
		},
		Cipher: &KeystoreModule{
			Function: "aes-128-ctr",
			Params:   map[string]interface{}{"iv": misc.EncodeHex(aesIV)},
			Message:  misc.EncodeHex(cipherText),
		},
		Checksum: &KeystoreModule{
			Function: "sha256",
			Params:   map[string]interface{}{},
			Message:  misc.EncodeHex(checksum[:]),
		},
	}
}

func NewEmptyKeystoreCrypto() *KeystoreCrypto {
	return &KeystoreCrypto{
		KDF: &KeystoreModule{
			Params: map[string]interface{}{},
		},
		Cipher: &KeystoreModule{
			Params: map[string]interface{}{},
		},
		Checksum: &KeystoreModule{
			Params: map[string]interface{}{},
		},
	}
}
