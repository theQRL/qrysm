package slasherkv

import (
	"sort"

	"github.com/pkg/errors"
	slashertypes "github.com/theQRL/qrysm/beacon-chain/slasher/types"
	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// mergeAttestationRecord combines an attestation record already stored under a
// data root with an incoming attestation for the same data. The result carries
// the union of the attesting indices, sorted, with one signature per index when
// both records carry one signature per index (the layout VerifyIndexedAttestation
// requires). It reports whether the stored record has to be rewritten.
//
// If the union would exceed the committee size, which cannot happen for valid
// attestations of one committee, the incoming record replaces the stored one.
func mergeAttestationRecord(existingEnc []byte, incoming *slashertypes.IndexedAttestationWrapper) ([]byte, bool, error) {
	existing, err := decodeAttestationRecord(existingEnc)
	if err != nil {
		return nil, false, errors.Wrap(err, "could not decode the stored attestation record")
	}
	existingAtt := existing.IndexedAttestation
	incomingAtt := incoming.IndexedAttestation

	have := make(map[uint64]struct{}, len(existingAtt.AttestingIndices))
	for _, idx := range existingAtt.AttestingIndices {
		have[idx] = struct{}{}
	}
	newIndices := 0
	for _, idx := range incomingAtt.AttestingIndices {
		if _, ok := have[idx]; !ok {
			newIndices++
		}
	}
	if newIndices == 0 {
		return nil, false, nil
	}

	unionSize := len(have) + newIndices
	if uint64(unionSize) > params.BeaconConfig().MaxValidatorsPerCommittee {
		log.WithField("signingRoot", incoming.SigningRoot).WithField("indices", unionSize).
			Warn("Attestation record union exceeds the committee size; keeping the incoming record only")
		enc, err := encodeAttestationRecord(incoming)
		return enc, true, err
	}

	// Signatures are aligned with the indices only when both records are.
	aligned := len(existingAtt.Signatures) == len(existingAtt.AttestingIndices) &&
		len(incomingAtt.Signatures) == len(incomingAtt.AttestingIndices)

	type entry struct {
		index uint64
		sig   []byte
	}
	entries := make([]entry, 0, unionSize)
	for i, idx := range existingAtt.AttestingIndices {
		e := entry{index: idx}
		if aligned {
			e.sig = existingAtt.Signatures[i]
		}
		entries = append(entries, e)
	}
	for i, idx := range incomingAtt.AttestingIndices {
		if _, ok := have[idx]; ok {
			continue
		}
		have[idx] = struct{}{}
		e := entry{index: idx}
		if aligned {
			e.sig = incomingAtt.Signatures[i]
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].index < entries[j].index })

	merged := &qrysmpb.IndexedAttestation{
		AttestingIndices: make([]uint64, 0, len(entries)),
		Data:             existingAtt.Data,
	}
	if aligned {
		merged.Signatures = make([][]byte, 0, len(entries))
	} else {
		// Without a per-index layout the signatures cannot be attributed; keep
		// the incoming ones, as an overwrite would have.
		merged.Signatures = incomingAtt.Signatures
	}
	for _, e := range entries {
		merged.AttestingIndices = append(merged.AttestingIndices, e.index)
		if aligned {
			merged.Signatures = append(merged.Signatures, e.sig)
		}
	}
	enc, err := encodeAttestationRecord(&slashertypes.IndexedAttestationWrapper{
		IndexedAttestation: merged,
		SigningRoot:        incoming.SigningRoot,
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "could not encode the merged attestation record")
	}
	return enc, true, nil
}
