package components_test

/*
import (
	"context"
	"testing"
	"time"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/testing/endtoend/components"
	e2eparams "github.com/theQRL/qrysm/testing/endtoend/params"
	"github.com/theQRL/qrysm/testing/require"
)

func TestWeb3RemoteSigner_StartsAndReturnsPublicKeys(t *testing.T) {
	require.NoError(t, e2eparams.Init(t, 0))

	wsc := components.NewWeb3RemoteSigner()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		if err := wsc.Start(ctx); err != nil {
			t.Error(err)
			panic(err)
		}
	}()

	select {
	case <-ctx.Done():
		t.Fatal("Web3RemoteSigner did not start within timeout")
	case <-wsc.Started():
		t.Log("Web3RemoteSigner started")
		break
	}

	time.Sleep(10 * time.Second)

	keys, err := wsc.PublicKeys(ctx)
	require.NoError(t, err)

	if uint64(len(keys)) != params.BeaconConfig().MinGenesisActiveValidatorCount {
		t.Fatalf("Expected %d keys, got %d", params.BeaconConfig().MinGenesisActiveValidatorCount, len(keys))
	}
}
*/
