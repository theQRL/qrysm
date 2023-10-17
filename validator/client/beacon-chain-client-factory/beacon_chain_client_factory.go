package validator_client_factory

import (
	"github.com/theQRL/qrysm/v4/config/features"
	beaconApi "github.com/theQRL/qrysm/v4/validator/client/beacon-api"
	grpcApi "github.com/theQRL/qrysm/v4/validator/client/grpc-api"
	"github.com/theQRL/qrysm/v4/validator/client/iface"
	validatorHelpers "github.com/theQRL/qrysm/v4/validator/helpers"
)

func NewBeaconChainClient(validatorConn validatorHelpers.NodeConnection) iface.BeaconChainClient {
	grpcClient := grpcApi.NewGrpcBeaconChainClient(validatorConn.GetGrpcClientConn())
	featureFlags := features.Get()

	if featureFlags.EnableBeaconRESTApi {
		return beaconApi.NewBeaconApiBeaconChainClientWithFallback(validatorConn.GetBeaconApiUrl(), validatorConn.GetBeaconApiTimeout(), grpcClient)
	} else {
		return grpcClient
	}
}
