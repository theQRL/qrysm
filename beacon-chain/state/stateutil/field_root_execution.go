package stateutil

import (
	"github.com/pkg/errors"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// ExecutionRoot computes the HashTreeRoot Merkleization of
// a BeaconBlockHeader struct according to the eth2
// Simple Serialize specification.
func ExecutionRoot(executionData *qrysmpb.ExecutionData) ([32]byte, error) {
	if executionData == nil {
		return [32]byte{}, errors.New("nil execution data")
	}
	return ExecutionDataRootWithHasher(executionData)
}

// ExecutionDataVotesRoot computes the HashTreeRoot Merkleization of
// a list of ExecutionData structs according to the eth2
// Simple Serialize specification.
func ExecutionDataVotesRoot(executionDataVotes []*qrysmpb.ExecutionData) ([32]byte, error) {
	return ExecutionDatasRoot(executionDataVotes)
}
