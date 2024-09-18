package attestations

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation/aggregation"
	"golang.org/x/exp/slices"
)

// attList represents list of attestations, defined for easier en masse operations (filtering, sorting).
type attList []*zondpb.Attestation

var _ = logrus.WithField("prefix", "aggregation.attestations")

// ErrInvalidAttestationCount is returned when insufficient number
// of attestations is provided for aggregation.
var ErrInvalidAttestationCount = errors.New("invalid number of attestations")

// Aggregate aggregates attestations. The minimal number of attestations is returned.
// Aggregation occurs in-place i.e. contents of input array will be modified. Should you need to
// preserve input attestations, clone them before aggregating:
//
//	clonedAtts := make([]*zondpb.Attestation, len(atts))
//	for i, a := range atts {
//	    clonedAtts[i] = stateTrie.CopyAttestation(a)
//	}
//	aggregatedAtts, err := attaggregation.Aggregate(clonedAtts)
func Aggregate(atts []*zondpb.Attestation) ([]*zondpb.Attestation, error) {
	return MaxCoverAttestationAggregation(atts)
}

// AggregateDisjointOneBitAtts aggregates unaggregated attestations with the
// exact same attestation data.
func AggregateDisjointOneBitAtts(atts []*zondpb.Attestation) (*zondpb.Attestation, error) {
	if len(atts) == 0 {
		return nil, nil
	}
	for i, att := range atts {
		if len(att.Signatures) != len(att.AggregationBits.BitIndices()) {
			return nil, fmt.Errorf("signatures length %d is not equal to the attesting participants indices length %d for attestation with index %d", len(att.Signatures), len(att.AggregationBits.BitIndices()), i)
		}
	}

	if len(atts) == 1 {
		return atts[0], nil
	}
	coverage, err := atts[0].AggregationBits.ToBitlist64()
	if err != nil {
		return nil, errors.Wrap(err, "could not get aggregation bits")
	}
	for _, att := range atts[1:] {
		bits, err := att.AggregationBits.ToBitlist64()
		if err != nil {
			return nil, errors.Wrap(err, "could not get aggregation bits")
		}
		err = coverage.NoAllocOr(bits, coverage)
		if err != nil {
			return nil, errors.Wrap(err, "could not get aggregation bits")
		}
	}
	keys := make([]int, len(atts))
	for i := 0; i < len(atts); i++ {
		keys[i] = i
	}
	idx, err := aggregateAttestations(atts, keys, coverage)
	if err != nil {
		return nil, errors.Wrap(err, "could not aggregate attestations")
	}
	if idx != 0 {
		return nil, errors.New("could not aggregate attestations, obtained non zero index")
	}
	return atts[0], nil
}

// AggregatePair aggregates pair of attestations a1 and a2 together.
func AggregatePair(a1, a2 *zondpb.Attestation) (*zondpb.Attestation, error) {
	if len(a1.AggregationBits.BitIndices()) != len(a1.Signatures) {
		return nil, fmt.Errorf("att1: signatures length %d is not equal to the attesting participants indices length %d", len(a1.Signatures), len(a1.AggregationBits.BitIndices()))
	}
	if len(a2.AggregationBits.BitIndices()) != len(a2.Signatures) {
		return nil, fmt.Errorf("att2: signatures length %d is not equal to the attesting participants indices length %d", len(a2.Signatures), len(a2.AggregationBits.BitIndices()))
	}

	o, err := a1.AggregationBits.Overlaps(a2.AggregationBits)
	if err != nil {
		return nil, err
	}
	if o {
		return nil, aggregation.ErrBitsOverlap
	}

	baseAtt := zondpb.CopyAttestation(a1)
	newAtt := zondpb.CopyAttestation(a2)
	if newAtt.AggregationBits.Count() > baseAtt.AggregationBits.Count() {
		baseAtt, newAtt = newAtt, baseAtt
	}

	// update the signatures slice
	// 1. check for new participants in the new attestation with the help of an aux map
	// containing the base participants and index the new required signatures.
	// 2. search for the insert index of the participants to add(sorted) on the slice of
	// the base participants(sorted) and update the base signatures slice accordingly
	duplicates := make(map[int]struct{})
	baseParticipants := baseAtt.AggregationBits.BitIndices()
	for _, baseParticipant := range baseParticipants {
		duplicates[baseParticipant] = struct{}{}
	}

	newParticipants := newAtt.AggregationBits.BitIndices()
	participantsToAdd := make([]int, 0, len(newParticipants))
	sigIndex := make(map[int][]byte)
	for i, newParticipant := range newParticipants {
		_, ok := duplicates[newParticipant]
		if !ok {
			participantsToAdd = append(participantsToAdd, newParticipant)
			sigIndex[newParticipant] = newAtt.Signatures[i]
		}
	}

	// base attestation already contains all the participants of the new attestation
	if len(participantsToAdd) == 0 {
		return baseAtt, nil
	}

	initialIdx := 0
	for i, participant := range participantsToAdd {
		insertIdx, err := attestation.SearchInsertIdxWithOffset(baseParticipants, initialIdx, participant)
		if err != nil {
			return nil, err
		}

		// no need for more index searches; just append the signatures of the remaining
		// participants that we need to add.
		if insertIdx > (len(baseParticipants) - 1) {
			for _, missingParticipant := range participantsToAdd[i:] {
				baseAtt.Signatures = slices.Insert(baseAtt.Signatures, insertIdx, sigIndex[missingParticipant])
			}
			break
		}

		baseParticipants = slices.Insert(baseParticipants, insertIdx, participant)
		baseAtt.Signatures = slices.Insert(baseAtt.Signatures, insertIdx, sigIndex[participant])
		initialIdx = insertIdx + 1
	}

	// update the participants bitfield
	participants, err := baseAtt.AggregationBits.Or(newAtt.AggregationBits)
	if err != nil {
		return nil, err
	}
	baseAtt.AggregationBits = participants

	return baseAtt, nil
}
