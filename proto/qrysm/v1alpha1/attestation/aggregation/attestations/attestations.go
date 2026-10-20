package attestations

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// attList represents list of attestations, defined for easier en masse operations (filtering, sorting).
type attList []*qrysmpb.Attestation

var _ = logrus.WithField("prefix", "aggregation.attestations")

// ErrInvalidAttestationCount is returned when insufficient number
// of attestations is provided for aggregation.
var ErrInvalidAttestationCount = errors.New("invalid number of attestations")

// Aggregate aggregates attestations. The minimal number of attestations is returned.
// Aggregation occurs in-place i.e. contents of input array will be modified. Should you need to
// preserve input attestations, clone them before aggregating:
//
//	clonedAtts := make([]*qrysmpb.Attestation, len(atts))
//	for i, a := range atts {
//	    clonedAtts[i] = stateTrie.CopyAttestation(a)
//	}
//	aggregatedAtts, err := attaggregation.Aggregate(clonedAtts)
func Aggregate(atts []*qrysmpb.Attestation) ([]*qrysmpb.Attestation, error) {
	return MaxCoverAttestationAggregation(atts)
}

// AggregateDisjointOneBitAtts aggregates unaggregated attestations with the
// exact same attestation data.
func AggregateDisjointOneBitAtts(atts []*qrysmpb.Attestation) (*qrysmpb.Attestation, error) {
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
	for i := range atts {
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
