package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/grpc"
)

type abstractSignedBlockResponseJson struct {
	Version             string          `json:"version" enum:"true"`
	ExecutionOptimistic bool            `json:"execution_optimistic"`
	Finalized           bool            `json:"finalized"`
	Data                json.RawMessage `json:"data"`
}

type streamBlocksAltairClient struct {
	grpc.ClientStream
	ctx                 context.Context
	beaconApiClient     beaconApiValidatorClient
	streamBlocksRequest *zondpb.StreamBlocksRequest
	prevBlockSlot       primitives.Slot
	pingDelay           time.Duration
}

type headSignedBeaconBlockResult struct {
	streamBlocksResponse *zondpb.StreamBlocksResponse
	executionOptimistic  bool
	slot                 primitives.Slot
}

func (c beaconApiValidatorClient) streamBlocks(ctx context.Context, in *zondpb.StreamBlocksRequest, pingDelay time.Duration) zondpb.BeaconNodeValidator_StreamBlocksAltairClient {
	return &streamBlocksAltairClient{
		ctx:                 ctx,
		beaconApiClient:     c,
		streamBlocksRequest: in,
		pingDelay:           pingDelay,
	}
}

func (c *streamBlocksAltairClient) Recv() (*zondpb.StreamBlocksResponse, error) {
	result, err := c.beaconApiClient.getHeadSignedBeaconBlock(c.ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get latest signed block")
	}

	// We keep querying the beacon chain for the latest block until we receive a new slot
	for (c.streamBlocksRequest.VerifiedOnly && result.executionOptimistic) || c.prevBlockSlot == result.slot {
		select {
		case <-time.After(c.pingDelay):
			result, err = c.beaconApiClient.getHeadSignedBeaconBlock(c.ctx)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get latest signed block")
			}
		case <-c.ctx.Done():
			return nil, errors.New("context canceled")
		}
	}

	c.prevBlockSlot = result.slot
	return result.streamBlocksResponse, nil
}

func (c beaconApiValidatorClient) getHeadSignedBeaconBlock(ctx context.Context) (*headSignedBeaconBlockResult, error) {
	// Since we don't know yet what the json looks like, we unmarshal into an abstract structure that has only a version
	// and a blob of data
	signedBlockResponseJson := abstractSignedBlockResponseJson{}
	if _, err := c.jsonRestHandler.GetRestJsonResponse(ctx, "/zond/v1/beacon/blocks/head", &signedBlockResponseJson); err != nil {
		return nil, errors.Wrap(err, "failed to query GET REST endpoint")
	}

	// Once we know what the consensus version is, we can go ahead and unmarshal into the specific structs unique to each version
	decoder := json.NewDecoder(bytes.NewReader(signedBlockResponseJson.Data))
	decoder.DisallowUnknownFields()

	response := &zondpb.StreamBlocksResponse{}
	var slot primitives.Slot

	switch signedBlockResponseJson.Version {
	case "capella":
		jsonCapellaBlock := apimiddleware.SignedBeaconBlockCapellaJson{}
		if err := decoder.Decode(&jsonCapellaBlock); err != nil {
			return nil, errors.Wrap(err, "failed to decode signed capella block response json")
		}

		capellaBlock, err := c.beaconBlockConverter.ConvertRESTCapellaBlockToProto(jsonCapellaBlock.Message)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get signed capella block")
		}

		decodedSignature, err := hexutil.Decode(jsonCapellaBlock.Signature)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode capella block signature `%s`", jsonCapellaBlock.Signature)
		}

		response.Block = &zondpb.StreamBlocksResponse_CapellaBlock{
			CapellaBlock: &zondpb.SignedBeaconBlockCapella{
				Signature: decodedSignature,
				Block:     capellaBlock,
			},
		}

		slot = capellaBlock.Slot
	default:
		return nil, errors.Errorf("unsupported consensus version `%s`", signedBlockResponseJson.Version)
	}

	return &headSignedBeaconBlockResult{
		streamBlocksResponse: response,
		executionOptimistic:  signedBlockResponseJson.ExecutionOptimistic,
		slot:                 slot,
	}, nil
}
