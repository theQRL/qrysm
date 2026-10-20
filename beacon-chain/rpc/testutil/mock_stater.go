package testutil

import (
	"context"

	"github.com/theQRL/qrysm/encoding/bytesutil"

	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/time/slots"
)

// MockStater is a fake implementation of lookup.Stater.
type MockStater struct {
	BeaconState       state.BeaconState
	StateProviderFunc func(ctx context.Context, stateId []byte) (state.BeaconState, error)
	BeaconStateRoot   []byte
	StatesBySlot      map[primitives.Slot]state.BeaconState
	StatesByEpoch     map[primitives.Epoch]state.BeaconState
	StatesByRoot      map[[32]byte]state.BeaconState
}

// State --
func (m *MockStater) State(ctx context.Context, id []byte) (state.BeaconState, error) {
	if m.StateProviderFunc != nil {
		return m.StateProviderFunc(ctx, id)
	}

	if m.BeaconState != nil {
		return m.BeaconState, nil
	}

	return m.StatesByRoot[bytesutil.ToBytes32(id)], nil
}

// StateRoot --
func (m *MockStater) StateRoot(context.Context, []byte) ([]byte, error) {
	return m.BeaconStateRoot, nil
}

// StateBySlot --
func (m *MockStater) StateBySlot(_ context.Context, s primitives.Slot) (state.BeaconState, error) {
	return m.StatesBySlot[s], nil
}

// StateByEpoch --
func (m *MockStater) StateByEpoch(_ context.Context, e primitives.Epoch) (state.BeaconState, error) {
	if st, ok := m.StatesByEpoch[e]; ok {
		return st, nil
	}
	if startSlot, err := slots.EpochStart(e); err == nil {
		if st, ok := m.StatesBySlot[startSlot]; ok {
			return st, nil
		}
	}
	if m.BeaconState != nil {
		return m.BeaconState, nil
	}
	return nil, nil
}
