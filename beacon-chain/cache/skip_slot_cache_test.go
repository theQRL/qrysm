package cache_test

import (
	"context"
	"sync"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSkipSlotCache_RoundTrip(t *testing.T) {
	ctx := context.Background()
	c := cache.NewSkipSlotCache()

	r := [32]byte{'a'}
	s, err := c.Get(ctx, r)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), s, "Empty cache returned an object")

	require.NoError(t, c.MarkInProgress(r))

	s, err = state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: 10,
	})
	require.NoError(t, err)

	c.Put(ctx, r, s)
	c.MarkNotInProgress(r)

	res, err := c.Get(ctx, r)
	require.NoError(t, err)
	assert.DeepEqual(t, res.ToProto(), s.ToProto(), "Expected equal protos to return from cache")
}

func TestSkipSlotCache_DisabledAndEnabled(t *testing.T) {
	ctx := context.Background()
	c := cache.NewSkipSlotCache()

	r := [32]byte{'a'}
	c.Disable()

	require.NoError(t, c.MarkInProgress(r))

	c.Enable()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		// Get call will only terminate when
		// it is no longer in progress.
		obj, err := c.Get(ctx, r)
		require.NoError(t, err)
		require.Equal(t, state.BeaconState(nil), obj)
		wg.Done()
	}()

	c.MarkNotInProgress(r)
	wg.Wait()
}
