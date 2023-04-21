package attestations

import (
	"github.com/cyyber/qrysm/v4/beacon-chain/operations/attestations/kv"
)

var _ Pool = (*kv.AttCaches)(nil)
