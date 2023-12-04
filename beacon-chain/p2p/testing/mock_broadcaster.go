package testing

import (
	"context"

	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// MockBroadcaster implements p2p.Broadcaster for testing.
type MockBroadcaster struct {
	BroadcastCalled       bool
	BroadcastMessages     []proto.Message
	BroadcastAttestations []*zondpb.Attestation
}

// Broadcast records a broadcast occurred.
func (m *MockBroadcaster) Broadcast(_ context.Context, msg proto.Message) error {
	m.BroadcastCalled = true
	m.BroadcastMessages = append(m.BroadcastMessages, msg)
	return nil
}

// BroadcastAttestation records a broadcast occurred.
func (m *MockBroadcaster) BroadcastAttestation(_ context.Context, _ uint64, a *zondpb.Attestation) error {
	m.BroadcastCalled = true
	m.BroadcastAttestations = append(m.BroadcastAttestations, a)
	return nil
}

// BroadcastSyncCommitteeMessage records a broadcast occurred.
func (m *MockBroadcaster) BroadcastSyncCommitteeMessage(_ context.Context, _ uint64, _ *zondpb.SyncCommitteeMessage) error {
	m.BroadcastCalled = true
	return nil
}

// BroadcastBlob broadcasts a blob for mock.
func (m *MockBroadcaster) BroadcastBlob(context.Context, uint64, *zondpb.SignedBlobSidecar) error {
	m.BroadcastCalled = true
	return nil
}
