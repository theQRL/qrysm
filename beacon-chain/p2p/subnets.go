package p2p

import (
	"context"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/go-qrl/p2p/qnode"
	"github.com/theQRL/go-qrl/p2p/qnr"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/cmd/beacon-chain/flags"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/consensus-types/wrapper"
	"go.opencensus.io/trace"

	"github.com/theQRL/qrysm/config/params"
	pb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

var attestationSubnetCount = params.BeaconNetworkConfig().AttestationSubnetCount
var syncCommsSubnetCount = params.BeaconConfig().SyncCommitteeSubnetCount

var attSubnetQnrKey = params.BeaconNetworkConfig().AttSubnetKey
var syncCommsSubnetQnrKey = params.BeaconNetworkConfig().SyncCommsSubnetKey

// The value used with the subnet, inorder
// to create an appropriate key to retrieve
// the relevant lock. This is used to differentiate
// sync subnets from attestation subnets. This is deliberately
// chosen as more than 64(attestation subnet count).
const syncLockerVal = 100

// nodeFilter returns a function that filters nodes based on the subnet topic and subnet index.
func (s *Service) nodeFilter(topic string, index uint64) (func(node *qnode.Node) bool, error) {
	switch {
	case strings.Contains(topic, GossipAttestationMessage):
		return s.filterPeerForAttSubnet(index), nil
	case strings.Contains(topic, GossipSyncCommitteeMessage):
		return s.filterPeerForSyncSubnet(index), nil
	default:
		return nil, errors.Errorf("no subnet exists for provided topic: %s", topic)
	}
}

// searchForPeers performs a network search for peers subscribed to a particular subnet.
// It exits as soon as one of these conditions is met:
// - It looped through `batchSize` nodes.
// - It found `peersToFindCount` peers corresponding to the `filter` criteria.
// - Iterator is exhausted.
func searchForPeers(
	iterator qnode.Iterator,
	batchSize int,
	peersToFindCount uint,
	filter func(node *qnode.Node) bool,
) []*qnode.Node {
	nodeFromNodeID := make(map[qnode.ID]*qnode.Node, batchSize)
	for i := 0; i < batchSize && uint(len(nodeFromNodeID)) < peersToFindCount && iterator.Next(); i++ {
		node := iterator.Node()

		// Dedup first: keep the previously stored node when its sequence
		// number is at least as high as the new one. Doing this before the
		// filter ensures that when a node ID arrives multiple times during
		// iteration, we always evaluate the freshest record.
		prevNode, ok := nodeFromNodeID[node.ID()]
		if ok && prevNode.Seq() >= node.Seq() {
			continue
		}

		// Filter out nodes that do not meet the criteria. If a newer ENR
		// for the same node ID fails the filter (e.g. it dropped the
		// requested subnet), discard the stale lower-seq entry too — the
		// peer is no longer a valid match. (upstream PR #15578)
		if !filter(node) {
			if ok {
				delete(nodeFromNodeID, prevNode.ID())
			}
			continue
		}

		nodeFromNodeID[node.ID()] = node
	}

	// Convert the map to a slice.
	nodes := make([]*qnode.Node, 0, len(nodeFromNodeID))
	for _, node := range nodeFromNodeID {
		nodes = append(nodes, node)
	}

	return nodes
}

// dialPeer dials a peer in a separate goroutine.
func (s *Service) dialPeer(ctx context.Context, wg *sync.WaitGroup, node *qnode.Node) {
	info, _, err := convertToAddrInfo(node)
	if err != nil {
		return
	}

	if info == nil {
		return
	}

	wg.Add(1)
	go func() {
		if err := s.connectWithPeer(ctx, *info); err != nil {
			log.WithError(err).Tracef("Could not connect with peer %s", info.String())
		}

		wg.Done()
	}()
}

// FindPeersWithSubnet performs a network search for peers
// subscribed to a particular subnet. Then it tries to connect
// with those peers. This method will block until either:
// - The required amount of peers are found, the method returns true.
// - The context is canceled, the method returns false.
// On some edge cases, this method may hang indefinitely while peers
// are actually found. In such a case, the user should cancel the context
// and re-run the method again.
func (s *Service) FindPeersWithSubnet(
	ctx context.Context,
	topic string,
	index uint64,
	threshold int,
) (bool, error) {
	const minLogInterval = 1 * time.Minute

	ctx, span := trace.StartSpan(ctx, "p2p.FindPeersWithSubnet")
	defer span.End()

	span.AddAttributes(trace.Int64Attribute("index", int64(index))) // lint:ignore uintcast -- It's safe to do this for tracing.

	if s.dv5Listener == nil {
		// Return if discovery isn't set.
		return false, nil
	}

	topic += s.Encoding().ProtocolSuffix()
	iterator := s.dv5Listener.RandomNodes()
	defer iterator.Close()

	filter, err := s.nodeFilter(topic, index)
	if err != nil {
		return false, errors.Wrap(err, "node filter")
	}

	peersSummary := func(topic string, threshold int) (int, int) {
		// Retrieve how many peers we have for this topic.
		peerCountForTopic := len(s.pubsub.ListPeers(topic))

		// Compute how many peers we are missing to reach the threshold.
		missingPeerCountForTopic := max(0, threshold-peerCountForTopic)

		return peerCountForTopic, missingPeerCountForTopic
	}

	// Compute how many peers we are missing to reach the threshold.
	peerCountForTopic, missingPeerCountForTopic := peersSummary(topic, threshold)

	// Exit early if we have enough peers.
	if missingPeerCountForTopic == 0 {
		return true, nil
	}

	logEntry := log.WithFields(logrus.Fields{
		"topic":           topic,
		"targetPeerCount": threshold,
	})

	logEntry.WithField("currentPeerCount", peerCountForTopic).Debug("Searching for new peers for a subnet - start")

	lastLogTime := time.Now()

	wg := new(sync.WaitGroup)
	for {
		// If the context is done, we can exit the loop. This is the unhappy path.
		if err := ctx.Err(); err != nil {
			return false, errors.Errorf(
				"unable to find requisite number of peers for topic %s - only %d out of %d peers available after searching",
				topic, peerCountForTopic, threshold,
			)
		}

		// Search for new peers in the network.
		nodes := searchForPeers(iterator, batchSize, uint(missingPeerCountForTopic), filter)

		// Restrict dials if limit is applied.
		maxConcurrentDials := math.MaxInt
		if flags.MaxDialIsActive() {
			maxConcurrentDials = flags.Get().MaxConcurrentDials
		}

		// Dial the peers in batches.
		for start := 0; start < len(nodes); start += maxConcurrentDials {
			stop := min(start+maxConcurrentDials, len(nodes))
			for _, node := range nodes[start:stop] {
				s.dialPeer(ctx, wg, node)
			}

			// Wait for all dials to be completed.
			wg.Wait()
		}

		peerCountForTopic, missingPeerCountForTopic = peersSummary(topic, threshold)

		// If we have enough peers, we can exit the loop. This is the happy path.
		if missingPeerCountForTopic == 0 {
			break
		}

		if time.Since(lastLogTime) > minLogInterval {
			lastLogTime = time.Now()
			logEntry.WithField("currentPeerCount", peerCountForTopic).Debug("Searching for new peers for a subnet - continue")
		}
	}

	logEntry.WithField("currentPeerCount", threshold).Debug("Searching for new peers for a subnet - success")
	return true, nil
}

// returns a method with filters peers specifically for a particular attestation subnet.
// Errors from attSubnets are logged at Debug level so a single peer with a
// malformed subnet record is skipped (not silently dropped) without aborting
// the broader subnet search — qrysm's filter signature is bool, but the
// diagnostic intent matches upstream PR #15815.
func (s *Service) filterPeerForAttSubnet(index uint64) func(node *qnode.Node) bool {
	return func(node *qnode.Node) bool {
		if !s.filterPeer(node) {
			return false
		}
		subnets, err := attSubnets(node.Record())
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"nodeID":      node.ID(),
				"topicFormat": GossipAttestationMessage,
			}).Debug("Could not get needed subnets from peer")
			return false
		}
		return slices.Contains(subnets, index)
	}
}

// returns a method with filters peers specifically for a particular sync subnet.
// See filterPeerForAttSubnet for rationale on the error-handling pattern.
// (upstream PR #15815)
func (s *Service) filterPeerForSyncSubnet(index uint64) func(node *qnode.Node) bool {
	return func(node *qnode.Node) bool {
		if !s.filterPeer(node) {
			return false
		}
		subnets, err := syncSubnets(node.Record())
		if err != nil {
			log.WithError(err).WithFields(logrus.Fields{
				"nodeID":      node.ID(),
				"topicFormat": GossipSyncCommitteeMessage,
			}).Debug("Could not get needed subnets from peer")
			return false
		}
		return slices.Contains(subnets, index)
	}
}

// lower threshold to broadcast object compared to searching
// for a subnet. So that even in the event of poor peer
// connectivity, we can still broadcast an attestation.
func (s *Service) hasPeerWithSubnet(topic string) bool {
	// In the event peer threshold is lower, we will choose the lower
	// threshold.
	minPeers := min(1, uint64(flags.Get().MinimumPeersPerSubnet))
	return len(s.pubsub.ListPeers(topic+s.Encoding().ProtocolSuffix())) >= int(minPeers) // lint:ignore uintcast -- Min peers can be safely cast to int.
}

func advertisedSubnetBitfields(currEpoch primitives.Epoch) (bitfield.Bitvector64, bitfield.Bitvector4) {
	if flags.Get().SubscribeToAllSubnets {
		return allAttestationSubnetsBitfield(), allSyncCommitteeSubnetsBitfield()
	}

	bitVAtt := bitfield.NewBitvector64()
	committees := cache.SubnetIDs.GetAllSubnets()
	for _, idx := range committees {
		bitVAtt.SetBitAt(idx, true)
	}

	bitVSync := bitfield.Bitvector4{byte(0x00)}
	committees = cache.SyncSubnetIDs.GetAllSubnets(currEpoch)
	for _, idx := range committees {
		bitVSync.SetBitAt(idx, true)
	}

	return bitVAtt, bitVSync
}

func allAttestationSubnetsBitfield() bitfield.Bitvector64 {
	bitV := bitfield.NewBitvector64()
	for idx := uint64(0); idx < attestationSubnetCount; idx++ {
		bitV.SetBitAt(idx, true)
	}
	return bitV
}

func allSyncCommitteeSubnetsBitfield() bitfield.Bitvector4 {
	bitV := bitfield.Bitvector4{byte(0x00)}
	for idx := uint64(0); idx < syncCommsSubnetCount; idx++ {
		bitV.SetBitAt(idx, true)
	}
	return bitV
}

func (s *Service) metadataBitfields() (bitfield.Bitvector64, bitfield.Bitvector4) {
	if s.metaData == nil || s.metaData.IsNil() || s.metaData.MetadataObjV1() == nil {
		return bitfield.NewBitvector64(), bitfield.Bitvector4{byte(0x00)}
	}

	md := s.metaData.MetadataObjV1()
	return md.Attnets, md.Syncnets
}

// Updates the service's discv5 listener record's attestation subnet
// with a new value for a bitfield of subnets tracked. It also record's
// the sync committee subnet in the qnr. It also updates the node's
// metadata by increasing the sequence number and the subnets tracked by the node.
func (s *Service) updateSubnetRecordWithMetadata(bitVAtt bitfield.Bitvector64, bitVSync bitfield.Bitvector4) error {
	if s.dv5Listener != nil {
		entry := qnr.WithEntry(attSubnetQnrKey, &bitVAtt)
		subEntry := qnr.WithEntry(syncCommsSubnetQnrKey, &bitVSync)
		s.dv5Listener.LocalNode().Set(entry)
		s.dv5Listener.LocalNode().Set(subEntry)
	}

	seq := uint64(0)
	if s.metaData != nil && !s.metaData.IsNil() {
		seq = s.metaData.SequenceNumber()
	}
	s.metaData = wrapper.WrappedMetadataV1(&pb.MetaDataV1{
		SeqNumber: seq + 1,
		Attnets:   bitVAtt,
		Syncnets:  bitVSync,
	})

	if err := s.saveSequenceNumberIfNeeded(); err != nil {
		return errors.Wrap(err, "saving sequence number after updating subnets")
	}
	return nil
}

// saveSequenceNumberIfNeeded persists the metadata sequence number to the database when
// the node uses a static peer ID, so peers don't reject our metadata responses after a
// restart with a smaller sequence number.
func (s *Service) saveSequenceNumberIfNeeded() error {
	if s.cfg == nil || !s.cfg.StaticPeerID || s.cfg.DB == nil {
		return nil
	}
	return s.cfg.DB.SaveMetadataSeqNum(s.ctx, s.metaData.SequenceNumber())
}

// Initializes a bitvector of attestation subnets beacon nodes is subscribed to
// and creates a new QNR entry with its default value.
func initializeAttSubnets(node *qnode.LocalNode) *qnode.LocalNode {
	bitV := bitfield.NewBitvector64()
	entry := qnr.WithEntry(attSubnetQnrKey, bitV.Bytes())
	node.Set(entry)
	return node
}

// Initializes a bitvector of sync committees subnets beacon nodes is subscribed to
// and creates a new QNR entry with its default value.
func initializeSyncCommSubnets(node *qnode.LocalNode) *qnode.LocalNode {
	bitV := bitfield.Bitvector4{byte(0x00)}
	entry := qnr.WithEntry(syncCommsSubnetQnrKey, bitV.Bytes())
	node.Set(entry)
	return node
}

// Reads the attestation subnets entry from a node's QNR and determines
// the committee indices of the attestation subnets the node is subscribed to.
func attSubnets(record *qnr.Record) ([]uint64, error) {
	bitV, err := attBitvector(record)
	if err != nil {
		return nil, err
	}
	// lint:ignore uintcast -- subnet count can be safely cast to int.
	if len(bitV) != byteCount(int(attestationSubnetCount)) {
		return []uint64{}, errors.Errorf("invalid bitvector provided, it has a size of %d", len(bitV))
	}
	var committeeIdxs []uint64
	for i := uint64(0); i < attestationSubnetCount; i++ {
		if bitV.BitAt(i) {
			committeeIdxs = append(committeeIdxs, i)
		}
	}
	return committeeIdxs, nil
}

// Reads the sync subnets entry from a node's QNR and determines
// the committee indices of the sync subnets the node is subscribed to.
func syncSubnets(record *qnr.Record) ([]uint64, error) {
	bitV, err := syncBitvector(record)
	if err != nil {
		return nil, err
	}
	// lint:ignore uintcast -- subnet count can be safely cast to int.
	if len(bitV) != byteCount(int(syncCommsSubnetCount)) {
		return []uint64{}, errors.Errorf("invalid bitvector provided, it has a size of %d", len(bitV))
	}
	var committeeIdxs []uint64
	for i := uint64(0); i < syncCommsSubnetCount; i++ {
		if bitV.BitAt(i) {
			committeeIdxs = append(committeeIdxs, i)
		}
	}
	return committeeIdxs, nil
}

// Parses the attestation subnets QNR entry in a node and extracts its value
// as a bitvector for further manipulation.
func attBitvector(record *qnr.Record) (bitfield.Bitvector64, error) {
	bitV := bitfield.NewBitvector64()
	entry := qnr.WithEntry(attSubnetQnrKey, &bitV)
	err := record.Load(entry)
	if err != nil {
		return nil, err
	}
	return bitV, nil
}

// Parses the attestation subnets QNR entry in a node and extracts its value
// as a bitvector for further manipulation.
func syncBitvector(record *qnr.Record) (bitfield.Bitvector4, error) {
	bitV := bitfield.Bitvector4{byte(0x00)}
	entry := qnr.WithEntry(syncCommsSubnetQnrKey, &bitV)
	err := record.Load(entry)
	if err != nil {
		return nil, err
	}
	return bitV, nil
}

// The subnet locker is a map which keeps track of all
// mutexes stored per subnet. This locker is re-used
// between both the attestation and sync subnets. In
// order to differentiate between attestation and sync
// subnets. Sync subnets are stored by (subnet+syncLockerVal). This
// is to prevent conflicts while allowing both subnets
// to use a single locker.
func (s *Service) subnetLocker(i uint64) *sync.RWMutex {
	s.subnetsLockLock.Lock()
	defer s.subnetsLockLock.Unlock()
	l, ok := s.subnetsLock[i]
	if !ok {
		l = &sync.RWMutex{}
		s.subnetsLock[i] = l
	}
	return l
}

// Determines the number of bytes that are used
// to represent the provided number of bits.
func byteCount(bitCount int) int {
	numOfBytes := bitCount / 8
	if bitCount%8 != 0 {
		numOfBytes++
	}
	return numOfBytes
}
