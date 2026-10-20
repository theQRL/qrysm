package p2p

import (
	"context"
	"fmt"
	"net"
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-qrl/crypto"
	"github.com/theQRL/go-qrl/p2p/qnode"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

// Test `verifyConnectivity` by dialing a local TCP listener (succeeds) and an
// unreachable IP (logs an error). Using a local listener instead of a
// hardcoded external IP avoids CI flakes when the external host moves.
func TestVerifyConnectivity(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		require.NoError(t, ln.Close())
	}()
	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	var port uint
	_, err = fmt.Sscanf(portStr, "%d", &port)
	require.NoError(t, err)

	hook := logTest.NewGlobal()
	cases := []struct {
		address              string
		port                 uint
		expectedConnectivity bool
		name                 string
	}{
		{host, port, true, "Dialing a reachable local listener"},
		{"123.123.123.123", 19000, false, "Dialing an unreachable IP: 123.123.123.123:19000"},
	}
	for _, tc := range cases {
		t.Run(tc.name,
			func(t *testing.T) {
				verifyConnectivity(tc.address, tc.port, "tcp")
				logMessage := "IP address is not accessible"
				if tc.expectedConnectivity {
					require.LogsDoNotContain(t, hook, logMessage)
				} else {
					require.LogsContain(t, hook, logMessage)
				}
			})
	}
}

func TestSerializeQNR(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	t.Run("Ok", func(t *testing.T) {
		key, err := crypto.GenerateKey()
		require.NoError(t, err)
		db, err := qnode.OpenDB("")
		require.NoError(t, err)
		lNode := qnode.NewLocalNode(db, key)
		record := lNode.Node().Record()
		s, err := SerializeQNR(record)
		require.NoError(t, err)
		assert.NotEqual(t, "", s)
		s = "qnr:" + s
		newRec, err := qnode.Parse(qnode.ValidSchemes, s)
		require.NoError(t, err)
		assert.Equal(t, s, newRec.String())
	})

	t.Run("Nil record", func(t *testing.T) {
		_, err := SerializeQNR(nil)
		require.NotNil(t, err)
		assert.ErrorContains(t, "could not serialize nil record", err)
	})
}

func TestMetadataFromDB(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	t.Run("Metadata from DB", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)
		require.NoError(t, beaconDB.SaveMetadataSeqNum(context.Background(), 42))

		metaData, err := metaDataFromDB(context.Background(), beaconDB)
		require.NoError(t, err)
		assert.Equal(t, uint64(42), metaData.SequenceNumber())
	})

	t.Run("Default sequence number when key is missing", func(t *testing.T) {
		beaconDB := testDB.SetupDB(t)

		metaData, err := metaDataFromDB(context.Background(), beaconDB)
		require.NoError(t, err)
		assert.Equal(t, uint64(0), metaData.SequenceNumber())
	})
}
