package state_native

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/state/state-native/types"
	"github.com/theQRL/qrysm/beacon-chain/state/stateutil"
	"github.com/theQRL/qrysm/config/features"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	consensus_types "github.com/theQRL/qrysm/consensus-types"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// SetValidators for the beacon state. Updates the entire
// to a new value by overwriting the previous one.
// normalizeRandaoCommitment gives a validator without a RANDAO commitment the
// zero commitment, so that the field always has its fixed SSZ size. A zero
// commitment has no known pre-image: such a validator never proposes a valid
// block, which matches how process_deposit treats an empty commitment.
func normalizeRandaoCommitment(v *qrysmpb.Validator) {
	if v != nil && len(v.RandaoCommitment) == 0 {
		v.RandaoCommitment = make([]byte, fieldparams.RandaoCommitmentLength)
	}
}

func (b *BeaconState) SetValidators(val []*qrysmpb.Validator) error {
	for _, v := range val {
		normalizeRandaoCommitment(v)
	}
	b.lock.Lock()
	defer b.lock.Unlock()

	if features.Get().EnableExperimentalState {
		if b.validatorsMultiValue != nil {
			b.validatorsMultiValue.Detach(b)
		}
		b.validatorsMultiValue = NewMultiValueValidators(val)
	} else {
		b.validators = val
		b.sharedFieldReferences[types.Validators].MinusRef()
		b.sharedFieldReferences[types.Validators] = stateutil.NewRef(1)
	}

	b.markFieldAsDirty(types.Validators)
	b.rebuildTrie[types.Validators] = true
	if b.valMapHandler != nil {
		b.valMapHandler.MinusRef()
	}
	b.valMapHandler = stateutil.NewValMapHandler(val)
	return nil
}

// ApplyToEveryValidator applies the provided callback function to each validator in the
// validator registry. The callback receives a copy and may call other state methods.
// Successful updates remain applied if a later callback fails.
func (b *BeaconState) ApplyToEveryValidator(f func(idx int, val *qrysmpb.Validator) (bool, *qrysmpb.Validator, error)) error {
	for i := range b.NumValidators() {
		val, err := b.ValidatorAtIndex(primitives.ValidatorIndex(i))
		if err != nil {
			return err
		}
		changed, newVal, err := f(i, val)
		if err != nil {
			return err
		}
		if changed {
			// Publish the value and its dirty index together, respecting any
			// state copies made while the callback was running.
			if err := b.UpdateValidatorAtIndex(primitives.ValidatorIndex(i), newVal); err != nil {
				return errors.Wrapf(err, "could not update validator at index %d", i)
			}
		}
	}

	return nil
}

// UpdateValidatorAtIndex for the beacon state. Updates the validator
// at a specific index to a new value.
func (b *BeaconState) UpdateValidatorAtIndex(idx primitives.ValidatorIndex, val *qrysmpb.Validator) error {
	normalizeRandaoCommitment(val)
	b.lock.Lock()
	defer b.lock.Unlock()

	if features.Get().EnableExperimentalState {
		if err := b.validatorsMultiValue.UpdateAt(b, uint64(idx), val); err != nil {
			return errors.Wrap(err, "could not update validator")
		}
	} else {
		if uint64(len(b.validators)) <= uint64(idx) {
			return errors.Wrapf(consensus_types.ErrOutOfBounds, "validator index %d does not exist", idx)
		}

		v := b.validators
		if ref := b.sharedFieldReferences[types.Validators]; ref.Refs() > 1 {
			v = b.validatorsReferences()
			ref.MinusRef()
			b.sharedFieldReferences[types.Validators] = stateutil.NewRef(1)
		}
		v[idx] = val
		b.validators = v
	}

	b.markFieldAsDirty(types.Validators)
	b.addDirtyIndices(types.Validators, []uint64{uint64(idx)})
	return nil
}

// SetBalances for the beacon state. Updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetBalances(val []uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if features.Get().EnableExperimentalState {
		if b.balancesMultiValue != nil {
			b.balancesMultiValue.Detach(b)
		}
		b.balancesMultiValue = NewMultiValueBalances(val)
	} else {
		b.sharedFieldReferences[types.Balances].MinusRef()
		b.sharedFieldReferences[types.Balances] = stateutil.NewRef(1)
		b.balances = val
	}

	b.markFieldAsDirty(types.Balances)
	b.rebuildTrie[types.Balances] = true
	return nil
}

// UpdateBalancesAtIndex for the beacon state. This method updates the balance
// at a specific index to a new value.
func (b *BeaconState) UpdateBalancesAtIndex(idx primitives.ValidatorIndex, val uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if features.Get().EnableExperimentalState {
		if err := b.balancesMultiValue.UpdateAt(b, uint64(idx), val); err != nil {
			return errors.Wrap(err, "could not update balances")
		}
	} else {
		if uint64(len(b.balances)) <= uint64(idx) {
			return errors.Wrapf(consensus_types.ErrOutOfBounds, "balance index %d does not exist", idx)
		}

		bals := b.balances
		if b.sharedFieldReferences[types.Balances].Refs() > 1 {
			bals = b.balancesVal()
			b.sharedFieldReferences[types.Balances].MinusRef()
			b.sharedFieldReferences[types.Balances] = stateutil.NewRef(1)
		}
		bals[idx] = val
		b.balances = bals
	}

	b.markFieldAsDirty(types.Balances)
	b.addDirtyIndices(types.Balances, []uint64{uint64(idx)})
	return nil
}

// SetSlashings for the beacon state. Updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetSlashings(val []uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.sharedFieldReferences[types.Slashings].MinusRef()
	b.sharedFieldReferences[types.Slashings] = stateutil.NewRef(1)

	b.slashings = val
	b.markFieldAsDirty(types.Slashings)
	return nil
}

// UpdateSlashingsAtIndex for the beacon state. Updates the slashings
// at a specific index to a new value.
func (b *BeaconState) UpdateSlashingsAtIndex(idx, val uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if uint64(len(b.slashings)) <= idx {
		return errors.Errorf("invalid index provided %d", idx)
	}

	s := b.slashings
	if b.sharedFieldReferences[types.Slashings].Refs() > 1 {
		s = b.slashingsVal()
		b.sharedFieldReferences[types.Slashings].MinusRef()
		b.sharedFieldReferences[types.Slashings] = stateutil.NewRef(1)
	}

	s[idx] = val

	b.slashings = s

	b.markFieldAsDirty(types.Slashings)
	return nil
}

// AppendValidator for the beacon state. Appends the new value
// to the end of list.
func (b *BeaconState) AppendValidator(val *qrysmpb.Validator) error {
	if val == nil {
		return errors.New("cannot append nil validator")
	}
	normalizeRandaoCommitment(val)
	b.lock.Lock()
	defer b.lock.Unlock()

	var valIdx primitives.ValidatorIndex
	if features.Get().EnableExperimentalState {
		b.validatorsMultiValue.Append(b, val)
		valIdx = primitives.ValidatorIndex(b.validatorsMultiValue.Len(b) - 1)
	} else {
		vals := b.validators
		if b.sharedFieldReferences[types.Validators].Refs() > 1 {
			vals = b.validatorsReferences()
			b.sharedFieldReferences[types.Validators].MinusRef()
			b.sharedFieldReferences[types.Validators] = stateutil.NewRef(1)
		}

		b.validators = append(vals, val)
		valIdx = primitives.ValidatorIndex(len(b.validators) - 1)
	}

	if b.valMapHandler.Refs() > 1 {
		m := b.valMapHandler.Copy()
		b.valMapHandler.MinusRef()
		b.valMapHandler = m
	}
	b.valMapHandler.Set(bytesutil.ToBytes2592(val.PublicKey), valIdx)
	b.markFieldAsDirty(types.Validators)
	b.addDirtyIndices(types.Validators, []uint64{uint64(valIdx)})
	return nil
}

// AppendBalance for the beacon state. Appends the new value
// to the end of list.
func (b *BeaconState) AppendBalance(bal uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	var balIdx uint64
	if features.Get().EnableExperimentalState {
		b.balancesMultiValue.Append(b, bal)
		balIdx = uint64(b.balancesMultiValue.Len(b) - 1)
	} else {
		bals := b.balances
		if b.sharedFieldReferences[types.Balances].Refs() > 1 {
			bals = make([]uint64, 0, len(b.balances)+int(params.BeaconConfig().MaxDeposits))
			bals = append(bals, b.balances...)
			b.sharedFieldReferences[types.Balances].MinusRef()
			b.sharedFieldReferences[types.Balances] = stateutil.NewRef(1)
		}

		b.balances = append(bals, bal)
		balIdx = uint64(len(b.balances) - 1)
	}

	b.markFieldAsDirty(types.Balances)
	b.addDirtyIndices(types.Balances, []uint64{balIdx})
	return nil
}

// AppendInactivityScore for the beacon state.
func (b *BeaconState) AppendInactivityScore(s uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if features.Get().EnableExperimentalState {
		b.inactivityScoresMultiValue.Append(b, s)
	} else {
		scores := b.inactivityScores
		if b.sharedFieldReferences[types.InactivityScores].Refs() > 1 {
			scores = make([]uint64, 0, len(b.inactivityScores)+int(params.BeaconConfig().MaxDeposits))
			scores = append(scores, b.inactivityScores...)
			b.sharedFieldReferences[types.InactivityScores].MinusRef()
			b.sharedFieldReferences[types.InactivityScores] = stateutil.NewRef(1)
		}
		b.inactivityScores = append(scores, s)
	}

	b.markFieldAsDirty(types.InactivityScores)
	return nil
}

// SetInactivityScores for the beacon state. Updates the entire
// list to a new value by overwriting the previous one.
func (b *BeaconState) SetInactivityScores(val []uint64) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if features.Get().EnableExperimentalState {
		if b.inactivityScoresMultiValue != nil {
			b.inactivityScoresMultiValue.Detach(b)
		}
		b.inactivityScoresMultiValue = NewMultiValueInactivityScores(val)
	} else {
		b.sharedFieldReferences[types.InactivityScores].MinusRef()
		b.sharedFieldReferences[types.InactivityScores] = stateutil.NewRef(1)
		b.inactivityScores = val
	}

	b.markFieldAsDirty(types.InactivityScores)
	return nil
}
