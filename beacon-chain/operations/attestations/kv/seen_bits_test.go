package kv

import (
	"testing"
	"time"

	"github.com/theQRL/go-bitfield"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestAttCaches_hasSeenBit(t *testing.T) {
	c := NewAttCaches()

	seenA1 := util.HydrateAttestation(&zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000011}})
	seenA2 := util.HydrateAttestation(&zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b11100000}})
	require.NoError(t, c.insertSeenBit(seenA1))
	require.NoError(t, c.insertSeenBit(seenA2))
	tests := []struct {
		att  *zondpb.Attestation
		want bool
	}{
		{att: util.HydrateAttestation(&zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000000}}), want: true},
		{att: util.HydrateAttestation(&zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000001}}), want: true},
		{att: util.HydrateAttestation(&zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b11100000}}), want: true},
		{att: util.HydrateAttestation(&zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000011}}), want: true},
		{att: util.HydrateAttestation(&zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b10001000}}), want: false},
		{att: util.HydrateAttestation(&zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b11110111}}), want: false},
	}
	for _, tt := range tests {
		got, err := c.hasSeenBit(tt.att)
		require.NoError(t, err)
		if got != tt.want {
			t.Errorf("hasSeenBit() got = %v, want %v", got, tt.want)
		}
	}
}

func TestAttCaches_insertSeenBitDuplicates(t *testing.T) {
	c := NewAttCaches()
	att1 := util.HydrateAttestation(&zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b10000011}})
	r, err := hashFn(att1.Data)
	require.NoError(t, err)
	require.NoError(t, c.insertSeenBit(att1))
	require.Equal(t, 1, c.seenAtt.ItemCount())

	_, expirationTime1, ok := c.seenAtt.GetWithExpiration(string(r[:]))
	require.Equal(t, true, ok)

	// NOTE(rgeraldes24): required to create a time gap otherwise the last check fails sometimes
	time.Sleep(2 * time.Second)

	// Make sure that duplicates are not inserted, but expiration time gets updated.
	require.NoError(t, c.insertSeenBit(att1))
	require.Equal(t, 1, c.seenAtt.ItemCount())
	_, expirationqrysmTime, ok := c.seenAtt.GetWithExpiration(string(r[:]))
	require.Equal(t, true, ok)
	require.Equal(t, true, expirationqrysmTime.After(expirationTime1), "Expiration time is not updated")
}
