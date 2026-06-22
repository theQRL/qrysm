package state_native

import (
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// ExecutionData corresponding to the execution chain information stored in the beacon state.
func (b *BeaconState) ExecutionData() *qrysmpb.ExecutionData {
	if b.executionData == nil {
		return nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.executionDataVal()
}

// executionDataVal corresponding to the execution chain information stored in the beacon state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) executionDataVal() *qrysmpb.ExecutionData {
	if b.executionData == nil {
		return nil
	}

	return qrysmpb.CopyExecutionData(b.executionData)
}

// ExecutionDataVotes corresponds to votes from Ethereum on the canonical execution chain
// data retrieved from execution.
func (b *BeaconState) ExecutionDataVotes() []*qrysmpb.ExecutionData {
	if b.executionDataVotes == nil {
		return nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.executionDataVotesVal()
}

// executionDataVotesVal corresponds to votes from QRL on the canonical execution chain
// data retrieved from execution.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) executionDataVotesVal() []*qrysmpb.ExecutionData {
	if b.executionDataVotes == nil {
		return nil
	}

	res := make([]*qrysmpb.ExecutionData, len(b.executionDataVotes))
	for i := range res {
		res[i] = qrysmpb.CopyExecutionData(b.executionDataVotes[i])
	}
	return res
}

// ExecutionDepositIndex corresponds to the index of the deposit made to the
// validator deposit contract at the time of this state's execution data.
func (b *BeaconState) ExecutionDepositIndex() uint64 {
	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.executionDepositIndex
}
