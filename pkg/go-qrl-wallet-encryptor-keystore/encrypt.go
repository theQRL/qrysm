package keystorev1

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"golang.org/x/crypto/argon2"
)

const (
	// Algorithms.
	algoArgon2id = "argon2id"

	// Argon2id parameters.
	argon2idT      = 8
	argon2idM      = 1 << 18
	argon2idP      = 1
	argon2idKeyLen = 32

	// Ciphers.
	cipherAes256Gcm = "aes-256-gcm"

	// Misc constants.
	saltSize = 32
	ivSize   = 12
)

// Encrypt encrypts data.
func (e *Encryptor) Encrypt(secret []byte, passphrase string) (map[string]any, error) {
	if secret == nil {
		return nil, errors.New("no secret")
	}

	// Random salt.
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, errors.Wrap(err, "failed to obtain random salt")
	}

	normedPassphrase := []byte(normPassphrase(passphrase))

	decryptionKey, err := e.generateDecryptionKey(salt, normedPassphrase)
	if err != nil {
		return nil, err
	}

	aesCipher, err := aes.NewCipher(decryptionKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cipher")
	}

	// Random IV.
	iv := make([]byte, ivSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, errors.Wrap(err, "failed to obtain initialization vector")
	}

	// Generate the cipher message.
	block, err := cipher.NewGCM(aesCipher)
	if err != nil {
		return nil, errors.Wrap(err, "invalid cipher")
	}
	cipherMsg := block.Seal(nil, iv, secret, nil)

	kdf := e.buildKDF(salt)

	res, err := buildEncryptOutput(kdf, iv, cipherMsg)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (e *Encryptor) generateDecryptionKey(salt []byte, normedPassphrase []byte) ([]byte, error) {
	var decryptionKey []byte

	switch e.cipher {
	case algoArgon2id:
		decryptionKey = argon2.IDKey(normedPassphrase, salt, argon2idT, argon2idM, argon2idP, argon2idKeyLen)
	default:
		return nil, fmt.Errorf("unknown cipher %q", e.cipher)
	}

	return decryptionKey, nil
}

func (e *Encryptor) buildKDF(salt []byte) *ksKDF {
	var kdf *ksKDF
	if e.cipher == algoArgon2id {
		kdf = &ksKDF{
			Function: algoArgon2id,
			Params: &ksKDFParams{
				DKLen: argon2idKeyLen,
				T:     argon2idT,
				M:     argon2idM,
				P:     argon2idP,
				Salt:  hex.EncodeToString(salt),
			},
		}
	}

	return kdf
}

func buildEncryptOutput(kdf *ksKDF, iv []byte, cipherMsg []byte) (map[string]any, error) {
	output := &keystoreV1{
		KDF: kdf,
		Cipher: &ksCipher{
			Function: cipherAes256Gcm,
			Params: &ksCipherParams{
				IV: hex.EncodeToString(iv),
			},
			Message: hex.EncodeToString(cipherMsg),
		},
	}

	// We need to return a generic map; go to JSON and back to obtain it.
	bytes, err := json.Marshal(output)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal JSON")
	}
	res := make(map[string]any)
	err = json.Unmarshal(bytes, &res)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal JSON")
	}

	return res, nil
}
