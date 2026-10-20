package params

import (
	"fmt"
	"math/bits"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
)

// MaxActiveValidators returns the largest cap for which every active validator
// count up to that cap fits the committee layout and attestation SSZ bounds.
//
// Committee selection uses max(1, min(MaxCommitteesPerSlot,
// activeValidators / SlotsPerEpoch / TargetCommitteeSize)) committees per slot.
// The cap must also cover smaller counts reached after exits, even when a larger
// validator set would be individually representable.
func (b *BeaconChainConfig) MaxActiveValidators() (uint64, error) {
	if b.MaxCommitteesPerSlot == 0 {
		return 0, fmt.Errorf("MAX_COMMITTEES_PER_SLOT must be non-zero")
	}
	if err := b.validateCommitteeSize(); err != nil {
		return 0, err
	}
	if b.SlotsPerEpoch == 0 {
		return 0, fmt.Errorf("SLOTS_PER_EPOCH must be non-zero")
	}
	if b.TargetCommitteeSize == 0 {
		return 0, fmt.Errorf("TARGET_COMMITTEE_SIZE must be non-zero")
	}

	hi, committeesPerEpoch := bits.Mul64(b.MaxCommitteesPerSlot, uint64(b.SlotsPerEpoch))
	if hi != 0 {
		return 0, fmt.Errorf("MAX_COMMITTEES_PER_SLOT * SLOTS_PER_EPOCH overflows uint64")
	}
	hi, maxActiveValidators := bits.Mul64(committeesPerEpoch, b.MaxValidatorsPerCommittee)
	if hi != 0 {
		return 0, fmt.Errorf("MAX_COMMITTEES_PER_SLOT * SLOTS_PER_EPOCH * MAX_VALIDATORS_PER_COMMITTEE overflows uint64")
	}
	if b.MaxCommitteesPerSlot > 1 {
		// If the second committee is not selected before the first would exceed
		// its bound, the safe range ends at the single-committee capacity. If it
		// is selected in time, later transitions are also safe: the peak number
		// of members per committee decreases as the committee count increases.
		// The checked full capacity above, with MaxCommitteesPerSlot > 1,
		// guarantees that both the product and the increment below fit uint64.
		singleCommitteeCapacity := uint64(b.SlotsPerEpoch) * b.MaxValidatorsPerCommittee
		if (singleCommitteeCapacity+1)/uint64(b.SlotsPerEpoch)/b.TargetCommitteeSize < 2 {
			return singleCommitteeCapacity, nil
		}
	}
	return maxActiveValidators, nil
}

func (b *BeaconChainConfig) validateCommitteeSize() error {
	if b.MaxValidatorsPerCommittee == 0 {
		return fmt.Errorf("MAX_VALIDATORS_PER_COMMITTEE must be non-zero")
	}
	if b.MaxValidatorsPerCommittee > fieldparams.MaxValidatorsPerCommittee {
		return fmt.Errorf("MAX_VALIDATORS_PER_COMMITTEE (%d) must not exceed this binary's SSZ attestation limit (%d)",
			b.MaxValidatorsPerCommittee, fieldparams.MaxValidatorsPerCommittee)
	}
	return nil
}
