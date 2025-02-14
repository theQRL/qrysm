package apimiddleware_test

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/testing/require"
)

func TestBeaconEndpointFactory_AllPathsRegistered(t *testing.T) {
	f := &apimiddleware.BeaconEndpointFactory{}

	for _, p := range f.Paths() {
		_, err := f.Create(p)
		require.NoError(t, err, "failed to register %s", p)
	}
}
