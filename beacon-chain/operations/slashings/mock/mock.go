package mock

import (
	"context"

	"github.com/theQRL/qrysm/beacon-chain/state"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	PendingAttSlashings  []*qrysmpb.AttesterSlashing
	PendingPropSlashings []*qrysmpb.ProposerSlashing
}

// PendingAttesterSlashings --
func (m *PoolMock) PendingAttesterSlashings(_ context.Context, _ state.ReadOnlyBeaconState, _ bool) []*qrysmpb.AttesterSlashing {
	return m.PendingAttSlashings
}

// PendingProposerSlashings --
func (m *PoolMock) PendingProposerSlashings(_ context.Context, _ state.ReadOnlyBeaconState, _ bool) []*qrysmpb.ProposerSlashing {
	return m.PendingPropSlashings
}

// InsertAttesterSlashing --
func (m *PoolMock) InsertAttesterSlashing(_ context.Context, _ state.ReadOnlyBeaconState, slashing *qrysmpb.AttesterSlashing) error {
	m.PendingAttSlashings = append(m.PendingAttSlashings, slashing)
	return nil
}

// InsertProposerSlashing --
func (m *PoolMock) InsertProposerSlashing(_ context.Context, _ state.ReadOnlyBeaconState, slashing *qrysmpb.ProposerSlashing) error {
	m.PendingPropSlashings = append(m.PendingPropSlashings, slashing)
	return nil
}

// MarkIncludedAttesterSlashing --
func (*PoolMock) MarkIncludedAttesterSlashing(_ *qrysmpb.AttesterSlashing) {
	panic("implement me")
}

// MarkIncludedProposerSlashing --
func (*PoolMock) MarkIncludedProposerSlashing(_ *qrysmpb.ProposerSlashing) {
	panic("implement me")
}
