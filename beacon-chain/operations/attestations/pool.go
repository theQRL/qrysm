package attestations

import (
	"context"

	"github.com/theQRL/qrysm/beacon-chain/operations/attestations/kv"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// Pool defines the necessary methods for Qrysm attestations pool to serve
// fork choice and validators. In the current design, aggregated attestations
// are used by proposer actor. Unaggregated attestations are used by
// aggregator actor.
type Pool interface {
	// For Aggregated attestations
	AggregateUnaggregatedAttestations(ctx context.Context) error
	SaveAggregatedAttestation(att *zondpb.Attestation) error
	SaveAggregatedAttestations(atts []*zondpb.Attestation) error
	AggregatedAttestations() []*zondpb.Attestation
	AggregatedAttestationsBySlotIndex(ctx context.Context, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) []*zondpb.Attestation
	DeleteAggregatedAttestation(att *zondpb.Attestation) error
	HasAggregatedAttestation(att *zondpb.Attestation) (bool, error)
	AggregatedAttestationCount() int
	// For unaggregated attestations.
	SaveUnaggregatedAttestation(att *zondpb.Attestation) error
	SaveUnaggregatedAttestations(atts []*zondpb.Attestation) error
	UnaggregatedAttestations() ([]*zondpb.Attestation, error)
	UnaggregatedAttestationsBySlotIndex(ctx context.Context, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) []*zondpb.Attestation
	DeleteUnaggregatedAttestation(att *zondpb.Attestation) error
	DeleteSeenUnaggregatedAttestations() (int, error)
	UnaggregatedAttestationCount() int
	// For attestations that were included in the block.
	SaveBlockAttestation(att *zondpb.Attestation) error
	BlockAttestations() []*zondpb.Attestation
	DeleteBlockAttestation(att *zondpb.Attestation) error
	// For attestations to be passed to fork choice.
	SaveForkchoiceAttestation(att *zondpb.Attestation) error
	SaveForkchoiceAttestations(atts []*zondpb.Attestation) error
	ForkchoiceAttestations() []*zondpb.Attestation
	DeleteForkchoiceAttestation(att *zondpb.Attestation) error
	ForkchoiceAttestationCount() int
}

// NewPool initializes a new attestation pool.
func NewPool() *kv.AttCaches {
	return kv.NewAttCaches()
}
