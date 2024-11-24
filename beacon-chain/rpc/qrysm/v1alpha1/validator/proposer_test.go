package validator

import (
	"context"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/common/hexutil"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/builder"
	builderTest "github.com/theQRL/qrysm/beacon-chain/builder/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/cache/depositcache"
	b "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	coretime "github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/db"
	dbutil "github.com/theQRL/qrysm/beacon-chain/db/testing"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/beacon-chain/operations/dilithiumtoexec"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/operations/synccommittee"
	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	mockp2p "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/beacon-chain/rpc/testutil"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/container/trie"
	"github.com/theQRL/qrysm/crypto/dilithium"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/encoding/ssz"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	attaggregation "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestServer_GetBeaconBlock_Capella(t *testing.T) {
	db := dbutil.SetupDB(t)
	ctx := context.Background()
	transition.SkipSlotCache.Disable()

	params.SetupTestConfigCleanup(t)
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 64)

	stateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")

	genesis := b.NewGenesisBlock(stateRoot[:])
	util.SaveBlock(t, ctx, db, genesis)

	parentRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, db.SaveState(ctx, beaconState, parentRoot), "Could not save genesis state")
	require.NoError(t, db.SaveHeadBlockRoot(ctx, parentRoot), "Could not save genesis state")

	// NOTE(rgeraldes24) the slot must be > fieldparams.SlotsPerEpoch
	capellaSlot := primitives.Slot(fieldparams.SlotsPerEpoch + 1)

	var scBits [fieldparams.SyncAggregateSyncCommitteeBytesLength]byte
	blk := &zondpb.SignedBeaconBlockCapella{
		Block: &zondpb.BeaconBlockCapella{
			Slot:       capellaSlot + 1,
			ParentRoot: parentRoot[:],
			StateRoot:  genesis.Block.StateRoot,
			Body: &zondpb.BeaconBlockBodyCapella{
				RandaoReveal:  genesis.Block.Body.RandaoReveal,
				Graffiti:      genesis.Block.Body.Graffiti,
				Eth1Data:      genesis.Block.Body.Eth1Data,
				SyncAggregate: &zondpb.SyncAggregate{SyncCommitteeBits: scBits[:], SyncCommitteeSignatures: [][]byte{}},
				ExecutionPayload: &enginev1.ExecutionPayloadCapella{
					ParentHash:    make([]byte, fieldparams.RootLength),
					FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:     make([]byte, fieldparams.RootLength),
					ReceiptsRoot:  make([]byte, fieldparams.RootLength),
					LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:    make([]byte, fieldparams.RootLength),
					BaseFeePerGas: make([]byte, fieldparams.RootLength),
					BlockHash:     make([]byte, fieldparams.RootLength),
				},
			},
		},
		Signature: genesis.Signature,
	}

	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, db.SaveState(ctx, beaconState, blkRoot), "Could not save genesis state")
	require.NoError(t, db.SaveHeadBlockRoot(ctx, blkRoot), "Could not save genesis state")

	random, err := helpers.RandaoMix(beaconState, slots.ToEpoch(beaconState.Slot()))
	require.NoError(t, err)
	timeStamp, err := slots.ToTime(beaconState.GenesisTime(), capellaSlot+1)
	require.NoError(t, err)
	payload := &enginev1.ExecutionPayloadCapella{
		ParentHash:    make([]byte, fieldparams.RootLength),
		FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
		StateRoot:     make([]byte, fieldparams.RootLength),
		ReceiptsRoot:  make([]byte, fieldparams.RootLength),
		LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
		PrevRandao:    random,
		BaseFeePerGas: make([]byte, fieldparams.RootLength),
		BlockHash:     make([]byte, fieldparams.RootLength),
		Transactions:  make([][]byte, 0),
		ExtraData:     make([]byte, 0),
		BlockNumber:   1,
		GasLimit:      2,
		GasUsed:       3,
		Timestamp:     uint64(timeStamp.Unix()),
	}

	proposerServer := getProposerServer(db, beaconState, parentRoot[:])
	proposerServer.ExecutionEngineCaller = &mockExecution.EngineClient{
		PayloadIDBytes:          &enginev1.PayloadIDBytes{1},
		ExecutionPayloadCapella: payload,
	}

	randaoReveal, err := util.RandaoReveal(beaconState, 0, privKeys)
	require.NoError(t, err)

	graffiti := bytesutil.ToBytes32([]byte("eth2"))
	require.NoError(t, err)
	req := &zondpb.BlockRequest{
		Slot:         capellaSlot + 1,
		RandaoReveal: randaoReveal,
		Graffiti:     graffiti[:],
	}

	copiedState := beaconState.Copy()
	copiedState, err = transition.ProcessSlots(ctx, copiedState, capellaSlot+1)
	require.NoError(t, err)
	change, err := util.GenerateDilithiumToExecutionChange(copiedState, privKeys[1], 0)
	require.NoError(t, err)
	proposerServer.DilithiumChangesPool.InsertDilithiumToExecChange(change)

	got, err := proposerServer.GetBeaconBlock(ctx, req)
	require.NoError(t, err)
	require.Equal(t, 1, len(got.GetCapella().Body.DilithiumToExecutionChanges))
	require.DeepEqual(t, change, got.GetCapella().Body.DilithiumToExecutionChanges[0])
}

func TestServer_GetBeaconBlock_Optimistic(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	capellaSlot := primitives.Slot(0)

	mockChainService := &mock.ChainService{ForkChoiceStore: doublylinkedtree.New()}
	proposerServer := &Server{
		OptimisticModeFetcher: &mock.ChainService{Optimistic: true},
		SyncChecker:           &mockSync.Sync{},
		ForkFetcher:           mockChainService,
		ForkchoiceFetcher:     mockChainService,
		TimeFetcher:           &mock.ChainService{}}
	req := &zondpb.BlockRequest{
		Slot: capellaSlot + 1,
	}
	_, err := proposerServer.GetBeaconBlock(context.Background(), req)
	s, ok := status.FromError(err)
	require.Equal(t, true, ok)
	require.DeepEqual(t, codes.Unavailable, s.Code())
	require.ErrorContains(t, errOptimisticMode.Error(), err)
}

func getProposerServer(db db.HeadAccessDatabase, headState state.BeaconState, headRoot []byte) *Server {
	mockChainService := &mock.ChainService{State: headState, Root: headRoot, ForkChoiceStore: doublylinkedtree.New()}
	return &Server{
		HeadFetcher:           mockChainService,
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		BlockReceiver:         mockChainService,
		ChainStartFetcher:     &mockExecution.Chain{},
		Eth1InfoFetcher:       &mockExecution.Chain{},
		Eth1BlockFetcher:      &mockExecution.Chain{},
		FinalizationFetcher:   mockChainService,
		ForkFetcher:           mockChainService,
		ForkchoiceFetcher:     mockChainService,
		MockEth1Votes:         true,
		AttPool:               attestations.NewPool(),
		SlashingsPool:         slashings.NewPool(),
		ExitPool:              voluntaryexits.NewPool(),
		StateGen:              stategen.New(db, doublylinkedtree.New()),
		SyncCommitteePool:     synccommittee.NewStore(),
		OptimisticModeFetcher: &mock.ChainService{},
		TimeFetcher: &testutil.MockGenesisTimeFetcher{
			Genesis: time.Now(),
		},
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
		BeaconDB:               db,
		DilithiumChangesPool:   dilithiumtoexec.NewPool(),
		BlockBuilder:           &builderTest.MockBuilderService{HasConfigured: true},
	}
}

func injectSlashings(t *testing.T, st state.BeaconState, keys []dilithium.DilithiumKey, server *Server) ([]*zondpb.ProposerSlashing, []*zondpb.AttesterSlashing) {
	proposerSlashings := make([]*zondpb.ProposerSlashing, params.BeaconConfig().MaxProposerSlashings)
	for i := primitives.ValidatorIndex(0); uint64(i) < params.BeaconConfig().MaxProposerSlashings; i++ {
		proposerSlashing, err := util.GenerateProposerSlashingForValidator(st, keys[i], i /* validator index */)
		require.NoError(t, err)
		proposerSlashings[i] = proposerSlashing
		err = server.SlashingsPool.InsertProposerSlashing(context.Background(), st, proposerSlashing)
		require.NoError(t, err)
	}

	attSlashings := make([]*zondpb.AttesterSlashing, params.BeaconConfig().MaxAttesterSlashings)
	for i := uint64(0); i < params.BeaconConfig().MaxAttesterSlashings; i++ {
		attesterSlashing, err := util.GenerateAttesterSlashingForValidator(st, keys[i+params.BeaconConfig().MaxProposerSlashings], primitives.ValidatorIndex(i+params.BeaconConfig().MaxProposerSlashings) /* validator index */)
		require.NoError(t, err)
		attSlashings[i] = attesterSlashing
		err = server.SlashingsPool.InsertAttesterSlashing(context.Background(), st, attesterSlashing)
		require.NoError(t, err)
	}
	return proposerSlashings, attSlashings
}

func TestProposer_ProposeBlock_OK(t *testing.T) {
	tests := []struct {
		name  string
		block func([32]byte) *zondpb.GenericSignedBeaconBlock
		err   string
	}{

		{
			name: "blind capella",
			block: func(parent [32]byte) *zondpb.GenericSignedBeaconBlock {
				blockToPropose := util.NewBlindedBeaconBlockCapella()
				blockToPropose.Block.Slot = 5
				blockToPropose.Block.ParentRoot = parent[:]
				txRoot, err := ssz.TransactionsRoot([][]byte{})
				require.NoError(t, err)
				withdrawalsRoot, err := ssz.WithdrawalSliceRoot([]*enginev1.Withdrawal{}, fieldparams.MaxWithdrawalsPerPayload)
				require.NoError(t, err)
				blockToPropose.Block.Body.ExecutionPayloadHeader.TransactionsRoot = txRoot[:]
				blockToPropose.Block.Body.ExecutionPayloadHeader.WithdrawalsRoot = withdrawalsRoot[:]
				blk := &zondpb.GenericSignedBeaconBlock_BlindedCapella{BlindedCapella: blockToPropose}
				return &zondpb.GenericSignedBeaconBlock{Block: blk}
			},
		},
		{
			name: "capella",
			block: func(parent [32]byte) *zondpb.GenericSignedBeaconBlock {
				blockToPropose := util.NewBeaconBlockCapella()
				blockToPropose.Block.Slot = 5
				blockToPropose.Block.ParentRoot = parent[:]
				blk := &zondpb.GenericSignedBeaconBlock_Capella{Capella: blockToPropose}
				return &zondpb.GenericSignedBeaconBlock{Block: blk}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			numDeposits := uint64(64)
			beaconState, _ := util.DeterministicGenesisStateCapella(t, numDeposits)
			bsRoot, err := beaconState.HashTreeRoot(ctx)
			require.NoError(t, err)

			c := &mock.ChainService{Root: bsRoot[:], State: beaconState}
			db := dbutil.SetupDB(t)
			proposerServer := &Server{
				BlockReceiver: c,
				BlockNotifier: c.BlockNotifier(),
				P2P:           mockp2p.NewTestP2P(t),
				BlockBuilder:  &builderTest.MockBuilderService{HasConfigured: true, PayloadCapella: emptyPayloadCapella()},
				BeaconDB:      db,
			}
			blockToPropose := tt.block(bsRoot)
			res, err := proposerServer.ProposeBeaconBlock(context.Background(), blockToPropose)
			if tt.err != "" { // Expecting an error
				require.ErrorContains(t, tt.err, err)
			} else {
				assert.NoError(t, err, "Could not propose block correctly")
				if res == nil || len(res.BlockRoot) == 0 {
					t.Error("No block root was returned")
				}
			}
		})
	}
}

func TestProposer_ComputeStateRoot_OK(t *testing.T) {
	db := dbutil.SetupDB(t)
	ctx := context.Background()

	beaconState, parentRoot, privKeys := util.DeterministicGenesisStateCapellaWithGenesisBlock(t, ctx, db, 100)

	proposerServer := &Server{
		ChainStartFetcher: &mockExecution.Chain{},
		Eth1InfoFetcher:   &mockExecution.Chain{},
		Eth1BlockFetcher:  &mockExecution.Chain{},
		StateGen:          stategen.New(db, doublylinkedtree.New()),
	}
	req := util.NewBeaconBlockCapella()
	req.Block.ProposerIndex = 18
	req.Block.ParentRoot = parentRoot[:]
	req.Block.Slot = 1
	require.NoError(t, beaconState.SetSlot(beaconState.Slot()+1))
	randaoReveal, err := util.RandaoReveal(beaconState, 0, privKeys)
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, beaconState)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(beaconState.Slot()-1))
	req.Block.Body.RandaoReveal = randaoReveal
	currentEpoch := coretime.CurrentEpoch(beaconState)
	req.Signature, err = signing.ComputeDomainAndSign(beaconState, currentEpoch, req.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	wsb, err := blocks.NewSignedBeaconBlock(req)
	require.NoError(t, err)
	_, err = proposerServer.computeStateRoot(context.Background(), wsb)
	require.NoError(t, err)
}

func TestProposer_PendingDeposits_Eth1DataVoteOK(t *testing.T) {
	ctx := context.Background()

	height := big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance))
	newHeight := big.NewInt(height.Int64() + 11000)
	p := &mockExecution.Chain{
		LatestBlockNumber: height,
		HashesByHeight: map[int][]byte{
			int(height.Int64()):    []byte("0x0"),
			int(newHeight.Int64()): []byte("0x1"),
		},
	}

	var votes []*zondpb.Eth1Data

	blockHash := make([]byte, 32)
	copy(blockHash, "0x1")
	vote := &zondpb.Eth1Data{
		DepositRoot:  make([]byte, 32),
		BlockHash:    blockHash,
		DepositCount: 3,
	}
	period := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerEth1VotingPeriod)))
	for i := 0; i <= int(period/2); i++ {
		votes = append(votes, vote)
	}

	blockHash = make([]byte, 32)
	copy(blockHash, "0x0")
	beaconState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetEth1DepositIndex(2))
	require.NoError(t, beaconState.SetEth1Data(&zondpb.Eth1Data{
		DepositRoot:  make([]byte, 32),
		BlockHash:    blockHash,
		DepositCount: 2,
	}))
	require.NoError(t, beaconState.SetEth1DataVotes(votes))

	blk := util.NewBeaconBlockCapella()
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	bs := &Server{
		ChainStartFetcher: p,
		Eth1InfoFetcher:   p,
		Eth1BlockFetcher:  p,
		BlockReceiver:     &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		HeadFetcher:       &mock.ChainService{State: beaconState, Root: blkRoot[:]},
	}

	// It should also return the recent deposits after their follow window.
	p.LatestBlockNumber = big.NewInt(0).Add(p.LatestBlockNumber, big.NewInt(10000))
	_, eth1Height, err := bs.canonicalEth1Data(ctx, beaconState, &zondpb.Eth1Data{})
	require.NoError(t, err)

	assert.Equal(t, 0, eth1Height.Cmp(height))

	newState, err := b.ProcessEth1DataInBlock(ctx, beaconState, blk.Block.Body.Eth1Data)
	require.NoError(t, err)

	if proto.Equal(newState.Eth1Data(), vote) {
		t.Errorf("eth1data in the state equal to vote, when not expected to"+
			"have majority: Got %v", vote)
	}

	blk.Block.Body.Eth1Data = vote

	_, eth1Height, err = bs.canonicalEth1Data(ctx, beaconState, vote)
	require.NoError(t, err)
	assert.Equal(t, 0, eth1Height.Cmp(newHeight))

	newState, err = b.ProcessEth1DataInBlock(ctx, beaconState, blk.Block.Body.Eth1Data)
	require.NoError(t, err)

	if !proto.Equal(newState.Eth1Data(), vote) {
		t.Errorf("eth1data in the state not of the expected kind: Got %v but wanted %v", newState.Eth1Data(), vote)
	}
}

func TestProposer_PendingDeposits_OutsideEth1FollowWindow(t *testing.T) {
	ctx := context.Background()

	height := big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance))
	p := &mockExecution.Chain{
		LatestBlockNumber: height,
		HashesByHeight: map[int][]byte{
			int(height.Int64()): []byte("0x0"),
		},
	}

	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Eth1Data: &zondpb.Eth1Data{
			BlockHash:   bytesutil.PadTo([]byte("0x0"), 32),
			DepositRoot: make([]byte, 32),
		},
		Eth1DepositIndex: 2,
	})
	require.NoError(t, err)

	var mockSig [field_params.DilithiumSignatureLength]byte
	var mockCreds [32]byte

	// Using the merkleTreeIndex as the block number for this test...
	readyDeposits := []*zondpb.DepositContainer{
		{
			Index:           0,
			Eth1BlockHeight: 2,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("a"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index:           1,
			Eth1BlockHeight: 8,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("b"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	recentDeposits := []*zondpb.DepositContainer{
		{
			Index:           2,
			Eth1BlockHeight: 400,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("c"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index:           3,
			Eth1BlockHeight: 600,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("d"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	depositCache, err := depositcache.New()
	require.NoError(t, err)

	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not setup deposit trie")
	for _, dp := range append(readyDeposits, recentDeposits...) {
		depositHash, err := dp.Deposit.Data.HashTreeRoot()
		require.NoError(t, err, "Unable to determine hashed value of deposit")

		assert.NoError(t, depositTrie.Insert(depositHash[:], int(dp.Index)))
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		assert.NoError(t, depositCache.InsertDeposit(ctx, dp.Deposit, dp.Eth1BlockHeight, dp.Index, root))
	}
	for _, dp := range recentDeposits {
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		depositCache.InsertPendingDeposit(ctx, dp.Deposit, dp.Eth1BlockHeight, dp.Index, root)
	}

	blk := util.NewBeaconBlockCapella()
	blk.Block.Slot = beaconState.Slot()

	blkRoot, err := blk.HashTreeRoot()
	require.NoError(t, err)

	bs := &Server{
		ChainStartFetcher:      p,
		Eth1InfoFetcher:        p,
		Eth1BlockFetcher:       p,
		DepositFetcher:         depositCache,
		PendingDepositsFetcher: depositCache,
		BlockReceiver:          &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		HeadFetcher:            &mock.ChainService{State: beaconState, Root: blkRoot[:]},
	}

	deposits, err := bs.deposits(ctx, beaconState, &zondpb.Eth1Data{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(deposits), "Received unexpected list of deposits")

	// It should not return the recent deposits after their follow window.
	// as latest block number makes no difference in retrieval of deposits
	p.LatestBlockNumber = big.NewInt(0).Add(p.LatestBlockNumber, big.NewInt(10000))
	deposits, err = bs.deposits(ctx, beaconState, &zondpb.Eth1Data{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(deposits), "Received unexpected number of pending deposits")
}

func TestProposer_PendingDeposits_FollowsCorrectEth1Block(t *testing.T) {
	ctx := context.Background()

	height := big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance))
	newHeight := big.NewInt(height.Int64() + 11000)
	p := &mockExecution.Chain{
		LatestBlockNumber: height,
		HashesByHeight: map[int][]byte{
			int(height.Int64()):    []byte("0x0"),
			int(newHeight.Int64()): []byte("0x1"),
		},
	}

	var votes []*zondpb.Eth1Data

	vote := &zondpb.Eth1Data{
		BlockHash:    bytesutil.PadTo([]byte("0x1"), 32),
		DepositRoot:  make([]byte, 32),
		DepositCount: 7,
	}
	period := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerEth1VotingPeriod)))
	for i := 0; i <= int(period/2); i++ {
		votes = append(votes, vote)
	}

	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Eth1Data: &zondpb.Eth1Data{
			BlockHash:    []byte("0x0"),
			DepositRoot:  make([]byte, 32),
			DepositCount: 5,
		},
		Eth1DepositIndex: 1,
		Eth1DataVotes:    votes,
	})
	require.NoError(t, err)
	blk := util.NewBeaconBlockCapella()
	blk.Block.Slot = beaconState.Slot()

	blkRoot, err := blk.HashTreeRoot()
	require.NoError(t, err)

	var mockSig [field_params.DilithiumSignatureLength]byte
	var mockCreds [32]byte

	// Using the merkleTreeIndex as the block number for this test...
	readyDeposits := []*zondpb.DepositContainer{
		{
			Index:           0,
			Eth1BlockHeight: 8,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("a"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index:           1,
			Eth1BlockHeight: 14,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("b"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	recentDeposits := []*zondpb.DepositContainer{
		{
			Index:           2,
			Eth1BlockHeight: 5000,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("c"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index:           3,
			Eth1BlockHeight: 6000,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("d"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	depositCache, err := depositcache.New()
	require.NoError(t, err)

	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not setup deposit trie")
	for _, dp := range append(readyDeposits, recentDeposits...) {
		depositHash, err := dp.Deposit.Data.HashTreeRoot()
		require.NoError(t, err, "Unable to determine hashed value of deposit")

		assert.NoError(t, depositTrie.Insert(depositHash[:], int(dp.Index)))
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		assert.NoError(t, depositCache.InsertDeposit(ctx, dp.Deposit, dp.Eth1BlockHeight, dp.Index, root))
	}
	for _, dp := range recentDeposits {
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		depositCache.InsertPendingDeposit(ctx, dp.Deposit, dp.Eth1BlockHeight, dp.Index, root)
	}

	bs := &Server{
		ChainStartFetcher:      p,
		Eth1InfoFetcher:        p,
		Eth1BlockFetcher:       p,
		DepositFetcher:         depositCache,
		PendingDepositsFetcher: depositCache,
		BlockReceiver:          &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		HeadFetcher:            &mock.ChainService{State: beaconState, Root: blkRoot[:]},
	}

	deposits, err := bs.deposits(ctx, beaconState, &zondpb.Eth1Data{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(deposits), "Received unexpected list of deposits")

	// It should also return the recent deposits after their follow window.
	p.LatestBlockNumber = big.NewInt(0).Add(p.LatestBlockNumber, big.NewInt(10000))
	// we should get our pending deposits once this vote pushes the vote tally to include
	// the updated eth1 data.
	deposits, err = bs.deposits(ctx, beaconState, vote)
	require.NoError(t, err)
	assert.Equal(t, len(recentDeposits), len(deposits), "Received unexpected number of pending deposits")
}

func TestProposer_PendingDeposits_CantReturnBelowStateEth1DepositIndex(t *testing.T) {
	ctx := context.Background()
	height := big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance))
	p := &mockExecution.Chain{
		LatestBlockNumber: height,
		HashesByHeight: map[int][]byte{
			int(height.Int64()): []byte("0x0"),
		},
	}

	beaconState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetEth1Data(&zondpb.Eth1Data{
		BlockHash:    bytesutil.PadTo([]byte("0x0"), 32),
		DepositRoot:  make([]byte, 32),
		DepositCount: 100,
	}))
	require.NoError(t, beaconState.SetEth1DepositIndex(10))
	blk := util.NewBeaconBlockCapella()
	blk.Block.Slot = beaconState.Slot()
	blkRoot, err := blk.HashTreeRoot()
	require.NoError(t, err)

	var mockSig [field_params.DilithiumSignatureLength]byte
	var mockCreds [32]byte

	readyDeposits := []*zondpb.DepositContainer{
		{
			Index: 0,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("a"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index: 1,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("b"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	var recentDeposits []*zondpb.DepositContainer
	for i := int64(2); i < 16; i++ {
		recentDeposits = append(recentDeposits, &zondpb.DepositContainer{
			Index: i,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte{byte(i)}, field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		})
	}
	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not setup deposit trie")

	depositCache, err := depositcache.New()
	require.NoError(t, err)

	for _, dp := range append(readyDeposits, recentDeposits...) {
		depositHash, err := dp.Deposit.Data.HashTreeRoot()
		require.NoError(t, err, "Unable to determine hashed value of deposit")

		assert.NoError(t, depositTrie.Insert(depositHash[:], int(dp.Index)))
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		assert.NoError(t, depositCache.InsertDeposit(ctx, dp.Deposit, uint64(dp.Index), dp.Index, root))
	}
	for _, dp := range recentDeposits {
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		depositCache.InsertPendingDeposit(ctx, dp.Deposit, uint64(dp.Index), dp.Index, root)
	}

	bs := &Server{
		ChainStartFetcher:      p,
		Eth1InfoFetcher:        p,
		Eth1BlockFetcher:       p,
		DepositFetcher:         depositCache,
		PendingDepositsFetcher: depositCache,
		BlockReceiver:          &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		HeadFetcher:            &mock.ChainService{State: beaconState, Root: blkRoot[:]},
	}

	// It should also return the recent deposits after their follow window.
	p.LatestBlockNumber = big.NewInt(0).Add(p.LatestBlockNumber, big.NewInt(10000))
	deposits, err := bs.deposits(ctx, beaconState, &zondpb.Eth1Data{})
	require.NoError(t, err)

	expectedDeposits := 6
	assert.Equal(t, expectedDeposits, len(deposits), "Received unexpected number of pending deposits")
}

func TestProposer_PendingDeposits_CantReturnMoreThanMax(t *testing.T) {
	ctx := context.Background()

	height := big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance))
	p := &mockExecution.Chain{
		LatestBlockNumber: height,
		HashesByHeight: map[int][]byte{
			int(height.Int64()): []byte("0x0"),
		},
	}

	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Eth1Data: &zondpb.Eth1Data{
			BlockHash:    bytesutil.PadTo([]byte("0x0"), 32),
			DepositRoot:  make([]byte, 32),
			DepositCount: 100,
		},
		Eth1DepositIndex: 2,
	})
	require.NoError(t, err)
	blk := util.NewBeaconBlockCapella()
	blk.Block.Slot = beaconState.Slot()
	blkRoot, err := blk.HashTreeRoot()
	require.NoError(t, err)
	var mockSig [field_params.DilithiumSignatureLength]byte
	var mockCreds [32]byte

	readyDeposits := []*zondpb.DepositContainer{
		{
			Index: 0,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("a"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index: 1,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("b"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	var recentDeposits []*zondpb.DepositContainer
	for i := int64(2); i < 22; i++ {
		recentDeposits = append(recentDeposits, &zondpb.DepositContainer{
			Index: i,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte{byte(i)}, field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		})
	}
	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not setup deposit trie")

	depositCache, err := depositcache.New()
	require.NoError(t, err)

	for _, dp := range append(readyDeposits, recentDeposits...) {
		depositHash, err := dp.Deposit.Data.HashTreeRoot()
		require.NoError(t, err, "Unable to determine hashed value of deposit")

		assert.NoError(t, depositTrie.Insert(depositHash[:], int(dp.Index)))
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		assert.NoError(t, depositCache.InsertDeposit(ctx, dp.Deposit, height.Uint64(), dp.Index, root))
	}
	for _, dp := range recentDeposits {
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		depositCache.InsertPendingDeposit(ctx, dp.Deposit, height.Uint64(), dp.Index, root)
	}

	bs := &Server{
		ChainStartFetcher:      p,
		Eth1InfoFetcher:        p,
		Eth1BlockFetcher:       p,
		DepositFetcher:         depositCache,
		PendingDepositsFetcher: depositCache,
		BlockReceiver:          &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		HeadFetcher:            &mock.ChainService{State: beaconState, Root: blkRoot[:]},
	}

	// It should also return the recent deposits after their follow window.
	p.LatestBlockNumber = big.NewInt(0).Add(p.LatestBlockNumber, big.NewInt(10000))
	deposits, err := bs.deposits(ctx, beaconState, &zondpb.Eth1Data{})
	require.NoError(t, err)
	assert.Equal(t, params.BeaconConfig().MaxDeposits, uint64(len(deposits)), "Received unexpected number of pending deposits")
}

func TestProposer_PendingDeposits_CantReturnMoreThanDepositCount(t *testing.T) {
	ctx := context.Background()

	height := big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance))
	p := &mockExecution.Chain{
		LatestBlockNumber: height,
		HashesByHeight: map[int][]byte{
			int(height.Int64()): []byte("0x0"),
		},
	}

	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Eth1Data: &zondpb.Eth1Data{
			BlockHash:    bytesutil.PadTo([]byte("0x0"), 32),
			DepositRoot:  make([]byte, 32),
			DepositCount: 5,
		},
		Eth1DepositIndex: 2,
	})
	require.NoError(t, err)
	blk := util.NewBeaconBlockCapella()
	blk.Block.Slot = beaconState.Slot()
	blkRoot, err := blk.HashTreeRoot()
	require.NoError(t, err)
	var mockSig [field_params.DilithiumSignatureLength]byte
	var mockCreds [32]byte

	readyDeposits := []*zondpb.DepositContainer{
		{
			Index: 0,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("a"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index: 1,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("b"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	var recentDeposits []*zondpb.DepositContainer
	for i := int64(2); i < 22; i++ {
		recentDeposits = append(recentDeposits, &zondpb.DepositContainer{
			Index: i,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte{byte(i)}, field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		})
	}
	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not setup deposit trie")

	depositCache, err := depositcache.New()
	require.NoError(t, err)

	for _, dp := range append(readyDeposits, recentDeposits...) {
		depositHash, err := dp.Deposit.Data.HashTreeRoot()
		require.NoError(t, err, "Unable to determine hashed value of deposit")

		assert.NoError(t, depositTrie.Insert(depositHash[:], int(dp.Index)))
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		assert.NoError(t, depositCache.InsertDeposit(ctx, dp.Deposit, uint64(dp.Index), dp.Index, root))
	}
	for _, dp := range recentDeposits {
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		depositCache.InsertPendingDeposit(ctx, dp.Deposit, uint64(dp.Index), dp.Index, root)
	}

	bs := &Server{
		BlockReceiver:          &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		HeadFetcher:            &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		ChainStartFetcher:      p,
		Eth1InfoFetcher:        p,
		Eth1BlockFetcher:       p,
		DepositFetcher:         depositCache,
		PendingDepositsFetcher: depositCache,
	}

	// It should also return the recent deposits after their follow window.
	p.LatestBlockNumber = big.NewInt(0).Add(p.LatestBlockNumber, big.NewInt(10000))
	deposits, err := bs.deposits(ctx, beaconState, &zondpb.Eth1Data{})
	require.NoError(t, err)
	assert.Equal(t, 3, len(deposits), "Received unexpected number of pending deposits")
}

func TestProposer_DepositTrie_UtilizesCachedFinalizedDeposits(t *testing.T) {
	ctx := context.Background()

	height := big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance))
	p := &mockExecution.Chain{
		LatestBlockNumber: height,
		HashesByHeight: map[int][]byte{
			int(height.Int64()): []byte("0x0"),
		},
	}

	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Eth1Data: &zondpb.Eth1Data{
			BlockHash:    bytesutil.PadTo([]byte("0x0"), 32),
			DepositRoot:  make([]byte, 32),
			DepositCount: 4,
		},
		Eth1DepositIndex: 1,
	})
	require.NoError(t, err)
	blk := util.NewBeaconBlockCapella()
	blk.Block.Slot = beaconState.Slot()

	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	var mockSig [field_params.DilithiumSignatureLength]byte
	var mockCreds [32]byte

	// Using the merkleTreeIndex as the block number for this test...
	finalizedDeposits := []*zondpb.DepositContainer{
		{
			Index:           0,
			Eth1BlockHeight: 10,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("a"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index:           1,
			Eth1BlockHeight: 10,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("b"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	recentDeposits := []*zondpb.DepositContainer{
		{
			Index:           2,
			Eth1BlockHeight: 11,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("c"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index:           3,
			Eth1BlockHeight: 11,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("d"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	depositCache, err := depositcache.New()
	require.NoError(t, err)

	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not setup deposit trie")
	for _, dp := range append(finalizedDeposits, recentDeposits...) {
		depositHash, err := dp.Deposit.Data.HashTreeRoot()
		require.NoError(t, err, "Unable to determine hashed value of deposit")

		assert.NoError(t, depositTrie.Insert(depositHash[:], int(dp.Index)))
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		assert.NoError(t, depositCache.InsertDeposit(ctx, dp.Deposit, dp.Eth1BlockHeight, dp.Index, root))
	}
	for _, dp := range recentDeposits {
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		depositCache.InsertPendingDeposit(ctx, dp.Deposit, dp.Eth1BlockHeight, dp.Index, root)
	}

	bs := &Server{
		ChainStartFetcher:      p,
		Eth1InfoFetcher:        p,
		Eth1BlockFetcher:       p,
		DepositFetcher:         depositCache,
		PendingDepositsFetcher: depositCache,
		BlockReceiver:          &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		HeadFetcher:            &mock.ChainService{State: beaconState, Root: blkRoot[:]},
	}

	dt, err := bs.depositTrie(ctx, &zondpb.Eth1Data{}, big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance)))
	require.NoError(t, err)

	actualRoot, err := dt.HashTreeRoot()
	require.NoError(t, err)
	expectedRoot, err := depositTrie.HashTreeRoot()
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, actualRoot, "Incorrect deposit trie root")
}

func TestProposer_DepositTrie_RebuildTrie(t *testing.T) {
	ctx := context.Background()

	height := big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance))
	p := &mockExecution.Chain{
		LatestBlockNumber: height,
		HashesByHeight: map[int][]byte{
			int(height.Int64()): []byte("0x0"),
		},
	}

	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Eth1Data: &zondpb.Eth1Data{
			BlockHash:    bytesutil.PadTo([]byte("0x0"), 32),
			DepositRoot:  make([]byte, 32),
			DepositCount: 4,
		},
		Eth1DepositIndex: 1,
	})
	require.NoError(t, err)
	blk := util.NewBeaconBlockCapella()
	blk.Block.Slot = beaconState.Slot()

	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	var mockSig [field_params.DilithiumSignatureLength]byte
	var mockCreds [32]byte

	// Using the merkleTreeIndex as the block number for this test...
	finalizedDeposits := []*zondpb.DepositContainer{
		{
			Index:           0,
			Eth1BlockHeight: 10,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("a"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index:           1,
			Eth1BlockHeight: 10,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("b"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	recentDeposits := []*zondpb.DepositContainer{
		{
			Index:           2,
			Eth1BlockHeight: 11,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("c"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index:           3,
			Eth1BlockHeight: 11,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("d"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	depositCache, err := depositcache.New()
	require.NoError(t, err)

	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not setup deposit trie")
	for _, dp := range append(finalizedDeposits, recentDeposits...) {
		depositHash, err := dp.Deposit.Data.HashTreeRoot()
		require.NoError(t, err, "Unable to determine hashed value of deposit")

		assert.NoError(t, depositTrie.Insert(depositHash[:], int(dp.Index)))
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		assert.NoError(t, depositCache.InsertDeposit(ctx, dp.Deposit, dp.Eth1BlockHeight, dp.Index, root))
	}
	for _, dp := range recentDeposits {
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		depositCache.InsertPendingDeposit(ctx, dp.Deposit, dp.Eth1BlockHeight, dp.Index, root)
	}
	d := depositCache.AllDepositContainers(ctx)
	origDeposit, ok := proto.Clone(d[0].Deposit).(*zondpb.Deposit)
	assert.Equal(t, true, ok)
	junkCreds := mockCreds
	copy(junkCreds[:1], []byte{'A'})
	// Mutate it since its a pointer
	d[0].Deposit.Data.WithdrawalCredentials = junkCreds[:]
	// Insert junk to corrupt trie.
	err = depositCache.InsertFinalizedDeposits(ctx, 2, [32]byte{}, 0)
	require.NoError(t, err)

	// Add original back
	d[0].Deposit = origDeposit

	bs := &Server{
		ChainStartFetcher:      p,
		Eth1InfoFetcher:        p,
		Eth1BlockFetcher:       p,
		DepositFetcher:         depositCache,
		PendingDepositsFetcher: depositCache,
		BlockReceiver:          &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		HeadFetcher:            &mock.ChainService{State: beaconState, Root: blkRoot[:]},
	}

	dt, err := bs.depositTrie(ctx, &zondpb.Eth1Data{}, big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance)))
	require.NoError(t, err)

	expectedRoot, err := depositTrie.HashTreeRoot()
	require.NoError(t, err)
	actualRoot, err := dt.HashTreeRoot()
	require.NoError(t, err)
	assert.Equal(t, expectedRoot, actualRoot, "Incorrect deposit trie root")

}

func TestProposer_ValidateDepositTrie(t *testing.T) {
	tt := []struct {
		name            string
		eth1dataCreator func() *zondpb.Eth1Data
		trieCreator     func() *trie.SparseMerkleTrie
		success         bool
	}{
		{
			name: "invalid trie items",
			eth1dataCreator: func() *zondpb.Eth1Data {
				return &zondpb.Eth1Data{DepositRoot: []byte{}, DepositCount: 10, BlockHash: []byte{}}
			},
			trieCreator: func() *trie.SparseMerkleTrie {
				newTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
				assert.NoError(t, err)
				return newTrie
			},
			success: false,
		},
		{
			name: "invalid deposit root",
			eth1dataCreator: func() *zondpb.Eth1Data {
				newTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
				assert.NoError(t, err)
				assert.NoError(t, newTrie.Insert([]byte{'a'}, 0))
				assert.NoError(t, newTrie.Insert([]byte{'b'}, 1))
				assert.NoError(t, newTrie.Insert([]byte{'c'}, 2))
				return &zondpb.Eth1Data{DepositRoot: []byte{'B'}, DepositCount: 3, BlockHash: []byte{}}
			},
			trieCreator: func() *trie.SparseMerkleTrie {
				newTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
				assert.NoError(t, err)
				assert.NoError(t, newTrie.Insert([]byte{'a'}, 0))
				assert.NoError(t, newTrie.Insert([]byte{'b'}, 1))
				assert.NoError(t, newTrie.Insert([]byte{'c'}, 2))
				return newTrie
			},
			success: false,
		},
		{
			name: "valid deposit trie",
			eth1dataCreator: func() *zondpb.Eth1Data {
				newTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
				assert.NoError(t, err)
				assert.NoError(t, newTrie.Insert([]byte{'a'}, 0))
				assert.NoError(t, newTrie.Insert([]byte{'b'}, 1))
				assert.NoError(t, newTrie.Insert([]byte{'c'}, 2))
				rt, err := newTrie.HashTreeRoot()
				require.NoError(t, err)
				return &zondpb.Eth1Data{DepositRoot: rt[:], DepositCount: 3, BlockHash: []byte{}}
			},
			trieCreator: func() *trie.SparseMerkleTrie {
				newTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
				assert.NoError(t, err)
				assert.NoError(t, newTrie.Insert([]byte{'a'}, 0))
				assert.NoError(t, newTrie.Insert([]byte{'b'}, 1))
				assert.NoError(t, newTrie.Insert([]byte{'c'}, 2))
				return newTrie
			},
			success: true,
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			valid, err := validateDepositTrie(test.trieCreator(), test.eth1dataCreator())
			assert.Equal(t, test.success, valid)
			if valid {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProposer_Eth1Data_MajorityVote_SpansGenesis(t *testing.T) {
	ctx := context.Background()
	// Voting period will span genesis, causing the special case for pre-mined genesis to kick in.
	// In other words some part of the valid time range is before genesis, so querying the block cache would fail
	// without the special case added to allow this for testnets.
	slot := primitives.Slot(0)
	earliestValidTime, latestValidTime := majorityVoteBoundaryTime(slot)

	p := mockExecution.New().
		InsertBlock(50, earliestValidTime, []byte("earliest")).
		InsertBlock(100, latestValidTime, []byte("latest"))

	headBlockHash := []byte("headb")
	depositCache, err := depositcache.New()
	require.NoError(t, err)
	ps := &Server{
		ChainStartFetcher: p,
		Eth1InfoFetcher:   p,
		Eth1BlockFetcher:  p,
		BlockFetcher:      p,
		DepositFetcher:    depositCache,
		HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{BlockHash: headBlockHash, DepositCount: 0}},
	}

	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Slot: slot,
		Eth1DataVotes: []*zondpb.Eth1Data{
			{BlockHash: []byte("earliest"), DepositCount: 1},
		},
	})
	require.NoError(t, err)
	majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
	require.NoError(t, err)
	assert.DeepEqual(t, headBlockHash, majorityVoteEth1Data.BlockHash)
}

func TestProposer_Eth1Data_MajorityVote(t *testing.T) {
	followDistanceSecs := params.BeaconConfig().Eth1FollowDistance * params.BeaconConfig().SecondsPerETH1Block
	followSlots := followDistanceSecs / params.BeaconConfig().SecondsPerSlot
	slot := primitives.Slot(256 + followSlots)
	earliestValidTime, latestValidTime := majorityVoteBoundaryTime(slot)

	dc := zondpb.DepositContainer{
		Index:           0,
		Eth1BlockHeight: 0,
		Deposit: &zondpb.Deposit{
			Data: &zondpb.Deposit_Data{
				PublicKey:             bytesutil.PadTo([]byte("a"), field_params.DilithiumPubkeyLength),
				Signature:             make([]byte, field_params.DilithiumSignatureLength),
				WithdrawalCredentials: make([]byte, 32),
			}},
	}
	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err)
	depositCache, err := depositcache.New()
	require.NoError(t, err)
	root, err := depositTrie.HashTreeRoot()
	require.NoError(t, err)
	assert.NoError(t, depositCache.InsertDeposit(context.Background(), dc.Deposit, dc.Eth1BlockHeight, dc.Index, root))

	t.Run("choose highest count", func(t *testing.T) {
		t.Skip()
		p := mockExecution.New().
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(51, earliestValidTime+1, []byte("first")).
			InsertBlock(52, earliestValidTime+2, []byte("second")).
			InsertBlock(100, latestValidTime, []byte("latest"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("first"), DepositCount: 1},
				{BlockHash: []byte("first"), DepositCount: 1},
				{BlockHash: []byte("second"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("first")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("highest count at earliest valid time - choose highest count", func(t *testing.T) {
		t.Skip()
		p := mockExecution.New().
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(52, earliestValidTime+2, []byte("second")).
			InsertBlock(100, latestValidTime, []byte("latest"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("earliest"), DepositCount: 1},
				{BlockHash: []byte("earliest"), DepositCount: 1},
				{BlockHash: []byte("second"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("earliest")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("highest count at latest valid time - choose highest count", func(t *testing.T) {
		t.Skip()
		p := mockExecution.New().
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(51, earliestValidTime+1, []byte("first")).
			InsertBlock(100, latestValidTime, []byte("latest"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("first"), DepositCount: 1},
				{BlockHash: []byte("latest"), DepositCount: 1},
				{BlockHash: []byte("latest"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("latest")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("highest count before range - choose highest count within range", func(t *testing.T) {
		t.Skip()
		p := mockExecution.New().
			InsertBlock(49, earliestValidTime-1, []byte("before_range")).
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(51, earliestValidTime+1, []byte("first")).
			InsertBlock(100, latestValidTime, []byte("latest"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("before_range"), DepositCount: 1},
				{BlockHash: []byte("before_range"), DepositCount: 1},
				{BlockHash: []byte("first"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("first")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("highest count after range - choose highest count within range", func(t *testing.T) {
		t.Skip()
		p := mockExecution.New().
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(51, earliestValidTime+1, []byte("first")).
			InsertBlock(100, latestValidTime, []byte("latest")).
			InsertBlock(101, latestValidTime+1, []byte("after_range"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("first"), DepositCount: 1},
				{BlockHash: []byte("after_range"), DepositCount: 1},
				{BlockHash: []byte("after_range"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("first")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("highest count on unknown block - choose known block with highest count", func(t *testing.T) {
		t.Skip()
		p := mockExecution.New().
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(51, earliestValidTime+1, []byte("first")).
			InsertBlock(52, earliestValidTime+2, []byte("second")).
			InsertBlock(100, latestValidTime, []byte("latest"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("unknown"), DepositCount: 1},
				{BlockHash: []byte("unknown"), DepositCount: 1},
				{BlockHash: []byte("first"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("first")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("no blocks in range - choose current eth1data", func(t *testing.T) {
		p := mockExecution.New().
			InsertBlock(49, earliestValidTime-1, []byte("before_range")).
			InsertBlock(101, latestValidTime+1, []byte("after_range"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
		})
		require.NoError(t, err)

		currentEth1Data := &zondpb.Eth1Data{DepositCount: 1, BlockHash: []byte("current")}
		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: currentEth1Data},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("current")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("no votes in range - choose most recent block", func(t *testing.T) {
		p := mockExecution.New().
			InsertBlock(49, earliestValidTime-1, []byte("before_range")).
			InsertBlock(51, earliestValidTime+1, []byte("first")).
			InsertBlock(52, earliestValidTime+2, []byte("second")).
			InsertBlock(101, latestValidTime+1, []byte("after_range"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("before_range"), DepositCount: 1},
				{BlockHash: []byte("after_range"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := make([]byte, 32)
		copy(expectedHash, "second")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("no votes - choose more recent block", func(t *testing.T) {
		p := mockExecution.New().
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(100, latestValidTime, []byte("latest"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot:          slot,
			Eth1DataVotes: []*zondpb.Eth1Data{}})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := make([]byte, 32)
		copy(expectedHash, "latest")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("no votes and more recent block has less deposits - choose current eth1data", func(t *testing.T) {
		p := mockExecution.New().
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(100, latestValidTime, []byte("latest"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
		})
		require.NoError(t, err)

		// Set the deposit count in current eth1data to exceed the latest most recent block's deposit count.
		currentEth1Data := &zondpb.Eth1Data{DepositCount: 2, BlockHash: []byte("current")}
		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: currentEth1Data},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("current")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("same count - choose more recent block", func(t *testing.T) {
		t.Skip()
		p := mockExecution.New().
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(51, earliestValidTime+1, []byte("first")).
			InsertBlock(52, earliestValidTime+2, []byte("second")).
			InsertBlock(100, latestValidTime, []byte("latest"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("first"), DepositCount: 1},
				{BlockHash: []byte("second"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("second")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("highest count on block with less deposits - choose another block", func(t *testing.T) {
		t.Skip()
		p := mockExecution.New().
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(51, earliestValidTime+1, []byte("first")).
			InsertBlock(52, earliestValidTime+2, []byte("second")).
			InsertBlock(100, latestValidTime, []byte("latest"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("no_new_deposits"), DepositCount: 0},
				{BlockHash: []byte("no_new_deposits"), DepositCount: 0},
				{BlockHash: []byte("second"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("second")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("only one block at earliest valid time - choose this block", func(t *testing.T) {
		t.Skip()
		p := mockExecution.New().InsertBlock(50, earliestValidTime, []byte("earliest"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("earliest"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("earliest")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("vote on last block before range - choose next block", func(t *testing.T) {
		p := mockExecution.New().
			InsertBlock(49, earliestValidTime-1, []byte("before_range")).
			// It is important to have height `50` with time `earliestValidTime+1` and not `earliestValidTime`
			// because of earliest block increment in the algorithm.
			InsertBlock(50, earliestValidTime+1, []byte("first"))

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("before_range"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 1}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := make([]byte, 32)
		copy(expectedHash, "first")
		assert.DeepEqual(t, expectedHash, hash)
	})

	t.Run("no deposits - choose chain start eth1data", func(t *testing.T) {
		p := mockExecution.New().
			InsertBlock(50, earliestValidTime, []byte("earliest")).
			InsertBlock(100, latestValidTime, []byte("latest"))
		p.Eth1Data = &zondpb.Eth1Data{
			BlockHash: []byte("eth1data"),
		}

		depositCache, err := depositcache.New()
		require.NoError(t, err)

		beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
			Slot: slot,
			Eth1DataVotes: []*zondpb.Eth1Data{
				{BlockHash: []byte("earliest"), DepositCount: 1},
			},
		})
		require.NoError(t, err)

		ps := &Server{
			ChainStartFetcher: p,
			Eth1InfoFetcher:   p,
			Eth1BlockFetcher:  p,
			BlockFetcher:      p,
			DepositFetcher:    depositCache,
			HeadFetcher:       &mock.ChainService{ETH1Data: &zondpb.Eth1Data{DepositCount: 0}},
		}

		ctx := context.Background()
		majorityVoteEth1Data, err := ps.eth1DataMajorityVote(ctx, beaconState)
		require.NoError(t, err)

		hash := majorityVoteEth1Data.BlockHash

		expectedHash := []byte("eth1data")
		assert.DeepEqual(t, expectedHash, hash)
	})
}

func TestProposer_FilterAttestation(t *testing.T) {
	genesis := util.NewBeaconBlockCapella()

	numValidators := uint64(64)
	st, privKeys := util.DeterministicGenesisStateCapella(t, numValidators)
	require.NoError(t, st.SetGenesisValidatorsRoot(params.BeaconConfig().ZeroHash[:]))
	assert.NoError(t, st.SetSlot(1))

	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err)

	tests := []struct {
		name         string
		wantedErr    string
		inputAtts    func() []*zondpb.Attestation
		expectedAtts func(inputAtts []*zondpb.Attestation) []*zondpb.Attestation
	}{
		{
			name: "nil attestations",
			inputAtts: func() []*zondpb.Attestation {
				return nil
			},
			expectedAtts: func(inputAtts []*zondpb.Attestation) []*zondpb.Attestation {
				return []*zondpb.Attestation{}
			},
		},
		{
			name: "invalid attestations",
			inputAtts: func() []*zondpb.Attestation {
				atts := make([]*zondpb.Attestation, 10)
				for i := 0; i < len(atts); i++ {
					atts[i] = util.HydrateAttestation(&zondpb.Attestation{
						Data: &zondpb.AttestationData{
							CommitteeIndex: primitives.CommitteeIndex(i),
						},
					})
				}
				return atts
			},
			expectedAtts: func(inputAtts []*zondpb.Attestation) []*zondpb.Attestation {
				return []*zondpb.Attestation{}
			},
		},
		{
			name: "filter aggregates ok",
			inputAtts: func() []*zondpb.Attestation {
				atts := make([]*zondpb.Attestation, 10)
				for i := 0; i < len(atts); i++ {
					atts[i] = util.HydrateAttestation(&zondpb.Attestation{
						Data: &zondpb.AttestationData{
							CommitteeIndex: primitives.CommitteeIndex(i),
							Source:         &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
						},
						AggregationBits: bitfield.Bitlist{0b00010010},
					})
					committee, err := helpers.BeaconCommitteeFromState(context.Background(), st, atts[i].Data.Slot, atts[i].Data.CommitteeIndex)
					assert.NoError(t, err)
					attestingIndices, err := attestation.AttestingIndices(atts[i].AggregationBits, committee)
					require.NoError(t, err)
					assert.NoError(t, err)
					domain, err := signing.Domain(st.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, params.BeaconConfig().ZeroHash[:])
					require.NoError(t, err)
					sigs := make([][]byte, len(attestingIndices))
					var zeroSig [field_params.DilithiumSignatureLength]byte
					atts[i].Signatures = [][]byte{zeroSig[:]}

					for i, indice := range attestingIndices {
						hashTreeRoot, err := signing.ComputeSigningRoot(atts[i].Data, domain)
						require.NoError(t, err)
						sig := privKeys[indice].Sign(hashTreeRoot[:]).Marshal()
						sigs[i] = sig
					}
					atts[i].Signatures = sigs
				}
				return atts
			},
			expectedAtts: func(inputAtts []*zondpb.Attestation) []*zondpb.Attestation {
				return []*zondpb.Attestation{inputAtts[0], inputAtts[1]}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proposerServer := &Server{
				AttPool:     attestations.NewPool(),
				HeadFetcher: &mock.ChainService{State: st, Root: genesisRoot[:]},
			}
			atts := tt.inputAtts()
			received, err := proposerServer.validateAndDeleteAttsInPool(context.Background(), st, atts)
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
				assert.Equal(t, nil, received)
			} else {
				assert.NoError(t, err)
				assert.DeepEqual(t, tt.expectedAtts(atts), received)
			}
		})
	}
}

func TestProposer_Deposits_ReturnsEmptyList_IfLatestEth1DataEqGenesisEth1Block(t *testing.T) {
	ctx := context.Background()

	height := big.NewInt(int64(params.BeaconConfig().Eth1FollowDistance))
	p := &mockExecution.Chain{
		LatestBlockNumber: height,
		HashesByHeight: map[int][]byte{
			int(height.Int64()): []byte("0x0"),
		},
		GenesisEth1Block: height,
	}

	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Eth1Data: &zondpb.Eth1Data{
			BlockHash:   bytesutil.PadTo([]byte("0x0"), 32),
			DepositRoot: make([]byte, 32),
		},
		Eth1DepositIndex: 2,
	})
	require.NoError(t, err)
	blk := util.NewBeaconBlockCapella()
	blk.Block.Slot = beaconState.Slot()
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	var mockSig [field_params.DilithiumSignatureLength]byte
	var mockCreds [32]byte

	readyDeposits := []*zondpb.DepositContainer{
		{
			Index: 0,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("a"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
		{
			Index: 1,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte("b"), field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		},
	}

	var recentDeposits []*zondpb.DepositContainer
	for i := int64(2); i < 22; i++ {
		recentDeposits = append(recentDeposits, &zondpb.DepositContainer{
			Index: i,
			Deposit: &zondpb.Deposit{
				Data: &zondpb.Deposit_Data{
					PublicKey:             bytesutil.PadTo([]byte{byte(i)}, field_params.DilithiumPubkeyLength),
					Signature:             mockSig[:],
					WithdrawalCredentials: mockCreds[:],
				}},
		})
	}
	depositTrie, err := trie.NewTrie(params.BeaconConfig().DepositContractTreeDepth)
	require.NoError(t, err, "Could not setup deposit trie")

	depositCache, err := depositcache.New()
	require.NoError(t, err)

	for _, dp := range append(readyDeposits, recentDeposits...) {
		depositHash, err := dp.Deposit.Data.HashTreeRoot()
		require.NoError(t, err, "Unable to determine hashed value of deposit")

		assert.NoError(t, depositTrie.Insert(depositHash[:], int(dp.Index)))
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		assert.NoError(t, depositCache.InsertDeposit(ctx, dp.Deposit, uint64(dp.Index), dp.Index, root))
	}
	for _, dp := range recentDeposits {
		root, err := depositTrie.HashTreeRoot()
		require.NoError(t, err)
		depositCache.InsertPendingDeposit(ctx, dp.Deposit, uint64(dp.Index), dp.Index, root)
	}

	bs := &Server{
		BlockReceiver:          &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		HeadFetcher:            &mock.ChainService{State: beaconState, Root: blkRoot[:]},
		ChainStartFetcher:      p,
		Eth1InfoFetcher:        p,
		Eth1BlockFetcher:       p,
		DepositFetcher:         depositCache,
		PendingDepositsFetcher: depositCache,
	}

	// It should also return the recent deposits after their follow window.
	p.LatestBlockNumber = big.NewInt(0).Add(p.LatestBlockNumber, big.NewInt(10000))
	deposits, err := bs.deposits(ctx, beaconState, &zondpb.Eth1Data{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(deposits), "Received unexpected number of pending deposits")
}

func TestProposer_DeleteAttsInPool_Aggregated(t *testing.T) {
	s := &Server{
		AttPool: attestations.NewPool(),
	}
	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte("foo")).Marshal()
	aggregatedAtts := []*zondpb.Attestation{
		util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b10101}, Signatures: [][]byte{sig, sig}}),
		util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b11010}, Signatures: [][]byte{sig, sig}})}
	unaggregatedAtts := []*zondpb.Attestation{
		util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b10010}, Signatures: [][]byte{sig}}),
		util.HydrateAttestation(&zondpb.Attestation{Data: &zondpb.AttestationData{Slot: 1}, AggregationBits: bitfield.Bitlist{0b10100}, Signatures: [][]byte{sig}})}

	require.NoError(t, s.AttPool.SaveAggregatedAttestations(aggregatedAtts))
	require.NoError(t, s.AttPool.SaveUnaggregatedAttestations(unaggregatedAtts))

	aa, err := attaggregation.Aggregate(aggregatedAtts)
	require.NoError(t, err)
	require.NoError(t, s.deleteAttsInPool(context.Background(), append(aa, unaggregatedAtts...)))
	assert.Equal(t, 0, len(s.AttPool.AggregatedAttestations()), "Did not delete aggregated attestation")
	atts, err := s.AttPool.UnaggregatedAttestations()
	require.NoError(t, err)
	assert.Equal(t, 0, len(atts), "Did not delete unaggregated attestation")
}

func TestProposer_GetSyncAggregate_OK(t *testing.T) {
	proposerServer := &Server{
		SyncChecker:       &mockSync.Sync{IsSyncing: false},
		SyncCommitteePool: synccommittee.NewStore(),
	}

	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sigsLen := 8
	sigs := make([][]byte, 0, sigsLen)
	for i := 0; i < sigsLen; i++ {
		sigs = append(sigs, priv.Sign([]byte(fmt.Sprintf("foo%d", i))).Marshal())
	}

	r := params.BeaconConfig().ZeroHash

	conts := []*zondpb.SyncCommitteeContribution{
		{Slot: 1, SubcommitteeIndex: 0, Signatures: [][]byte{sigs[0]}, AggregationBits: []byte{0b0001}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 0, Signatures: [][]byte{sigs[0], sigs[3]}, AggregationBits: []byte{0b1001}, BlockRoot: r[:]},
		{Slot: 1, SubcommitteeIndex: 0, Signatures: [][]byte{sigs[1], sigs[2], sigs[3]}, AggregationBits: []byte{0b1110}, BlockRoot: r[:]},
		{Slot: 2, SubcommitteeIndex: 0, Signatures: [][]byte{sigs[1], sigs[3], sigs[5], sigs[7]}, AggregationBits: []byte{0b10101010}, BlockRoot: r[:]},
	}

	for _, cont := range conts {
		require.NoError(t, proposerServer.SyncCommitteePool.SaveSyncCommitteeContribution(cont))
	}

	aggregate, err := proposerServer.getSyncAggregate(context.Background(), 1, bytesutil.ToBytes32(conts[0].BlockRoot))
	require.NoError(t, err)
	require.DeepEqual(t, bitfield.Bitvector16{0xf}, aggregate.SyncCommitteeBits)

	aggregate, err = proposerServer.getSyncAggregate(context.Background(), 2, bytesutil.ToBytes32(conts[0].BlockRoot))
	require.NoError(t, err)
	require.DeepEqual(t, bitfield.Bitvector16{0xaa}, aggregate.SyncCommitteeBits)

	aggregate, err = proposerServer.getSyncAggregate(context.Background(), 3, bytesutil.ToBytes32(conts[0].BlockRoot))
	require.NoError(t, err)
	require.DeepEqual(t, bitfield.NewBitvector16(), aggregate.SyncCommitteeBits)
}

func TestProposer_PrepareBeaconProposer(t *testing.T) {
	type args struct {
		request *zondpb.PrepareBeaconProposerRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr string
	}{
		{
			name: "Happy Path",
			args: args{
				request: &zondpb.PrepareBeaconProposerRequest{
					Recipients: []*zondpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
						{
							FeeRecipient:   make([]byte, fieldparams.FeeRecipientLength),
							ValidatorIndex: 1,
						},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "invalid fee recipient length",
			args: args{
				request: &zondpb.PrepareBeaconProposerRequest{
					Recipients: []*zondpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
						{
							FeeRecipient:   make([]byte, field_params.DilithiumPubkeyLength),
							ValidatorIndex: 1,
						},
					},
				},
			},
			wantErr: "Invalid fee recipient address",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := dbutil.SetupDB(t)
			ctx := context.Background()
			proposerServer := &Server{BeaconDB: db}
			_, err := proposerServer.PrepareBeaconProposer(ctx, tt.args.request)
			if tt.wantErr != "" {
				require.ErrorContains(t, tt.wantErr, err)
				return
			}
			require.NoError(t, err)
			address, err := proposerServer.BeaconDB.FeeRecipientByValidatorID(ctx, 1)
			require.NoError(t, err)
			require.Equal(t, common.BytesToAddress(tt.args.request.Recipients[0].FeeRecipient), address)

		})
	}
}

func TestProposer_PrepareBeaconProposerOverlapping(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbutil.SetupDB(t)
	ctx := context.Background()
	proposerServer := &Server{BeaconDB: db}

	// New validator
	f := bytesutil.PadTo([]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF}, fieldparams.FeeRecipientLength)
	req := &zondpb.PrepareBeaconProposerRequest{
		Recipients: []*zondpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
			{FeeRecipient: f, ValidatorIndex: 1},
		},
	}
	_, err := proposerServer.PrepareBeaconProposer(ctx, req)
	require.NoError(t, err)
	require.LogsContain(t, hook, "Updated fee recipient addresses for validator indices")

	// Same validator
	hook.Reset()
	_, err = proposerServer.PrepareBeaconProposer(ctx, req)
	require.NoError(t, err)
	require.LogsDoNotContain(t, hook, "Updated fee recipient addresses for validator indices")

	// Same validator with different fee recipient
	hook.Reset()
	f = bytesutil.PadTo([]byte{0x01, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF}, fieldparams.FeeRecipientLength)
	req = &zondpb.PrepareBeaconProposerRequest{
		Recipients: []*zondpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
			{FeeRecipient: f, ValidatorIndex: 1},
		},
	}
	_, err = proposerServer.PrepareBeaconProposer(ctx, req)
	require.NoError(t, err)
	require.LogsContain(t, hook, "Updated fee recipient addresses for validator indices")

	// More than one validator
	hook.Reset()
	f = bytesutil.PadTo([]byte{0x01, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF}, fieldparams.FeeRecipientLength)
	req = &zondpb.PrepareBeaconProposerRequest{
		Recipients: []*zondpb.PrepareBeaconProposerRequest_FeeRecipientContainer{
			{FeeRecipient: f, ValidatorIndex: 1},
			{FeeRecipient: f, ValidatorIndex: 2},
		},
	}
	_, err = proposerServer.PrepareBeaconProposer(ctx, req)
	require.NoError(t, err)
	require.LogsContain(t, hook, "Updated fee recipient addresses for validator indices")

	// Same validators
	hook.Reset()
	_, err = proposerServer.PrepareBeaconProposer(ctx, req)
	require.NoError(t, err)
	require.LogsDoNotContain(t, hook, "Updated fee recipient addresses for validator indices")
}

func BenchmarkServer_PrepareBeaconProposer(b *testing.B) {
	db := dbutil.SetupDB(b)
	ctx := context.Background()
	proposerServer := &Server{BeaconDB: db}

	f := bytesutil.PadTo([]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF}, fieldparams.FeeRecipientLength)
	recipients := make([]*zondpb.PrepareBeaconProposerRequest_FeeRecipientContainer, 0)
	for i := 0; i < 10000; i++ {
		recipients = append(recipients, &zondpb.PrepareBeaconProposerRequest_FeeRecipientContainer{FeeRecipient: f, ValidatorIndex: primitives.ValidatorIndex(i)})
	}

	req := &zondpb.PrepareBeaconProposerRequest{
		Recipients: recipients,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := proposerServer.PrepareBeaconProposer(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestProposer_SubmitValidatorRegistrations(t *testing.T) {
	ctx := context.Background()
	proposerServer := &Server{}
	reg := &zondpb.SignedValidatorRegistrationsV1{}
	_, err := proposerServer.SubmitValidatorRegistrations(ctx, reg)
	require.ErrorContains(t, builder.ErrNoBuilder.Error(), err)
	proposerServer = &Server{BlockBuilder: &builderTest.MockBuilderService{}}
	_, err = proposerServer.SubmitValidatorRegistrations(ctx, reg)
	require.ErrorContains(t, builder.ErrNoBuilder.Error(), err)
	proposerServer = &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true}}
	_, err = proposerServer.SubmitValidatorRegistrations(ctx, reg)
	require.NoError(t, err)
	proposerServer = &Server{BlockBuilder: &builderTest.MockBuilderService{HasConfigured: true, ErrRegisterValidator: errors.New("bad")}}
	_, err = proposerServer.SubmitValidatorRegistrations(ctx, reg)
	require.ErrorContains(t, "bad", err)
}

func majorityVoteBoundaryTime(slot primitives.Slot) (uint64, uint64) {
	s := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerEth1VotingPeriod))
	slotStartTime := uint64(mockExecution.GenesisTime) + uint64((slot - (slot % (s))).Mul(params.BeaconConfig().SecondsPerSlot))
	earliestValidTime := slotStartTime - 2*params.BeaconConfig().SecondsPerETH1Block*params.BeaconConfig().Eth1FollowDistance
	latestValidTime := slotStartTime - params.BeaconConfig().SecondsPerETH1Block*params.BeaconConfig().Eth1FollowDistance

	return earliestValidTime, latestValidTime
}

func TestProposer_GetFeeRecipientByPubKey(t *testing.T) {
	db := dbutil.SetupDB(t)
	ctx := context.Background()
	numDeposits := uint64(64)
	beaconState, _ := util.DeterministicGenesisStateCapella(t, numDeposits)
	bsRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err)
	proposerServer := &Server{
		BeaconDB:    db,
		HeadFetcher: &mock.ChainService{Root: bsRoot[:], State: beaconState},
	}
	pubkey, err := hexutil.Decode("0xa057816155ad77931185101128655c0191bd0214c201ca48ed887f6c4c6adf334070efcd75140eada5ac83a92506dd7a")
	require.NoError(t, err)
	resp, err := proposerServer.GetFeeRecipientByPubKey(ctx, &zondpb.FeeRecipientByPubKeyRequest{
		PublicKey: pubkey,
	})
	require.NoError(t, err)

	require.Equal(t, params.BeaconConfig().DefaultFeeRecipient.Hex(), hexutil.EncodeZ(resp.FeeRecipient))
	defaultFeeRecipient, err := common.NewAddressFromString("Z046Fb65722E7b2455012BFEBf6177F1D2e9728D9")
	require.NoError(t, err)
	params.BeaconConfig().DefaultFeeRecipient = defaultFeeRecipient
	resp, err = proposerServer.GetFeeRecipientByPubKey(ctx, &zondpb.FeeRecipientByPubKeyRequest{
		PublicKey: beaconState.Validators()[0].PublicKey,
	})
	require.NoError(t, err)

	require.Equal(t, params.BeaconConfig().DefaultFeeRecipient.Hex(), common.BytesToAddress(resp.FeeRecipient).Hex())
	index, err := proposerServer.ValidatorIndex(ctx, &zondpb.ValidatorIndexRequest{
		PublicKey: beaconState.Validators()[0].PublicKey,
	})
	require.NoError(t, err)
	feeRecipient, err := common.NewAddressFromString("Z055Fb65722E7b2455012BFEBf6177F1D2e9728D8")
	require.NoError(t, err)
	err = proposerServer.BeaconDB.SaveFeeRecipientsByValidatorIDs(ctx, []primitives.ValidatorIndex{index.Index}, []common.Address{feeRecipient})
	require.NoError(t, err)
	resp, err = proposerServer.GetFeeRecipientByPubKey(ctx, &zondpb.FeeRecipientByPubKeyRequest{
		PublicKey: beaconState.Validators()[0].PublicKey,
	})
	require.NoError(t, err)

	require.Equal(t, feeRecipient.Hex(), common.BytesToAddress(resp.FeeRecipient).Hex())
}
