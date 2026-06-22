package grpc_api

import (
	"context"

	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/validator/client/iface"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type grpcBeaconChainClient struct {
	beaconChainClient qrysmpb.BeaconChainClient
}

func (c *grpcBeaconChainClient) GetChainHead(ctx context.Context, in *emptypb.Empty) (*qrysmpb.ChainHead, error) {
	return c.beaconChainClient.GetChainHead(ctx, in)
}

func (c *grpcBeaconChainClient) ListValidatorBalances(ctx context.Context, in *qrysmpb.ListValidatorBalancesRequest) (*qrysmpb.ValidatorBalances, error) {
	return c.beaconChainClient.ListValidatorBalances(ctx, in)
}

func (c *grpcBeaconChainClient) ListValidators(ctx context.Context, in *qrysmpb.ListValidatorsRequest) (*qrysmpb.Validators, error) {
	return c.beaconChainClient.ListValidators(ctx, in)
}

func (c *grpcBeaconChainClient) GetValidatorPerformance(ctx context.Context, in *qrysmpb.ValidatorPerformanceRequest) (*qrysmpb.ValidatorPerformanceResponse, error) {
	return c.beaconChainClient.GetValidatorPerformance(ctx, in)
}

func NewGrpcBeaconChainClient(cc grpc.ClientConnInterface) iface.BeaconChainClient {
	return &grpcBeaconChainClient{qrysmpb.NewBeaconChainClient(cc)}
}
