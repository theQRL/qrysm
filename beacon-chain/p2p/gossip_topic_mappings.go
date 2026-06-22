package p2p

import (
	"reflect"

	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// gossipTopicMappings maps each gossip topic to a factory that constructs a
// fresh, empty protobuf message of the type associated with the topic.
//
// Factories are used instead of singleton values so callers can obtain an
// independent zero-valued message without paying the cost of proto.Clone — a
// reflection-heavy deep copy — on every received gossip message.
var gossipTopicMappings = map[string]func() proto.Message{
	BlockSubnetTopicFormat:                    func() proto.Message { return &qrysmpb.SignedBeaconBlockZond{} },
	AttestationSubnetTopicFormat:              func() proto.Message { return &qrysmpb.Attestation{} },
	ExitSubnetTopicFormat:                     func() proto.Message { return &qrysmpb.SignedVoluntaryExit{} },
	ProposerSlashingSubnetTopicFormat:         func() proto.Message { return &qrysmpb.ProposerSlashing{} },
	AttesterSlashingSubnetTopicFormat:         func() proto.Message { return &qrysmpb.AttesterSlashing{} },
	AggregateAndProofSubnetTopicFormat:        func() proto.Message { return &qrysmpb.SignedAggregateAttestationAndProof{} },
	SyncContributionAndProofSubnetTopicFormat: func() proto.Message { return &qrysmpb.SignedContributionAndProof{} },
	SyncCommitteeSubnetTopicFormat:            func() proto.Message { return &qrysmpb.SyncCommitteeMessage{} },
}

// GossipTopicMappings returns a freshly-allocated zero-valued protobuf message
// for the supplied topic, or nil if the topic is unknown.
func GossipTopicMappings(topic string) proto.Message {
	fn, ok := gossipTopicMappings[topic]
	if !ok {
		return nil
	}
	return fn()
}

// AllTopics returns all topics stored in our
// gossip mapping.
func AllTopics() []string {
	var topics []string
	for k := range gossipTopicMappings {
		topics = append(topics, k)
	}
	return topics
}

// GossipTypeMapping is the inverse of GossipTopicMappings so that an arbitrary protobuf message
// can be mapped to a protocol ID string.
var GossipTypeMapping = make(map[reflect.Type]string, len(gossipTopicMappings))

func init() {
	for k, fn := range gossipTopicMappings {
		GossipTypeMapping[reflect.TypeOf(fn())] = k
	}
}
