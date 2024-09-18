package blockchain

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/async/event"
	"github.com/theQRL/qrysm/beacon-chain/cache/depositcache"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/db"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/beacon-chain/operations/dilithiumtoexec"
	"github.com/theQRL/qrysm/beacon-chain/p2p"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"google.golang.org/protobuf/proto"
)

type mockBeaconNode struct {
	stateFeed *event.Feed
}

// StateFeed mocks the same method in the beacon node.
func (mbn *mockBeaconNode) StateFeed() *event.Feed {
	if mbn.stateFeed == nil {
		mbn.stateFeed = new(event.Feed)
	}
	return mbn.stateFeed
}

type mockBroadcaster struct {
	broadcastCalled bool
}

func (mb *mockBroadcaster) Broadcast(_ context.Context, _ proto.Message) error {
	mb.broadcastCalled = true
	return nil
}

func (mb *mockBroadcaster) BroadcastAttestation(_ context.Context, _ uint64, _ *zondpb.Attestation) error {
	mb.broadcastCalled = true
	return nil
}

func (mb *mockBroadcaster) BroadcastSyncCommitteeMessage(_ context.Context, _ uint64, _ *zondpb.SyncCommitteeMessage) error {
	mb.broadcastCalled = true
	return nil
}

func (mb *mockBroadcaster) BroadcastDilithiumChanges(_ context.Context, _ []*zondpb.SignedDilithiumToExecutionChange) {
}

var _ p2p.Broadcaster = (*mockBroadcaster)(nil)

type testServiceRequirements struct {
	ctx           context.Context
	db            db.Database
	fcs           forkchoice.ForkChoicer
	sg            *stategen.State
	notif         statefeed.Notifier
	cs            *startup.ClockSynchronizer
	attPool       attestations.Pool
	attSrv        *attestations.Service
	dilithiumPool *dilithiumtoexec.Pool
	dc            *depositcache.DepositCache
}

func minimalTestService(t *testing.T, opts ...Option) (*Service, *testServiceRequirements) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	fcs := doublylinkedtree.New()
	sg := stategen.New(beaconDB, fcs)
	notif := &mockBeaconNode{}
	fcs.SetBalancesByRooter(sg.ActiveNonSlashedBalancesByRoot)
	cs := startup.NewClockSynchronizer()
	attPool := attestations.NewPool()
	attSrv, err := attestations.NewService(ctx, &attestations.Config{Pool: attPool})
	require.NoError(t, err)
	dilithiumPool := dilithiumtoexec.NewPool()
	dc, err := depositcache.New()
	require.NoError(t, err)
	mockEngine := &mockExecution.EngineClient{ErrNewPayload: execution.ErrAcceptedSyncingPayloadStatus, ErrForkchoiceUpdated: execution.ErrAcceptedSyncingPayloadStatus}
	req := &testServiceRequirements{
		ctx:           ctx,
		db:            beaconDB,
		fcs:           fcs,
		sg:            sg,
		notif:         notif,
		cs:            cs,
		attPool:       attPool,
		attSrv:        attSrv,
		dilithiumPool: dilithiumPool,
		dc:            dc,
	}
	defOpts := []Option{WithDatabase(req.db),
		WithStateNotifier(req.notif),
		WithStateGen(req.sg),
		WithForkChoiceStore(req.fcs),
		WithClockSynchronizer(req.cs),
		WithAttestationPool(req.attPool),
		WithAttestationService(req.attSrv),
		WithDilithiumToExecPool(req.dilithiumPool),
		WithDepositCache(dc),
		WithExecutionEngineCaller(mockEngine),
	}
	// append the variadic opts so they override the defaults by being processed afterwards
	opts = append(defOpts, opts...)
	s, err := NewService(req.ctx, opts...)

	require.NoError(t, err)
	return s, req
}
