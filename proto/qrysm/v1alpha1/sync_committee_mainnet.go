//go:build !minimal

package qrl

import (
	"github.com/theQRL/go-bitfield"
)

func NewSyncCommitteeAggregationBits() bitfield.Bitvector128 {
	return bitfield.NewBitvector128()
}

func ConvertToSyncContributionBitVector(b []byte) bitfield.Bitvector128 {
	return b
}
