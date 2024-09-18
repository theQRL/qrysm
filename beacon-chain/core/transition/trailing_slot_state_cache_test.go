package transition_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestTrailingSlotState_RoundTrip(t *testing.T) {
	ctx := context.Background()
	r := []byte{'a'}
	s := transition.NextSlotState(r, 0)
	require.Equal(t, nil, s)

	s, _ = util.DeterministicGenesisStateCapella(t, 1)
	require.NoError(t, transition.UpdateNextSlotCache(ctx, r, s))
	s = transition.NextSlotState(r, 1)
	require.Equal(t, primitives.Slot(1), s.Slot())

	lastRoot, lastState := transition.LastCachedState()
	require.DeepEqual(t, r, lastRoot)
	require.Equal(t, s.Slot(), lastState.Slot())

	require.NoError(t, transition.UpdateNextSlotCache(ctx, r, s))
	s = transition.NextSlotState(r, 2)
	require.Equal(t, primitives.Slot(2), s.Slot())

	lastRoot, lastState = transition.LastCachedState()
	require.DeepEqual(t, r, lastRoot)
	require.Equal(t, s.Slot(), lastState.Slot())
}

func TestTrailingSlotState_StateAdvancedBeyondRequest(t *testing.T) {
	ctx := context.Background()
	r := []byte{'a'}
	s := transition.NextSlotState(r, 0)
	require.Equal(t, nil, s)

	s, _ = util.DeterministicGenesisStateCapella(t, 1)
	assert.NoError(t, s.SetSlot(2))
	require.NoError(t, transition.UpdateNextSlotCache(ctx, r, s))
	s = transition.NextSlotState(r, 1)
	require.Equal(t, nil, s)
}
