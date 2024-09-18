// Package beacon defines a gRPC beacon service implementation, providing
// useful endpoints for checking fetching chain-specific data such as
// blocks, committees, validators, assignments, and more.
package beacon

import (
	"context"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	blockfeed "github.com/theQRL/qrysm/beacon-chain/core/feed/block"
	"github.com/theQRL/qrysm/beacon-chain/core/feed/operation"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/p2p"
	"github.com/theQRL/qrysm/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/beacon-chain/sync"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// Server defines a server implementation of the gRPC Beacon Chain service,
// providing RPC endpoints to access data relevant to the Zond beacon chain.
type Server struct {
	BeaconDB                    db.ReadOnlyDatabase
	Ctx                         context.Context
	ChainStartFetcher           execution.ChainStartFetcher
	HeadFetcher                 blockchain.HeadFetcher
	CanonicalFetcher            blockchain.CanonicalFetcher
	FinalizationFetcher         blockchain.FinalizationFetcher
	DepositFetcher              cache.DepositFetcher
	BlockFetcher                execution.POWBlockFetcher
	GenesisTimeFetcher          blockchain.TimeFetcher
	StateNotifier               statefeed.Notifier
	BlockNotifier               blockfeed.Notifier
	AttestationNotifier         operation.Notifier
	Broadcaster                 p2p.Broadcaster
	AttestationsPool            attestations.Pool
	SlashingsPool               slashings.PoolManager
	ChainStartChan              chan time.Time
	ReceivedAttestationsBuffer  chan *zondpb.Attestation
	CollectedAttestationsBuffer chan []*zondpb.Attestation
	StateGen                    stategen.StateManager
	SyncChecker                 sync.Checker
	ReplayerBuilder             stategen.ReplayerBuilder
	OptimisticModeFetcher       blockchain.OptimisticModeFetcher
	CoreService                 *core.Service
}
