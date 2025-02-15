package blocks_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/core/blocks"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	"google.golang.org/protobuf/proto"
)

func FakeDeposits(n uint64) []*zondpb.Eth1Data {
	deposits := make([]*zondpb.Eth1Data, n)
	for i := uint64(0); i < n; i++ {
		deposits[i] = &zondpb.Eth1Data{
			DepositCount: 1,
			DepositRoot:  bytesutil.PadTo([]byte("root"), 32),
		}
	}
	return deposits
}

func TestEth1DataHasEnoughSupport(t *testing.T) {
	tests := []struct {
		stateVotes         []*zondpb.Eth1Data
		data               *zondpb.Eth1Data
		hasSupport         bool
		votingPeriodLength primitives.Epoch
	}{
		{
			stateVotes: FakeDeposits(uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))),
			data: &zondpb.Eth1Data{
				DepositCount: 1,
				DepositRoot:  bytesutil.PadTo([]byte("root"), 32),
			},
			hasSupport:         true,
			votingPeriodLength: 7,
		}, {
			stateVotes: FakeDeposits(uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))),
			data: &zondpb.Eth1Data{
				DepositCount: 1,
				DepositRoot:  bytesutil.PadTo([]byte("root"), 32),
			},
			hasSupport:         false,
			votingPeriodLength: 8,
		}, {
			stateVotes: FakeDeposits(uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))),
			data: &zondpb.Eth1Data{
				DepositCount: 1,
				DepositRoot:  bytesutil.PadTo([]byte("root"), 32),
			},
			hasSupport:         false,
			votingPeriodLength: 10,
		},
	}

	params.SetupTestConfigCleanup(t)
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			c := params.BeaconConfig()
			c.EpochsPerEth1VotingPeriod = tt.votingPeriodLength
			params.OverrideBeaconConfig(c)

			s, err := state_native.InitializeFromProtoPhase0(&zondpb.BeaconState{
				Eth1DataVotes: tt.stateVotes,
			})
			require.NoError(t, err)
			result, err := blocks.Eth1DataHasEnoughSupport(s, tt.data)
			require.NoError(t, err)

			if result != tt.hasSupport {
				t.Errorf(
					"blocks.Eth1DataHasEnoughSupport(%+v) = %t, wanted %t",
					tt.data,
					result,
					tt.hasSupport,
				)
			}
		})
	}
}

func TestAreEth1DataEqual(t *testing.T) {
	type args struct {
		a *zondpb.Eth1Data
		b *zondpb.Eth1Data
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "true when both are nil",
			args: args{
				a: nil,
				b: nil,
			},
			want: true,
		},
		{
			name: "false when only one is nil",
			args: args{
				a: nil,
				b: &zondpb.Eth1Data{
					DepositRoot:  make([]byte, 32),
					DepositCount: 0,
					BlockHash:    make([]byte, 32),
				},
			},
			want: false,
		},
		{
			name: "true when real equality",
			args: args{
				a: &zondpb.Eth1Data{
					DepositRoot:  make([]byte, 32),
					DepositCount: 0,
					BlockHash:    make([]byte, 32),
				},
				b: &zondpb.Eth1Data{
					DepositRoot:  make([]byte, 32),
					DepositCount: 0,
					BlockHash:    make([]byte, 32),
				},
			},
			want: true,
		},
		{
			name: "false is field value differs",
			args: args{
				a: &zondpb.Eth1Data{
					DepositRoot:  make([]byte, 32),
					DepositCount: 0,
					BlockHash:    make([]byte, 32),
				},
				b: &zondpb.Eth1Data{
					DepositRoot:  make([]byte, 32),
					DepositCount: 64,
					BlockHash:    make([]byte, 32),
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, blocks.AreEth1DataEqual(tt.args.a, tt.args.b))
		})
	}
}

func TestProcessEth1Data_SetsCorrectly(t *testing.T) {
	beaconState, err := state_native.InitializeFromProtoPhase0(&zondpb.BeaconState{
		Eth1DataVotes: []*zondpb.Eth1Data{},
	})
	require.NoError(t, err)

	b := util.NewBeaconBlock()
	b.Block = &zondpb.BeaconBlock{
		Body: &zondpb.BeaconBlockBody{
			Eth1Data: &zondpb.Eth1Data{
				DepositRoot: []byte{2},
				BlockHash:   []byte{3},
			},
		},
	}

	period := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerEth1VotingPeriod)))
	for i := uint64(0); i < period; i++ {
		processedState, err := blocks.ProcessEth1DataInBlock(context.Background(), beaconState, b.Block.Body.Eth1Data)
		require.NoError(t, err)
		require.Equal(t, true, processedState.Version() == version.Phase0)
	}

	newETH1DataVotes := beaconState.Eth1DataVotes()
	if len(newETH1DataVotes) <= 1 {
		t.Error("Expected new ETH1 data votes to have length > 1")
	}
	if !proto.Equal(beaconState.Eth1Data(), zondpb.CopyETH1Data(b.Block.Body.Eth1Data)) {
		t.Errorf(
			"Expected latest eth1 data to have been set to %v, received %v",
			b.Block.Body.Eth1Data,
			beaconState.Eth1Data(),
		)
	}
}
