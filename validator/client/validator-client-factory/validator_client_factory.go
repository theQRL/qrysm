package validator_client_factory

import (
	"github.com/theQRL/qrysm/config/features"
	beaconApi "github.com/theQRL/qrysm/validator/client/beacon-api"
	grpcApi "github.com/theQRL/qrysm/validator/client/grpc-api"
	"github.com/theQRL/qrysm/validator/client/iface"
	validatorHelpers "github.com/theQRL/qrysm/validator/helpers"
)

func NewValidatorClient(validatorConn validatorHelpers.NodeConnection) iface.ValidatorClient {
	featureFlags := features.Get()

	if featureFlags.EnableBeaconRESTApi {
		return beaconApi.NewBeaconApiValidatorClient(validatorConn.GetBeaconApiUrl(), validatorConn.GetBeaconApiTimeout())
	} else {
		return grpcApi.NewGrpcValidatorClient(validatorConn.GetGrpcClientConn())
	}
}
