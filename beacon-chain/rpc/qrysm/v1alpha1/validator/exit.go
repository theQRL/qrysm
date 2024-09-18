package validator

import (
	"context"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	opfeed "github.com/theQRL/qrysm/beacon-chain/core/feed/operation"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ProposeExit proposes an exit for a validator.
func (vs *Server) ProposeExit(ctx context.Context, req *zondpb.SignedVoluntaryExit) (*zondpb.ProposeExitResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "nil request")
	}
	s, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get head state: %v", err)
	}
	if req.Exit == nil {
		return nil, status.Error(codes.InvalidArgument, "voluntary exit does not exist")
	}
	if req.Signature == nil || len(req.Signature) != field_params.DilithiumSignatureLength {
		return nil, status.Error(codes.InvalidArgument, "invalid signature provided")
	}

	// Confirm the validator is eligible to exit with the parameters provided.
	val, err := s.ValidatorAtIndexReadOnly(req.Exit.ValidatorIndex)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "validator index exceeds validator set length")
	}

	if err := blocks.VerifyExitAndSignature(val, s, req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	vs.OperationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.ExitReceived,
		Data: &opfeed.ExitReceivedData{
			Exit: req,
		},
	})

	vs.ExitPool.InsertVoluntaryExit(req)

	r, err := req.Exit.HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get tree hash of exit: %v", err)
	}

	return &zondpb.ProposeExitResponse{
		ExitRoot: r[:],
	}, vs.P2P.Broadcast(ctx, req)
}
