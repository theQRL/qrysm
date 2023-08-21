package misc

import (
	"encoding/hex"
	"github.com/theQRL/go-qrllib/common"
)

func StrSeedToBinSeed(strSeed string) [common.SeedSize]uint8 {
	var seed [common.SeedSize]uint8

	unSizedSeed, err := hex.DecodeString(strSeed)
	if err != nil {
		panic("failed to decode string")
	}

	copy(seed[:], unSizedSeed)
	return seed
}
