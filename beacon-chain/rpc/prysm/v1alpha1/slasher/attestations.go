package slasher

import (
	"context"

	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsSlashableAttestation returns an attester slashing if an input
// attestation is found to be slashable.
func (s *Server) IsSlashableAttestation(
	ctx context.Context, req *zondpb.IndexedAttestation,
) (*zondpb.AttesterSlashingResponse, error) {
	attesterSlashings, err := s.SlashingChecker.IsSlashableAttestation(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine if attestation is slashable: %v", err)
	}
	if len(attesterSlashings) > 0 {
		return &zondpb.AttesterSlashingResponse{
			AttesterSlashings: attesterSlashings,
		}, nil
	}
	return &zondpb.AttesterSlashingResponse{}, nil
}

// HighestAttestations returns the highest source and target epochs attested for
// validator indices that have been observed by slasher.
func (s *Server) HighestAttestations(
	ctx context.Context, req *zondpb.HighestAttestationRequest,
) (*zondpb.HighestAttestationResponse, error) {
	valIndices := make([]primitives.ValidatorIndex, len(req.ValidatorIndices))
	for i, valIdx := range req.ValidatorIndices {
		valIndices[i] = primitives.ValidatorIndex(valIdx)
	}
	atts, err := s.SlashingChecker.HighestAttestations(ctx, valIndices)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get highest attestations: %v", err)
	}
	return &zondpb.HighestAttestationResponse{Attestations: atts}, nil
}
