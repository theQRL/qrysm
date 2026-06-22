package apimiddleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/api/gateway/apimiddleware"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
)

type zondPublishBlockRequestJson struct {
	ZondBlock *SignedBeaconBlockZondJson `json:"zond_block"`
}

type zondPublishBlindedBlockRequestJson struct {
	ZondBlock *SignedBlindedBeaconBlockZondJson `json:"zond_block"`
}

// setInitialPublishBlockPostRequest is triggered before we deserialize the request JSON into a struct.
// We don't know which version of the block got posted, but we can determine it from the slot.
// We know that blocks of all versions have a Message field with a Slot field,
// so we deserialize the request into a struct s, which has the right fields, to obtain the slot.
// Once we know the slot, we can determine what the PostRequest field of the endpoint should be, and we set it appropriately.
func setInitialPublishBlockPostRequest(endpoint *apimiddleware.Endpoint,
	_ http.ResponseWriter,
	req *http.Request,
) (apimiddleware.RunDefault, apimiddleware.ErrorJson) {
	s := struct {
		Slot string
	}{}

	buf, err := io.ReadAll(req.Body)
	if err != nil {
		return false, apimiddleware.InternalServerErrorWithMessage(err, "could not read body")
	}

	typeParseMap := make(map[string]json.RawMessage)
	if err := json.Unmarshal(buf, &typeParseMap); err != nil {
		return false, apimiddleware.InternalServerErrorWithMessage(err, "could not parse object")
	}
	if val, ok := typeParseMap["message"]; ok {
		if err := json.Unmarshal(val, &s); err != nil {
			return false, apimiddleware.InternalServerErrorWithMessage(err, "could not unmarshal field 'message' ")
		}
	} else if val, ok := typeParseMap["signed_block"]; ok {
		temp := struct {
			Message struct {
				Slot string
			}
		}{}
		if err := json.Unmarshal(val, &temp); err != nil {
			return false, apimiddleware.InternalServerErrorWithMessage(err, "could not unmarshal field 'signed_block' ")
		}
		s.Slot = temp.Message.Slot
	} else {
		return false, &apimiddleware.DefaultErrorJson{Message: "could not parse slot from request", Code: http.StatusInternalServerError}
	}

	endpoint.PostRequest = &SignedBeaconBlockZondJson{}

	req.Body = io.NopCloser(bytes.NewBuffer(buf))
	return true, nil
}

// In preparePublishedBlock we transform the PostRequest.
// gRPC expects an XXX_block field in the JSON object, but we have a message field at this point.
// We do a simple conversion depending on the type of endpoint.PostRequest
// (which was filled out previously in setInitialPublishBlockPostRequest).
func preparePublishedBlock(endpoint *apimiddleware.Endpoint, _ http.ResponseWriter, _ *http.Request) apimiddleware.ErrorJson {
	if block, ok := endpoint.PostRequest.(*SignedBeaconBlockZondJson); ok {
		// Prepare post request that can be properly decoded on gRPC side.
		endpoint.PostRequest = &zondPublishBlockRequestJson{
			ZondBlock: block,
		}
		return nil
	}
	return apimiddleware.InternalServerError(errors.New("unsupported block type"))
}

// setInitialPublishBlindedBlockPostRequest is triggered before we deserialize the request JSON into a struct.
// We don't know which version of the block got posted, but we can determine it from the slot.
// We know that blocks of all versions have a Message field with a Slot field,
// so we deserialize the request into a struct s, which has the right fields, to obtain the slot.
// Once we know the slot, we can determine what the PostRequest field of the endpoint should be, and we set it appropriately.
func setInitialPublishBlindedBlockPostRequest(endpoint *apimiddleware.Endpoint,
	_ http.ResponseWriter,
	req *http.Request,
) (apimiddleware.RunDefault, apimiddleware.ErrorJson) {
	s := struct {
		Slot string
	}{}

	buf, err := io.ReadAll(req.Body)
	if err != nil {
		return false, apimiddleware.InternalServerErrorWithMessage(err, "could not read body")
	}

	typeParseMap := make(map[string]json.RawMessage)
	if err = json.Unmarshal(buf, &typeParseMap); err != nil {
		return false, apimiddleware.InternalServerErrorWithMessage(err, "could not parse object")
	}
	if val, ok := typeParseMap["message"]; ok {
		if err = json.Unmarshal(val, &s); err != nil {
			return false, apimiddleware.InternalServerErrorWithMessage(err, "could not unmarshal field 'message' ")
		}
	} else if val, ok = typeParseMap["signed_blinded_block"]; ok {
		temp := struct {
			Message struct {
				Slot string
			}
		}{}
		if err = json.Unmarshal(val, &temp); err != nil {
			return false, apimiddleware.InternalServerErrorWithMessage(err, "could not unmarshal field 'signed_block' ")
		}
		s.Slot = temp.Message.Slot
	} else {
		return false, &apimiddleware.DefaultErrorJson{Message: "could not parse slot from request", Code: http.StatusInternalServerError}
	}

	endpoint.PostRequest = &SignedBlindedBeaconBlockZondJson{}

	req.Body = io.NopCloser(bytes.NewBuffer(buf))
	return true, nil
}

// In preparePublishedBlindedBlock we transform the PostRequest.
// gRPC expects either an XXX_block field in the JSON object, but we have a message field at this point.
// We do a simple conversion depending on the type of endpoint.PostRequest
// (which was filled out previously in setInitialPublishBlockPostRequest).
func preparePublishedBlindedBlock(endpoint *apimiddleware.Endpoint, _ http.ResponseWriter, _ *http.Request) apimiddleware.ErrorJson {
	if block, ok := endpoint.PostRequest.(*SignedBlindedBeaconBlockZondJson); ok {
		// Prepare post request that can be properly decoded on gRPC side.
		actualPostReq := &zondPublishBlindedBlockRequestJson{
			ZondBlock: &SignedBlindedBeaconBlockZondJson{
				Message:   block.Message,
				Signature: block.Signature,
			},
		}
		endpoint.PostRequest = actualPostReq
		return nil
	}
	return apimiddleware.InternalServerError(errors.New("unsupported block type"))
}

type tempSyncCommitteesResponseJson struct {
	Data *tempSyncCommitteeValidatorsJson `json:"data"`
}

type tempSyncCommitteeValidatorsJson struct {
	Validators          []string                              `json:"validators"`
	ValidatorAggregates []*tempSyncSubcommitteeValidatorsJson `json:"validator_aggregates"`
}

type tempSyncSubcommitteeValidatorsJson struct {
	Validators []string `json:"validators"`
}

// https://ethereum.github.io/beacon-APIs/?urls.primaryName=v2.0.0#/Beacon/getEpochSyncCommittees returns validator_aggregates as a nested array.
// grpc-gateway returns a struct with nested fields which we have to transform into a plain 2D array.
func prepareValidatorAggregates(body []byte, responseContainer any) (apimiddleware.RunDefault, apimiddleware.ErrorJson) {
	tempContainer := &tempSyncCommitteesResponseJson{}
	if err := json.Unmarshal(body, tempContainer); err != nil {
		return false, apimiddleware.InternalServerErrorWithMessage(err, "could not unmarshal response into temp container")
	}
	container, ok := responseContainer.(*SyncCommitteesResponseJson)
	if !ok {
		return false, apimiddleware.InternalServerError(errors.New("container is not of the correct type"))
	}

	container.Data = &SyncCommitteeValidatorsJson{}
	container.Data.Validators = tempContainer.Data.Validators
	container.Data.ValidatorAggregates = make([][]string, len(tempContainer.Data.ValidatorAggregates))
	for i, srcValAgg := range tempContainer.Data.ValidatorAggregates {
		dstValAgg := make([]string, len(srcValAgg.Validators))
		copy(dstValAgg, tempContainer.Data.ValidatorAggregates[i].Validators)
		container.Data.ValidatorAggregates[i] = dstValAgg
	}

	return false, nil
}

type zondBlockResponseJson struct {
	Version             string                     `json:"version"`
	Data                *SignedBeaconBlockZondJson `json:"data"`
	ExecutionOptimistic bool                       `json:"execution_optimistic"`
	Finalized           bool                       `json:"finalized"`
}

type zondBlindedBlockResponseJson struct {
	Version             string                            `json:"version" enum:"true"`
	Data                *SignedBlindedBeaconBlockZondJson `json:"data"`
	ExecutionOptimistic bool                              `json:"execution_optimistic"`
	Finalized           bool                              `json:"finalized"`
}

func serializeBlock(response any) (apimiddleware.RunDefault, []byte, apimiddleware.ErrorJson) {
	respContainer, ok := response.(*BlockResponseJson)
	if !ok {
		return false, nil, apimiddleware.InternalServerError(errors.New("container is not of the correct type"))
	}

	var actualRespContainer any
	switch {
	case strings.EqualFold(respContainer.Version, strings.ToLower(qrlpb.Version_ZOND.String())):
		actualRespContainer = &zondBlockResponseJson{
			Version: respContainer.Version,
			Data: &SignedBeaconBlockZondJson{
				Message:   respContainer.Data.ZondBlock,
				Signature: respContainer.Data.Signature,
			},
			ExecutionOptimistic: respContainer.ExecutionOptimistic,
			Finalized:           respContainer.Finalized,
		}
	default:
		return false, nil, apimiddleware.InternalServerError(fmt.Errorf("unsupported block version '%s'", respContainer.Version))
	}

	j, err := json.Marshal(actualRespContainer)
	if err != nil {
		return false, nil, apimiddleware.InternalServerErrorWithMessage(err, "could not marshal response")
	}
	return false, j, nil
}

func serializeBlindedBlock(response any) (apimiddleware.RunDefault, []byte, apimiddleware.ErrorJson) {
	respContainer, ok := response.(*BlindedBlockResponseJson)
	if !ok {
		return false, nil, apimiddleware.InternalServerError(errors.New("container is not of the correct type"))
	}

	var actualRespContainer any
	switch {
	case strings.EqualFold(respContainer.Version, strings.ToLower(qrlpb.Version_ZOND.String())):
		actualRespContainer = &zondBlindedBlockResponseJson{
			Version: respContainer.Version,
			Data: &SignedBlindedBeaconBlockZondJson{
				Message:   respContainer.Data.ZondBlock,
				Signature: respContainer.Data.Signature,
			},
			ExecutionOptimistic: respContainer.ExecutionOptimistic,
			Finalized:           respContainer.Finalized,
		}
	default:
		return false, nil, apimiddleware.InternalServerError(fmt.Errorf("unsupported block version '%s'", respContainer.Version))
	}

	j, err := json.Marshal(actualRespContainer)
	if err != nil {
		return false, nil, apimiddleware.InternalServerErrorWithMessage(err, "could not marshal response")
	}
	return false, j, nil
}

type zondStateResponseJson struct {
	Version string               `json:"version" enum:"true"`
	Data    *BeaconStateZondJson `json:"data"`
}

func serializeState(response any) (apimiddleware.RunDefault, []byte, apimiddleware.ErrorJson) {
	respContainer, ok := response.(*BeaconStateResponseJson)
	if !ok {
		return false, nil, apimiddleware.InternalServerError(errors.New("container is not of the correct type"))
	}

	var actualRespContainer any
	switch {
	case strings.EqualFold(respContainer.Version, strings.ToLower(qrlpb.Version_ZOND.String())):
		actualRespContainer = &zondStateResponseJson{
			Version: respContainer.Version,
			Data:    respContainer.Data.ZondState,
		}
	default:
		return false, nil, apimiddleware.InternalServerError(fmt.Errorf("unsupported state version '%s'", respContainer.Version))
	}

	j, err := json.Marshal(actualRespContainer)
	if err != nil {
		return false, nil, apimiddleware.InternalServerErrorWithMessage(err, "could not marshal response")
	}
	return false, j, nil
}

type zondProduceBlockResponseJson struct {
	Version string               `json:"version" enum:"true"`
	Data    *BeaconBlockZondJson `json:"data"`
}

type zondProduceBlindedBlockResponseJson struct {
	Version string                      `json:"version" enum:"true"`
	Data    *BlindedBeaconBlockZondJson `json:"data"`
}

func serializeProducedBlock(response any) (apimiddleware.RunDefault, []byte, apimiddleware.ErrorJson) {
	respContainer, ok := response.(*ProduceBlockResponseJson)
	if !ok {
		return false, nil, apimiddleware.InternalServerError(errors.New("container is not of the correct type"))
	}

	var actualRespContainer any
	switch {
	case strings.EqualFold(respContainer.Version, strings.ToLower(qrlpb.Version_ZOND.String())):
		actualRespContainer = &zondProduceBlockResponseJson{
			Version: respContainer.Version,
			Data:    respContainer.Data.ZondBlock,
		}
	default:
		return false, nil, apimiddleware.InternalServerError(fmt.Errorf("unsupported block version '%s'", respContainer.Version))
	}

	j, err := json.Marshal(actualRespContainer)
	if err != nil {
		return false, nil, apimiddleware.InternalServerErrorWithMessage(err, "could not marshal response")
	}
	return false, j, nil
}

func serializeProducedBlindedBlock(response any) (apimiddleware.RunDefault, []byte, apimiddleware.ErrorJson) {
	respContainer, ok := response.(*ProduceBlindedBlockResponseJson)
	if !ok {
		return false, nil, apimiddleware.InternalServerError(errors.New("container is not of the correct type"))
	}

	var actualRespContainer any
	switch {
	case strings.EqualFold(respContainer.Version, strings.ToLower(qrlpb.Version_ZOND.String())):
		actualRespContainer = &zondProduceBlindedBlockResponseJson{
			Version: respContainer.Version,
			Data:    respContainer.Data.ZondBlock,
		}
	default:
		return false, nil, apimiddleware.InternalServerError(fmt.Errorf("unsupported block version '%s'", respContainer.Version))
	}

	j, err := json.Marshal(actualRespContainer)
	if err != nil {
		return false, nil, apimiddleware.InternalServerErrorWithMessage(err, "could not marshal response")
	}
	return false, j, nil
}

func prepareForkChoiceResponse(response any) (apimiddleware.RunDefault, []byte, apimiddleware.ErrorJson) {
	dump, ok := response.(*ForkChoiceDumpJson)
	if !ok {
		return false, nil, apimiddleware.InternalServerError(errors.New("response is not of the correct type"))
	}

	nodes := make([]*ForkChoiceNodeResponseJson, len(dump.ForkChoiceNodes))
	for i, n := range dump.ForkChoiceNodes {
		nodes[i] = &ForkChoiceNodeResponseJson{
			Slot:               n.Slot,
			BlockRoot:          n.BlockRoot,
			ParentRoot:         n.ParentRoot,
			JustifiedEpoch:     n.JustifiedEpoch,
			FinalizedEpoch:     n.FinalizedEpoch,
			Weight:             n.Weight,
			Validity:           n.Validity,
			ExecutionBlockHash: n.ExecutionBlockHash,
			ExtraData: &ForkChoiceNodeExtraDataJson{
				UnrealizedJustifiedEpoch: n.UnrealizedJustifiedEpoch,
				UnrealizedFinalizedEpoch: n.UnrealizedFinalizedEpoch,
				Balance:                  n.Balance,
				ExecutionOptimistic:      n.ExecutionOptimistic,
				TimeStamp:                n.TimeStamp,
			},
		}
	}
	forkChoice := &ForkChoiceResponseJson{
		JustifiedCheckpoint: dump.JustifiedCheckpoint,
		FinalizedCheckpoint: dump.FinalizedCheckpoint,
		ForkChoiceNodes:     nodes,
		ExtraData: &ForkChoiceResponseExtraDataJson{
			BestJustifiedCheckpoint:       dump.BestJustifiedCheckpoint,
			UnrealizedJustifiedCheckpoint: dump.UnrealizedJustifiedCheckpoint,
			UnrealizedFinalizedCheckpoint: dump.UnrealizedFinalizedCheckpoint,
			ProposerBoostRoot:             dump.ProposerBoostRoot,
			PreviousProposerBoostRoot:     dump.PreviousProposerBoostRoot,
			HeadRoot:                      dump.HeadRoot,
		},
	}

	result, err := json.Marshal(forkChoice)
	if err != nil {
		return false, nil, apimiddleware.InternalServerError(errors.New("could not marshal fork choice to JSON"))
	}
	return false, result, nil
}
