package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/beacon-chain/rpc/zond/shared"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
)

func (c *beaconApiValidatorClient) submitSyncMessage(ctx context.Context, syncMessage *zondpb.SyncCommitteeMessage) error {
	const endpoint = "/zond/v1/beacon/pool/sync_committees"

	jsonSyncCommitteeMessage := &shared.SyncCommitteeMessage{
		Slot:            strconv.FormatUint(uint64(syncMessage.Slot), 10),
		BeaconBlockRoot: hexutil.Encode(syncMessage.BlockRoot),
		ValidatorIndex:  strconv.FormatUint(uint64(syncMessage.ValidatorIndex), 10),
		Signature:       hexutil.Encode(syncMessage.Signature),
	}

	marshalledJsonSyncCommitteeMessage, err := json.Marshal([]*shared.SyncCommitteeMessage{jsonSyncCommitteeMessage})
	if err != nil {
		return errors.Wrap(err, "failed to marshal sync committee message")
	}

	if _, err := c.jsonRestHandler.PostRestJson(ctx, endpoint, nil, bytes.NewBuffer(marshalledJsonSyncCommitteeMessage), nil); err != nil {
		return errors.Wrapf(err, "failed to send POST data to `%s` REST endpoint", endpoint)
	}

	return nil
}

func (c *beaconApiValidatorClient) getSyncMessageBlockRoot(ctx context.Context) (*zondpb.SyncMessageBlockRootResponse, error) {
	// Get head beacon block root.
	var resp apimiddleware.BlockRootResponseJson
	if _, err := c.jsonRestHandler.GetRestJsonResponse(ctx, "/zond/v1/beacon/blocks/head/root", &resp); err != nil {
		return nil, errors.Wrap(err, "failed to query GET REST endpoint")
	}

	// An optimistic validator MUST NOT participate in sync committees
	// (i.e., sign across the DOMAIN_SYNC_COMMITTEE, DOMAIN_SYNC_COMMITTEE_SELECTION_PROOF or DOMAIN_CONTRIBUTION_AND_PROOF domains).
	if resp.ExecutionOptimistic {
		return nil, errors.New("the node is currently optimistic and cannot serve validators")
	}

	if resp.Data == nil {
		return nil, errors.New("no data returned")
	}

	if resp.Data.Root == "" {
		return nil, errors.New("no root returned")
	}

	blockRoot, err := hexutil.Decode(resp.Data.Root)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode beacon block root")
	}

	return &zondpb.SyncMessageBlockRootResponse{
		Root: blockRoot,
	}, nil
}

func (c *beaconApiValidatorClient) getSyncCommitteeContribution(
	ctx context.Context,
	req *zondpb.SyncCommitteeContributionRequest,
) (*zondpb.SyncCommitteeContribution, error) {
	blockRootResponse, err := c.getSyncMessageBlockRoot(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sync message block root")
	}

	blockRoot := hexutil.Encode(blockRootResponse.Root)

	params := url.Values{}
	params.Add("slot", strconv.FormatUint(uint64(req.Slot), 10))
	params.Add("subcommittee_index", strconv.FormatUint(req.SubnetId, 10))
	params.Add("beacon_block_root", blockRoot)

	url := buildURL("/zond/v1/validator/sync_committee_contribution", params)

	var resp apimiddleware.ProduceSyncCommitteeContributionResponseJson
	if _, err := c.jsonRestHandler.GetRestJsonResponse(ctx, url, &resp); err != nil {
		return nil, errors.Wrap(err, "failed to query GET REST endpoint")
	}

	return convertSyncContributionJsonToProto(resp.Data)
}

func (c *beaconApiValidatorClient) getSyncSubcommitteeIndex(ctx context.Context, in *zondpb.SyncSubcommitteeIndexRequest) (*zondpb.SyncSubcommitteeIndexResponse, error) {
	validatorIndexResponse, err := c.validatorIndex(ctx, &zondpb.ValidatorIndexRequest{PublicKey: in.PublicKey})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator index")
	}

	syncDuties, err := c.dutiesProvider.GetSyncDuties(ctx, slots.ToEpoch(in.Slot), []primitives.ValidatorIndex{validatorIndexResponse.Index})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sync committee duties")
	}

	if len(syncDuties) == 0 {
		return nil, errors.Errorf("no sync committee duty for the given slot %d", in.Slot)
	}

	// First sync duty is required since we requested sync duties for one validator index.
	syncDuty := syncDuties[0]

	var indices []primitives.CommitteeIndex
	for _, idx := range syncDuty.ValidatorSyncCommitteeIndices {
		syncCommIdx, err := strconv.ParseUint(idx, 10, 64)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse validator sync committee index %s", idx)
		}

		indices = append(indices, primitives.CommitteeIndex(syncCommIdx))
	}

	return &zondpb.SyncSubcommitteeIndexResponse{Indices: indices}, nil
}

func convertSyncContributionJsonToProto(contribution *apimiddleware.SyncCommitteeContributionJson) (*zondpb.SyncCommitteeContribution, error) {
	if contribution == nil {
		return nil, errors.New("sync committee contribution is nil")
	}

	slot, err := strconv.ParseUint(contribution.Slot, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse slot `%s`", contribution.Slot)
	}

	blockRoot, err := hexutil.Decode(contribution.BeaconBlockRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode beacon block root `%s`", contribution.BeaconBlockRoot)
	}

	subcommitteeIdx, err := strconv.ParseUint(contribution.SubcommitteeIndex, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse subcommittee index `%s`", contribution.SubcommitteeIndex)
	}

	aggregationBits, err := hexutil.Decode(contribution.AggregationBits)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode aggregation bits `%s`", contribution.AggregationBits)
	}

	signatures := make([][]byte, len(contribution.Signatures))
	for i, sig := range contribution.Signatures {
		decodedSig, err := hexutil.Decode(sig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode contribution signature `%s`", sig)
		}
		signatures[i] = decodedSig
	}

	return &zondpb.SyncCommitteeContribution{
		Slot:              primitives.Slot(slot),
		BlockRoot:         blockRoot,
		SubcommitteeIndex: subcommitteeIdx,
		AggregationBits:   aggregationBits,
		Signatures:        signatures,
	}, nil
}
