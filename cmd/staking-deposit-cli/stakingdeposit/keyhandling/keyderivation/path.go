package keyderivation

import (
	"errors"
	"fmt"

	walletmldsa87 "github.com/theQRL/go-qrllib/wallet/ml_dsa_87"
	"github.com/theQRL/qrysm/cmd/staking-deposit-cli/misc"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"golang.org/x/crypto/sha3"
)

// SeedAndPathToSeed TODO: (cyyber) algorithm needs to be reviewed in future
func SeedAndPathToSeed(strSeed, path string) (string, error) {
	binSeed := misc.DecodeHex(strSeed)
	if len(binSeed) != fieldparams.MLDSA87SeedLength {
		return "", fmt.Errorf("invalid seed size %d", len(binSeed))
	}

	var seed [fieldparams.MLDSA87SeedLength]uint8
	copy(seed[:], binSeed)

	h := sha3.NewShake256()
	if _, err := h.Write(seed[:]); err != nil {
		return "", fmt.Errorf("shake256 hash write failed %v", err)
	}
	if _, err := h.Write([]byte(path)); err != nil {
		return "", fmt.Errorf("shake256 hash write failed %v", err)
	}

	var newSeed [fieldparams.MLDSA87SeedLength]uint8
	_, err := h.Read(newSeed[:])
	if err != nil {
		return "", err
	}

	// Try generating ML-DSA-87 from seed to ensure seed validity
	_, err = walletmldsa87.NewWalletFromSeed(newSeed)
	if err != nil {
		return "", errors.New("could not generate ml-dsa-87 from mnemonic")
	}

	return misc.EncodeHex(newSeed[:]), nil
}
