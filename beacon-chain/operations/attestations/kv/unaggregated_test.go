package kv

import (
	"bytes"
	"context"
	"sort"
	"testing"

	c "github.com/patrickmn/go-cache"
	fssz "github.com/prysmaticlabs/fastssz"
	"github.com/theQRL/go-bitfield"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestKV_Unaggregated_SaveUnaggregatedAttestation(t *testing.T) {
	tests := []struct {
		name          string
		att           *zondpb.Attestation
		count         int
		wantErrString string
	}{
		{
			name: "nil attestation",
			att:  nil,
		},
		{
			name:          "already aggregated",
			att:           &zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b10101}, Data: &zondpb.AttestationData{Slot: 2}},
			wantErrString: "attestation is aggregated",
		},
		{
			name: "invalid hash",
			att: &zondpb.Attestation{
				Data: &zondpb.AttestationData{
					BeaconBlockRoot: []byte{0b0},
				},
			},
			wantErrString: fssz.ErrBytesLength.Error(),
		},
		{
			name:  "normal save",
			att:   util.HydrateAttestation(&zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b0001}}),
			count: 1,
		},
		{
			name: "already seen",
			att: util.HydrateAttestation(&zondpb.Attestation{
				Data: &zondpb.AttestationData{
					Slot: 100,
				},
				AggregationBits: bitfield.Bitlist{0b10000001},
			}),
			count: 0,
		},
	}
	r, err := hashFn(util.HydrateAttestationData(&zondpb.AttestationData{Slot: 100}))
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewAttCaches()
			cache.seenAtt.Set(string(r[:]), []bitfield.Bitlist{{0xff}}, c.DefaultExpiration)
			assert.Equal(t, 0, len(cache.unAggregatedAtt), "Invalid start pool, atts: %d", len(cache.unAggregatedAtt))

			if tt.att != nil && tt.att.Signatures == nil {
				tt.att.Signatures = [][]byte{}
			}

			err := cache.SaveUnaggregatedAttestation(tt.att)
			if tt.wantErrString != "" {
				assert.ErrorContains(t, tt.wantErrString, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.count, len(cache.unAggregatedAtt), "Wrong attestation count")
			assert.Equal(t, tt.count, cache.UnaggregatedAttestationCount(), "Wrong attestation count")
		})
	}
}

func TestKV_Unaggregated_SaveUnaggregatedAttestations(t *testing.T) {
	tests := []struct {
		name          string
		atts          []*zondpb.Attestation
		count         int
		wantErrString string
	}{
		{
			name: "unaggregated only",
			atts: []*zondpb.Attestation{
				util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1}}),
				util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 2}}),
				util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 3}}),
			},
			count: 3,
		},
		{
			name: "has aggregated",
			atts: []*zondpb.Attestation{
				util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1}}),
				{AggregationBits: bitfield.Bitlist{0b1111}, Data: &zondpb.AttestationData{Slot: 2}},
				util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 3}}),
			},
			wantErrString: "attestation is aggregated",
			count:         1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewAttCaches()
			assert.Equal(t, 0, len(cache.unAggregatedAtt), "Invalid start pool, atts: %d", len(cache.unAggregatedAtt))

			err := cache.SaveUnaggregatedAttestations(tt.atts)
			if tt.wantErrString != "" {
				assert.ErrorContains(t, tt.wantErrString, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.count, len(cache.unAggregatedAtt), "Wrong attestation count")
			assert.Equal(t, tt.count, cache.UnaggregatedAttestationCount(), "Wrong attestation count")
		})
	}
}

func TestKV_Unaggregated_DeleteUnaggregatedAttestation(t *testing.T) {
	t.Run("nil attestation", func(t *testing.T) {
		cache := NewAttCaches()
		assert.NoError(t, cache.DeleteUnaggregatedAttestation(nil))
	})

	t.Run("aggregated attestation", func(t *testing.T) {
		cache := NewAttCaches()
		att := &zondpb.Attestation{AggregationBits: bitfield.Bitlist{0b1111}, Data: &zondpb.AttestationData{Slot: 2}}
		err := cache.DeleteUnaggregatedAttestation(att)
		assert.ErrorContains(t, "attestation is aggregated", err)
	})

	t.Run("successful deletion", func(t *testing.T) {
		cache := NewAttCaches()
		att1 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b101}})
		att2 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 2}, AggregationBits: bitfield.Bitlist{0b110}})
		att3 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 3}, AggregationBits: bitfield.Bitlist{0b110}})
		atts := []*zondpb.Attestation{att1, att2, att3}
		require.NoError(t, cache.SaveUnaggregatedAttestations(atts))
		for _, att := range atts {
			assert.NoError(t, cache.DeleteUnaggregatedAttestation(att))
		}
		returned, err := cache.UnaggregatedAttestations()
		require.NoError(t, err)
		assert.DeepEqual(t, []*zondpb.Attestation{}, returned)
	})
}

func TestKV_Unaggregated_DeleteSeenUnaggregatedAttestations(t *testing.T) {
	d := util.HydrateAttestationData(&zondpb.AttestationData{})

	t.Run("no attestations", func(t *testing.T) {
		cache := NewAttCaches()
		count, err := cache.DeleteSeenUnaggregatedAttestations()
		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("none seen", func(t *testing.T) {
		cache := NewAttCaches()
		atts := []*zondpb.Attestation{
			util.HydrateAttestation(&zondpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b1001}}),
			util.HydrateAttestation(&zondpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b1010}}),
			util.HydrateAttestation(&zondpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b1100}}),
		}
		require.NoError(t, cache.SaveUnaggregatedAttestations(atts))
		assert.Equal(t, 3, cache.UnaggregatedAttestationCount())

		// As none of attestations have been marked seen, nothing should be deleted.
		count, err := cache.DeleteSeenUnaggregatedAttestations()
		assert.NoError(t, err)
		assert.Equal(t, 0, count)
		assert.Equal(t, 3, cache.UnaggregatedAttestationCount())
	})

	t.Run("some seen", func(t *testing.T) {
		cache := NewAttCaches()
		atts := []*zondpb.Attestation{
			util.HydrateAttestation(&zondpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b1001}}),
			util.HydrateAttestation(&zondpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b1010}}),
			util.HydrateAttestation(&zondpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b1100}}),
		}
		require.NoError(t, cache.SaveUnaggregatedAttestations(atts))
		assert.Equal(t, 3, cache.UnaggregatedAttestationCount())

		require.NoError(t, cache.insertSeenBit(atts[1]))

		// Only seen attestations must be deleted.
		count, err := cache.DeleteSeenUnaggregatedAttestations()
		assert.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Equal(t, 2, cache.UnaggregatedAttestationCount())
		returned, err := cache.UnaggregatedAttestations()
		sort.Slice(returned, func(i, j int) bool {
			return bytes.Compare(returned[i].AggregationBits, returned[j].AggregationBits) < 0
		})
		require.NoError(t, err)
		assert.DeepEqual(t, []*zondpb.Attestation{atts[0], atts[2]}, returned)
	})

	t.Run("all seen", func(t *testing.T) {
		cache := NewAttCaches()
		atts := []*zondpb.Attestation{
			util.HydrateAttestation(&zondpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b1001}}),
			util.HydrateAttestation(&zondpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b1010}}),
			util.HydrateAttestation(&zondpb.Attestation{Data: d, AggregationBits: bitfield.Bitlist{0b1100}}),
		}
		require.NoError(t, cache.SaveUnaggregatedAttestations(atts))
		assert.Equal(t, 3, cache.UnaggregatedAttestationCount())

		require.NoError(t, cache.insertSeenBit(atts[0]))
		require.NoError(t, cache.insertSeenBit(atts[1]))
		require.NoError(t, cache.insertSeenBit(atts[2]))

		// All attestations have been processed -- all should be removed.
		count, err := cache.DeleteSeenUnaggregatedAttestations()
		assert.NoError(t, err)
		assert.Equal(t, 3, count)
		assert.Equal(t, 0, cache.UnaggregatedAttestationCount())
		returned, err := cache.UnaggregatedAttestations()
		require.NoError(t, err)
		assert.DeepEqual(t, []*zondpb.Attestation{}, returned)
	})
}

func TestKV_Unaggregated_UnaggregatedAttestationsBySlotIndex(t *testing.T) {
	cache := NewAttCaches()

	att1 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b101}})
	att2 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1, CommitteeIndex: 2}, AggregationBits: bitfield.Bitlist{0b110}})
	att3 := util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 2, CommitteeIndex: 1}, AggregationBits: bitfield.Bitlist{0b110}})
	atts := []*zondpb.Attestation{att1, att2, att3}

	for _, att := range atts {
		require.NoError(t, cache.SaveUnaggregatedAttestation(att))
	}
	ctx := context.Background()
	returned := cache.UnaggregatedAttestationsBySlotIndex(ctx, 1, 1)
	assert.DeepEqual(t, []*zondpb.Attestation{att1}, returned)
	returned = cache.UnaggregatedAttestationsBySlotIndex(ctx, 1, 2)
	assert.DeepEqual(t, []*zondpb.Attestation{att2}, returned)
	returned = cache.UnaggregatedAttestationsBySlotIndex(ctx, 2, 1)
	assert.DeepEqual(t, []*zondpb.Attestation{att3}, returned)
}
