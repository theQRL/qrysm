package v1

import (
	"bytes"
	"math/bits"
)

const (
	NextSyncCommitteeIndex = uint64(55)
	FinalizedRootIndex     = uint64(105)
)

func (x *SyncCommittee) Equals(other *SyncCommittee) bool {
	if len(x.Pubkeys) != len(other.Pubkeys) {
		return false
	}
	for i := range x.Pubkeys {
		if !bytes.Equal(x.Pubkeys[i], other.Pubkeys[i]) {
			return false
		}
	}
	return true
}

// FloorLog2 returns the floor of log2(x), or -1 when x is zero.
func FloorLog2(x uint64) int {
	return bits.Len64(x) - 1
}

func isEmptyWithLength(bb [][]byte, length uint64) bool {
	if len(bb) == 0 {
		return true
	}
	l := FloorLog2(length)
	if len(bb) != l {
		return false
	}
	var zeroRoot [32]byte
	for _, b := range bb {
		if len(b) != 0 && !bytes.Equal(b, zeroRoot[:]) {
			return false
		}
	}
	return true
}

func (x *LightClientUpdate) IsSyncCommiteeUpdate() bool {
	return !isEmptyWithLength(x.GetNextSyncCommitteeBranch(), NextSyncCommitteeIndex)
}

func (x *LightClientUpdate) IsFinalityUpdate() bool {
	return !isEmptyWithLength(x.GetFinalityBranch(), FinalizedRootIndex)
}
