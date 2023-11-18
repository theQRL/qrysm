package apimiddleware

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/api/gateway/apimiddleware"
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
		"/zond/v1/beacon/states/{state_id}/root",
		"/zond/v1/beacon/states/{state_id}/sync_committees",
		"/zond/v1/beacon/states/{state_id}/randao",
		"/zond/v1/beacon/blocks/{block_id}",
		"/zond/v2/beacon/blocks/{block_id}",
		"/zond/v1/beacon/blocks/{block_id}/attestations",
		"/zond/v1/beacon/blinded_blocks/{block_id}",
		"/zond/v1/beacon/pool/attester_slashings",
		"/zond/v1/beacon/pool/proposer_slashings",
		"/zond/v1/beacon/pool/dilithium_to_execution_changes",
		"/zond/v1/beacon/pool/dilithium_to_execution_changes",
		"/zond/v1/beacon/weak_subjectivity",
		"/zond/v1/node/identity",
		"/zond/v1/node/peers",
		"/zond/v1/node/peers/{peer_id}",
		"/zond/v1/node/peer_count",
		"/zond/v1/node/version",
		"/zond/v1/node/health",
		"/zond/v1/debug/beacon/states/{state_id}",
		"/zond/v2/debug/beacon/states/{state_id}",
		"/zond/v1/debug/beacon/heads",
		"/zond/v2/debug/beacon/heads",
		"/zond/v1/debug/fork_choice",
		"/zond/v1/config/fork_schedule",
		"/zond/v1/config/spec",
		"/zond/v1/events",
		"/zond/v1/validator/blocks/{slot}",
		"/zond/v2/validator/blocks/{slot}",
		"/zond/v1/validator/blinded_blocks/{slot}",
	}
}

// Create returns a new endpoint for the provided API path.
func (_ *BeaconEndpointFactory) Create(path string) (*apimiddleware.Endpoint, error) {
	endpoint := apimiddleware.DefaultEndpoint()
	switch path {
	case "/zond/v1/beacon/states/{state_id}/root":
		endpoint.GetResponse = &StateRootResponseJson{}
	case "/zond/v1/beacon/states/{state_id}/sync_committees":
		endpoint.RequestQueryParams = []apimiddleware.QueryParam{{Name: "epoch"}}
		endpoint.GetResponse = &SyncCommitteesResponseJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreDeserializeGrpcResponseBodyIntoContainer: prepareValidatorAggregates,
		}
	case "/zond/v1/beacon/states/{state_id}/randao":
		endpoint.RequestQueryParams = []apimiddleware.QueryParam{{Name: "epoch"}}
		endpoint.GetResponse = &RandaoResponseJson{}
	case "/zond/v1/beacon/blocks/{block_id}":
		endpoint.GetResponse = &BlockResponseJson{}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleGetBeaconBlockSSZ}
	case "/zond/v2/beacon/blocks/{block_id}":
		endpoint.GetResponse = &BlockV2ResponseJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: serializeV2Block,
		}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleGetBeaconBlockSSZV2}
	case "/zond/v1/beacon/blocks/{block_id}/attestations":
		endpoint.GetResponse = &BlockAttestationsResponseJson{}
	case "/zond/v1/beacon/blinded_blocks/{block_id}":
		endpoint.GetResponse = &BlindedBlockResponseJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: serializeBlindedBlock,
		}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleGetBlindedBeaconBlockSSZ}
	case "/zond/v1/beacon/pool/attester_slashings":
		endpoint.PostRequest = &AttesterSlashingJson{}
		endpoint.GetResponse = &AttesterSlashingsPoolResponseJson{}
	case "/zond/v1/beacon/pool/proposer_slashings":
		endpoint.PostRequest = &ProposerSlashingJson{}
		endpoint.GetResponse = &ProposerSlashingsPoolResponseJson{}
	case "/zond/v1/beacon/pool/dilithium_to_execution_changes":
		endpoint.PostRequest = &SubmitDilithiumToExecutionChangesRequest{}
		endpoint.GetResponse = &DilithiumToExecutionChangesPoolResponseJson{}
		endpoint.Err = &IndexedVerificationFailureErrorJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreDeserializeRequestBodyIntoContainer: wrapDilithiumChangesArray,
		}
	case "/zond/v1/beacon/weak_subjectivity":
		endpoint.GetResponse = &WeakSubjectivityResponse{}
	case "/zond/v1/node/identity":
		endpoint.GetResponse = &IdentityResponseJson{}
	case "/zond/v1/node/peers":
		endpoint.RequestQueryParams = []apimiddleware.QueryParam{{Name: "state", Enum: true}, {Name: "direction", Enum: true}}
		endpoint.GetResponse = &PeersResponseJson{}
	case "/zond/v1/node/peers/{peer_id}":
		endpoint.RequestURLLiterals = []string{"peer_id"}
		endpoint.GetResponse = &PeerResponseJson{}
	case "/zond/v1/node/peer_count":
		endpoint.GetResponse = &PeerCountResponseJson{}
	case "/zond/v1/node/version":
		endpoint.GetResponse = &VersionResponseJson{}
	case "/zond/v1/node/health":
		// Use default endpoint
	case "/zond/v1/debug/beacon/states/{state_id}":
		endpoint.GetResponse = &BeaconStateResponseJson{}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleGetBeaconStateSSZ}
	case "/zond/v2/debug/beacon/states/{state_id}":
		endpoint.GetResponse = &BeaconStateV2ResponseJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: serializeV2State,
		}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleGetBeaconStateSSZV2}
	case "/zond/v1/debug/beacon/heads":
		endpoint.GetResponse = &ForkChoiceHeadsResponseJson{}
	case "/zond/v2/debug/beacon/heads":
		endpoint.GetResponse = &V2ForkChoiceHeadsResponseJson{}
	case "/zond/v1/debug/fork_choice":
		endpoint.GetResponse = &ForkChoiceDumpJson{}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: prepareForkChoiceResponse,
		}
	case "/zond/v1/config/fork_schedule":
		endpoint.GetResponse = &ForkScheduleResponseJson{}
	case "/zond/v1/config/spec":
		endpoint.GetResponse = &SpecResponseJson{}
	case "/zond/v1/events":
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleEvents}
	case "/zond/v1/validator/blocks/{slot}":
		endpoint.GetResponse = &ProduceBlockResponseJson{}
		endpoint.RequestURLLiterals = []string{"slot"}
		endpoint.RequestQueryParams = []apimiddleware.QueryParam{{Name: "randao_reveal", Hex: true}, {Name: "graffiti", Hex: true}}
	case "/zond/v2/validator/blocks/{slot}":
		endpoint.GetResponse = &ProduceBlockResponseV2Json{}
		endpoint.RequestURLLiterals = []string{"slot"}
		endpoint.RequestQueryParams = []apimiddleware.QueryParam{{Name: "randao_reveal", Hex: true}, {Name: "graffiti", Hex: true}}
		endpoint.Hooks = apimiddleware.HookCollection{
			OnPreSerializeMiddlewareResponseIntoJson: serializeProducedV2Block,
		}
		endpoint.CustomHandlers = []apimiddleware.CustomHandler{handleProduceBlockSSZ}
	case "/zond/v1/validator/blinded_blocks/{slot}":
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
