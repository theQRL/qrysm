package state_native_test

import (
	"context"
	"fmt"
	"testing"

	statenative "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestExecutionVotingStateSSZRoundTrip(t *testing.T) {
	for _, doublePeriod := range []bool{false, true} {
		t.Run(fmt.Sprintf("double_period_%t", doublePeriod), func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			cfg := params.MainnetConfig().Copy()
			if fieldparams.Preset == params.MinimalName {
				cfg = params.MinimalSpecConfig().Copy()
			}
			if doublePeriod {
				cfg.EpochsPerExecutionVotingPeriod *= 2
				cfg.SlotsPerEpoch /= 2
			}
			require.NoError(t, cfg.Validate())
			require.NoError(t, cfg.ValidateStateLayout())
			params.OverrideBeaconConfig(cfg)
			st, err := util.NewBeaconStateZond()
			require.NoError(t, err)

			// Reuse the state to exercise both initial hashing and cached-trie updates.
			for _, count := range []int{0, 1, fieldparams.ExecutionDataVotesLength} {
				t.Run(fmt.Sprintf("votes_%d", count), func(t *testing.T) {
					votes := make([]*qrysmpb.ExecutionData, count)
					for i := range votes {
						votes[i] = &qrysmpb.ExecutionData{
							DepositRoot:  make([]byte, 32),
							DepositCount: uint64(i + 1),
							BlockHash:    make([]byte, 32),
						}
					}
					require.NoError(t, st.SetExecutionDataVotes(votes))
					nativeRoot, err := st.HashTreeRoot(context.Background())
					require.NoError(t, err)
					pb, err := statenative.ProtobufBeaconStateZond(st.ToProto())
					require.NoError(t, err)
					generatedRoot, err := pb.HashTreeRoot()
					require.NoError(t, err)
					require.Equal(t, generatedRoot, nativeRoot)

					encoded, err := st.MarshalSSZ()
					require.NoError(t, err)
					decoded := new(qrysmpb.BeaconStateZond)
					require.NoError(t, decoded.UnmarshalSSZ(encoded))
					require.Equal(t, count, len(decoded.ExecutionDataVotes))
					for i, vote := range votes {
						require.DeepEqual(t, vote, decoded.ExecutionDataVotes[i])
					}
					decodedRoot, err := decoded.HashTreeRoot()
					require.NoError(t, err)
					require.Equal(t, nativeRoot, decodedRoot)
				})
			}
		})
	}
}
