package validator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/async/event"
	mockChain "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache/depositcache"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/dilithium"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/mock"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestValidatorIndex_OK(t *testing.T) {
	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)

	pubKey := pubKey(1)

	err = st.SetValidators([]*zondpb.Validator{{PublicKey: pubKey}})
	require.NoError(t, err)

	Server := &Server{
		HeadFetcher: &mockChain.ChainService{State: st},
	}

	req := &zondpb.ValidatorIndexRequest{
		PublicKey: pubKey,
	}
	_, err = Server.ValidatorIndex(context.Background(), req)
	assert.NoError(t, err, "Could not get validator index")
}

func TestValidatorIndex_StateEmpty(t *testing.T) {
	Server := &Server{
		HeadFetcher: &mockChain.ChainService{},
	}
	pubKey := pubKey(1)
	req := &zondpb.ValidatorIndexRequest{
		PublicKey: pubKey,
	}
	_, err := Server.ValidatorIndex(context.Background(), req)
	assert.ErrorContains(t, "head state is empty", err)
}

func TestWaitForActivation_ContextClosed(t *testing.T) {
	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Slot:       0,
		Validators: []*zondpb.Validator{},
	})
	require.NoError(t, err)
	block := util.NewBeaconBlockCapella()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	ctx, cancel := context.WithCancel(context.Background())
	depositCache, err := depositcache.New()
	require.NoError(t, err)

	vs := &Server{
		Ctx:               ctx,
		ChainStartFetcher: &mockExecution.Chain{},
		BlockFetcher:      &mockExecution.Chain{},
		Eth1InfoFetcher:   &mockExecution.Chain{},
		DepositFetcher:    depositCache,
		HeadFetcher:       &mockChain.ChainService{State: beaconState, Root: genesisRoot[:]},
	}
	req := &zondpb.ValidatorActivationRequest{
		PublicKeys: [][]byte{pubKey(1)},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockChainStream := mock.NewMockBeaconNodeValidator_WaitForActivationServer(ctrl)
	mockChainStream.EXPECT().Context().Return(context.Background())
	mockChainStream.EXPECT().Send(gomock.Any()).Return(nil)
	mockChainStream.EXPECT().Context().Return(context.Background())
	exitRoutine := make(chan bool)
	go func(tt *testing.T) {
		want := "context canceled"
		assert.ErrorContains(tt, want, vs.WaitForActivation(req, mockChainStream))
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestWaitForActivation_MultipleStatuses(t *testing.T) {
	priv1, err := dilithium.RandKey()
	require.NoError(t, err)
	priv2, err := dilithium.RandKey()
	require.NoError(t, err)
	priv3, err := dilithium.RandKey()
	require.NoError(t, err)

	pubKey1 := priv1.PublicKey().Marshal()
	pubKey2 := priv2.PublicKey().Marshal()
	pubKey3 := priv3.PublicKey().Marshal()

	beaconState := &zondpb.BeaconStateCapella{
		Slot: 4000,
		Validators: []*zondpb.Validator{
			{
				PublicKey:       pubKey1,
				ActivationEpoch: 1,
				ExitEpoch:       params.BeaconConfig().FarFutureEpoch,
			},
			{
				PublicKey:                  pubKey2,
				ActivationEpoch:            params.BeaconConfig().FarFutureEpoch,
				ActivationEligibilityEpoch: 6,
				ExitEpoch:                  params.BeaconConfig().FarFutureEpoch,
			},
			{
				PublicKey:                  pubKey3,
				ActivationEpoch:            0,
				ActivationEligibilityEpoch: 0,
				ExitEpoch:                  0,
			},
		},
	}
	block := util.NewBeaconBlockCapella()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	s, err := state_native.InitializeFromProtoUnsafeCapella(beaconState)
	require.NoError(t, err)
	vs := &Server{
		Ctx:               context.Background(),
		ChainStartFetcher: &mockExecution.Chain{},
		HeadFetcher:       &mockChain.ChainService{State: s, Root: genesisRoot[:]},
	}
	req := &zondpb.ValidatorActivationRequest{
		PublicKeys: [][]byte{pubKey1, pubKey2, pubKey3},
	}
	ctrl := gomock.NewController(t)

	defer ctrl.Finish()
	mockChainStream := mock.NewMockBeaconNodeValidator_WaitForActivationServer(ctrl)
	mockChainStream.EXPECT().Context().Return(context.Background())
	mockChainStream.EXPECT().Send(
		&zondpb.ValidatorActivationResponse{
			Statuses: []*zondpb.ValidatorActivationResponse_Status{
				{
					PublicKey: pubKey1,
					Status: &zondpb.ValidatorStatusResponse{
						Status:          zondpb.ValidatorStatus_ACTIVE,
						ActivationEpoch: 1,
					},
					Index: 0,
				},
				{
					PublicKey: pubKey2,
					Status: &zondpb.ValidatorStatusResponse{
						Status:                    zondpb.ValidatorStatus_PENDING,
						ActivationEpoch:           params.BeaconConfig().FarFutureEpoch,
						PositionInActivationQueue: 1,
					},
					Index: 1,
				},
				{
					PublicKey: pubKey3,
					Status: &zondpb.ValidatorStatusResponse{
						Status: zondpb.ValidatorStatus_EXITED,
					},
					Index: 2,
				},
			},
		},
	).Return(nil)

	require.NoError(t, vs.WaitForActivation(req, mockChainStream), "Could not setup wait for activation stream")
}

func TestWaitForChainStart_ContextClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	chainService := &mockChain.ChainService{}
	server := &Server{
		Ctx: ctx,
		ChainStartFetcher: &mockExecution.FaultyExecutionChain{
			ChainFeed: new(event.Feed),
		},
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
		ClockWaiter:   startup.NewClockSynchronizer(),
	}

	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_WaitForChainStartServer(ctrl)
	mockStream.EXPECT().Context().Return(ctx)
	go func(tt *testing.T) {
		err := server.WaitForChainStart(&emptypb.Empty{}, mockStream)
		assert.ErrorContains(tt, "Context canceled", err)
		<-exitRoutine
	}(t)
	cancel()
	exitRoutine <- true
}

func TestWaitForChainStart_AlreadyStarted(t *testing.T) {
	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(3))
	genesisValidatorsRoot := bytesutil.ToBytes32([]byte("validators"))
	require.NoError(t, st.SetGenesisValidatorsRoot(genesisValidatorsRoot[:]))

	chainService := &mockChain.ChainService{State: st, ValidatorsRoot: genesisValidatorsRoot}
	Server := &Server{
		Ctx: context.Background(),
		ChainStartFetcher: &mockExecution.Chain{
			ChainFeed: new(event.Feed),
		},
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_WaitForChainStartServer(ctrl)
	mockStream.EXPECT().Send(
		&zondpb.ChainStartResponse{
			Started:               true,
			GenesisTime:           uint64(time.Unix(0, 0).Unix()),
			GenesisValidatorsRoot: genesisValidatorsRoot[:],
		},
	).Return(nil)
	mockStream.EXPECT().Context().Return(context.Background())
	assert.NoError(t, Server.WaitForChainStart(&emptypb.Empty{}, mockStream), "Could not call RPC method")
}

func TestWaitForChainStart_HeadStateDoesNotExist(t *testing.T) {
	// Set head state to nil
	chainService := &mockChain.ChainService{State: nil}
	gs := startup.NewClockSynchronizer()
	Server := &Server{
		Ctx: context.Background(),
		ChainStartFetcher: &mockExecution.Chain{
			ChainFeed: new(event.Feed),
		},
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
		ClockWaiter:   gs,
	}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_WaitForChainStartServer(ctrl)
	mockStream.EXPECT().Context().Return(context.Background())

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		assert.NoError(t, Server.WaitForChainStart(&emptypb.Empty{}, mockStream), "Could not call RPC method")
		wg.Done()
	}()

	util.WaitTimeout(wg, time.Second)
}

func TestWaitForChainStart_NotStartedThenLogFired(t *testing.T) {
	hook := logTest.NewGlobal()

	genesisValidatorsRoot := bytesutil.ToBytes32([]byte("validators"))
	chainService := &mockChain.ChainService{}
	gs := startup.NewClockSynchronizer()

	Server := &Server{
		Ctx: context.Background(),
		ChainStartFetcher: &mockExecution.FaultyExecutionChain{
			ChainFeed: new(event.Feed),
		},
		StateNotifier: chainService.StateNotifier(),
		HeadFetcher:   chainService,
		ClockWaiter:   gs,
	}
	exitRoutine := make(chan bool)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStream := mock.NewMockBeaconNodeValidator_WaitForChainStartServer(ctrl)
	mockStream.EXPECT().Send(
		&zondpb.ChainStartResponse{
			Started:               true,
			GenesisTime:           uint64(time.Unix(0, 0).Unix()),
			GenesisValidatorsRoot: genesisValidatorsRoot[:],
		},
	).Return(nil)
	mockStream.EXPECT().Context().Return(context.Background())
	go func(tt *testing.T) {
		assert.NoError(tt, Server.WaitForChainStart(&emptypb.Empty{}, mockStream))
		<-exitRoutine
	}(t)

	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	require.NoError(t, gs.SetClock(startup.NewClock(time.Unix(0, 0), genesisValidatorsRoot)))

	exitRoutine <- true
	require.LogsContain(t, hook, "Sending genesis time")
}

func TestServer_DomainData_Exits(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ForkVersionSchedule = map[[4]byte]primitives.Epoch{
		[4]byte(cfg.GenesisForkVersion): primitives.Epoch(0),
	}

	params.OverrideBeaconConfig(cfg)
	beaconState := &zondpb.BeaconStateCapella{
		Slot: 4000,
	}
	block := util.NewBeaconBlockCapella()
	genesisRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	s, err := state_native.InitializeFromProtoUnsafeCapella(beaconState)
	require.NoError(t, err)
	vs := &Server{
		Ctx:               context.Background(),
		ChainStartFetcher: &mockExecution.Chain{},
		HeadFetcher:       &mockChain.ChainService{State: s, Root: genesisRoot[:]},
	}

	reqDomain, err := vs.DomainData(context.Background(), &zondpb.DomainRequest{
		Epoch:  100,
		Domain: params.BeaconConfig().DomainDeposit[:],
	})
	assert.NoError(t, err)
	wantedDomain, err := signing.ComputeDomain(params.BeaconConfig().DomainDeposit, params.BeaconConfig().GenesisForkVersion, make([]byte, 32))
	assert.NoError(t, err)
	assert.DeepEqual(t, reqDomain.SignatureDomain, wantedDomain)

	beaconStateNew := &zondpb.BeaconStateCapella{
		Slot: 4000,
	}
	s, err = state_native.InitializeFromProtoUnsafeCapella(beaconStateNew)
	require.NoError(t, err)
	vs.HeadFetcher = &mockChain.ChainService{State: s, Root: genesisRoot[:]}

	reqDomain, err = vs.DomainData(context.Background(), &zondpb.DomainRequest{
		Epoch:  100,
		Domain: params.BeaconConfig().DomainVoluntaryExit[:],
	})
	require.NoError(t, err)

	wantedDomain, err = signing.ComputeDomain(params.BeaconConfig().DomainVoluntaryExit, params.BeaconConfig().GenesisForkVersion, make([]byte, 32))
	require.NoError(t, err)

	assert.DeepEqual(t, reqDomain.SignatureDomain, wantedDomain)
}
