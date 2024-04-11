package validator

import (
	"github.com/theQRL/qrysm/v4/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/v4/beacon-chain/db"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/v4/beacon-chain/sync"
)

// Server defines a server implementation for HTTP endpoints, providing
// access data relevant to the Zond Beacon Chain.
type Server struct {
	GenesisTimeFetcher    blockchain.TimeFetcher
	SyncChecker           sync.Checker
	HeadFetcher           blockchain.HeadFetcher
	CoreService           *core.Service
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	Stater                lookup.Stater
	ChainInfoFetcher      blockchain.ChainInfoFetcher
	BeaconDB              db.ReadOnlyDatabase
	FinalizationFetcher   blockchain.FinalizationFetcher
}
