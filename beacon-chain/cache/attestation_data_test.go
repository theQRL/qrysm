package cache_test

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/cache"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"google.golang.org/protobuf/proto"
)

func TestAttestationCache_RoundTrip(t *testing.T) {
	ctx := context.Background()
	c := cache.NewAttestationCache()

	req := &qrysmpb.AttestationDataRequest{
		CommitteeIndex: 0,
		Slot:           1,
	}

	response, err := c.Get(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, (*qrysmpb.AttestationData)(nil), response)

	assert.NoError(t, c.MarkInProgress(req))

	res := &qrysmpb.AttestationData{
		Target: &qrysmpb.Checkpoint{Epoch: 5, Root: make([]byte, 32)},
	}

	assert.NoError(t, c.Put(ctx, req, res))
	assert.NoError(t, c.MarkNotInProgress(req))

	response, err = c.Get(ctx, req)
	assert.NoError(t, err)

	if !proto.Equal(response, res) {
		t.Error("Expected equal protos to return from cache")
	}
}

func TestAttestationCache_Clear(t *testing.T) {
	ctx := context.Background()
	c := cache.NewAttestationCache()
	req := &qrysmpb.AttestationDataRequest{Slot: 5, CommitteeIndex: 0}
	require.NoError(t, c.Put(ctx, req, &qrysmpb.AttestationData{Slot: 5, BeaconBlockRoot: make([]byte, 32)}))
	res, err := c.Get(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)

	c.Clear()

	res, err = c.Get(ctx, req)
	require.NoError(t, err)
	require.Equal(t, (*qrysmpb.AttestationData)(nil), res)

	// The cache keeps working after being cleared.
	require.NoError(t, c.Put(ctx, req, &qrysmpb.AttestationData{Slot: 5, BeaconBlockRoot: make([]byte, 32)}))
	res, err = c.Get(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestAttestationCache_ClearInProgress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		c := cache.NewAttestationCache()
		req := &qrysmpb.AttestationDataRequest{Slot: 5}
		otherCommittee := &qrysmpb.AttestationDataRequest{Slot: 5, CommitteeIndex: 1}
		require.NoError(t, c.MarkInProgress(req))
		done := make(chan struct{})
		go func() {
			defer close(done)
			res, err := c.Get(ctx, otherCommittee)
			assert.NoError(t, err)
			assert.Equal(t, (*qrysmpb.AttestationData)(nil), res)
		}()
		synctest.Wait()
		c.Clear()
		require.ErrorIs(t, c.MarkInProgress(otherCommittee), cache.ErrAlreadyInProgress)
		require.ErrorIs(t, c.Put(ctx, req, &qrysmpb.AttestationData{Slot: 5}), cache.ErrAttestationDataStale)
		require.NoError(t, c.MarkNotInProgress(req))
		<-done
		// A new producer must be able to populate the same slot after the old
		// producer has released it, without inheriting the invalidation.
		require.NoError(t, c.MarkInProgress(otherCommittee))
		fresh := &qrysmpb.AttestationData{Slot: 5, BeaconBlockRoot: []byte{'n'}}
		require.NoError(t, c.Put(ctx, otherCommittee, fresh))
		require.NoError(t, c.MarkNotInProgress(otherCommittee))
		res, err := c.Get(ctx, req)
		require.NoError(t, err)
		require.DeepEqual(t, fresh, res)
	})
}
