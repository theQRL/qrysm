package execution

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	qrl "github.com/theQRL/go-qrl"
	"github.com/theQRL/go-qrl/accounts/abi/bind/backends"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/common/hexutil"
	gqrltypes "github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/go-qrl/rpc"
	"github.com/theQRL/qrysm/async/event"
	"github.com/theQRL/qrysm/beacon-chain/cache/depositcache"
	dbutil "github.com/theQRL/qrysm/beacon-chain/db/testing"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/execution/types"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/container/trie"
	contracts "github.com/theQRL/qrysm/contracts/deposit"
	"github.com/theQRL/qrysm/contracts/deposit/mock"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/monitoring/clientstats"
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
		{Index: 0, ExecutionBlockHeight: 2, Deposit: &qrysmpb.Deposit{Proof: [][]byte{[]byte("A")}, Data: &qrysmpb.Deposit_Data{PublicKey: []byte{}}}},
		{Index: 1, ExecutionBlockHeight: 4, Deposit: &qrysmpb.Deposit{Proof: [][]byte{[]byte("B")}, Data: &qrysmpb.Deposit_Data{PublicKey: []byte{}}}},
		{Index: 2, ExecutionBlockHeight: 6, Deposit: &qrysmpb.Deposit{Proof: [][]byte{[]byte("c")}, Data: &qrysmpb.Deposit_Data{PublicKey: []byte{}}}},
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

func TestInitDepositCacheWithFinalization_OK(t *testing.T) {
	ctrs := []*qrysmpb.DepositContainer{
		{
			Index:                0,
			ExecutionBlockHeight: 2,
			Deposit: &qrysmpb.Deposit{
				Data: &qrysmpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte{0}, 2592),
					WithdrawalCredentials: make([]byte, 64),
					Signature:             make([]byte, 4627),
				},
			},
		},
		{
			Index:                1,
			ExecutionBlockHeight: 4,
			Deposit: &qrysmpb.Deposit{
				Data: &qrysmpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte{1}, 2592),
					WithdrawalCredentials: make([]byte, 64),
					Signature:             make([]byte, 4627),
				},
			},
		},
		{
			Index:                2,
			ExecutionBlockHeight: 6,
			Deposit: &qrysmpb.Deposit{
				Data: &qrysmpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte{2}, 2592),
					WithdrawalCredentials: make([]byte, 64),
					Signature:             make([]byte, 4627),
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
	_, err := srv.batchRequestHeaders(start, end)
	require.ErrorContains(t, "cannot be >", err)

	start = uint64(200)
	end = uint64(100)
	_, err = srv.batchRequestHeaders(start, end)
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
	assert.NoError(t, s.cacheBlockHeaders(1, 1000))
	assert.Equal(t, 1, rClient.numOfCalls)
	// Reset Num of Calls
	rClient.numOfCalls = 0
	// Increase header request limit to trigger the batch limiting
	// code path.
	s.cfg.executionHeaderReqLimit = 1001

	assert.NoError(t, s.cacheBlockHeaders(1000, 3000))
	// 1000 - 2000 would be 1001 headers which is higher than our request limit, it
	// is then reduced to 500 and tried again.
	assert.Equal(t, 5, rClient.numOfCalls)
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

func (s *slowRPCClient) BatchCall(b []rpc.BatchElem) error {
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
