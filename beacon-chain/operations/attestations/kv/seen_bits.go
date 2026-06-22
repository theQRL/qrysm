package kv

import (
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/theQRL/go-bitfield"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func (c *AttCaches) insertSeenBit(att *qrysmpb.Attestation) error {
	return insertSeenBit(c.seenAtt, att)
}

func (c *AttCaches) insertSeenAggregatedBit(att *qrysmpb.Attestation) error {
	return insertSeenBit(c.seenAggregatedAtt, att)
}

func insertSeenBit(seenCache *cache.Cache, att *qrysmpb.Attestation) error {
	r, err := hashFn(att.Data)
	if err != nil {
		return err
	}

	v, ok := seenCache.Get(string(r[:]))
	if ok {
		seenBits, ok := v.([]bitfield.Bitlist)
		if !ok {
			return errors.New("could not convert to bitlist type")
		}
		alreadyExists := false
		for _, bit := range seenBits {
			if c, err := bit.Contains(att.AggregationBits); err != nil {
				return errors.Wrap(err, "could not check attestation bits containment")
			} else if c {
				alreadyExists = true
				break
			}
		}
		if !alreadyExists {
			seenBits = append(seenBits, att.AggregationBits)
		}
		seenCache.Set(string(r[:]), seenBits, cache.DefaultExpiration /* two epochs (EIP-7045 window) */)
		return nil
	}

	seenCache.Set(string(r[:]), []bitfield.Bitlist{att.AggregationBits}, cache.DefaultExpiration /* two epochs (EIP-7045 window) */)
	return nil
}

func (c *AttCaches) hasSeenBit(att *qrysmpb.Attestation) (bool, error) {
	return hasSeenBit(c.seenAtt, att)
}

func (c *AttCaches) hasSeenAggregatedBit(att *qrysmpb.Attestation) (bool, error) {
	return hasSeenBit(c.seenAggregatedAtt, att)
}

func hasSeenBit(seenCache *cache.Cache, att *qrysmpb.Attestation) (bool, error) {
	r, err := hashFn(att.Data)
	if err != nil {
		return false, err
	}

	v, ok := seenCache.Get(string(r[:]))
	if ok {
		seenBits, ok := v.([]bitfield.Bitlist)
		if !ok {
			return false, errors.New("could not convert to bitlist type")
		}
		for _, bit := range seenBits {
			if c, err := bit.Contains(att.AggregationBits); err != nil {
				return false, errors.Wrap(err, "could not check attestation bits containment")
			} else if c {
				return true, nil
			}
		}
	}
	return false, nil
}
