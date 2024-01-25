package state_native

import (
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
)

// CurrentSyncCommittee of the current sync committee in beacon chain state.
func (b *BeaconState) CurrentSyncCommittee() (*zondpb.SyncCommittee, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.version == version.Phase0 {
		return nil, errNotSupported("CurrentSyncCommittee", b.version)
	}

	if b.currentSyncCommittee == nil {
		return nil, nil
	}

	return b.currentSyncCommitteeVal(), nil
}

// currentSyncCommitteeVal of the current sync committee in beacon chain state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) currentSyncCommitteeVal() *zondpb.SyncCommittee {
	return copySyncCommittee(b.currentSyncCommittee)
}

// NextSyncCommittee of the next sync committee in beacon chain state.
func (b *BeaconState) NextSyncCommittee() (*zondpb.SyncCommittee, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.version == version.Phase0 {
		return nil, errNotSupported("NextSyncCommittee", b.version)
	}

	if b.nextSyncCommittee == nil {
		return nil, nil
	}

	return b.nextSyncCommitteeVal(), nil
}

// nextSyncCommitteeVal of the next sync committee in beacon chain state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) nextSyncCommitteeVal() *zondpb.SyncCommittee {
	return copySyncCommittee(b.nextSyncCommittee)
}

// copySyncCommittee copies the provided sync committee object.
func copySyncCommittee(data *zondpb.SyncCommittee) *zondpb.SyncCommittee {
	if data == nil {
		return nil
	}
	return &zondpb.SyncCommittee{
		Pubkeys: bytesutil.SafeCopy2dBytes(data.Pubkeys),
	}
}
