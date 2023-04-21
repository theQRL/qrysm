package blockchain

import (
	"context"
	"io"
	"testing"

	testDB "github.com/cyyber/qrysm/v4/beacon-chain/db/testing"
	"github.com/cyyber/qrysm/v4/consensus-types/blocks"
	"github.com/cyyber/qrysm/v4/testing/require"
	"github.com/cyyber/qrysm/v4/testing/util"
	"github.com/sirupsen/logrus"
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
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlock())
	st, _ := util.DeterministicGenesisState(t, 1)
	require.NoError(t, err)
	go func() {
		require.NoError(t, s.saveHead(context.Background(), [32]byte{}, b, st))
	}()
	require.NoError(t, s.saveHead(context.Background(), [32]byte{}, b, st))
}
