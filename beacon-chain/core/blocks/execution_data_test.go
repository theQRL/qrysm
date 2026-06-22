package blocks_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/protobuf/proto"
)

func FakeDeposits(n uint64) []*qrysmpb.ExecutionData {
	deposits := make([]*qrysmpb.ExecutionData, n)
	for i := range n {
		deposits[i] = &qrysmpb.ExecutionData{
			DepositCount: 1,
			DepositRoot:  bytesutil.PadTo([]byte("root"), 32),
		}
	}
	return deposits
}

func TestExecutionDataHasEnoughSupport(t *testing.T) {
	tests := []struct {
		stateVotes         []*qrysmpb.ExecutionData
		data               *qrysmpb.ExecutionData
		hasSupport         bool
		votingPeriodLength primitives.Epoch
	}{
		{
			stateVotes: FakeDeposits(uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))),
			data: &qrysmpb.ExecutionData{
				DepositCount: 1,
				DepositRoot:  bytesutil.PadTo([]byte("root"), 32),
			},
			hasSupport:         true,
			votingPeriodLength: 7,
		}, {
			stateVotes: FakeDeposits(uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))),
			data: &qrysmpb.ExecutionData{
				DepositCount: 1,
				DepositRoot:  bytesutil.PadTo([]byte("root"), 32),
			},
			hasSupport:         false,
			votingPeriodLength: 8,
		}, {
			stateVotes: FakeDeposits(uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))),
			data: &qrysmpb.ExecutionData{
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
			c.EpochsPerExecutionVotingPeriod = tt.votingPeriodLength
			params.OverrideBeaconConfig(c)

			s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
				ExecutionDataVotes: tt.stateVotes,
			})
			require.NoError(t, err)
			result, err := blocks.ExecutionDataHasEnoughSupport(s, tt.data)
			require.NoError(t, err)

			if result != tt.hasSupport {
				t.Errorf(
					"blocks.ExecutionDataHasEnoughSupport(%+v) = %t, wanted %t",
					tt.data,
					result,
					tt.hasSupport,
				)
			}
		})
	}
}

func TestAreExecutionDataEqual(t *testing.T) {
	type args struct {
		a *qrysmpb.ExecutionData
		b *qrysmpb.ExecutionData
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
				b: &qrysmpb.ExecutionData{
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
				a: &qrysmpb.ExecutionData{
					DepositRoot:  make([]byte, 32),
					DepositCount: 0,
					BlockHash:    make([]byte, 32),
				},
				b: &qrysmpb.ExecutionData{
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
				a: &qrysmpb.ExecutionData{
					DepositRoot:  make([]byte, 32),
					DepositCount: 0,
					BlockHash:    make([]byte, 32),
				},
				b: &qrysmpb.ExecutionData{
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
			assert.Equal(t, tt.want, blocks.AreExecutionDataEqual(tt.args.a, tt.args.b))
		})
	}
}

func TestProcessExecutionData_SetsCorrectly(t *testing.T) {
	beaconState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		ExecutionDataVotes: []*qrysmpb.ExecutionData{},
	})
	require.NoError(t, err)

	b := util.NewBeaconBlockZond()
	b.Block = &qrysmpb.BeaconBlockZond{
		Body: &qrysmpb.BeaconBlockBodyZond{
			ExecutionData: &qrysmpb.ExecutionData{
				DepositRoot: []byte{2},
				BlockHash:   []byte{3},
			},
		},
	}

	period := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerExecutionVotingPeriod)))
	for range period {
		processedState, err := blocks.ProcessExecutionDataInBlock(context.Background(), beaconState, b.Block.Body.ExecutionData)
		require.NoError(t, err)
		require.Equal(t, true, processedState.Version() == version.Zond)
	}

	newExecutionDataVotes := beaconState.ExecutionDataVotes()
	if len(newExecutionDataVotes) <= 1 {
		t.Error("Expected new execution data votes to have length > 1")
	}
	if !proto.Equal(beaconState.ExecutionData(), qrysmpb.CopyExecutionData(b.Block.Body.ExecutionData)) {
		t.Errorf(
			"Expected latest execution data to have been set to %v, received %v",
			b.Block.Body.ExecutionData,
			beaconState.ExecutionData(),
		)
	}
}
