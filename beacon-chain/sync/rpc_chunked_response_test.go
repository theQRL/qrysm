package sync

import (
	"reflect"
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/blockchain"
	mock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
)

func TestExtractBlockDataType(t *testing.T) {
	// Precompute digests
	genDigest, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)
	altairDigest, err := signing.ComputeForkDigest(params.BeaconConfig().AltairForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)
	bellatrixDigest, err := signing.ComputeForkDigest(params.BeaconConfig().BellatrixForkVersion, params.BeaconConfig().ZeroHash[:])
	require.NoError(t, err)

	type args struct {
		digest []byte
		chain  blockchain.ChainInfoFetcher
	}
	tests := []struct {
		name    string
		args    args
		want    interfaces.ReadOnlySignedBeaconBlock
		wantErr bool
	}{
		{
			name: "no digest",
			args: args{
				digest: []byte{},
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},

			want: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlock{Block: &zondpb.BeaconBlock{Body: &zondpb.BeaconBlockBody{}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantErr: false,
		},
		{
			name: "invalid digest",
			args: args{
				digest: []byte{0x00, 0x01},
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "non existent digest",
			args: args{
				digest: []byte{0x00, 0x01, 0x02, 0x03},
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "genesis fork version",
			args: args{
				digest: genDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlock{Block: &zondpb.BeaconBlock{Body: &zondpb.BeaconBlockBody{}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantErr: false,
		},
		{
			name: "altair fork version",
			args: args{
				digest: altairDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockAltair{Block: &zondpb.BeaconBlockAltair{Body: &zondpb.BeaconBlockBodyAltair{}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantErr: false,
		},
		{
			name: "bellatrix fork version",
			args: args{
				digest: bellatrixDigest[:],
				chain:  &mock.ChainService{ValidatorsRoot: [32]byte{}},
			},
			want: func() interfaces.ReadOnlySignedBeaconBlock {
				wsb, err := blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockBellatrix{Block: &zondpb.BeaconBlockBellatrix{Body: &zondpb.BeaconBlockBodyBellatrix{}}})
				require.NoError(t, err)
				return wsb
			}(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractBlockDataType(tt.args.digest, tt.args.chain)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractBlockDataType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractBlockDataType() got = %v, want %v", got, tt.want)
			}
		})
	}
}
