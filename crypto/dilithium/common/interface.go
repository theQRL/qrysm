// Package common provides the Dilithium interfaces that are implemented by the various Dilithium wrappers.
//
// This package should not be used by downstream consumers. These interfaces are re-exporter by
// github.com/theQRL/qrysm/crypto/dilithium. This package exists to prevent an import circular
// dependency.
package common

// SecretKey represents a Dilithium secret or private key.
type SecretKey interface {
	PublicKey() PublicKey
	Sign(msg []byte) Signature
	Marshal() []byte
}

// PublicKey represents a Dilithium public key.
type PublicKey interface {
	Marshal() []byte
	Copy() PublicKey
	Equals(p2 PublicKey) bool
}

// Signature represents a Dilithium signature.
type Signature interface {
	Verify(pubKey PublicKey, msg []byte) bool
	Marshal() []byte
	Copy() Signature
}
