// Package execution defines a runtime service which is tasked with
// communicating with an execution endpoint, processing logs from a deposit
// contract, and the latest execution data headers for usage in the beacon node.
package execution

import (
	"context"
	"fmt"
	"math/big"
	"reflect"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrl/accounts/abi/bind"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/go-qrl/rpc"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/cache/depositsnapshot"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/execution/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/container/trie"
	contracts "github.com/theQRL/qrysm/contracts/deposit"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/monitoring/clientstats"
	"github.com/theQRL/qrysm/network"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	qrysmTime "github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/time/slots"
)

var (
	validDepositsCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_chain_valid_deposits_received",
		Help: "The number of valid deposits received in the deposit contract",
	})
	blockNumberGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "execution_chain_block_number",
		Help: "The current block number in the execution chain",
	})
	missedDepositLogsCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "execution_chain_missed_deposit_logs",
		Help: "The number of times a missed deposit log is detected",
	})
)

var (
	// time to wait before trying to reconnect with the execution node.
	backOffPeriod = 15 * time.Second
	// amount of times before we log the status of the execution dial attempt.
	logThreshold = 8
)

// ChainStartFetcher retrieves information pertaining to the chain start event
// of the beacon chain for usage across various services.
type ChainStartFetcher interface {
	ChainStartExecutionData() *qrysmpb.ExecutionData
}

// ChainInfoFetcher retrieves information about execution metadata at the QRL consensus genesis time.
type ChainInfoFetcher interface {
	GenesisExecutionChainInfo() (uint64, *big.Int)
	ExecutionClientConnected() bool
	ExecutionClientEndpoint() string
	ExecutionClientConnectionErr() error
}

// ExecutionBlockFetcher defines a struct that can retrieve execution chain blocks.
type ExecutionBlockFetcher interface {
	BlockTimeByHeight(ctx context.Context, height *big.Int) (uint64, error)
	BlockByTimestamp(ctx context.Context, time uint64) (*types.HeaderInfo, error)
	BlockHashByHeight(ctx context.Context, height *big.Int) (common.Hash, error)
	BlockExists(ctx context.Context, hash common.Hash) (bool, *big.Int, error)
}

// Chain defines a standard interface for the execution chain service in Qrysm.
type Chain interface {
	ChainStartFetcher
	ChainInfoFetcher
	ExecutionBlockFetcher
}

// RPCClient defines the rpc methods required to interact with the execution node.
type RPCClient interface {
	Close()
	BatchCall(b []rpc.BatchElem) error
	CallContext(ctx context.Context, result any, method string, args ...any) error
}

type RPCClientEmpty struct {
}

func (RPCClientEmpty) Close() {}
func (RPCClientEmpty) BatchCall([]rpc.BatchElem) error {
	return errors.New("rpc client is not initialized")
}

func (RPCClientEmpty) CallContext(context.Context, any, string, ...any) error {
	return errors.New("rpc client is not initialized")
}

// config defines a config struct for dependencies into the service.
type config struct {
	depositContractAddr     common.Address
	beaconDB                db.HeadAccessDatabase
	depositCache            cache.DepositCache
	stateNotifier           statefeed.Notifier
	stateGen                *stategen.State
	executionHeaderReqLimit uint64
	beaconNodeStatsUpdater  BeaconNodeStatsUpdater
	currHttpEndpoint        network.Endpoint
	headers                 []string
	finalizedStateAtStartup state.BeaconState
}

// Service fetches important information about the canonical
// execution chain via a web3 endpoint using a qrlclient.
// The beacon chain requires synchronization with the execution chain's current
// block hash, block number, and access to logs within the
// Validator Registration Contract on the execution chain to kick off the beacon
// chain's validator registration process.
type Service struct {
	// genesisBlockResolved guards the one-shot genesis block height lookup so a pruned
	// execution client doesn't make us spam HeaderByHash + retry on every loop iteration.
	genesisBlockResolved    bool
	isRunning               bool
	connectedExecution      bool
	processingLock          sync.RWMutex
	latestExecutionDataLock sync.RWMutex
	lastReceivedMerkleIndex int64 // Keeps track of the last received index to prevent log spam.
	chainStartData          *qrysmpb.ChainStartData
	depositContractCaller   *contracts.DepositContractCaller
	latestExecutionData     *qrysmpb.LatestExecutionData
	headerCache             *headerCache // cache to store block hash/block height.
	cancel                  context.CancelFunc
	executionHeadTicker     *time.Ticker
	cfg                     *config
	ctx                     context.Context
	rpcClient               RPCClient
	depositTrie             cache.MerkleTree
	runError                error
	preGenesisState         state.BeaconState
	httpLogger              bind.ContractFilterer
}

// NewService sets up a new instance with an ethclient when given a web3 endpoint as a string in the config.
func NewService(ctx context.Context, opts ...Option) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)
	_ = cancel // govet fix for lost cancel. Cancel is handled in service.Stop()
	var depositTrie cache.MerkleTree
	var err error
	if features.Get().EnableEIP4881 {
		depositTrie = depositsnapshot.NewDepositTree()
	} else {
		depositTrie, err = trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
		if err != nil {
			return nil, errors.Wrap(err, "could not set up deposit trie")
		}
	}
	genState, err := transition.EmptyGenesisStateZond()
	if err != nil {
		return nil, errors.Wrap(err, "could not set up genesis state")
	}

	s := &Service{
		ctx:       ctx,
		cancel:    cancel,
		rpcClient: RPCClientEmpty{},
		cfg: &config{
			beaconNodeStatsUpdater:  &NopBeaconNodeStatsUpdater{},
			executionHeaderReqLimit: defaultExecutionHeaderReqLimit,
		},
		latestExecutionData: &qrysmpb.LatestExecutionData{
			BlockHeight:        0,
			BlockTime:          0,
			BlockHash:          []byte{},
			LastRequestedBlock: 0,
		},
		headerCache: newHeaderCache(),
		depositTrie: depositTrie,
		chainStartData: &qrysmpb.ChainStartData{
			ExecutionData: &qrysmpb.ExecutionData{},
		},
		lastReceivedMerkleIndex: -1,
		preGenesisState:         genState,
		executionHeadTicker:     time.NewTicker(time.Duration(params.BeaconConfig().SecondsPerExecutionBlock) * time.Second),
	}

	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	executionData, err := s.validExecutionChainData(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "unable to validate execution chain data")
	}
	if err := s.initializeExecutionData(ctx, executionData); err != nil {
		return nil, err
	}
	return s, nil
}

// Start the execution chain service's main event loop.
func (s *Service) Start() {
	if err := s.setupExecutionClientConnections(s.ctx, s.cfg.currHttpEndpoint); err != nil {
		log.WithError(err).Error("Could not connect to execution endpoint")
	}

	s.isRunning = true

	// Poll the execution client connection and fallback if errors occur.
	s.pollConnectionStatus(s.ctx)

	go s.run(s.ctx.Done())
}

// Stop the web3 service's main event loop and associated goroutines.
func (s *Service) Stop() error {
	if s.cancel != nil {
		defer s.cancel()
	}
	if s.rpcClient != nil {
		s.rpcClient.Close()
	}
	return nil
}

// ChainStartExecutionData returns the execution data at chainstart.
func (s *Service) ChainStartExecutionData() *qrysmpb.ExecutionData {
	return s.chainStartData.ExecutionData
}

// Status is service health checks. Return nil or error.
func (s *Service) Status() error {
	// Service don't start
	if !s.isRunning {
		return nil
	}
	// get error from run function
	return s.runError
}

// ExecutionClientConnected checks whether are connected via RPC.
func (s *Service) ExecutionClientConnected() bool {
	return s.connectedExecution
}

// ExecutionClientEndpoint returns the URL of the current, connected execution client.
func (s *Service) ExecutionClientEndpoint() string {
	return s.cfg.currHttpEndpoint.Url
}

// ExecutionClientConnectionErr returns the error (if any) of the current connection.
func (s *Service) ExecutionClientConnectionErr() error {
	return s.runError
}

func (s *Service) updateBeaconNodeStats() {
	bs := clientstats.BeaconNodeStats{}
	if s.ExecutionClientConnected() {
		bs.SyncExecutionConnected = true
	}
	s.cfg.beaconNodeStatsUpdater.Update(bs)
}

func (s *Service) updateConnectedExecution(state bool) {
	s.connectedExecution = state
	s.updateBeaconNodeStats()
}

// refers to the latest execution block which follows the condition: execution_timestamp +
// SECONDS_PER_EXECUTION_BLOCK * EXECUTION_FOLLOW_DISTANCE <= current_unix_time
func (s *Service) followedBlockHeight(ctx context.Context) (uint64, error) {
	followTime := params.BeaconConfig().ExecutionFollowDistance * params.BeaconConfig().SecondsPerExecutionBlock
	s.latestExecutionDataLock.RLock()
	blockTime := s.latestExecutionData.BlockTime
	blockHeight := s.latestExecutionData.BlockHeight
	s.latestExecutionDataLock.RUnlock()
	latestBlockTime := uint64(0)
	if blockTime > followTime {
		latestBlockTime = max(blockTime-followTime, s.chainStartData.GenesisTime)
		// This should only come into play in testnets - when the chain hasn't advanced past the follow distance,
		// we don't want to consider any block before the genesis block.
		if blockHeight < params.BeaconConfig().ExecutionFollowDistance {
			latestBlockTime = blockTime
		}
	}
	blk, err := s.BlockByTimestamp(ctx, latestBlockTime)
	if err != nil {
		return 0, errors.Wrapf(err, "BlockByTimestamp=%d", latestBlockTime)
	}
	return blk.Number.Uint64(), nil
}

func (s *Service) initDepositCaches(ctx context.Context, ctrs []*qrysmpb.DepositContainer) error {
	if len(ctrs) == 0 {
		return nil
	}
	s.cfg.depositCache.InsertDepositContainers(ctx, ctrs)

	genesisState, err := s.cfg.beaconDB.GenesisState(ctx)
	if err != nil {
		return err
	}
	// Default to all post-genesis deposits in
	// the event we cannot find a finalized state.
	currIndex := genesisState.ExecutionDepositIndex()
	chkPt, err := s.cfg.beaconDB.FinalizedCheckpoint(ctx)
	if err != nil {
		return err
	}
	rt := bytesutil.ToBytes32(chkPt.Root)
	if rt != [32]byte{} {
		fState := s.cfg.finalizedStateAtStartup
		if fState == nil || fState.IsNil() {
			return errors.Errorf("finalized state with root %#x is nil", rt)
		}
		// Set deposit index to the one in the current archived state.
		currIndex = fState.ExecutionDepositIndex()

		// When a node pauses for some time and starts again, the deposits to finalize
		// accumulates. We finalize them here before we are ready to receive a block.
		// Otherwise, the first few blocks will be slower to compute as we will
		// hold the lock and be busy finalizing the deposits.
		// The deposit index in the state is always the index of the next deposit
		// to be included (rather than the last one to be processed). This was most likely
		// done as the state cannot represent signed integers.
		actualIndex := int64(currIndex) - 1 // lint:ignore uintcast -- deposit index will not exceed int64 in your lifetime.
		if err = s.cfg.depositCache.InsertFinalizedDeposits(ctx, actualIndex, common.Hash(fState.ExecutionData().BlockHash),
			0 /* Setting a zero value as we have no access to block height */); err != nil {
			return err
		}

		// Deposit proofs are only used during state transition and can be safely removed to save space.
		if err = s.cfg.depositCache.PruneProofs(ctx, actualIndex); err != nil {
			return errors.Wrap(err, "could not prune deposit proofs")
		}
	}
	validDepositsCount.Add(float64(currIndex))
	// Only add pending deposits if the container slice length
	// is more than the current index in state.
	if uint64(len(ctrs)) > currIndex {
		for _, c := range ctrs[currIndex:] {
			s.cfg.depositCache.InsertPendingDeposit(ctx, c.Deposit, c.ExecutionBlockHeight, c.Index, bytesutil.ToBytes32(c.DepositRoot))
		}
	}
	return nil
}

// processBlockHeader adds a newly observed execution block to the block cache and
// updates the latest blockHeight, blockHash, and blockTime properties of the service.
func (s *Service) processBlockHeader(header *types.HeaderInfo) {
	defer safelyHandlePanic()
	blockNumberGauge.Set(float64(header.Number.Int64()))
	s.latestExecutionDataLock.Lock()
	s.latestExecutionData.BlockHeight = header.Number.Uint64()
	s.latestExecutionData.BlockHash = header.Hash.Bytes()
	s.latestExecutionData.BlockTime = header.Time
	s.latestExecutionDataLock.Unlock()
	log.WithFields(logrus.Fields{
		"blockNumber": s.latestExecutionData.BlockHeight,
		"blockHash":   hexutil.Encode(s.latestExecutionData.BlockHash),
	}).Debug("Latest execution chain event")
}

// batchRequestHeaders requests the block range specified in the arguments. Instead of requesting
// each block in one call, it batches all requests into a single rpc call.
func (s *Service) batchRequestHeaders(startBlock, endBlock uint64) ([]*types.HeaderInfo, error) {
	if startBlock > endBlock {
		return nil, fmt.Errorf("start block height %d cannot be > end block height %d", startBlock, endBlock)
	}
	requestRange := (endBlock - startBlock) + 1
	elems := make([]rpc.BatchElem, 0, requestRange)
	headers := make([]*types.HeaderInfo, 0, requestRange)
	if requestRange == 0 {
		return headers, nil
	}
	for i := startBlock; i <= endBlock; i++ {
		header := &types.HeaderInfo{}
		elems = append(elems, rpc.BatchElem{
			Method: "qrl_getBlockByNumber",
			Args:   []any{hexutil.EncodeBig(big.NewInt(0).SetUint64(i)), false},
			Result: header,
			Error:  error(nil),
		})
		headers = append(headers, header)
	}
	ioErr := s.rpcClient.BatchCall(elems)
	if ioErr != nil {
		return nil, ioErr
	}
	for _, e := range elems {
		if e.Error != nil {
			return nil, e.Error
		}
	}
	for _, h := range headers {
		if h != nil {
			if err := s.headerCache.AddHeader(h); err != nil {
				return nil, err
			}
		}
	}
	return headers, nil
}

// safelyHandleHeader will recover and log any panic that occurs from the block
func safelyHandlePanic() {
	if r := recover(); r != nil {
		log.WithFields(logrus.Fields{
			"r": r,
		}).Error("Panicked when handling data from QRL execution chain! Recovering...")

		debug.PrintStack()
	}
}

func (s *Service) handleExecutionFollowDistance() {
	defer safelyHandlePanic()
	ctx := s.ctx

	// use a 5 minutes timeout for block time, because the max mining time is 278 sec (block 7208027)
	// (analyzed the time of the block from 2018-09-01 to 2019-02-13)
	fiveMinutesTimeout := qrysmTime.Now().Add(-5 * time.Minute)
	s.latestExecutionDataLock.RLock()
	blockTime := s.latestExecutionData.BlockTime
	lastRequestedBlock := s.latestExecutionData.LastRequestedBlock
	blockHeight := s.latestExecutionData.BlockHeight
	s.latestExecutionDataLock.RUnlock()
	// check that web3 client is syncing
	blockUnixTime, ok := unixTimeFromUint64(blockTime)
	if !ok {
		log.Warn("Execution client returned an invalid block time")
	} else if blockUnixTime.Before(fiveMinutesTimeout) {
		log.Warn("Execution client is not syncing")
	}

	// If the last requested block has not changed,
	// we do not request batched logs as this means there are no new
	// logs for the execution chain service to process. Also it is a potential
	// failure condition as would mean we have not respected the protocol threshold.
	if lastRequestedBlock == blockHeight {
		log.Error("Beacon node is not respecting the follow distance")
		return
	}
	if err := s.requestBatchedHeadersAndLogs(ctx); err != nil {
		s.runError = errors.Wrap(err, "requestBatchedHeadersAndLogs")
		log.Error(err)
		return
	}
	// Reset the Status.
	if s.runError != nil {
		s.runError = nil
	}
}

func unixTimeFromUint64(seconds uint64) (time.Time, bool) {
	const maxInt64 = uint64(1<<63 - 1)
	if seconds > maxInt64 {
		return time.Time{}, false
	}
	// lint:ignore uintcast blockTime is checked above against int64 overflow.
	return time.Unix(int64(seconds), 0), true
}

func (s *Service) initExecutionService() {
	// Use a custom logger to only log errors
	logCounter := 0
	errorLogger := func(err error, msg string) {
		if logCounter > logThreshold {
			log.WithError(err).Error(msg)
			logCounter = 0
		}
		logCounter++
	}

	// Run in a select loop to retry in the event of any failures.
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			ctx := s.ctx
			header, err := s.HeaderByNumber(ctx, nil)
			if err != nil {
				err = errors.Wrap(err, "HeaderByNumber")
				s.retryExecutionClientConnection(ctx, err)
				errorLogger(err, "Unable to retrieve latest execution client header")
				continue
			}

			s.latestExecutionDataLock.Lock()
			s.latestExecutionData.BlockHeight = header.Number.Uint64()
			s.latestExecutionData.BlockHash = header.Hash.Bytes()
			s.latestExecutionData.BlockTime = header.Time
			s.latestExecutionDataLock.Unlock()

			if err := s.processPastLogs(ctx); err != nil {
				err = errors.Wrap(err, "processPastLogs")
				s.retryExecutionClientConnection(ctx, err)
				errorLogger(
					err,
					"Unable to process past deposit contract logs, perhaps your execution client is not fully synced",
				)
				continue
			}
			// Cache execution headers from our voting period.
			if err := s.cacheHeadersForExecutionDataVote(ctx); err != nil {
				err = errors.Wrap(err, "cacheHeadersForExecutionDataVote")
				s.retryExecutionClientConnection(ctx, err)
				if errors.Is(err, errBlockTimeTooLate) {
					log.WithError(err).Debug("Unable to cache headers for execution client votes")
				} else {
					errorLogger(err, "Unable to cache headers for execution client votes")
				}
				continue
			}
			// Handle edge case with embedded genesis state by fetching genesis header to determine
			// its height. Attempt at most once per process: with PoS-from-genesis the resolved
			// block number is 0 (the same as the uninitialized sentinel), so we'd otherwise retry
			// every loop iteration and spam "pruned history unavailable" on pruned execution
			// clients. A failed lookup is non-fatal — leave GenesisBlock at 0 and continue.
			if !s.genesisBlockResolved && s.chainStartData.GenesisBlock == 0 {
				s.genesisBlockResolved = true
				genHash := common.BytesToHash(s.chainStartData.ExecutionData.BlockHash)
				if genHash != [32]byte{} {
					genHeader, err := s.HeaderByHash(ctx, genHash)
					if err != nil {
						log.WithError(err).WithField("hash", fmt.Sprintf("%#x", genHash)).
							Warn("Could not retrieve proof-of-stake genesis block data; assuming genesis block 0")
					} else {
						s.chainStartData.GenesisBlock = genHeader.Number.Uint64()
					}
				}
				if err := s.saveExecutionChainData(ctx); err != nil {
					err = errors.Wrap(err, "saveExecutionChainData")
					s.retryExecutionClientConnection(ctx, err)
					errorLogger(err, "Unable to save execution client data")
					continue
				}
			}
			return
		}
	}
}

// run subscribes to all the services for the execution chain.
func (s *Service) run(done <-chan struct{}) {
	s.runError = nil

	s.initExecutionService()
	// The finalized state is no longer needed after init; release the
	// (potentially multi-MB) reference so it can be garbage-collected.
	s.removeStartupState()

	for {
		select {
		case <-done:
			s.isRunning = false
			s.runError = nil
			s.rpcClient.Close()
			s.updateConnectedExecution(false)
			log.Debug("Context closed, exiting goroutine")
			return
		case <-s.executionHeadTicker.C:
			head, err := s.HeaderByNumber(s.ctx, nil)
			if err != nil {
				s.pollConnectionStatus(s.ctx)
				log.WithError(err).Debug("Could not fetch latest execution header")
				continue
			}
			s.processBlockHeader(head)
			s.handleExecutionFollowDistance()
		}
	}
}

// cacheHeadersForExecutionDataVote makes sure that voting for executiondata after startup utilizes cached headers
// instead of making multiple RPC requests to the execution endpoint.
func (s *Service) cacheHeadersForExecutionDataVote(ctx context.Context) error {
	// Find the end block to request from.
	end, err := s.followedBlockHeight(ctx)
	if err != nil {
		return errors.Wrap(err, "followedBlockHeight")
	}
	start, err := s.determineEarliestVotingBlock(ctx, end)
	if err != nil {
		return errors.Wrapf(err, "determineEarliestVotingBlock=%d", end)
	}
	return s.cacheBlockHeaders(start, end)
}

// Caches block headers from the desired range.
func (s *Service) cacheBlockHeaders(start, end uint64) error {
	batchSize := s.cfg.executionHeaderReqLimit
	for i := start; i < end; i += batchSize {
		startReq := i
		endReq := i + batchSize
		if endReq > 0 {
			// Reduce the end request by one
			// to prevent total batch size from exceeding
			// the allotted limit.
			endReq -= 1
		}
		if endReq > end {
			endReq = end
		}
		// We call batchRequestHeaders for its header caching side-effect, so we don't need the return value.
		_, err := s.batchRequestHeaders(startReq, endReq)
		if err != nil {
			if clientTimedOutError(err) {
				// Reduce batch size as execution node is
				// unable to respond to the request in time.
				batchSize /= 2
				// Always have it greater than 0.
				if batchSize == 0 {
					batchSize += 1
				}

				// Reset request value
				if i > batchSize {
					i -= batchSize
				}
				continue
			}
			return errors.Wrapf(err, "cacheBlockHeaders, start=%d, end=%d", startReq, endReq)
		}
	}
	return nil
}

// Determines the earliest voting block from which to start caching all our previous headers from.
func (s *Service) determineEarliestVotingBlock(ctx context.Context, followBlock uint64) (uint64, error) {
	genesisTime := s.chainStartData.GenesisTime
	currSlot := slots.CurrentSlot(genesisTime)

	// In the event genesis has not occurred yet, we just request to go back follow_distance blocks.
	if genesisTime == 0 || currSlot == 0 {
		earliestBlk := uint64(0)
		if followBlock > params.BeaconConfig().ExecutionFollowDistance {
			earliestBlk = followBlock - params.BeaconConfig().ExecutionFollowDistance
		}
		return earliestBlk, nil
	}
	// This should only come into play in testnets - when the chain hasn't advanced past the follow distance,
	// we don't want to consider any block before the genesis block.
	s.latestExecutionDataLock.RLock()
	blockHeight := s.latestExecutionData.BlockHeight
	s.latestExecutionDataLock.RUnlock()
	if blockHeight < params.BeaconConfig().ExecutionFollowDistance {
		return 0, nil
	}
	votingTime := slots.VotingPeriodStartTime(genesisTime, currSlot)
	followBackDist := 2 * params.BeaconConfig().SecondsPerExecutionBlock * params.BeaconConfig().ExecutionFollowDistance
	if followBackDist > votingTime {
		return 0, errors.Errorf("invalid genesis time provided. %d > %d", followBackDist, votingTime)
	}
	earliestValidTime := votingTime - followBackDist
	if earliestValidTime < genesisTime {
		return 0, nil
	}
	hdr, err := s.BlockByTimestamp(ctx, earliestValidTime)
	if err != nil {
		return 0, err
	}
	return hdr.Number.Uint64(), nil
}

// initializes our service from the provided executiondata object by initializing all the relevant
// fields and data.
func (s *Service) initializeExecutionData(ctx context.Context, executionDataInDB *qrysmpb.ExecutionChainData) error {
	// The node has no executiondata persisted on disk, so we exit and instead
	// request from contract logs.
	if executionDataInDB == nil {
		return nil
	}
	var err error
	if features.Get().EnableEIP4881 {
		if executionDataInDB.DepositSnapshot != nil {
			s.depositTrie, err = depositsnapshot.DepositTreeFromSnapshotProto(executionDataInDB.DepositSnapshot)
		} else {
			if err := s.migrateOldDepositTree(executionDataInDB); err != nil {
				return err
			}
		}
	} else {
		if executionDataInDB.Trie == nil && executionDataInDB.DepositSnapshot != nil {
			return errors.Errorf("trying to use old deposit trie after migration to the new trie. "+
				"Run with the --%s flag to resume normal operations.", features.EnableEIP4881.Name)
		}
		s.depositTrie, err = trie.CreateTrieFromProto(executionDataInDB.Trie)
	}
	if err != nil {
		return err
	}
	s.chainStartData = executionDataInDB.ChainstartData
	if !reflect.ValueOf(executionDataInDB.BeaconState).IsZero() {
		s.preGenesisState, err = native.InitializeFromProtoZond(executionDataInDB.BeaconState)
		if err != nil {
			return errors.Wrap(err, "Could not initialize state trie")
		}
	}
	s.latestExecutionData = executionDataInDB.CurrentExecutionData
	if features.Get().EnableEIP4881 {
		ctrs := executionDataInDB.DepositContainers
		// Look at previously finalized index, as we are building off a finalized
		// snapshot rather than the full trie.
		lastFinalizedIndex := int64(s.depositTrie.NumOfItems() - 1)
		// Correctly initialize missing deposits into active trie.
		for _, c := range ctrs {
			if c.Index > lastFinalizedIndex {
				depRoot, err := c.Deposit.Data.HashTreeRoot()
				if err != nil {
					return err
				}
				if err := s.depositTrie.Insert(depRoot[:], int(c.Index)); err != nil {
					return err
				}
			}
		}
	}
	numOfItems := s.depositTrie.NumOfItems()
	s.lastReceivedMerkleIndex = int64(numOfItems - 1)
	if err := s.initDepositCaches(ctx, executionDataInDB.DepositContainers); err != nil {
		return errors.Wrap(err, "could not initialize caches")
	}
	return nil
}

// Validates that all deposit containers are valid and have their relevant indices
// in order.
func validateDepositContainers(ctrs []*qrysmpb.DepositContainer) bool {
	ctrLen := len(ctrs)
	// Exit for empty containers.
	if ctrLen == 0 {
		return true
	}
	// Sort deposits in ascending order.
	sort.Slice(ctrs, func(i, j int) bool {
		return ctrs[i].Index < ctrs[j].Index
	})
	startIndex := int64(0)
	for _, c := range ctrs {
		if c.Index != startIndex {
			log.Info("Recovering missing deposit containers, node is re-requesting missing deposit data")
			return false
		}
		startIndex++
	}
	return true
}

// Validates the current execution chain data is saved and makes sure that any
// embedded genesis state is correctly accounted for.
func (s *Service) validExecutionChainData(ctx context.Context) (*qrysmpb.ExecutionChainData, error) {
	genState, err := s.cfg.beaconDB.GenesisState(ctx)
	if err != nil {
		return nil, err
	}
	executionData, err := s.cfg.beaconDB.ExecutionChainData(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "unable to retrieve execution data")
	}
	// Exit early if no genesis state is saved.
	if genState == nil || genState.IsNil() {
		return executionData, nil
	}
	if executionData == nil || !validateDepositContainers(executionData.DepositContainers) {
		pbState, err := native.ProtobufBeaconStateZond(s.preGenesisState.ToProtoUnsafe())
		if err != nil {
			return nil, err
		}
		s.chainStartData = &qrysmpb.ChainStartData{
			GenesisTime:   genState.GenesisTime(),
			GenesisBlock:  0,
			ExecutionData: genState.ExecutionData(),
		}
		executionData = &qrysmpb.ExecutionChainData{
			CurrentExecutionData: s.latestExecutionData,
			ChainstartData:       s.chainStartData,
			BeaconState:          pbState,
			DepositContainers:    s.cfg.depositCache.AllDepositContainers(ctx),
		}
		if features.Get().EnableEIP4881 {
			trie, ok := s.depositTrie.(*depositsnapshot.DepositTree)
			if !ok {
				return nil, errors.New("deposit trie was not EIP4881 DepositTree")
			}
			executionData.DepositSnapshot, err = trie.ToProto()
			if err != nil {
				return nil, err
			}
		} else {
			trie, ok := s.depositTrie.(*trie.SparseMerkleTrie)
			if !ok {
				return nil, errors.New("deposit trie was not SparseMerkleTrie")
			}
			executionData.Trie = trie.ToProto()
		}
		if err := s.cfg.beaconDB.SaveExecutionChainData(ctx, executionData); err != nil {
			return nil, err
		}
	}
	return executionData, nil
}

func (s *Service) removeStartupState() {
	s.cfg.finalizedStateAtStartup = nil
}

func (s *Service) migrateOldDepositTree(executionDataInDB *qrysmpb.ExecutionChainData) error {
	oldDepositTrie, err := trie.CreateTrieFromProto(executionDataInDB.Trie)
	if err != nil {
		return err
	}
	newDepositTrie := depositsnapshot.NewDepositTree()
	for i, item := range oldDepositTrie.Items() {
		if err = newDepositTrie.Insert(item, i); err != nil {
			return errors.Wrapf(err, "could not insert item at index %d into deposit snapshot tree", i)
		}
	}
	newDepositRoot, err := newDepositTrie.HashTreeRoot()
	if err != nil {
		return err
	}
	depositRoot, err := oldDepositTrie.HashTreeRoot()
	if err != nil {
		return err
	}
	if newDepositRoot != depositRoot {
		return errors.Wrapf(err, "mismatched deposit roots, old %#x != new %#x", depositRoot, newDepositRoot)
	}
	s.depositTrie = newDepositTrie
	return nil
}
