package debug

import (
	"context"

	"github.com/theQRL/qrysm/beacon-chain/rpc/zond/helpers"
	"github.com/theQRL/qrysm/proto/migration"
	zondpbv1 "github.com/theQRL/qrysm/proto/zond/v1"
	"github.com/theQRL/qrysm/runtime/version"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetBeaconState returns the full beacon state for a given state ID.
func (ds *Server) GetBeaconState(ctx context.Context, req *zondpbv1.BeaconStateRequest) (*zondpbv1.BeaconStateResponse, error) {
	ctx, span := trace.StartSpan(ctx, "debug.GetBeaconState")
	defer span.End()

	beaconSt, err := ds.Stater.State(ctx, req.StateId)
	if err != nil {
		return nil, helpers.PrepareStateFetchGRPCError(err)
	}
	isOptimistic, err := helpers.IsOptimistic(ctx, req.StateId, ds.OptimisticModeFetcher, ds.Stater, ds.ChainInfoFetcher, ds.BeaconDB)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not check if slot's block is optimistic: %v", err)
	}
	blockRoot, err := beaconSt.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not calculate root of latest block header")
	}
	isFinalized := ds.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	switch beaconSt.Version() {
	case version.Capella:
		protoState, err := migration.BeaconStateCapellaToProto(beaconSt)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not convert state to proto: %v", err)
		}
		return &zondpbv1.BeaconStateResponse{
			Version: zondpbv1.Version_CAPELLA,
			Data: &zondpbv1.BeaconStateContainer{
				State: &zondpbv1.BeaconStateContainer_CapellaState{CapellaState: protoState},
			},
			ExecutionOptimistic: isOptimistic,
			Finalized:           isFinalized,
		}, nil
	default:
		return nil, status.Error(codes.Internal, "Unsupported state version")
	}
}

// GetBeaconStateSSZ returns the SSZ-serialized version of the full beacon state object for given state ID.
func (ds *Server) GetBeaconStateSSZ(ctx context.Context, req *zondpbv1.BeaconStateRequest) (*zondpbv1.SSZContainer, error) {
	ctx, span := trace.StartSpan(ctx, "debug.GetBeaconStateSSZ")
	defer span.End()

	st, err := ds.Stater.State(ctx, req.StateId)
	if err != nil {
		return nil, helpers.PrepareStateFetchGRPCError(err)
	}

	sszState, err := st.MarshalSSZ()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not marshal state into SSZ: %v", err)
	}
	var ver zondpbv1.Version
	switch st.Version() {
	case version.Capella:
		ver = zondpbv1.Version_CAPELLA
	default:
		return nil, status.Error(codes.Internal, "Unsupported state version")
	}

	return &zondpbv1.SSZContainer{Data: sszState, Version: ver}, nil
}

// ListForkChoiceHeads retrieves the leaves of the current fork choice tree.
func (ds *Server) ListForkChoiceHeads(ctx context.Context, _ *emptypb.Empty) (*zondpbv1.ForkChoiceHeadsResponse, error) {
	ctx, span := trace.StartSpan(ctx, "debug.ListForkChoiceHeads")
	defer span.End()

	headRoots, headSlots := ds.HeadFetcher.ChainHeads()
	resp := &zondpbv1.ForkChoiceHeadsResponse{
		Data: make([]*zondpbv1.ForkChoiceHead, len(headRoots)),
	}
	for i := range headRoots {
		isOptimistic, err := ds.OptimisticModeFetcher.IsOptimisticForRoot(ctx, headRoots[i])
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not check if head is optimistic: %v", err)
		}
		resp.Data[i] = &zondpbv1.ForkChoiceHead{
			Root:                headRoots[i][:],
			Slot:                headSlots[i],
			ExecutionOptimistic: isOptimistic,
		}
	}

	return resp, nil
}

// GetForkChoice returns a dump fork choice store.
func (ds *Server) GetForkChoice(ctx context.Context, _ *emptypb.Empty) (*zondpbv1.ForkChoiceDump, error) {
	return ds.ForkchoiceFetcher.ForkChoiceDump(ctx)
}
