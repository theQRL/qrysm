package cache

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"google.golang.org/protobuf/proto"
)

func TestCheckpointStateCache_StateByCheckpoint(t *testing.T) {
	cache := NewCheckpointStateCache()

	cp1 := &qrysmpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'A'}, 32)}
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		GenesisValidatorsRoot: params.BeaconConfig().ZeroHash[:],
		Slot:                  64,
	})
	require.NoError(t, err)

	s, err := cache.StateByCheckpoint(cp1)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), s, "Expected state not to exist in empty cache")

	require.NoError(t, cache.AddCheckpointState(cp1, st))

	s, err = cache.StateByCheckpoint(cp1)
	require.NoError(t, err)

	pbState1, err := state_native.ProtobufBeaconStateZond(s.ToProtoUnsafe())
	require.NoError(t, err)
	pbstate, err := state_native.ProtobufBeaconStateZond(st.ToProtoUnsafe())
	require.NoError(t, err)
	if !proto.Equal(pbState1, pbstate) {
		t.Error("incorrectly cached state")
	}

	cp2 := &qrysmpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{'B'}, 32)}
	st2, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: 128,
	})
	require.NoError(t, err)
	require.NoError(t, cache.AddCheckpointState(cp2, st2))

	s, err = cache.StateByCheckpoint(cp2)
	require.NoError(t, err)
	assert.DeepEqual(t, st2.ToProto(), s.ToProto(), "incorrectly cached state")

	s, err = cache.StateByCheckpoint(cp1)
	require.NoError(t, err)
	assert.DeepEqual(t, st.ToProto(), s.ToProto(), "incorrectly cached state")
}

func TestCheckpointStateCache_MaxSize(t *testing.T) {
	c := NewCheckpointStateCache()
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: 0,
	})
	require.NoError(t, err)

	for i := uint64(0); i < uint64(maxCheckpointStateSize+100); i++ {
		require.NoError(t, st.SetSlot(primitives.Slot(i)))
		require.NoError(t, c.AddCheckpointState(&qrysmpb.Checkpoint{Epoch: primitives.Epoch(i), Root: make([]byte, 32)}, st))
	}

	assert.Equal(t, maxCheckpointStateSize, len(c.cache.Keys()))
}

func TestCheckpointStateCache_EvictUpTo(t *testing.T) {
	c := NewCheckpointStateCache()

	cp1 := &qrysmpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'A'}, 32)}
	st1, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	require.NoError(t, st1.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	require.NoError(t, c.AddCheckpointState(cp1, st1))

	cp2 := &qrysmpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{'B'}, 32)}
	st2, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	require.NoError(t, st2.SetSlot(params.BeaconConfig().SlotsPerEpoch.Mul(2)))
	require.NoError(t, c.AddCheckpointState(cp2, st2))

	cp5 := &qrysmpb.Checkpoint{Epoch: 5, Root: bytesutil.PadTo([]byte{'C'}, 32)}
	st5, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	require.NoError(t, st5.SetSlot(params.BeaconConfig().SlotsPerEpoch.Mul(5)))
	require.NoError(t, c.AddCheckpointState(cp5, st5))

	evicted := c.EvictUpTo(3)
	assert.Equal(t, 2, evicted)
	assert.Equal(t, 1, len(c.cache.Keys()))

	got, err := c.StateByCheckpoint(cp1)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), got)

	got, err = c.StateByCheckpoint(cp2)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), got)

	got, err = c.StateByCheckpoint(cp5)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, st5.Slot(), got.Slot())
}
