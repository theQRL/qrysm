package monitor

import (
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProcessSyncCommitteeContribution(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupService(t)

	contrib := &qrysmpb.SignedContributionAndProof{
		Message: &qrysmpb.ContributionAndProof{
			AggregatorIndex: 1,
		},
	}

	s.processSyncCommitteeContribution(contrib)
	require.LogsContain(t, hook, "\"Sync committee aggregation processed\" ValidatorIndex=1")
	require.LogsDoNotContain(t, hook, "ValidatorIndex=2 prefix=monitor")
}

func TestProcessSyncAggregate(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupService(t)
	beaconState, _ := util.DeterministicGenesisStateZond(t, 256)

	block := &qrysmpb.BeaconBlockZond{
		Slot: 2,
		Body: &qrysmpb.BeaconBlockBodyZond{
			SyncAggregate: &qrysmpb.SyncAggregate{
				SyncCommitteeBits: bitfield.Bitvector128{
					0x31, 0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
				},
			},
		},
	}

	wrappedBlock, err := blocks.NewBeaconBlock(block)
	require.NoError(t, err)

	s.processSyncAggregate(beaconState, wrappedBlock)
	require.LogsContain(t, hook, "\"Sync committee contribution included\" BalanceChange=0 ContribCount=1 ExpectedContribCount=4 NewBalance=40000000000000 ValidatorIndex=1 prefix=monitor")
	require.LogsContain(t, hook, "\"Sync committee contribution included\" BalanceChange=100000000 ContribCount=2 ExpectedContribCount=2 NewBalance=40000000000000 ValidatorIndex=44 prefix=monitor")
	require.LogsDoNotContain(t, hook, "ValidatorIndex=2 prefix=monitor")
}
