package validator

import (
	"testing"

	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func Test_getEmptyBlock(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	tests := []struct {
		name string
		slot primitives.Slot
		want func() interfaces.ReadOnlySignedBeaconBlock
	}{
		{
			name: "capella",
			slot: primitives.Slot(0),
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockCapella{Block: &zondpb.BeaconBlockCapella{Body: &zondpb.BeaconBlockBodyCapella{}}})
				require.NoError(t, err)
				return b
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getEmptyBlock(tt.slot)
			require.NoError(t, err)
			require.DeepEqual(t, tt.want(), got, "getEmptyBlock() = %v, want %v", got, tt.want())
		})
	}
}
