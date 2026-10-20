package kv

import (
	"sort"
	"testing"

	"github.com/theQRL/go-bitfield"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestKV_BlockAttestation_CanSaveRetrieve(t *testing.T) {
	cache := NewAttCaches()

	att1 := util.HydrateAttestation(&qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1101}})
	att2 := util.HydrateAttestation(&qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1101}})
	att3 := util.HydrateAttestation(&qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Slot: 3}, AggregationBits: bitfield.Bitlist{0b1101}})
	atts := []*qrysmpb.Attestation{att1, att2, att3}

	for _, att := range atts {
		require.NoError(t, cache.SaveBlockAttestation(att))
	}
	// A different bit length is a distinct pending vote, not a comparison error.
	att4 := util.HydrateAttestation(&qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Slot: 4}, AggregationBits: bitfield.Bitlist{0b11011}})
	require.NoError(t, cache.SaveBlockAttestation(att4))
	atts = append(atts, att4)

	returned := cache.BlockAttestations()

	sort.Slice(returned, func(i, j int) bool {
		return returned[i].Data.Slot < returned[j].Data.Slot
	})

	assert.DeepEqual(t, atts, returned)
}

func TestKV_BlockAttestation_CanDelete(t *testing.T) {
	cache := NewAttCaches()

	att1 := util.HydrateAttestation(&qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1101}})
	att2 := util.HydrateAttestation(&qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1101}})
	att3 := util.HydrateAttestation(&qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Slot: 3}, AggregationBits: bitfield.Bitlist{0b1101}})
	atts := []*qrysmpb.Attestation{att1, att2, att3}

	for _, att := range atts {
		require.NoError(t, cache.SaveBlockAttestation(att))
	}

	require.NoError(t, cache.DeleteBlockAttestation(att1))
	require.NoError(t, cache.DeleteBlockAttestation(att3))

	returned := cache.BlockAttestations()
	wanted := []*qrysmpb.Attestation{att2}
	assert.DeepEqual(t, wanted, returned)
}

func TestKV_BlockAttestation_DeletePreservesPendingParticipants(t *testing.T) {
	cache := NewAttCaches()
	processed := recoveryAttestation(0b10001)
	pending := recoveryAttestation(0b10010)
	require.NoError(t, cache.SaveBlockAttestation(processed))
	require.NoError(t, cache.SaveBlockAttestation(pending))
	for range 2 {
		require.NoError(t, cache.DeleteBlockAttestation(processed))
		require.DeepSSZEqual(t, []*qrysmpb.Attestation{pending}, cache.BlockAttestations(), "a later retry must still see unprocessed participants with the same data")
		seen, err := cache.hasSeenAggregatedBit(pending)
		require.NoError(t, err)
		require.Equal(t, false, seen, "deletion must only mark the processed participants seen")
	}
	require.NoError(t, cache.DeleteBlockAttestation(pending))
	require.Equal(t, 0, len(cache.BlockAttestations()))
}

func TestKV_BlockAttestation_KeepsUnverifiedVariants(t *testing.T) {
	cache := NewAttCaches()
	full := recoveryAttestation(0b10011)
	// Same data and bits with different signatures: blocks on branches with
	// different committees can both include this, and only the retry against
	// the voted target state tells which set is genuine.
	variant := qrysmpb.CopyAttestation(full)
	variant.Signatures[0][0] ^= 0xff
	subset := recoveryAttestation(0b10001)

	require.NoError(t, cache.SaveBlockAttestation(full))
	require.NoError(t, cache.SaveBlockAttestation(full), "an identical entry is a duplicate")
	require.NoError(t, cache.SaveBlockAttestation(variant))
	require.NoError(t, cache.SaveBlockAttestation(subset), "a superset may fail where its subset succeeds")
	require.Equal(t, 3, len(cache.BlockAttestations()))
}

func TestKV_BlockAttestation_DiscardDoesNotMarkSeen(t *testing.T) {
	cache := NewAttCaches()
	rejected := recoveryAttestation(0b10011)
	applied := recoveryAttestation(0b10100)
	require.NoError(t, cache.SaveBlockAttestation(rejected))
	require.NoError(t, cache.SaveBlockAttestation(applied))

	require.NoError(t, cache.DiscardBlockAttestation(rejected))
	require.DeepSSZEqual(t, []*qrysmpb.Attestation{applied}, cache.BlockAttestations())
	seen, err := cache.hasSeenAggregatedBit(rejected)
	require.NoError(t, err)
	require.Equal(t, false, seen, "a vote that never applied must not shadow genuine signatures for its bits")

	require.NoError(t, cache.DeleteBlockAttestation(applied))
	require.Equal(t, 0, len(cache.BlockAttestations()))
	seen, err = cache.hasSeenAggregatedBit(applied)
	require.NoError(t, err)
	require.Equal(t, true, seen)
	seen, err = cache.hasSeenBit(applied)
	require.NoError(t, err)
	require.Equal(t, false, seen, "proposal deduplication is left to canonical pruning")
}
