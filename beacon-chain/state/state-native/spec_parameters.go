package state_native

import (
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/runtime/version"
)

func (b *BeaconState) ProportionalSlashingMultiplier() (uint64, error) {
	switch b.version {
	case version.Zond:
		return params.BeaconConfig().ProportionalSlashingMultiplier, nil
	}
	return 0, errNotSupported("ProportionalSlashingMultiplier()", b.version)
}

func (b *BeaconState) InactivityPenaltyQuotient() (uint64, error) {
	switch b.version {
	case version.Zond:
		return params.BeaconConfig().InactivityPenaltyQuotient, nil
	}
	return 0, errNotSupported("InactivityPenaltyQuotient()", b.version)
}
