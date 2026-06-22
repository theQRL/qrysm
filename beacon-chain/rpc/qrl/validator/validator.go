package validator

import (
	"context"
	"fmt"

	rpchelpers "github.com/theQRL/qrysm/beacon-chain/rpc/qrl/helpers"
	"github.com/theQRL/qrysm/proto/migration"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ProduceBlock requests the beacon node to produce a valid unsigned beacon block, which can then be signed by a proposer and submitted.
// By definition `/qrl/v1/validator/blocks/{slot}`, does not produce block using mev-boost and relayer network.
// The following endpoint states that the returned object is a BeaconBlock, not a BlindedBeaconBlock. As such, the block must return a full ExecutionPayload:
// https://ethereum.github.io/beacon-APIs/?urls.primaryName=v2.3.0#/Validator/produceBlockV2
//
// To use mev-boost and relayer network. It's recommended to use the following endpoint:
// https://github.com/ethereum/beacon-APIs/blob/master/apis/validator/blinded_block.yaml
func (vs *Server) ProduceBlock(ctx context.Context, req *qrlpb.ProduceBlockRequest) (*qrlpb.ProduceBlockResponse, error) {
	ctx, span := trace.StartSpan(ctx, "validator.ProduceBlock")
	defer span.End()

	if err := rpchelpers.ValidateSyncGRPC(ctx, vs.SyncChecker, vs.HeadFetcher, vs.TimeFetcher, vs.OptimisticModeFetcher); err != nil {
		// We simply return the error because it's already a gRPC error.
		return nil, err
	}

	v1alpha1req := &qrysmpb.BlockRequest{
		Slot:         req.Slot,
		RandaoReveal: req.RandaoReveal,
		Graffiti:     req.Graffiti,
		SkipMevBoost: true, // Skip mev-boost and relayer network
	}
	v1alpha1resp, err := vs.V1Alpha1Server.GetBeaconBlock(ctx, v1alpha1req)
	if err != nil {
		// We simply return err because it's already of a gRPC error type.
		return nil, err
	}
	optimistic, err := vs.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine if the node is a optimistic node: %v", err)
	}
	if optimistic {
		return nil, status.Errorf(codes.Unavailable, "The node is currently optimistic and cannot serve validators")
	}
	_, ok := v1alpha1resp.Block.(*qrysmpb.GenericBeaconBlock_BlindedZond)
	if ok {
		return nil, status.Error(codes.Internal, "Prepared Zond beacon block is blinded")
	}
	zondBlock, ok := v1alpha1resp.Block.(*qrysmpb.GenericBeaconBlock_Zond)
	if ok {
		block, err := migration.V1Alpha1BeaconBlockZondToV1(zondBlock.Zond)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not prepare beacon block: %v", err)
		}
		return &qrlpb.ProduceBlockResponse{
			Version: qrlpb.Version_ZOND,
			Data: &qrlpb.BeaconBlockContainer{
				Block: &qrlpb.BeaconBlockContainer_ZondBlock{ZondBlock: block},
			},
		}, nil
	}

	return nil, status.Error(codes.InvalidArgument, "Unsupported block type")
}

// ProduceBlockSSZ requests the beacon node to produce a valid unsigned beacon block, which can then be signed by a proposer and submitted.
//
// The produced block is in SSZ form.
// By definition `/qrl/v1/validator/blocks/{slot}/ssz`, does not produce block using mev-boost and relayer network:
// The following endpoint states that the returned object is a BeaconBlock, not a BlindedBeaconBlock. As such, the block must return a full ExecutionPayload:
// https://ethereum.github.io/beacon-APIs/?urls.primaryName=v2.3.0#/Validator/produceBlockV2
//
// To use mev-boost and relayer network. It's recommended to use the following endpoint:
// https://github.com/ethereum/beacon-APIs/blob/master/apis/validator/blinded_block.yaml
func (vs *Server) ProduceBlockSSZ(ctx context.Context, req *qrlpb.ProduceBlockRequest) (*qrlpb.SSZContainer, error) {
	ctx, span := trace.StartSpan(ctx, "validator.ProduceBlockSSZ")
	defer span.End()

	if err := rpchelpers.ValidateSyncGRPC(ctx, vs.SyncChecker, vs.HeadFetcher, vs.TimeFetcher, vs.OptimisticModeFetcher); err != nil {
		// We simply return the error because it's already a gRPC error.
		return nil, err
	}

	v1alpha1req := &qrysmpb.BlockRequest{
		Slot:         req.Slot,
		RandaoReveal: req.RandaoReveal,
		Graffiti:     req.Graffiti,
		SkipMevBoost: true, // Skip mev-boost and relayer network
	}
	v1alpha1resp, err := vs.V1Alpha1Server.GetBeaconBlock(ctx, v1alpha1req)
	if err != nil {
		// We simply return err because it's already of a gRPC error type.
		return nil, err
	}
	optimistic, err := vs.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine if the node is a optimistic node: %v", err)
	}
	if optimistic {
		return nil, status.Errorf(codes.Unavailable, "The node is currently optimistic and cannot serve validators")
	}
	_, ok := v1alpha1resp.Block.(*qrysmpb.GenericBeaconBlock_BlindedZond)
	if ok {
		return nil, status.Error(codes.Internal, "Prepared Zond beacon block is blinded")
	}
	zondBlock, ok := v1alpha1resp.Block.(*qrysmpb.GenericBeaconBlock_Zond)
	if ok {
		block, err := migration.V1Alpha1BeaconBlockZondToV1(zondBlock.Zond)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not prepare beacon block: %v", err)
		}
		sszBlock, err := block.MarshalSSZ()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not marshal block into SSZ format: %v", err)
		}
		return &qrlpb.SSZContainer{
			Version: qrlpb.Version_ZOND,
			Data:    sszBlock,
		}, nil
	}

	return nil, status.Error(codes.InvalidArgument, "Unsupported block type")
}

// ProduceBlindedBlock requests the beacon node to produce a valid unsigned blinded beacon block,
// which can then be signed by a proposer and submitted.
//
// Under the following conditions, this endpoint will return an error.
// - The node is syncing or optimistic mode (after bellatrix).
// - The builder is not figured (after bellatrix).
// - The relayer circuit breaker is activated (after bellatrix).
// - The relayer responded with an error (after bellatrix).
func (vs *Server) ProduceBlindedBlock(ctx context.Context, req *qrlpb.ProduceBlockRequest) (*qrlpb.ProduceBlindedBlockResponse, error) {
	ctx, span := trace.StartSpan(ctx, "validator.ProduceBlindedBlock")
	defer span.End()

	if !vs.BlockBuilder.Configured() {
		return nil, status.Error(codes.Internal, "Block builder not configured")
	}
	if err := rpchelpers.ValidateSyncGRPC(ctx, vs.SyncChecker, vs.HeadFetcher, vs.TimeFetcher, vs.OptimisticModeFetcher); err != nil {
		// We simply return the error because it's already a gRPC error.
		return nil, err
	}

	v1alpha1req := &qrysmpb.BlockRequest{
		Slot:         req.Slot,
		RandaoReveal: req.RandaoReveal,
		Graffiti:     req.Graffiti,
	}
	v1alpha1resp, err := vs.V1Alpha1Server.GetBeaconBlock(ctx, v1alpha1req)
	if err != nil {
		// We simply return err because it's already of a gRPC error type.
		return nil, err
	}

	optimistic, err := vs.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine if the node is a optimistic node: %v", err)
	}
	if optimistic {
		return nil, status.Errorf(codes.Unavailable, "The node is currently optimistic and cannot serve validators")
	}
	_, ok := v1alpha1resp.Block.(*qrysmpb.GenericBeaconBlock_Zond)
	if ok {
		return nil, status.Error(codes.Internal, "Prepared beacon block is not blinded")
	}
	zondBlock, ok := v1alpha1resp.Block.(*qrysmpb.GenericBeaconBlock_BlindedZond)
	if ok {
		blk, err := migration.V1Alpha1BeaconBlockBlindedZondToV1Blinded(zondBlock.BlindedZond)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not prepare beacon block: %v", err)
		}
		return &qrlpb.ProduceBlindedBlockResponse{
			Version: qrlpb.Version_ZOND,
			Data: &qrlpb.BlindedBeaconBlockContainer{
				Block: &qrlpb.BlindedBeaconBlockContainer_ZondBlock{ZondBlock: blk},
			},
		}, nil
	}

	return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("block was not a supported blinded block type, validator may not be registered if using a relay. received: %T", v1alpha1resp.Block))
}

// ProduceBlindedBlockSSZ requests the beacon node to produce a valid unsigned blinded beacon block,
// which can then be signed by a proposer and submitted.
//
// The produced block is in SSZ form.
//
// Pre-Bellatrix, this endpoint will return a regular block.
func (vs *Server) ProduceBlindedBlockSSZ(ctx context.Context, req *qrlpb.ProduceBlockRequest) (*qrlpb.SSZContainer, error) {
	ctx, span := trace.StartSpan(ctx, "validator.ProduceBlindedBlockSSZ")
	defer span.End()

	if !vs.BlockBuilder.Configured() {
		return nil, status.Error(codes.Internal, "Block builder not configured")
	}
	if err := rpchelpers.ValidateSyncGRPC(ctx, vs.SyncChecker, vs.HeadFetcher, vs.TimeFetcher, vs.OptimisticModeFetcher); err != nil {
		// We simply return the error because it's already a gRPC error.
		return nil, err
	}

	v1alpha1req := &qrysmpb.BlockRequest{
		Slot:         req.Slot,
		RandaoReveal: req.RandaoReveal,
		Graffiti:     req.Graffiti,
	}
	v1alpha1resp, err := vs.V1Alpha1Server.GetBeaconBlock(ctx, v1alpha1req)
	if err != nil {
		// We simply return err because it's already of a gRPC error type.
		return nil, err
	}

	optimistic, err := vs.OptimisticModeFetcher.IsOptimistic(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not determine if the node is a optimistic node: %v", err)
	}
	if optimistic {
		return nil, status.Errorf(codes.Unavailable, "The node is currently optimistic and cannot serve validators")
	}
	_, ok := v1alpha1resp.Block.(*qrysmpb.GenericBeaconBlock_Zond)
	if ok {
		return nil, status.Error(codes.Internal, "Prepared Zond beacon block is not blinded")
	}
	zondBlock, ok := v1alpha1resp.Block.(*qrysmpb.GenericBeaconBlock_BlindedZond)
	if ok {
		block, err := migration.V1Alpha1BeaconBlockBlindedZondToV1Blinded(zondBlock.BlindedZond)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not prepare beacon block: %v", err)
		}
		sszBlock, err := block.MarshalSSZ()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Could not marshal block into SSZ format: %v", err)
		}
		return &qrlpb.SSZContainer{
			Version: qrlpb.Version_ZOND,
			Data:    sszBlock,
		}, nil
	}

	return nil, status.Error(codes.InvalidArgument, "Unsupported block type")
}
