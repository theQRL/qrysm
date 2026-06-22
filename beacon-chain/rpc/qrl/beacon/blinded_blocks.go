package beacon

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/api"
	rpchelpers "github.com/theQRL/qrysm/beacon-chain/rpc/qrl/helpers"
	consensus_types "github.com/theQRL/qrysm/consensus-types"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/proto/migration"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	"github.com/theQRL/qrysm/runtime/version"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// GetBlindedBlock retrieves blinded block for given block id.
func (bs *Server) GetBlindedBlock(ctx context.Context, req *qrlpb.BlockRequest) (*qrlpb.BlindedBlockResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.GetBlindedBlock")
	defer span.End()

	blk, err := bs.Blocker.Block(ctx, req.BlockId)
	err = rpchelpers.HandleGetBlockError(blk, err)
	if err != nil {
		return nil, err
	}
	blkRoot, err := blk.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get block root")
	}
	if err := grpc.SetHeader(ctx, metadata.Pairs(api.VersionHeader, version.String(blk.Version()))); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not set "+api.VersionHeader+" header: %v", err)
	}

	result, err := bs.getBlindedBlockZond(ctx, blk)
	if result != nil {
		result.Finalized = bs.FinalizationFetcher.IsFinalized(ctx, blkRoot)
		return result, nil
	}
	// ErrUnsupportedField means that we have another block type
	if !errors.Is(err, consensus_types.ErrUnsupportedField) {
		return nil, status.Errorf(codes.Internal, "Could not get blinded block: %v", err)
	}

	return nil, status.Errorf(codes.Internal, "Unknown block type %T", blk)
}

// GetBlindedBlockSSZ returns the SSZ-serialized version of the blinded beacon block for given block id.
func (bs *Server) GetBlindedBlockSSZ(ctx context.Context, req *qrlpb.BlockRequest) (*qrlpb.SSZContainer, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.GetBlindedBlockSSZ")
	defer span.End()

	blk, err := bs.Blocker.Block(ctx, req.BlockId)
	err = rpchelpers.HandleGetBlockError(blk, err)
	if err != nil {
		return nil, err
	}
	blkRoot, err := blk.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get block root")
	}

	result, err := bs.getBlindedSSZBlockZond(ctx, blk)
	if result != nil {
		result.Finalized = bs.FinalizationFetcher.IsFinalized(ctx, blkRoot)
		return result, nil
	}
	// ErrUnsupportedField means that we have another block type
	if !errors.Is(err, consensus_types.ErrUnsupportedField) {
		return nil, status.Errorf(codes.Internal, "Could not get signed beacon block: %v", err)
	}

	return nil, status.Errorf(codes.Internal, "Unknown block type %T", blk)
}

func (bs *Server) getBlindedBlockZond(ctx context.Context, blk interfaces.ReadOnlySignedBeaconBlock) (*qrlpb.BlindedBlockResponse, error) {
	zondBlk, err := blk.PbZondBlock()
	if err != nil {
		// ErrUnsupportedField means that we have another block type
		if errors.Is(err, consensus_types.ErrUnsupportedField) {
			if blindedZondBlk, err := blk.PbBlindedZondBlock(); err == nil {
				if blindedZondBlk == nil {
					return nil, errNilBlock
				}
				v2Blk, err := migration.V1Alpha1BeaconBlockBlindedZondToV1Blinded(blindedZondBlk.Block)
				if err != nil {
					return nil, errors.Wrapf(err, "Could not convert beacon block")
				}
				root, err := blk.Block().HashTreeRoot()
				if err != nil {
					return nil, errors.Wrapf(err, "could not get block root")
				}
				isOptimistic, err := bs.OptimisticModeFetcher.IsOptimisticForRoot(ctx, root)
				if err != nil {
					return nil, errors.Wrapf(err, "could not check if block is optimistic")
				}
				sig := blk.Signature()
				return &qrlpb.BlindedBlockResponse{
					Version: qrlpb.Version_ZOND,
					Data: &qrlpb.SignedBlindedBeaconBlockContainer{
						Message:   &qrlpb.SignedBlindedBeaconBlockContainer_ZondBlock{ZondBlock: v2Blk},
						Signature: sig[:],
					},
					ExecutionOptimistic: isOptimistic,
				}, nil
			}
			return nil, err
		}
		return nil, err
	}

	if zondBlk == nil {
		return nil, errNilBlock
	}
	blindedBlkInterface, err := blk.ToBlinded()
	if err != nil {
		return nil, errors.Wrapf(err, "could not convert block to blinded block")
	}
	blindedZondBlock, err := blindedBlkInterface.PbBlindedZondBlock()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get signed beacon block")
	}
	v2Blk, err := migration.V1Alpha1BeaconBlockBlindedZondToV1Blinded(blindedZondBlock.Block)
	if err != nil {
		return nil, errors.Wrapf(err, "could not convert beacon block")
	}
	root, err := blk.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get block root")
	}
	isOptimistic, err := bs.OptimisticModeFetcher.IsOptimisticForRoot(ctx, root)
	if err != nil {
		return nil, errors.Wrapf(err, "could not check if block is optimistic")
	}
	sig := blk.Signature()
	return &qrlpb.BlindedBlockResponse{
		Version: qrlpb.Version_ZOND,
		Data: &qrlpb.SignedBlindedBeaconBlockContainer{
			Message:   &qrlpb.SignedBlindedBeaconBlockContainer_ZondBlock{ZondBlock: v2Blk},
			Signature: sig[:],
		},
		ExecutionOptimistic: isOptimistic,
	}, nil
}

func (bs *Server) getBlindedSSZBlockZond(ctx context.Context, blk interfaces.ReadOnlySignedBeaconBlock) (*qrlpb.SSZContainer, error) {
	zondBlk, err := blk.PbZondBlock()
	if err != nil {
		// ErrUnsupportedField means that we have another block type
		if errors.Is(err, consensus_types.ErrUnsupportedField) {
			if blindedZondBlk, err := blk.PbBlindedZondBlock(); err == nil {
				if blindedZondBlk == nil {
					return nil, errNilBlock
				}
				v2Blk, err := migration.V1Alpha1BeaconBlockBlindedZondToV1Blinded(blindedZondBlk.Block)
				if err != nil {
					return nil, errors.Wrapf(err, "could not get signed beacon block")
				}
				root, err := blk.Block().HashTreeRoot()
				if err != nil {
					return nil, errors.Wrapf(err, "could not get block root")
				}
				isOptimistic, err := bs.OptimisticModeFetcher.IsOptimisticForRoot(ctx, root)
				if err != nil {
					return nil, errors.Wrapf(err, "could not check if block is optimistic")
				}
				sig := blk.Signature()
				data := &qrlpb.SignedBlindedBeaconBlockZond{
					Message:   v2Blk,
					Signature: sig[:],
				}
				sszData, err := data.MarshalSSZ()
				if err != nil {
					return nil, errors.Wrapf(err, "could not marshal block into SSZ")
				}
				return &qrlpb.SSZContainer{
					Version:             qrlpb.Version_ZOND,
					ExecutionOptimistic: isOptimistic,
					Data:                sszData,
				}, nil
			}
			return nil, err
		}
	}

	if zondBlk == nil {
		return nil, errNilBlock
	}
	blindedBlkInterface, err := blk.ToBlinded()
	if err != nil {
		return nil, errors.Wrapf(err, "could not convert block to blinded block")
	}
	blindedZondBlock, err := blindedBlkInterface.PbBlindedZondBlock()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get signed beacon block")
	}
	v2Blk, err := migration.V1Alpha1BeaconBlockBlindedZondToV1Blinded(blindedZondBlock.Block)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get signed beacon block")
	}
	root, err := blk.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get block root")
	}
	isOptimistic, err := bs.OptimisticModeFetcher.IsOptimisticForRoot(ctx, root)
	if err != nil {
		return nil, errors.Wrapf(err, "could not check if block is optimistic")
	}
	sig := blk.Signature()
	data := &qrlpb.SignedBlindedBeaconBlockZond{
		Message:   v2Blk,
		Signature: sig[:],
	}
	sszData, err := data.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrapf(err, "could not marshal block into SSZ")
	}
	return &qrlpb.SSZContainer{Version: qrlpb.Version_ZOND, ExecutionOptimistic: isOptimistic, Data: sszData}, nil
}
