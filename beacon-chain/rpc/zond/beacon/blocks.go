package beacon

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/api"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	rpchelpers "github.com/theQRL/qrysm/beacon-chain/rpc/zond/helpers"
	"github.com/theQRL/qrysm/config/params"
	consensus_types "github.com/theQRL/qrysm/consensus-types"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/proto/migration"
	zondpbv1 "github.com/theQRL/qrysm/proto/zond/v1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	errNilBlock = errors.New("nil block")
)

// GetWeakSubjectivity computes the starting epoch of the current weak subjectivity period, and then also
// determines the best block root and state root to use for a Checkpoint Sync starting from that point.
// DEPRECATED: GetWeakSubjectivity endpoint will no longer be supported
func (bs *Server) GetWeakSubjectivity(ctx context.Context, _ *empty.Empty) (*zondpbv1.WeakSubjectivityResponse, error) {
	if err := rpchelpers.ValidateSyncGRPC(ctx, bs.SyncChecker, bs.HeadFetcher, bs.GenesisTimeFetcher, bs.OptimisticModeFetcher); err != nil {
		// This is already a grpc error, so we can't wrap it any further
		return nil, err
	}

	hs, err := bs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not get head state")
	}
	wsEpoch, err := helpers.LatestWeakSubjectivityEpoch(ctx, hs, params.BeaconConfig())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get weak subjectivity epoch: %v", err)
	}
	wsSlot, err := slots.EpochStart(wsEpoch)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not get weak subjectivity slot: %v", err)
	}
	cbr, err := bs.CanonicalHistory.BlockRootForSlot(ctx, wsSlot)
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("could not find highest block below slot %d", wsSlot))
	}
	cb, err := bs.BeaconDB.Block(ctx, cbr)
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("block with root %#x from slot index %d not found in db", cbr, wsSlot))
	}
	stateRoot := cb.Block().StateRoot()
	log.Printf("weak subjectivity checkpoint reported as epoch=%d, block root=%#x, state root=%#x", wsEpoch, cbr, stateRoot)
	return &zondpbv1.WeakSubjectivityResponse{
		Data: &zondpbv1.WeakSubjectivityData{
			WsCheckpoint: &zondpbv1.Checkpoint{
				Epoch: wsEpoch,
				Root:  cbr[:],
			},
			StateRoot: stateRoot[:],
		},
	}, nil
}

// GetBlock retrieves block details for given block ID.
func (bs *Server) GetBlock(ctx context.Context, req *zondpbv1.BlockRequest) (*zondpbv1.BlockResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.GetBlock")
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
	result, err := bs.getBlockCapella(ctx, blk)
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

// GetBlockSSZ returns the SSZ-serialized version of the beacon block for given block ID.
func (bs *Server) GetBlockSSZ(ctx context.Context, req *zondpbv1.BlockRequest) (*zondpbv1.SSZContainer, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.GetBlockSSZ")
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

	result, err := bs.getSSZBlockCapella(ctx, blk)
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

// ListBlockAttestations retrieves attestation included in requested block.
func (bs *Server) ListBlockAttestations(ctx context.Context, req *zondpbv1.BlockRequest) (*zondpbv1.BlockAttestationsResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.ListBlockAttestations")
	defer span.End()

	blk, err := bs.Blocker.Block(ctx, req.BlockId)
	err = rpchelpers.HandleGetBlockError(blk, err)
	if err != nil {
		return nil, err
	}

	v1Alpha1Attestations := blk.Block().Body().Attestations()
	v1Attestations := make([]*zondpbv1.Attestation, 0, len(v1Alpha1Attestations))
	for _, att := range v1Alpha1Attestations {
		migratedAtt := migration.V1Alpha1AttestationToV1(att)
		v1Attestations = append(v1Attestations, migratedAtt)
	}
	root, err := blk.Block().HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get block root: %v", err)
	}
	isOptimistic, err := bs.OptimisticModeFetcher.IsOptimisticForRoot(ctx, root)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not check if block is optimistic: %v", err)
	}
	return &zondpbv1.BlockAttestationsResponse{
		Data:                v1Attestations,
		ExecutionOptimistic: isOptimistic,
		Finalized:           bs.FinalizationFetcher.IsFinalized(ctx, root),
	}, nil
}

func (bs *Server) getBlockCapella(ctx context.Context, blk interfaces.ReadOnlySignedBeaconBlock) (*zondpbv1.BlockResponse, error) {
	capellaBlk, err := blk.PbCapellaBlock()
	if err != nil {
		// ErrUnsupportedField means that we have another block type
		if errors.Is(err, consensus_types.ErrUnsupportedField) {
			if blindedCapellaBlk, err := blk.PbBlindedCapellaBlock(); err == nil {
				if blindedCapellaBlk == nil {
					return nil, errNilBlock
				}
				signedFullBlock, err := bs.ExecutionPayloadReconstructor.ReconstructFullBlock(ctx, blk)
				if err != nil {
					return nil, errors.Wrapf(err, "could not reconstruct full execution payload to create signed beacon block")
				}
				capellaBlk, err = signedFullBlock.PbCapellaBlock()
				if err != nil {
					return nil, errors.Wrapf(err, "could not get signed beacon block")
				}
				v2Blk, err := migration.V1Alpha1BeaconBlockCapellaToV1(capellaBlk.Block)
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
				return &zondpbv1.BlockResponse{
					Version: zondpbv1.Version_CAPELLA,
					Data: &zondpbv1.SignedBeaconBlockContainer{
						Message:   &zondpbv1.SignedBeaconBlockContainer_CapellaBlock{CapellaBlock: v2Blk},
						Signature: sig[:],
					},
					ExecutionOptimistic: isOptimistic,
				}, nil
			}
			return nil, err
		}
		return nil, err
	}

	if capellaBlk == nil {
		return nil, errNilBlock
	}
	v2Blk, err := migration.V1Alpha1BeaconBlockCapellaToV1(capellaBlk.Block)
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
	return &zondpbv1.BlockResponse{
		Version: zondpbv1.Version_CAPELLA,
		Data: &zondpbv1.SignedBeaconBlockContainer{
			Message:   &zondpbv1.SignedBeaconBlockContainer_CapellaBlock{CapellaBlock: v2Blk},
			Signature: sig[:],
		},
		ExecutionOptimistic: isOptimistic,
	}, nil
}

func (bs *Server) getSSZBlockCapella(ctx context.Context, blk interfaces.ReadOnlySignedBeaconBlock) (*zondpbv1.SSZContainer, error) {
	capellaBlk, err := blk.PbCapellaBlock()
	if err != nil {
		// ErrUnsupportedField means that we have another block type
		if errors.Is(err, consensus_types.ErrUnsupportedField) {
			if blindedCapellaBlk, err := blk.PbBlindedCapellaBlock(); err == nil {
				if blindedCapellaBlk == nil {
					return nil, errNilBlock
				}
				signedFullBlock, err := bs.ExecutionPayloadReconstructor.ReconstructFullBlock(ctx, blk)
				if err != nil {
					return nil, errors.Wrapf(err, "could not reconstruct full execution payload to create signed beacon block")
				}
				capellaBlk, err = signedFullBlock.PbCapellaBlock()
				if err != nil {
					return nil, errors.Wrapf(err, "could not get signed beacon block")
				}
				v2Blk, err := migration.V1Alpha1BeaconBlockCapellaToV1(capellaBlk.Block)
				if err != nil {
					return nil, errors.Wrapf(err, "could not convert signed beacon block")
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
				data := &zondpbv1.SignedBeaconBlockCapella{
					Message:   v2Blk,
					Signature: sig[:],
				}
				sszData, err := data.MarshalSSZ()
				if err != nil {
					return nil, errors.Wrapf(err, "could not marshal block into SSZ")
				}
				return &zondpbv1.SSZContainer{
					Version:             zondpbv1.Version_CAPELLA,
					ExecutionOptimistic: isOptimistic,
					Data:                sszData,
				}, nil
			}
			return nil, err
		}
		return nil, err
	}

	if capellaBlk == nil {
		return nil, errNilBlock
	}
	v2Blk, err := migration.V1Alpha1BeaconBlockCapellaToV1(capellaBlk.Block)
	if err != nil {
		return nil, errors.Wrapf(err, "could not convert signed beacon block")
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
	data := &zondpbv1.SignedBeaconBlockCapella{
		Message:   v2Blk,
		Signature: sig[:],
	}
	sszData, err := data.MarshalSSZ()
	if err != nil {
		return nil, errors.Wrapf(err, "could not marshal block into SSZ")
	}
	return &zondpbv1.SSZContainer{Version: zondpbv1.Version_CAPELLA, ExecutionOptimistic: isOptimistic, Data: sszData}, nil
}
