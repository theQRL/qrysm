package kv

import (
	"slices"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

// SaveBlockAttestation saves an block attestation in cache.
func (c *AttCaches) SaveBlockAttestation(att *qrysmpb.Attestation) error {
	if att == nil || att.Data == nil {
		return nil
	}
	r, err := hashFn(att.Data)
	if err != nil {
		return errors.Wrap(err, "could not tree hash attestation")
	}

	c.blockAttLock.Lock()
	defer c.blockAttLock.Unlock()
	atts, ok := c.blockAtt[r]
	if !ok {
		atts = make([]*qrysmpb.Attestation, 0, 1)
	}

	// Pending votes have not been authenticated against their target state
	// yet. Blocks on branches with different committees can carry the same
	// data and participant bits with different signatures, of which only one
	// set is valid there, and a superset can fail where a subset succeeds.
	// Only an identical entry is a duplicate.
	for _, a := range atts {
		if proto.Equal(a, att) {
			return nil
		}
	}

	c.blockAtt[r] = append(atts, qrysmpb.CopyAttestation(att))

	return nil
}

// BlockAttestations returns the block attestations in cache.
func (c *AttCaches) BlockAttestations() []*qrysmpb.Attestation {
	atts := make([]*qrysmpb.Attestation, 0)

	c.blockAttLock.RLock()
	defer c.blockAttLock.RUnlock()
	for _, att := range c.blockAtt {
		atts = append(atts, att...)
	}

	return atts
}

// DeleteBlockAttestation removes an applied pending vote and marks its
// participants seen for gossip, so the same vote is not processed again.
// Proposal deduplication is left to canonical pruning: the containing block
// may not be canonical, and a recovered copy must stay proposable. Other
// votes with the same data keep waiting for their own retry.
func (c *AttCaches) DeleteBlockAttestation(att *qrysmpb.Attestation) error {
	return c.removeBlockAttestation(att, true)
}

// DiscardBlockAttestation removes a pending vote that was never applied,
// such as one whose signatures failed against the voted target state, without
// marking its participants seen. Another block or a gossip aggregate may still
// carry the genuine signatures for the same data and bits.
func (c *AttCaches) DiscardBlockAttestation(att *qrysmpb.Attestation) error {
	return c.removeBlockAttestation(att, false)
}

// MarkAppliedAttestation records that fork choice authenticated an included
// vote in its target state and applied it. Its participants count as seen for
// gossip, so the same vote is not processed again. A block only proves its
// votes valid in its own state, so nothing else may mark them, and as with
// DeleteBlockAttestation proposal deduplication is left to canonical pruning.
func (c *AttCaches) MarkAppliedAttestation(att *qrysmpb.Attestation) error {
	if err := helpers.ValidateNilAttestation(att); err != nil {
		return err
	}
	return c.insertSeenAggregatedBit(att)
}

func (c *AttCaches) removeBlockAttestation(att *qrysmpb.Attestation, markSeen bool) error {
	if att == nil || att.Data == nil {
		return nil
	}
	r, err := hashFn(att.Data)
	if err != nil {
		return errors.Wrap(err, "could not tree hash attestation")
	}

	c.blockAttLock.Lock()
	defer c.blockAttLock.Unlock()
	atts := c.blockAtt[r]
	for i, existingAtt := range atts {
		if !proto.Equal(existingAtt, att) {
			continue
		}
		if markSeen {
			if err := c.insertSeenAggregatedBit(existingAtt); err != nil {
				return err
			}
		}
		atts = slices.Delete(atts, i, i+1)
		if len(atts) == 0 {
			delete(c.blockAtt, r)
		} else {
			c.blockAtt[r] = atts
		}
		break
	}

	return nil
}
