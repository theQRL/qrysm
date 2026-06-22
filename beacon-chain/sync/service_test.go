package sync

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
	gcache "github.com/patrickmn/go-cache"
	"github.com/theQRL/go-qrl/p2p/qnr"
	"github.com/theQRL/qrysm/async/abool"
	mockChain "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	dbTest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/beacon-chain/p2p/peers"
	p2ptest "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	p2ptypes "github.com/theQRL/qrysm/beacon-chain/p2p/types"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	leakybucket "github.com/theQRL/qrysm/container/leaky-bucket"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestService_StatusZeroEpoch(t *testing.T) {
	bState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Slot: 0})
	require.NoError(t, err)
	chain := &mockChain.ChainService{
		Genesis: time.Now(),
		State:   bState,
	}
	r := &Service{
		cfg: &config{
			p2p:         p2ptest.NewTestP2P(t),
			initialSync: new(mockSync.Sync),
			chain:       chain,
			clock:       startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
		},
		chainStarted: abool.New(),
	}
	r.chainStarted.Set()

	assert.NoError(t, r.Status(), "Wanted non failing status")
}

func TestSyncHandlers_WaitToSync(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	gs := startup.NewClockSynchronizer()
	r := Service{
		ctx: context.Background(),
		cfg: &config{
			p2p:         p2p,
			chain:       chainService,
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		chainStarted: abool.New(),
		clockWaiter:  gs,
	}

	topic := "/consensus/%x/beacon_block"
	go r.registerHandlers()
	go r.waitForChainStart()
	time.Sleep(100 * time.Millisecond)

	var vr [32]byte
	require.NoError(t, gs.SetClock(startup.NewClock(time.Now(), vr)))
	b := []byte("sk")
	b48 := bytesutil.ToBytes48(b)
	sk, err := ml_dsa_87.SecretKeyFromSeed(b48[:])
	require.NoError(t, err)

	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = util.Random32Bytes(t)
	msg.Signature = sk.Sign([]byte("data")).Marshal()
	p2p.ReceivePubSub(topic, msg)
	// wait for chainstart to be sent
	time.Sleep(400 * time.Millisecond)
	require.Equal(t, true, r.chainStarted.IsSet(), "Did not receive chain start event.")
}

func TestSyncHandlers_WaitForChainStart(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	gs := startup.NewClockSynchronizer()
	r := Service{
		ctx: context.Background(),
		cfg: &config{
			p2p:         p2p,
			chain:       chainService,
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		chainStarted:        abool.New(),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		clockWaiter:         gs,
	}

	go r.registerHandlers()
	var vr [32]byte
	require.NoError(t, gs.SetClock(startup.NewClock(time.Now(), vr)))
	r.waitForChainStart()

	require.Equal(t, true, r.chainStarted.IsSet(), "Did not receive chain start event.")
}

func TestSyncHandlers_WaitTillSynced(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gs := startup.NewClockSynchronizer()
	r := Service{
		ctx: ctx,
		cfg: &config{
			p2p:           p2p,
			beaconDB:      dbTest.SetupDB(t),
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
			initialSync:   &mockSync.Sync{IsSyncing: false},
		},
		chainStarted:        abool.New(),
		subHandler:          newSubTopicHandler(),
		clockWaiter:         gs,
		initialSyncComplete: make(chan struct{}),
	}
	r.initCaches()

	syncCompleteCh := make(chan bool)
	go func() {
		r.registerHandlers()
		syncCompleteCh <- true
	}()
	var vr [32]byte
	require.NoError(t, gs.SetClock(startup.NewClock(time.Now(), vr)))
	r.waitForChainStart()
	require.Equal(t, true, r.chainStarted.IsSet(), "Did not receive chain start event.")

	blockChan := make(chan *feed.Event, 1)
	sub := r.cfg.blockNotifier.BlockFeed().Subscribe(blockChan)
	defer sub.Unsubscribe()

	b := []byte("sk")
	b48 := bytesutil.ToBytes48(b)
	sk, err := ml_dsa_87.SecretKeyFromSeed(b48[:])
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = util.Random32Bytes(t)
	msg.Signature = sk.Sign([]byte("data")).Marshal()
	p2p.Digest, err = r.currentForkDigest()
	require.NoError(t, err)

	// Save block into DB so that validateBeaconBlockPubSub() process gets short cut.
	util.SaveBlock(t, ctx, r.cfg.beaconDB, msg)

	topic := "/consensus/%x/beacon_block"
	p2p.ReceivePubSub(topic, msg)
	assert.Equal(t, 0, len(blockChan), "block was received by sync service despite not being fully synced")
	close(r.initialSyncComplete)
	<-syncCompleteCh
	p2p.ReceivePubSub(topic, msg)

	select {
	case <-blockChan:
	case <-ctx.Done():
	}
	assert.NoError(t, ctx.Err())
}

func TestSyncService_StopCleanly(t *testing.T) {
	p2p := p2ptest.NewTestP2P(t)
	chainService := &mockChain.ChainService{
		Genesis:        time.Now(),
		ValidatorsRoot: [32]byte{'A'},
	}
	ctx, cancel := context.WithCancel(context.Background())
	gs := startup.NewClockSynchronizer()
	r := Service{
		ctx:    ctx,
		cancel: cancel,
		cfg: &config{
			p2p:         p2p,
			chain:       chainService,
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		chainStarted:        abool.New(),
		subHandler:          newSubTopicHandler(),
		clockWaiter:         gs,
		initialSyncComplete: make(chan struct{}),
	}

	go r.registerHandlers()
	var vr [32]byte
	require.NoError(t, gs.SetClock(startup.NewClock(time.Now(), vr)))
	r.waitForChainStart()

	var err error
	p2p.Digest, err = r.currentForkDigest()
	require.NoError(t, err)

	// wait for chainstart to be sent
	time.Sleep(2 * time.Second)
	require.Equal(t, true, r.chainStarted.IsSet(), "Did not receive chain start event.")

	close(r.initialSyncComplete)
	time.Sleep(1 * time.Second)

	require.NotEqual(t, 0, len(r.cfg.p2p.PubSub().GetTopics()))
	require.NotEqual(t, 0, len(r.cfg.p2p.Host().Mux().Protocols()))

	// Both pubsub and rpc topcis should be unsubscribed.
	require.NoError(t, r.Stop())

	// Sleep to allow pubsub topics to be deregistered.
	time.Sleep(1 * time.Second)
	require.Equal(t, 0, len(r.cfg.p2p.PubSub().GetTopics()))
	require.Equal(t, 0, len(r.cfg.p2p.Host().Mux().Protocols()))
}

// stopServiceWithPeer wires a minimal *Service whose Stop() loop will iterate
// over `connectedPeers` (each marked PeerConnected on a host connected to the
// service's p2p). Returns the service ready to call Stop().
func stopServiceWithConnectedPeers(t *testing.T, p1 *p2ptest.TestP2P, peerHosts ...*p2ptest.TestP2P) *Service {
	t.Helper()
	for _, ph := range peerHosts {
		p1.Connect(ph)
		p1.Peers().Add(new(qnr.Record), ph.PeerID(), nil, network.DirOutbound)
		p1.Peers().SetConnectionState(ph.PeerID(), peers.PeerConnected)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Service{
		ctx:    ctx,
		cancel: cancel,
		cfg: &config{
			p2p:         p1,
			chain:       &mockChain.ChainService{Genesis: time.Now(), ValidatorsRoot: [32]byte{}},
			initialSync: &mockSync.Sync{IsSyncing: false},
		},
		rateLimiter: newRateLimiter(p1),
	}
	pcl := protocol.ID("/consensus/beacon_chain/req/goodbye/1/ssz_snappy")
	r.rateLimiter.limiterMap[string(pcl)] = leakybucket.NewCollector(1, 1, time.Second, false)
	return r
}

// TestSyncService_Stop_SendsGoodbyeMessages is the regression test for the
// per-peer parallel goodbye dispatch (upstream PR #15542): every connected peer
// must receive a goodbye message with the ClientShutdown code when Stop() is
// called.
func TestSyncService_Stop_SendsGoodbyeMessages(t *testing.T) {
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	r := stopServiceWithConnectedPeers(t, p1, p2)

	pcl := protocol.ID("/consensus/beacon_chain/req/goodbye/1/ssz_snappy")
	var wg sync.WaitGroup
	wg.Add(1)
	var receivedCode primitives.SSZUint64
	p2.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
		defer wg.Done()
		assert.NoError(t, p1.Encoding().DecodeWithMaxLength(stream, &receivedCode))
		assert.NoError(t, stream.Close())
	})

	require.NoError(t, r.Stop())
	if util.WaitTimeout(&wg, 2*time.Second) {
		t.Fatal("did not receive goodbye stream within 2s")
	}
	require.Equal(t, p2ptypes.GoodbyeCodeClientShutdown, receivedCode)
}

// TestSyncService_Stop_TimeoutHandling is the regression test for the
// goodbyeShutdownTimeout bound (upstream PR #15542): Stop() must return within
// the timeout even when peers never respond to the goodbye RPC. Without the
// fix, this would block until each per-peer respTimeout fires sequentially.
func TestSyncService_Stop_TimeoutHandling(t *testing.T) {
	saved := goodbyeShutdownTimeout
	goodbyeShutdownTimeout = 500 * time.Millisecond
	defer func() { goodbyeShutdownTimeout = saved }()

	p1 := p2ptest.NewTestP2P(t)
	const numPeers = 5
	hosts := make([]*p2ptest.TestP2P, numPeers)
	for i := range hosts {
		hosts[i] = p2ptest.NewTestP2P(t)
	}
	r := stopServiceWithConnectedPeers(t, p1, hosts...)

	// Each peer accepts the stream but blocks indefinitely.
	pcl := protocol.ID("/consensus/beacon_chain/req/goodbye/1/ssz_snappy")
	block := make(chan struct{})
	defer close(block)
	for _, h := range hosts {
		h.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
			<-block
		})
	}

	start := time.Now()
	require.NoError(t, r.Stop())
	elapsed := time.Since(start)
	// Allow generous slack for stream setup but well below the multi-minute
	// hang the bug produced (sequential N * respTimeout).
	require.Equal(t, true, elapsed < 5*time.Second,
		"Stop() did not honor goodbyeShutdownTimeout, took %s", elapsed)
}

// TestSyncService_Stop_ConcurrentGoodbyeMessages verifies goodbyes are
// dispatched in parallel (upstream PR #15542). With each handler taking
// perHandlerLatency, sequential dispatch would be N * perHandlerLatency;
// parallel dispatch should complete in roughly perHandlerLatency.
func TestSyncService_Stop_ConcurrentGoodbyeMessages(t *testing.T) {
	const numPeers = 5
	const perHandlerLatency = 300 * time.Millisecond

	p1 := p2ptest.NewTestP2P(t)
	hosts := make([]*p2ptest.TestP2P, numPeers)
	for i := range hosts {
		hosts[i] = p2ptest.NewTestP2P(t)
	}
	r := stopServiceWithConnectedPeers(t, p1, hosts...)

	pcl := protocol.ID("/consensus/beacon_chain/req/goodbye/1/ssz_snappy")
	var received atomic.Int64
	for _, h := range hosts {
		h.BHost.SetStreamHandler(pcl, func(stream network.Stream) {
			out := new(primitives.SSZUint64)
			_ = p1.Encoding().DecodeWithMaxLength(stream, out)
			time.Sleep(perHandlerLatency)
			received.Add(1)
			_ = stream.Close()
		})
	}

	start := time.Now()
	require.NoError(t, r.Stop())
	elapsed := time.Since(start)
	// Sequential lower bound is numPeers * perHandlerLatency. Cap parallel
	// expectation generously below that to avoid CI flakiness while still
	// proving concurrency.
	upperBound := time.Duration(float64(numPeers*perHandlerLatency) * 0.6)
	require.Equal(t, true, elapsed < upperBound,
		"goodbyes did not run in parallel: elapsed=%s, sequential lower bound=%s",
		elapsed, numPeers*perHandlerLatency)
}
