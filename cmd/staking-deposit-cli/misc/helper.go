package misc

import (
	"encoding/hex"
	"fmt"

	"github.com/theQRL/go-qrllib/common"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
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

func DecodeHex(hexString string) []byte {
	hexBytes, err := hex.DecodeString(hexString)
	if err != nil {
		panic(fmt.Errorf("failed to decode string %s | reason %v",
			hexString, err))
	}
	return hexBytes
}

func ToSizedDilithiumSignature(sig []byte) [dilithium2.CryptoBytes]byte {
	if len(sig) != dilithium2.CryptoBytes {
		panic(fmt.Errorf("cannot convert sig to sized dilithium sig, invalid sig length %d", len(sig)))
	}
	var sizedSig [dilithium2.CryptoBytes]byte
	copy(sizedSig[:], sig)
	return sizedSig
}

func ToSizedDilithiumPublicKey(pk []byte) [dilithium2.CryptoPublicKeyBytes]byte {
	if len(pk) != dilithium2.CryptoPublicKeyBytes {
		panic(fmt.Errorf("cannot convert pk to sized dilithium pk, invalid pk length %d", len(pk)))
	}
	var sizedPK [dilithium2.CryptoPublicKeyBytes]byte
	copy(sizedPK[:], pk)
	return sizedPK
}
