// Package p2p defines the network protocol implementation for QRL consensus
// used by beacon nodes, including peer discovery using discv5, gossip-sub
// using libp2p, and handing peer lifecycles + handshakes.
package p2p

import (
	"context"
	"crypto/ecdsa"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrl/p2p/qnode"
	"github.com/theQRL/go-qrl/p2p/qnr"
	"github.com/theQRL/qrysm/async"
	"github.com/theQRL/qrysm/beacon-chain/p2p/encoder"
	"github.com/theQRL/qrysm/beacon-chain/p2p/peers"
	"github.com/theQRL/qrysm/beacon-chain/p2p/peers/scorers"
	"github.com/theQRL/qrysm/beacon-chain/p2p/types"
	"github.com/theQRL/qrysm/config/params"
	leakybucket "github.com/theQRL/qrysm/container/leaky-bucket"
	qrysmnetwork "github.com/theQRL/qrysm/network"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/metadata"
	"github.com/theQRL/qrysm/runtime"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
)

var _ runtime.Service = (*Service)(nil)

// In the event that we are at our peer limit, we
// stop looking for new peers and instead poll
// for the current peer limit status for the time period
// defined below.
var pollingPeriod = 6 * time.Second

// When looking for new nodes, if not enough nodes are found,
// we stop after this amount of iterations.
var batchSize = 2_000

// Refresh rate of QNR set at twice per slot.
var refreshRate = slots.DivideSlotBy(2)

// maxBadResponses is the maximum number of bad responses from a peer before we stop talking to it.
const maxBadResponses = 5

// pubsubQueueSize is the size that we assign to our validation queue and outbound message queue for
// gossipsub.
const pubsubQueueSize = 600

// maxDialTimeout is the timeout for a single peer dial.
var maxDialTimeout = params.BeaconNetworkConfig().RespTimeout

// Service for managing peer to peer (p2p) networking.
type Service struct {
	started                  atomic.Bool
	isPreGenesis             bool
	pingMethod               func(ctx context.Context, id peer.ID) error
	pingMethodLock           sync.RWMutex
	cancel                   context.CancelFunc
	cfg                      *Config
	peers                    *peers.Status
	addrFilter               *multiaddr.Filters
	ipLimiter                *leakybucket.Collector
	privKey                  *ecdsa.PrivateKey
	metaData                 metadata.Metadata
	pubsub                   *pubsub.PubSub
	joinedTopics             map[string]*pubsub.Topic
	joinedTopicsLock         sync.Mutex
	subnetsLock              map[uint64]*sync.RWMutex
	subnetsLockLock          sync.Mutex // Lock access to subnetsLock
	initializationLock       sync.Mutex
	dv5Listener              Listener
	startupErr               error
	ctx                      context.Context
	host                     host.Host
	genesisTime              time.Time
	genesisValidatorsRoot    []byte
	activeValidatorCount     uint64
	activeValidatorCountLock sync.Mutex
	peerDisconnectionTime    *cache.Cache
}

// NewService initializes a new p2p service compatible with shared.Service interface. No
// connections are made until the Start function is called during the service registry startup.
func NewService(ctx context.Context, cfg *Config) (*Service, error) {
	var err error
	ctx, cancel := context.WithCancel(ctx)
	_ = cancel // govet fix for lost cancel. Cancel is handled in service.Stop().

	s := &Service{
		ctx:                   ctx,
		cancel:                cancel,
		cfg:                   cfg,
		isPreGenesis:          true,
		joinedTopics:          make(map[string]*pubsub.Topic, len(gossipTopicMappings)),
		subnetsLock:           make(map[uint64]*sync.RWMutex),
		peerDisconnectionTime: cache.New(1*time.Second, 1*time.Minute),
	}

	dv5Nodes := parseBootStrapAddrs(s.cfg.BootstrapNodeAddr)

	cfg.Discv5BootStrapAddr = dv5Nodes

	ipAddr := qrysmnetwork.IPAddr()
	s.privKey, err = privKey(s.cfg)
	if err != nil {
		log.WithError(err).Error("Failed to generate p2p private key")
		return nil, err
	}
	s.metaData, err = metaDataFromDB(ctx, s.cfg.DB)
	if err != nil {
		log.WithError(err).Error("Failed to create peer metadata")
		return nil, err
	}
	s.addrFilter, err = configureFilter(s.cfg)
	if err != nil {
		log.WithError(err).Error("Failed to create address filter")
		return nil, err
	}
	s.ipLimiter = leakybucket.NewCollector(ipLimit, ipBurst, 30*time.Second, true /* deleteEmptyBuckets */)

	opts := s.buildOptions(ipAddr, s.privKey)
	// Tighten mplex stream reset timeout before constructing the libp2p host.
	configureMplex()
	h, err := libp2p.New(opts...)
	if err != nil {
		log.WithError(err).Error("Failed to create p2p host")
		return nil, err
	}

	s.host = h
	// Gossipsub registration is done before we add in any new peers
	// due to libp2p's gossipsub implementation not taking into
	// account previously added peers when creating the gossipsub
	// object.
	psOpts := s.pubsubOptions()
	// Set the pubsub global parameters that we require.
	setPubSubParameters()
	// Reinitialize them in the event we are running a custom config.
	attestationSubnetCount = params.BeaconNetworkConfig().AttestationSubnetCount
	syncCommsSubnetCount = params.BeaconConfig().SyncCommitteeSubnetCount

	gs, err := pubsub.NewGossipSub(s.ctx, s.host, psOpts...)
	if err != nil {
		log.WithError(err).Error("Failed to start pubsub")
		return nil, err
	}
	s.pubsub = gs

	s.peers = peers.NewStatus(ctx, &peers.StatusConfig{
		PeerLimit: int(s.cfg.MaxPeers),
		ScorerParams: &scorers.Config{
			BadResponsesScorerConfig: &scorers.BadResponsesScorerConfig{
				Threshold:     maxBadResponses,
				DecayInterval: time.Hour,
			},
		},
	})

	// Initialize Data maps.
	types.InitializeDataMaps()

	return s, nil
}

// Start the p2p service.
func (s *Service) Start() {
	if s.Started() {
		log.Error("Attempted to start p2p service when it was already started")
		return
	}

	// Waits until the state is initialized via an event feed.
	// Used for fork-related data when connecting peers.
	s.awaitStateInitialized()
	s.isPreGenesis = false

	var relayNodes []string
	if s.cfg.RelayNodeAddr != "" {
		relayNodes = append(relayNodes, s.cfg.RelayNodeAddr)
		if err := dialRelayNode(s.ctx, s.host, s.cfg.RelayNodeAddr); err != nil {
			log.WithError(err).Errorf("Could not dial relay node")
		}
	}

	if !s.cfg.NoDiscovery {
		ipAddr := qrysmnetwork.IPAddr()
		listener, err := s.startDiscoveryV5(
			ipAddr,
			s.privKey,
		)
		if err != nil {
			log.WithError(err).Fatal("Failed to start discovery")
			s.startupErr = err
			return
		}
		err = s.connectToBootnodes()
		if err != nil {
			log.WithError(err).Error("Could not add bootnode to the exclusion list")
			s.startupErr = err
			return
		}
		s.dv5Listener = listener
		go s.listenForNewNodes()
	}

	s.started.Store(true)

	if len(s.cfg.StaticPeers) > 0 {
		addrs, err := PeersFromStringAddrs(s.cfg.StaticPeers)
		if err != nil {
			log.WithError(err).Error("Could not connect to static peer")
		}
		// Set trusted peers for those that are provided as static addresses.
		pids := peerIdsFromMultiAddrs(addrs)
		s.peers.SetTrustedPeers(pids)
		s.connectWithAllTrustedPeers(addrs)
	}
	// Initialize metadata according to the
	// current epoch.
	s.RefreshQNR()

	// Periodic functions.
	async.RunEvery(s.ctx, params.BeaconNetworkConfig().TtfbTimeout, func() {
		ensurePeerConnections(s.ctx, s.host, s.peers, relayNodes...)
	})
	async.RunEvery(s.ctx, 30*time.Minute, s.Peers().Prune)
	async.RunEvery(s.ctx, params.BeaconNetworkConfig().RespTimeout, s.updateMetrics)
	async.RunEvery(s.ctx, refreshRate, s.RefreshQNR)
	async.RunEvery(s.ctx, 1*time.Minute, func() {
		log.WithFields(logrus.Fields{
			"inbound":     len(s.peers.InboundConnected()),
			"outbound":    len(s.peers.OutboundConnected()),
			"activePeers": len(s.peers.Active()),
		}).Info("Peer summary")
	})

	multiAddrs := s.host.Network().ListenAddresses()
	logIPAddr(s.host.ID(), multiAddrs...)

	p2pHostAddress := s.cfg.HostAddress
	p2pTCPPort := s.cfg.TCPPort

	if p2pHostAddress != "" {
		logExternalIPAddr(s.host.ID(), p2pHostAddress, p2pTCPPort)
		verifyConnectivity(p2pHostAddress, p2pTCPPort, "tcp")
	}

	p2pHostDNS := s.cfg.HostDNS
	if p2pHostDNS != "" {
		logExternalDNSAddr(s.host.ID(), p2pHostDNS, p2pTCPPort)
	}
	go s.forkWatcher(params.BeaconConfig().SecondsPerSlot)
}

// Stop the p2p service and terminate all peer connections.
func (s *Service) Stop() error {
	defer s.cancel()
	s.started.Store(false)
	if s.dv5Listener != nil {
		s.dv5Listener.Close()
	}
	return nil
}

// Status of the p2p service. Will return an error if the service is considered unhealthy to
// indicate that this node should not serve traffic until the issue has been resolved.
func (s *Service) Status() error {
	if s.isPreGenesis {
		return nil
	}
	if !s.started.Load() {
		return errors.New("not running")
	}
	if s.startupErr != nil {
		return s.startupErr
	}
	if s.genesisTime.IsZero() {
		return errors.New("no genesis time set")
	}
	return nil
}

// Started returns true if the p2p service has successfully started.
func (s *Service) Started() bool {
	return s.started.Load()
}

// Encoding returns the configured networking encoding.
func (*Service) Encoding() encoder.NetworkEncoding {
	return &encoder.SszNetworkEncoder{}
}

// PubSub returns the p2p pubsub framework.
func (s *Service) PubSub() *pubsub.PubSub {
	return s.pubsub
}

// Host returns the currently running libp2p
// host of the service.
func (s *Service) Host() host.Host {
	return s.host
}

// SetStreamHandler sets the protocol handler on the p2p host multiplexer.
// This method is a pass through to libp2pcore.Host.SetStreamHandler.
func (s *Service) SetStreamHandler(topic string, handler network.StreamHandler) {
	s.host.SetStreamHandler(protocol.ID(topic), handler)
}

// PeerID returns the Peer ID of the local peer.
func (s *Service) PeerID() peer.ID {
	return s.host.ID()
}

// Disconnect from a peer.
func (s *Service) Disconnect(pid peer.ID) error {
	return s.host.Network().ClosePeer(pid)
}

// Connect to a specific peer.
func (s *Service) Connect(pi peer.AddrInfo) error {
	return s.host.Connect(s.ctx, pi)
}

// Peers returns the peer status interface.
func (s *Service) Peers() *peers.Status {
	return s.peers
}

// QNR returns the local node's current QNR.
func (s *Service) QNR() *qnr.Record {
	if s.dv5Listener == nil {
		return nil
	}
	return s.dv5Listener.Self().Record()
}

// DiscoveryAddresses represents our qnr addresses as multiaddresses.
func (s *Service) DiscoveryAddresses() ([]multiaddr.Multiaddr, error) {
	if s.dv5Listener == nil {
		return nil, nil
	}
	return convertToUdpMultiAddr(s.dv5Listener.Self())
}

// Metadata returns a copy of the peer's metadata.
func (s *Service) Metadata() metadata.Metadata {
	return s.metaData.Copy()
}

// MetadataSeq returns the metadata sequence number.
func (s *Service) MetadataSeq() uint64 {
	return s.metaData.SequenceNumber()
}

// AddPingMethod adds the metadata ping rpc method to the p2p service, so that it can
// be used to refresh QNR.
func (s *Service) AddPingMethod(reqFunc func(ctx context.Context, id peer.ID) error) {
	s.pingMethodLock.Lock()
	s.pingMethod = reqFunc
	s.pingMethodLock.Unlock()
}

func (s *Service) pingPeers() {
	s.pingMethodLock.RLock()
	defer s.pingMethodLock.RUnlock()
	if s.pingMethod == nil {
		return
	}
	for _, pid := range s.peers.Connected() {
		go func(id peer.ID) {
			if err := s.pingMethod(s.ctx, id); err != nil {
				log.WithField("peer", id).WithError(err).Debug("Failed to ping peer")
			}
		}(pid)
	}
}

// Waits for the beacon state to be initialized, important
// for initializing the p2p service as p2p needs to be aware
// of genesis information for peering.
func (s *Service) awaitStateInitialized() {
	s.initializationLock.Lock()
	defer s.initializationLock.Unlock()
	if s.isInitialized() {
		return
	}
	clock, err := s.cfg.ClockWaiter.WaitForClock(s.ctx)
	if err != nil {
		log.WithError(err).Fatal("Failed to receive initial genesis data")
	}
	s.genesisTime = clock.GenesisTime()
	gvr := clock.GenesisValidatorsRoot()
	s.genesisValidatorsRoot = gvr[:]
	_, err = s.currentForkDigest() // initialize fork digest cache
	if err != nil {
		log.WithError(err).Error("Could not initialize fork digest")
	}
}

func (s *Service) connectWithAllTrustedPeers(multiAddrs []multiaddr.Multiaddr) {
	addrInfos, err := peer.AddrInfosFromP2pAddrs(multiAddrs...)
	if err != nil {
		log.WithError(err).Error("Could not convert to peer address info's from multiaddresses")
		return
	}
	for _, info := range addrInfos {
		// add peer into peer status
		s.peers.Add(nil, info.ID, info.Addrs[0], network.DirUnknown)
		// make each dial non-blocking
		go func(info peer.AddrInfo) {
			if err := s.connectWithPeer(s.ctx, info); err != nil {
				log.WithError(err).Debugf("Could not connect with trusted peer %s", info.String())
			}
		}(info)
	}
}

func (s *Service) connectWithAllPeers(multiAddrs []multiaddr.Multiaddr) {
	addrInfos, err := peer.AddrInfosFromP2pAddrs(multiAddrs...)
	if err != nil {
		log.WithError(err).Error("Could not convert to peer address info's from multiaddresses")
		return
	}
	for _, info := range addrInfos {
		// make each dial non-blocking
		go func(info peer.AddrInfo) {
			if err := s.connectWithPeer(s.ctx, info); err != nil {
				log.WithError(err).Debugf("Could not connect with peer %s", info.String())
			}
		}(info)
	}
}

func (s *Service) connectWithPeer(ctx context.Context, info peer.AddrInfo) error {
	ctx, span := trace.StartSpan(ctx, "p2p.connectWithPeer")
	defer span.End()

	pid := info.ID
	if pid == s.host.ID() {
		return nil
	}
	if s.Peers().IsBad(pid) {
		return errors.New("refused to connect to bad peer")
	}

	ctx, cancel := context.WithTimeout(ctx, maxDialTimeout)
	defer cancel()

	if err := s.host.Connect(ctx, info); err != nil {
		s.downscorePeer(pid, "connectionError")
		return errors.Wrap(err, "peer connect")
	}
	return nil
}

func (s *Service) connectToBootnodes() error {
	nodes := make([]*qnode.Node, 0, len(s.cfg.Discv5BootStrapAddr))
	for _, addr := range s.cfg.Discv5BootStrapAddr {
		bootNode, err := qnode.Parse(qnode.ValidSchemes, addr)
		if err != nil {
			return err
		}
		// do not dial bootnodes with their tcp ports not set
		if err := bootNode.Record().Load(qnr.WithEntry("tcp", new(qnr.TCP))); err != nil {
			if !qnr.IsNotFound(err) {
				log.WithError(err).Error("Could not retrieve tcp port")
			}
			continue
		}
		nodes = append(nodes, bootNode)
	}
	multiAddresses := convertToMultiAddr(nodes)
	s.connectWithAllPeers(multiAddresses)
	return nil
}

// Returns true if the service is aware of the genesis time and genesis validators root. This is
// required for discovery and pubsub validation.
func (s *Service) isInitialized() bool {
	return !s.genesisTime.IsZero() && len(s.genesisValidatorsRoot) == 32
}

// downscorePeer increments the bad responses counter for the peer and emits a debug log
// recording the new score. Use a stable, descriptive `reason` so the log is greppable.
func (s *Service) downscorePeer(peerID peer.ID, reason string) {
	newScore := s.Peers().Scorers().BadResponsesScorer().Increment(peerID)
	log.WithFields(logrus.Fields{
		"peerID":   peerID,
		"reason":   reason,
		"newScore": newScore,
	}).Debug("Downscore peer")
}

// wasDisconnectedTooRecently returns a non-nil error if a disconnect from the given peer was
// recorded within the last second. libp2p has been observed to fire ConnectedF immediately after
// DisconnectedF for the same outbound peer; this guard lets callers skip the redundant connect.
func (s *Service) wasDisconnectedTooRecently(peerID peer.ID) error {
	const disconnectionDurationThreshold = 1 * time.Second

	if s.peerDisconnectionTime == nil {
		return nil
	}

	peerDisconnectionTimeObj, ok := s.peerDisconnectionTime.Get(peerID.String())
	if !ok {
		return nil
	}

	peerDisconnectionTime, ok := peerDisconnectionTimeObj.(time.Time)
	if !ok {
		return errors.New("invalid peer disconnection time type")
	}

	timeSinceDisconnection := time.Since(peerDisconnectionTime)
	if timeSinceDisconnection < disconnectionDurationThreshold {
		return errors.Errorf("peer %s was disconnected too recently: %s", peerID, timeSinceDisconnection)
	}

	return nil
}

// recordPeerDisconnection stores the current time as the most recent disconnection for the peer.
// Returns an error iff the cache already had a fresh entry for the peer (i.e. DisconnectedF fired
// twice in quick succession, which is a libp2p quirk).
func (s *Service) recordPeerDisconnection(peerID peer.ID) error {
	if s.peerDisconnectionTime == nil {
		return nil
	}
	return s.peerDisconnectionTime.Add(peerID.String(), time.Now(), cache.DefaultExpiration)
}
