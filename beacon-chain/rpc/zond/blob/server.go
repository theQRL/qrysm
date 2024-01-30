package blob

import (
	"github.com/theQRL/qrysm/v4/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/v4/beacon-chain/db"
)

type Server struct {
	ChainInfoFetcher blockchain.ChainInfoFetcher
	BeaconDB         db.ReadOnlyDatabase
}
