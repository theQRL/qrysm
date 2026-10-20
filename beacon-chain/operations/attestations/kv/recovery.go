package kv

import (
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// RecoverAttestation restores an orphaned attestation for proposal inclusion.
// Canonical pruning marked its participants as seen, so ordinary save methods
// would silently discard it. Forget only those participants, preserving history
// for other votes, and merge directly into the proposal pool. A copy in the
// separate block/forkchoice queue must not prevent recovery for inclusion.
func (c *AttCaches) RecoverAttestation(att *qrysmpb.Attestation) error {
	if err := helpers.ValidateNilAttestation(att); err != nil {
		return err
	}
	if _, err := hashFn(att); err != nil {
		return errors.Wrap(err, "could not tree hash orphaned attestation")
	}
	r, err := hashFn(att.Data)
	if err != nil {
		return err
	}
	if err := c.forgetSeenParticipants(string(r[:]), att.AggregationBits); err != nil {
		return err
	}
	if helpers.IsAggregated(att) {
		return c.saveAggregatedAttestation(att)
	}
	return c.saveUnaggregatedAttestation(att)
}

func (c *AttCaches) forgetSeenParticipants(key string, participants bitfield.Bitlist) error {
	c.seenAttLock.Lock()
	defer c.seenAttLock.Unlock()
	// Prepare both updates before changing either cache. The mutex also
	// prevents a concurrent read/modify/write from restoring the old markers.
	caches := []*cache.Cache{c.seenAtt, c.seenAggregatedAtt}
	remaining := make([][]bitfield.Bitlist, len(caches))
	keep := participants.Not()
	for i, history := range caches {
		value, ok := history.Get(key)
		if !ok {
			continue
		}
		seen, ok := value.([]bitfield.Bitlist)
		if !ok {
			return errors.New("could not convert to bitlist type")
		}
		for _, bits := range seen {
			bits, err := bits.And(keep)
			if err != nil {
				return errors.Wrap(err, "could not clear orphaned attestation participants")
			}
			if bits.Count() > 0 {
				remaining[i] = append(remaining[i], bits)
			}
		}
	}
	for i, history := range caches {
		if len(remaining[i]) == 0 {
			history.Delete(key)
		} else {
			history.Set(key, remaining[i], cache.DefaultExpiration)
		}
	}
	return nil
}
