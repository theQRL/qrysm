package keyhandling

import "crypto/sha256"

type KeystoreCrypto struct {
	kdf      *KeystoreModule
	checksum *KeystoreModule
	cipher   *KeystoreModule
}

func NewKeystoreCryptoFromJSON() *KeystoreCrypto {
	return nil
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
		kdf: &KeystoreModule{
			params: map[string]interface{}{"salt": salt},
		},
		cipher: &KeystoreModule{
			params:  map[string]interface{}{"iv": aesIV},
			message: cipherText,
		},
		checksum: &KeystoreModule{
			message: checksum[:],
		},
	}
}
