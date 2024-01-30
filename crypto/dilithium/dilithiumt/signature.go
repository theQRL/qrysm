package dilithiumt

import (
	"bytes"
	"fmt"

	"github.com/pkg/errors"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/crypto/bls/common"
	dilithiumCommon "github.com/theQRL/qrysm/v4/crypto/dilithium/common"
)

// Signature used in the BLS signature scheme.
type Signature struct {
	s *[dilithium2.CryptoBytes]uint8
}

func SignatureFromBytes(sig []byte) (common.Signature, error) {
	if len(sig) != dilithium2.CryptoBytes {
		return nil, fmt.Errorf("signature must be %d bytes", dilithium2.CryptoBytes)
	}
	var signature [dilithium2.CryptoBytes]uint8
	copy(signature[:], sig)
	return &Signature{s: &signature}, nil
}

func AggregateCompressedSignatures(multiSigs [][]byte) (common.Signature, error) {
	panic("AggregateCompressedSignatures not supported for dilithium")
}

func MultipleSignaturesFromBytes(multiSigs [][]byte) ([]common.Signature, error) {
	if len(multiSigs) == 0 {
		return nil, fmt.Errorf("0 signatures provided to the method")
	}
	for _, s := range multiSigs {
		if len(s) != dilithium2.CryptoBytes {
			return nil, fmt.Errorf("signature must be %d bytes", dilithium2.CryptoBytes)
		}
	}
	wrappedSigs := make([]common.Signature, len(multiSigs))
	for i, signature := range multiSigs {
		var copiedSig [dilithium2.CryptoBytes]uint8
		copy(copiedSig[:], signature)
		wrappedSigs[i] = &Signature{s: &copiedSig}
	}
	return wrappedSigs, nil
}

func (s *Signature) Verify(pubKey common.PublicKey, msg []byte) bool {
	return dilithium2.Verify(msg, *s.s, pubKey.(*PublicKey).p)
}

func (s *Signature) AggregateVerify(pubKeys []common.PublicKey, msgs [][32]byte) bool {
	panic("AggregateVerify not supported for dilithium")
}

func (s *Signature) FastAggregateVerify(pubKeys []common.PublicKey, msg [32]byte) bool {
	panic("FastAggregateVerify not supported for dilithium")
}

func (s *Signature) Eth2FastAggregateVerify(pubKeys []common.PublicKey, msg [32]byte) bool {
	if len(pubKeys) == 0 && bytes.Equal(s.Marshal(), dilithiumCommon.InfiniteSignature[:]) {
		return true
	}
	panic("Eth2FastAggregateVerify not supported for dilithium")
}

func NewAggregateSignature() common.Signature {
	panic("NewAggregateSignature not supported for dilithium")
}

func AggregateSignatures(sigs []common.Signature) common.Signature {
	panic("AggregateSignatures not supported for dilithium")
}

func UnaggregatedSignatures(sigs []common.Signature) []byte {
	if len(sigs) == 0 {
		return nil
	}

	unaggregatedSigns := make([]byte, dilithium2.CryptoBytes*len(sigs))
	offset := 0
	for i := 0; i < len(sigs); i++ {
		copy(unaggregatedSigns[offset:offset+dilithium2.CryptoBytes], sigs[i].Marshal())
		offset += dilithium2.CryptoBytes
	}
	return unaggregatedSigns
}

func VerifySignature(sig []byte, msg [32]byte, pubKey common.PublicKey) (bool, error) {
	rSig, err := SignatureFromBytes(sig)
	if err != nil {
		return false, err
	}
	return rSig.Verify(pubKey, msg[:]), nil
}

// VerifyMultipleSignatures TODO: (cyyber) make multiple parallel verification using go routine
func VerifyMultipleSignatures(sigs [][]byte, msgs [][32]byte, pubKeys [][]common.PublicKey) (bool, error) {
	if len(sigs) == 0 || len(pubKeys) == 0 {
		return false, nil
	}

	length := len(sigs)
	if length != len(pubKeys) || length != len(msgs) {
		return false, errors.Errorf("provided signatures, pubkeys and messages have differing lengths. S: %d, P: %d,M %d",
			length, len(pubKeys), len(msgs))
	}

	for i := range sigs {
		for pubKeyIndex, pubKey := range pubKeys[i] {
			offset := pubKeyIndex * dilithium2.CryptoBytes
			sig := sigs[i][offset : offset+dilithium2.CryptoBytes]
			if ok, err := VerifySignature(sig, msgs[i], pubKey); !ok {
				return ok, err
			}
		}
	}

	return true, nil
}

func (s *Signature) Marshal() []byte {
	return s.s[:]
}

func (s *Signature) Copy() common.Signature {
	sign := *s.s
	return &Signature{s: &sign}
}
