package mock

import (
	"context"

	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

// PoolMock is a fake implementation of PoolManager.
type PoolMock struct {
	PendingAttSlashings  []*zondpb.AttesterSlashing
	PendingPropSlashings []*zondpb.ProposerSlashing
}

// PendingAttesterSlashings --
func (m *PoolMock) PendingAttesterSlashings(_ context.Context, _ state.ReadOnlyBeaconState, _ bool) []*zondpb.AttesterSlashing {
	return m.PendingAttSlashings
}

// PendingProposerSlashings --
func (m *PoolMock) PendingProposerSlashings(_ context.Context, _ state.ReadOnlyBeaconState, _ bool) []*zondpb.ProposerSlashing {
	return m.PendingPropSlashings
}

// InsertAttesterSlashing --
func (m *PoolMock) InsertAttesterSlashing(_ context.Context, _ state.ReadOnlyBeaconState, slashing *zondpb.AttesterSlashing) error {
	m.PendingAttSlashings = append(m.PendingAttSlashings, slashing)
	return nil
}

// InsertProposerSlashing --
func (m *PoolMock) InsertProposerSlashing(_ context.Context, _ state.ReadOnlyBeaconState, slashing *zondpb.ProposerSlashing) error {
	m.PendingPropSlashings = append(m.PendingPropSlashings, slashing)
	return nil
}

// MarkIncludedAttesterSlashing --
func (*PoolMock) MarkIncludedAttesterSlashing(_ *zondpb.AttesterSlashing) {
	panic("implement me")
}

// MarkIncludedProposerSlashing --
func (*PoolMock) MarkIncludedProposerSlashing(_ *zondpb.ProposerSlashing) {
	panic("implement me")
}
