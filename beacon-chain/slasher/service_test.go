package slasher

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/async/event"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	dbtest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestMain(m *testing.M) {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetOutput(io.Discard)

	m.Run()
}

func TestService_StartStop_ChainInitialized(t *testing.T) {
	slasherDB := dbtest.SetupSlasherDB(t)
	hook := logTest.NewGlobal()
	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	currentSlot := primitives.Slot(4)
	require.NoError(t, beaconState.SetSlot(currentSlot))
	mockChain := &mock.ChainService{
		State: beaconState,
		Slot:  &currentSlot,
	}
	gs := startup.NewClockSynchronizer()
	srv, err := New(context.Background(), &ServiceConfig{
		IndexedAttestationsFeed: new(event.Feed),
		BeaconBlockHeadersFeed:  new(event.Feed),
		StateNotifier:           &mock.MockStateNotifier{},
		Database:                slasherDB,
		HeadStateFetcher:        mockChain,
		SyncChecker:             &mockSync.Sync{IsSyncing: false},
		ClockWaiter:             gs,
	})
	require.NoError(t, err)
	go srv.Start()
	time.Sleep(time.Millisecond * 100)
	var vr [32]byte
	require.NoError(t, gs.SetClock(startup.NewClock(time.Now(), vr)))
	time.Sleep(time.Millisecond * 100)
	srv.attsSlotTicker = &slots.SlotTicker{}
	srv.blocksSlotTicker = &slots.SlotTicker{}
	srv.pruningSlotTicker = &slots.SlotTicker{}
	require.NoError(t, srv.Stop())
	require.NoError(t, srv.Status())
	require.LogsContain(t, hook, "received chain initialization")
}

// Stopping the service before the chain clock is known must neither
// dereference a nil clock nor race the run goroutine: Stop waits for run, and
// run exits as soon as the clock wait is cancelled.
func TestService_StopBeforeChainStart(t *testing.T) {
	slasherDB := dbtest.SetupSlasherDB(t)
	s, err := New(context.Background(), &ServiceConfig{
		Database:    slasherDB,
		ClockWaiter: startup.NewClockSynchronizer(),
	})
	require.NoError(t, err)
	s.latestEpochWrittenForValidator[1] = 3

	s.Start()
	done := make(chan error, 1)
	go func() { done <- s.Stop() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Stop did not return")
	}
	require.ErrorIs(t, s.ctx.Err(), context.Canceled)
}
