package builder

import (
	"github.com/theQRL/qrysm/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/beacon-chain/rpc/lookup"
)

type Server struct {
	FinalizationFetcher   blockchain.FinalizationFetcher
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	Stater                lookup.Stater
}
