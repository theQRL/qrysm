package p2p

import (
	"reflect"
	"testing"

	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
)

func TestMappingHasNoDuplicates(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	m := make(map[reflect.Type]bool)
	for _, fn := range gossipTopicMappings {
		v := fn()
		if _, ok := m[reflect.TypeOf(v)]; ok {
			t.Errorf("%T is duplicated in the topic mapping", v)
		}
		m[reflect.TypeOf(v)] = true
	}
}

func TestGossipTopicMappings_CorrectBlockType(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	// Zond Fork
	pMessage := GossipTopicMappings(BlockSubnetTopicFormat)
	_, ok := pMessage.(*qrysmpb.SignedBeaconBlockZond)
	assert.Equal(t, true, ok)
}

func TestGossipTopicMappings_ReturnsFreshInstancePerCall(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	// Each call must produce a distinct, independent message — otherwise
	// concurrent decoders would race on the same singleton.
	a := GossipTopicMappings(BlockSubnetTopicFormat)
	b := GossipTopicMappings(BlockSubnetTopicFormat)
	assert.NotNil(t, a)
	assert.NotNil(t, b)
	if a == b {
		t.Fatal("GossipTopicMappings returned the same pointer on repeat calls; factory must allocate fresh instances")
	}
}

func TestGossipTopicMappings_UnknownTopicReturnsNil(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	assert.Equal(t, nil, GossipTopicMappings("/nonexistent/topic"))
}
