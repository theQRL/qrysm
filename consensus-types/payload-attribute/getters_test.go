package payloadattribute

import (
	"testing"

	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestPayloadAttributeGetters(t *testing.T) {
	tests := []struct {
		name string
		tc   func(t *testing.T)
	}{
		{
			name: "Get version",
			tc: func(t *testing.T) {
				a := EmptyWithVersion(version.Capella)
				require.Equal(t, version.Capella, a.Version())
			},
		},
		{
			name: "Get prev randao (capella)",
			tc: func(t *testing.T) {
				r := []byte{1, 2, 3}
				a, err := New(&enginev1.PayloadAttributesV2{PrevRandao: r})
				require.NoError(t, err)
				require.DeepEqual(t, r, a.PrevRandao())
			},
		},
		{
			name: "Get suggested fee recipient (capella)",
			tc: func(t *testing.T) {
				r := []byte{4, 5, 6}
				a, err := New(&enginev1.PayloadAttributesV2{SuggestedFeeRecipient: r})
				require.NoError(t, err)
				require.DeepEqual(t, r, a.SuggestedFeeRecipient())
			},
		},
		{
			name: "Get timestamp (capella)",
			tc: func(t *testing.T) {
				r := uint64(123)
				a, err := New(&enginev1.PayloadAttributesV2{Timestamp: r})
				require.NoError(t, err)
				require.Equal(t, r, a.Timestamps())
			},
		},
		{
			name: "Get withdrawals (capella)",
			tc: func(t *testing.T) {
				wd := []*enginev1.Withdrawal{{Index: 1}, {Index: 2}, {Index: 3}}
				a, err := New(&enginev1.PayloadAttributesV2{Withdrawals: wd})
				require.NoError(t, err)
				got, err := a.Withdrawals()
				require.NoError(t, err)
				require.DeepEqual(t, wd, got)
			},
		},
		{
			name: "Get PbCapella (nil)",
			tc: func(t *testing.T) {
				a, err := New(&enginev1.PayloadAttributesV2{})
				require.NoError(t, err)
				got, err := a.PbV2()
				require.NoError(t, err)
				require.Equal(t, (*enginev1.PayloadAttributesV2)(nil), got)
			},
		},
		{
			name: "Get PbCapella",
			tc: func(t *testing.T) {
				p := &enginev1.PayloadAttributesV2{
					Timestamp:             1,
					PrevRandao:            []byte{1, 2, 3},
					SuggestedFeeRecipient: []byte{4, 5, 6},
					Withdrawals:           []*enginev1.Withdrawal{{Index: 1}, {Index: 2}, {Index: 3}},
				}
				a, err := New(p)
				require.NoError(t, err)
				got, err := a.PbV2()
				require.NoError(t, err)
				require.DeepEqual(t, p, got)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, test.tc)
	}
}
