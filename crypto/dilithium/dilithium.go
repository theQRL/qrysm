package dilithium

import (
	"github.com/theQRL/qrysm/crypto/dilithium/common"
	"github.com/theQRL/qrysm/crypto/dilithium/dilithiumt"
)

// SecretKeyFromBytes creates a Dilithium private key from a seed.
func SecretKeyFromSeed(seed []byte) (DilithiumKey, error) {
	return dilithiumt.SecretKeyFromSeed(seed)
}

// PublicKeyFromBytes creates a Dilithium public key from a byte slice.
func PublicKeyFromBytes(pubKey []byte) (PublicKey, error) {
	return dilithiumt.PublicKeyFromBytes(pubKey)
}

// SignatureFromBytes creates a Dilithium signature from a byte slice.
func SignatureFromBytes(sig []byte) (Signature, error) {
	return dilithiumt.SignatureFromBytes(sig)
}

// VerifySignature verifies a single signature. For performance reason, always use VerifyMultipleSignatures if possible.
func VerifySignature(sig []byte, msg [32]byte, pubKey common.PublicKey) (bool, error) {
	return dilithiumt.VerifySignature(sig, msg, pubKey)
}

// VerifyMultipleSignatures verifies multiple signatures for distinct messages securely.
func VerifyMultipleSignatures(sigs [][][]byte, msgs [][32]byte, pubKeys [][]common.PublicKey) (bool, error) {
	return dilithiumt.VerifyMultipleSignatures(sigs, msgs, pubKeys)
}

// RandKey creates a new private key using a random input.
func RandKey() (common.SecretKey, error) {
	return dilithiumt.RandKey()
}
