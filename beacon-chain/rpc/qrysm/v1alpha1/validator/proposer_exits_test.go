package validator

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zond "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestServer_getExits(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.ShardCommitteePeriod = 0
	params.OverrideBeaconConfig(config)

	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 256)

	proposerServer := &Server{
		ExitPool: voluntaryexits.NewPool(),
	}

	exits := make([]*zond.SignedVoluntaryExit, params.BeaconConfig().MaxVoluntaryExits)
	for i := primitives.ValidatorIndex(0); uint64(i) < params.BeaconConfig().MaxVoluntaryExits; i++ {
		exit, err := util.GenerateVoluntaryExits(beaconState, privKeys[i], i)
		require.NoError(t, err)
		proposerServer.ExitPool.InsertVoluntaryExit(exit)
		exits[i] = exit
	}

	e := proposerServer.getExits(beaconState, 1)
	require.Equal(t, len(e), int(params.BeaconConfig().MaxVoluntaryExits))
	require.DeepEqual(t, e, exits)
}
