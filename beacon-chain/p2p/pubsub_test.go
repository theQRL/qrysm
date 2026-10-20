package p2p

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/p2p/encoder"
	testp2p "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestService_PublishToTopicConcurrentMapWrite(t *testing.T) {
	cs := startup.NewClockSynchronizer()
	s, err := NewService(context.Background(), &Config{
		StateNotifier: &mock.MockStateNotifier{},
		ClockWaiter:   cs,
	})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go s.awaitStateInitialized()
	fd := initializeStateWithForkDigest(ctx, t, cs)

	if !s.isInitialized() {
		t.Fatal("service was not initialized")
	}

	// Set up two connected test hosts.
	p0 := testp2p.NewTestP2P(t)
	p1 := testp2p.NewTestP2P(t)
	p0.Connect(p1)
	s.host = p0.BHost
	s.pubsub = p0.PubSub()

	topic := fmt.Sprintf(BlockSubnetTopicFormat, fd) + "/" + encoder.ProtocolSuffixSSZSnappy

	// Establish the remote peer to be subscribed to the outgoing topic.
	_, err = p1.SubscribeToTopic(topic)
	require.NoError(t, err)

	wg := sync.WaitGroup{}
	wg.Add(10)
	for i := range 10 {
		go func(i int) {
			assert.NoError(t, s.PublishToTopic(ctx, topic, []byte{}))
			wg.Done()
		}(i)
	}
	wg.Wait()
}

// TestPubSub_RepublishAfterIgnoreIsDelivered guards the go-libp2p-pubsub
// behaviour the sync package depends on. A gossip message the topic validator
// IGNOREs (for example an attestation whose block has not arrived yet) is
// marked seen; when the pending-attestation queue later re-broadcasts the very
// same bytes, that local publish must still be validated and delivered rather
// than dropped as a duplicate. go-libp2p-pubsub < v0.17.0 dropped it.
func TestPubSub_RepublishAfterIgnoreIsDelivered(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	newHost := func() host.Host {
		h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, h.Close()) })
		return h
	}
	// Content-derived message ids, like MsgID, so that re-publishing the same
	// bytes yields the same id (the default id is per-publish and would never
	// collide).
	msgID := func(pmsg *pubsubpb.Message) string {
		h := sha256.Sum256(append([]byte(pmsg.GetTopic()), pmsg.GetData()...))
		return string(h[:20])
	}
	newPubSub := func(h host.Host) *pubsub.PubSub {
		ps, err := pubsub.NewGossipSub(ctx, h,
			pubsub.WithMessageSigning(false),
			pubsub.WithStrictSignatureVerification(false),
			pubsub.WithMessageIdFn(msgID),
		)
		require.NoError(t, err)
		return ps
	}

	h0, h1 := newHost(), newHost()
	ps0, ps1 := newPubSub(h0), newPubSub(h1)

	const topic = "/consensus/aabbccdd/beacon_attestation_0/ssz_snappy"
	var accept atomic.Bool
	ignored := make(chan struct{}, 1)
	require.NoError(t, ps0.RegisterTopicValidator(topic, func(_ context.Context, _ peer.ID, _ *pubsub.Message) pubsub.ValidationResult {
		if accept.Load() {
			return pubsub.ValidationAccept
		}
		select {
		case ignored <- struct{}{}:
		default:
		}
		return pubsub.ValidationIgnore
	}))
	topic0, err := ps0.Join(topic)
	require.NoError(t, err)
	sub0, err := topic0.Subscribe()
	require.NoError(t, err)

	require.NoError(t, h1.Connect(ctx, peer.AddrInfo{ID: h0.ID(), Addrs: h0.Addrs()}))
	topic1, err := ps1.Join(topic)
	require.NoError(t, err)
	// Wait until h1 has learnt of h0's subscription so the publish reaches it.
	for len(topic1.ListPeers()) == 0 {
		select {
		case <-ctx.Done():
			t.Fatal("peer never learnt of the subscription")
		case <-time.After(10 * time.Millisecond):
		}
	}

	data := []byte("attestation for a block we do not have yet")
	require.NoError(t, topic1.Publish(ctx, data))
	select {
	case <-ignored:
	case <-ctx.Done():
		t.Fatal("message never reached the validator")
	}

	// The block arrived: the pending queue re-broadcasts the same bytes.
	accept.Store(true)
	require.NoError(t, topic0.Publish(ctx, data))

	msg, err := sub0.Next(ctx)
	require.NoError(t, err, "re-published message was dropped as a duplicate")
	require.Equal(t, string(data), string(msg.Data))
}

func TestExtractGossipDigest(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		want    [4]byte
		wantErr bool
		error   error
	}{
		{
			name:    "empty topic",
			topic:   "",
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("invalid topic format"),
		},
		{
			name:    "too short topic",
			topic:   "/consensus/",
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("invalid topic format"),
		},
		{
			name:    "bogus topic prefix",
			topic:   "/eth3/b5303f2a/beacon_coin",
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("invalid topic format"),
		},
		{
			name:    "invalid digest in topic",
			topic:   "/consensus/zzxxyyaa/beacon_block" + "/" + encoder.ProtocolSuffixSSZSnappy,
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("encoding/hex: invalid byte"),
		},
		{
			name:    "short digest",
			topic:   fmt.Sprintf(BlockSubnetTopicFormat, []byte{0xb5, 0x30, 0x3f}) + "/" + encoder.ProtocolSuffixSSZSnappy,
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("invalid digest length wanted"),
		},
		{
			name:    "too short topic, missing suffixes",
			topic:   "/consensus/b5303f2a",
			want:    [4]byte{},
			wantErr: true,
			error:   errors.New("invalid topic format"),
		},
		{
			name:    "valid topic",
			topic:   fmt.Sprintf(BlockSubnetTopicFormat, []byte{0xb5, 0x30, 0x3f, 0x2a}) + "/" + encoder.ProtocolSuffixSSZSnappy,
			want:    [4]byte{0xb5, 0x30, 0x3f, 0x2a},
			wantErr: false,
			error:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractGossipDigest(tt.topic)
			assert.Equal(t, err != nil, tt.wantErr)
			if tt.wantErr {
				assert.ErrorContains(t, tt.error.Error(), err)
			}
			assert.DeepEqual(t, tt.want, got)
		})
	}
}

func BenchmarkExtractGossipDigest(b *testing.B) {
	topic := fmt.Sprintf(BlockSubnetTopicFormat, []byte{0xb5, 0x30, 0x3f, 0x2a}) + "/" + encoder.ProtocolSuffixSSZSnappy

	for b.Loop() {
		_, err := ExtractGossipDigest(topic)
		if err != nil {
			b.Fatal(err)
		}
	}
}
