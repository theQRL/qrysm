package sync_contribution

import (
	"fmt"

	v1alpha1 "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation/aggregation"
	"golang.org/x/exp/slices"
)

// naiveSyncContributionAggregation aggregates naively, without any complex algorithms or optimizations.
// Note: this is currently a naive implementation to the order of O(mn^2).
func naiveSyncContributionAggregation(contributions []*v1alpha1.SyncCommitteeContribution) ([]*v1alpha1.SyncCommitteeContribution, error) {
	for i, c := range contributions {
		if len(c.Signatures) != len(c.AggregationBits.BitIndices()) {
			return nil, fmt.Errorf("signatures length %d is not equal to the attesting participants indices length %d for contribution with index %d", len(c.Signatures), len(c.AggregationBits.BitIndices()), i)
		}
	}

	if len(contributions) <= 1 {
		return contributions, nil
	}

	// Naive aggregation. O(n^2) time.
	for i, a := range contributions {
		if i >= len(contributions) {
			break
		}
		for j := i + 1; j < len(contributions); j++ {
			b := contributions[j]
			if o, err := a.AggregationBits.Overlaps(b.AggregationBits); err != nil {
				return nil, err
			} else if !o {
				var err error
				a, err = aggregate(a, b)
				if err != nil {
					return nil, err
				}
				// Delete b
				contributions = append(contributions[:j], contributions[j+1:]...)
				j--
				contributions[i] = a
			}
		}
	}

	// Naive deduplication of identical contributions. O(n^2) time.
	for i, a := range contributions {
		for j := i + 1; j < len(contributions); j++ {
			b := contributions[j]

			if a.AggregationBits.Len() != b.AggregationBits.Len() {
				continue
			}

			if c, err := a.AggregationBits.Contains(b.AggregationBits); err != nil {
				return nil, err
			} else if c {
				// If b is fully contained in a, then b can be removed.
				contributions = append(contributions[:j], contributions[j+1:]...)
				j--
			} else if c, err := b.AggregationBits.Contains(a.AggregationBits); err != nil {
				return nil, err
			} else if c {
				// if a is fully contained in b, then a can be removed.
				contributions = append(contributions[:i], contributions[i+1:]...)
				break // Stop the inner loop, advance a.
			}
		}
	}

	return contributions, nil
}

// aggregates pair of sync contributions c1 and c2 together.
func aggregate(c1, c2 *v1alpha1.SyncCommitteeContribution) (*v1alpha1.SyncCommitteeContribution, error) {
	o, err := c1.AggregationBits.Overlaps(c2.AggregationBits)
	if err != nil {
		return nil, err
	}
	if o {
		return nil, aggregation.ErrBitsOverlap
	}

	baseContribution := v1alpha1.CopySyncCommitteeContribution(c1)
	newContribution := v1alpha1.CopySyncCommitteeContribution(c2)
	if newContribution.AggregationBits.Count() > baseContribution.AggregationBits.Count() {
		baseContribution, newContribution = newContribution, baseContribution
	}

	// update the signatures slice
	// 1. check for new participants in the new contribution with the help of an aux map
	// containing the base participants and index the new required signatures.
	// 2. search for the insert index of the participants to add(sorted) on the slice of
	// the base participants(sorted) and update the base signatures slice accordingly
	duplicates := make(map[int]struct{})
	baseParticipants := baseContribution.AggregationBits.BitIndices()
	for _, baseParticipant := range baseParticipants {
		duplicates[baseParticipant] = struct{}{}
	}

	newParticipants := newContribution.AggregationBits.BitIndices()
	participantsToAdd := make([]int, 0, len(newParticipants))
	sigIndex := make(map[int][]byte)
	for i, newParticipant := range newParticipants {
		_, ok := duplicates[newParticipant]
		if !ok {
			participantsToAdd = append(participantsToAdd, newParticipant)
			sigIndex[newParticipant] = newContribution.Signatures[i]
		}
	}

	// base attestation already contains all the participants of the new attestation
	if len(participantsToAdd) == 0 {
		return baseContribution, nil
	}

	initialIdx := 0
	for i, participant := range participantsToAdd {
		insertIdx, err := attestation.SearchInsertIdxWithOffset(baseParticipants, initialIdx, participant)
		if err != nil {
			return nil, err
		}

		// no need for more index searches - the remaining indexes to add are greater
		// than the ones in the base participation.
		if insertIdx > (len(baseParticipants) - 1) {
			for _, missingParticipant := range participantsToAdd[i:] {
				baseContribution.Signatures = slices.Insert(baseContribution.Signatures, insertIdx, sigIndex[missingParticipant])
			}
			break
		}

		baseParticipants = slices.Insert(baseParticipants, insertIdx, participant)
		baseContribution.Signatures = slices.Insert(baseContribution.Signatures, insertIdx, sigIndex[participant])
		initialIdx = insertIdx + 1
	}

	// update the participants bitfield
	participants, err := baseContribution.AggregationBits.Or(newContribution.AggregationBits)
	if err != nil {
		return nil, err
	}
	baseContribution.AggregationBits = participants

	return baseContribution, nil
}
