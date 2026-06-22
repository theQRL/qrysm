package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

type abstractProduceBlockResponseJson struct {
	Version string          `json:"version" enum:"true"`
	Data    json.RawMessage `json:"data"`
}

func (c beaconApiValidatorClient) getBeaconBlock(ctx context.Context, slot primitives.Slot, randaoReveal []byte, graffiti []byte) (*qrysmpb.GenericBeaconBlock, error) {
	queryParams := neturl.Values{}
	queryParams.Add("randao_reveal", hexutil.Encode(randaoReveal))

	if len(graffiti) > 0 {
		queryParams.Add("graffiti", hexutil.Encode(graffiti))
	}

	queryUrl := buildURL(fmt.Sprintf("/qrl/v1/validator/blocks/%d", slot), queryParams)

	// Since we don't know yet what the json looks like, we unmarshal into an abstract structure that has only a version
	// and a blob of data
	produceBlockResponseJson := abstractProduceBlockResponseJson{}
	if _, err := c.jsonRestHandler.GetRestJsonResponse(ctx, queryUrl, &produceBlockResponseJson); err != nil {
		return nil, errors.Wrap(err, "failed to query GET REST endpoint")
	}

	// Once we know what the consensus version is, we can go ahead and unmarshal into the specific structs unique to each version
	decoder := json.NewDecoder(bytes.NewReader(produceBlockResponseJson.Data))
	decoder.DisallowUnknownFields()

	response := &qrysmpb.GenericBeaconBlock{}

	switch produceBlockResponseJson.Version {
	case "zond":
		jsonZondBlock := apimiddleware.BeaconBlockZondJson{}
		if err := decoder.Decode(&jsonZondBlock); err != nil {
			return nil, errors.Wrap(err, "failed to decode zond block response json")
		}

		zondBlock, err := c.beaconBlockConverter.ConvertRESTZondBlockToProto(&jsonZondBlock)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get zond block")
		}
		response.Block = &qrysmpb.GenericBeaconBlock_Zond{
			Zond: zondBlock,
		}
	default:
		return nil, errors.Errorf("unsupported consensus version `%s`", produceBlockResponseJson.Version)
	}
	return response, nil
}
