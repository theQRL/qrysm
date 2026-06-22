// Package testing provides test utilities for the beacon-chain/p2p package.
package testing

import (
	"context"
	"errors"
	"fmt"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/sirupsen/logrus"
)

var tracerLog = logrus.WithField("prefix", "gossiptracer")

type topicPeer struct {
	topic string
	peer  peer.ID
}

// GossipTracer implements pubsub.RawTracer for use in tests. It allows callers
// to block until specific gossipsub-internal events have fired, which is useful
// for avoiding races between the various maps maintained by the pubsub event loop.
//
// Individual methods (RemovePeer, Prune, ValidateMessage, etc.) can be extended
// as needed by future tests. (upstream PR #16395)
type GossipTracer struct {
	mu             sync.Mutex
	addPeerWaiters map[peer.ID]chan struct{}
	addedPeers     map[peer.ID]bool

	joinedTopics map[string]bool

	graftedPeers map[topicPeer]bool
	graftWaiters map[topicPeer]chan struct{}
	topicWaiters map[string]*topicEventWaiter
}

// NewGossipTracer returns a new tracer ready for use. Pass it to
// pubsub.NewGossipSub via pubsub.WithRawTracer(tracer).
func NewGossipTracer() *GossipTracer {
	return &GossipTracer{
		addPeerWaiters: make(map[peer.ID]chan struct{}),
		addedPeers:     make(map[peer.ID]bool),
		joinedTopics:   make(map[string]bool),
		graftedPeers:   make(map[topicPeer]bool),
		graftWaiters:   make(map[topicPeer]chan struct{}),
		topicWaiters:   make(map[string]*topicEventWaiter),
	}
}

func (t *GossipTracer) waitForAddPeer(ctx context.Context, pid peer.ID) error {
	t.mu.Lock()
	if t.addedPeers[pid] {
		t.mu.Unlock()
		return nil
	}
	ch, ok := t.addPeerWaiters[pid]
	if !ok {
		ch = make(chan struct{})
		t.addPeerWaiters[pid] = ch
	}
	t.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *GossipTracer) waitForGraft(ctx context.Context, topic string, pid peer.ID) error {
	key := topicPeer{topic: topic, peer: pid}

	t.mu.Lock()
	if t.graftedPeers[key] {
		t.mu.Unlock()
		return nil
	}
	ch, ok := t.graftWaiters[key]
	if !ok {
		ch = make(chan struct{})
		t.graftWaiters[key] = ch
	}
	t.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (t *GossipTracer) isSubscribed(topic string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.joinedTopics[topic]
}

// CanPublishToPeer blocks until the gossipsub event loop is in a state where
// publishing a message on the given topic will successfully reach pid.
//
// The conditions depend on whether we have locally subscribed to the topic:
//   - Subscribed (mesh path): waits until pid has been grafted into our mesh
//     for the topic.
//   - Not subscribed (fanout path): waits until both PeerJoin (pid is in
//     p.topics[topic]) and AddPeer (pid is in p.peers with an rpcQueue) have
//     fired.
//
// Note: You must call 'JoinAndWatchTopic' first before calling this method.
func (t *GossipTracer) CanPublishToPeer(ctx context.Context, topic string, pid peer.ID) error {
	if t.isSubscribed(topic) {
		return t.waitForGraft(ctx, topic, pid)
	}

	// Fanout path: need both PeerJoin and AddPeer.
	w := t.getTopicWaiter(topic)
	if w == nil {
		return errors.New("topic waiter not found, please call JoinAndWatchTopic first")
	}
	if err := w.waitForPeerJoin(ctx, pid); err != nil {
		return fmt.Errorf("wait for peer join: %w", err)
	}
	if err := t.waitForAddPeer(ctx, pid); err != nil {
		return fmt.Errorf("wait for add peer: %w", err)
	}
	return nil
}

// topicEventWaiter tracks PeerJoin/PeerLeave events for a single topic.
type topicEventWaiter struct {
	mu      sync.Mutex
	joined  map[peer.ID]struct{}
	waiters map[peer.ID]chan struct{}
}

type topicJoiner interface {
	JoinTopic(topic string, opts ...pubsub.TopicOpt) (*pubsub.Topic, error)
}

// JoinAndWatchTopic joins the topic on the given joiner and starts watching
// peer-join events so CanPublishToPeer can block on the fanout path.
func (t *GossipTracer) JoinAndWatchTopic(ctx context.Context, topic string, joiner topicJoiner) (*pubsub.Topic, error) {
	topicHandle, err := joiner.JoinTopic(topic)
	if err != nil {
		return nil, fmt.Errorf("join topic: %w", err)
	}
	if err := t.watchTopic(ctx, topicHandle); err != nil {
		return nil, fmt.Errorf("watch topic: %w", err)
	}
	return topicHandle, nil
}

func (t *GossipTracer) watchTopic(ctx context.Context, topicHandle *pubsub.Topic) error {
	ev, err := topicHandle.EventHandler()
	if err != nil {
		return fmt.Errorf("event handler: %w", err)
	}

	w := &topicEventWaiter{
		joined:  make(map[peer.ID]struct{}),
		waiters: make(map[peer.ID]chan struct{}),
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.topicWaiters[topicHandle.String()] = w

	go func() {
		defer ev.Cancel()
		for {
			pe, err := ev.NextPeerEvent(ctx)
			if err != nil {
				if ctx.Err() == nil {
					tracerLog.WithError(err).Debug("NextPeerEvent failed")
				}
				return
			}
			if pe.Type == pubsub.PeerJoin {
				w.handlePeerJoin(pe.Peer)
			}
		}
	}()

	return nil
}

func (t *GossipTracer) getTopicWaiter(topic string) *topicEventWaiter {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.topicWaiters[topic]
}

func (w *topicEventWaiter) handlePeerJoin(pid peer.ID) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.joined[pid] = struct{}{}
	if ch, ok := w.waiters[pid]; ok {
		close(ch)
		delete(w.waiters, pid)
	}
}

func (w *topicEventWaiter) waitForPeerJoin(ctx context.Context, pid peer.ID) error {
	w.mu.Lock()
	if _, ok := w.joined[pid]; ok {
		w.mu.Unlock()
		return nil
	}
	ch, ok := w.waiters[pid]
	if !ok {
		ch = make(chan struct{})
		w.waiters[pid] = ch
	}
	w.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- pubsub.RawTracer implementation ---

// AddPeer is invoked by the gossipsub event loop after a peer has been fully
// registered.
func (t *GossipTracer) AddPeer(p peer.ID, _ protocol.ID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.addedPeers[p] = true
	if ch, ok := t.addPeerWaiters[p]; ok {
		close(ch)
		delete(t.addPeerWaiters, p)
	}
}

func (t *GossipTracer) RemovePeer(_ peer.ID) {}

// Join is invoked when we locally subscribe to a topic (a mesh is created).
func (t *GossipTracer) Join(topic string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.joinedTopics[topic] = true
}

// Leave is invoked when we unsubscribe from a topic (mesh is torn down).
func (t *GossipTracer) Leave(topic string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.joinedTopics, topic)
}

// Graft is invoked when a peer is added to our mesh for a topic.
func (t *GossipTracer) Graft(p peer.ID, topic string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := topicPeer{topic: topic, peer: p}
	t.graftedPeers[key] = true
	if ch, ok := t.graftWaiters[key]; ok {
		close(ch)
		delete(t.graftWaiters, key)
	}
}

func (t *GossipTracer) Prune(_ peer.ID, _ string)                 {}
func (t *GossipTracer) ValidateMessage(_ *pubsub.Message)         {}
func (t *GossipTracer) DeliverMessage(_ *pubsub.Message)          {}
func (t *GossipTracer) RejectMessage(_ *pubsub.Message, _ string) {}
func (t *GossipTracer) DuplicateMessage(_ *pubsub.Message)        {}
func (t *GossipTracer) ThrottlePeer(_ peer.ID)                    {}
func (t *GossipTracer) RecvRPC(_ *pubsub.RPC)                     {}
func (t *GossipTracer) SendRPC(_ *pubsub.RPC, _ peer.ID)          {}
func (t *GossipTracer) DropRPC(_ *pubsub.RPC, _ peer.ID)          {}
func (t *GossipTracer) UndeliverableMessage(_ *pubsub.Message)    {}
