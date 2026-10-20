package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrysm/validator"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/validator/client/iface"
	"google.golang.org/protobuf/types/known/emptypb"
)

// NewQrysmBeaconChainClient returns a REST-backed QrysmChainClient. The
// supplied nodeClient is used to verify the connected beacon node is a qrysm
// node before calling the qrysm-specific performance endpoint.
func NewQrysmBeaconChainClient(host string, timeout time.Duration, nodeClient iface.NodeClient) iface.QrysmChainClient {
	return &qrysmBeaconChainClient{
		nodeClient:      nodeClient,
		jsonRestHandler: newBeaconAPIJSONRestHandler(host, timeout),
	}
}

const getValidatorPerformanceEndpoint = "/qrysm/validators/performance"

type qrysmBeaconChainClient struct {
	nodeClient      iface.NodeClient
	jsonRestHandler jsonRestHandler
}

// GetValidatorPerformance calls the qrysm-specific performance endpoint, after
// verifying the connected beacon node is a qrysm node. Returns iface.ErrNotSupported
// when talking to a non-qrysm beacon node so callers can skip cleanly.
func (c qrysmBeaconChainClient) GetValidatorPerformance(ctx context.Context, in *qrysmpb.ValidatorPerformanceRequest) (*qrysmpb.ValidatorPerformanceResponse, error) {
	nodeVersion, err := c.nodeClient.GetVersion(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get node version")
	}
	if !strings.Contains(strings.ToLower(nodeVersion.Version), "qrysm") {
		return nil, iface.ErrNotSupported
	}

	request, err := json.Marshal(validator.ValidatorPerformanceRequest{
		PublicKeys: in.PublicKeys,
		Indices:    in.Indices,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal request")
	}
	resp := &validator.ValidatorPerformanceResponse{}
	if _, err := c.jsonRestHandler.PostRestJson(
		ctx,
		getValidatorPerformanceEndpoint,
		nil,
		bytes.NewBuffer(request),
		resp,
	); err != nil {
		return nil, errors.Wrap(err, "failed to get validator performance")
	}

	return &qrysmpb.ValidatorPerformanceResponse{
		CurrentEffectiveBalances:      resp.CurrentEffectiveBalances,
		CorrectlyVotedSource:          resp.CorrectlyVotedSource,
		CorrectlyVotedTarget:          resp.CorrectlyVotedTarget,
		CorrectlyVotedHead:            resp.CorrectlyVotedHead,
		BalancesBeforeEpochTransition: resp.BalancesBeforeEpochTransition,
		BalancesAfterEpochTransition:  resp.BalancesAfterEpochTransition,
		MissingValidators:             resp.MissingValidators,
		PublicKeys:                    resp.PublicKeys,
		InactivityScores:              resp.InactivityScores,
	}, nil
}
