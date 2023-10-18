package beacon

import (
	"bytes"
	"context"
	"strconv"

	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/eth/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/lookup"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/zond/v1"
	zond2 "github.com/theQRL/qrysm/v4/proto/zond/v2"
	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/time/slots"
	"go.opencensus.io/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type stateRequest struct {
	epoch   *primitives.Epoch
	stateId []byte
}

// GetGenesis retrieves details of the chain's genesis which can be used to identify chain.
func (bs *Server) GetGenesis(ctx context.Context, _ *emptypb.Empty) (*zondpb.GenesisResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.GetGenesis")
	defer span.End()

	genesisTime := bs.GenesisTimeFetcher.GenesisTime()
	if genesisTime.IsZero() {
		return nil, status.Errorf(codes.NotFound, "Chain genesis info is not yet known")
	}
	validatorRoot := bs.ChainInfoFetcher.GenesisValidatorsRoot()
	if bytes.Equal(validatorRoot[:], params.BeaconConfig().ZeroHash[:]) {
		return nil, status.Errorf(codes.NotFound, "Chain genesis info is not yet known")
	}
	forkVersion := params.BeaconConfig().GenesisForkVersion

	return &zondpb.GenesisResponse{
		Data: &zondpb.GenesisResponse_Genesis{
			GenesisTime: &timestamppb.Timestamp{
				Seconds: genesisTime.Unix(),
				Nanos:   0,
			},
			GenesisValidatorsRoot: validatorRoot[:],
			GenesisForkVersion:    forkVersion,
		},
	}, nil
}

// GetStateRoot calculates HashTreeRoot for state with given 'stateId'. If stateId is root, same value will be returned.
func (bs *Server) GetStateRoot(ctx context.Context, req *zondpb.StateRequest) (*zondpb.StateRootResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.GetStateRoot")
	defer span.End()

	stateRoot, err := bs.Stater.StateRoot(ctx, req.StateId)
	if err != nil {
		if rootNotFoundErr, ok := err.(*lookup.StateRootNotFoundError); ok {
			return nil, status.Errorf(codes.NotFound, "State root not found: %v", rootNotFoundErr)
		} else if parseErr, ok := err.(*lookup.StateIdParseError); ok {
			return nil, status.Errorf(codes.InvalidArgument, "Invalid state ID: %v", parseErr)
		}
		return nil, status.Errorf(codes.Internal, "Could not get state root: %v", err)
	}
	st, err := bs.Stater.State(ctx, req.StateId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get state: %v", err)
	}
	isOptimistic, err := helpers.IsOptimistic(ctx, req.StateId, bs.OptimisticModeFetcher, bs.Stater, bs.ChainInfoFetcher, bs.BeaconDB)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not check if slot's block is optimistic: %v", err)
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not calculate root of latest block header")
	}
	isFinalized := bs.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	return &zondpb.StateRootResponse{
		Data: &zondpb.StateRootResponse_StateRoot{
			Root: stateRoot,
		},
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}, nil
}

// GetStateFork returns Fork object for state with given 'stateId'.
func (bs *Server) GetStateFork(ctx context.Context, req *zondpb.StateRequest) (*zondpb.StateForkResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.GetStateFork")
	defer span.End()

	st, err := bs.Stater.State(ctx, req.StateId)
	if err != nil {
		return nil, helpers.PrepareStateFetchGRPCError(err)
	}
	fork := st.Fork()
	isOptimistic, err := helpers.IsOptimistic(ctx, req.StateId, bs.OptimisticModeFetcher, bs.Stater, bs.ChainInfoFetcher, bs.BeaconDB)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not check if slot's block is optimistic: %v", err)
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not calculate root of latest block header")
	}
	isFinalized := bs.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	return &zondpb.StateForkResponse{
		Data: &zondpb.Fork{
			PreviousVersion: fork.PreviousVersion,
			CurrentVersion:  fork.CurrentVersion,
			Epoch:           fork.Epoch,
		},
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}, nil
}

// GetFinalityCheckpoints returns finality checkpoints for state with given 'stateId'. In case finality is
// not yet achieved, checkpoint should return epoch 0 and ZERO_HASH as root.
func (bs *Server) GetFinalityCheckpoints(ctx context.Context, req *zondpb.StateRequest) (*zondpb.StateFinalityCheckpointResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.GetFinalityCheckpoints")
	defer span.End()

	st, err := bs.Stater.State(ctx, req.StateId)
	if err != nil {
		return nil, helpers.PrepareStateFetchGRPCError(err)
	}
	isOptimistic, err := helpers.IsOptimistic(ctx, req.StateId, bs.OptimisticModeFetcher, bs.Stater, bs.ChainInfoFetcher, bs.BeaconDB)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not check if slot's block is optimistic: %v", err)
	}
	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not calculate root of latest block header")
	}
	isFinalized := bs.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	return &zondpb.StateFinalityCheckpointResponse{
		Data: &zondpb.StateFinalityCheckpointResponse_StateFinalityCheckpoint{
			PreviousJustified: checkpoint(st.PreviousJustifiedCheckpoint()),
			CurrentJustified:  checkpoint(st.CurrentJustifiedCheckpoint()),
			Finalized:         checkpoint(st.FinalizedCheckpoint()),
		},
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}, nil
}

// GetRandao fetches the RANDAO mix for the requested epoch from the state identified by state_id.
// If an epoch is not specified then the RANDAO mix for the state's current epoch will be returned.
// By adjusting the state_id parameter you can query for any historic value of the RANDAO mix.
// Ordinarily states from the same epoch will mutate the RANDAO mix for that epoch as blocks are applied.
func (bs *Server) GetRandao(ctx context.Context, req *zond2.RandaoRequest) (*zond2.RandaoResponse, error) {
	ctx, span := trace.StartSpan(ctx, "beacon.GetRandao")
	defer span.End()

	st, err := bs.Stater.State(ctx, req.StateId)
	if err != nil {
		return nil, helpers.PrepareStateFetchGRPCError(err)
	}

	stEpoch := slots.ToEpoch(st.Slot())
	epoch := stEpoch
	if req.Epoch != nil {
		epoch = *req.Epoch
	}

	// future epochs and epochs too far back are not supported.
	randaoEpochLowerBound := uint64(0)
	// Lower bound should not underflow.
	if uint64(stEpoch) > uint64(st.RandaoMixesLength()) {
		randaoEpochLowerBound = uint64(stEpoch) - uint64(st.RandaoMixesLength())
	}
	if epoch > stEpoch || uint64(epoch) < randaoEpochLowerBound+1 {
		return nil, status.Errorf(codes.InvalidArgument, "Epoch is out of range for the randao mixes of the state")
	}
	idx := epoch % params.BeaconConfig().EpochsPerHistoricalVector
	randao, err := st.RandaoMixAtIndex(uint64(idx))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not get randao mix at index %d", idx)
	}

	isOptimistic, err := helpers.IsOptimistic(ctx, req.StateId, bs.OptimisticModeFetcher, bs.Stater, bs.ChainInfoFetcher, bs.BeaconDB)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not check if slot's block is optimistic: %v", err)
	}

	blockRoot, err := st.LatestBlockHeader().HashTreeRoot()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not calculate root of latest block header")
	}
	isFinalized := bs.FinalizationFetcher.IsFinalized(ctx, blockRoot)

	return &zond2.RandaoResponse{
		Data:                &zond2.RandaoResponse_Randao{Randao: randao},
		ExecutionOptimistic: isOptimistic,
		Finalized:           isFinalized,
	}, nil
}

func (bs *Server) stateFromRequest(ctx context.Context, req *stateRequest) (state.BeaconState, error) {
	if req.epoch != nil {
		slot, err := slots.EpochStart(*req.epoch)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"Could not calculate start slot for epoch %d: %v",
				*req.epoch,
				err,
			)
		}
		st, err := bs.Stater.State(ctx, []byte(strconv.FormatUint(uint64(slot), 10)))
		if err != nil {
			return nil, helpers.PrepareStateFetchGRPCError(err)
		}
		return st, nil
	}
	var err error
	st, err := bs.Stater.State(ctx, req.stateId)
	if err != nil {
		return nil, helpers.PrepareStateFetchGRPCError(err)
	}
	return st, nil
}

func checkpoint(sourceCheckpoint *zond.Checkpoint) *zondpb.Checkpoint {
	if sourceCheckpoint != nil {
		return &zondpb.Checkpoint{
			Epoch: sourceCheckpoint.Epoch,
			Root:  sourceCheckpoint.Root,
		}
	}
	return &zondpb.Checkpoint{
		Epoch: 0,
		Root:  params.BeaconConfig().ZeroHash[:],
	}
}
