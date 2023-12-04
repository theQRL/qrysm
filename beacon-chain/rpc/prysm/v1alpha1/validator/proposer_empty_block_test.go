package validator

import (
	"testing"

	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func Test_getEmptyBlock(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.AltairForkEpoch = 1
	config.BellatrixForkEpoch = 2
	config.CapellaForkEpoch = 3
	config.DenebForkEpoch = 4
	params.OverrideBeaconConfig(config)

	tests := []struct {
		name string
		slot primitives.Slot
		want func() interfaces.ReadOnlySignedBeaconBlock
	}{
		{
			name: "altair",
			slot: primitives.Slot(params.BeaconConfig().AltairForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockAltair{Block: &zondpb.BeaconBlockAltair{Body: &zondpb.BeaconBlockBodyAltair{}}})
				require.NoError(t, err)
				return b
			},
		},
		{
			name: "bellatrix",
			slot: primitives.Slot(params.BeaconConfig().BellatrixForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockBellatrix{Block: &zondpb.BeaconBlockBellatrix{Body: &zondpb.BeaconBlockBodyBellatrix{}}})
				require.NoError(t, err)
				return b
			},
		},
		{
			name: "capella",
			slot: primitives.Slot(params.BeaconConfig().CapellaForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockCapella{Block: &zondpb.BeaconBlockCapella{Body: &zondpb.BeaconBlockBodyCapella{}}})
				require.NoError(t, err)
				return b
			},
		},
		{
			name: "deneb",
			slot: primitives.Slot(params.BeaconConfig().DenebForkEpoch) * params.BeaconConfig().SlotsPerEpoch,
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				b, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockDeneb{Block: &zondpb.BeaconBlockDeneb{Body: &zondpb.BeaconBlockBodyDeneb{}}})
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
