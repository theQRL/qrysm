// Package helpers contains helper functions outlined in the Ethereum Beacon Chain spec, such as
// computing committees, randao, rewards/penalties, and more.
package helpers

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/container/slice"
	"github.com/theQRL/qrysm/crypto/hash"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/math"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
)

var (
	committeeCache       = cache.NewCommitteesCache()
	proposerIndicesCache = cache.NewProposerIndicesCache()
)

// SlotCommitteeCount returns the number of beacon committees of a slot. The
// active validator count is provided as an argument rather than an imported implementation
// from the spec definition. Having the active validator count as an argument allows for
// cheaper computation, instead of retrieving head state, one can retrieve the validator
// count.
//
// Spec pseudocode definition:
//
//	def get_committee_count_per_slot(state: BeaconState, epoch: Epoch) -> uint64:
//	 """
//	 Return the number of committees in each slot for the given ``epoch``.
//	 """
//	 return max(uint64(1), min(
//	     MAX_COMMITTEES_PER_SLOT,
//	     uint64(len(get_active_validator_indices(state, epoch))) // SLOTS_PER_EPOCH // TARGET_COMMITTEE_SIZE,
//	 ))
func SlotCommitteeCount(activeValidatorCount uint64) uint64 {
	var committeesPerSlot = activeValidatorCount / uint64(params.BeaconConfig().SlotsPerEpoch) / params.BeaconConfig().TargetCommitteeSize

	if committeesPerSlot > params.BeaconConfig().MaxCommitteesPerSlot {
		return params.BeaconConfig().MaxCommitteesPerSlot
	}
	if committeesPerSlot == 0 {
		return 1
	}

	return committeesPerSlot
}

// BeaconCommitteeFromState returns the crosslink committee of a given slot and committee index. This
// is a spec implementation where state is used as an argument. In case of state retrieval
// becomes expensive, consider using BeaconCommittee below.
//
// Spec pseudocode definition:
//
//	def get_beacon_committee(state: BeaconState, slot: Slot, index: CommitteeIndex) -> Sequence[ValidatorIndex]:
//	 """
//	 Return the beacon committee at ``slot`` for ``index``.
//	 """
//	 epoch = compute_epoch_at_slot(slot)
//	 committees_per_slot = get_committee_count_per_slot(state, epoch)
//	 return compute_committee(
//	     indices=get_active_validator_indices(state, epoch),
//	     seed=get_seed(state, epoch, DOMAIN_BEACON_ATTESTER),
//	     index=(slot % SLOTS_PER_EPOCH) * committees_per_slot + index,
//	     count=committees_per_slot * SLOTS_PER_EPOCH,
//	 )
func BeaconCommitteeFromState(ctx context.Context, state state.ReadOnlyBeaconState, slot primitives.Slot, committeeIndex primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error) {
	epoch := slots.ToEpoch(slot)
	seed, err := Seed(state, epoch, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		return nil, errors.Wrap(err, "could not get seed")
	}

	committee, err := committeeCache.Committee(ctx, slot, seed, committeeIndex)
	if err != nil {
		return nil, errors.Wrap(err, "could not interface with committee cache")
	}
	if committee != nil {
		return committee, nil
	}

	activeIndices, err := ActiveValidatorIndices(ctx, state, epoch)
	if err != nil {
		return nil, errors.Wrap(err, "could not get active indices")
	}

	return BeaconCommittee(ctx, activeIndices, seed, slot, committeeIndex)
}

// BeaconCommittee returns the beacon committee of a given slot and committee index. The
// validator indices and seed are provided as an argument rather than an imported implementation
// from the spec definition. Having them as an argument allows for cheaper computation run time.
//
// Spec pseudocode definition:
//
//	def get_beacon_committee(state: BeaconState, slot: Slot, index: CommitteeIndex) -> Sequence[ValidatorIndex]:
//	 """
//	 Return the beacon committee at ``slot`` for ``index``.
//	 """
//	 epoch = compute_epoch_at_slot(slot)
//	 committees_per_slot = get_committee_count_per_slot(state, epoch)
//	 return compute_committee(
//	     indices=get_active_validator_indices(state, epoch),
//	     seed=get_seed(state, epoch, DOMAIN_BEACON_ATTESTER),
//	     index=(slot % SLOTS_PER_EPOCH) * committees_per_slot + index,
//	     count=committees_per_slot * SLOTS_PER_EPOCH,
//	 )
func BeaconCommittee(
	ctx context.Context,
	validatorIndices []primitives.ValidatorIndex,
	seed [32]byte,
	slot primitives.Slot,
	committeeIndex primitives.CommitteeIndex,
) ([]primitives.ValidatorIndex, error) {
	committee, err := committeeCache.Committee(ctx, slot, seed, committeeIndex)
	if err != nil {
		return nil, errors.Wrap(err, "could not interface with committee cache")
	}
	if committee != nil {
		return committee, nil
	}

	committeesPerSlot := SlotCommitteeCount(uint64(len(validatorIndices)))

	indexOffset, err := math.Add64(uint64(committeeIndex), uint64(slot.ModSlot(params.BeaconConfig().SlotsPerEpoch).Mul(committeesPerSlot)))
	if err != nil {
		return nil, errors.Wrap(err, "could not add calculate index offset")
	}
	count := committeesPerSlot * uint64(params.BeaconConfig().SlotsPerEpoch)

	return computeCommittee(validatorIndices, seed, indexOffset, count)
}

// CommitteeAssignmentContainer represents a committee list, committee index, and to be attested slot for a given epoch.
type CommitteeAssignmentContainer struct {
	Committee      []primitives.ValidatorIndex
	AttesterSlot   primitives.Slot
	CommitteeIndex primitives.CommitteeIndex
}

// verifyAssignmentEpoch verifies if the given epoch is valid for assignment based on the provided state.
// It checks if the epoch is not greater than the next epoch, and if the start slot of the epoch is greater
// than or equal to the minimum valid start slot calculated based on the state's current slot and historical roots.
func verifyAssignmentEpoch(epoch primitives.Epoch, state state.BeaconState) error {
	nextEpoch := time.NextEpoch(state)
	if epoch > nextEpoch {
		return fmt.Errorf("epoch %d can't be greater than next epoch %d", epoch, nextEpoch)
	}

	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return err
	}
	minValidStartSlot := primitives.Slot(0)
	if stateSlot := state.Slot(); stateSlot >= params.BeaconConfig().SlotsPerHistoricalRoot {
		minValidStartSlot = stateSlot - params.BeaconConfig().SlotsPerHistoricalRoot
	}
	if startSlot < minValidStartSlot {
		return fmt.Errorf("start slot %d is smaller than the minimum valid start slot %d", startSlot, minValidStartSlot)
	}
	return nil
}

// ProposerAssignments calculates proposer assignments for each validator during the specified epoch.
// It verifies the validity of the epoch, then iterates through each slot in the epoch to determine the
// proposer for that slot and assigns them accordingly.
func ProposerAssignments(ctx context.Context, state state.BeaconState, epoch primitives.Epoch) (map[primitives.ValidatorIndex][]primitives.Slot, error) {
	if err := verifyAssignmentEpoch(epoch, state); err != nil {
		return nil, err
	}
	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}

	proposerAssignments := make(map[primitives.ValidatorIndex][]primitives.Slot)

	for slot := startSlot; slot < startSlot+params.BeaconConfig().SlotsPerEpoch; slot++ {
		// Skip proposer assignment for genesis slot.
		if slot == 0 {
			continue
		}
		i, err := BeaconProposerIndexAtSlot(ctx, state, slot)
		if err != nil {
			return nil, errors.Wrapf(err, "could not check proposer at slot %d", slot)
		}
		proposerAssignments[i] = append(proposerAssignments[i], slot)
	}

	return proposerAssignments, nil
}

// CommitteeAssignments calculates committee assignments for the requested validators during the specified epoch.
// It retrieves the active validator count, determines the number of committees per slot, and computes assignments
// only for validators whose indices are present in the validators slice. The returned map is bounded by the
// requested validator set, avoiding the O(N_active) allocation of the previous implementation.
func CommitteeAssignments(
	ctx context.Context,
	state state.BeaconState,
	epoch primitives.Epoch,
	validators []primitives.ValidatorIndex,
) (map[primitives.ValidatorIndex]*CommitteeAssignmentContainer, error) {
	if err := verifyAssignmentEpoch(epoch, state); err != nil {
		return nil, err
	}

	activeValidatorCount, err := ActiveValidatorCount(ctx, state, epoch)
	if err != nil {
		return nil, err
	}
	numCommitteesPerSlot := SlotCommitteeCount(activeValidatorCount)

	startSlot, err := slots.EpochStart(epoch)
	if err != nil {
		return nil, err
	}

	assignments := make(map[primitives.ValidatorIndex]*CommitteeAssignmentContainer)
	requested := make(map[primitives.ValidatorIndex]struct{}, len(validators))
	for _, v := range validators {
		requested[v] = struct{}{}
	}

	for slot := startSlot; slot < startSlot+params.BeaconConfig().SlotsPerEpoch; slot++ {
		for j := uint64(0); j < numCommitteesPerSlot; j++ {
			committee, err := BeaconCommitteeFromState(ctx, state, slot, primitives.CommitteeIndex(j))
			if err != nil {
				return nil, err
			}

			for _, vIndex := range committee {
				if _, ok := requested[vIndex]; !ok {
					continue
				}
				if _, ok := assignments[vIndex]; !ok {
					assignments[vIndex] = &CommitteeAssignmentContainer{}
				}
				assignments[vIndex].Committee = committee
				assignments[vIndex].AttesterSlot = slot
				assignments[vIndex].CommitteeIndex = primitives.CommitteeIndex(j)
			}
		}
	}

	return assignments, nil
}

// VerifyBitfieldLength verifies that a bitfield length matches the given committee size.
func VerifyBitfieldLength(bf bitfield.Bitfield, committeeSize uint64) error {
	if bf.Len() != committeeSize {
		return fmt.Errorf(
			"wanted participants bitfield length %d, got: %d",
			committeeSize,
			bf.Len())
	}
	return nil
}

// VerifyAttestationBitfieldLengths verifies that an attestations aggregation bitfields is
// a valid length matching the size of the committee.
func VerifyAttestationBitfieldLengths(ctx context.Context, state state.ReadOnlyBeaconState, att *qrysmpb.Attestation) error {
	committee, err := BeaconCommitteeFromState(ctx, state, att.Data.Slot, att.Data.CommitteeIndex)
	if err != nil {
		return errors.Wrap(err, "could not retrieve beacon committees")
	}

	if committee == nil {
		return errors.New("no committee exist for this attestation")
	}

	if err := VerifyBitfieldLength(att.AggregationBits, uint64(len(committee))); err != nil {
		return errors.Wrap(err, "failed to verify aggregation bitfield")
	}
	return nil
}

// ShuffledIndices uses input beacon state and returns the shuffled indices of the input epoch,
// the shuffled indices then can be used to break up into committees.
func ShuffledIndices(s state.ReadOnlyBeaconState, epoch primitives.Epoch) ([]primitives.ValidatorIndex, error) {
	seed, err := Seed(s, epoch, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get seed for epoch %d", epoch)
	}

	indices := make([]primitives.ValidatorIndex, 0, s.NumValidators())
	if err := s.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		if IsActiveValidatorUsingTrie(val, epoch) {
			indices = append(indices, primitives.ValidatorIndex(idx))
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// UnshuffleList is used as an optimized implementation for raw speed.
	return UnshuffleList(indices, seed)
}

// UpdateCommitteeCache gets called at the beginning of every epoch to cache the committee shuffled indices
// list with committee index and epoch number. It caches the shuffled indices for the input epoch.
func UpdateCommitteeCache(ctx context.Context, state state.ReadOnlyBeaconState, e primitives.Epoch) error {
	seed, err := Seed(state, e, params.BeaconConfig().DomainBeaconAttester)
	if err != nil {
		return err
	}
	if committeeCache.HasEntry(string(seed[:])) {
		return nil
	}
	shuffledIndices, err := ShuffledIndices(state, e)
	if err != nil {
		return err
	}

	count := SlotCommitteeCount(uint64(len(shuffledIndices)))

	// Store the sorted indices as well as shuffled indices. In current spec,
	// sorted indices is required to retrieve proposer index. This is also
	// used for failing verify signature fallback.
	sortedIndices := make([]primitives.ValidatorIndex, len(shuffledIndices))
	copy(sortedIndices, shuffledIndices)
	slices.Sort(sortedIndices)
	if err := committeeCache.AddCommitteeShuffledList(ctx, &cache.Committees{
		ShuffledIndices: shuffledIndices,
		CommitteeCount:  uint64(params.BeaconConfig().SlotsPerEpoch.Mul(count)),
		Seed:            seed,
		SortedIndices:   sortedIndices,
	}); err != nil {
		return err
	}
	return nil
}

// UpdateProposerIndicesInCache updates proposer indices entry of the committee cache.
// Input state is used to retrieve active validator indices.
// Input epoch is the epoch to retrieve proposer indices for.
func UpdateProposerIndicesInCache(ctx context.Context, state state.ReadOnlyBeaconState, epoch primitives.Epoch) error {
	// The cache uses the state root at the (current epoch - 1)'s slot as key. (e.g. for epoch 2, the key is root at slot 63)
	// Which is the reason why we skip genesis epoch.
	if epoch <= params.BeaconConfig().GenesisEpoch+params.BeaconConfig().MinSeedLookahead {
		return nil
	}

	// Use state root from (current_epoch - 1))
	s, err := slots.EpochEnd(epoch - 1)
	if err != nil {
		return err
	}
	r, err := state.StateRootAtIndex(uint64(s % params.BeaconConfig().SlotsPerHistoricalRoot))
	if err != nil {
		return err
	}
	// Skip cache update if we have an invalid key
	if r == nil || bytes.Equal(r, params.BeaconConfig().ZeroHash[:]) {
		return nil
	}
	// Skip cache update if the key already exists
	exists, err := proposerIndicesCache.HasProposerIndices(bytesutil.ToBytes32(r))
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	indices, err := ActiveValidatorIndices(ctx, state, epoch)
	if err != nil {
		return err
	}
	proposerIndices, err := precomputeProposerIndices(state, indices, epoch)
	if err != nil {
		return err
	}
	return proposerIndicesCache.AddProposerIndices(&cache.ProposerIndices{
		BlockRoot:       bytesutil.ToBytes32(r),
		ProposerIndices: proposerIndices,
	})
}

// ClearCache clears the beacon committee cache and sync committee cache.
func ClearCache() {
	committeeCache.Clear()
	proposerIndicesCache.Clear()
	syncCommitteeCache.Clear()
	balanceCache.Clear()
}

// computeCommittee returns the requested shuffled committee out of the total committees using
// validator indices and seed.
//
// Spec pseudocode definition:
//
//	def compute_committee(indices: Sequence[ValidatorIndex],
//	                    seed: Bytes32,
//	                    index: uint64,
//	                    count: uint64) -> Sequence[ValidatorIndex]:
//	  """
//	  Return the committee corresponding to ``indices``, ``seed``, ``index``, and committee ``count``.
//	  """
//	  start = (len(indices) * index) // count
//	  end = (len(indices) * uint64(index + 1)) // count
//	  return [indices[compute_shuffled_index(uint64(i), uint64(len(indices)), seed)] for i in range(start, end)]
func computeCommittee(
	indices []primitives.ValidatorIndex,
	seed [32]byte,
	index, count uint64,
) ([]primitives.ValidatorIndex, error) {
	validatorCount := uint64(len(indices))
	start := slice.SplitOffset(validatorCount, count, index)
	end := slice.SplitOffset(validatorCount, count, index+1)

	if start > validatorCount || end > validatorCount {
		return nil, errors.New("index out of range")
	}

	// Save the shuffled indices in cache, this is only needed once per epoch or once per new committee index.
	shuffledIndices := make([]primitives.ValidatorIndex, len(indices))
	copy(shuffledIndices, indices)
	// UnshuffleList is used here as it is an optimized implementation created
	// for fast computation of committees.
	// Reference implementation: https://github.com/protolambda/eth2-shuffle
	shuffledList, err := UnshuffleList(shuffledIndices, seed)
	if err != nil {
		return nil, err
	}

	return shuffledList[start:end], nil
}

// This computes proposer indices of the current epoch and returns a list of proposer indices,
// the index of the list represents the slot number.
func precomputeProposerIndices(state state.ReadOnlyBeaconState, activeIndices []primitives.ValidatorIndex, e primitives.Epoch) ([]primitives.ValidatorIndex, error) {
	hashFunc := hash.CustomSHA256Hasher()
	proposerIndices := make([]primitives.ValidatorIndex, params.BeaconConfig().SlotsPerEpoch)

	seed, err := Seed(state, e, params.BeaconConfig().DomainBeaconProposer)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate seed")
	}
	slot, err := slots.EpochStart(e)
	if err != nil {
		return nil, err
	}
	for i := uint64(0); i < uint64(params.BeaconConfig().SlotsPerEpoch); i++ {
		seedWithSlot := append(seed[:], bytesutil.Bytes8(uint64(slot)+i)...)
		seedWithSlotHash := hashFunc(seedWithSlot)
		index, err := ComputeProposerIndex(state, activeIndices, seedWithSlotHash)
		if err != nil {
			return nil, err
		}
		proposerIndices[i] = index
	}

	return proposerIndices, nil
}
