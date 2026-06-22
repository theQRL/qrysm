package debug

import (
	"context"

	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/helpers"
	"github.com/theQRL/qrysm/proto/migration"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	"github.com/theQRL/qrysm/runtime/version"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// GetBeaconState returns the full beacon state for a given state ID.
func (ds *Server) GetBeaconState(ctx context.Context, req *qrlpb.BeaconStateRequest) (*qrlpb.BeaconStateResponse, error) {
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
	case version.Zond:
		protoState, err := migration.BeaconStateZondToProto(beaconSt)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not convert state to proto: %v", err)
		}
		return &qrlpb.BeaconStateResponse{
			Version: qrlpb.Version_ZOND,
			Data: &qrlpb.BeaconStateContainer{
				State: &qrlpb.BeaconStateContainer_ZondState{ZondState: protoState},
			},
			ExecutionOptimistic: isOptimistic,
			Finalized:           isFinalized,
		}, nil
	default:
		return nil, status.Error(codes.Internal, "Unsupported state version")
	}
}

// GetBeaconStateSSZ returns the SSZ-serialized version of the full beacon state object for given state ID.
func (ds *Server) GetBeaconStateSSZ(ctx context.Context, req *qrlpb.BeaconStateRequest) (*qrlpb.SSZContainer, error) {
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
	var ver qrlpb.Version
	switch st.Version() {
	case version.Zond:
		ver = qrlpb.Version_ZOND
	default:
		return nil, status.Error(codes.Internal, "Unsupported state version")
	}

	return &qrlpb.SSZContainer{Data: sszState, Version: ver}, nil
}

// ListForkChoiceHeads retrieves the leaves of the current fork choice tree.
func (ds *Server) ListForkChoiceHeads(ctx context.Context, _ *emptypb.Empty) (*qrlpb.ForkChoiceHeadsResponse, error) {
	ctx, span := trace.StartSpan(ctx, "debug.ListForkChoiceHeads")
	defer span.End()

	headRoots, headSlots := ds.HeadFetcher.ChainHeads()
	resp := &qrlpb.ForkChoiceHeadsResponse{
		Data: make([]*qrlpb.ForkChoiceHead, len(headRoots)),
	}
	for i := range headRoots {
		isOptimistic, err := ds.OptimisticModeFetcher.IsOptimisticForRoot(ctx, headRoots[i])
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not check if head is optimistic: %v", err)
		}
		resp.Data[i] = &qrlpb.ForkChoiceHead{
			Root:                headRoots[i][:],
			Slot:                headSlots[i],
			ExecutionOptimistic: isOptimistic,
		}
	}

	return resp, nil
}

// GetForkChoice returns a dump fork choice store.
func (ds *Server) GetForkChoice(ctx context.Context, _ *emptypb.Empty) (*qrlpb.ForkChoiceDump, error) {
	return ds.ForkchoiceFetcher.ForkChoiceDump(ctx)
}
