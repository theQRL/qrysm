package mock

import (
	"context"

	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/primitives"
)

// MockStateManager is a fake implementation of StateManager.
type MockStateManager struct {
	StatesByRoot map[[32]byte]state.BeaconState
	StatesBySlot map[primitives.Slot]state.BeaconState
}

// NewMockService --
func NewMockService() *MockStateManager {
	return &MockStateManager{
		StatesByRoot: make(map[[32]byte]state.BeaconState),
		StatesBySlot: make(map[primitives.Slot]state.BeaconState),
	}
}

// StateByRootIfCached returns a snapshot of the mock's cached state, if present.
func (m *MockStateManager) StateByRootIfCached(root [32]byte) state.BeaconState {
	st := m.StatesByRoot[root]
	if st == nil {
		return nil
	}
	return st.Copy()
}

// StateByRootIfCachedNoCopy --
func (_ *MockStateManager) StateByRootIfCachedNoCopy(_ [32]byte) state.BeaconState {
	panic("implement me") // lint:nopanic
}

// Resume --
func (_ *MockStateManager) Resume(_ context.Context, _ state.BeaconState) (state.BeaconState, error) {
	panic("implement me") // lint:nopanic
}

// SaveFinalizedState --
func (_ *MockStateManager) SaveFinalizedState(_ primitives.Slot, _ [32]byte, _ state.BeaconState) {
	panic("implement me") // lint:nopanic
}

// MigrateToCold --
func (_ *MockStateManager) MigrateToCold(_ context.Context, _ [32]byte) error {
	panic("implement me") // lint:nopanic
}

// HasState --
func (_ *MockStateManager) HasState(_ context.Context, _ [32]byte) (bool, error) {
	panic("implement me") // lint:nopanic
}

// StateByRoot --
func (m *MockStateManager) StateByRoot(_ context.Context, blockRoot [32]byte) (state.BeaconState, error) {
	return m.StatesByRoot[blockRoot], nil
}

// BalancesByCheckpoint --
func (*MockStateManager) BalancesByCheckpoint(_ context.Context, _ *forkchoicetypes.Checkpoint) (*forkchoicetypes.JustifiedBalances, error) {
	return &forkchoicetypes.JustifiedBalances{Balances: []uint64{}}, nil
}

// StateByRootInitialSync --
func (_ *MockStateManager) StateByRootInitialSync(_ context.Context, _ [32]byte) (state.BeaconState, error) {
	panic("implement me") // lint:nopanic
}

// StateBySlot --
func (m *MockStateManager) StateBySlot(_ context.Context, slot primitives.Slot) (state.BeaconState, error) {
	return m.StatesBySlot[slot], nil
}

// SaveState --
func (_ *MockStateManager) SaveState(_ context.Context, _ [32]byte, _ state.BeaconState) error {
	panic("implement me") // lint:nopanic
}

// ForceCheckpoint --
func (_ *MockStateManager) ForceCheckpoint(_ context.Context, _ []byte) error {
	panic("implement me") // lint:nopanic
}

// EnableSaveHotStateToDB --
func (_ *MockStateManager) EnableSaveHotStateToDB(_ context.Context) {
	panic("implement me") // lint:nopanic
}

// DisableSaveHotStateToDB --
func (_ *MockStateManager) DisableSaveHotStateToDB(_ context.Context) error {
	panic("implement me") // lint:nopanic
}

// AddStateForRoot --
func (m *MockStateManager) AddStateForRoot(state state.BeaconState, blockRoot [32]byte) {
	m.StatesByRoot[blockRoot] = state
}

// AddStateForSlot --
func (m *MockStateManager) AddStateForSlot(state state.BeaconState, slot primitives.Slot) {
	m.StatesBySlot[slot] = state
}

// DeleteStateFromCaches --
func (m *MockStateManager) DeleteStateFromCaches(context.Context, [32]byte) error {
	return nil
}
