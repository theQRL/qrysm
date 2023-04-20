// Package stateutils contains useful tools for faster computation
// of state transitions using maps to represent validators instead
// of slices.
package stateutils

import (
	"github.com/prysmaticlabs/prysm/v4/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v4/encoding/bytesutil"
	ethpb "github.com/prysmaticlabs/prysm/v4/proto/prysm/v1alpha1"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
)

// ValidatorIndexMap builds a lookup map for quickly determining the index of
// a validator by their public key.
func ValidatorIndexMap(validators []*ethpb.Validator) map[[dilithium2.CryptoPublicKeyBytes]byte]primitives.ValidatorIndex {
	m := make(map[[dilithium2.CryptoPublicKeyBytes]byte]primitives.ValidatorIndex, len(validators))
	if validators == nil {
		return m
	}
	for idx, record := range validators {
		if record == nil {
			continue
		}
		key := bytesutil.ToBytes2592(record.PublicKey)
		m[key] = primitives.ValidatorIndex(idx)
	}
	return m
}
