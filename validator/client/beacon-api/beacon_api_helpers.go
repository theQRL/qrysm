package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	neturl "net/url"
	"regexp"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/zond/beacon"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/zond/validator"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"

	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

var beaconAPITogRPCValidatorStatus = map[string]zondpb.ValidatorStatus{
	"pending_initialized": zondpb.ValidatorStatus_DEPOSITED,
	"pending_queued":      zondpb.ValidatorStatus_PENDING,
	"active_ongoing":      zondpb.ValidatorStatus_ACTIVE,
	"active_exiting":      zondpb.ValidatorStatus_EXITING,
	"active_slashed":      zondpb.ValidatorStatus_SLASHING,
	"exited_unslashed":    zondpb.ValidatorStatus_EXITED,
	"exited_slashed":      zondpb.ValidatorStatus_EXITED,
	"withdrawal_possible": zondpb.ValidatorStatus_EXITED,
	"withdrawal_done":     zondpb.ValidatorStatus_EXITED,
}

func validRoot(root string) bool {
	matchesRegex, err := regexp.MatchString("^0x[a-fA-F0-9]{64}$", root)
	if err != nil {
		return false
	}
	return matchesRegex
}

func uint64ToString[T uint64 | primitives.Slot | primitives.ValidatorIndex | primitives.CommitteeIndex | primitives.Epoch](val T) string {
	return strconv.FormatUint(uint64(val), 10)
}

func buildURL(path string, queryParams ...neturl.Values) string {
	if len(queryParams) == 0 {
		return path
	}

	return fmt.Sprintf("%s?%s", path, queryParams[0].Encode())
}

func (c *beaconApiValidatorClient) getFork(ctx context.Context) (*beacon.GetStateForkResponse, error) {
	const endpoint = "/zond/v1/beacon/states/head/fork"

	stateForkResponseJson := &beacon.GetStateForkResponse{}

	if _, err := c.jsonRestHandler.GetRestJsonResponse(
		ctx,
		endpoint,
		stateForkResponseJson,
	); err != nil {
		return nil, errors.Wrapf(err, "failed to get json response from `%s` REST endpoint", endpoint)
	}

	return stateForkResponseJson, nil
}

func (c *beaconApiValidatorClient) getHeaders(ctx context.Context) (*beacon.GetBlockHeadersResponse, error) {
	const endpoint = "/zond/v1/beacon/headers"

	blockHeadersResponseJson := &beacon.GetBlockHeadersResponse{}

	if _, err := c.jsonRestHandler.GetRestJsonResponse(
		ctx,
		endpoint,
		blockHeadersResponseJson,
	); err != nil {
		return nil, errors.Wrapf(err, "failed to get json response from `%s` REST endpoint", endpoint)
	}

	return blockHeadersResponseJson, nil
}

func (c *beaconApiValidatorClient) getLiveness(ctx context.Context, epoch primitives.Epoch, validatorIndexes []string) (*validator.GetLivenessResponse, error) {
	const endpoint = "/zond/v1/validator/liveness/"
	url := endpoint + strconv.FormatUint(uint64(epoch), 10)

	livenessResponseJson := &validator.GetLivenessResponse{}

	marshalledJsonValidatorIndexes, err := json.Marshal(validatorIndexes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal validator indexes")
	}

	if _, err := c.jsonRestHandler.PostRestJson(ctx, url, nil, bytes.NewBuffer(marshalledJsonValidatorIndexes), livenessResponseJson); err != nil {
		return nil, errors.Wrapf(err, "failed to send POST data to `%s` REST URL", url)
	}

	return livenessResponseJson, nil
}

func (c *beaconApiValidatorClient) getSyncing(ctx context.Context) (*apimiddleware.SyncingResponseJson, error) {
	const endpoint = "/zond/v1/node/syncing"

	syncingResponseJson := &apimiddleware.SyncingResponseJson{}

	if _, err := c.jsonRestHandler.GetRestJsonResponse(
		ctx,
		endpoint,
		syncingResponseJson,
	); err != nil {
		return nil, errors.Wrapf(err, "failed to get json response from `%s` REST endpoint", endpoint)
	}

	return syncingResponseJson, nil
}

func (c *beaconApiValidatorClient) isSyncing(ctx context.Context) (bool, error) {
	response, err := c.getSyncing(ctx)
	if err != nil || response == nil || response.Data == nil {
		return true, errors.Wrapf(err, "failed to get syncing status")
	}

	return response.Data.IsSyncing, err
}

func (c *beaconApiValidatorClient) isOptimistic(ctx context.Context) (bool, error) {
	response, err := c.getSyncing(ctx)
	if err != nil || response == nil || response.Data == nil {
		return true, errors.Wrapf(err, "failed to get syncing status")
	}

	return response.Data.IsOptimistic, err
}
