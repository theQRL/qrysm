package keyhandling

import (
	"crypto/sha256"
	"encoding/hex"
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
			Params: map[string]interface{}{"salt": hex.EncodeToString(salt)},
		},
		Cipher: &KeystoreModule{
			Function: "aes-128-ctr",
			Params:   map[string]interface{}{"iv": hex.EncodeToString(aesIV)},
			Message:  hex.EncodeToString(cipherText),
		},
		Checksum: &KeystoreModule{
			Function: "sha256",
			Params:   map[string]interface{}{},
			Message:  hex.EncodeToString(checksum[:]),
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
