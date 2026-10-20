package validator

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/operations/synccommittee"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestProposer_SyncAggregateSubnetLayout(t *testing.T) {
	for _, count := range []uint64{1, 2, 4} {
		t.Run(fmt.Sprintf("subnets_%d", count), func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			cfg := params.MainnetConfig().Copy()
			if fieldparams.Preset == params.MinimalName {
				cfg = params.MinimalSpecConfig().Copy()
			}
			cfg.SyncCommitteeSubnetCount = count
			// Divisibility alone permits all these counts. Startup must also
			// enforce the binary's contribution and aggregate bitfield sizes.
			require.NoError(t, cfg.Validate())
			layoutErr := cfg.ValidateStateLayout()

			// Deliberately bypass startup validation to exercise the actual
			// proposer assembly, including its empty-contribution case.
			params.OverrideBeaconConfig(cfg)
			proposer := &Server{SyncCommitteePool: synccommittee.NewStore()}
			aggregate, err := proposer.getSyncAggregate(context.Background(), 1, [32]byte{}, nil)
			require.NoError(t, err)
			require.Equal(t, count*fieldparams.SyncCommitteeAggregationBytesLength, uint64(len(aggregate.SyncCommitteeBits)))
			encoded, err := aggregate.MarshalSSZ()
			if count != 1 {
				require.ErrorContains(t, "SyncCommitteeBits", err)
				require.ErrorContains(t, "SYNC_COMMITTEE_SUBNET_COUNT", layoutErr)
				return
			}
			require.NoError(t, layoutErr)
			require.NoError(t, err)
			decoded := new(qrysmpb.SyncAggregate)
			require.NoError(t, decoded.UnmarshalSSZ(encoded))
			require.DeepEqual(t, aggregate, decoded)
		})
	}
}
