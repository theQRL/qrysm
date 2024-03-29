package state_native

import (
	"github.com/theQRL/qrysm/v4/beacon-chain/state/state-native/types"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
)

// SetCurrentSyncCommittee for the beacon state.
func (b *BeaconState) SetCurrentSyncCommittee(val *zondpb.SyncCommittee) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.version == version.Phase0 {
		return errNotSupported("SetCurrentSyncCommittee", b.version)
	}

	b.currentSyncCommittee = val
	b.markFieldAsDirty(types.CurrentSyncCommittee)
	return nil
}

// SetNextSyncCommittee for the beacon state.
func (b *BeaconState) SetNextSyncCommittee(val *zondpb.SyncCommittee) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.version == version.Phase0 {
		return errNotSupported("SetNextSyncCommittee", b.version)
	}

	b.nextSyncCommittee = val
	b.markFieldAsDirty(types.NextSyncCommittee)
	return nil
}
