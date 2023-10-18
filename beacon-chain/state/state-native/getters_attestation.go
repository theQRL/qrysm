package state_native

import (
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
)

// PreviousEpochAttestations corresponding to blocks on the beacon chain.
func (b *BeaconState) PreviousEpochAttestations() ([]*zondpb.PendingAttestation, error) {
	if b.version != version.Phase0 {
		return nil, errNotSupported("PreviousEpochAttestations", b.version)
	}

	if b.previousEpochAttestations == nil {
		return nil, nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.previousEpochAttestationsVal(), nil
}

// previousEpochAttestationsVal corresponding to blocks on the beacon chain.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) previousEpochAttestationsVal() []*zondpb.PendingAttestation {
	return zondpb.CopyPendingAttestationSlice(b.previousEpochAttestations)
}

// CurrentEpochAttestations corresponding to blocks on the beacon chain.
func (b *BeaconState) CurrentEpochAttestations() ([]*zondpb.PendingAttestation, error) {
	if b.version != version.Phase0 {
		return nil, errNotSupported("CurrentEpochAttestations", b.version)
	}

	if b.currentEpochAttestations == nil {
		return nil, nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	return b.currentEpochAttestationsVal(), nil
}

// currentEpochAttestations corresponding to blocks on the beacon chain.
// This assumes that a lock is already held on BeaconState.
func (b *BeaconState) currentEpochAttestationsVal() []*zondpb.PendingAttestation {
	return zondpb.CopyPendingAttestationSlice(b.currentEpochAttestations)
}
