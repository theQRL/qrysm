package apimiddleware

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/api/gateway/apimiddleware"
)

// BeaconEndpointFactory creates endpoints used for running beacon chain API calls through the API Middleware.
type BeaconEndpointFactory struct {
}

func (f *BeaconEndpointFactory) IsNil() bool {
	return f == nil
}

// Paths is a collection of all valid beacon chain API paths.
func (_ *BeaconEndpointFactory) Paths() []string {
	return []string{
		"/qrl/v1/beacon/states/{state_id}/root",
		"/qrl/v1/beacon/states/{state_id}/sync_committees",
		"/qrl/v1/beacon/states/{state_id}/randao",
		"/qrl/v1/beacon/blocks/{block_id}",
		"/qrl/v1/beacon/blocks/{block_id}/attestations",
		"/qrl/v1/beacon/blinded_blocks/{block_id}",
		"/qrl/v1/beacon/pool/attester_slashings",
		"/qrl/v1/beacon/pool/proposer_slashings",
		"/qrl/v1/beacon/weak_subjectivity",
		"/qrl/v1/node/identity",
		"/qrl/v1/node/peers",
		"/qrl/v1/node/peers/{peer_id}",
		"/qrl/v1/node/peer_count",
		"/qrl/v1/node/version",
		"/qrl/v1/node/health",
		"/qrl/v1/debug/beacon/states/{state_id}",
		"/qrl/v1/debug/beacon/heads",
		"/qrl/v1/debug/fork_choice",
		"/qrl/v1/config/fork_schedule",
		"/qrl/v1/config/spec",
		"/qrl/v1/events",
		"/qrl/v1/validator/blocks/{slot}",
		"/qrl/v1/validator/blinded_blocks/{slot}",
	}
}

// Create returns a new endpoint for the provided API path.
func (_ *BeaconEndpointFactory) Create(path string) (*apimiddleware.Endpoint, error) {
	endpoint := apimiddleware.DefaultEndpoint()
	switch path {
	case "/qrl/v1/beacon/states/{state_id}/root":
		endpoint.GetResponse = &StateRootResponseJson{}
	case "/qrl/v1/beacon/states/{state_id}/sync_committees":
		endpoint.RequestQueryParams = []apimiddleware.QueryParam{{Name: "epoch"}}
		endpoint.GetResponse = &SyncCommitteesResponseJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreDeserializeGrpcResponseBodyIntoContainer: prepareValidatorAggregates,
		}
	case "/qrl/v1/beacon/states/{state_id}/randao":
		endpoint.RequestQueryParams = []apimiddleware.QueryParam{{Name: "epoch"}}
		endpoint.GetResponse = &RandaoResponseJson{}
	case "/qrl/v1/beacon/blocks/{block_id}":
		endpoint.GetResponse = &BlockResponseJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: serializeBlock,
		}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleGetBeaconBlockSSZ}
	case "/qrl/v1/beacon/blocks/{block_id}/attestations":
		endpoint.GetResponse = &BlockAttestationsResponseJson{}
	case "/qrl/v1/beacon/blinded_blocks/{block_id}":
		endpoint.GetResponse = &BlindedBlockResponseJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: serializeBlindedBlock,
		}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleGetBlindedBeaconBlockSSZ}
	case "/qrl/v1/beacon/pool/attester_slashings":
		endpoint.PostRequest = &AttesterSlashingJson{}
		endpoint.GetResponse = &AttesterSlashingsPoolResponseJson{}
	case "/qrl/v1/beacon/pool/proposer_slashings":
		endpoint.PostRequest = &ProposerSlashingJson{}
		endpoint.GetResponse = &ProposerSlashingsPoolResponseJson{}
	case "/qrl/v1/beacon/weak_subjectivity":
		endpoint.GetResponse = &WeakSubjectivityResponse{}
	case "/qrl/v1/node/identity":
		endpoint.GetResponse = &IdentityResponseJson{}
	case "/qrl/v1/node/peers":
		endpoint.RequestQueryParams = []apimiddleware.QueryParam{{Name: "state", Enum: true}, {Name: "direction", Enum: true}}
		endpoint.GetResponse = &PeersResponseJson{}
	case "/qrl/v1/node/peers/{peer_id}":
		endpoint.RequestURLLiterals = []string{"peer_id"}
		endpoint.GetResponse = &PeerResponseJson{}
	case "/qrl/v1/node/peer_count":
		endpoint.GetResponse = &PeerCountResponseJson{}
	case "/qrl/v1/node/version":
		endpoint.GetResponse = &VersionResponseJson{}
	case "/qrl/v1/node/health":
		// Use default endpoint
	case "/qrl/v1/debug/beacon/states/{state_id}":
		endpoint.GetResponse = &BeaconStateResponseJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: serializeState,
		}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleGetBeaconStateSSZ}
	case "/qrl/v1/debug/beacon/heads":
		endpoint.GetResponse = &ForkChoiceHeadsResponseJson{}
	case "/qrl/v1/debug/fork_choice":
		endpoint.GetResponse = &ForkChoiceDumpJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: prepareForkChoiceResponse,
		}
	case "/qrl/v1/config/fork_schedule":
		endpoint.GetResponse = &ForkScheduleResponseJson{}
	case "/qrl/v1/config/spec":
		endpoint.GetResponse = &SpecResponseJson{}
	case "/qrl/v1/events":
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleEvents}
	case "/qrl/v1/validator/blocks/{slot}":
		endpoint.GetResponse = &ProduceBlockResponseJson{}
		endpoint.RequestURLLiterals = []string{"slot"}
		endpoint.RequestQueryParams = []apimiddleware.QueryParam{{Name: "randao_reveal", Hex: true}, {Name: "graffiti", Hex: true}}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: serializeProducedBlock,
		}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleProduceBlockSSZ}
	case "/qrl/v1/validator/blinded_blocks/{slot}":
		endpoint.GetResponse = &ProduceBlindedBlockResponseJson{}
		endpoint.RequestURLLiterals = []string{"slot"}
		endpoint.RequestQueryParams = []apimiddleware.QueryParam{{Name: "randao_reveal", Hex: true}, {Name: "graffiti", Hex: true}}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: serializeProducedBlindedBlock,
		}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleProduceBlindedBlockSSZ}
	default:
		return nil, errors.New("invalid path")
	}

	endpoint.Path = path
	return &endpoint, nil
}
