package keyderivation

import (
	"github.com/pkg/errors"
	goqrllib_misc "github.com/theQRL/go-qrllib/wallet/misc"
	"github.com/theQRL/qrysm/crypto/rand"
)

// mnemonicSeedSize is the number of random bytes encoded by the 32-word
// mnemonic: each word of the 4096-word list carries 12 bits, so 32 words hold
// 384 bits = 48 bytes.
const mnemonicSeedSize = 48

// GetRandomMnemonic returns a freshly generated 32-word mnemonic. The mnemonic
// is the root seed of every validator key derived by `deposit new-seed`, so the
// randomness is drawn from the cryptographically secure generator (crypto/rand)
// and encoded with the same word mapping used to decode it (MnemonicToBin),
// giving the full 384 bits of entropy. It must never use math/rand.
func GetRandomMnemonic() (string, error) {
	seed := make([]byte, mnemonicSeedSize)
	if _, err := rand.NewGenerator().Read(seed); err != nil {
		return "", errors.Wrap(err, "could not read random seed")
	}
	mnemonic, err := goqrllib_misc.BinToMnemonic(seed)
	if err != nil {
		return "", errors.Wrap(err, "could not convert seed to mnemonic")
	}
	return mnemonic, nil
}
