package iface

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

type BeaconChainClient interface {
	GetChainHead(ctx context.Context, in *empty.Empty) (*zondpb.ChainHead, error)
	ListValidatorBalances(ctx context.Context, in *zondpb.ListValidatorBalancesRequest) (*zondpb.ValidatorBalances, error)
	ListValidators(ctx context.Context, in *zondpb.ListValidatorsRequest) (*zondpb.Validators, error)
	GetValidatorQueue(ctx context.Context, in *empty.Empty) (*zondpb.ValidatorQueue, error)
	GetValidatorPerformance(ctx context.Context, in *zondpb.ValidatorPerformanceRequest) (*zondpb.ValidatorPerformanceResponse, error)
	GetValidatorParticipation(ctx context.Context, in *zondpb.GetValidatorParticipationRequest) (*zondpb.ValidatorParticipationResponse, error)
}
