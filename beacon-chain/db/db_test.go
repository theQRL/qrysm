package db

import "github.com/cyyber/qrysm/v4/beacon-chain/db/kv"

var _ Database = (*kv.Store)(nil)
