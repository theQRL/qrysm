package attestations

import (
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations/kv"
)

var _ Pool = (*kv.AttCaches)(nil)
