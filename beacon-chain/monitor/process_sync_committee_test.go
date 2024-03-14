package monitor

import (
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestProcessSyncCommitteeContribution(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupService(t)

	contrib := &zondpb.SignedContributionAndProof{
		Message: &zondpb.ContributionAndProof{
			AggregatorIndex: 1,
		},
	}

	s.processSyncCommitteeContribution(contrib)
	require.LogsContain(t, hook, "\"Sync committee aggregation processed\" ValidatorIndex=1")
	require.LogsDoNotContain(t, hook, "ValidatorIndex=2")
}

func TestProcessSyncAggregate(t *testing.T) {
	hook := logTest.NewGlobal()
	s := setupService(t)
	beaconState, _ := util.DeterministicGenesisStateCapella(t, 256)

	block := &zondpb.BeaconBlockCapella{
		Slot: 2,
		Body: &zondpb.BeaconBlockBodyCapella{
			SyncAggregate: &zondpb.SyncAggregate{
				SyncCommitteeBits: bitfield.Bitvector16{
					0x31, 0xff,
				},
			},
		},
	}

	wrappedBlock, err := blocks.NewBeaconBlock(block)
	require.NoError(t, err)

	s.processSyncAggregate(beaconState, wrappedBlock)
	require.LogsContain(t, hook, "\"Sync committee contribution included\" BalanceChange=0 ContribCount=1 ExpectedContribCount=4 NewBalance=40000000000000 ValidatorIndex=1 prefix=monitor")
	require.LogsContain(t, hook, "\"Sync committee contribution included\" BalanceChange=100000000 ContribCount=2 ExpectedContribCount=2 NewBalance=40000000000000 ValidatorIndex=86 prefix=monitor")
	require.LogsDoNotContain(t, hook, "ValidatorIndex=2")
}
