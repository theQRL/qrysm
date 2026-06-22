package p2p

import (
	"context"
	"crypto/rand"
	"reflect"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/pkg/errors"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/go-qrl/p2p/discover"
	"github.com/theQRL/go-qrl/p2p/qnode"
	"github.com/theQRL/go-qrl/p2p/qnr"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/db/kv"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/cmd/beacon-chain/flags"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/wrapper"
	ecdsaqrysm "github.com/theQRL/qrysm/crypto/ecdsa"
	pb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestStartDiscV5_DiscoverPeersWithSubnets(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	// This test needs to be entirely rewritten and should be done in a follow up PR from #7885.
	t.Skip("This test is now failing after PR 7885 due to false positive")
	gFlags := new(flags.GlobalFlags)
	gFlags.MinimumPeersPerSubnet = 4
	flags.Init(gFlags)
	// Reset config.
	defer flags.Init(new(flags.GlobalFlags))
	port := 2000
	ipAddr, pkey := createAddrAndPrivKey(t)
	genesisTime := time.Now()
	genesisValidatorsRoot := make([]byte, 32)
	s := &Service{
		cfg:                   &Config{UDPPort: uint(port)},
		genesisTime:           genesisTime,
		genesisValidatorsRoot: genesisValidatorsRoot,
	}
	bootListener, err := s.createListener(ipAddr, pkey)
	require.NoError(t, err)
	defer bootListener.Close()

	bootNode := bootListener.Self()
	// Use shorter period for testing.
	currentPeriod := pollingPeriod
	pollingPeriod = 1 * time.Second
	defer func() {
		pollingPeriod = currentPeriod
	}()

	var listeners []*discover.UDPv5
	for i := 1; i <= 3; i++ {
		port = 3000 + i
		cfg := &Config{
			BootstrapNodeAddr:   []string{bootNode.String()},
			Discv5BootStrapAddr: []string{bootNode.String()},
			MaxPeers:            30,
			UDPPort:             uint(port),
		}
		ipAddr, pkey := createAddrAndPrivKey(t)
		s = &Service{
			cfg:                   cfg,
			genesisTime:           genesisTime,
			genesisValidatorsRoot: genesisValidatorsRoot,
		}
		listener, err := s.startDiscoveryV5(ipAddr, pkey)
		assert.NoError(t, err, "Could not start discovery for node")
		bitV := bitfield.NewBitvector64()
		bitV.SetBitAt(uint64(i), true)

		entry := qnr.WithEntry(attSubnetQnrKey, &bitV)
		listener.LocalNode().Set(entry)
		listeners = append(listeners, listener)
	}
	defer func() {
		// Close down all peers.
		for _, listener := range listeners {
			listener.Close()
		}
	}()

	// Make one service on port 4001.
	port = 4001
	gs := startup.NewClockSynchronizer()
	cfg := &Config{
		BootstrapNodeAddr:   []string{bootNode.String()},
		Discv5BootStrapAddr: []string{bootNode.String()},
		MaxPeers:            30,
		UDPPort:             uint(port),
		ClockWaiter:         gs,
	}
	s, err = NewService(context.Background(), cfg)
	require.NoError(t, err)

	exitRoutine := make(chan bool)
	go func() {
		s.Start()
		<-exitRoutine
	}()
	time.Sleep(50 * time.Millisecond)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	var vr [32]byte
	require.NoError(t, gs.SetClock(startup.NewClock(time.Now(), vr)))

	// Wait for the nodes to have their local routing tables to be populated with the other nodes
	time.Sleep(6 * discoveryWaitTime)

	// look up 3 different subnets
	ctx := context.Background()
	exists, err := s.FindPeersWithSubnet(ctx, "", 1, flags.Get().MinimumPeersPerSubnet)
	require.NoError(t, err)
	exists2, err := s.FindPeersWithSubnet(ctx, "", 2, flags.Get().MinimumPeersPerSubnet)
	require.NoError(t, err)
	exists3, err := s.FindPeersWithSubnet(ctx, "", 3, flags.Get().MinimumPeersPerSubnet)
	require.NoError(t, err)
	if !exists || !exists2 || !exists3 {
		t.Fatal("Peer with subnet doesn't exist")
	}

	// Update QNR of a peer.
	testService := &Service{
		dv5Listener: listeners[0],
		metaData: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
			Attnets: bitfield.NewBitvector64(),
		}),
	}
	cache.SubnetIDs.AddAttesterSubnetID(0, 10)
	testService.RefreshQNR()
	time.Sleep(2 * time.Second)

	exists, err = s.FindPeersWithSubnet(ctx, "", 2, flags.Get().MinimumPeersPerSubnet)
	require.NoError(t, err)

	assert.Equal(t, true, exists, "Peer with subnet doesn't exist")
	assert.NoError(t, s.Stop())
	exitRoutine <- true
}

func Test_AttSubnets(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	tests := []struct {
		name        string
		record      func(t *testing.T) *qnr.Record
		want        []uint64
		wantErr     bool
		errContains string
	}{
		{
			name: "valid record",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				localNode = initializeAttSubnets(localNode)
				return localNode.Node().Record()
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "too small subnet",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				entry := qnr.WithEntry(attSubnetQnrKey, []byte{})
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "half sized subnet",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				entry := qnr.WithEntry(attSubnetQnrKey, make([]byte, 4))
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "too large subnet",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				entry := qnr.WithEntry(attSubnetQnrKey, make([]byte, byteCount(int(attestationSubnetCount))+1))
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "very large subnet",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				entry := qnr.WithEntry(attSubnetQnrKey, make([]byte, byteCount(int(attestationSubnetCount))+100))
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "single subnet",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				bitV := bitfield.NewBitvector64()
				bitV.SetBitAt(0, true)
				entry := qnr.WithEntry(attSubnetQnrKey, bitV.Bytes())
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:    []uint64{0},
			wantErr: false,
		},
		{
			name: "multiple subnets",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				bitV := bitfield.NewBitvector64()
				for i := uint64(0); i < bitV.Len(); i++ {
					// skip 2 subnets
					if (i+1)%2 == 0 {
						continue
					}
					bitV.SetBitAt(i, true)
				}
				bitV.SetBitAt(0, true)
				entry := qnr.WithEntry(attSubnetQnrKey, bitV.Bytes())
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want: []uint64{0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20,
				22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48,
				50, 52, 54, 56, 58, 60, 62},
			wantErr: false,
		},
		{
			name: "all subnets",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				bitV := bitfield.NewBitvector64()
				for i := uint64(0); i < bitV.Len(); i++ {
					bitV.SetBitAt(i, true)
				}
				entry := qnr.WithEntry(attSubnetQnrKey, bitV.Bytes())
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want: []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49,
				50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := attSubnets(tt.record(t))
			if (err != nil) != tt.wantErr {
				t.Errorf("syncSubnets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				assert.ErrorContains(t, tt.errContains, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("syncSubnets() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_SyncSubnets(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	tests := []struct {
		name        string
		record      func(t *testing.T) *qnr.Record
		want        []uint64
		wantErr     bool
		errContains string
	}{
		{
			name: "valid record",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				localNode = initializeSyncCommSubnets(localNode)
				return localNode.Node().Record()
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "too small subnet",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				entry := qnr.WithEntry(syncCommsSubnetQnrKey, []byte{})
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "too large subnet",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				entry := qnr.WithEntry(syncCommsSubnetQnrKey, make([]byte, byteCount(int(syncCommsSubnetCount))+1))
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "very large subnet",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				entry := qnr.WithEntry(syncCommsSubnetQnrKey, make([]byte, byteCount(int(syncCommsSubnetCount))+100))
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:        []uint64{},
			wantErr:     true,
			errContains: "invalid bitvector provided, it has a size of",
		},
		{
			name: "single subnet",
			record: func(t *testing.T) *qnr.Record {
				db, err := qnode.OpenDB("")
				assert.NoError(t, err)
				priv, _, err := crypto.GenerateSecp256k1Key(rand.Reader)
				assert.NoError(t, err)
				convertedKey, err := ecdsaqrysm.ConvertFromInterfacePrivKey(priv)
				assert.NoError(t, err)
				localNode := qnode.NewLocalNode(db, convertedKey)
				bitV := bitfield.Bitvector4{byte(0x00)}
				bitV.SetBitAt(0, true)
				entry := qnr.WithEntry(syncCommsSubnetQnrKey, bitV.Bytes())
				localNode.Set(entry)
				return localNode.Node().Record()
			},
			want:    []uint64{0},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := syncSubnets(tt.record(t))
			if (err != nil) != tt.wantErr {
				t.Errorf("syncSubnets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				assert.ErrorContains(t, tt.errContains, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("syncSubnets() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestService_UpdateSubnetRecord_PersistsSeqNumWithStaticPeerID(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := context.Background()

	s := &Service{
		ctx: ctx,
		cfg: &Config{
			StaticPeerID: true,
			DB:           beaconDB,
		},
		metaData: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
			SeqNumber: 4,
			Attnets:   bitfield.NewBitvector64(),
			Syncnets:  bitfield.NewBitvector4(),
		}),
	}

	bitV := bitfield.NewBitvector64()
	bitV.SetBitAt(1, true)
	bitS := bitfield.NewBitvector4()
	require.NoError(t, s.updateSubnetRecordWithMetadata(bitV, bitS))

	got, err := beaconDB.MetadataSeqNum(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), got)
}

func TestService_UpdateSubnetRecord_DoesNotPersistWithoutStaticPeerID(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := context.Background()

	s := &Service{
		ctx: ctx,
		cfg: &Config{
			StaticPeerID: false,
			DB:           beaconDB,
		},
		metaData: wrapper.WrappedMetadataV1(&pb.MetaDataV1{
			SeqNumber: 4,
			Attnets:   bitfield.NewBitvector64(),
			Syncnets:  bitfield.NewBitvector4(),
		}),
	}

	bitV := bitfield.NewBitvector64()
	bitS := bitfield.NewBitvector4()
	require.NoError(t, s.updateSubnetRecordWithMetadata(bitV, bitS))

	_, err := beaconDB.MetadataSeqNum(ctx)
	require.Equal(t, true, errors.Is(err, kv.ErrNotFoundMetadataSeqNum))
}

// sliceIterator is a minimal qnode.Iterator backed by a fixed slice of nodes,
// used by searchForPeers regression tests. It mimics discv5's behavior where
// the same node ID can appear multiple times in succession with different ENR
// sequence numbers.
type sliceIterator struct {
	nodes []*qnode.Node
	idx   int
}

func (it *sliceIterator) Next() bool {
	if it.idx >= len(it.nodes) {
		return false
	}
	it.idx++
	return true
}

func (it *sliceIterator) Node() *qnode.Node { return it.nodes[it.idx-1] }
func (it *sliceIterator) Close()            {}

// makeNullSignedNode builds a *qnode.Node with a deterministic ID and the given
// sequence number. Uses the null signature scheme so tests don't need real keys.
func makeNullSignedNode(t *testing.T, id qnode.ID, seq uint64) *qnode.Node {
	t.Helper()
	r := new(qnr.Record)
	r.SetSeq(seq)
	return qnode.SignNull(r, id)
}

// TestSearchForPeers_NewerEnrFailsFilter_RemovesStale is the regression test
// for upstream PR #15578. Before the fix, searchForPeers ran the filter before
// deduping by node ID, so a node observed first at a low Seq (passing the
// filter) and then again at a higher Seq (failing the filter) would leave the
// stale lower-Seq record in the dial set. The fix dedups first and removes the
// stale entry when the newer ENR fails the filter.
func TestSearchForPeers_NewerEnrFailsFilter_RemovesStale(t *testing.T) {
	id := qnode.ID{0x01}
	low := makeNullSignedNode(t, id, 1)
	high := makeNullSignedNode(t, id, 2)

	// Filter passes only for the low-seq record (e.g. the older ENR was
	// subscribed to the requested subnet but the newer ENR is not).
	filter := func(n *qnode.Node) bool { return n.Seq() == low.Seq() }

	it := &sliceIterator{nodes: []*qnode.Node{low, high}}
	got := searchForPeers(it, 16, 4, filter)
	assert.Equal(t, 0, len(got), "stale lower-Seq node must not be returned when newer ENR fails filter")
}

// TestSearchForPeers_NewerEnrPassesFilter keeps the freshest record when an
// older ENR for the same node ID was rejected by the filter. This passed
// before PR #15578 too, but the test guards against future regressions in the
// dedup-first ordering.
func TestSearchForPeers_NewerEnrPassesFilter(t *testing.T) {
	id := qnode.ID{0x02}
	low := makeNullSignedNode(t, id, 1)
	high := makeNullSignedNode(t, id, 2)

	filter := func(n *qnode.Node) bool { return n.Seq() == high.Seq() }

	it := &sliceIterator{nodes: []*qnode.Node{low, high}}
	got := searchForPeers(it, 16, 4, filter)
	require.Equal(t, 1, len(got))
	assert.Equal(t, high.Seq(), got[0].Seq())
}
