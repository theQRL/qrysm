package grpc_api

import (
	"context"

	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/validator/client/iface"
	"google.golang.org/grpc"
)

type grpcQrysmBeaconChainClient struct {
	chainClient iface.BeaconChainClient
}

func (c *grpcQrysmBeaconChainClient) GetValidatorPerformance(ctx context.Context, in *qrysmpb.ValidatorPerformanceRequest) (*qrysmpb.ValidatorPerformanceResponse, error) {
	return c.chainClient.GetValidatorPerformance(ctx, in)
}

// NewGrpcQrysmBeaconChainClient returns a gRPC-backed QrysmChainClient that
// delegates to the standard BeaconChainClient.
func NewGrpcQrysmBeaconChainClient(cc grpc.ClientConnInterface) iface.QrysmChainClient {
	return &grpcQrysmBeaconChainClient{chainClient: NewGrpcBeaconChainClient(cc)}
}
