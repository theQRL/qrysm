package testutil

import (
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

// ActiveKey represents a public key whose status is ACTIVE.
var ActiveKey = bytesutil.ToBytes2592([]byte("active"))

// GenerateMultipleValidatorStatusResponse prepares a response from the passed in keys.
func GenerateMultipleValidatorStatusResponse(pubkeys [][]byte) *zondpb.MultipleValidatorStatusResponse {
	resp := &zondpb.MultipleValidatorStatusResponse{
		PublicKeys: make([][]byte, len(pubkeys)),
		Statuses:   make([]*zondpb.ValidatorStatusResponse, len(pubkeys)),
		Indices:    make([]primitives.ValidatorIndex, len(pubkeys)),
	}
	for i, key := range pubkeys {
		resp.PublicKeys[i] = key
		resp.Statuses[i] = &zondpb.ValidatorStatusResponse{
			Status: zondpb.ValidatorStatus_UNKNOWN_STATUS,
		}
		resp.Indices[i] = primitives.ValidatorIndex(i)
	}

	return resp
}
