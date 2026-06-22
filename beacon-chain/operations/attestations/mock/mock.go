package mock

import (
	"context"

	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// PoolMock --
type PoolMock struct {
	AggregatedAtts []*qrysmpb.Attestation
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
func (*PoolMock) SaveAggregatedAttestation(_ *qrysmpb.Attestation) error {
	panic("implement me")
}

// SaveAggregatedAttestations --
func (m *PoolMock) SaveAggregatedAttestations(atts []*qrysmpb.Attestation) error {
	m.AggregatedAtts = append(m.AggregatedAtts, atts...)
	return nil
}

// AggregatedAttestations --
func (m *PoolMock) AggregatedAttestations() []*qrysmpb.Attestation {
	return m.AggregatedAtts
}

// AggregatedAttestationsBySlotIndex --
func (*PoolMock) AggregatedAttestationsBySlotIndex(_ context.Context, _ primitives.Slot, _ primitives.CommitteeIndex) []*qrysmpb.Attestation {
	panic("implement me")
}

// DeleteAggregatedAttestation --
func (*PoolMock) DeleteAggregatedAttestation(_ *qrysmpb.Attestation) error {
	panic("implement me")
}

// HasAggregatedAttestation --
func (*PoolMock) HasAggregatedAttestation(_ *qrysmpb.Attestation) (bool, error) {
	panic("implement me")
}

// AggregatedAttestationCount --
func (*PoolMock) AggregatedAttestationCount() int {
	panic("implement me")
}

// SaveUnaggregatedAttestation --
func (*PoolMock) SaveUnaggregatedAttestation(_ *qrysmpb.Attestation) error {
	panic("implement me")
}

// SaveUnaggregatedAttestations --
func (*PoolMock) SaveUnaggregatedAttestations(_ []*qrysmpb.Attestation) error {
	panic("implement me")
}

// UnaggregatedAttestations --
func (*PoolMock) UnaggregatedAttestations() ([]*qrysmpb.Attestation, error) {
	panic("implement me")
}

// UnaggregatedAttestationsBySlotIndex --
func (*PoolMock) UnaggregatedAttestationsBySlotIndex(_ context.Context, _ primitives.Slot, _ primitives.CommitteeIndex) []*qrysmpb.Attestation {
	panic("implement me")
}

// DeleteUnaggregatedAttestation --
func (*PoolMock) DeleteUnaggregatedAttestation(_ *qrysmpb.Attestation) error {
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
func (*PoolMock) SaveBlockAttestation(_ *qrysmpb.Attestation) error {
	panic("implement me")
}

// SaveBlockAttestations --
func (*PoolMock) SaveBlockAttestations(_ []*qrysmpb.Attestation) error {
	panic("implement me")
}

// BlockAttestations --
func (*PoolMock) BlockAttestations() []*qrysmpb.Attestation {
	panic("implement me")
}

// DeleteBlockAttestation --
func (*PoolMock) DeleteBlockAttestation(_ *qrysmpb.Attestation) error {
	panic("implement me")
}

// SaveForkchoiceAttestation --
func (*PoolMock) SaveForkchoiceAttestation(_ *qrysmpb.Attestation) error {
	panic("implement me")
}

// SaveForkchoiceAttestations --
func (*PoolMock) SaveForkchoiceAttestations(_ []*qrysmpb.Attestation) error {
	panic("implement me")
}

// ForkchoiceAttestations --
func (*PoolMock) ForkchoiceAttestations() []*qrysmpb.Attestation {
	panic("implement me")
}

// DeleteForkchoiceAttestation --
func (*PoolMock) DeleteForkchoiceAttestation(_ *qrysmpb.Attestation) error {
	panic("implement me")
}

// ForkchoiceAttestationCount --
func (*PoolMock) ForkchoiceAttestationCount() int {
	panic("implement me")
}
