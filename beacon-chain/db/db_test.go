package db

import "github.com/theQRL/qrysm/v4/beacon-chain/db/kv"

var _ Database = (*kv.Store)(nil)
