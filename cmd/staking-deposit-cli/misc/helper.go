package misc

import (
	"encoding/hex"
	"fmt"

	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
)

func StrSeedToBinSeed(strSeed string) [field_params.DilithiumSeedLength]uint8 {
	var seed [field_params.DilithiumSeedLength]uint8

	unSizedSeed := DecodeHex(strSeed)

	copy(seed[:], unSizedSeed)
	return seed
}

func DecodeHex(hexString string) []byte {
	if hexString[:2] != "0x" {
		panic(fmt.Errorf("invalid hex string prefix %s", hexString[:2]))
	}
	hexBytes, err := hex.DecodeString(hexString[2:])
	if err != nil {
		panic(fmt.Errorf("failed to decode string %s | reason %v",
			hexString, err))
	}
	return hexBytes
}

func EncodeHex(hexBytes []byte) string {
	return fmt.Sprintf("0x%x", hexBytes)
}

func ToSizedDilithiumSignature(sig []byte) [field_params.DilithiumSignatureLength]byte {
	if len(sig) != field_params.DilithiumSignatureLength {
		panic(fmt.Errorf("cannot convert sig to sized dilithium sig, invalid sig length %d", len(sig)))
	}
	var sizedSig [field_params.DilithiumSignatureLength]byte
	copy(sizedSig[:], sig)
	return sizedSig
}

func ToSizedDilithiumPublicKey(pk []byte) [field_params.DilithiumPubkeyLength]byte {
	if len(pk) != field_params.DilithiumPubkeyLength {
		panic(fmt.Errorf("cannot convert pk to sized dilithium pk, invalid pk length %d", len(pk)))
	}
	var sizedPK [field_params.DilithiumPubkeyLength]byte
	copy(sizedPK[:], pk)
	return sizedPK
}
