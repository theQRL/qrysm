package validator

import (
	"github.com/theQRL/qrysm/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/beacon-chain/builder"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/core/feed/operation"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/beacon-chain/operations/synccommittee"
	"github.com/theQRL/qrysm/beacon-chain/p2p"
	"github.com/theQRL/qrysm/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/beacon-chain/sync"
	zond "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
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
