package attestations

import (
	"fmt"
	"sort"

	"github.com/pkg/errors"
	"github.com/theQRL/go-bitfield"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation/aggregation"
	"golang.org/x/exp/slices"
)

// MaxCoverAttestationAggregation relies on Maximum Coverage greedy algorithm for aggregation.
// Aggregation occurs in many rounds, up until no more aggregation is possible (all attestations
// are overlapping).
// See https://hackmd.io/@farazdagi/in-place-attagg for design and rationale.
func MaxCoverAttestationAggregation(atts []*zondpb.Attestation) ([]*zondpb.Attestation, error) {
	for i, att := range atts {
		if len(att.Signatures) != len(att.AggregationBits.BitIndices()) {
			return nil, fmt.Errorf("signatures length %d is not equal to the attesting participants indices length %d for attestation with index %d", len(att.Signatures), len(att.AggregationBits.BitIndices()), i)
		}
	}
	if len(atts) < 2 {
		return atts, nil
	}

	if err := attList(atts).validate(); err != nil {
		return nil, err
	}

	// In the future this conversion will be redundant, as attestation bitlist will be of a Bitlist64
	// type, so incoming `atts` parameters can be used as candidates list directly.
	candidates := make([]*bitfield.Bitlist64, len(atts))
	for i := 0; i < len(atts); i++ {
		var err error
		candidates[i], err = atts[i].AggregationBits.ToBitlist64()
		if err != nil {
			return nil, err
		}
	}
	coveredBitsSoFar := bitfield.NewBitlist64(candidates[0].Len())

	// In order not to re-allocate anything we rely on the very same underlying array, which
	// can only shrink (while the `aggregated` slice length can increase).
	// The `aggregated` slice grows by combining individual attestations and appending to that slice.
	// Both aggregated and non-aggregated slices operate on the very same underlying array.
	aggregated := atts[:0]
	unaggregated := atts

	// Aggregation over n/2 rounds is enough to find all aggregatable items (exits earlier if there
	// are many items that can be aggregated).
	for i := 0; i < len(atts)/2; i++ {
		if len(unaggregated) < 2 {
			break
		}

		// Find maximum non-overlapping coverage for subset of still non-processed candidates.
		roundCandidates := candidates[len(aggregated) : len(aggregated)+len(unaggregated)]
		selectedKeys, coverage, err := aggregation.MaxCover(
			roundCandidates, len(roundCandidates), false /* allowOverlaps */)
		if err != nil {
			// Return aggregated attestations, and attestations that couldn't be aggregated.
			return append(aggregated, unaggregated...), err
		}

		// Exit earlier, if possible cover does not allow aggregation (less than two items).
		if selectedKeys.Count() < 2 {
			break
		}

		// Pad selected key indexes, as `roundCandidates` is a subset of `candidates`.
		keys := padSelectedKeys(selectedKeys.BitIndices(), len(aggregated))

		// Create aggregated attestation and update solution lists. Process aggregates only if they
		// feature at least one unknown bit i.e. can increase the overall coverage.
		xc, err := coveredBitsSoFar.XorCount(coverage)
		if err != nil {
			return nil, err
		}
		if xc > 0 {
			aggIdx, err := aggregateAttestations(atts, keys, coverage)
			if err != nil {
				return append(aggregated, unaggregated...), err
			}

			// Unless we are already at the right position, swap aggregation and the first non-aggregated item.
			idx0 := len(aggregated)
			if idx0 < aggIdx {
				atts[idx0], atts[aggIdx] = atts[aggIdx], atts[idx0]
				candidates[idx0], candidates[aggIdx] = candidates[aggIdx], candidates[idx0]
			}

			// Expand to the newly created aggregate.
			aggregated = atts[:idx0+1]

			// Shift the starting point of the slice to the right.
			unaggregated = unaggregated[1:]

			// Update covered bits map.
			if err := coveredBitsSoFar.NoAllocOr(coverage, coveredBitsSoFar); err != nil {
				return nil, err
			}
			keys = keys[1:]
		}

		// Remove processed attestations.
		rearrangeProcessedAttestations(atts, candidates, keys)
		unaggregated = unaggregated[:len(unaggregated)-len(keys)]
	}

	filtered, err := attList(unaggregated).filterContained()
	if err != nil {
		return nil, err
	}
	return append(aggregated, filtered...), nil
}

// NewMaxCover returns initialized Maximum Coverage problem for attestations aggregation.
func NewMaxCover(atts []*zondpb.Attestation) *aggregation.MaxCoverProblem {
	candidates := make([]*aggregation.MaxCoverCandidate, len(atts))
	for i := 0; i < len(atts); i++ {
		candidates[i] = aggregation.NewMaxCoverCandidate(i, &atts[i].AggregationBits)
	}
	return &aggregation.MaxCoverProblem{Candidates: candidates}
}

// padSelectedKeys adds additional value to every key.
func padSelectedKeys(keys []int, pad int) []int {
	for i, key := range keys {
		keys[i] = key + pad
	}
	return keys
}

// aggregateAttestations combines signatures of selected attestations into a single aggregate attestation, and
// pushes that aggregated attestation into the position of the first of selected attestations.
func aggregateAttestations(atts []*zondpb.Attestation, keys []int, coverage *bitfield.Bitlist64) (targetIdx int, err error) {
	if len(keys) < 2 || atts == nil || len(atts) < 2 {
		return targetIdx, errors.Wrap(ErrInvalidAttestationCount, "cannot aggregate")
	}
	if coverage == nil || coverage.Count() == 0 {
		return targetIdx, errors.New("invalid or empty coverage")
	}

	var data *zondpb.AttestationData
	sigs := make([][]byte, 0, len(keys))
	participants := make([]int, 0, len(keys))
	duplicates := make(map[int]struct{})

	for i, idx := range keys {
		newAtt := atts[idx]

		if i == 0 {
			for _, participant := range newAtt.AggregationBits.BitIndices() {
				duplicates[participant] = struct{}{}
			}

			participants = append(participants, newAtt.AggregationBits.BitIndices()...)
			sigs = append(sigs, newAtt.Signatures...)

			data = zondpb.CopyAttestationData(newAtt.Data)
			targetIdx = idx

			continue
		}

		newParticipants := newAtt.AggregationBits.BitIndices()
		participantsToAdd := make([]int, 0, len(newParticipants))
		sigIndex := make(map[int][]byte)
		for i, newParticipant := range newParticipants {
			_, ok := duplicates[newParticipant]
			if !ok {
				duplicates[newParticipant] = struct{}{}
				participantsToAdd = append(participantsToAdd, newParticipant)
				sigIndex[newParticipant] = newAtt.Signatures[i]
			}
		}

		if len(participantsToAdd) == 0 {
			continue
		}

		initialIdx := 0
		for i, participant := range participantsToAdd {
			insertIdx, err := attestation.SearchInsertIdxWithOffset(participants, initialIdx, participant)
			if err != nil {
				return 0, err
			}

			// no need for more index searches
			if insertIdx > (len(participants) - 1) {
				for _, missingParticipant := range participantsToAdd[i:] {
					participants = slices.Insert(participants, insertIdx, participant)
					sigs = slices.Insert(sigs, insertIdx, sigIndex[missingParticipant])
				}
				break
			}

			participants = slices.Insert(participants, insertIdx, participant)
			sigs = slices.Insert(sigs, insertIdx, sigIndex[participant])
			initialIdx = insertIdx + 1
		}
	}

	// Put aggregated attestation at a position of the first selected attestation.
	atts[targetIdx] = &zondpb.Attestation{
		// Append size byte, which will be unnecessary on switch to Bitlist64.
		AggregationBits: coverage.ToBitlist(),
		Data:            data,
		Signatures:      sigs,
	}
	return
}

// rearrangeProcessedAttestations pushes processed attestations to the end of the slice, returning
// the number of items re-arranged (so that caller can cut the slice, and allow processed items to be
// garbage collected).
func rearrangeProcessedAttestations(atts []*zondpb.Attestation, candidates []*bitfield.Bitlist64, processedKeys []int) {
	if atts == nil || candidates == nil || processedKeys == nil {
		return
	}
	// Set all selected keys to nil.
	for _, idx := range processedKeys {
		atts[idx] = nil
		candidates[idx] = nil
	}
	// Re-arrange nil items, move them to end of slice.
	sort.Ints(processedKeys)
	lastIdx := len(atts) - 1
	for _, idx0 := range processedKeys {
		// Make sure that nil items are swapped for non-nil items only.
		for lastIdx > idx0 && atts[lastIdx] == nil {
			lastIdx--
		}
		if idx0 == lastIdx {
			break
		}
		atts[idx0], atts[lastIdx] = atts[lastIdx], atts[idx0]
		candidates[idx0], candidates[lastIdx] = candidates[lastIdx], candidates[idx0]
	}
}

// merge combines two attestation lists into one.
func (al attList) merge(al1 attList) attList {
	return append(al, al1...)
}

// selectUsingKeys returns only items with specified keys.
func (al attList) selectUsingKeys(keys []int) attList {
	filtered := make([]*zondpb.Attestation, len(keys))
	for i, key := range keys {
		filtered[i] = al[key]
	}
	return filtered
}

// selectComplementUsingKeys returns only items with keys that are NOT specified.
func (al attList) selectComplementUsingKeys(keys []int) attList {
	foundInKeys := func(key int) bool {
		for i := 0; i < len(keys); i++ {
			if keys[i] == key {
				keys[i] = keys[len(keys)-1]
				keys = keys[:len(keys)-1]
				return true
			}
		}
		return false
	}
	filtered := al[:0]
	for i, att := range al {
		if !foundInKeys(i) {
			filtered = append(filtered, att)
		}
	}
	return filtered
}

// hasCoverage returns true if a given coverage is found in attestations list.
func (al attList) hasCoverage(coverage bitfield.Bitlist) (bool, error) {
	for _, att := range al {
		x, err := att.AggregationBits.Xor(coverage)
		if err != nil {
			return false, err
		}
		if x.Count() == 0 {
			return true, nil
		}
	}
	return false, nil
}

// filterContained removes attestations that are contained within other attestations.
func (al attList) filterContained() (attList, error) {
	if len(al) < 2 {
		return al, nil
	}
	sort.Slice(al, func(i, j int) bool {
		return al[i].AggregationBits.Count() > al[j].AggregationBits.Count()
	})
	filtered := al[:0]
	filtered = append(filtered, al[0])
	for i := 1; i < len(al); i++ {
		c, err := filtered[len(filtered)-1].AggregationBits.Contains(al[i].AggregationBits)
		if err != nil {
			return nil, err
		}
		if c {
			continue
		}
		filtered = append(filtered, al[i])
	}
	return filtered, nil
}

// validate checks attestation list for validity (equal bitlength, non-nil bitlist etc).
func (al attList) validate() error {
	if al == nil {
		return errors.New("nil list")
	}
	if len(al) == 0 {
		return errors.Wrap(aggregation.ErrInvalidMaxCoverProblem, "empty list")
	}
	if al[0].AggregationBits == nil || al[0].AggregationBits.Len() == 0 {
		return errors.Wrap(aggregation.ErrInvalidMaxCoverProblem, "bitlist cannot be nil or empty")
	}
	for i := 1; i < len(al); i++ {
		if al[i].AggregationBits == nil || al[i].AggregationBits.Len() == 0 {
			return errors.Wrap(aggregation.ErrInvalidMaxCoverProblem, "bitlist cannot be nil or empty")
		}
	}
	return nil
}
