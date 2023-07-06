package validator

import (
	"github.com/cyyber/qrysm/v4/beacon-chain/blockchain"
	"github.com/cyyber/qrysm/v4/beacon-chain/cache"
	"github.com/cyyber/qrysm/v4/beacon-chain/db"
	"github.com/cyyber/qrysm/v4/beacon-chain/operations/attestations"
	"github.com/cyyber/qrysm/v4/beacon-chain/operations/synccommittee"
	"github.com/cyyber/qrysm/v4/beacon-chain/p2p"
	"github.com/cyyber/qrysm/v4/beacon-chain/rpc/lookup"
	v1alpha1validator "github.com/cyyber/qrysm/v4/beacon-chain/rpc/prysm/v1alpha1/validator"
	"github.com/cyyber/qrysm/v4/beacon-chain/sync"
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
	V1Alpha1Server         *v1alpha1validator.Server
	ProposerSlotIndexCache *cache.ProposerPayloadIDsCache
	ChainInfoFetcher       blockchain.ChainInfoFetcher
	BeaconDB               db.ReadOnlyDatabase
}
