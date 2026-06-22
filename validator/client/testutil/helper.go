package testutil

import (
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// ActiveKey represents a public key whose status is ACTIVE.
var ActiveKey = bytesutil.ToBytes2592([]byte("active"))

// GenerateMultipleValidatorStatusResponse prepares a response from the passed in keys.
func GenerateMultipleValidatorStatusResponse(pubkeys [][]byte) *qrysmpb.MultipleValidatorStatusResponse {
	resp := &qrysmpb.MultipleValidatorStatusResponse{
		PublicKeys: make([][]byte, len(pubkeys)),
		Statuses:   make([]*qrysmpb.ValidatorStatusResponse, len(pubkeys)),
		Indices:    make([]primitives.ValidatorIndex, len(pubkeys)),
	}
	for i, key := range pubkeys {
		resp.PublicKeys[i] = key
		resp.Statuses[i] = &qrysmpb.ValidatorStatusResponse{
			Status: qrysmpb.ValidatorStatus_UNKNOWN_STATUS,
		}
		resp.Indices[i] = primitives.ValidatorIndex(i)
	}

	return resp
}
