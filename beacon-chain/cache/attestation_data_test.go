package cache_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/cache"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"google.golang.org/protobuf/proto"
)

func TestAttestationCache_RoundTrip(t *testing.T) {
	ctx := context.Background()
	c := cache.NewAttestationCache()

	req := &zondpb.AttestationDataRequest{
		CommitteeIndex: 0,
		Slot:           1,
	}

	response, err := c.Get(ctx, req)
	assert.NoError(t, err)
	assert.Equal(t, (*zondpb.AttestationData)(nil), response)

	assert.NoError(t, c.MarkInProgress(req))

	res := &zondpb.AttestationData{
		Target: &zondpb.Checkpoint{Epoch: 5, Root: make([]byte, 32)},
	}

	assert.NoError(t, c.Put(ctx, req, res))
	assert.NoError(t, c.MarkNotInProgress(req))

	response, err = c.Get(ctx, req)
	assert.NoError(t, err)

	if !proto.Equal(response, res) {
		t.Error("Expected equal protos to return from cache")
	}
}
