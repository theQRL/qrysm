package blockchain

import (
	"context"
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	testDB "github.com/theQRL/qrysm/v4/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)
}

func TestChainService_SaveHead_DataRace(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	s := &Service{
		cfg: &config{BeaconDB: beaconDB},
	}
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
	st, _ := util.DeterministicGenesisStateCapella(t, 1)
	require.NoError(t, err)
	go func() {
		require.NoError(t, s.saveHead(context.Background(), [32]byte{}, b, st))
	}()
	require.NoError(t, s.saveHead(context.Background(), [32]byte{}, b, st))
}
