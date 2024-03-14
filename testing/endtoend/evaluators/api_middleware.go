package evaluators

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/theQRL/qrysm/v4/proto/zond/service"
	zondpbv1 "github.com/theQRL/qrysm/v4/proto/zond/v1"
	"github.com/theQRL/qrysm/v4/testing/endtoend/params"
	"github.com/theQRL/qrysm/v4/testing/endtoend/policies"
	e2etypes "github.com/theQRL/qrysm/v4/testing/endtoend/types"
	"google.golang.org/grpc"
)

// APIMiddlewareVerifyIntegrity tests our API Middleware for the official Ethereum API.
// This ensures our API Middleware returns good data compared to gRPC.
var APIMiddlewareVerifyIntegrity = e2etypes.Evaluator{
	Name:       "api_middleware_verify_integrity_epoch_%d",
	Policy:     policies.OnEpoch(6),
	Evaluation: apiMiddlewareVerify,
}

const (
	v1MiddlewarePathTemplate = "http://localhost:%d/zond/v1"
)

func apiMiddlewareVerify(_ *e2etypes.EvaluationContext, conns ...*grpc.ClientConn) error {
	for beaconNodeIdx, conn := range conns {
		if err := runAPIComparisonFunctions(
			beaconNodeIdx,
			conn,
			withCompareSyncCommittee,
		); err != nil {
			return err
		}
	}
	return nil
}

func withCompareSyncCommittee(beaconNodeIdx int, conn *grpc.ClientConn) error {
	type syncCommitteeValidatorsJson struct {
		Validators          []string   `json:"validators"`
		ValidatorAggregates [][]string `json:"validator_aggregates"`
	}
	type syncCommitteesResponseJson struct {
		Data *syncCommitteeValidatorsJson `json:"data"`
	}
	ctx := context.Background()
	beaconClient := service.NewBeaconChainClient(conn)
	resp, err := beaconClient.ListSyncCommittees(ctx, &zondpbv1.StateSyncCommitteesRequest{
		StateId: []byte("head"),
	})
	if err != nil {
		return err
	}
	respJSON := &syncCommitteesResponseJson{}
	if err := doMiddlewareJSONGetRequestV1(
		"/beacon/states/head/sync_committees",
		beaconNodeIdx,
		respJSON,
	); err != nil {
		return err
	}
	if len(respJSON.Data.Validators) != len(resp.Data.Validators) {
		return fmt.Errorf(
			"API Middleware number of validators %d does not match gRPC %d",
			len(respJSON.Data.Validators),
			len(resp.Data.Validators),
		)
	}
	if len(respJSON.Data.ValidatorAggregates) != len(resp.Data.ValidatorAggregates) {
		return fmt.Errorf(
			"API Middleware number of validator aggregates %d does not match gRPC %d",
			len(respJSON.Data.ValidatorAggregates),
			len(resp.Data.ValidatorAggregates),
		)
	}
	return nil
}

func doMiddlewareJSONGetRequestV1(requestPath string, beaconNodeIdx int, dst interface{}) error {
	basePath := fmt.Sprintf(v1MiddlewarePathTemplate, params.TestParams.Ports.QrysmBeaconNodeGatewayPort+beaconNodeIdx)
	httpResp, err := http.Get(
		basePath + requestPath,
	)
	if err != nil {
		return err
	}
	return json.NewDecoder(httpResp.Body).Decode(&dst)
}
