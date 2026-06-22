package attestations

import (
	"context"

	"github.com/theQRL/qrysm/beacon-chain/operations/attestations/kv"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// Pool defines the necessary methods for Qrysm attestations pool to serve
// fork choice and validators. In the current design, aggregated attestations
// are used by proposer actor. Unaggregated attestations are used by
// aggregator actor.
type Pool interface {
	// For Aggregated attestations
	AggregateUnaggregatedAttestations(ctx context.Context) error
	SaveAggregatedAttestation(att *qrysmpb.Attestation) error
	SaveAggregatedAttestations(atts []*qrysmpb.Attestation) error
	AggregatedAttestations() []*qrysmpb.Attestation
	AggregatedAttestationsBySlotIndex(ctx context.Context, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) []*qrysmpb.Attestation
	DeleteAggregatedAttestation(att *qrysmpb.Attestation) error
	HasAggregatedAttestation(att *qrysmpb.Attestation) (bool, error)
	AggregatedAttestationCount() int
	// For unaggregated attestations.
	SaveUnaggregatedAttestation(att *qrysmpb.Attestation) error
	SaveUnaggregatedAttestations(atts []*qrysmpb.Attestation) error
	UnaggregatedAttestations() ([]*qrysmpb.Attestation, error)
	UnaggregatedAttestationsBySlotIndex(ctx context.Context, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) []*qrysmpb.Attestation
	DeleteUnaggregatedAttestation(att *qrysmpb.Attestation) error
	DeleteSeenUnaggregatedAttestations() (int, error)
	UnaggregatedAttestationCount() int
	// For attestations that were included in the block.
	SaveBlockAttestation(att *qrysmpb.Attestation) error
	BlockAttestations() []*qrysmpb.Attestation
	DeleteBlockAttestation(att *qrysmpb.Attestation) error
	// For attestations to be passed to fork choice.
	SaveForkchoiceAttestation(att *qrysmpb.Attestation) error
	SaveForkchoiceAttestations(atts []*qrysmpb.Attestation) error
	ForkchoiceAttestations() []*qrysmpb.Attestation
	DeleteForkchoiceAttestation(att *qrysmpb.Attestation) error
	ForkchoiceAttestationCount() int
}

// NewPool initializes a new attestation pool.
func NewPool() *kv.AttCaches {
	return kv.NewAttCaches()
}
