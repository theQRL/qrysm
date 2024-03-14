package time_test

import (
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/time/slots"
)

func TestSlotToEpoch_OK(t *testing.T) {
	tests := []struct {
		slot  primitives.Slot
		epoch primitives.Epoch
	}{
		{slot: 0, epoch: 0},
		{slot: 199, epoch: 1},
		{slot: 256, epoch: 2},
		{slot: 512, epoch: 4},
		{slot: 768, epoch: 6},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.epoch, slots.ToEpoch(tt.slot), "ToEpoch(%d)", tt.slot)
	}
}

func TestCurrentEpoch_OK(t *testing.T) {
	tests := []struct {
		slot  primitives.Slot
		epoch primitives.Epoch
	}{
		{slot: 0, epoch: 0},
		{slot: 199, epoch: 1},
		{slot: 256, epoch: 2},
		{slot: 512, epoch: 4},
		{slot: 768, epoch: 6},
	}
	for _, tt := range tests {
		st, err := state_native.InitializeFromProtoCapella(&zond.BeaconStateCapella{Slot: tt.slot})
		require.NoError(t, err)
		assert.Equal(t, tt.epoch, time.CurrentEpoch(st), "ActiveCurrentEpoch(%d)", st.Slot())
	}
}

func TestPrevEpoch_OK(t *testing.T) {
	tests := []struct {
		slot  primitives.Slot
		epoch primitives.Epoch
	}{
		{slot: 0, epoch: 0},
		{slot: 0 + params.BeaconConfig().SlotsPerEpoch + 1, epoch: 0},
		{slot: 2 * params.BeaconConfig().SlotsPerEpoch, epoch: 1},
	}
	for _, tt := range tests {
		st, err := state_native.InitializeFromProtoCapella(&zond.BeaconStateCapella{Slot: tt.slot})
		require.NoError(t, err)
		assert.Equal(t, tt.epoch, time.PrevEpoch(st), "ActivePrevEpoch(%d)", st.Slot())
	}
}

func TestNextEpoch_OK(t *testing.T) {
	tests := []struct {
		slot  primitives.Slot
		epoch primitives.Epoch
	}{
		{slot: 0, epoch: primitives.Epoch(0/params.BeaconConfig().SlotsPerEpoch + 1)},
		{slot: 199, epoch: primitives.Epoch(0/params.BeaconConfig().SlotsPerEpoch + 2)},
		{slot: 256, epoch: primitives.Epoch(256/params.BeaconConfig().SlotsPerEpoch + 1)},
		{slot: 512, epoch: primitives.Epoch(512/params.BeaconConfig().SlotsPerEpoch + 1)},
		{slot: 768, epoch: primitives.Epoch(768/params.BeaconConfig().SlotsPerEpoch + 1)},
	}
	for _, tt := range tests {
		st, err := state_native.InitializeFromProtoCapella(&zond.BeaconStateCapella{Slot: tt.slot})
		require.NoError(t, err)
		assert.Equal(t, tt.epoch, time.NextEpoch(st), "NextEpoch(%d)", st.Slot())
	}
}

func TestCanProcessEpoch_TrueOnEpochsLastSlot(t *testing.T) {
	tests := []struct {
		slot            primitives.Slot
		canProcessEpoch bool
	}{
		{
			slot:            1,
			canProcessEpoch: false,
		}, {
			slot:            255,
			canProcessEpoch: true,
		},
		{
			slot:            256,
			canProcessEpoch: false,
		}, {
			slot:            511,
			canProcessEpoch: true,
		}, {
			slot:            4000000000,
			canProcessEpoch: false,
		},
	}

	for _, tt := range tests {
		b := &zond.BeaconStateCapella{Slot: tt.slot}
		s, err := state_native.InitializeFromProtoCapella(b)
		require.NoError(t, err)
		assert.Equal(t, tt.canProcessEpoch, time.CanProcessEpoch(s), "CanProcessEpoch(%d)", tt.slot)
	}
}
