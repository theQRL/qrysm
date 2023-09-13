package mock

import (
	"github.com/cyyber/qrysm/v4/beacon-chain/state"
	"github.com/cyyber/qrysm/v4/consensus-types/primitives"
	eth "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	Changes []*eth.SignedDilithiumToExecutionChange
}

// PendingDilithiumToExecChanges --
func (m *PoolMock) PendingDilithiumToExecChanges() ([]*eth.SignedDilithiumToExecutionChange, error) {
	return m.Changes, nil
}

// DilithiumToExecChangesForInclusion --
func (m *PoolMock) DilithiumToExecChangesForInclusion(_ state.ReadOnlyBeaconState) ([]*eth.SignedDilithiumToExecutionChange, error) {
	return m.Changes, nil
}

// InsertDilithiumToExecChange --
func (m *PoolMock) InsertDilithiumToExecChange(change *eth.SignedDilithiumToExecutionChange) {
	m.Changes = append(m.Changes, change)
}

// MarkIncluded --
func (*PoolMock) MarkIncluded(_ *eth.SignedDilithiumToExecutionChange) {
	panic("implement me")
}

// ValidatorExists --
func (*PoolMock) ValidatorExists(_ primitives.ValidatorIndex) bool {
	panic("implement me")
}
