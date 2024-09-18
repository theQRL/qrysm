package beacon_api

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func (c beaconApiValidatorClient) validatorIndex(ctx context.Context, in *zondpb.ValidatorIndexRequest) (*zondpb.ValidatorIndexResponse, error) {
	stringPubKey := hexutil.Encode(in.PublicKey)

	stateValidator, err := c.stateValidatorsProvider.GetStateValidators(ctx, []string{stringPubKey}, nil, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get state validator")
	}

	if len(stateValidator.Data) == 0 {
		return nil, errors.Errorf("could not find validator index for public key `%s`", stringPubKey)
	}

	stringValidatorIndex := stateValidator.Data[0].Index

	index, err := strconv.ParseUint(stringValidatorIndex, 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse validator index")
	}

	return &zondpb.ValidatorIndexResponse{Index: primitives.ValidatorIndex(index)}, nil
}
