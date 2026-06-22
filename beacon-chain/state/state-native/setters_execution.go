package state_native

import (
	"github.com/theQRL/qrysm/beacon-chain/state/state-native/types"
	"github.com/theQRL/qrysm/beacon-chain/state/stateutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// SetExecutionData for the beacon state.
func (b *BeaconState) SetExecutionData(val *qrysmpb.ExecutionData) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.executionData = val
	b.markFieldAsDirty(types.ExecutionData)
	return nil
}

// SetExecutionDataVotes for the beacon state. Updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetExecutionDataVotes(val []*qrysmpb.ExecutionData) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.ExecutionDataVotes].MinusRef()
	b.sharedFieldReferences[types.ExecutionDataVotes] = stateutil.NewRef(1)

	b.executionDataVotes = val
	b.markFieldAsDirty(types.ExecutionDataVotes)
	b.rebuildTrie[types.ExecutionDataVotes] = true
	return nil
}

// SetExecutionDepositIndex for the beacon state.
func (b *BeaconState) SetExecutionDepositIndex(val uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.executionDepositIndex = val
	b.markFieldAsDirty(types.ExecutionDepositIndex)
	return nil
}

// AppendExecutionDataVotes for the beacon state. Appends the new value
// to the end of list.
func (b *BeaconState) AppendExecutionDataVotes(val *qrysmpb.ExecutionData) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	votes := b.executionDataVotes
	if b.sharedFieldReferences[types.ExecutionDataVotes].Refs() > 1 {
		// Copy elements in underlying array by reference.
		votes = make([]*qrysmpb.ExecutionData, 0, len(b.executionDataVotes)+1)
		votes = append(votes, b.executionDataVotes...)
		b.sharedFieldReferences[types.ExecutionDataVotes].MinusRef()
		b.sharedFieldReferences[types.ExecutionDataVotes] = stateutil.NewRef(1)
	}

	b.executionDataVotes = append(votes, val)
	b.markFieldAsDirty(types.ExecutionDataVotes)
	b.addDirtyIndices(types.ExecutionDataVotes, []uint64{uint64(len(b.executionDataVotes) - 1)})
	return nil
}
