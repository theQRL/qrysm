package ml_dsa_87t

import (
	"errors"
	"fmt"

	cryptoerrors "github.com/theQRL/go-qrllib/crypto/errors"
	cryptomldsa87 "github.com/theQRL/go-qrllib/crypto/ml_dsa_87"
	"github.com/theQRL/go-qrllib/wallet/ml_dsa_87"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/common"
)

// ErrWeakPublicKey is returned for keys rejected by go-qrllib's ML-DSA-87
// key validation. It shares the upstream sentinel so errors.Is works across
// both packages.
var ErrWeakPublicKey = cryptoerrors.ErrWeakPublicKey

// ErrZeroT1PublicKey is retained as an alias for callers using the former
// name. Key validation also rejects weak keys with nonzero t1 coefficients.
var ErrZeroT1PublicKey = ErrWeakPublicKey

type PublicKey struct {
	p *ml_dsa_87.PK
}

// Marshal returns the public key bytes, or nil for a nil or uninitialized key
// so that callers never dereference a missing value.
func (p *PublicKey) Marshal() []byte {
	if p == nil || p.p == nil {
		return nil
	}
	return p.p[:]
}

// PublicKeyFromBytes returns a public key that owns its bytes. Parsing checks
// the length, runs go-qrllib's weak-key validation, and copies the bytes.
// Keys must not be retained globally here, since callers may supply keys
// from deposits that later fail validation.
func PublicKeyFromBytes(pubKey []byte) (common.PublicKey, error) {
	if len(pubKey) != field_params.MLDSA87PubkeyLength {
		return nil, fmt.Errorf("public key must be %d bytes", field_params.MLDSA87PubkeyLength)
	}
	var p ml_dsa_87.PK
	copy(p[:], pubKey)
	if err := cryptomldsa87.ValidatePublicKey((*[cryptomldsa87.CRYPTO_PUBLIC_KEY_BYTES]uint8)(&p)); err != nil {
		if errors.Is(err, cryptoerrors.ErrWeakPublicKey) {
			return nil, ErrWeakPublicKey
		}
		return nil, fmt.Errorf("invalid public key: %w", err)
	}
	return &PublicKey{p: &p}, nil
}

func (p *PublicKey) Copy() common.PublicKey {
	if p == nil || p.p == nil {
		return nil
	}
	np := *p.p
	return &PublicKey{p: &np}
}

// Equals reports whether p2 is an initialized ML-DSA-87 public key with the
// same bytes. A nil, uninitialized, or foreign implementation never compares
// equal, so callers cannot be tricked into deduplicating against it.
func (p *PublicKey) Equals(p2 common.PublicKey) bool {
	other, ok := p2.(*PublicKey)
	if !ok || p == nil || other == nil || p.p == nil || other.p == nil {
		return false
	}
	return *p.p == *other.p
}
