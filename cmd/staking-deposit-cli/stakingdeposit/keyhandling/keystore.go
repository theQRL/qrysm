package keyhandling

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"

	"github.com/google/uuid"
	"github.com/theQRL/go-qrllib/wallet/ml_dsa_87"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/misc"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"golang.org/x/crypto/argon2"
)

const (
	// Argon2id parameters.
	standardArgon2idT uint32 = 8
	standardArgon2idM uint32 = 1 << 18
	standardArgon2idP uint8  = 1
	lightArgon2idT    uint32 = 8
	lightArgon2idM    uint32 = 1 << 12
	lightArgon2idP    uint8  = 1
	argon2idKeyLen           = 32

	// Misc constants.
	saltSize = 32
	ivSize   = 12
)

type Keystore struct {
	Crypto      *KeystoreCrypto `json:"crypto"`
	Description string          `json:"description"`
	PubKey      string          `json:"pubkey"`
	Path        string          `json:"path"`
	UUID        string          `json:"uuid"`
	Version     uint64          `json:"version"`
}

func (k *Keystore) ToJSON() []byte {
	b, err := json.Marshal(k)
	if err != nil {
		panic("failed to marshal keystore to json")
	}
	return b
}

func (k *Keystore) Save(fileFolder string) error {
	f, err := os.Create(fileFolder)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Println(err)
		}
	}()
	if _, err := f.Write(k.ToJSON()); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(fileFolder, 0440); err != nil {
			return err
		}
	}
	return nil
}

func (k *Keystore) Decrypt(password string) [field_params.MLDSA87SeedLength]byte {
	iv, err := hex.DecodeString(k.Crypto.Cipher.Params["iv"].(string))
	if err != nil {
		panic(fmt.Errorf("iv hex.DecodeString failed | reason %v", err))
	}

	salt, err := hex.DecodeString(k.Crypto.KDF.Params["salt"].(string))
	if err != nil {
		panic(fmt.Errorf("salt hex.DecodeString failed | reason %v", err))
	}

	ciphertext, err := hex.DecodeString(k.Crypto.Cipher.Message)
	if err != nil {
		panic(fmt.Errorf("salt hex.DecodeString failed | reason %v", err))
	}

	dkLen := uint32(ensureInt(k.Crypto.KDF.Params["dklen"]))

	t := uint32(ensureInt(k.Crypto.KDF.Params["t"]))
	m := uint32(ensureInt(k.Crypto.KDF.Params["m"]))
	p := uint8(ensureInt(k.Crypto.KDF.Params["p"]))

	derivedKey := argon2.IDKey([]byte(password), salt, t, m, p, dkLen)
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		panic(fmt.Errorf("aes.NewCipher failed | reason %v", err))
	}

	gcmBlock, err := cipher.NewGCM(block)
	if err != nil {
		panic(fmt.Errorf("cipher.NewGCM failed | reason %v", err))
	}

	plaintext, err := gcmBlock.Open(nil, iv, ciphertext, nil)
	if err != nil {
		panic(fmt.Errorf("gcmBlock.Open failed | reason %v", err))
	}

	return bytesutil.ToBytes48(plaintext)
}

func NewKeystoreFromJSON(data []uint8) *Keystore {
	k := NewEmptyKeystore()
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

func NewEmptyKeystore() *Keystore {
	k := &Keystore{}
	k.Crypto = NewEmptyKeystoreCrypto()
	return k
}

func Encrypt(seed [field_params.MLDSA87SeedLength]uint8, password, path string, lightKDF bool, salt, aesIV []byte) (*Keystore, error) {
	if salt == nil {
		salt = make([]uint8, saltSize)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, err
		}
	}
	if aesIV == nil {
		aesIV = make([]uint8, ivSize)
		if _, err := io.ReadFull(rand.Reader, aesIV); err != nil {
			return nil, err
		}
	}

	t, m, p := standardArgon2idT, standardArgon2idM, standardArgon2idP
	if lightKDF {
		t, m, p = lightArgon2idT, lightArgon2idM, lightArgon2idP
	}
	derivedKey := argon2.IDKey([]byte(password), salt, t, m, p, argon2idKeyLen)

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, err
	}

	gcmBlock, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	ciphertext := gcmBlock.Seal(nil, aesIV, seed[:], nil)

	w, err := ml_dsa_87.NewWalletFromSeed(seed)
	if err != nil {
		return nil, err
	}
	pk := w.GetPK()
	return &Keystore{
		UUID:   uuid.New().String(),
		Crypto: NewKeystoreCrypto(salt, aesIV, ciphertext, t, m, p, argon2idKeyLen),
		PubKey: misc.EncodeHex(pk[:]),
		Path:   path,
	}, nil
}

// TODO: can we do without this when unmarshalling dynamic JSON?
// why do integers in KDF params end up as float64 and not int after
// unmarshal?
func ensureInt(x any) int {
	res, ok := x.(int)
	if !ok {
		res = int(x.(float64))
	}
	return res
}
