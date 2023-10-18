package slasher

import (
	"context"

	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsSlashableBlock returns a proposer slashing if an input
// signed beacon block header is found to be slashable.
func (s *Server) IsSlashableBlock(
	ctx context.Context, req *zondpb.SignedBeaconBlockHeader,
) (*zondpb.ProposerSlashingResponse, error) {
	proposerSlashing, err := s.SlashingChecker.IsSlashableBlock(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine if block is slashable: %v", err)
	}
	if proposerSlashing == nil {
		return &zondpb.ProposerSlashingResponse{
			ProposerSlashings: []*zondpb.ProposerSlashing{},
		}, nil
	}
	return &zondpb.ProposerSlashingResponse{
		ProposerSlashings: []*zondpb.ProposerSlashing{proposerSlashing},
	}, nil
}
