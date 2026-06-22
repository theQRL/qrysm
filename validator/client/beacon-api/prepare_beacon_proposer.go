package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/shared"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func (c *beaconApiValidatorClient) prepareBeaconProposer(ctx context.Context, recipients []*qrysmpb.PrepareBeaconProposerRequest_FeeRecipientContainer) error {
	jsonRecipients := make([]*shared.FeeRecipient, len(recipients))
	for index, recipient := range recipients {
		jsonRecipients[index] = &shared.FeeRecipient{
			FeeRecipient:   hexutil.EncodeQ(recipient.FeeRecipient),
			ValidatorIndex: strconv.FormatUint(uint64(recipient.ValidatorIndex), 10),
		}
	}

	marshalledJsonRecipients, err := json.Marshal(jsonRecipients)
	if err != nil {
		return errors.Wrap(err, "failed to marshal recipients")
	}

	if _, err := c.jsonRestHandler.PostRestJson(ctx, "/qrl/v1/validator/prepare_beacon_proposer", nil, bytes.NewBuffer(marshalledJsonRecipients), nil); err != nil {
		return errors.Wrap(err, "failed to send POST data to REST endpoint")
	}

	return nil
}
