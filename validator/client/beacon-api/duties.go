package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/beacon"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/shared"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/validator"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

type dutiesProvider interface {
	GetAttesterDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) (*validator.GetAttesterDutiesResponse, error)
	GetProposerDuties(ctx context.Context, epoch primitives.Epoch) (*validator.GetProposerDutiesResponse, error)
	GetSyncDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*validator.SyncCommitteeDuty, error)
	GetCommittees(ctx context.Context, epoch primitives.Epoch) ([]*shared.Committee, error)
}

type beaconApiDutiesProvider struct {
	jsonRestHandler jsonRestHandler
}

type committeeIndexSlotPair struct {
	committeeIndex primitives.CommitteeIndex
	slot           primitives.Slot
}

func (c beaconApiValidatorClient) getDuties(ctx context.Context, in *qrysmpb.DutiesRequest) (*qrysmpb.DutiesResponse, error) {
	all, err := c.multipleValidatorStatus(ctx, &qrysmpb.MultipleValidatorStatusRequest{PublicKeys: in.PublicKeys})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get validator status")
	}
	// Filter out validators with UNKNOWN_STATUS — beacon API endpoints reject
	// unknown public keys, so passing them through would fail the entire duty
	// fetch when even one validator in the subscription set is unrecognised.
	known := &qrysmpb.MultipleValidatorStatusResponse{
		PublicKeys: make([][]byte, 0, len(all.PublicKeys)),
		Statuses:   make([]*qrysmpb.ValidatorStatusResponse, 0, len(all.Statuses)),
		Indices:    make([]primitives.ValidatorIndex, 0, len(all.Indices)),
	}
	for i, status := range all.Statuses {
		if status.Status != qrysmpb.ValidatorStatus_UNKNOWN_STATUS {
			known.PublicKeys = append(known.PublicKeys, all.PublicKeys[i])
			known.Statuses = append(known.Statuses, all.Statuses[i])
			known.Indices = append(known.Indices, all.Indices[i])
		}
	}

	currentEpochDuties, prevDependentRoot, currDependentRoot, err := c.getDutiesForEpoch(ctx, in.Epoch, known)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get duties for current epoch `%d`", in.Epoch)
	}

	nextEpochDuties, _, _, err := c.getDutiesForEpoch(ctx, in.Epoch+1, known)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get duties for next epoch `%d`", in.Epoch+1)
	}

	return &qrysmpb.DutiesResponse{
		CurrentEpochDuties:        currentEpochDuties,
		NextEpochDuties:           nextEpochDuties,
		PreviousDutyDependentRoot: prevDependentRoot,
		CurrentDutyDependentRoot:  currDependentRoot,
	}, nil
}

func (c beaconApiValidatorClient) getDutiesForEpoch(
	ctx context.Context,
	epoch primitives.Epoch,
	multipleValidatorStatus *qrysmpb.MultipleValidatorStatusResponse,
) ([]*qrysmpb.DutiesResponse_Duty, []byte, []byte, error) {
	attesterDutiesResp, err := c.dutiesProvider.GetAttesterDuties(ctx, epoch, multipleValidatorStatus.Indices)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to get attester duties for epoch `%d`", epoch)
	}
	attesterDuties := attesterDutiesResp.Data

	var syncDuties []*validator.SyncCommitteeDuty
	if syncDuties, err = c.dutiesProvider.GetSyncDuties(ctx, epoch, multipleValidatorStatus.Indices); err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to get sync duties for epoch `%d`", epoch)
	}

	proposerDutiesResp, err := c.dutiesProvider.GetProposerDuties(ctx, epoch)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to get proposer duties for epoch `%d`", epoch)
	}
	proposerDuties := proposerDutiesResp.Data

	committees, err := c.dutiesProvider.GetCommittees(ctx, epoch)
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to get committees for epoch `%d`", epoch)
	}

	// Capture the duty dependent roots from the same REST responses we already fetched,
	// so DutiesResponse can carry them inline and the validator can skip the separate
	// dutyDependentRootProvider round-trip on the refresh path.
	prevDependentRoot, err := hexutil.Decode(attesterDutiesResp.DependentRoot)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to decode attester duty dependent root")
	}
	currDependentRoot, err := hexutil.Decode(proposerDutiesResp.DependentRoot)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to decode proposer duty dependent root")
	}

	// Mapping from a validator index to its attesting committee's index and slot
	attesterDutiesMapping := make(map[primitives.ValidatorIndex]committeeIndexSlotPair)
	for _, attesterDuty := range attesterDuties {
		validatorIndex, err := strconv.ParseUint(attesterDuty.ValidatorIndex, 10, 64)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "failed to parse attester validator index `%s`", attesterDuty.ValidatorIndex)
		}

		slot, err := strconv.ParseUint(attesterDuty.Slot, 10, 64)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "failed to parse attester slot `%s`", attesterDuty.Slot)
		}

		committeeIndex, err := strconv.ParseUint(attesterDuty.CommitteeIndex, 10, 64)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "failed to parse attester committee index `%s`", attesterDuty.CommitteeIndex)
		}

		attesterDutiesMapping[primitives.ValidatorIndex(validatorIndex)] = committeeIndexSlotPair{
			slot:           primitives.Slot(slot),
			committeeIndex: primitives.CommitteeIndex(committeeIndex),
		}
	}

	// Mapping from a validator index to its proposal slot
	proposerDutySlots := make(map[primitives.ValidatorIndex][]primitives.Slot)
	for _, proposerDuty := range proposerDuties {
		validatorIndex, err := strconv.ParseUint(proposerDuty.ValidatorIndex, 10, 64)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "failed to parse proposer validator index `%s`", proposerDuty.ValidatorIndex)
		}

		slot, err := strconv.ParseUint(proposerDuty.Slot, 10, 64)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "failed to parse proposer slot `%s`", proposerDuty.Slot)
		}

		proposerDutySlots[primitives.ValidatorIndex(validatorIndex)] = append(proposerDutySlots[primitives.ValidatorIndex(validatorIndex)], primitives.Slot(slot))
	}

	// Set containing all validator indices that are part of a sync committee for this epoch
	syncDutiesMapping := make(map[primitives.ValidatorIndex]bool)
	for _, syncDuty := range syncDuties {
		validatorIndex, err := strconv.ParseUint(syncDuty.ValidatorIndex, 10, 64)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "failed to parse sync validator index `%s`", syncDuty.ValidatorIndex)
		}

		syncDutiesMapping[primitives.ValidatorIndex(validatorIndex)] = true
	}

	// Mapping from the {committeeIndex, slot} to each of the committee's validator indices
	committeeMapping := make(map[committeeIndexSlotPair][]primitives.ValidatorIndex)
	for _, committee := range committees {
		committeeIndex, err := strconv.ParseUint(committee.Index, 10, 64)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "failed to parse committee index `%s`", committee.Index)
		}

		slot, err := strconv.ParseUint(committee.Slot, 10, 64)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "failed to parse slot `%s`", committee.Slot)
		}

		validatorIndices := make([]primitives.ValidatorIndex, len(committee.Validators))
		for index, validatorIndexString := range committee.Validators {
			validatorIndex, err := strconv.ParseUint(validatorIndexString, 10, 64)
			if err != nil {
				return nil, nil, nil, errors.Wrapf(err, "failed to parse committee validator index `%s`", validatorIndexString)
			}
			validatorIndices[index] = primitives.ValidatorIndex(validatorIndex)
		}

		key := committeeIndexSlotPair{
			committeeIndex: primitives.CommitteeIndex(committeeIndex),
			slot:           primitives.Slot(slot),
		}
		committeeMapping[key] = validatorIndices
	}

	duties := make([]*qrysmpb.DutiesResponse_Duty, len(multipleValidatorStatus.Statuses))
	for index, validatorStatus := range multipleValidatorStatus.Statuses {
		validatorIndex := multipleValidatorStatus.Indices[index]
		pubkey := multipleValidatorStatus.PublicKeys[index]

		var attesterSlot primitives.Slot
		var committeeIndex primitives.CommitteeIndex
		var committeeValidatorIndices []primitives.ValidatorIndex

		if committeeMappingKey, ok := attesterDutiesMapping[validatorIndex]; ok {
			committeeIndex = committeeMappingKey.committeeIndex
			attesterSlot = committeeMappingKey.slot

			if committeeValidatorIndices, ok = committeeMapping[committeeMappingKey]; !ok {
				return nil, nil, nil, errors.Errorf("failed to find validators for committee index `%d` and slot `%d`", committeeIndex, attesterSlot)
			}
		}

		duties[index] = &qrysmpb.DutiesResponse_Duty{
			Committee:       committeeValidatorIndices,
			CommitteeIndex:  committeeIndex,
			AttesterSlot:    attesterSlot,
			ProposerSlots:   proposerDutySlots[validatorIndex],
			PublicKey:       pubkey,
			Status:          validatorStatus.Status,
			ValidatorIndex:  validatorIndex,
			IsSyncCommittee: syncDutiesMapping[validatorIndex],
		}
	}

	return duties, prevDependentRoot, currDependentRoot, nil
}

// GetCommittees retrieves the committees for the given epoch
func (c beaconApiDutiesProvider) GetCommittees(ctx context.Context, epoch primitives.Epoch) ([]*shared.Committee, error) {
	committeeParams := url.Values{}
	committeeParams.Add("epoch", strconv.FormatUint(uint64(epoch), 10))
	committeesRequest := buildURL("/qrl/v1/beacon/states/head/committees", committeeParams)

	var stateCommittees beacon.GetCommitteesResponse
	if _, err := c.jsonRestHandler.GetRestJsonResponse(ctx, committeesRequest, &stateCommittees); err != nil {
		return nil, errors.Wrapf(err, "failed to query committees for epoch `%d`", epoch)
	}

	if stateCommittees.Data == nil {
		return nil, errors.New("state committees data is nil")
	}

	for index, committee := range stateCommittees.Data {
		if committee == nil {
			return nil, errors.Errorf("committee at index `%d` is nil", index)
		}
	}

	return stateCommittees.Data, nil
}

// GetAttesterDuties retrieves the attester duties for the given epoch and validatorIndices.
// Returns the full response so callers can also access the dependent root.
func (c beaconApiDutiesProvider) GetAttesterDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) (*validator.GetAttesterDutiesResponse, error) {
	jsonValidatorIndices := make([]string, len(validatorIndices))
	for index, validatorIndex := range validatorIndices {
		jsonValidatorIndices[index] = strconv.FormatUint(uint64(validatorIndex), 10)
	}

	validatorIndicesBytes, err := json.Marshal(jsonValidatorIndices)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal validator indices")
	}

	attesterDuties := &validator.GetAttesterDutiesResponse{}
	if _, err := c.jsonRestHandler.PostRestJson(ctx, fmt.Sprintf("/qrl/v1/validator/duties/attester/%d", epoch), nil, bytes.NewBuffer(validatorIndicesBytes), attesterDuties); err != nil {
		return nil, errors.Wrap(err, "failed to send POST data to REST endpoint")
	}

	for index, attesterDuty := range attesterDuties.Data {
		if attesterDuty == nil {
			return nil, errors.Errorf("attester duty at index `%d` is nil", index)
		}
	}

	return attesterDuties, nil
}

// GetProposerDuties retrieves the proposer duties for the given epoch.
// Returns the full response so callers can also access the dependent root.
func (c beaconApiDutiesProvider) GetProposerDuties(ctx context.Context, epoch primitives.Epoch) (*validator.GetProposerDutiesResponse, error) {
	proposerDuties := &validator.GetProposerDutiesResponse{}
	if _, err := c.jsonRestHandler.GetRestJsonResponse(ctx, fmt.Sprintf("/qrl/v1/validator/duties/proposer/%d", epoch), proposerDuties); err != nil {
		return nil, errors.Wrapf(err, "failed to query proposer duties for epoch `%d`", epoch)
	}

	if proposerDuties.Data == nil {
		return nil, errors.New("proposer duties data is nil")
	}

	for index, proposerDuty := range proposerDuties.Data {
		if proposerDuty == nil {
			return nil, errors.Errorf("proposer duty at index `%d` is nil", index)
		}
	}

	return proposerDuties, nil
}

// GetSyncDuties retrieves the sync committee duties for the given epoch and validatorIndices
func (c beaconApiDutiesProvider) GetSyncDuties(ctx context.Context, epoch primitives.Epoch, validatorIndices []primitives.ValidatorIndex) ([]*validator.SyncCommitteeDuty, error) {
	jsonValidatorIndices := make([]string, len(validatorIndices))
	for index, validatorIndex := range validatorIndices {
		jsonValidatorIndices[index] = strconv.FormatUint(uint64(validatorIndex), 10)
	}

	validatorIndicesBytes, err := json.Marshal(jsonValidatorIndices)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal validator indices")
	}

	syncDuties := validator.GetSyncCommitteeDutiesResponse{}
	if _, err := c.jsonRestHandler.PostRestJson(ctx, fmt.Sprintf("/qrl/v1/validator/duties/sync/%d", epoch), nil, bytes.NewBuffer(validatorIndicesBytes), &syncDuties); err != nil {
		return nil, errors.Wrap(err, "failed to send POST data to REST endpoint")
	}

	if syncDuties.Data == nil {
		return nil, errors.New("sync duties data is nil")
	}

	for index, syncDuty := range syncDuties.Data {
		if syncDuty == nil {
			return nil, errors.Errorf("sync duty at index `%d` is nil", index)
		}
	}

	return syncDuties.Data, nil
}
