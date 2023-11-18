package p2p

import (
	"reflect"

	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// gossipTopicMappings represent the protocol ID to protobuf message type map for easy
// lookup.
var gossipTopicMappings = map[string]proto.Message{
	BlockSubnetTopicFormat:                      &zondpb.SignedBeaconBlock{},
	AttestationSubnetTopicFormat:                &zondpb.Attestation{},
	ExitSubnetTopicFormat:                       &zondpb.SignedVoluntaryExit{},
	ProposerSlashingSubnetTopicFormat:           &zondpb.ProposerSlashing{},
	AttesterSlashingSubnetTopicFormat:           &zondpb.AttesterSlashing{},
	AggregateAndProofSubnetTopicFormat:          &zondpb.SignedAggregateAttestationAndProof{},
	SyncContributionAndProofSubnetTopicFormat:   &zondpb.SignedContributionAndProof{},
	SyncCommitteeSubnetTopicFormat:              &zondpb.SyncCommitteeMessage{},
	DilithiumToExecutionChangeSubnetTopicFormat: &zondpb.SignedDilithiumToExecutionChange{},
	BlobSubnetTopicFormat:                       &zondpb.SignedBlobSidecar{},
}

// GossipTopicMappings is a function to return the assigned data type
// versioned by epoch.
func GossipTopicMappings(topic string, epoch primitives.Epoch) proto.Message {
	if topic == BlockSubnetTopicFormat {
		if epoch >= params.BeaconConfig().DenebForkEpoch {
			return &zondpb.SignedBeaconBlockDeneb{}
		}
		if epoch >= params.BeaconConfig().CapellaForkEpoch {
			return &zondpb.SignedBeaconBlockCapella{}
		}
		if epoch >= params.BeaconConfig().BellatrixForkEpoch {
			return &zondpb.SignedBeaconBlockBellatrix{}
		}
		if epoch >= params.BeaconConfig().AltairForkEpoch {
			return &zondpb.SignedBeaconBlockAltair{}
		}
	}
	return gossipTopicMappings[topic]
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
	for k, v := range gossipTopicMappings {
		GossipTypeMapping[reflect.TypeOf(v)] = k
	}
	// Specially handle Altair objects.
	GossipTypeMapping[reflect.TypeOf(&zondpb.SignedBeaconBlockAltair{})] = BlockSubnetTopicFormat
	// Specially handle Bellatrix objects.
	GossipTypeMapping[reflect.TypeOf(&zondpb.SignedBeaconBlockBellatrix{})] = BlockSubnetTopicFormat
	// Specially handle Capella objects.
	GossipTypeMapping[reflect.TypeOf(&zondpb.SignedBeaconBlockCapella{})] = BlockSubnetTopicFormat
	// Specially handle Deneb objects.
	GossipTypeMapping[reflect.TypeOf(&zondpb.SignedBeaconBlockDeneb{})] = BlockSubnetTopicFormat
}
