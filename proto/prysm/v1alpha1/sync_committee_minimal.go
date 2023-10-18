//go:build minimal

package zond

import (
	"github.com/prysmaticlabs/go-bitfield"
)

func NewSyncCommitteeAggregationBits() bitfield.Bitvector8 {
	return bitfield.NewBitvector8()
}

func ConvertToSyncContributionBitVector(b []byte) bitfield.Bitvector8 {
	return b
}
