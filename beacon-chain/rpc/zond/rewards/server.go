package rewards

import (
	"github.com/theQRL/qrysm/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
)

type Server struct {
	Blocker               lookup.Blocker
	OptimisticModeFetcher blockchain.OptimisticModeFetcher
	FinalizationFetcher   blockchain.FinalizationFetcher
	ReplayerBuilder       stategen.ReplayerBuilder
	TimeFetcher           blockchain.TimeFetcher
	Stater                lookup.Stater
	HeadFetcher           blockchain.HeadFetcher
}
