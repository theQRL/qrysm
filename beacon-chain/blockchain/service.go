// Package blockchain defines the life-cycle of the blockchain at the core of
// Ethereum, including processing of new blocks and attestations using proof of stake.
package blockchain

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/async/event"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	f "github.com/theQRL/qrysm/beacon-chain/forkchoice"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	"github.com/theQRL/qrysm/beacon-chain/p2p"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	qrysmTime "github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/time/slots"
)

// Service represents a service that handles the internal
// logic of managing the full PoS beacon chain.
type Service struct {
	cfg                  *config
	ctx                  context.Context
	cancel               context.CancelFunc
	genesisTime          time.Time
	head                 *head
	headLock             sync.RWMutex
	originBlockRoot      [32]byte // genesis root, or weak subjectivity checkpoint root, depending on how the node is initialized
	boundaryRoots        [][32]byte
	checkpointStateCache *cache.CheckpointStateCache
	initSyncBlocks       map[[32]byte]interfaces.ReadOnlySignedBeaconBlock
	initSyncBlocksLock   sync.RWMutex
	wsVerifier           *WeakSubjectivityVerifier
	clockSetter          startup.ClockSetter
	clockWaiter          startup.ClockWaiter
	syncComplete         chan struct{}
	blockBeingSynced     *currentlySyncingBlock
}

// config options for the service.
type config struct {
	BeaconBlockBuf          int
	ChainStartFetcher       execution.ChainStartFetcher
	BeaconDB                db.HeadAccessDatabase
	DepositCache            cache.DepositCache
	ProposerSlotIndexCache  *cache.ProposerPayloadIDsCache
	AttPool                 attestations.Pool
	ExitPool                voluntaryexits.PoolManager
	SlashingPool            slashings.PoolManager
	P2p                     p2p.Broadcaster
	MaxRoutines             int
	StateNotifier           statefeed.Notifier
	ForkChoiceStore         f.ForkChoicer
	AttService              *attestations.Service
	StateGen                *stategen.State
	SlasherAttestationsFeed *event.Feed
	WeakSubjectivityCheckpt *qrysmpb.Checkpoint
	BlockFetcher            execution.ExecutionBlockFetcher
	FinalizedStateAtStartUp state.BeaconState
	ExecutionEngineCaller   execution.EngineCaller
}

var ErrMissingClockSetter = errors.New("blockchain Service initialized without a startup.ClockSetter")

// NewService instantiates a new block service instance that will
// be registered into a running beacon node.
func NewService(ctx context.Context, opts ...Option) (*Service, error) {
	var err error
	ctx, cancel := context.WithCancel(ctx)
	srv := &Service{
		ctx:                  ctx,
		cancel:               cancel,
		boundaryRoots:        [][32]byte{},
		checkpointStateCache: cache.NewCheckpointStateCache(),
		initSyncBlocks:       make(map[[32]byte]interfaces.ReadOnlySignedBeaconBlock),
		cfg:                  &config{ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache()},
		blockBeingSynced:     &currentlySyncingBlock{roots: make(map[[32]byte]struct{})},
	}
	for _, opt := range opts {
		if err := opt(srv); err != nil {
			return nil, err
		}
	}
	if srv.clockSetter == nil {
		return nil, ErrMissingClockSetter
	}
	srv.wsVerifier, err = NewWeakSubjectivityVerifier(srv.cfg.WeakSubjectivityCheckpt, srv.cfg.BeaconDB)
	if err != nil {
		return nil, err
	}
	return srv, nil
}

// Start a blockchain service's main event loop.
func (s *Service) Start() {
	saved := s.cfg.FinalizedStateAtStartUp
	defer s.removeStartupState()

	if saved != nil && !saved.IsNil() {
		if err := s.StartFromSavedState(saved); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal("Start from execution chain deprecated, please provide a genesis file")
	}
	s.spawnProcessAttestationsRoutine()
	go s.runLateBlockTasks()
}

// Stop the blockchain service's main event loop and associated goroutines.
func (s *Service) Stop() error {
	defer s.cancel()

	// lock before accessing s.head, s.head.state, s.head.state.FinalizedCheckpoint().Root
	s.headLock.RLock()
	if s.cfg.StateGen != nil && s.head != nil && s.head.state != nil {
		r := s.head.state.FinalizedCheckpoint().Root
		s.headLock.RUnlock()
		// Save the last finalized state so that starting up in the following run will be much faster.
		if err := s.cfg.StateGen.ForceCheckpoint(s.ctx, r); err != nil {
			return err
		}
	} else {
		s.headLock.RUnlock()
	}
	// Save initial sync cached blocks to the DB before stop.
	return s.cfg.BeaconDB.SaveBlocks(s.ctx, s.getInitSyncBlocks())
}

// Status always returns nil unless there is an error condition that causes
// this service to be unhealthy.
func (s *Service) Status() error {
	optimistic, err := s.IsOptimistic(s.ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check if service is optimistic")
	}
	if optimistic {
		return errors.New("service is optimistic, and only limited service functionality is provided " +
			"please check if execution layer is fully synced")
	}

	if s.originBlockRoot == params.BeaconConfig().ZeroHash {
		return errors.New("genesis state has not been created")
	}
	if runtime.NumGoroutine() > s.cfg.MaxRoutines {
		return fmt.Errorf("too many goroutines (%d)", runtime.NumGoroutine())
	}
	return nil
}

// StartFromSavedState initializes the blockchain using a previously saved finalized checkpoint.
func (s *Service) StartFromSavedState(saved state.BeaconState) error {
	log.Info("Blockchain data already exists in DB, initializing...")
	s.genesisTime = time.Unix(int64(saved.GenesisTime()), 0) // lint:ignore uintcast -- Genesis time will not exceed int64 in your lifetime.
	s.cfg.AttService.SetGenesisTime(saved.GenesisTime())

	originRoot, err := s.originRootFromSavedState(s.ctx)
	if err != nil {
		return err
	}
	s.originBlockRoot = originRoot
	st, err := s.cfg.StateGen.Resume(s.ctx, s.cfg.FinalizedStateAtStartUp)
	if err != nil {
		return errors.Wrap(err, "could not get finalized state from db")
	}
	spawnCountdownIfPreGenesis(s.ctx, s.genesisTime, s.cfg.BeaconDB)
	if err := s.setupForkchoice(st); err != nil {
		return errors.Wrap(err, "could not set up forkchoice")
	}

	// not attempting to save initial sync blocks here, because there shouldn't be any until
	// after the statefeed.Initialized event is fired (below)
	cp := s.FinalizedCheckpt()
	if err := s.wsVerifier.VerifyWeakSubjectivity(s.ctx, cp.Epoch); err != nil {
		// Exit run time if the node failed to verify weak subjectivity checkpoint.
		return errors.Wrap(err, "could not verify initial checkpoint provided for chain sync")
	}

	vr := bytesutil.ToBytes32(saved.GenesisValidatorsRoot())
	if err := s.clockSetter.SetClock(startup.NewClock(s.genesisTime, vr)); err != nil {
		return errors.Wrap(err, "failed to initialize blockchain service")
	}

	return nil
}

func (s *Service) originRootFromSavedState(ctx context.Context) ([32]byte, error) {
	// first check if we have started from checkpoint sync and have a root
	originRoot, err := s.cfg.BeaconDB.OriginCheckpointBlockRoot(ctx)
	if err == nil {
		return originRoot, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return originRoot, errors.Wrap(err, "could not retrieve checkpoint sync chain origin data from db")
	}

	// we got here because OriginCheckpointBlockRoot gave us an ErrNotFound. this means the node was started from a genesis state,
	// so we should have a value for GenesisBlock
	genesisBlock, err := s.cfg.BeaconDB.GenesisBlock(ctx)
	if err != nil {
		return originRoot, errors.Wrap(err, "could not get genesis block from db")
	}
	if err := blocks.BeaconBlockIsNil(genesisBlock); err != nil {
		return originRoot, err
	}
	genesisBlkRoot, err := genesisBlock.Block().HashTreeRoot()
	if err != nil {
		return genesisBlkRoot, errors.Wrap(err, "could not get signing root of genesis block")
	}
	return genesisBlkRoot, nil
}

// initializeHead uses the finalized checkpoint and head block root from forkchoice to set the current head.
// Note that this may block until stategen replays blocks between the finalized and head blocks
// if the head sync flag was specified and the gap between the finalized and head blocks is at least 128 epochs long.
func (s *Service) initializeHead(ctx context.Context, st state.BeaconState) error {
	cp := s.FinalizedCheckpt()
	fRoot := s.ensureRootNotZeros([32]byte(cp.Root))
	if st == nil || st.IsNil() {
		return errors.New("finalized state can't be nil")
	}

	s.cfg.ForkChoiceStore.RLock()
	root := s.cfg.ForkChoiceStore.HighestReceivedBlockRoot()
	s.cfg.ForkChoiceStore.RUnlock()
	blk, err := s.cfg.BeaconDB.Block(ctx, root)
	if err != nil {
		return errors.Wrap(err, "could not get head block")
	}
	if root != fRoot {
		st, err = s.cfg.StateGen.StateByRoot(ctx, root)
		if err != nil {
			return errors.Wrap(err, "could not get head state")
		}
	}
	if err := s.setHead(&head{root, blk, st, blk.Block().Slot(), false}); err != nil {
		return errors.Wrap(err, "could not set head")
	}
	log.WithFields(logrus.Fields{
		"root": fmt.Sprintf("%#x", root),
		"slot": blk.Block().Slot(),
	}).Info("Initialized head block from DB")
	return nil
}

// This gets called when beacon chain is first initialized to save genesis data (state, block, and more) in db.
func (s *Service) saveGenesisData(ctx context.Context, genesisState state.BeaconState) error {
	if err := s.cfg.BeaconDB.SaveGenesisData(ctx, genesisState); err != nil {
		return errors.Wrap(err, "could not save genesis data")
	}
	genesisBlk, err := s.cfg.BeaconDB.GenesisBlock(ctx)
	if err != nil || genesisBlk == nil || genesisBlk.IsNil() {
		return fmt.Errorf("could not load genesis block: %v", err)
	}
	genesisBlkRoot, err := genesisBlk.Block().HashTreeRoot()
	if err != nil {
		return errors.Wrap(err, "could not get genesis block root")
	}

	s.originBlockRoot = genesisBlkRoot
	s.cfg.StateGen.SaveFinalizedState(0 /*slot*/, genesisBlkRoot, genesisState)

	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	gb, err := blocks.NewROBlockWithRoot(genesisBlk, genesisBlkRoot)
	if err != nil {
		return err
	}
	if err := s.cfg.ForkChoiceStore.InsertNode(ctx, genesisState, gb); err != nil {
		log.WithError(err).Fatal("Could not process genesis block for fork choice")
	}
	s.cfg.ForkChoiceStore.SetOriginRoot(genesisBlkRoot)
	// Set genesis as fully validated
	if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(ctx, genesisBlkRoot); err != nil {
		return errors.Wrap(err, "Could not set optimistic status of genesis block to false")
	}
	s.cfg.ForkChoiceStore.SetGenesisTime(uint64(s.genesisTime.Unix()))

	if err := s.setHead(&head{
		genesisBlkRoot,
		genesisBlk,
		genesisState,
		genesisBlk.Block().Slot(),
		false,
	}); err != nil {
		log.WithError(err).Fatal("Could not set head")
	}
	return nil
}

// This returns true if block has been processed before. Two ways to verify the block has been processed:
// 1.) Check fork choice store.
// 2.) Check DB.
// Checking 1.) is ten times faster than checking 2.)
// this function requires a lock in forkchoice
func (s *Service) hasBlock(ctx context.Context, root [32]byte) bool {
	if s.cfg.ForkChoiceStore.HasNode(root) {
		return true
	}

	return s.cfg.BeaconDB.HasBlock(ctx, root)
}

// removeStartupState releases the (potentially multi-MB) finalized-state
// reference once the service has finished consuming it during startup, so
// it can be garbage-collected instead of living for the process lifetime.
func (s *Service) removeStartupState() {
	s.cfg.FinalizedStateAtStartUp = nil
}

func spawnCountdownIfPreGenesis(ctx context.Context, genesisTime time.Time, db db.HeadAccessDatabase) {
	currentTime := qrysmTime.Now()
	if currentTime.After(genesisTime) {
		return
	}

	gState, err := db.GenesisState(ctx)
	if err != nil {
		log.WithError(err).Fatal("Could not retrieve genesis state")
	}
	gRoot, err := gState.HashTreeRoot(ctx)
	if err != nil {
		log.WithError(err).Fatal("Could not hash tree root genesis state")
	}
	go slots.CountdownToGenesis(ctx, genesisTime, uint64(gState.NumValidators()), gRoot)
}
