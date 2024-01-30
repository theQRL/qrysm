//go:build minimal

package zond

import (
	"github.com/theQRL/go-bitfield"
)

func NewSyncCommitteeAggregationBits() bitfield.Bitvector16 {
	return bitfield.NewBitvector16()
}

func ConvertToSyncContributionBitVector(b []byte) bitfield.Bitvector16 {
	return b
}
