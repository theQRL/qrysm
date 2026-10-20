package attestations

import (
	"io"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/config/params"
	aggtesting "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation/aggregation/testing"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)
	m.Run()
}

func TestAggregate(t *testing.T) {
	// Each test defines the aggregation bitfield inputs and the wanted output result.
	bitlistLen := params.BeaconConfig().MaxValidatorsPerCommittee
	tests := []struct {
		name   string
		inputs []bitfield.Bitlist
		want   []bitfield.Bitlist
		err    error
	}{
		{
			name:   "empty list",
			inputs: []bitfield.Bitlist{},
			want:   []bitfield.Bitlist{},
		},
		{
			name: "single attestation",
			inputs: []bitfield.Bitlist{
				{0b00000010, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000010, 0b1},
			},
		},
		{
			name: "two attestations with no overlap",
			inputs: []bitfield.Bitlist{
				{0b00000001, 0b1},
				{0b00000010, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b1},
			},
		},
		{
			// Every input is bitlistLen wide and the helper sets bit i%bitlistLen,
			// so the aggregate is a single full bitlist of that width.
			name:   "256 attestations with single bit set",
			inputs: aggtesting.BitlistsWithSingleBitSet(256, bitlistLen),
			want: []bitfield.Bitlist{
				aggtesting.BitlistWithAllBitsSet(bitlistLen),
			},
		},
		{
			name:   "1024 attestations with single bit set",
			inputs: aggtesting.BitlistsWithSingleBitSet(1024, bitlistLen),
			want: []bitfield.Bitlist{
				aggtesting.BitlistWithAllBitsSet(bitlistLen),
			},
		},
		{
			name: "two attestations with overlap",
			inputs: []bitfield.Bitlist{
				{0b00000101, 0b1},
				{0b00000110, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000101, 0b1},
				{0b00000110, 0b1},
			},
		},
		{
			name: "some attestations overlap",
			inputs: []bitfield.Bitlist{
				{0b00001001, 0b1},
				{0b00010110, 0b1},
				{0b00001010, 0b1},
				{0b00110001, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00111011, 0b1},
				{0b00011111, 0b1},
			},
		},
		{
			name: "some attestations produce duplicates which are removed",
			inputs: []bitfield.Bitlist{
				{0b00000101, 0b1},
				{0b00000110, 0b1},
				{0b00001010, 0b1},
				{0b00001001, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00001111, 0b1}, // both 0&1 and 2&3 produce this bitlist
			},
		},
		{
			name: "two attestations where one is fully contained within the other",
			inputs: []bitfield.Bitlist{
				{0b00000001, 0b1},
				{0b00000011, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b1},
			},
		},
		{
			name: "two attestations where one is fully contained within the other reversed",
			inputs: []bitfield.Bitlist{
				{0b00000011, 0b1},
				{0b00000001, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b1},
			},
		},
		{
			name: "attestations with different bitlist lengths",
			inputs: []bitfield.Bitlist{
				{0b00000011, 0b10},
				{0b00000111, 0b100},
				{0b00000100, 0b1},
			},
			want: []bitfield.Bitlist{
				{0b00000011, 0b10},
				{0b00000111, 0b100},
				{0b00000100, 0b1},
			},
			err: bitfield.ErrBitlistDifferentLength,
		},
	}
	for _, tt := range tests {
		runner := func() {
			got, err := Aggregate(aggtesting.MakeAttestationsFromBitlists(tt.inputs))
			if tt.err != nil {
				require.ErrorContains(t, tt.err.Error(), err)
				return
			}
			require.NoError(t, err)
			sort.Slice(got, func(i, j int) bool {
				return got[i].AggregationBits.Bytes()[0] < got[j].AggregationBits.Bytes()[0]
			})
			sort.Slice(tt.want, func(i, j int) bool {
				return tt.want[i].Bytes()[0] < tt.want[j].Bytes()[0]
			})
			assert.Equal(t, len(tt.want), len(got))
			for i, w := range tt.want {
				assert.DeepEqual(t, w.Bytes(), got[i].AggregationBits.Bytes())
			}
		}
		t.Run(tt.name, func(t *testing.T) {
			runner()
		})
	}

	t.Run("broken attestation bitset", func(t *testing.T) {
		wantErr := "bitlist cannot be nil or empty: invalid max_cover problem"
		_, err := Aggregate(aggtesting.MakeAttestationsFromBitlists([]bitfield.Bitlist{
			{0b00000011, 0b0},
			{0b00000111, 0b100},
			{0b00000100, 0b1},
		}))
		assert.ErrorContains(t, wantErr, err)
	})

	t.Run("candidate swapping when aggregating", func(t *testing.T) {
		// The first item cannot be aggregated, and should be pushed down the list,
		// by two swaps with aggregated items (aggregation is done in-place, so the very same
		// underlying array is used for storing both aggregated and non-aggregated items).
		got, err := Aggregate(aggtesting.MakeAttestationsFromBitlists([]bitfield.Bitlist{
			{0b10000000, 0b1},
			{0b11000101, 0b1},
			{0b00011000, 0b1},
			{0b01010100, 0b1},
			{0b10001010, 0b1},
		}))
		want := []bitfield.Bitlist{
			{0b11011101, 0b1},
			{0b11011110, 0b1},
			{0b10000000, 0b1},
		}
		assert.NoError(t, err)
		assert.Equal(t, len(want), len(got))
		for i, w := range want {
			assert.DeepEqual(t, w.Bytes(), got[i].AggregationBits.Bytes())
		}
	})
}
