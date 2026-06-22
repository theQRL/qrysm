package debug

import (
	"context"
	"testing"

	mockP2p "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

// TODO(now.youtrack.cloud/issue/TQ-18): fails sometimes
/*
func TestDebugServer_GetPeer(t *testing.T) {
	peersProvider := &mockP2p.MockPeersProvider{}
	mP2P := mockP2p.NewTestP2P(t)
	ds := &Server{
		PeersFetcher: peersProvider,
		PeerManager:  &mockP2p.MockPeerManager{BHost: mP2P.BHost},
	}
	firstPeer := peersProvider.Peers().All()[0]

	res, err := ds.GetPeer(context.Background(), &qrysmpb.PeerRequest{PeerId: firstPeer.String()})
	require.NoError(t, err)
	require.Equal(t, firstPeer.String(), res.PeerId, "Unexpected peer ID")

	assert.Equal(t, int(qrysmpb.PeerDirection_INBOUND), int(res.Direction), "Expected 1st peer to be an inbound connection")
	assert.Equal(t, qrysmpb.ConnectionState_CONNECTED, res.ConnectionState, "Expected peer to be connected")
}
*/

func TestDebugServer_ListPeers(t *testing.T) {
	peersProvider := &mockP2p.MockPeersProvider{}
	mP2P := mockP2p.NewTestP2P(t)
	ds := &Server{
		PeersFetcher: peersProvider,
		PeerManager:  &mockP2p.MockPeerManager{BHost: mP2P.BHost},
	}

	res, err := ds.ListPeers(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	assert.Equal(t, 2, len(res.Responses))

	direction1 := res.Responses[0].Direction
	direction2 := res.Responses[1].Direction
	assert.Equal(t,
		true,
		direction1 == qrysmpb.PeerDirection_INBOUND || direction2 == qrysmpb.PeerDirection_INBOUND,
		"Expected an inbound peer")
	assert.Equal(t,
		true,
		direction1 == qrysmpb.PeerDirection_OUTBOUND || direction2 == qrysmpb.PeerDirection_OUTBOUND,
		"Expected an outbound peer")
	if len(res.Responses[0].ListeningAddresses) == 0 {
		t.Errorf("Expected 1st peer to have a multiaddress, instead they have no addresses")
	}
	if len(res.Responses[1].ListeningAddresses) == 0 {
		t.Errorf("Expected 2nd peer to have a multiaddress, instead they have no addresses")
	}
}
