package mock

import (
	"context"

	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

// PoolMock --
type PoolMock struct {
	AggregatedAtts []*zondpb.Attestation
}

// AggregateUnaggregatedAttestations --
func (*PoolMock) AggregateUnaggregatedAttestations(_ context.Context) error {
	panic("implement me")
}

// AggregateUnaggregatedAttestationsBySlotIndex --
func (*PoolMock) AggregateUnaggregatedAttestationsBySlotIndex(_ context.Context, _ primitives.Slot, _ primitives.CommitteeIndex) error {
	panic("implement me")
}

// SaveAggregatedAttestation --
func (*PoolMock) SaveAggregatedAttestation(_ *zondpb.Attestation) error {
	panic("implement me")
}

// SaveAggregatedAttestations --
func (m *PoolMock) SaveAggregatedAttestations(atts []*zondpb.Attestation) error {
	m.AggregatedAtts = append(m.AggregatedAtts, atts...)
	return nil
}

// AggregatedAttestations --
func (m *PoolMock) AggregatedAttestations() []*zondpb.Attestation {
	return m.AggregatedAtts
}

// AggregatedAttestationsBySlotIndex --
func (*PoolMock) AggregatedAttestationsBySlotIndex(_ context.Context, _ primitives.Slot, _ primitives.CommitteeIndex) []*zondpb.Attestation {
	panic("implement me")
}

// DeleteAggregatedAttestation --
func (*PoolMock) DeleteAggregatedAttestation(_ *zondpb.Attestation) error {
	panic("implement me")
}

// HasAggregatedAttestation --
func (*PoolMock) HasAggregatedAttestation(_ *zondpb.Attestation) (bool, error) {
	panic("implement me")
}

// AggregatedAttestationCount --
func (*PoolMock) AggregatedAttestationCount() int {
	panic("implement me")
}

// SaveUnaggregatedAttestation --
func (*PoolMock) SaveUnaggregatedAttestation(_ *zondpb.Attestation) error {
	panic("implement me")
}

// SaveUnaggregatedAttestations --
func (*PoolMock) SaveUnaggregatedAttestations(_ []*zondpb.Attestation) error {
	panic("implement me")
}

// UnaggregatedAttestations --
func (*PoolMock) UnaggregatedAttestations() ([]*zondpb.Attestation, error) {
	panic("implement me")
}

// UnaggregatedAttestationsBySlotIndex --
func (*PoolMock) UnaggregatedAttestationsBySlotIndex(_ context.Context, _ primitives.Slot, _ primitives.CommitteeIndex) []*zondpb.Attestation {
	panic("implement me")
}

// DeleteUnaggregatedAttestation --
func (*PoolMock) DeleteUnaggregatedAttestation(_ *zondpb.Attestation) error {
	panic("implement me")
}

// DeleteSeenUnaggregatedAttestations --
func (*PoolMock) DeleteSeenUnaggregatedAttestations() (int, error) {
	panic("implement me")
}

// UnaggregatedAttestationCount --
func (*PoolMock) UnaggregatedAttestationCount() int {
	panic("implement me")
}

// SaveBlockAttestation --
func (*PoolMock) SaveBlockAttestation(_ *zondpb.Attestation) error {
	panic("implement me")
}

// SaveBlockAttestations --
func (*PoolMock) SaveBlockAttestations(_ []*zondpb.Attestation) error {
	panic("implement me")
}

// BlockAttestations --
func (*PoolMock) BlockAttestations() []*zondpb.Attestation {
	panic("implement me")
}

// DeleteBlockAttestation --
func (*PoolMock) DeleteBlockAttestation(_ *zondpb.Attestation) error {
	panic("implement me")
}

// SaveForkchoiceAttestation --
func (*PoolMock) SaveForkchoiceAttestation(_ *zondpb.Attestation) error {
	panic("implement me")
}

// SaveForkchoiceAttestations --
func (*PoolMock) SaveForkchoiceAttestations(_ []*zondpb.Attestation) error {
	panic("implement me")
}

// ForkchoiceAttestations --
func (*PoolMock) ForkchoiceAttestations() []*zondpb.Attestation {
	panic("implement me")
}

// DeleteForkchoiceAttestation --
func (*PoolMock) DeleteForkchoiceAttestation(_ *zondpb.Attestation) error {
	panic("implement me")
}

// ForkchoiceAttestationCount --
func (*PoolMock) ForkchoiceAttestationCount() int {
	panic("implement me")
}
