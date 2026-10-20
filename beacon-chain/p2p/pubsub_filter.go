package p2p

import (
	"fmt"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/theQRL/qrysm/config/params"
)

var _ pubsub.SubscriptionFilter = (*Service)(nil)

// It is set at this limit to handle the possibility
// of double topic subscriptions at fork boundaries.
// -> 64 Attestation Subnets * 2.
// -> 4 Sync Committee Subnets * 2.
// -> Block,Aggregate,ProposerSlashing,AttesterSlashing,Exits,SyncContribution * 2.
const pubsubSubscriptionRequestLimit = 200

// CanSubscribe returns true if the topic is of interest and we could subscribe to it.
//
// The topic must match one of the node's gossip topics for the current fork
// digest exactly. pubsub records every accepted topic in a per-peer map that is
// only pruned when the peer unsubscribes, so anything looser than an exact
// match (the previous fmt.Sscanf-based check accepted "beacon_block" followed
// by arbitrary text and subnet ids such as "5zzzz", "-5" or "05") lets a peer
// grow that map without bound by sending near-arbitrary topic strings.
func (s *Service) CanSubscribe(topic string) bool {
	if !s.isInitialized() {
		return false
	}
	digest, err := s.currentForkDigest()
	if err != nil {
		log.WithError(err).Error("Could not determine Zond fork digest")
		return false
	}
	_, ok := s.subscribableTopics(digest)[topic]
	return ok
}

// subscribableTopics returns the exact set of gossip topics (including the
// encoding suffix) the node accepts subscriptions for under the given fork
// digest: every non-subnet topic plus one topic per attestation and sync
// committee subnet. The set is cached and rebuilt when the digest changes.
func (s *Service) subscribableTopics(digest [4]byte) map[string]struct{} {
	s.subscribableTopicsLock.Lock()
	defer s.subscribableTopicsLock.Unlock()

	if s.subscribableTopicSet != nil && s.subscribableTopicsDigest == digest {
		return s.subscribableTopicSet
	}

	suffix := s.Encoding().ProtocolSuffix()
	attSubnetCount := params.BeaconNetworkConfig().AttestationSubnetCount
	syncSubnetCount := params.BeaconConfig().SyncCommitteeSubnetCount

	topics := make(map[string]struct{}, len(gossipTopicMappings)+int(attSubnetCount)+int(syncSubnetCount)) // lint:ignore uintcast -- subnet counts are small protocol constants.
	for format := range gossipTopicMappings {
		switch format {
		case AttestationSubnetTopicFormat:
			for subnet := uint64(0); subnet < attSubnetCount; subnet++ {
				topics[fmt.Sprintf(format, digest, subnet)+suffix] = struct{}{}
			}
		case SyncCommitteeSubnetTopicFormat:
			for subnet := uint64(0); subnet < syncSubnetCount; subnet++ {
				topics[fmt.Sprintf(format, digest, subnet)+suffix] = struct{}{}
			}
		default:
			topics[fmt.Sprintf(format, digest)+suffix] = struct{}{}
		}
	}

	s.subscribableTopicSet = topics
	s.subscribableTopicsDigest = digest
	return topics
}

// FilterIncomingSubscriptions is invoked for all RPCs containing subscription notifications.
// This method returns only the topics of interest and may return an error if the subscription
// request contains too many topics.
func (s *Service) FilterIncomingSubscriptions(_ peer.ID, subs []*pubsubpb.RPC_SubOpts) ([]*pubsubpb.RPC_SubOpts, error) {
	if len(subs) > pubsubSubscriptionRequestLimit {
		return nil, pubsub.ErrTooManySubscriptions
	}

	return pubsub.FilterSubscriptions(subs, s.CanSubscribe), nil
}
