package misc

import (
	"encoding/hex"
	"fmt"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
)

func StrSeedToBinSeed(strSeed string) [fieldparams.MLDSA87SeedLength]uint8 {
	var seed [fieldparams.MLDSA87SeedLength]uint8

	unSizedSeed := DecodeHex(strSeed)

	copy(seed[:], unSizedSeed)
	return seed
}

func DecodeHex(hexString string) []byte {
	if hexString[:2] == "0x" {
		hexString = hexString[2:]
	}
	hexBytes, err := hex.DecodeString(hexString)
	if err != nil {
		panic(fmt.Errorf("failed to decode string %s | reason %v",
			hexString, err))
	}
	return hexBytes
}

func EncodeHex(hexBytes []byte) string {
	return fmt.Sprintf("0x%x", hexBytes)
}

func ToSizedMLDSA87Signature(sig []byte) [fieldparams.MLDSA87SignatureLength]byte {
	if len(sig) != fieldparams.MLDSA87SignatureLength {
		panic(fmt.Errorf("cannot convert sig to sized ml-dsa-87 sig, invalid sig length %d", len(sig)))
	}
	var sizedSig [fieldparams.MLDSA87SignatureLength]byte
	copy(sizedSig[:], sig)
	return sizedSig
}

func ToSizedMLDSA87PublicKey(pk []byte) [fieldparams.MLDSA87PubkeyLength]byte {
	if len(pk) != fieldparams.MLDSA87PubkeyLength {
		panic(fmt.Errorf("cannot convert pk to sized ml-dsa-87 pk, invalid pk length %d", len(pk)))
	}
	var sizedPK [fieldparams.MLDSA87PubkeyLength]byte
	copy(sizedPK[:], pk)
	return sizedPK
}
