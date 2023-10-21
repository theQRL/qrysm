package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/apimiddleware"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

func (c *beaconApiValidatorClient) prepareBeaconProposer(ctx context.Context, recipients []*zondpb.PrepareBeaconProposerRequest_FeeRecipientContainer) error {
	jsonRecipients := make([]*apimiddleware.FeeRecipientJson, len(recipients))
	for index, recipient := range recipients {
		jsonRecipients[index] = &apimiddleware.FeeRecipientJson{
			FeeRecipient:   hexutil.Encode(recipient.FeeRecipient),
			ValidatorIndex: strconv.FormatUint(uint64(recipient.ValidatorIndex), 10),
		}
	}

	marshalledJsonRecipients, err := json.Marshal(jsonRecipients)
	if err != nil {
		return errors.Wrap(err, "failed to marshal recipients")
	}

	if _, err := c.jsonRestHandler.PostRestJson(ctx, "/zond/v1/validator/prepare_beacon_proposer", nil, bytes.NewBuffer(marshalledJsonRecipients), nil); err != nil {
		return errors.Wrap(err, "failed to send POST data to REST endpoint")
	}

	return nil
}
