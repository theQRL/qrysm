package dilithium

import (
	"github.com/theQRL/qrysm/v4/crypto/bls/common"
	"github.com/theQRL/qrysm/v4/crypto/dilithium/dilithiumt"
)

// TODO (cyyber): Rename SecretKeyFromBytes to SecretKeyFromSeed
func SecretKeyFromBytes(seed []byte) (DilithiumKey, error) {
	return dilithiumt.SecretKeyFromBytes(seed)
}

func PublicKeyFromBytes(pubKey []byte) (PublicKey, error) {
	return dilithiumt.PublicKeyFromBytes(pubKey)
}

func SignatureFromBytes(sig []byte) (Signature, error) {
	return dilithiumt.SignatureFromBytes(sig)
}

func MultipleSignaturesFromBytes(sigs [][]byte) ([]Signature, error) {
	return dilithiumt.MultipleSignaturesFromBytes(sigs)
}

func AggregatePublicKeys(pubs [][]byte) (PublicKey, error) {
	return dilithiumt.AggregatePublicKeys(pubs)
}

func AggregateMultiplePubkeys(pubs []PublicKey) PublicKey {
	return dilithiumt.AggregateMultiplePubkeys(pubs)
}

func AggregateSignatures(sigs []common.Signature) common.Signature {
	return dilithiumt.AggregateSignatures(sigs)
}

func UnaggregatedSignatures(sigs []common.Signature) []byte {
	return dilithiumt.UnaggregatedSignatures(sigs)
}

func AggregateCompressedSignatures(multiSigs [][]byte) (common.Signature, error) {
	return dilithiumt.AggregateCompressedSignatures(multiSigs)
}

func VerifySignature(sig []byte, msg [32]byte, pubKey common.PublicKey) (bool, error) {
	return dilithiumt.VerifySignature(sig, msg, pubKey)
}

func VerifyMultipleSignatures(sigs [][]byte, msgs [][32]byte, pubKeys [][]common.PublicKey) (bool, error) {
	return dilithiumt.VerifyMultipleSignatures(sigs, msgs, pubKeys)
}

func NewAggregateSignature() common.Signature {
	return dilithiumt.NewAggregateSignature()
}

func RandKey() (common.SecretKey, error) {
	return dilithiumt.RandKey()
}
