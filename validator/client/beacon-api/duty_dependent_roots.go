package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/validator"
	"github.com/theQRL/qrysm/consensus-types/primitives"
)

type DutyDependentRootsProvider struct {
	jsonRestHandler jsonRestHandler
}

func NewDutyDependentRootsProvider(host string, timeout time.Duration) *DutyDependentRootsProvider {
	return &DutyDependentRootsProvider{
		jsonRestHandler: newBeaconAPIJSONRestHandler(host, timeout),
	}
}

func (p *DutyDependentRootsProvider) GetDutyDependentRoots(ctx context.Context, epoch primitives.Epoch) ([]byte, []byte, error) {
	emptyValidatorIndices, err := json.Marshal([]string{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to marshal empty validator index set")
	}

	attesterDuties := &validator.GetAttesterDutiesResponse{}
	if _, err := p.jsonRestHandler.PostRestJson(ctx, fmt.Sprintf("/qrl/v1/validator/duties/attester/%d", epoch), nil, bytes.NewBuffer(emptyValidatorIndices), attesterDuties); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to query attester duty dependent root for epoch `%d`", epoch)
	}
	previousDutyDependentRoot, err := hexutil.Decode(attesterDuties.DependentRoot)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to decode attester duty dependent root")
	}

	proposerDuties := &validator.GetProposerDutiesResponse{}
	if _, err := p.jsonRestHandler.GetRestJsonResponse(ctx, fmt.Sprintf("/qrl/v1/validator/duties/proposer/%d", epoch), proposerDuties); err != nil {
		return nil, nil, errors.Wrapf(err, "failed to query proposer duty dependent root for epoch `%d`", epoch)
	}
	currentDutyDependentRoot, err := hexutil.Decode(proposerDuties.DependentRoot)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to decode proposer duty dependent root")
	}

	return previousDutyDependentRoot, currentDutyDependentRoot, nil
}
