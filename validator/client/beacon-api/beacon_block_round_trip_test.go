package beacon_api

import (
	"encoding/json"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/shared"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	test_helpers "github.com/theQRL/qrysm/validator/client/beacon-api/test-helpers"
)

// The beacon node renders and parses block JSON with the structs in
// beacon-chain/rpc/qrl/shared, while the REST validator client uses its own
// apimiddleware structs and hexutil helpers. The unit tests of each side only
// exercise that side against itself, so these tests push blocks through both:
// a field the node writes with one prefix (e.g. Q-prefixed execution
// addresses) and the client reads with another fails here instead of at
// proposal time.

// Produce-block direction: node proto -> node JSON -> client proto.
func TestBeaconBlockZond_RoundTrip_NodeJsonToClientProto(t *testing.T) {
	expected := test_helpers.GenerateProtoZondBeaconBlock()

	nodeJson, err := shared.BeaconBlockZondFromConsensus(expected)
	require.NoError(t, err)
	encoded, err := json.Marshal(nodeJson)
	require.NoError(t, err)

	clientJson := &apimiddleware.BeaconBlockZondJson{}
	require.NoError(t, json.Unmarshal(encoded, clientJson))

	actual, err := beaconApiBeaconBlockConverter{}.ConvertRESTZondBlockToProto(clientJson)
	require.NoError(t, err)
	assert.DeepEqual(t, expected, actual)
}

// Publish-block direction: client proto -> client JSON -> node proto.
func TestSignedBeaconBlockZond_RoundTrip_ClientJsonToNodeProto(t *testing.T) {
	expected := generateSignedZondBlock().Zond

	encoded, err := marshallBeaconBlockZond(expected)
	require.NoError(t, err)

	nodeJson := &shared.SignedBeaconBlockZond{}
	require.NoError(t, json.Unmarshal(encoded, nodeJson))

	generic, err := nodeJson.ToGeneric()
	require.NoError(t, err)
	assert.DeepEqual(t, expected, generic.GetZond())
}

func TestSignedBlindedBeaconBlockZond_RoundTrip_ClientJsonToNodeProto(t *testing.T) {
	expected := generateSignedBlindedZondBlock().BlindedZond

	encoded, err := marshallBeaconBlockBlindedZond(expected)
	require.NoError(t, err)

	nodeJson := &shared.SignedBlindedBeaconBlockZond{}
	require.NoError(t, json.Unmarshal(encoded, nodeJson))

	generic, err := nodeJson.ToGeneric()
	require.NoError(t, err)
	assert.DeepEqual(t, expected, generic.GetBlindedZond())
}
