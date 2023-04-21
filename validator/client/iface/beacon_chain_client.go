package iface

import (
	"context"

	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/golang/protobuf/ptypes/empty"
)

type BeaconChainClient interface {
	GetChainHead(ctx context.Context, in *empty.Empty) (*ethpb.ChainHead, error)
	ListValidatorBalances(ctx context.Context, in *ethpb.ListValidatorBalancesRequest) (*ethpb.ValidatorBalances, error)
	ListValidators(ctx context.Context, in *ethpb.ListValidatorsRequest) (*ethpb.Validators, error)
	GetValidatorQueue(ctx context.Context, in *empty.Empty) (*ethpb.ValidatorQueue, error)
	GetValidatorPerformance(ctx context.Context, in *ethpb.ValidatorPerformanceRequest) (*ethpb.ValidatorPerformanceResponse, error)
	GetValidatorParticipation(ctx context.Context, in *ethpb.GetValidatorParticipationRequest) (*ethpb.ValidatorParticipationResponse, error)
}
