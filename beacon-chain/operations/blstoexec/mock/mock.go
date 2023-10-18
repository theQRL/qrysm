package mock

import (
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	Changes []*zond.SignedDilithiumToExecutionChange
}

// PendingDilithiumToExecChanges --
func (m *PoolMock) PendingDilithiumToExecChanges() ([]*zond.SignedDilithiumToExecutionChange, error) {
	return m.Changes, nil
}

// DilithiumToExecChangesForInclusion --
func (m *PoolMock) DilithiumToExecChangesForInclusion(_ state.ReadOnlyBeaconState) ([]*zond.SignedDilithiumToExecutionChange, error) {
	return m.Changes, nil
}

// InsertDilithiumToExecChange --
func (m *PoolMock) InsertDilithiumToExecChange(change *zond.SignedDilithiumToExecutionChange) {
	m.Changes = append(m.Changes, change)
}

// MarkIncluded --
func (*PoolMock) MarkIncluded(_ *zond.SignedDilithiumToExecutionChange) {
	panic("implement me")
}

// ValidatorExists --
func (*PoolMock) ValidatorExists(_ primitives.ValidatorIndex) bool {
	panic("implement me")
}
