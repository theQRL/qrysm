package keyhandling

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/theQRL/go-qrllib/common"
	"github.com/theQRL/go-qrllib/dilithium"
	"golang.org/x/crypto/sha3"
	"io"
	"os"
	"reflect"
	"runtime"
)

type Keystore struct {
	Crypto      *KeystoreCrypto
	Description string
	PubKey      string
	Path        string
	UUID        string
	Version     string
}

func (k *Keystore) ToJSON() []byte {
	b, err := json.Marshal(k)
	if err != nil {
		panic("failed to marshal keystore to json")
	}
	return b
}

func (k *Keystore) Save(fileFolder string) error {
	if err := os.WriteFile(fileFolder, k.ToJSON(), 0644); err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		if err := os.Chmod(fileFolder, 0440); err != nil {
			return err
		}
	}
	return nil
}

func (k *Keystore) Decrypt(password string) []byte {
	salt, ok := k.Crypto.kdf.params["salt"]
	if !ok {
		panic("salt not found in kdf params")
	}
	decryptionKey, err := passwordToDecryptionKey(password, salt.([]byte))
	if err != nil {
		panic(fmt.Errorf("passwordToDecryptionKey | reason %v", err))
	}

	checksum := CheckSumDecryptionKeyAndMessage(decryptionKey[16:32], k.Crypto.cipher.message)
	if !reflect.DeepEqual(checksum[:], k.Crypto.checksum.message) {
		panic(fmt.Errorf("checksum check failed | expected %s | found %s",
			checksum, k.Crypto.checksum.message))
	}

	block, err := aes.NewCipher(decryptionKey[:16])
	if err != nil {
		panic(fmt.Errorf("aes.NewCipher failed | reason %v", err))
	}

	var seed [common.SeedSize]uint8
	cipherText := make([]byte, aes.BlockSize+len(seed))

	aesIV, ok := k.Crypto.kdf.params["iv"]
	if !ok {
		panic(fmt.Errorf("aesIV not found in kdf params"))
	}

	stream := cipher.NewCTR(block, aesIV.([]byte))
	stream.XORKeyStream(seed[:], cipherText[aes.BlockSize:])

	return seed[:]
}

func NewKeystoreFromJSON(data []uint8) *Keystore {
	k := &Keystore{}
	err := json.Unmarshal(data, k)
	if err != nil {
		panic(fmt.Errorf("failed to marshal keystore to json | reason %v", err))
	}
	return k
}

func NewKeystoreFromFile(path string) *Keystore {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Errorf("cannot read file %s | reason %v", path, err))
	}
	return NewKeystoreFromJSON(data)
}

func Encrypt(seed [common.SeedSize]uint8, password, path string, salt, aesIV []byte) (*Keystore, error) {
	if salt == nil {
		salt = make([]uint8, 256)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, err
		}
	}
	if aesIV == nil {
		aesIV = make([]uint8, 128)
		if _, err := io.ReadFull(rand.Reader, aesIV); err != nil {
			return nil, err
		}
	}

	decryptionKey, err := passwordToDecryptionKey(password, salt)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(decryptionKey[:16])
	if err != nil {
		return nil, err
	}

	cipherText := make([]byte, aes.BlockSize+len(seed))

	stream := cipher.NewCTR(block, aesIV)
	stream.XORKeyStream(cipherText[aes.BlockSize:], seed[:])

	d, err := dilithium.NewDilithiumFromSeed(seed)
	if err != nil {
		return nil, err
	}
	pk := d.GetPK()
	return &Keystore{
		UUID:   uuid.New().String(),
		Crypto: NewKeystoreCrypto(salt, aesIV, cipherText, decryptionKey[16:]),
		PubKey: hex.EncodeToString(pk[:]),
		Path:   path,
	}, nil
}

func passwordToDecryptionKey(password string, salt []byte) ([32]byte, error) {
	h := sha3.NewShake256()
	if _, err := h.Write([]byte(password)); err != nil {
		return [32]byte{}, fmt.Errorf("shake256 hash write failed %v", err)
	}

	if _, err := h.Write(salt); err != nil {
		return [32]byte{}, fmt.Errorf("shake256 hash write failed %v", err)
	}

	var decryptionKey [32]uint8
	_, err := h.Read(decryptionKey[:])
	return decryptionKey, err
}
