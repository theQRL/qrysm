package state_native

import (
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// CurrentSyncCommittee of the current sync committee in beacon chain state.
func (b *BeaconState) CurrentSyncCommittee() (*qrysmpb.SyncCommittee, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.currentSyncCommittee == nil {
		return nil, nil
	}

	return b.currentSyncCommitteeVal(), nil
}

// currentSyncCommitteeVal of the current sync committee in beacon chain state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) currentSyncCommitteeVal() *qrysmpb.SyncCommittee {
	return copySyncCommittee(b.currentSyncCommittee)
}

// NextSyncCommittee of the next sync committee in beacon chain state.
func (b *BeaconState) NextSyncCommittee() (*qrysmpb.SyncCommittee, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.nextSyncCommittee == nil {
		return nil, nil
	}

	return b.nextSyncCommitteeVal(), nil
}

// nextSyncCommitteeVal of the next sync committee in beacon chain state.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) nextSyncCommitteeVal() *qrysmpb.SyncCommittee {
	return copySyncCommittee(b.nextSyncCommittee)
}

// copySyncCommittee copies the provided sync committee object.
func copySyncCommittee(data *qrysmpb.SyncCommittee) *qrysmpb.SyncCommittee {
	if data == nil {
		return nil
	}
	return &qrysmpb.SyncCommittee{
		Pubkeys: bytesutil.SafeCopy2dBytes(data.Pubkeys),
	}
}
