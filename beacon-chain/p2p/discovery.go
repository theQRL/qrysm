package p2p

import (
	"bytes"
	"crypto/ecdsa"
	"net"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrl/p2p/discover"
	"github.com/theQRL/go-qrl/p2p/qnode"
	"github.com/theQRL/go-qrl/p2p/qnr"
	"github.com/theQRL/qrysm/cmd/beacon-chain/flags"
	ecdsaqrysm "github.com/theQRL/qrysm/crypto/ecdsa"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/time/slots"
)

// Listener defines the discovery V5 network interface that is used
// to communicate with other peers.
type Listener interface {
	Self() *qnode.Node
	Close()
	Lookup(qnode.ID) []*qnode.Node
	Resolve(*qnode.Node) *qnode.Node
	RandomNodes() qnode.Iterator
	Ping(*qnode.Node) error
	RequestQNR(*qnode.Node) (*qnode.Node, error)
	LocalNode() *qnode.LocalNode
}

// RefreshQNR uses an epoch to refresh the qnr entry for our node
// with the tracked committee ids for the epoch, allowing our node
// to be dynamically discoverable by others given our tracked committee ids.
func (s *Service) RefreshQNR() {
	if !s.isInitialized() {
		return
	}

	currEpoch := slots.ToEpoch(slots.CurrentSlot(uint64(s.genesisTime.Unix())))
	bitV, bitS := advertisedSubnetBitfields(currEpoch)
	currentMetaBitV, currentMetaBitS := s.metadataBitfields()
	metadataUnchanged := bytes.Equal(bitV, currentMetaBitV) && bytes.Equal(bitS, currentMetaBitS) &&
		s.metaData != nil && !s.metaData.IsNil() && s.metaData.Version() == version.Zond

	qnrUnchanged := true
	if s.dv5Listener != nil {
		currentBitV, err := attBitvector(s.dv5Listener.Self().Record())
		if err != nil {
			log.WithError(err).Error("Could not retrieve att bitfield")
			return
		}
		currentBitS, err := syncBitvector(s.dv5Listener.Self().Record())
		if err != nil {
			log.WithError(err).Error("Could not retrieve sync bitfield")
			return
		}
		qnrUnchanged = bytes.Equal(bitV, currentBitV) && bytes.Equal(bitS, currentBitS)
	}

	if metadataUnchanged && qnrUnchanged {
		return
	}

	// Don't return on error: the failure is from saving the metadata sequence number,
	// which doesn't justify dropping the in-memory metadata update.
	if err := s.updateSubnetRecordWithMetadata(bitV, bitS); err != nil {
		log.WithError(err).Error("Failed to update subnet record with metadata")
	}

	// ping all peers to inform them of new metadata
	s.pingPeers()
}

// listen for new nodes watches for new nodes in the network and adds them to the peerstore.
func (s *Service) listenForNewNodes() {
	const minLogInterval = 1 * time.Minute

	peersSummary := func(threshold uint) (uint, uint) {
		// Retrieve how many active peers we have.
		activePeers := s.Peers().Active()
		activePeerCount := uint(len(activePeers))

		// Compute how many peers we are missing to reach the threshold.
		if activePeerCount >= threshold {
			return activePeerCount, 0
		}

		missingPeerCount := threshold - activePeerCount

		return activePeerCount, missingPeerCount
	}

	var lastLogTime time.Time

	iterator := s.dv5Listener.RandomNodes()
	defer iterator.Close()

	for {
		// Exit if service's context is canceled
		if s.ctx.Err() != nil {
			break
		}
		if s.isPeerAtLimit(false /* inbound */) {
			// Pause the main loop for a period to stop looking
			// for new peers.
			log.Trace("Not looking for peers, at peer limit")
			time.Sleep(pollingPeriod)
			continue
		}

		// Compute the number of new peers we want to dial.
		activePeerCount, missingPeerCount := peersSummary(s.cfg.MaxPeers)

		fields := logrus.Fields{
			"currentPeerCount": activePeerCount,
			"targetPeerCount":  s.cfg.MaxPeers,
		}

		if missingPeerCount == 0 {
			log.Trace("Not looking for peers, at peer limit")
			time.Sleep(pollingPeriod)
			continue
		}

		if time.Since(lastLogTime) > minLogInterval {
			lastLogTime = time.Now()
			log.WithFields(fields).Debug("Searching for new active peers")
		}

		// Restrict dials if limit is applied.
		if flags.MaxDialIsActive() {
			maxConcurrentDials := uint(flags.Get().MaxConcurrentDials)
			missingPeerCount = min(missingPeerCount, maxConcurrentDials)
		}

		// Search for new peers.
		wantedNodes := searchForPeers(iterator, batchSize, missingPeerCount, s.filterPeer)

		wg := new(sync.WaitGroup)
		for i := 0; i < len(wantedNodes); i++ {
			node := wantedNodes[i]
			peerInfo, _, err := convertToAddrInfo(node)
			if err != nil {
				log.WithError(err).Error("Could not convert to peer info")
				continue
			}

			if peerInfo == nil {
				continue
			}

			// Make sure that peer is not dialed too often, for each connection attempt there's a backoff period.
			s.Peers().RandomizeBackOff(peerInfo.ID)
			wg.Add(1)
			go func(info *peer.AddrInfo) {
				if err := s.connectWithPeer(s.ctx, *info); err != nil {
					log.WithError(err).Debugf("Could not connect with new peer %s", info.String())
				}
				wg.Done()
			}(peerInfo)
		}
		wg.Wait()
	}
}

func (s *Service) createListener(
	ipAddr net.IP,
	privKey *ecdsa.PrivateKey,
) (*discover.UDPv5, error) {
	// BindIP is used to specify the ip
	// on which we will bind our listener on
	// by default we will listen to all interfaces.
	var bindIP net.IP
	switch udpVersionFromIP(ipAddr) {
	case "udp4":
		bindIP = net.IPv4zero
	case "udp6":
		bindIP = net.IPv6zero
	default:
		return nil, errors.New("invalid ip provided")
	}

	// If local ip is specified then use that instead.
	if s.cfg.LocalIP != "" {
		ipAddr = net.ParseIP(s.cfg.LocalIP)
		if ipAddr == nil {
			return nil, errors.New("invalid local ip provided")
		}
		bindIP = ipAddr
	}
	udpAddr := &net.UDPAddr{
		IP:   bindIP,
		Port: int(s.cfg.UDPPort),
	}
	// Listen to all network interfaces
	// for both ip protocols.
	networkVersion := "udp"
	conn, err := net.ListenUDP(networkVersion, udpAddr)
	if err != nil {
		return nil, errors.Wrap(err, "could not listen to UDP")
	}

	localNode, err := s.createLocalNode(
		privKey,
		ipAddr,
		int(s.cfg.UDPPort),
		int(s.cfg.TCPPort),
	)
	if err != nil {
		return nil, errors.Wrap(err, "could not create local node")
	}
	if s.cfg.HostAddress != "" {
		hostIP := net.ParseIP(s.cfg.HostAddress)
		if hostIP.To4() == nil && hostIP.To16() == nil {
			log.Errorf("Invalid host address given: %s", hostIP.String())
		} else {
			localNode.SetFallbackIP(hostIP)
			localNode.SetStaticIP(hostIP)
		}
	}
	if s.cfg.HostDNS != "" {
		host := s.cfg.HostDNS
		ips, err := net.LookupIP(host)
		if err != nil {
			return nil, errors.Wrap(err, "could not resolve host address")
		}
		if len(ips) > 0 {
			// Use first IP returned from the
			// resolver.
			firstIP := ips[0]
			localNode.SetFallbackIP(firstIP)
		}
	}
	dv5Cfg := discover.Config{
		PrivateKey: privKey,
	}
	dv5Cfg.Bootnodes = []*qnode.Node{}
	for _, addr := range s.cfg.Discv5BootStrapAddr {
		bootNode, err := qnode.Parse(qnode.ValidSchemes, addr)
		if err != nil {
			return nil, errors.Wrap(err, "could not bootstrap addr")
		}
		dv5Cfg.Bootnodes = append(dv5Cfg.Bootnodes, bootNode)
	}

	listener, err := discover.ListenV5(conn, localNode, dv5Cfg)
	if err != nil {
		return nil, errors.Wrap(err, "could not listen to discV5")
	}
	return listener, nil
}

func (s *Service) createLocalNode(
	privKey *ecdsa.PrivateKey,
	ipAddr net.IP,
	udpPort, tcpPort int,
) (*qnode.LocalNode, error) {
	db, err := qnode.OpenDB("")
	if err != nil {
		return nil, errors.Wrap(err, "could not open node's peer database")
	}
	localNode := qnode.NewLocalNode(db, privKey)

	ipEntry := qnr.IP(ipAddr)
	udpEntry := qnr.UDP(udpPort)
	tcpEntry := qnr.TCP(tcpPort)
	localNode.Set(ipEntry)
	localNode.Set(udpEntry)
	localNode.Set(tcpEntry)
	localNode.SetFallbackIP(ipAddr)
	localNode.SetFallbackUDP(udpPort)

	localNode, err = addForkEntry(localNode, s.genesisTime, s.genesisValidatorsRoot)
	if err != nil {
		return nil, errors.Wrap(err, "could not add consensus fork version entry to qnr")
	}
	localNode = initializeAttSubnets(localNode)
	return initializeSyncCommSubnets(localNode), nil
}

func (s *Service) startDiscoveryV5(
	addr net.IP,
	privKey *ecdsa.PrivateKey,
) (*discover.UDPv5, error) {
	listener, err := s.createListener(addr, privKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not create listener")
	}
	record := listener.Self()
	log.WithField("QNR", record.String()).Info("Started discovery v5")
	return listener, nil
}

// filterPeer validates each node that we retrieve from our dht. We
// try to ascertain that the peer can be a valid protocol peer.
// Validity Conditions:
//  1. The local node is still actively looking for peers to
//     connect to.
//  2. Peer has a valid IP and TCP port set in their qnr.
//  3. Peer hasn't been marked as 'bad'
//  4. Peer is not currently active or connected.
//  5. Peer is ready to receive incoming connections.
//  6. Peer's fork digest in their QNR matches that of
//     our localnodes.
func (s *Service) filterPeer(node *qnode.Node) bool {
	// Ignore nil node entries passed in.
	if node == nil {
		return false
	}
	// ignore nodes with no ip address stored.
	if node.IP() == nil {
		return false
	}
	// do not dial nodes with their tcp ports not set
	if err := node.Record().Load(qnr.WithEntry("tcp", new(qnr.TCP))); err != nil {
		if !qnr.IsNotFound(err) {
			log.WithError(err).Debug("Could not retrieve tcp port")
		}
		return false
	}
	peerData, multiAddr, err := convertToAddrInfo(node)
	if err != nil {
		log.WithError(err).Debug("Could not convert to peer data")
		return false
	}
	if s.peers.IsBad(peerData.ID) {
		return false
	}
	if s.peers.IsActive(peerData.ID) {
		// Constantly update QNR for known peers.
		s.peers.UpdateENR(node.Record(), peerData.ID)
		return false
	}
	if s.host.Network().Connectedness(peerData.ID) == network.Connected {
		return false
	}
	if !s.peers.IsReadyToDial(peerData.ID) {
		return false
	}
	nodeQNR := node.Record()
	// Decide whether or not to connect to peer that does not
	// match the proper fork QNR data with our local node.
	if s.genesisValidatorsRoot != nil {
		if err := s.compareForkQNR(nodeQNR); err != nil {
			log.WithError(err).Trace("Fork QNR mismatches between peer and local node")
			return false
		}
	}
	// Add peer to peer handler.
	s.peers.Add(nodeQNR, peerData.ID, multiAddr, network.DirUnknown)
	return true
}

// This checks our set max peers in our config, and
// determines whether our currently connected and
// active peers are above our set max peer limit.
func (s *Service) isPeerAtLimit(inbound bool) bool {
	numOfConns := len(s.host.Network().Peers())
	maxPeers := int(s.cfg.MaxPeers)
	// If we are measuring the limit for inbound peers
	// we apply the high watermark buffer.
	if inbound {
		maxPeers += highWatermarkBuffer
		maxInbound := s.peers.InboundLimit() + highWatermarkBuffer
		currInbound := len(s.peers.InboundConnected())
		// Exit early if we are at the inbound limit.
		if currInbound >= maxInbound {
			return true
		}
	}
	activePeers := len(s.Peers().Active())
	return activePeers >= maxPeers || numOfConns >= maxPeers
}

// PeersFromStringAddrs converts peer raw QNRs into multiaddrs for p2p.
func PeersFromStringAddrs(addrs []string) ([]ma.Multiaddr, error) {
	var allAddrs []ma.Multiaddr
	qnodeString, multiAddrString := parseGenericAddrs(addrs)
	for _, stringAddr := range multiAddrString {
		addr, err := multiAddrFromString(stringAddr)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not get multiaddr from string")
		}
		allAddrs = append(allAddrs, addr)
	}
	for _, stringAddr := range qnodeString {
		qnodeAddr, err := qnode.Parse(qnode.ValidSchemes, stringAddr)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not get qnode from string")
		}
		addr, err := convertToSingleMultiAddr(qnodeAddr)
		if err != nil {
			return nil, errors.Wrapf(err, "Could not get multiaddr")
		}
		allAddrs = append(allAddrs, addr)
	}
	return allAddrs, nil
}

func parseBootStrapAddrs(addrs []string) (discv5Nodes []string) {
	discv5Nodes, _ = parseGenericAddrs(addrs)
	if len(discv5Nodes) == 0 {
		log.Warn("No bootstrap addresses supplied")
	}
	return discv5Nodes
}

func parseGenericAddrs(addrs []string) (qnodeString, multiAddrString []string) {
	for _, addr := range addrs {
		if addr == "" {
			// Ignore empty entries
			continue
		}
		_, err := qnode.Parse(qnode.ValidSchemes, addr)
		if err == nil {
			qnodeString = append(qnodeString, addr)
			continue
		}
		_, err = multiAddrFromString(addr)
		if err == nil {
			multiAddrString = append(multiAddrString, addr)
			continue
		}
		log.WithError(err).Errorf("Invalid address of %s provided", addr)
	}
	return qnodeString, multiAddrString
}

func convertToMultiAddr(nodes []*qnode.Node) []ma.Multiaddr {
	var multiAddrs []ma.Multiaddr
	for _, node := range nodes {
		// ignore nodes with no ip address stored
		if node.IP() == nil {
			continue
		}
		multiAddr, err := convertToSingleMultiAddr(node)
		if err != nil {
			log.WithError(err).Error("Could not convert to multiAddr")
			continue
		}
		multiAddrs = append(multiAddrs, multiAddr)
	}
	return multiAddrs
}

func convertToAddrInfo(node *qnode.Node) (*peer.AddrInfo, ma.Multiaddr, error) {
	multiAddr, err := convertToSingleMultiAddr(node)
	if err != nil {
		return nil, nil, err
	}
	info, err := peer.AddrInfoFromP2pAddr(multiAddr)
	if err != nil {
		return nil, nil, err
	}
	return info, multiAddr, nil
}

func convertToSingleMultiAddr(node *qnode.Node) (ma.Multiaddr, error) {
	pubkey := node.Pubkey()
	assertedKey, err := ecdsaqrysm.ConvertToInterfacePubkey(pubkey)
	if err != nil {
		return nil, errors.Wrap(err, "could not get pubkey")
	}
	id, err := peer.IDFromPublicKey(assertedKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not get peer id")
	}
	return multiAddressBuilderWithID(node.IP().String(), "tcp", uint(node.TCP()), id)
}

func convertToUdpMultiAddr(node *qnode.Node) ([]ma.Multiaddr, error) {
	pubkey := node.Pubkey()
	assertedKey, err := ecdsaqrysm.ConvertToInterfacePubkey(pubkey)
	if err != nil {
		return nil, errors.Wrap(err, "could not get pubkey")
	}
	id, err := peer.IDFromPublicKey(assertedKey)
	if err != nil {
		return nil, errors.Wrap(err, "could not get peer id")
	}

	var addresses []ma.Multiaddr
	var ip4 qnr.IPv4
	var ip6 qnr.IPv6
	if node.Load(&ip4) == nil {
		address, ipErr := multiAddressBuilderWithID(net.IP(ip4).String(), "udp", uint(node.UDP()), id)
		if ipErr != nil {
			return nil, errors.Wrap(ipErr, "could not build IPv4 address")
		}
		addresses = append(addresses, address)
	}
	if node.Load(&ip6) == nil {
		address, ipErr := multiAddressBuilderWithID(net.IP(ip6).String(), "udp", uint(node.UDP()), id)
		if ipErr != nil {
			return nil, errors.Wrap(ipErr, "could not build IPv6 address")
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
}

func peerIdsFromMultiAddrs(addrs []ma.Multiaddr) []peer.ID {
	peers := []peer.ID{}
	for _, a := range addrs {
		info, err := peer.AddrInfoFromP2pAddr(a)
		if err != nil {
			log.WithError(err).Errorf("Could not derive peer info from multiaddress %s", a.String())
			continue
		}
		peers = append(peers, info.ID)
	}
	return peers
}

func multiAddrFromString(address string) (ma.Multiaddr, error) {
	return ma.NewMultiaddr(address)
}

func udpVersionFromIP(ipAddr net.IP) string {
	if ipAddr.To4() != nil {
		return "udp4"
	}
	return "udp6"
}
