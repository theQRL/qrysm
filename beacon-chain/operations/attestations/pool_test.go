package attestations

import (
	"github.com/theQRL/qrysm/v4/beacon-chain/operations/attestations/kv"
)

var _ Pool = (*kv.AttCaches)(nil)
