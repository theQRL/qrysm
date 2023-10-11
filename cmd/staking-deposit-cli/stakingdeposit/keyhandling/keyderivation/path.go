package keyderivation

import (
	"errors"
	"fmt"

	"github.com/cyyber/qrysm/v4/cmd/staking-deposit-cli/misc"
	"github.com/theQRL/go-qrllib/common"
	"github.com/theQRL/go-qrllib/dilithium"
	"golang.org/x/crypto/sha3"
)

// SeedAndPathToSeed TODO: (cyyber) algorithm needs to be reviewed in future
func SeedAndPathToSeed(strSeed, path string) (string, error) {
	binSeed := misc.DecodeHex(strSeed)
	if len(binSeed) != common.SeedSize {
		return "", fmt.Errorf("invalid seed size %d", len(binSeed))
	}

	var seed [common.SeedSize]uint8
	copy(seed[:], binSeed)

	h := sha3.NewShake256()
	if _, err := h.Write(seed[:]); err != nil {
		return "", fmt.Errorf("shake256 hash write failed %v", err)
	}
	if _, err := h.Write([]byte(path)); err != nil {
		return "", fmt.Errorf("shake256 hash write failed %v", err)
	}

	var newSeed [common.SeedSize]uint8
	_, err := h.Read(newSeed[:])

	// Try generating Dilithium from seed to ensure seed validity
	_, err = dilithium.NewDilithiumFromSeed(newSeed)
	if err != nil {
		return "", errors.New("could not generate dilithium from mnemonic")
	}

	return misc.EncodeHex(newSeed[:]), nil
}
