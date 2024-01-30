package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/zond/shared"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

func (c *beaconApiValidatorClient) submitValidatorRegistrations(ctx context.Context, registrations []*zondpb.SignedValidatorRegistrationV1) error {
	const endpoint = "/zond/v1/validator/register_validator"

	jsonRegistration := make([]*shared.SignedValidatorRegistration, len(registrations))

	for index, registration := range registrations {
		outMessage, err := shared.SignedValidatorRegistrationFromConsensus(registration)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to encode to SignedValidatorRegistration at index %d", index))
		}
		jsonRegistration[index] = outMessage
	}

	marshalledJsonRegistration, err := json.Marshal(jsonRegistration)
	if err != nil {
		return errors.Wrap(err, "failed to marshal registration")
	}

	if _, err := c.jsonRestHandler.PostRestJson(ctx, endpoint, nil, bytes.NewBuffer(marshalledJsonRegistration), nil); err != nil {
		return errors.Wrapf(err, "failed to send POST data to `%s` REST endpoint", endpoint)
	}

	return nil
}
