package validator

import (
	"github.com/theQRL/qrysm/v4/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/v4/beacon-chain/builder"
	"github.com/theQRL/qrysm/v4/beacon-chain/cache"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/feed/operation"
	"github.com/theQRL/qrysm/v4/beacon-chain/db"
	"github.com/theQRL/qrysm/v4/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/v4/beacon-chain/operations/synccommittee"
	"github.com/theQRL/qrysm/v4/beacon-chain/p2p"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/v4/beacon-chain/sync"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

// Server defines a server implementation of the gRPC Validator service,
// providing RPC endpoints intended for validator clients.
type Server struct {
	HeadFetcher            blockchain.HeadFetcher
	TimeFetcher            blockchain.TimeFetcher
	SyncChecker            sync.Checker
	AttestationsPool       attestations.Pool
	PeerManager            p2p.PeerManager
	Broadcaster            p2p.Broadcaster
	Stater                 lookup.Stater
	OptimisticModeFetcher  blockchain.OptimisticModeFetcher
	SyncCommitteePool      synccommittee.Pool
	V1Alpha1Server         zond.BeaconNodeValidatorServer
	ProposerSlotIndexCache *cache.ProposerPayloadIDsCache
	ChainInfoFetcher       blockchain.ChainInfoFetcher
	BeaconDB               db.HeadAccessDatabase
	BlockBuilder           builder.BlockBuilder
	OperationNotifier      operation.Notifier
	CoreService            *core.Service
}
