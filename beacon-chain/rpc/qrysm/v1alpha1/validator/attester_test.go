package validator

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	mock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/cache"
	dbutil "github.com/theQRL/qrysm/v4/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/v4/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/v4/beacon-chain/operations/attestations"
	mockp2p "github.com/theQRL/qrysm/v4/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/core"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/v4/beacon-chain/state/stategen"
	mockSync "github.com/theQRL/qrysm/v4/beacon-chain/sync/initial-sync/testing"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	qrysmTime "github.com/theQRL/qrysm/v4/time"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestProposeAttestation_OK(t *testing.T) {
	attesterServer := &Server{
		HeadFetcher:       &mock.ChainService{},
		P2P:               &mockp2p.MockBroadcaster{},
		AttPool:           attestations.NewPool(),
		OperationNotifier: (&mock.ChainService{}).OperationNotifier(),
	}
	head := util.NewBeaconBlockCapella()
	head.Block.Slot = 999
	head.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	root, err := head.Block.HashTreeRoot()
	require.NoError(t, err)

	validators := make([]*zondpb.Validator, 64)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			PublicKey:             make([]byte, 48),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
		}
	}

	state, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, state.SetSlot(params.BeaconConfig().SlotsPerEpoch+1))
	require.NoError(t, state.SetValidators(validators))

	sk, err := dilithium.RandKey()
	require.NoError(t, err)
	sig := sk.Sign([]byte("dummy_test_data"))
	req := &zondpb.Attestation{
		Signatures: [][]byte{sig.Marshal()},
		Data: &zondpb.AttestationData{
			BeaconBlockRoot: root[:],
			Source:          &zondpb.Checkpoint{Root: make([]byte, 32)},
			Target:          &zondpb.Checkpoint{Root: make([]byte, 32)},
		},
	}
	_, err = attesterServer.ProposeAttestation(context.Background(), req)
	assert.NoError(t, err)
}

func TestProposeAttestation_IncorrectSignature(t *testing.T) {
	attesterServer := &Server{
		HeadFetcher:       &mock.ChainService{},
		P2P:               &mockp2p.MockBroadcaster{},
		AttPool:           attestations.NewPool(),
		OperationNotifier: (&mock.ChainService{}).OperationNotifier(),
	}

	req := util.HydrateAttestation(&zondpb.Attestation{Signatures: [][]byte{make([]byte, 999)}})
	wanted := "Incorrect attestation signature"
	_, err := attesterServer.ProposeAttestation(context.Background(), req)
	assert.ErrorContains(t, wanted, err)
}

func TestGetAttestationData_OK(t *testing.T) {
	block := util.NewBeaconBlockCapella()
	block.Block.Slot = 3*params.BeaconConfig().SlotsPerEpoch + 1
	targetBlock := util.NewBeaconBlockCapella()
	targetBlock.Block.Slot = 1 * params.BeaconConfig().SlotsPerEpoch
	justifiedBlock := util.NewBeaconBlockCapella()
	justifiedBlock.Block.Slot = 2 * params.BeaconConfig().SlotsPerEpoch
	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not hash beacon block")
	justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for justified block")
	targetRoot, err := targetBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for target block")
	slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
	beaconState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(slot))
	err = beaconState.SetCurrentJustifiedCheckpoint(&zondpb.Checkpoint{
		Epoch: 2,
		Root:  justifiedRoot[:],
	})
	require.NoError(t, err)

	blockRoots := beaconState.BlockRoots()
	blockRoots[1] = blockRoot[:]
	blockRoots[1*params.BeaconConfig().SlotsPerEpoch] = targetRoot[:]
	blockRoots[2*params.BeaconConfig().SlotsPerEpoch] = justifiedRoot[:]
	require.NoError(t, beaconState.SetBlockRoots(blockRoots))
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			AttestationCache: cache.NewAttestationCache(),
			HeadFetcher: &mock.ChainService{
				State: beaconState, Root: blockRoot[:],
			},
			GenesisTimeFetcher: &mock.ChainService{
				Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second),
			},
		},
	}

	req := &zondpb.AttestationDataRequest{
		CommitteeIndex: 0,
		Slot:           3*params.BeaconConfig().SlotsPerEpoch + 1,
	}
	res, err := attesterServer.GetAttestationData(context.Background(), req)
	require.NoError(t, err, "Could not get attestation info at slot")

	expectedInfo := &zondpb.AttestationData{
		Slot:            3*params.BeaconConfig().SlotsPerEpoch + 1,
		BeaconBlockRoot: blockRoot[:],
		Source: &zondpb.Checkpoint{
			Epoch: 2,
			Root:  justifiedRoot[:],
		},
		Target: &zondpb.Checkpoint{
			Epoch: 3,
			Root:  blockRoot[:],
		},
	}

	if !proto.Equal(res, expectedInfo) {
		t.Errorf("Expected attestation info to match, received %v, wanted %v", res, expectedInfo)
	}
}

func TestGetAttestationData_SyncNotReady(t *testing.T) {
	as := Server{
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}
	_, err := as.GetAttestationData(context.Background(), &zondpb.AttestationDataRequest{})
	assert.ErrorContains(t, "Syncing to latest head", err)
}

func TestGetAttestationData_Optimistic(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	as := &Server{
		SyncChecker: &mockSync.Sync{},
		TimeFetcher: &mock.ChainService{Genesis: time.Now()},
		CoreService: &core.Service{
			GenesisTimeFetcher: &mock.ChainService{Genesis: time.Now()},
			HeadFetcher:        &mock.ChainService{},
		},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: true},
	}
	_, err := as.GetAttestationData(context.Background(), &zondpb.AttestationDataRequest{})
	s, ok := status.FromError(err)
	require.Equal(t, true, ok)
	require.DeepEqual(t, codes.Unavailable, s.Code())
	require.ErrorContains(t, errOptimisticMode.Error(), err)

	beaconState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	as = &Server{
		SyncChecker:           &mockSync.Sync{},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		CoreService: &core.Service{
			GenesisTimeFetcher: &mock.ChainService{Genesis: time.Now()},
			HeadFetcher:        &mock.ChainService{Optimistic: false, State: beaconState},
			AttestationCache:   cache.NewAttestationCache(),
		},
	}
	_, err = as.GetAttestationData(context.Background(), &zondpb.AttestationDataRequest{})
	require.NoError(t, err)
}

func TestAttestationDataSlot_handlesInProgressRequest(t *testing.T) {
	s := &zondpb.BeaconStateCapella{Slot: 100}
	state, err := state_native.InitializeFromProtoCapella(s)
	require.NoError(t, err)
	ctx := context.Background()
	slot := primitives.Slot(2)
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	server := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			AttestationCache:   cache.NewAttestationCache(),
			HeadFetcher:        &mock.ChainService{State: state},
			GenesisTimeFetcher: &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		},
	}

	req := &zondpb.AttestationDataRequest{
		CommitteeIndex: 1,
		Slot:           slot,
	}

	res := &zondpb.AttestationData{
		CommitteeIndex: 1,
		Target:         &zondpb.Checkpoint{Epoch: 55, Root: make([]byte, 32)},
	}

	require.NoError(t, server.CoreService.AttestationCache.MarkInProgress(req))

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		response, err := server.GetAttestationData(ctx, req)
		require.NoError(t, err)
		if !proto.Equal(res, response) {
			t.Error("Expected  equal responses from cache")
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		assert.NoError(t, server.CoreService.AttestationCache.Put(ctx, req, res))
		assert.NoError(t, server.CoreService.AttestationCache.MarkNotInProgress(req))
	}()

	wg.Wait()
}

func TestServer_GetAttestationData_InvalidRequestSlot(t *testing.T) {
	ctx := context.Background()

	slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			GenesisTimeFetcher: &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		},
	}

	req := &zondpb.AttestationDataRequest{
		Slot: 1000000000000,
	}
	_, err := attesterServer.GetAttestationData(ctx, req)
	assert.ErrorContains(t, "invalid request", err)
}

func TestServer_GetAttestationData_HeadStateSlotGreaterThanRequestSlot(t *testing.T) {
	// There exists a rare scenario where the validator may request an attestation for a slot less
	// than the head state's slot. The Ethereum consensus spec constraints require the block root the
	// attestation is referencing be less than or equal to the attestation data slot.
	// See: https://github.com/theQRL/qrysm/issues/5164
	ctx := context.Background()
	db := dbutil.SetupDB(t)

	slot := 3*params.BeaconConfig().SlotsPerEpoch + 1
	block := util.NewBeaconBlockCapella()
	block.Block.Slot = slot
	block2 := util.NewBeaconBlockCapella()
	block2.Block.Slot = slot - 1
	targetBlock := util.NewBeaconBlockCapella()
	targetBlock.Block.Slot = 1 * params.BeaconConfig().SlotsPerEpoch
	justifiedBlock := util.NewBeaconBlockCapella()
	justifiedBlock.Block.Slot = 2 * params.BeaconConfig().SlotsPerEpoch
	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not hash beacon block")
	blockRoot2, err := block2.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, db, block2)
	justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for justified block")
	targetRoot, err := targetBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for target block")

	beaconState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(slot))
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix()-offset)))
	err = beaconState.SetLatestBlockHeader(util.HydrateBeaconHeader(&zondpb.BeaconBlockHeader{
		ParentRoot: blockRoot2[:],
	}))
	require.NoError(t, err)
	err = beaconState.SetCurrentJustifiedCheckpoint(&zondpb.Checkpoint{
		Epoch: 2,
		Root:  justifiedRoot[:],
	})
	require.NoError(t, err)
	blockRoots := beaconState.BlockRoots()
	blockRoots[1] = blockRoot[:]
	blockRoots[1*params.BeaconConfig().SlotsPerEpoch] = targetRoot[:]
	blockRoots[2*params.BeaconConfig().SlotsPerEpoch] = justifiedRoot[:]
	blockRoots[3*params.BeaconConfig().SlotsPerEpoch] = blockRoot2[:]
	require.NoError(t, beaconState.SetBlockRoots(blockRoots))

	beaconstate := beaconState.Copy()
	require.NoError(t, beaconstate.SetSlot(beaconstate.Slot()-1))
	require.NoError(t, db.SaveState(ctx, beaconstate, blockRoot2))
	offset = int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			AttestationCache:   cache.NewAttestationCache(),
			HeadFetcher:        &mock.ChainService{State: beaconState, Root: blockRoot[:]},
			GenesisTimeFetcher: &mock.ChainService{Genesis: time.Now().Add(time.Duration(-1*offset) * time.Second)},
			StateGen:           stategen.New(db, doublylinkedtree.New()),
		},
	}
	require.NoError(t, db.SaveState(ctx, beaconState, blockRoot))
	util.SaveBlock(t, ctx, db, block)
	require.NoError(t, db.SaveHeadBlockRoot(ctx, blockRoot))

	req := &zondpb.AttestationDataRequest{
		CommitteeIndex: 0,
		Slot:           slot - 1,
	}
	res, err := attesterServer.GetAttestationData(ctx, req)
	require.NoError(t, err, "Could not get attestation info at slot")

	expectedInfo := &zondpb.AttestationData{
		Slot:            slot - 1,
		BeaconBlockRoot: blockRoot2[:],
		Source: &zondpb.Checkpoint{
			Epoch: 2,
			Root:  justifiedRoot[:],
		},
		Target: &zondpb.Checkpoint{
			Epoch: 3,
			Root:  blockRoot2[:],
		},
	}

	if !proto.Equal(res, expectedInfo) {
		t.Errorf("Expected attestation info to match, received %v, wanted %v", res, expectedInfo)
	}
}

func TestGetAttestationData_SucceedsInFirstEpoch(t *testing.T) {
	slot := primitives.Slot(5)
	block := util.NewBeaconBlockCapella()
	block.Block.Slot = slot
	targetBlock := util.NewBeaconBlockCapella()
	targetBlock.Block.Slot = 0
	justifiedBlock := util.NewBeaconBlockCapella()
	justifiedBlock.Block.Slot = 0
	blockRoot, err := block.Block.HashTreeRoot()
	require.NoError(t, err, "Could not hash beacon block")
	justifiedRoot, err := justifiedBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for justified block")
	targetRoot, err := targetBlock.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root for target block")

	beaconState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(slot))
	err = beaconState.SetCurrentJustifiedCheckpoint(&zondpb.Checkpoint{
		Epoch: 0,
		Root:  justifiedRoot[:],
	})
	require.NoError(t, err)
	blockRoots := beaconState.BlockRoots()
	blockRoots[1] = blockRoot[:]
	blockRoots[1*params.BeaconConfig().SlotsPerEpoch] = targetRoot[:]
	blockRoots[2*params.BeaconConfig().SlotsPerEpoch] = justifiedRoot[:]
	require.NoError(t, beaconState.SetBlockRoots(blockRoots))
	offset := int64(slot.Mul(params.BeaconConfig().SecondsPerSlot))
	attesterServer := &Server{
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
		TimeFetcher:           &mock.ChainService{Genesis: qrysmTime.Now().Add(time.Duration(-1*offset) * time.Second)},
		CoreService: &core.Service{
			AttestationCache: cache.NewAttestationCache(),
			HeadFetcher: &mock.ChainService{
				State: beaconState, Root: blockRoot[:],
			},
			GenesisTimeFetcher: &mock.ChainService{Genesis: qrysmTime.Now().Add(time.Duration(-1*offset) * time.Second)},
		},
	}

	req := &zondpb.AttestationDataRequest{
		CommitteeIndex: 0,
		Slot:           5,
	}
	res, err := attesterServer.GetAttestationData(context.Background(), req)
	require.NoError(t, err, "Could not get attestation info at slot")

	expectedInfo := &zondpb.AttestationData{
		Slot:            slot,
		BeaconBlockRoot: blockRoot[:],
		Source: &zondpb.Checkpoint{
			Epoch: 0,
			Root:  justifiedRoot[:],
		},
		Target: &zondpb.Checkpoint{
			Epoch: 0,
			Root:  blockRoot[:],
		},
	}

	if !proto.Equal(res, expectedInfo) {
		t.Errorf("Expected attestation info to match, received %v, wanted %v", res, expectedInfo)
	}
}

func TestServer_SubscribeCommitteeSubnets_NoSlots(t *testing.T) {
	attesterServer := &Server{
		HeadFetcher:       &mock.ChainService{},
		P2P:               &mockp2p.MockBroadcaster{},
		AttPool:           attestations.NewPool(),
		OperationNotifier: (&mock.ChainService{}).OperationNotifier(),
	}

	_, err := attesterServer.SubscribeCommitteeSubnets(context.Background(), &zondpb.CommitteeSubnetsSubscribeRequest{
		Slots:        nil,
		CommitteeIds: nil,
		IsAggregator: nil,
	})
	assert.ErrorContains(t, "no attester slots provided", err)
}

func TestServer_SubscribeCommitteeSubnets_DifferentLengthSlots(t *testing.T) {
	// fixed seed
	s := rand.NewSource(10)
	randGen := rand.New(s)

	attesterServer := &Server{
		HeadFetcher:       &mock.ChainService{},
		P2P:               &mockp2p.MockBroadcaster{},
		AttPool:           attestations.NewPool(),
		OperationNotifier: (&mock.ChainService{}).OperationNotifier(),
	}

	var ss []primitives.Slot
	var comIdxs []primitives.CommitteeIndex
	var isAggregator []bool

	for i := primitives.Slot(100); i < 200; i++ {
		ss = append(ss, i)
		comIdxs = append(comIdxs, primitives.CommitteeIndex(randGen.Int63n(64)))
		boolVal := randGen.Uint64()%2 == 0
		isAggregator = append(isAggregator, boolVal)
	}

	ss = append(ss, 321)

	_, err := attesterServer.SubscribeCommitteeSubnets(context.Background(), &zondpb.CommitteeSubnetsSubscribeRequest{
		Slots:        ss,
		CommitteeIds: comIdxs,
		IsAggregator: isAggregator,
	})
	assert.ErrorContains(t, "request fields are not the same length", err)
}

func TestServer_SubscribeCommitteeSubnets_MultipleSlots(t *testing.T) {
	// fixed seed
	s := rand.NewSource(10)
	randGen := rand.New(s)

	validators := make([]*zondpb.Validator, 64)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		}
	}

	state, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))

	attesterServer := &Server{
		HeadFetcher:       &mock.ChainService{State: state},
		P2P:               &mockp2p.MockBroadcaster{},
		AttPool:           attestations.NewPool(),
		OperationNotifier: (&mock.ChainService{}).OperationNotifier(),
	}

	var ss []primitives.Slot
	var comIdxs []primitives.CommitteeIndex
	var isAggregator []bool

	for i := primitives.Slot(100); i < 200; i++ {
		ss = append(ss, i)
		comIdxs = append(comIdxs, primitives.CommitteeIndex(randGen.Int63n(64)))
		boolVal := randGen.Uint64()%2 == 0
		isAggregator = append(isAggregator, boolVal)
	}

	_, err = attesterServer.SubscribeCommitteeSubnets(context.Background(), &zondpb.CommitteeSubnetsSubscribeRequest{
		Slots:        ss,
		CommitteeIds: comIdxs,
		IsAggregator: isAggregator,
	})
	require.NoError(t, err)
	for i := primitives.Slot(100); i < 200; i++ {
		subnets := cache.SubnetIDs.GetAttesterSubnetIDs(i)
		assert.Equal(t, 1, len(subnets))
		if isAggregator[i-100] {
			subnets = cache.SubnetIDs.GetAggregatorSubnetIDs(i)
			assert.Equal(t, 1, len(subnets))
		}
	}
}
