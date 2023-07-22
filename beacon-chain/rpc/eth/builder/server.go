package builder

import (
	"github.com/cyyber/qrysm/v4/beacon-chain/blockchain"
	"github.com/cyyber/qrysm/v4/beacon-chain/rpc/lookup"
)

type Server struct {
	FinalizationFetcher   blockchain.FinalizationFetcher
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	Stater                lookup.Stater
}
