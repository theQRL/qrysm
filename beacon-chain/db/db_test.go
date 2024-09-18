package db

import "github.com/theQRL/qrysm/beacon-chain/db/kv"

var _ Database = (*kv.Store)(nil)
