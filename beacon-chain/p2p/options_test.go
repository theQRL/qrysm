package p2p

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"path"
	"testing"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/protocol"
	gqrlCrypto "github.com/theQRL/go-qrl/crypto"
	"github.com/theQRL/go-qrl/p2p/qnode"
	"github.com/theQRL/go-qrl/p2p/qnr"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/config/params"
	ecdsaqrysm "github.com/theQRL/qrysm/crypto/ecdsa"
	"github.com/theQRL/qrysm/network"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestPrivateKeyLoading(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	file, err := os.CreateTemp(t.TempDir(), "key")
	require.NoError(t, err)
	key, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err, "Could not generate key")
	raw, err := key.Raw()
	if err != nil {
		panic(err)
	}
	out := hex.EncodeToString(raw)

	err = os.WriteFile(file.Name(), []byte(out), params.BeaconIoConfig().ReadWritePermissions)
	require.NoError(t, err, "Could not write key to file")
	log.WithField("file", file.Name()).WithField("key", out).Info("Wrote key to file")
	cfg := &Config{
		PrivateKey: file.Name(),
	}
	pKey, err := privKey(cfg)
	require.NoError(t, err, "Could not apply option")
	newPkey, err := ecdsaqrysm.ConvertToInterfacePrivkey(pKey)
	require.NoError(t, err)
	rawBytes, err := key.Raw()
	require.NoError(t, err)
	newRaw, err := newPkey.Raw()
	require.NoError(t, err)
	assert.DeepEqual(t, rawBytes, newRaw, "Private keys do not match")
}

func TestPrivateKeyLoading_StaticPrivateKey(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	tempDir := t.TempDir()

	cfg := &Config{
		StaticPeerID: true,
		DataDir:      tempDir,
	}
	pKey, err := privKey(cfg)
	require.NoError(t, err, "Could not apply option")

	newPkey, err := ecdsaqrysm.ConvertToInterfacePrivkey(pKey)
	require.NoError(t, err)

	retrievedKey, err := privKeyFromFile(path.Join(tempDir, keyPath))
	require.NoError(t, err)
	retrievedPKey, err := ecdsaqrysm.ConvertToInterfacePrivkey(retrievedKey)
	require.NoError(t, err)

	rawBytes, err := retrievedPKey.Raw()
	require.NoError(t, err)
	newRaw, err := newPkey.Raw()
	require.NoError(t, err)
	assert.DeepEqual(t, rawBytes, newRaw, "Private keys do not match")
}

func TestIPV6Support(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	key, err := gqrlCrypto.GenerateKey()
	require.NoError(t, err)
	db, err := qnode.OpenDB("")
	if err != nil {
		log.Error("could not open node's peer database")
	}
	lNode := qnode.NewLocalNode(db, key)
	mockIPV6 := net.IP{0xff, 0x02, 0xAA, 0, 0x1F, 0, 0x2E, 0, 0, 0x36, 0x45, 0, 0, 0, 0, 0x02}
	lNode.Set(qnr.IP(mockIPV6))
	ma, err := convertToSingleMultiAddr(lNode.Node())
	if err != nil {
		t.Fatal(err)
	}
	ipv6Exists := false
	for _, p := range ma.Protocols() {
		if p.Name == "ip4" {
			t.Error("Got ip4 address instead of ip6")
		}
		if p.Name == "ip6" {
			ipv6Exists = true
		}
	}
	if !ipv6Exists {
		t.Error("Multiaddress did not have ipv6 protocol")
	}
}

func TestDefaultMultiplexers(t *testing.T) {
	var cfg libp2p.Config
	_ = cfg
	p2pCfg := &Config{
		TCPPort:       2000,
		UDPPort:       2000,
		StateNotifier: &mock.MockStateNotifier{},
	}
	svc := &Service{cfg: p2pCfg}
	var err error
	svc.privKey, err = privKey(svc.cfg)
	assert.NoError(t, err)
	ipAddr := network.IPAddr()
	opts := svc.buildOptions(ipAddr, svc.privKey)
	err = cfg.Apply(append(opts, libp2p.FallbackDefaults)...)
	assert.NoError(t, err)

	assert.Equal(t, protocol.ID("/yamux/1.0.0"), cfg.Muxers[0].ID)
	assert.Equal(t, protocol.ID("/mplex/6.7.0"), cfg.Muxers[1].ID)

}

func TestSetConnManagerOption(t *testing.T) {
	cases := []struct {
		name      string
		maxPeers  uint
		highWater int
	}{
		{
			name:      "MaxPeers lower than default high water mark",
			maxPeers:  defaultConnManagerPruneAbove - 1,
			highWater: defaultConnManagerPruneAbove,
		},
		{
			name:      "MaxPeers equal to default high water mark",
			maxPeers:  defaultConnManagerPruneAbove,
			highWater: defaultConnManagerPruneAbove,
		},
		{
			name:      "MaxPeers higher than default high water mark",
			maxPeers:  defaultConnManagerPruneAbove + 1,
			highWater: defaultConnManagerPruneAbove + 1 + connManagerPruneAmount,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{MaxPeers: tt.maxPeers}
			_, high := cfg.connManagerLowHigh()
			require.Equal(t, true, high > int(cfg.MaxPeers))

			var libCfg libp2p.Config
			require.NoError(t, libCfg.Apply(setConnManagerOption(cfg), libp2p.FallbackDefaults))
			checkConnLimit(t, libCfg.ConnManager, high)
		})
	}
}

type connLimitGetter int

func (m connLimitGetter) GetConnLimit() int {
	return int(m)
}

// checkConnLimit verifies the conn manager's high-water mark by probing CheckLimit at the
// expected value (must succeed) and one below (must fail).
func checkConnLimit(t *testing.T, cm connmgr.ConnManager, expected int) {
	require.NoError(t, cm.CheckLimit(connLimitGetter(expected)), "Connection manager limit check failed")
	if err := cm.CheckLimit(connLimitGetter(expected - 1)); err == nil {
		t.Errorf("connection manager limit is below the expected value of %d", expected)
	}
}
