package validator

import (
	"context"
	"testing"
	"time"

	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	opfeed "github.com/theQRL/qrysm/beacon-chain/core/feed/operation"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/operations/synccommittee"
	mockp2p "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/beacon-chain/rpc/testutil"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestGetSyncMessageBlockRoot_OK(t *testing.T) {
	r := []byte{'a'}
	server := &Server{
		HeadFetcher:           &mock.ChainService{Root: r},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{},
	}
	res, err := server.GetSyncMessageBlockRoot(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
	require.DeepEqual(t, r, res.Root)
}

func TestGetSyncMessageBlockRoot_Optimistic(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	server := &Server{
		HeadFetcher:           &mock.ChainService{},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: true},
	}
	_, err := server.GetSyncMessageBlockRoot(context.Background(), &emptypb.Empty{})
	s, ok := status.FromError(err)
	require.Equal(t, true, ok)
	require.DeepEqual(t, codes.Unavailable, s.Code())
	require.ErrorContains(t, errOptimisticMode.Error(), err)

	server = &Server{
		HeadFetcher:           &mock.ChainService{},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
	}
	_, err = server.GetSyncMessageBlockRoot(context.Background(), &emptypb.Empty{})
	require.NoError(t, err)
}

func TestSubmitSyncMessage_OK(t *testing.T) {
	f := testutil.NewSyncCommitteeFixture(t, nil, 1)
	server := &Server{
		CoreService: &core.Service{
			SyncCommitteePool:  synccommittee.NewStore(),
			P2P:                &mockp2p.MockBroadcaster{},
			HeadFetcher:        f.Head,
			GenesisTimeFetcher: f.Head,
		},
	}
	msg := f.Message(t, 2, make([]byte, 32))
	_, err := server.SubmitSyncMessage(context.Background(), msg)
	require.NoError(t, err)
	savedMsgs, err := server.CoreService.SyncCommitteePool.SyncCommitteeMessages(1)
	require.NoError(t, err)
	require.DeepEqual(t, []*qrysmpb.SyncCommitteeMessage{msg}, savedMsgs)
}

func TestGetSyncSubcommitteeIndex_Ok(t *testing.T) {
	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()

	server := &Server{
		HeadFetcher: &mock.ChainService{
			SyncCommitteeIndices: []primitives.CommitteeIndex{0},
		},
	}
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	// Request slot 0, should get the index 0 for validator 0.
	res, err := server.GetSyncSubcommitteeIndex(context.Background(), &qrysmpb.SyncSubcommitteeIndexRequest{
		PublicKey: pubKey[:], Slot: primitives.Slot(0),
	})
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, res.Indices)
}

func TestGetSyncCommitteeContribution_FiltersDuplicates(t *testing.T) {
	st, keys := util.DeterministicGenesisStateZond(t, 10)
	f := testutil.NewSyncCommitteeFixture(t, keys[2], 1)
	syncCommitteePool := synccommittee.NewStore()
	headFetcher := f.Head
	headFetcher.State = st
	headFetcher.SyncCommitteeIndices = []primitives.CommitteeIndex{10}
	server := &Server{
		CoreService: &core.Service{
			SyncCommitteePool:  syncCommitteePool,
			HeadFetcher:        headFetcher,
			GenesisTimeFetcher: headFetcher,
			P2P:                &mockp2p.MockBroadcaster{},
		},
		SyncCommitteePool:     syncCommitteePool,
		HeadFetcher:           headFetcher,
		P2P:                   &mockp2p.MockBroadcaster{},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{},
	}
	msg := f.Message(t, 2, make([]byte, 32))
	sig := msg.Signature
	_, err := server.SubmitSyncMessage(context.Background(), msg)
	require.NoError(t, err)
	_, err = server.SubmitSyncMessage(context.Background(), msg)
	require.NoError(t, err)
	val, err := st.ValidatorAtIndex(2)
	require.NoError(t, err)

	contr, err := server.GetSyncCommitteeContribution(context.Background(),
		&qrysmpb.SyncCommitteeContributionRequest{
			Slot:      1,
			PublicKey: val.PublicKey,
			SubnetId:  0})
	require.NoError(t, err)
	assert.DeepEqual(t, sig, contr.Signatures[0])
}

// TestGetSyncCommitteeContribution_UsesAggregatorRootNotHead ports upstream
// #17277: the contribution is built from the root the aggregator voted for,
// not the head at aggregation time, so a block arriving after the sync
// message deadline does not empty the contribution.
func TestGetSyncCommitteeContribution_UsesAggregatorRootNotHead(t *testing.T) {
	votedRoot := bytesutil.PadTo([]byte("A"), 32)
	lateBlockRoot := bytesutil.PadTo([]byte("B"), 32)

	st, keys := util.DeterministicGenesisStateZond(t, 10)
	f := testutil.NewSyncCommitteeFixture(t, keys[0], 1)
	syncCommitteePool := synccommittee.NewStore()
	headFetcher := f.Head
	headFetcher.State = st
	headFetcher.SyncCommitteeIndices = []primitives.CommitteeIndex{10}
	headFetcher.Root = lateBlockRoot
	server := &Server{
		CoreService: &core.Service{
			SyncCommitteePool:  syncCommitteePool,
			HeadFetcher:        headFetcher,
			GenesisTimeFetcher: headFetcher,
			P2P:                &mockp2p.MockBroadcaster{},
		},
		SyncCommitteePool:     syncCommitteePool,
		HeadFetcher:           headFetcher,
		P2P:                   &mockp2p.MockBroadcaster{},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{},
	}

	// The mock resolves any public key to validator index 0.
	msg := f.Message(t, 0, votedRoot)
	sig := msg.Signature
	_, err := server.SubmitSyncMessage(context.Background(), msg)
	require.NoError(t, err)

	val, err := st.ValidatorAtIndex(0)
	require.NoError(t, err)
	contr, err := server.GetSyncCommitteeContribution(context.Background(),
		&qrysmpb.SyncCommitteeContributionRequest{
			Slot:      1,
			PublicKey: val.PublicKey,
			SubnetId:  0})
	require.NoError(t, err)

	assert.DeepEqual(t, votedRoot, contr.BlockRoot)
	assert.Equal(t, uint64(1), contr.AggregationBits.Count())
	require.Equal(t, 1, len(contr.Signatures))
	assert.DeepEqual(t, sig, contr.Signatures[0])
}

func TestSubmitSignedContributionAndProof_OK(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SyncCommitteeSize = field_params.SyncCommitteeLength
	params.OverrideBeaconConfig(cfg)
	f := testutil.NewSyncCommitteeFixture(t, nil, 1)
	server := &Server{
		CoreService: &core.Service{
			HeadFetcher:        f.Head,
			GenesisTimeFetcher: f.Head,
			SyncCommitteePool:  synccommittee.NewStore(),
			Broadcaster:        &mockp2p.MockBroadcaster{},
			OperationNotifier:  (&mock.ChainService{}).OperationNotifier(),
		},
	}
	contribution := f.Contribution(t)
	_, err := server.SubmitSignedContributionAndProof(context.Background(), contribution)
	require.NoError(t, err)
	savedMsgs, err := server.CoreService.SyncCommitteePool.SyncCommitteeContributions(1)
	require.NoError(t, err)
	require.DeepEqual(t, []*qrysmpb.SyncCommitteeContribution{contribution.Message.Contribution}, savedMsgs)
}

func TestSubmitSignedContributionAndProof_Notification(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SyncCommitteeSize = field_params.SyncCommitteeLength
	params.OverrideBeaconConfig(cfg)
	f := testutil.NewSyncCommitteeFixture(t, nil, 1)
	server := &Server{
		CoreService: &core.Service{
			HeadFetcher:        f.Head,
			GenesisTimeFetcher: f.Head,
			SyncCommitteePool:  synccommittee.NewStore(),
			Broadcaster:        &mockp2p.MockBroadcaster{},
			OperationNotifier:  (&mock.ChainService{}).OperationNotifier(),
		},
	}

	// Subscribe to operation notifications.
	opChannel := make(chan *feed.Event, 1024)
	opSub := server.CoreService.OperationNotifier.OperationFeed().Subscribe(opChannel)
	defer opSub.Unsubscribe()

	contribution := f.Contribution(t)
	_, err := server.SubmitSignedContributionAndProof(context.Background(), contribution)
	require.NoError(t, err)

	// Ensure the state notification was broadcast.
	notificationFound := false
	for !notificationFound {
		select {
		case event := <-opChannel:
			if event.Type == opfeed.SyncCommitteeContributionReceived {
				notificationFound = true
				data, ok := event.Data.(*opfeed.SyncCommitteeContributionReceivedData)
				assert.Equal(t, true, ok, "Entity is of the wrong type")
				assert.NotNil(t, data.Contribution)
			}
		case <-opSub.Err():
			t.Error("Subscription to state notifier failed")
			return
		}
	}
}
