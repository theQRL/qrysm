package mock

import (
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	Exits []*qrysmpb.SignedVoluntaryExit
}

// PendingExits --
func (m *PoolMock) PendingExits() ([]*qrysmpb.SignedVoluntaryExit, error) {
	return m.Exits, nil
}

// ExitsForInclusion --
func (m *PoolMock) ExitsForInclusion(_ state.ReadOnlyBeaconState, _ primitives.Slot) ([]*qrysmpb.SignedVoluntaryExit, error) {
	return m.Exits, nil
}

// InsertVoluntaryExit --
func (m *PoolMock) InsertVoluntaryExit(exit *qrysmpb.SignedVoluntaryExit) {
	m.Exits = append(m.Exits, exit)
}

// MarkIncluded --
func (*PoolMock) MarkIncluded(_ *qrysmpb.SignedVoluntaryExit) {
	panic("implement me")
}
