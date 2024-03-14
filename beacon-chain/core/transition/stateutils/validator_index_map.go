// Package stateutils contains useful tools for faster computation
// of state transitions using maps to represent validators instead
// of slices.
package stateutils

import (
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

// ValidatorIndexMap builds a lookup map for quickly determining the index of
// a validator by their public key.
func ValidatorIndexMap(validators []*zondpb.Validator) map[[field_params.DilithiumPubkeyLength]byte]primitives.ValidatorIndex {
	m := make(map[[field_params.DilithiumPubkeyLength]byte]primitives.ValidatorIndex, len(validators))
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
