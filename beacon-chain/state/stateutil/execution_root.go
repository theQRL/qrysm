package stateutil

import (
	"bytes"
	"encoding/binary"

	"github.com/pkg/errors"
	params "github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/encoding/ssz"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// ExecutionDataRootWithHasher returns the hash tree root of input `executionData`.
func ExecutionDataRootWithHasher(executionData *qrysmpb.ExecutionData) ([32]byte, error) {
	if executionData == nil {
		return [32]byte{}, errors.New("nil execution data")
	}

	fieldRoots := make([][32]byte, 3)
	for i := range fieldRoots {
		fieldRoots[i] = [32]byte{}
	}

	if len(executionData.DepositRoot) > 0 {
		fieldRoots[0] = bytesutil.ToBytes32(executionData.DepositRoot)
	}

	executionDataCountBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(executionDataCountBuf, executionData.DepositCount)
	fieldRoots[1] = bytesutil.ToBytes32(executionDataCountBuf)
	if len(executionData.BlockHash) > 0 {
		fieldRoots[2] = bytesutil.ToBytes32(executionData.BlockHash)
	}
	root, err := ssz.BitwiseMerkleize(fieldRoots, uint64(len(fieldRoots)), uint64(len(fieldRoots)))
	if err != nil {
		return [32]byte{}, err
	}
	return root, nil
}

// ExecutionDatasRoot returns the hash tree root of input `executionDatas`.
func ExecutionDatasRoot(executionDatas []*qrysmpb.ExecutionData) ([32]byte, error) {
	executionVotesRoots := make([][32]byte, 0, len(executionDatas))
	for _, executionData := range executionDatas {
		execution, err := ExecutionDataRootWithHasher(executionData)
		if err != nil {
			return [32]byte{}, errors.Wrap(err, "could not compute executiondata merkleization")
		}
		executionVotesRoots = append(executionVotesRoots, execution)
	}

	executionVotesRootsRoot, err := ssz.BitwiseMerkleize(executionVotesRoots, uint64(len(executionVotesRoots)), params.BeaconConfig().ExecutionDataVotesLength())
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not compute executiondata votes merkleization")
	}
	executionVotesRootBuf := new(bytes.Buffer)
	if err := binary.Write(executionVotesRootBuf, binary.LittleEndian, uint64(len(executionDatas))); err != nil {
		return [32]byte{}, errors.Wrap(err, "could not marshal executiondata votes length")
	}
	// We need to mix in the length of the slice.
	executionVotesRootBufRoot := make([]byte, 32)
	copy(executionVotesRootBufRoot, executionVotesRootBuf.Bytes())
	root := ssz.MixInLength(executionVotesRootsRoot, executionVotesRootBufRoot)

	return root, nil
}
