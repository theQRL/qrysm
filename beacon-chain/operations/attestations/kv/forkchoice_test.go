package kv

import (
	"sort"
	"testing"

	"github.com/theQRL/go-bitfield"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestKV_Forkchoice_CanSaveRetrieve(t *testing.T) {
	cache := NewAttCaches()

	att1 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1101}})
	att2 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1101}})
	att3 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 3}, AggregationBits: bitfield.Bitlist{0b1101}})
	atts := []*zondpb.Attestation{att1, att2, att3}

	for _, att := range atts {
		require.NoError(t, cache.SaveForkchoiceAttestation(att))
	}

	returned := cache.ForkchoiceAttestations()

	sort.Slice(returned, func(i, j int) bool {
		return returned[i].Data.Slot < returned[j].Data.Slot
	})

	assert.DeepEqual(t, atts, returned)
}

func TestKV_Forkchoice_CanDelete(t *testing.T) {
	cache := NewAttCaches()

	att1 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1101}})
	att2 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1101}})
	att3 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 3}, AggregationBits: bitfield.Bitlist{0b1101}})
	atts := []*zondpb.Attestation{att1, att2, att3}

	for _, att := range atts {
		require.NoError(t, cache.SaveForkchoiceAttestation(att))
	}

	require.NoError(t, cache.DeleteForkchoiceAttestation(att1))
	require.NoError(t, cache.DeleteForkchoiceAttestation(att3))

	returned := cache.ForkchoiceAttestations()
	wanted := []*zondpb.Attestation{att2}
	assert.DeepEqual(t, wanted, returned)
}

func TestKV_Forkchoice_CanCount(t *testing.T) {
	cache := NewAttCaches()

	att1 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b1101}})
	att2 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b1101}})
	att3 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 3}, AggregationBits: bitfield.Bitlist{0b1101}})
	atts := []*zondpb.Attestation{att1, att2, att3}

	for _, att := range atts {
		require.NoError(t, cache.SaveForkchoiceAttestation(att))
	}

	require.Equal(t, 3, cache.ForkchoiceAttestationCount())
}
