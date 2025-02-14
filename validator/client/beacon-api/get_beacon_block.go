package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"

	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

type abstractProduceBlockResponseJson struct {
	Version string          `json:"version" enum:"true"`
	Data    json.RawMessage `json:"data"`
}

func (c beaconApiValidatorClient) getBeaconBlock(ctx context.Context, slot primitives.Slot, randaoReveal []byte, graffiti []byte) (*zondpb.GenericBeaconBlock, error) {
	queryParams := neturl.Values{}
	queryParams.Add("randao_reveal", hexutil.Encode(randaoReveal))

	if len(graffiti) > 0 {
		queryParams.Add("graffiti", hexutil.Encode(graffiti))
	}

	queryUrl := buildURL(fmt.Sprintf("/zond/v1/validator/blocks/%d", slot), queryParams)

	// Since we don't know yet what the json looks like, we unmarshal into an abstract structure that has only a version
	// and a blob of data
	produceBlockResponseJson := abstractProduceBlockResponseJson{}
	if _, err := c.jsonRestHandler.GetRestJsonResponse(ctx, queryUrl, &produceBlockResponseJson); err != nil {
		return nil, errors.Wrap(err, "failed to query GET REST endpoint")
	}

	// Once we know what the consensus version is, we can go ahead and unmarshal into the specific structs unique to each version
	decoder := json.NewDecoder(bytes.NewReader(produceBlockResponseJson.Data))
	decoder.DisallowUnknownFields()

	response := &zondpb.GenericBeaconBlock{}

	switch produceBlockResponseJson.Version {
	case "capella":
		jsonCapellaBlock := apimiddleware.BeaconBlockCapellaJson{}
		if err := decoder.Decode(&jsonCapellaBlock); err != nil {
			return nil, errors.Wrap(err, "failed to decode capella block response json")
		}

		capellaBlock, err := c.beaconBlockConverter.ConvertRESTCapellaBlockToProto(&jsonCapellaBlock)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get capella block")
		}
		response.Block = &zondpb.GenericBeaconBlock_Capella{
			Capella: capellaBlock,
		}
	default:
		return nil, errors.Errorf("unsupported consensus version `%s`", produceBlockResponseJson.Version)
	}
	return response, nil
}
