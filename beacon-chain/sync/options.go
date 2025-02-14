package sync

import (
	"github.com/theQRL/qrysm/async/event"
	blockfeed "github.com/theQRL/qrysm/beacon-chain/core/feed/block"
	"github.com/theQRL/qrysm/beacon-chain/core/feed/operation"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/beacon-chain/operations/dilithiumtoexec"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/operations/synccommittee"
	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	"github.com/theQRL/qrysm/beacon-chain/p2p"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
)

type Option func(s *Service) error

func WithAttestationNotifier(notifier operation.Notifier) Option {
	return func(s *Service) error {
		s.cfg.attestationNotifier = notifier
		return nil
	}
}

func WithP2P(p2p p2p.P2P) Option {
	return func(s *Service) error {
		s.cfg.p2p = p2p
		return nil
	}
}

func WithDatabase(db db.NoHeadAccessDatabase) Option {
	return func(s *Service) error {
		s.cfg.beaconDB = db
		return nil
	}
}

func WithAttestationPool(attPool attestations.Pool) Option {
	return func(s *Service) error {
		s.cfg.attPool = attPool
		return nil
	}
}

func WithExitPool(exitPool voluntaryexits.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.exitPool = exitPool
		return nil
	}
}

func WithSlashingPool(slashingPool slashings.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.slashingPool = slashingPool
		return nil
	}
}

func WithSyncCommsPool(syncCommsPool synccommittee.Pool) Option {
	return func(s *Service) error {
		s.cfg.syncCommsPool = syncCommsPool
		return nil
	}
}

func WithDilithiumToExecPool(dilithiumToExecPool dilithiumtoexec.PoolManager) Option {
	return func(s *Service) error {
		s.cfg.dilithiumToExecPool = dilithiumToExecPool
		return nil
	}
}

func WithChainService(chain blockchainService) Option {
	return func(s *Service) error {
		s.cfg.chain = chain
		return nil
	}
}

func WithInitialSync(initialSync Checker) Option {
	return func(s *Service) error {
		s.cfg.initialSync = initialSync
		return nil
	}
}

func WithBlockNotifier(blockNotifier blockfeed.Notifier) Option {
	return func(s *Service) error {
		s.cfg.blockNotifier = blockNotifier
		return nil
	}
}

func WithOperationNotifier(operationNotifier operation.Notifier) Option {
	return func(s *Service) error {
		s.cfg.operationNotifier = operationNotifier
		return nil
	}
}

func WithStateGen(stateGen *stategen.State) Option {
	return func(s *Service) error {
		s.cfg.stateGen = stateGen
		return nil
	}
}

func WithSlasherAttestationsFeed(slasherAttestationsFeed *event.Feed) Option {
	return func(s *Service) error {
		s.cfg.slasherAttestationsFeed = slasherAttestationsFeed
		return nil
	}
}

func WithSlasherBlockHeadersFeed(slasherBlockHeadersFeed *event.Feed) Option {
	return func(s *Service) error {
		s.cfg.slasherBlockHeadersFeed = slasherBlockHeadersFeed
		return nil
	}
}

func WithExecutionPayloadReconstructor(r execution.ExecutionPayloadReconstructor) Option {
	return func(s *Service) error {
		s.cfg.executionPayloadReconstructor = r
		return nil
	}
}

func WithClockWaiter(cw startup.ClockWaiter) Option {
	return func(s *Service) error {
		s.clockWaiter = cw
		return nil
	}
}

func WithInitialSyncComplete(c chan struct{}) Option {
	return func(s *Service) error {
		s.initialSyncComplete = c
		return nil
	}
}
