package core

import (
	"github.com/theQRL/qrysm/v4/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/v4/beacon-chain/cache"
	opfeed "github.com/theQRL/qrysm/v4/beacon-chain/core/feed/operation"
	"github.com/theQRL/qrysm/v4/beacon-chain/operations/synccommittee"
	"github.com/theQRL/qrysm/v4/beacon-chain/p2p"
	"github.com/theQRL/qrysm/v4/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/v4/beacon-chain/sync"
)

type Service struct {
	HeadFetcher        blockchain.HeadFetcher
	GenesisTimeFetcher blockchain.TimeFetcher
	SyncChecker        sync.Checker
	Broadcaster        p2p.Broadcaster
	SyncCommitteePool  synccommittee.Pool
	OperationNotifier  opfeed.Notifier
	AttestationCache   *cache.AttestationCache
	StateGen           stategen.StateManager
	P2P                p2p.Broadcaster
}
