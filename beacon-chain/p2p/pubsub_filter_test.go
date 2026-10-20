package p2p

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/theQRL/qrysm/beacon-chain/p2p/encoder"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/network/forks"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	qrysmTime "github.com/theQRL/qrysm/time"
)

func TestService_CanSubscribe(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	currentFork := [4]byte{0x01, 0x02, 0x03, 0x04}
	validProtocolSuffix := "/" + encoder.ProtocolSuffixSSZSnappy
	genesisTime := time.Now()
	var valRoot [32]byte
	digest, err := forks.CreateForkDigest(genesisTime, valRoot[:])
	assert.NoError(t, err)
	type test struct {
		name  string
		topic string
		want  bool
	}
	tests := []test{
		{
			name:  "block topic on current fork",
			topic: fmt.Sprintf(BlockSubnetTopicFormat, digest) + validProtocolSuffix,
			want:  true,
		},
		{
			name:  "block topic on unknown fork",
			topic: fmt.Sprintf(BlockSubnetTopicFormat, [4]byte{0xFF, 0xEE, 0x56, 0x21}) + validProtocolSuffix,
			want:  false,
		},
		{
			name:  "block topic missing protocol suffix",
			topic: fmt.Sprintf(BlockSubnetTopicFormat, currentFork),
			want:  false,
		},
		{
			name:  "block topic wrong protocol suffix",
			topic: fmt.Sprintf(BlockSubnetTopicFormat, currentFork) + "/foobar",
			want:  false,
		},
		{
			name:  "erroneous topic",
			topic: "hey, want to foobar?",
			want:  false,
		},
		{
			name:  "erroneous topic that has the correct amount of slashes",
			topic: "hey, want to foobar?////",
			want:  false,
		},
		{
			name:  "bad prefix",
			topic: fmt.Sprintf("/eth3/%x/foobar", digest) + validProtocolSuffix,
			want:  false,
		},
		{
			name:  "topic not in gossip mapping",
			topic: fmt.Sprintf("/consensus/%x/foobar", digest) + validProtocolSuffix,
			want:  false,
		},
		{
			name:  "att subnet topic on current fork",
			topic: fmt.Sprintf(AttestationSubnetTopicFormat, digest, 3 /*subnet*/) + validProtocolSuffix,
			want:  true,
		},
		{
			name:  "att subnet topic on unknown fork",
			topic: fmt.Sprintf(AttestationSubnetTopicFormat, [4]byte{0xCC, 0xBB, 0xAA, 0xA1} /*fork digest*/, 54 /*subnet*/) + validProtocolSuffix,
			want:  false,
		},
		// Partial matches. Each of these was accepted by the fmt.Sscanf-based
		// check and let a peer grow pubsub's per-peer topic map without bound.
		{
			name:  "att subnet id with trailing garbage",
			topic: fmt.Sprintf(AttestationSubnetTopicFormat, digest, 5) + "zzzz" + validProtocolSuffix,
			want:  false,
		},
		{
			name:  "att subnet id negative",
			topic: fmt.Sprintf(AttestationSubnetTopicFormat, digest, -5) + validProtocolSuffix,
			want:  false,
		},
		{
			name:  "att subnet id with leading zero",
			topic: fmt.Sprintf("/consensus/%x/beacon_attestation_05", digest) + validProtocolSuffix,
			want:  false,
		},
		{
			name:  "att subnet id out of range",
			topic: fmt.Sprintf(AttestationSubnetTopicFormat, digest, params.BeaconNetworkConfig().AttestationSubnetCount) + validProtocolSuffix,
			want:  false,
		},
		{
			name:  "sync subnet id out of range",
			topic: fmt.Sprintf(SyncCommitteeSubnetTopicFormat, digest, params.BeaconConfig().SyncCommitteeSubnetCount) + validProtocolSuffix,
			want:  false,
		},
		{
			name:  "block topic with trailing garbage",
			topic: fmt.Sprintf(BlockSubnetTopicFormat, digest) + "ZZZZ" + validProtocolSuffix,
			want:  false,
		},
		{
			name:  "block topic with leading garbage",
			topic: fmt.Sprintf("/consensus/%x/xbeacon_block", digest) + validProtocolSuffix,
			want:  false,
		},
		{
			name:  "digest with trailing garbage",
			topic: fmt.Sprintf("/consensus/%xff/beacon_block", digest) + validProtocolSuffix,
			want:  false,
		},
	}

	// Ensure all gossip topic mappings pass validation.
	for _, topic := range AllTopics() {
		formatting := []any{digest}

		// Special case for attestation subnets which have a second formatting placeholder.
		if topic == AttestationSubnetTopicFormat || topic == SyncCommitteeSubnetTopicFormat {
			formatting = append(formatting, 0 /* some subnet ID */)
		}

		tt := test{
			name:  topic,
			topic: fmt.Sprintf(topic, formatting...) + validProtocolSuffix,
			want:  true,
		}
		tests = append(tests, tt)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				genesisValidatorsRoot: valRoot[:],
				genesisTime:           genesisTime,
			}
			if got := s.CanSubscribe(tt.topic); got != tt.want {
				t.Errorf("CanSubscribe(%s) = %v, want %v", tt.topic, got, tt.want)
			}
		})
	}
}

func TestService_CanSubscribe_uninitialized(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	s := &Service{}
	require.Equal(t, false, s.CanSubscribe("foo"))
}

func TestService_FilterIncomingSubscriptions(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	validProtocolSuffix := "/" + encoder.ProtocolSuffixSSZSnappy
	genesisTime := time.Now()
	var valRoot [32]byte
	digest, err := forks.CreateForkDigest(genesisTime, valRoot[:])
	assert.NoError(t, err)
	type args struct {
		id   peer.ID
		subs []*pubsubpb.RPC_SubOpts
	}
	tests := []struct {
		name    string
		args    args
		want    []*pubsubpb.RPC_SubOpts
		wantErr bool
	}{
		{
			name: "too many topics",
			args: args{
				subs: make([]*pubsubpb.RPC_SubOpts, pubsubSubscriptionRequestLimit+1),
			},
			wantErr: true,
		},
		{
			name: "exactly topic limit",
			args: args{
				subs: make([]*pubsubpb.RPC_SubOpts, pubsubSubscriptionRequestLimit),
			},
			wantErr: false,
			want:    nil, // No topics matched filters.
		},
		{
			name: "blocks topic",
			args: args{
				subs: []*pubsubpb.RPC_SubOpts{
					{
						Subscribe: func() *bool {
							b := true
							return &b
						}(),
						Topicid: func() *string {
							s := fmt.Sprintf(BlockSubnetTopicFormat, digest) + validProtocolSuffix
							return &s
						}(),
					},
				},
			},
			wantErr: false,
			want: []*pubsubpb.RPC_SubOpts{
				{
					Subscribe: func() *bool {
						b := true
						return &b
					}(),
					Topicid: func() *string {
						s := fmt.Sprintf(BlockSubnetTopicFormat, digest) + validProtocolSuffix
						return &s
					}(),
				},
			},
		},
		{
			name: "blocks topic duplicated",
			args: args{
				subs: []*pubsubpb.RPC_SubOpts{
					{
						Subscribe: func() *bool {
							b := true
							return &b
						}(),
						Topicid: func() *string {
							s := fmt.Sprintf(BlockSubnetTopicFormat, digest) + validProtocolSuffix
							return &s
						}(),
					},
					{
						Subscribe: func() *bool {
							b := true
							return &b
						}(),
						Topicid: func() *string {
							s := fmt.Sprintf(BlockSubnetTopicFormat, digest) + validProtocolSuffix
							return &s
						}(),
					},
				},
			},
			wantErr: false,
			want: []*pubsubpb.RPC_SubOpts{ // Duplicated topics are only present once after filtering.
				{
					Subscribe: func() *bool {
						b := true
						return &b
					}(),
					Topicid: func() *string {
						s := fmt.Sprintf(BlockSubnetTopicFormat, digest) + validProtocolSuffix
						return &s
					}(),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{
				genesisValidatorsRoot: valRoot[:],
				genesisTime:           genesisTime,
			}
			got, err := s.FilterIncomingSubscriptions(tt.args.id, tt.args.subs)
			if (err != nil) != tt.wantErr {
				t.Errorf("FilterIncomingSubscriptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("FilterIncomingSubscriptions() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestService_MonitorsStateForkUpdates(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cs := startup.NewClockSynchronizer()
	s, err := NewService(ctx, &Config{ClockWaiter: cs})
	require.NoError(t, err)

	require.Equal(t, false, s.isInitialized())

	go s.awaitStateInitialized()

	vr := bytesutil.ToBytes32(bytesutil.PadTo([]byte("genesis"), 32))
	require.NoError(t, cs.SetClock(startup.NewClock(qrysmTime.Now(), vr)))

	time.Sleep(50 * time.Millisecond)

	require.Equal(t, true, s.isInitialized())
}

// TestService_subscribableTopics_IsExactAndBounded pins the size of the
// subscription allowlist: pubsub keeps one map entry per accepted topic per
// peer, so the set of topics a peer can make us record must be exactly the
// node's own gossip topics for the current digest.
func TestService_subscribableTopics_IsExactAndBounded(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	genesisTime := time.Now()
	var valRoot [32]byte
	digest, err := forks.CreateForkDigest(genesisTime, valRoot[:])
	require.NoError(t, err)
	s := &Service{
		genesisValidatorsRoot: valRoot[:],
		genesisTime:           genesisTime,
	}

	topics := s.subscribableTopics(digest)
	nonSubnetTopics := len(gossipTopicMappings) - 2 // attestation and sync committee subnet formats
	want := nonSubnetTopics +
		int(params.BeaconNetworkConfig().AttestationSubnetCount) +
		int(params.BeaconConfig().SyncCommitteeSubnetCount)
	require.Equal(t, want, len(topics))
	for topic := range topics {
		require.Equal(t, true, s.CanSubscribe(topic), "topic %s", topic)
	}

	// A flood of near-miss topics leaves nothing accepted.
	suffix := "/" + encoder.ProtocolSuffixSSZSnappy
	var subs []*pubsubpb.RPC_SubOpts
	for i := range pubsubSubscriptionRequestLimit {
		topic := fmt.Sprintf(AttestationSubnetTopicFormat, digest, i%3) + fmt.Sprintf("x%d", i) + suffix
		subscribe := true
		subs = append(subs, &pubsubpb.RPC_SubOpts{Subscribe: &subscribe, Topicid: &topic})
	}
	got, err := s.FilterIncomingSubscriptions("peer", subs)
	require.NoError(t, err)
	require.Equal(t, 0, len(got))

	// The cache is rebuilt for a new digest and no longer accepts the old one.
	otherDigest := [4]byte{0xde, 0xad, 0xbe, 0xef}
	otherTopics := s.subscribableTopics(otherDigest)
	require.Equal(t, want, len(otherTopics))
	_, ok := otherTopics[fmt.Sprintf(BlockSubnetTopicFormat, digest)+suffix]
	require.Equal(t, false, ok)
}
