// Package slasher implements slashing detection for qrl, able to catch slashable attestations
// and proposals that it receives via two event feeds, respectively. Any found slashings
// are then submitted to the beacon node's slashing operations pool. See the design document
// here https://hackmd.io/@prysmaticlabs/slasher.
package slasher

import (
	"context"
	"sync"
	"time"

	"github.com/theQRL/qrysm/async/event"
	"github.com/theQRL/qrysm/beacon-chain/blockchain"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/db/slasherkv"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	beaconChainSync "github.com/theQRL/qrysm/beacon-chain/sync"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
)

const (
	shutdownTimeout = time.Minute * 5
)

// ServiceConfig for the slasher service in the beacon node.
// This struct allows us to specify required dependencies and
// parameters for slasher to function as needed.
type ServiceConfig struct {
	IndexedAttestationsFeed *event.Feed
	BeaconBlockHeadersFeed  *event.Feed
	Database                db.SlasherDatabase
	StateNotifier           statefeed.Notifier
	AttestationStateFetcher blockchain.AttestationStateFetcher
	StateGen                stategen.StateManager
	SlashingPoolInserter    slashings.PoolInserter
	HeadStateFetcher        blockchain.HeadFetcher
	SyncChecker             beaconChainSync.Checker
	ClockWaiter             startup.ClockWaiter
}

// Service defining a slasher implementation as part of
// the beacon node, able to detect consensus slashable offenses.
type Service struct {
	params                         *Parameters
	serviceCfg                     *ServiceConfig
	indexedAttsChan                chan *qrysmpb.IndexedAttestation
	beaconBlockHeadersChan         chan *qrysmpb.SignedBeaconBlockHeader
	attsQueue                      *attestationsQueue
	blksQueue                      *blocksQueue
	ctx                            context.Context
	cancel                         context.CancelFunc
	genesisTime                    time.Time
	attsSlotTicker                 *slots.SlotTicker
	blocksSlotTicker               *slots.SlotTicker
	pruningSlotTicker              *slots.SlotTicker
	latestEpochWrittenForValidator map[primitives.ValidatorIndex]primitives.Epoch
	// lastEpochWrittenPersisted is the epoch at which latestEpochWrittenForValidator
	// was last flushed to disk by the attestation processing routine.
	lastEpochWrittenPersisted primitives.Epoch
	// maxChunksPerFlush bounds the span chunks saved in one call, which the
	// store commits in one transaction with their epoch markers.
	maxChunksPerFlush int
	// attestationRetries counts, per signing root, how often an attestation
	// whose spans could not be saved was queued again.
	attestationRetries map[[32]byte]uint8
	// proposalRetries counts, per header signing root, how often a conflicting
	// proposal whose signature could not be checked was queued again.
	proposalRetries map[[32]byte]uint8
	wg              sync.WaitGroup
}

// New instantiates a new slasher from configuration values.
func New(ctx context.Context, srvCfg *ServiceConfig) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)
	return &Service{
		params:                         DefaultParams(),
		serviceCfg:                     srvCfg,
		indexedAttsChan:                make(chan *qrysmpb.IndexedAttestation, 1),
		beaconBlockHeadersChan:         make(chan *qrysmpb.SignedBeaconBlockHeader, 1),
		attsQueue:                      newAttestationsQueue(),
		blksQueue:                      newBlocksQueue(),
		ctx:                            ctx,
		cancel:                         cancel,
		latestEpochWrittenForValidator: make(map[primitives.ValidatorIndex]primitives.Epoch),
		maxChunksPerFlush:              slasherkv.ChunkSaveBatchSize,
	}, nil
}

// Start listening for received indexed attestations and blocks
// and perform slashing detection on them.
func (s *Service) Start() {
	// run holds a count on the wait group for its whole life, so that Stop
	// waits for it as well: the worker goroutines are added to the wait group
	// from inside run, and a Stop that ran before them would otherwise return
	// while run still starts workers and writes the service's maps.
	s.wg.Add(1)
	go func() { // Start functions must be non-blocking.
		defer s.wg.Done()
		s.run()
	}()
}

func (s *Service) run() {
	if err := s.waitForChainInitialization(); err != nil {
		// A shutdown before the chain started; there is nothing to run.
		return
	}
	s.waitForSync(s.genesisTime)
	if s.ctx.Err() != nil {
		return
	}

	log.Info("Completed chain sync, starting slashing detection")

	// Get the latest epoch written for each validator from disk on startup.
	headState, err := s.serviceCfg.HeadStateFetcher.HeadState(s.ctx)
	if err != nil {
		log.WithError(err).Error("Failed to fetch head state")
		return
	}
	numVals := headState.NumValidators()
	validatorIndices := make([]primitives.ValidatorIndex, numVals)
	for i := range numVals {
		validatorIndices[i] = primitives.ValidatorIndex(i)
	}
	start := time.Now()
	log.Info("Reading last epoch written for each validator...")
	epochsByValidator, err := s.serviceCfg.Database.LastEpochWrittenForValidators(
		s.ctx, validatorIndices,
	)
	if err != nil {
		log.Error(err)
		return
	}
	for _, item := range epochsByValidator {
		s.latestEpochWrittenForValidator[item.ValidatorIndex] = item.Epoch
	}
	log.WithField("elapsed", time.Since(start)).Info(
		"Finished retrieving last epoch written per validator",
	)

	indexedAttsChan := make(chan *qrysmpb.IndexedAttestation, 1)
	beaconBlockHeadersChan := make(chan *qrysmpb.SignedBeaconBlockHeader, 1)

	s.wg.Add(1)
	go s.receiveAttestations(s.ctx, indexedAttsChan)

	s.wg.Add(1)
	go s.receiveBlocks(s.ctx, beaconBlockHeadersChan)

	secondsPerSlot := params.BeaconConfig().SecondsPerSlot
	s.attsSlotTicker = slots.NewSlotTicker(s.genesisTime, secondsPerSlot)
	s.blocksSlotTicker = slots.NewSlotTicker(s.genesisTime, secondsPerSlot)
	s.pruningSlotTicker = slots.NewSlotTicker(s.genesisTime, secondsPerSlot)

	s.wg.Add(1)
	go s.processQueuedAttestations(s.ctx, s.attsSlotTicker.C())

	s.wg.Add(1)
	go s.processQueuedBlocks(s.ctx, s.blocksSlotTicker.C())

	s.wg.Add(1)
	go s.pruneSlasherData(s.ctx, s.pruningSlotTicker.C())
}

// Stop the slasher service.
func (s *Service) Stop() error {
	s.cancel()
	s.wg.Wait()

	if s.attsSlotTicker != nil {
		s.attsSlotTicker.Done()
	}
	if s.blocksSlotTicker != nil {
		s.blocksSlotTicker.Done()
	}
	if s.pruningSlotTicker != nil {
		s.pruningSlotTicker.Done()
	}
	// Flush the latest epoch written map to disk.
	start := time.Now()
	// New context as the service context has already been canceled.
	ctx, innerCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer innerCancel()
	log.Info("Flushing last epoch written for each validator to disk, please wait")
	if err := s.serviceCfg.Database.SaveLastEpochsWrittenForValidators(
		ctx, s.latestEpochWrittenForValidator,
	); err != nil {
		log.Error(err)
	}
	log.WithField("elapsed", time.Since(start)).Debug(
		"Finished saving last epoch written per validator",
	)
	return nil
}

// Status of the slasher service.
func (*Service) Status() error {
	return nil
}

// waitForChainInitialization blocks until the chain clock is known. It returns
// an error when the service is stopped first; the clock is nil then and must
// not be used.
func (s *Service) waitForChainInitialization() error {
	clock, err := s.serviceCfg.ClockWaiter.WaitForClock(s.ctx)
	if err != nil {
		log.WithError(err).Error("Could not receive chain start notification")
		return err
	}
	s.genesisTime = clock.GenesisTime()
	log.WithField("genesisTime", s.genesisTime).Info(
		"Slasher received chain initialization event",
	)
	return nil
}

func (s *Service) waitForSync(genesisTime time.Time) {
	if slots.SinceGenesis(genesisTime) < params.BeaconConfig().SlotsPerEpoch || !s.serviceCfg.SyncChecker.Syncing() {
		return
	}
	slotTicker := slots.NewSlotTicker(s.genesisTime, params.BeaconConfig().SecondsPerSlot)
	defer slotTicker.Done()
	for {
		select {
		case <-slotTicker.C():
			// If node is still syncing, do not operate slasher.
			if s.serviceCfg.SyncChecker.Syncing() {
				continue
			}
			return
		case <-s.ctx.Done():
			return
		}
	}
}
