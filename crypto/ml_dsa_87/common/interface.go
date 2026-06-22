// Package common provides the ML-DSA-87 interfaces that are implemented by the various ML-DSA-87 wrappers.
//
// This package should not be used by downstream consumers. These interfaces are re-exporter by
// github.com/theQRL/qrysm/crypto/ml_dsa_87. This package exists to prevent an import circular
// dependency.
package common

// SecretKey represents a ML-DSA-87 secret or private key.
type SecretKey interface {
	PublicKey() PublicKey
	Sign(msg []byte) Signature
	Marshal() []byte
}

// PublicKey represents a ML-DSA-87 public key.
type PublicKey interface {
	Marshal() []byte
	Copy() PublicKey
	Equals(p2 PublicKey) bool
}

// Signature represents a ML-DSA-87 signature.
type Signature interface {
	Verify(pubKey PublicKey, msg []byte) bool
	Marshal() []byte
	Copy() Signature
}
