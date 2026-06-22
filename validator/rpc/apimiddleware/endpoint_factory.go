package apimiddleware

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/api/gateway/apimiddleware"
)

// ValidatorEndpointFactory creates endpoints used for running validator API calls through the API Middleware.
type ValidatorEndpointFactory struct {
}

func (f *ValidatorEndpointFactory) IsNil() bool {
	return f == nil
}

// Paths is a collection of all valid validator API paths.
func (*ValidatorEndpointFactory) Paths() []string {
	return []string{
		"/qrl/v1/keystores",
		// "/qrl/v1/remotekeys",
		"/qrl/v1/validator/{pubkey}/feerecipient",
		"/qrl/v1/validator/{pubkey}/gas_limit",
		"/qrl/v1/validator/{pubkey}/voluntary_exit",
	}
}

// Create returns a new endpoint for the provided API path.
func (*ValidatorEndpointFactory) Create(path string) (*apimiddleware.Endpoint, error) {
	endpoint := apimiddleware.DefaultEndpoint()
	switch path {
	case "/qrl/v1/keystores":
		endpoint.GetResponse = &ListKeystoresResponseJson{}
		endpoint.PostRequest = &ImportKeystoresRequestJson{}
		endpoint.PostResponse = &ImportKeystoresResponseJson{}
		endpoint.DeleteRequest = &DeleteKeystoresRequestJson{}
		endpoint.DeleteResponse = &DeleteKeystoresResponseJson{}
	// case "/qrl/v1/remotekeys":
	// 	endpoint.GetResponse = &ListRemoteKeysResponseJson{}
	// 	endpoint.PostRequest = &ImportRemoteKeysRequestJson{}
	// 	endpoint.PostResponse = &ImportRemoteKeysResponseJson{}
	// 	endpoint.DeleteRequest = &DeleteRemoteKeysRequestJson{}
	// 	endpoint.DeleteResponse = &DeleteRemoteKeysResponseJson{}
	case "/qrl/v1/validator/{pubkey}/feerecipient":
		endpoint.GetResponse = &GetFeeRecipientByPubkeyResponseJson{}
		endpoint.PostRequest = &SetFeeRecipientByPubkeyRequestJson{}
		endpoint.DeleteRequest = &DeleteFeeRecipientByPubkeyRequestJson{}
	case "/qrl/v1/validator/{pubkey}/gas_limit":
		endpoint.GetResponse = &GetGasLimitResponseJson{}
		endpoint.PostRequest = &SetGasLimitRequestJson{}
		endpoint.DeleteRequest = &DeleteGasLimitRequestJson{}
	case "/qrl/v1/validator/{pubkey}/voluntary_exit":
		endpoint.PostRequest = &SetVoluntaryExitRequestJson{}
		endpoint.PostResponse = &SetVoluntaryExitResponseJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreDeserializeRequestBodyIntoContainer: setVoluntaryExitEpoch,
		}
	default:
		return nil, errors.New("invalid path")
	}
	endpoint.Path = path
	return &endpoint, nil
}
