package grpc_api

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/validator/client/iface"
	"google.golang.org/grpc"
)

type grpcBeaconChainClient struct {
	beaconChainClient zondpb.BeaconChainClient
}

func (c *grpcBeaconChainClient) GetChainHead(ctx context.Context, in *empty.Empty) (*zondpb.ChainHead, error) {
	return c.beaconChainClient.GetChainHead(ctx, in)
}

func (c *grpcBeaconChainClient) ListValidatorBalances(ctx context.Context, in *zondpb.ListValidatorBalancesRequest) (*zondpb.ValidatorBalances, error) {
	return c.beaconChainClient.ListValidatorBalances(ctx, in)
}

func (c *grpcBeaconChainClient) ListValidators(ctx context.Context, in *zondpb.ListValidatorsRequest) (*zondpb.Validators, error) {
	return c.beaconChainClient.ListValidators(ctx, in)
}

func (c *grpcBeaconChainClient) GetValidatorPerformance(ctx context.Context, in *zondpb.ValidatorPerformanceRequest) (*zondpb.ValidatorPerformanceResponse, error) {
	return c.beaconChainClient.GetValidatorPerformance(ctx, in)
}

func NewGrpcBeaconChainClient(cc grpc.ClientConnInterface) iface.BeaconChainClient {
	return &grpcBeaconChainClient{zondpb.NewBeaconChainClient(cc)}
}
