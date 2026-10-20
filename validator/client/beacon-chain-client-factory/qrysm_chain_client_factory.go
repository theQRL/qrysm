package validator_client_factory

import (
	"github.com/theQRL/qrysm/config/features"
	beaconApi "github.com/theQRL/qrysm/validator/client/beacon-api"
	grpcApi "github.com/theQRL/qrysm/validator/client/grpc-api"
	"github.com/theQRL/qrysm/validator/client/iface"
	validatorHelpers "github.com/theQRL/qrysm/validator/helpers"
)

// NewQrysmChainClient returns the QrysmChainClient appropriate for the configured
// transport. When the REST API is enabled it returns a REST-backed implementation
// that gates calls on a node-version check (skipping cleanly with ErrNotSupported
// when the connected beacon node is not a qrysm node).
func NewQrysmChainClient(validatorConn validatorHelpers.NodeConnection, nodeClient iface.NodeClient) iface.QrysmChainClient {
	if features.Get().EnableBeaconRESTApi {
		return beaconApi.NewQrysmBeaconChainClient(validatorConn.GetBeaconApiUrl(), validatorConn.GetBeaconApiTimeout(), nodeClient)
	}
	return grpcApi.NewGrpcQrysmBeaconChainClient(validatorConn.GetGrpcClientConn())
}
