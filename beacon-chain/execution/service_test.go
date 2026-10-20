package execution

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	qrl "github.com/theQRL/go-qrl"
	"github.com/theQRL/go-qrl/accounts/abi"
	"github.com/theQRL/go-qrl/accounts/abi/bind/backends"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/common/hexutil"
	gqrltypes "github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/go-qrl/rpc"
	"github.com/theQRL/qrysm/async/event"
	"github.com/theQRL/qrysm/beacon-chain/cache/depositcache"
	"github.com/theQRL/qrysm/beacon-chain/cache/depositsnapshot"
	dbutil "github.com/theQRL/qrysm/beacon-chain/db/testing"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/execution/types"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/container/trie"
	contracts "github.com/theQRL/qrysm/contracts/deposit"
	"github.com/theQRL/qrysm/contracts/deposit/mock"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/monitoring/clientstats"
	"github.com/theQRL/qrysm/network"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

var _ ChainStartFetcher = (*Service)(nil)
var _ ChainInfoFetcher = (*Service)(nil)
var _ ExecutionBlockFetcher = (*Service)(nil)
var _ Chain = (*Service)(nil)

type goodLogger struct {
	backend *backends.SimulatedBackend
}

func (_ *goodLogger) Close() {}

func (g *goodLogger) SubscribeFilterLogs(ctx context.Context, q qrl.FilterQuery, ch chan<- gqrltypes.Log) (qrl.Subscription, error) {
	if g.backend == nil {
		return new(event.Feed).Subscribe(ch), nil
	}
	return g.backend.SubscribeFilterLogs(ctx, q, ch)
}

func (g *goodLogger) FilterLogs(ctx context.Context, q qrl.FilterQuery) ([]gqrltypes.Log, error) {
	if g.backend == nil {
		logs := make([]gqrltypes.Log, 3)
		for i := range logs {
			logs[i].Address = common.Address{}
			logs[i].Topics = make([]common.LogTopic, 5)
			logs[i].Topics[0] = common.LogTopic{'a'}
			logs[i].Topics[1] = common.LogTopic{'b'}
			logs[i].Topics[2] = common.LogTopic{'c'}

		}
		return logs, nil
	}
	return g.backend.FilterLogs(ctx, q)
}

type goodNotifier struct {
	MockStateFeed *event.Feed
}

func (g *goodNotifier) StateFeed() *event.Feed {
	if g.MockStateFeed == nil {
		g.MockStateFeed = new(event.Feed)
	}
	return g.MockStateFeed
}

func TestStart_OK(t *testing.T) {
	hook := logTest.NewGlobal()
	beaconDB := dbutil.SetupDB(t)
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup execution service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.rpcClient = &mockExecution.RPCClient{Backend: testAcc.Backend}
	web3Service.depositContractCaller, err = contracts.NewDepositContractCaller(testAcc.ContractAddr, testAcc.Backend)
	require.NoError(t, err)
	testAcc.Backend.Commit()

	web3Service.Start()
	if len(hook.Entries) > 0 {
		msg := hook.LastEntry().Message
		want := "Could not connect to execution endpoint"
		if strings.Contains(want, msg) {
			t.Errorf("incorrect log, expected %s, got %s", want, msg)
		}
	}
	hook.Reset()
	web3Service.cancel()
}

func TestStart_NoHttpEndpointDefinedFails_WithoutChainStarted(t *testing.T) {
	hook := logTest.NewGlobal()
	beaconDB := dbutil.SetupDB(t)
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	_, err = NewService(context.Background(),
		WithHttpEndpoint(""),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err)
	require.LogsDoNotContain(t, hook, "missing address")
}

func TestStop_OK(t *testing.T) {
	hook := logTest.NewGlobal()
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	beaconDB := dbutil.SetupDB(t)
	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 QRL execution chain service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.depositContractCaller, err = contracts.NewDepositContractCaller(testAcc.ContractAddr, testAcc.Backend)
	require.NoError(t, err)

	testAcc.Backend.Commit()

	err = web3Service.Stop()
	require.NoError(t, err, "Unable to stop web3 QRL execution chain service")

	// The context should have been canceled.
	assert.NotNil(t, web3Service.ctx.Err(), "Context wasnt canceled")

	hook.Reset()
}

func TestService_ExecutionSynced(t *testing.T) {
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	beaconDB := dbutil.SetupDB(t)
	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 QRL execution chain service")
	web3Service = setDefaultMocks(web3Service)
	web3Service.depositContractCaller, err = contracts.NewDepositContractCaller(testAcc.ContractAddr, testAcc.Backend)
	require.NoError(t, err)

	currTime := testAcc.Backend.Blockchain().CurrentHeader().Time
	now := time.Now()
	assert.NoError(t, testAcc.Backend.AdjustTime(now.Sub(time.Unix(int64(currTime), 0))))
	testAcc.Backend.Commit()
}

func TestFollowBlock_OK(t *testing.T) {
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	beaconDB := dbutil.SetupDB(t)
	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 QRL execution chain service")

	// simulated backend sets execution block
	// time as 10 seconds
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig().Copy()
	conf.SecondsPerExecutionBlock = 10
	params.OverrideBeaconConfig(conf)

	web3Service = setDefaultMocks(web3Service)
	web3Service.rpcClient = &mockExecution.RPCClient{Backend: testAcc.Backend}
	baseHeight := testAcc.Backend.Blockchain().CurrentBlock().Number.Uint64()
	// process follow_distance blocks
	for i := 0; i < int(params.BeaconConfig().ExecutionFollowDistance); i++ {
		testAcc.Backend.Commit()
	}
	// set current height
	web3Service.latestExecutionData.BlockHeight = testAcc.Backend.Blockchain().CurrentBlock().Number.Uint64()
	web3Service.latestExecutionData.BlockTime = testAcc.Backend.Blockchain().CurrentBlock().Time

	h, err := web3Service.followedBlockHeight(context.Background())
	require.NoError(t, err)
	assert.Equal(t, baseHeight, h, "Unexpected block height")
	numToForward := uint64(2)
	expectedHeight := numToForward + baseHeight
	// forward 2 blocks
	for range numToForward {
		testAcc.Backend.Commit()
	}
	// set current height
	web3Service.latestExecutionData.BlockHeight = testAcc.Backend.Blockchain().CurrentBlock().Number.Uint64()
	web3Service.latestExecutionData.BlockTime = testAcc.Backend.Blockchain().CurrentBlock().Time

	h, err = web3Service.followedBlockHeight(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedHeight, h, "Unexpected block height")
}

func TestStatus(t *testing.T) {
	now := time.Now()

	beforeFiveMinutesAgo := uint64(now.Add(-5*time.Minute - 30*time.Second).Unix())
	afterFiveMinutesAgo := uint64(now.Add(-5*time.Minute + 30*time.Second).Unix())

	testCases := map[*Service]string{
		// "status is ok" cases
		{}: "",
		{isRunning: true, latestExecutionData: &qrysmpb.LatestExecutionData{BlockTime: afterFiveMinutesAgo}}:   "",
		{isRunning: false, latestExecutionData: &qrysmpb.LatestExecutionData{BlockTime: beforeFiveMinutesAgo}}: "",
		{isRunning: false, runError: errors.New("test runError")}:                                              "",
		// "status is error" cases
		{isRunning: true, runError: errors.New("test runError")}: "test runError",
	}

	for web3ServiceState, wantedErrorText := range testCases {
		status := web3ServiceState.Status()
		if status == nil {
			assert.Equal(t, "", wantedErrorText)

		} else {
			assert.Equal(t, wantedErrorText, status.Error())
		}
	}
}

func TestStart_ConnectionFailureHealth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer synctest.Wait()
		defer cancel()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		s := &Service{
			ctx: ctx, rpcClient: RPCClientEmpty{}, executionHeadTicker: ticker,
			cfg: &config{
				currHttpEndpoint:       network.Endpoint{Url: "unsupported://execution"},
				beaconNodeStatsUpdater: &NopBeaconNodeStatsUpdater{},
			},
		}
		go s.Start()
		synctest.Wait()
		assert.NotNil(t, s.Status(), "startup connection failure must fail the health check")
		assert.NotNil(t, s.ExecutionClientConnectionErr())
		assert.Equal(t, false, s.ExecutionClientConnected())
	})
}

type connectionHealthClient struct {
	RPCClientEmpty
	header    *types.HeaderInfo
	nextError chan error
}

func (c *connectionHealthClient) CallContext(_ context.Context, result any, _ string, _ ...any) error {
	select {
	case err := <-c.nextError:
		return err
	default:
	}
	*result.(**types.HeaderInfo) = c.header.Copy()
	return nil
}

func (c *connectionHealthClient) BatchCallContext(_ context.Context, elems []rpc.BatchElem) error {
	for _, elem := range elems {
		*elem.Result.(*types.HeaderInfo) = *c.header.Copy()
	}
	return nil
}

func TestRun_ConnectionFailureHealth(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig().Copy()
	conf.ExecutionFollowDistance = 0
	params.OverrideBeaconConfig(conf)
	contractABI, err := abi.JSON(strings.NewReader(contracts.DepositContractABI))
	require.NoError(t, err)
	encoded, err := contractABI.Methods["get_deposit_count"].Outputs.Pack(bytesutil.Bytes8(0))
	require.NoError(t, err)
	caller, err := contracts.NewDepositContractCaller(common.Address{}, &depositCountBackend{result: encoded})
	require.NoError(t, err)
	beaconDB := dbutil.SetupDB(t)
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed=%t", failed), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer synctest.Wait()
				defer cancel()
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				client := &connectionHealthClient{
					header:    &types.HeaderInfo{Number: big.NewInt(1), Time: 100, Hash: common.Hash{1}},
					nextError: make(chan error, 1),
				}
				stats := &mockBSUpdater{lastBS: clientstats.BeaconNodeStats{SyncExecutionConnected: true}}
				s := &Service{
					ctx: ctx, rpcClient: client, executionHeadTicker: ticker,
					cfg:       &config{beaconDB: beaconDB, beaconNodeStatsUpdater: stats, executionHeaderReqLimit: 1},
					isRunning: true, connectedExecution: true, genesisBlockResolved: true,
					latestExecutionData: &qrysmpb.LatestExecutionData{}, chainStartData: &qrysmpb.ChainStartData{},
					headerCache: newHeaderCache(), depositContractCaller: caller, httpLogger: &limitedLogFilter{limit: 1},
				}
				go s.run(ctx.Done())
				synctest.Wait()
				require.NoError(t, s.Status())
				require.Equal(t, true, s.ExecutionClientConnected())
				callErr := errors.New("execution endpoint unavailable")
				if failed {
					client.nextError <- callErr
				}
				time.Sleep(time.Second)
				synctest.Wait()
				if failed {
					assert.Equal(t, true, errors.Is(s.Status(), callErr))
					assert.Equal(t, true, errors.Is(s.ExecutionClientConnectionErr(), callErr))
				} else {
					assert.NoError(t, s.Status())
					assert.NoError(t, s.ExecutionClientConnectionErr())
				}
				assert.Equal(t, !failed, s.ExecutionClientConnected())
				assert.Equal(t, !failed, stats.lastBS.SyncExecutionConnected)
			})
		})
	}
}

func TestInitExecutionService_ConcurrentGenesisInfo(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig().Copy()
	conf.ExecutionFollowDistance = 0
	params.OverrideBeaconConfig(conf)
	reset := features.InitWithReset(&features.Flags{})
	defer reset()
	deposits, err := depositcache.New()
	require.NoError(t, err)
	s, err := NewService(context.Background(), WithDatabase(dbutil.SetupDB(t)), WithDepositCache(deposits))
	require.NoError(t, err)
	defer func() { require.NoError(t, s.Stop()) }()
	defer s.executionHeadTicker.Stop()
	s.chainStartData.ExecutionData.BlockHash = bytesutil.PadTo([]byte{1}, 32)
	s.rpcClient = &connectionHealthClient{header: &types.HeaderInfo{Number: big.NewInt(1), Time: 100, Hash: common.Hash{1}}}
	s.cfg.executionHeaderReqLimit = 1
	s.httpLogger = &limitedLogFilter{limit: 1}
	contractABI, err := abi.JSON(strings.NewReader(contracts.DepositContractABI))
	require.NoError(t, err)
	encoded, err := contractABI.Methods["get_deposit_count"].Outputs.Pack(bytesutil.Bytes8(0))
	require.NoError(t, err)
	s.depositContractCaller, err = contracts.NewDepositContractCaller(common.Address{}, &depositCountBackend{result: encoded})
	require.NoError(t, err)

	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		close(started)
		for range 100 {
			s.GenesisExecutionChainInfo()
		}
	}()
	<-started
	s.initExecutionService()
	<-done
	_, height := s.GenesisExecutionChainInfo()
	require.Equal(t, uint64(1), height.Uint64())
}

func TestHandlePanic_OK(t *testing.T) {
	hook := logTest.NewGlobal()
	beaconDB := dbutil.SetupDB(t)
	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 QRL execution chain service")
	// nil executionDataFetcher would panic if cached value not used
	web3Service.rpcClient = nil
	web3Service.processBlockHeader(nil)
	require.LogsContain(t, hook, "Panicked when handling data from QRL execution chain!")
}

func TestInitDepositCache_OK(t *testing.T) {
	ctrs := []*qrysmpb.DepositContainer{
		{Index: 0, ExecutionBlockHeight: 2, Deposit: &qrysmpb.Deposit{Proof: [][]byte{[]byte("A")}, Data: &qrysmpb.Deposit_Data{PublicKey: []byte{}, RandaoCommitment: make([]byte, 32)}}},
		{Index: 1, ExecutionBlockHeight: 4, Deposit: &qrysmpb.Deposit{Proof: [][]byte{[]byte("B")}, Data: &qrysmpb.Deposit_Data{PublicKey: []byte{}, RandaoCommitment: make([]byte, 32)}}},
		{Index: 2, ExecutionBlockHeight: 6, Deposit: &qrysmpb.Deposit{Proof: [][]byte{[]byte("c")}, Data: &qrysmpb.Deposit_Data{PublicKey: []byte{}, RandaoCommitment: make([]byte, 32)}}},
	}
	gs, _ := util.DeterministicGenesisStateZond(t, 1)
	beaconDB := dbutil.SetupDB(t)
	s := &Service{
		chainStartData:  &qrysmpb.ChainStartData{},
		preGenesisState: gs,
		cfg:             &config{beaconDB: beaconDB},
	}
	var err error
	s.cfg.depositCache, err = depositcache.New()
	require.NoError(t, err)

	blockRootA := [32]byte{'a'}

	emptyState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, s.cfg.beaconDB.SaveGenesisBlockRoot(context.Background(), blockRootA))
	require.NoError(t, s.cfg.beaconDB.SaveState(context.Background(), emptyState, blockRootA))
	require.NoError(t, s.initDepositCaches(context.Background(), ctrs))
	require.Equal(t, 3, len(s.cfg.depositCache.PendingContainers(context.Background(), nil)))
}

// Regression: before any genesis state is saved (pre-genesis), the DB's
// GenesisState returns (nil, nil). initDepositCaches must not dereference it:
// the containers still go into the deposit cache, but nothing is pending
// because no deposit has been included in a chain yet (upstream's
// Chainstarted guard).
func TestInitDepositCache_NoGenesisState(t *testing.T) {
	// Use a config name without an embedded genesis state so that the DB has
	// no genesis state to fall back to.
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ConfigName = "no-embedded-genesis"
	params.OverrideBeaconConfig(cfg)

	ctrs := []*qrysmpb.DepositContainer{
		{Index: 0, ExecutionBlockHeight: 2, Deposit: &qrysmpb.Deposit{Proof: [][]byte{[]byte("A")}, Data: &qrysmpb.Deposit_Data{PublicKey: []byte{}, RandaoCommitment: make([]byte, 32)}}},
		{Index: 1, ExecutionBlockHeight: 4, Deposit: &qrysmpb.Deposit{Proof: [][]byte{[]byte("B")}, Data: &qrysmpb.Deposit_Data{PublicKey: []byte{}, RandaoCommitment: make([]byte, 32)}}},
		{Index: 2, ExecutionBlockHeight: 6, Deposit: &qrysmpb.Deposit{Proof: [][]byte{[]byte("c")}, Data: &qrysmpb.Deposit_Data{PublicKey: []byte{}, RandaoCommitment: make([]byte, 32)}}},
	}
	gs, _ := util.DeterministicGenesisStateZond(t, 1)
	beaconDB := dbutil.SetupDB(t)
	s := &Service{
		chainStartData:  &qrysmpb.ChainStartData{},
		preGenesisState: gs,
		cfg:             &config{beaconDB: beaconDB},
	}
	var err error
	s.cfg.depositCache, err = depositcache.New()
	require.NoError(t, err)

	// No genesis block root or genesis state is saved in the DB.
	genState, err := beaconDB.GenesisState(context.Background())
	require.NoError(t, err)
	require.Equal(t, true, genState == nil || genState.IsNil(), "test setup: expected no genesis state")

	require.NoError(t, s.initDepositCaches(context.Background(), ctrs))
	require.Equal(t, 3, len(s.cfg.depositCache.AllDepositContainers(context.Background())))
	require.Equal(t, 0, len(s.cfg.depositCache.PendingContainers(context.Background(), nil)))
}

func TestInitDepositCacheWithFinalization_OK(t *testing.T) {
	ctrs := []*qrysmpb.DepositContainer{
		{
			Index:                0,
			ExecutionBlockHeight: 2,
			Deposit: &qrysmpb.Deposit{
				Data: &qrysmpb.Deposit_Data{
					PublicKey:           bytesutil.PadTo([]byte{0}, 2592),
					WithdrawalRecipient: make([]byte, 64),
					Signature:           make([]byte, 4627),

					RandaoCommitment: make([]byte, 32),
				},
			},
		},
		{
			Index:                1,
			ExecutionBlockHeight: 4,
			Deposit: &qrysmpb.Deposit{
				Data: &qrysmpb.Deposit_Data{
					PublicKey:           bytesutil.PadTo([]byte{1}, 2592),
					WithdrawalRecipient: make([]byte, 64),
					Signature:           make([]byte, 4627),

					RandaoCommitment: make([]byte, 32),
				},
			},
		},
		{
			Index:                2,
			ExecutionBlockHeight: 6,
			Deposit: &qrysmpb.Deposit{
				Data: &qrysmpb.Deposit_Data{
					PublicKey:           bytesutil.PadTo([]byte{2}, 2592),
					WithdrawalRecipient: make([]byte, 64),
					Signature:           make([]byte, 4627),

					RandaoCommitment: make([]byte, 32),
				},
			},
		},
	}
	gs, _ := util.DeterministicGenesisStateZond(t, 1)
	beaconDB := dbutil.SetupDB(t)
	s := &Service{
		chainStartData:  &qrysmpb.ChainStartData{},
		preGenesisState: gs,
		cfg:             &config{beaconDB: beaconDB},
	}
	var err error
	s.cfg.depositCache, err = depositcache.New()
	require.NoError(t, err)

	headBlock := util.NewBeaconBlockZond()
	headRoot, err := headBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	stateGen := stategen.New(beaconDB, doublylinkedtree.New())

	emptyState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, s.cfg.beaconDB.SaveGenesisBlockRoot(context.Background(), headRoot))
	require.NoError(t, s.cfg.beaconDB.SaveState(context.Background(), emptyState, headRoot))
	require.NoError(t, stateGen.SaveState(context.Background(), headRoot, emptyState))
	s.cfg.stateGen = stateGen
	require.NoError(t, emptyState.SetExecutionDepositIndex(3))

	ctx := context.Background()
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: slots.ToEpoch(0), Root: headRoot[:]}))
	s.cfg.finalizedStateAtStartup = emptyState

	require.NoError(t, s.initDepositCaches(context.Background(), ctrs))
	fDeposits, err := s.cfg.depositCache.FinalizedDeposits(ctx)
	require.NoError(t, err)
	deps := s.cfg.depositCache.NonFinalizedDeposits(context.Background(), fDeposits.MerkleTrieIndex(), nil)
	assert.Equal(t, 0, len(deps))
}

func TestNewService_EarliestVotingBlock(t *testing.T) {
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	beaconDB := dbutil.SetupDB(t)
	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	web3Service, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 QRL execution chain service")
	// simulated backend sets execution block
	// time as 10 seconds
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig().Copy()
	conf.SecondsPerExecutionBlock = 10
	conf.ExecutionFollowDistance = 50
	params.OverrideBeaconConfig(conf)

	// Genesis not set
	followBlock := uint64(2000)
	blk, err := web3Service.determineEarliestVotingBlock(context.Background(), followBlock)
	require.NoError(t, err)
	assert.Equal(t, followBlock-conf.ExecutionFollowDistance, blk, "unexpected earliest voting block")

	// Genesis is set.

	numToForward := 1500
	// forward 1500 blocks
	for range numToForward {
		testAcc.Backend.Commit()
	}
	currTime := testAcc.Backend.Blockchain().CurrentHeader().Time
	now := time.Now()
	err = testAcc.Backend.AdjustTime(now.Sub(time.Unix(int64(currTime), 0)))
	require.NoError(t, err)
	testAcc.Backend.Commit()

	currTime = testAcc.Backend.Blockchain().CurrentHeader().Time
	web3Service.latestExecutionData.BlockHeight = testAcc.Backend.Blockchain().CurrentHeader().Number.Uint64()
	web3Service.latestExecutionData.BlockTime = testAcc.Backend.Blockchain().CurrentHeader().Time
	web3Service.chainStartData.GenesisTime = currTime

	// With a current slot of zero, only request follow_blocks behind.
	blk, err = web3Service.determineEarliestVotingBlock(context.Background(), followBlock)
	require.NoError(t, err)
	assert.Equal(t, followBlock-conf.ExecutionFollowDistance, blk, "unexpected earliest voting block")

}

func TestNewService_ExecutionHeaderRequLimit(t *testing.T) {
	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	beaconDB := dbutil.SetupDB(t)

	server, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	s1, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
	)
	require.NoError(t, err, "unable to setup web3 QRL execution chain service")
	assert.Equal(t, defaultExecutionHeaderReqLimit, s1.cfg.executionHeaderReqLimit, "default execution header request limit not set")
	s2, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
		WithExecutionHeaderRequestLimit(uint64(150)),
	)
	require.NoError(t, err, "unable to setup web3 QRL execution chain service")
	assert.Equal(t, uint64(150), s2.cfg.executionHeaderReqLimit, "unable to set executionHeaderRequestLimit")
}

type mockBSUpdater struct {
	lastBS clientstats.BeaconNodeStats
}

func (mbs *mockBSUpdater) Update(bs clientstats.BeaconNodeStats) {
	mbs.lastBS = bs
}

var _ BeaconNodeStatsUpdater = &mockBSUpdater{}

func Test_batchRequestHeaders_UnderflowChecks(t *testing.T) {
	srv := &Service{}
	start := uint64(101)
	end := uint64(100)
	_, err := srv.batchRequestHeaders(context.Background(), start, end)
	require.ErrorContains(t, "cannot be >", err)

	start = uint64(200)
	end = uint64(100)
	_, err = srv.batchRequestHeaders(context.Background(), start, end)
	require.ErrorContains(t, "cannot be >", err)
}

func TestService_EnsureConsistentExecutionChainData(t *testing.T) {
	beaconDB := dbutil.SetupDB(t)
	cache, err := depositcache.New()
	require.NoError(t, err)
	srv, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		srv.Stop()
	})
	s1, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDatabase(beaconDB),
		WithDepositCache(cache),
	)
	require.NoError(t, err)
	genState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	assert.NoError(t, genState.SetSlot(1000))

	require.NoError(t, s1.cfg.beaconDB.SaveGenesisData(context.Background(), genState))
	_, err = s1.validExecutionChainData(context.Background())
	require.NoError(t, err)

	executionData, err := s1.cfg.beaconDB.ExecutionChainData(context.Background())
	assert.NoError(t, err)

	assert.NotNil(t, executionData)
}

func TestService_InitializeCorrectly(t *testing.T) {
	beaconDB := dbutil.SetupDB(t)
	cache, err := depositcache.New()
	require.NoError(t, err)

	srv, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		srv.Stop()
	})
	s1, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDatabase(beaconDB),
		WithDepositCache(cache),
	)
	require.NoError(t, err)
	genState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	assert.NoError(t, genState.SetSlot(1000))

	require.NoError(t, s1.cfg.beaconDB.SaveGenesisData(context.Background(), genState))
	_, err = s1.validExecutionChainData(context.Background())
	require.NoError(t, err)

	executionData, err := s1.cfg.beaconDB.ExecutionChainData(context.Background())
	assert.NoError(t, err)

	assert.NoError(t, s1.initializeExecutionData(context.Background(), executionData))
	assert.Equal(t, int64(-1), s1.lastReceivedMerkleIndex, "received incorrect last received merkle index")
}

func TestService_EnsureValidExecutionChainData(t *testing.T) {
	beaconDB := dbutil.SetupDB(t)
	cache, err := depositcache.New()
	require.NoError(t, err)
	srv, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		srv.Stop()
	})
	s1, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDatabase(beaconDB),
		WithDepositCache(cache),
	)
	require.NoError(t, err)
	genState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	assert.NoError(t, genState.SetSlot(1000))

	require.NoError(t, s1.cfg.beaconDB.SaveGenesisData(context.Background(), genState))

	err = s1.cfg.beaconDB.SaveExecutionChainData(context.Background(), &qrysmpb.ExecutionChainData{
		ChainstartData:    &qrysmpb.ChainStartData{},
		DepositContainers: []*qrysmpb.DepositContainer{{Index: 1}},
	})
	require.NoError(t, err)
	_, err = s1.validExecutionChainData(context.Background())
	require.NoError(t, err)

	executionData, err := s1.cfg.beaconDB.ExecutionChainData(context.Background())
	assert.NoError(t, err)

	assert.NotNil(t, executionData)
	assert.Equal(t, 0, len(executionData.DepositContainers))
}

func TestService_ValidateDepositContainers(t *testing.T) {
	var tt = []struct {
		name        string
		ctrsFunc    func() []*qrysmpb.DepositContainer
		expectedRes bool
	}{
		{
			name: "zero containers",
			ctrsFunc: func() []*qrysmpb.DepositContainer {
				return make([]*qrysmpb.DepositContainer, 0)
			},
			expectedRes: true,
		},
		{
			name: "ordered containers",
			ctrsFunc: func() []*qrysmpb.DepositContainer {
				ctrs := make([]*qrysmpb.DepositContainer, 0)
				for i := range 10 {
					ctrs = append(ctrs, &qrysmpb.DepositContainer{Index: int64(i), ExecutionBlockHeight: uint64(i + 10)})
				}
				return ctrs
			},
			expectedRes: true,
		},
		{
			name: "0th container missing",
			ctrsFunc: func() []*qrysmpb.DepositContainer {
				ctrs := make([]*qrysmpb.DepositContainer, 0)
				for i := 1; i < 10; i++ {
					ctrs = append(ctrs, &qrysmpb.DepositContainer{Index: int64(i), ExecutionBlockHeight: uint64(i + 10)})
				}
				return ctrs
			},
			expectedRes: false,
		},
		{
			name: "skipped containers",
			ctrsFunc: func() []*qrysmpb.DepositContainer {
				ctrs := make([]*qrysmpb.DepositContainer, 0)
				for i := range 10 {
					if i == 5 || i == 7 {
						continue
					}
					ctrs = append(ctrs, &qrysmpb.DepositContainer{Index: int64(i), ExecutionBlockHeight: uint64(i + 10)})
				}
				return ctrs
			},
			expectedRes: false,
		},
	}

	for _, test := range tt {
		assert.Equal(t, test.expectedRes, validateDepositContainers(test.ctrsFunc()))
	}
}

func TestExecutionEndpoints(t *testing.T) {
	server, firstEndpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
	})
	endpoints := []string{firstEndpoint}

	testAcc, err := mock.Setup()
	require.NoError(t, err, "Unable to set up simulated backend")
	beaconDB := dbutil.SetupDB(t)

	mbs := &mockBSUpdater{}
	s1, err := NewService(context.Background(),
		WithHttpEndpoint(endpoints[0]),
		WithDepositContractAddress(testAcc.ContractAddr),
		WithDatabase(beaconDB),
		WithBeaconNodeStatsUpdater(mbs),
	)
	s1.cfg.beaconNodeStatsUpdater = mbs
	require.NoError(t, err)

	// Check default endpoint is set to current.
	assert.Equal(t, firstEndpoint, s1.ExecutionClientEndpoint(), "Unexpected http endpoint")
}

func TestService_CacheBlockHeaders(t *testing.T) {
	rClient := &slowRPCClient{limit: 1000}
	s := &Service{
		cfg:         &config{executionHeaderReqLimit: 1000},
		rpcClient:   rClient,
		headerCache: newHeaderCache(),
	}
	assert.NoError(t, s.cacheBlockHeaders(context.Background(), 1, 1000))
	assert.Equal(t, 1, rClient.numOfCalls)
	// Reset Num of Calls
	rClient.numOfCalls = 0
	// Increase header request limit to trigger the batch limiting
	// code path.
	s.cfg.executionHeaderReqLimit = 1001

	assert.NoError(t, s.cacheBlockHeaders(context.Background(), 1000, 3000))
	// The first 1001-header request fails, followed by four 500-header
	// requests and one final request for the inclusive end block 3000.
	assert.Equal(t, 6, rClient.numOfCalls)
}

func TestService_CacheBlockHeaders_RangeCoverage(t *testing.T) {
	for _, tt := range []struct {
		start, end uint64
		batchSize  uint64
		rpcLimit   int
	}{
		{0, 6, 3, 3},
		{2, 2, 3, 3},
		{0, 5, 4, 2},
		{1, 5, 4, 2},
	} {
		t.Run(fmt.Sprintf("%d-%d/batch=%d/limit=%d", tt.start, tt.end, tt.batchSize, tt.rpcLimit), func(t *testing.T) {
			s := &Service{
				cfg:       &config{executionHeaderReqLimit: tt.batchSize},
				rpcClient: &slowRPCClient{limit: tt.rpcLimit}, headerCache: newHeaderCache(),
			}
			require.NoError(t, s.cacheBlockHeaders(context.Background(), tt.start, tt.end))
			for height := tt.start; height <= tt.end; height++ {
				exists, _, err := s.headerCache.HeaderInfoByHeight(new(big.Int).SetUint64(height))
				require.NoError(t, err)
				assert.Equal(t, true, exists, "missing header %d", height)
			}
		})
	}
}

func TestWithExecutionHeaderRequestLimit_Zero(t *testing.T) {
	s := &Service{cfg: &config{executionHeaderReqLimit: defaultExecutionHeaderReqLimit}}
	require.ErrorContains(t, "greater than zero", WithExecutionHeaderRequestLimit(0)(s))
	require.Equal(t, defaultExecutionHeaderReqLimit, s.cfg.executionHeaderReqLimit)
}

func TestService_CacheBlockHeaders_SingleBlockTimeout(t *testing.T) {
	s := &Service{cfg: &config{executionHeaderReqLimit: 1}, rpcClient: &slowRPCClient{limit: 0}}
	require.ErrorIs(t, s.cacheBlockHeaders(context.Background(), 0, 0), errTimedOut)
}

// TODO(now.youtrack.cloud/issue/TQ-5)
/*
func TestService_FollowBlock(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig().Copy()
	conf.ExecutionFollowDistance = 2048
	conf.SecondsPerExecutionBlock = 14
	params.OverrideBeaconConfig(conf)

	followTime := params.BeaconConfig().ExecutionFollowDistance * params.BeaconConfig().SecondsPerExecutionBlock
	followTime += 10000
	bMap := make(map[uint64]*types.HeaderInfo)
	for i := uint64(3000); i > 0; i-- {
		h := &gqrltypes.Header{
			Number: big.NewInt(int64(i)),
			Time:   followTime + (i * 40),
		}
		bMap[i] = &types.HeaderInfo{
			Number: h.Number,
			Hash:   h.Hash(),
			Time:   h.Time,
		}
	}
	s := &Service{
		cfg:            &config{executionHeaderReqLimit: 1000},
		rpcClient:      &mockExecution.RPCClient{BlockNumMap: bMap},
		headerCache:    newHeaderCache(),
		latestExecutionData: &qrysmpb.LatestExecutionData{BlockTime: (3000 * 40) + followTime, BlockHeight: 3000},
	}
	h, err := s.followedBlockHeight(context.Background())
	assert.NoError(t, err)
	// With a much higher blocktime, the follow height is respectively shortened.
	assert.Equal(t, uint64(2283), h)
}
*/

type slowRPCClient struct {
	limit      int
	numOfCalls int
}

func (s *slowRPCClient) Close() {
	panic("implement me")
}

func (s *slowRPCClient) BatchCallContext(_ context.Context, b []rpc.BatchElem) error {
	s.numOfCalls++
	if len(b) > s.limit {
		return errTimedOut
	}
	for _, e := range b {
		num, err := hexutil.DecodeBig(e.Args[0].(string))
		if err != nil {
			return err
		}
		h := &gqrltypes.Header{Number: num}
		*e.Result.(*types.HeaderInfo) = types.HeaderInfo{Number: h.Number, Hash: h.Hash()}
	}
	return nil
}

func (s *slowRPCClient) CallContext(_ context.Context, _ any, _ string, _ ...any) error {
	panic("implement me")
}

func TestService_migrateOldDepositTree(t *testing.T) {
	beaconDB := dbutil.SetupDB(t)
	cache, err := depositcache.New()
	require.NoError(t, err)

	srv, endpoint, err := mockExecution.SetupRPCServer()
	require.NoError(t, err)
	t.Cleanup(func() {
		srv.Stop()
	})
	s, err := NewService(context.Background(),
		WithHttpEndpoint(endpoint),
		WithDatabase(beaconDB),
		WithDepositCache(cache),
	)
	require.NoError(t, err)
	executionData := &qrysmpb.ExecutionChainData{
		BeaconState: &qrysmpb.BeaconStateZond{
			ExecutionData: &qrysmpb.ExecutionData{
				DepositCount: 800,
			},
		},
		CurrentExecutionData: &qrysmpb.LatestExecutionData{
			BlockHeight: 100,
		},
	}

	totalDeposits := 1000
	input := bytesutil.ToBytes32([]byte("foo"))
	dt, err := trie.NewTrie(32)
	require.NoError(t, err)

	for i := range totalDeposits {
		err := dt.Insert(input[:], i)
		require.NoError(t, err)
	}
	executionData.Trie = dt.ToProto()

	err = s.migrateOldDepositTree(executionData)
	require.NoError(t, err)
	oldDepositTreeRoot, err := dt.HashTreeRoot()
	require.NoError(t, err)
	newDepositTreeRoot, err := s.depositTrie.HashTreeRoot()
	require.NoError(t, err)
	require.DeepEqual(t, oldDepositTreeRoot, newDepositTreeRoot)
}

func TestService_migrateOldDepositTree_RootValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		empty   bool
		corrupt bool
	}{
		{name: "populated"},
		{name: "empty", empty: true},
		{name: "mismatched root", corrupt: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldTree, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
			require.NoError(t, err)
			if !tc.empty {
				leaf := [32]byte{1}
				require.NoError(t, oldTree.Insert(leaf[:], 0))
			}
			data := &qrysmpb.ExecutionChainData{Trie: oldTree.ToProto()}
			if tc.corrupt {
				data.Trie.Layers[len(data.Trie.Layers)-1].Layer[0][0] ^= 1
			}
			original := depositsnapshot.NewDepositTree()
			s := &Service{depositTrie: original}
			err = s.migrateOldDepositTree(data)
			if tc.corrupt {
				require.ErrorContains(t, "mismatched deposit roots", err)
				require.Equal(t, original, s.depositTrie)
				return
			}
			require.NoError(t, err)
			require.Equal(t, oldTree.NumOfItems(), s.depositTrie.NumOfItems())
			want, err := oldTree.HashTreeRoot()
			require.NoError(t, err)
			got, err := s.depositTrie.HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}
