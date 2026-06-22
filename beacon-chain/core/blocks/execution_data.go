package blocks

import (
	"bytes"
	"context"
	"errors"

	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// ProcessExecutionDataInBlock is an operation performed on each
// beacon block to ensure the execution data votes are processed
// into the beacon state.
//
// Official spec definition:
//
//	def process_execution_data(state: BeaconState, body: BeaconBlockBody) -> None:
//	 state.execution_data_votes.append(body.execution_data)
//	 if state.execution_data_votes.count(body.execution_data) * 2 > EPOCHS_PER_EXECUTION_VOTING_PERIOD * SLOTS_PER_EPOCH:
//	     state.execution_data = body.execution_data
func ProcessExecutionDataInBlock(_ context.Context, beaconState state.BeaconState, executionData *qrysmpb.ExecutionData) (state.BeaconState, error) {
	if beaconState == nil || beaconState.IsNil() {
		return nil, errors.New("nil state")
	}
	if err := beaconState.AppendExecutionDataVotes(executionData); err != nil {
		return nil, err
	}
	hasSupport, err := ExecutionDataHasEnoughSupport(beaconState, executionData)
	if err != nil {
		return nil, err
	}
	if hasSupport {
		if err := beaconState.SetExecutionData(executionData); err != nil {
			return nil, err
		}
	}
	return beaconState, nil
}

// AreExecutionDataEqual checks equality between two execution data objects.
func AreExecutionDataEqual(a, b *qrysmpb.ExecutionData) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.DepositCount == b.DepositCount &&
		bytes.Equal(a.BlockHash, b.BlockHash) &&
		bytes.Equal(a.DepositRoot, b.DepositRoot)
}

// ExecutionDataHasEnoughSupport returns true when the given executiondata has more than 50% votes in the
// execution voting period. A vote is cast by including executiondata in a block and part of state processing
// appends executiondata to the state in the ExecutionDataVotes list. Iterating through this list checks the
// votes to see if they match the executiondata.
func ExecutionDataHasEnoughSupport(beaconState state.ReadOnlyBeaconState, data *qrysmpb.ExecutionData) (bool, error) {
	voteCount := uint64(0)
	data = qrysmpb.CopyExecutionData(data)

	for _, vote := range beaconState.ExecutionDataVotes() {
		if AreExecutionDataEqual(vote, data) {
			voteCount++
		}
	}

	// If 50+% majority converged on the same executiondata, then it has enough support to update the
	// state.
	support := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerExecutionVotingPeriod))
	return voteCount*2 > uint64(support), nil
}
