package blockchain

import (
	"bytes"
	"context"
	"sort"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestSaveHead_Same(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	r := [32]byte{'A'}
	service.head = &head{root: r}
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
	require.NoError(t, err)
	st, _ := util.DeterministicGenesisStateZond(t, 1)
	require.NoError(t, service.saveHead(context.Background(), r, b, st))
	assert.Equal(t, primitives.Slot(0), service.headSlot(), "Head did not stay the same")
	assert.Equal(t, r, service.headRoot(), "Head did not stay the same")
}

func TestSaveHead_Different(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	oldBlock := util.SaveBlock(t, context.Background(), service.cfg.BeaconDB, util.NewBeaconBlockZond())
	oldRoot, err := oldBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	state, blkRoot, err := prepareForkchoiceState(ctx, oldBlock.Block().Slot(), oldRoot, oldBlock.Block().ParentRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	service.head = &head{
		root:  oldRoot,
		block: oldBlock,
	}

	newHeadSignedBlock := util.NewBeaconBlockZond()
	newHeadSignedBlock.Block.Slot = 1
	newHeadBlock := newHeadSignedBlock.Block

	wsb := util.SaveBlock(t, context.Background(), service.cfg.BeaconDB, newHeadSignedBlock)
	newRoot, err := newHeadBlock.HashTreeRoot()
	require.NoError(t, err)
	state, blkRoot, err = prepareForkchoiceState(ctx, wsb.Block().Slot()-1, wsb.Block().ParentRoot(), service.cfg.ForkChoiceStore.CachedHeadRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))

	state, blkRoot, err = prepareForkchoiceState(ctx, wsb.Block().Slot(), newRoot, wsb.Block().ParentRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	headState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(1))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Slot: 1, Root: newRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveState(context.Background(), headState, newRoot))
	require.NoError(t, service.saveHead(context.Background(), newRoot, wsb, headState))

	assert.Equal(t, primitives.Slot(1), service.HeadSlot(), "Head did not change")

	cachedRoot, err := service.HeadRoot(context.Background())
	require.NoError(t, err)
	assert.DeepEqual(t, cachedRoot, newRoot[:], "Head did not change")
	headBlock, err := service.headBlock()
	require.NoError(t, err)
	pb, err := headBlock.Proto()
	require.NoError(t, err)
	assert.DeepEqual(t, newHeadSignedBlock, pb, "Head did not change")
	assert.DeepSSZEqual(t, headState.ToProto(), service.headState(ctx).ToProto(), "Head did not change")
}

func TestSaveHead_Different_Reorg(t *testing.T) {
	ctx := context.Background()
	hook := logTest.NewGlobal()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	oldBlock := util.SaveBlock(t, context.Background(), service.cfg.BeaconDB, util.NewBeaconBlockZond())
	oldRoot, err := oldBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	state, blkRoot, err := prepareForkchoiceState(ctx, oldBlock.Block().Slot(), oldRoot, oldBlock.Block().ParentRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	service.head = &head{
		root:  oldRoot,
		block: oldBlock,
	}

	reorgChainParent := [32]byte{'B'}
	state, blkRoot, err = prepareForkchoiceState(ctx, 0, reorgChainParent, oldRoot, oldBlock.Block().ParentRoot(), ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))

	newHeadSignedBlock := util.NewBeaconBlockZond()
	newHeadSignedBlock.Block.Slot = 1
	newHeadSignedBlock.Block.ParentRoot = reorgChainParent[:]
	newHeadBlock := newHeadSignedBlock.Block

	wsb := util.SaveBlock(t, context.Background(), service.cfg.BeaconDB, newHeadSignedBlock)
	newRoot, err := newHeadBlock.HashTreeRoot()
	require.NoError(t, err)
	state, blkRoot, err = prepareForkchoiceState(ctx, wsb.Block().Slot(), newRoot, wsb.Block().ParentRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	headState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(1))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Slot: 1, Root: newRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveState(context.Background(), headState, newRoot))
	require.NoError(t, service.saveHead(context.Background(), newRoot, wsb, headState))

	assert.Equal(t, primitives.Slot(1), service.HeadSlot(), "Head did not change")

	cachedRoot, err := service.HeadRoot(context.Background())
	require.NoError(t, err)
	if !bytes.Equal(cachedRoot, newRoot[:]) {
		t.Error("Head did not change")
	}
	headBlock, err := service.headBlock()
	require.NoError(t, err)
	pb, err := headBlock.Proto()
	require.NoError(t, err)
	assert.DeepEqual(t, newHeadSignedBlock, pb, "Head did not change")
	assert.DeepSSZEqual(t, headState.ToProto(), service.headState(ctx).ToProto(), "Head did not change")
	require.LogsContain(t, hook, "Chain reorg occurred")
	require.LogsContain(t, hook, "distance=1")
	require.LogsContain(t, hook, "depth=1")
}

func Test_notifyNewHeadEvent(t *testing.T) {
	ctx := context.Background()

	// notifyNewHeadEvent looks up the parent block's slot in forkchoice to
	// decide whether an epoch transition occurred. Tests therefore need a
	// real forkchoice store containing the parent block.
	insertParent := func(t *testing.T, srv *Service, parentRoot [32]byte, parentSlot primitives.Slot) {
		t.Helper()
		st, blk, err := prepareForkchoiceState(ctx, parentSlot, parentRoot, [32]byte{}, [32]byte{}, &qrysmpb.Checkpoint{}, &qrysmpb.Checkpoint{})
		require.NoError(t, err)
		require.NoError(t, srv.cfg.ForkChoiceStore.InsertNode(ctx, st, blk))
	}

	t.Run("genesis_state_root", func(t *testing.T) {
		bState, _ := util.DeterministicGenesisStateZond(t, 10)
		notifier := &mock.MockStateNotifier{RecordEvents: true}
		srv := &Service{
			cfg: &config{
				StateNotifier:   notifier,
				ForkChoiceStore: doublylinkedtree.New(),
			},
			originBlockRoot: [32]byte{1},
		}
		// bState is a genesis state; its latest block header's parent is zeros.
		insertParent(t, srv, [32]byte{}, 0)
		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		err := srv.notifyNewHeadEvent(ctx, 1, bState, newHeadStateRoot[:], newHeadRoot[:])
		require.NoError(t, err)
		events := notifier.ReceivedEvents()
		require.Equal(t, 1, len(events))

		eventHead, ok := events[0].Data.(*qrlpb.EventHead)
		require.Equal(t, true, ok)
		wanted := &qrlpb.EventHead{
			Slot:                      1,
			Block:                     newHeadRoot[:],
			State:                     newHeadStateRoot[:],
			EpochTransition:           false,
			PreviousDutyDependentRoot: srv.originBlockRoot[:],
			CurrentDutyDependentRoot:  srv.originBlockRoot[:],
		}
		require.DeepSSZEqual(t, wanted, eventHead)
	})
	t.Run("non_genesis_values", func(t *testing.T) {
		bState, _ := util.DeterministicGenesisStateZond(t, 10)
		notifier := &mock.MockStateNotifier{RecordEvents: true}
		genesisRoot := [32]byte{1}
		srv := &Service{
			cfg: &config{
				StateNotifier:   notifier,
				ForkChoiceStore: doublylinkedtree.New(),
			},
			originBlockRoot: genesisRoot,
		}
		insertParent(t, srv, [32]byte{}, 0)
		epoch1Start, err := slots.EpochStart(1)
		require.NoError(t, err)
		epoch2Start, err := slots.EpochStart(1)
		require.NoError(t, err)
		require.NoError(t, bState.SetSlot(epoch1Start))

		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		err = srv.notifyNewHeadEvent(ctx, epoch2Start, bState, newHeadStateRoot[:], newHeadRoot[:])
		require.NoError(t, err)
		events := notifier.ReceivedEvents()
		require.Equal(t, 1, len(events))

		eventHead, ok := events[0].Data.(*qrlpb.EventHead)
		require.Equal(t, true, ok)
		wanted := &qrlpb.EventHead{
			Slot:                      epoch2Start,
			Block:                     newHeadRoot[:],
			State:                     newHeadStateRoot[:],
			EpochTransition:           true,
			PreviousDutyDependentRoot: genesisRoot[:],
			CurrentDutyDependentRoot:  genesisRoot[:],
		}
		require.DeepSSZEqual(t, wanted, eventHead)
	})
	// Regression: the head lands several slots into a new epoch because the
	// epoch-boundary slot was skipped. The new head's slot is not the start of
	// an epoch, but an epoch transition still happened. EpochTransition must be
	// true (previous behavior used IsEpochStart and would report false).
	t.Run("epoch_transition_skipped_boundary", func(t *testing.T) {
		bState, _ := util.DeterministicGenesisStateZond(t, 10)
		notifier := &mock.MockStateNotifier{RecordEvents: true}
		genesisRoot := [32]byte{1}
		srv := &Service{
			cfg: &config{
				StateNotifier:   notifier,
				ForkChoiceStore: doublylinkedtree.New(),
			},
			originBlockRoot: genesisRoot,
		}
		// Parent is the last block of epoch 0; head lands mid-epoch 1 because
		// the epoch-boundary slot was empty.
		spe := params.BeaconConfig().SlotsPerEpoch
		parentSlot := spe - 1
		newHeadSlot := spe + 2
		parentRoot := [32]byte{0xAA}
		insertParent(t, srv, parentRoot, parentSlot)

		// Set the bState's latest block header parent_root to parentRoot so
		// notifyNewHeadEvent looks up the right node.
		hdr := bState.LatestBlockHeader()
		hdr.ParentRoot = parentRoot[:]
		require.NoError(t, bState.SetLatestBlockHeader(hdr))
		require.NoError(t, bState.SetSlot(newHeadSlot))

		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		require.NoError(t, srv.notifyNewHeadEvent(ctx, newHeadSlot, bState, newHeadStateRoot[:], newHeadRoot[:]))

		events := notifier.ReceivedEvents()
		require.Equal(t, 1, len(events))
		eventHead, ok := events[0].Data.(*qrlpb.EventHead)
		require.Equal(t, true, ok)
		require.Equal(t, false, slots.IsEpochStart(newHeadSlot), "test setup: head slot must not be an epoch start")
		require.Equal(t, true, eventHead.EpochTransition, "epoch transition must be reported across a skipped boundary")
	})
	// Regression: when BlockRootAtSlot returns the zero hash for the previous
	// duty slot (e.g. early in chain history before block_roots is populated),
	// notifyNewHeadEvent must fall back to originBlockRoot rather than emitting
	// an all-zero PreviousDutyDependentRoot.
	t.Run("previous_dependent_root_zero_falls_back_to_origin", func(t *testing.T) {
		bState, _ := util.DeterministicGenesisStateZond(t, 10)
		notifier := &mock.MockStateNotifier{RecordEvents: true}
		genesisRoot := [32]byte{0xAB}
		srv := &Service{
			cfg: &config{
				StateNotifier:   notifier,
				ForkChoiceStore: doublylinkedtree.New(),
			},
			originBlockRoot: genesisRoot,
		}
		insertParent(t, srv, [32]byte{}, 0)
		// newHeadSlot = epoch 2 start, so previousDutySlot = epoch 1 start > 0
		// and BlockRootAtSlot(state, previousDutySlot-1) returns the zero hash
		// because the freshly-set state has no blocks recorded for that slot.
		newHeadSlot, err := slots.EpochStart(2)
		require.NoError(t, err)
		require.NoError(t, bState.SetSlot(newHeadSlot))

		newHeadStateRoot := [32]byte{2}
		newHeadRoot := [32]byte{3}
		require.NoError(t, srv.notifyNewHeadEvent(ctx, newHeadSlot, bState, newHeadStateRoot[:], newHeadRoot[:]))

		events := notifier.ReceivedEvents()
		require.Equal(t, 1, len(events))
		eventHead, ok := events[0].Data.(*qrlpb.EventHead)
		require.Equal(t, true, ok)
		assert.DeepEqual(t, genesisRoot[:], eventHead.PreviousDutyDependentRoot)
		assert.DeepEqual(t, genesisRoot[:], eventHead.CurrentDutyDependentRoot)
	})
}

func TestRetrieveHead_ReadOnly(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)

	oldBlock := util.SaveBlock(t, context.Background(), service.cfg.BeaconDB, util.NewBeaconBlockZond())
	oldRoot, err := oldBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	service.head = &head{
		root:  oldRoot,
		block: oldBlock,
	}

	newHeadSignedBlock := util.NewBeaconBlockZond()
	newHeadSignedBlock.Block.Slot = 1
	newHeadBlock := newHeadSignedBlock.Block
	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}

	wsb := util.SaveBlock(t, context.Background(), service.cfg.BeaconDB, newHeadSignedBlock)
	newRoot, err := newHeadBlock.HashTreeRoot()
	require.NoError(t, err)
	state, blkRoot, err := prepareForkchoiceState(ctx, wsb.Block().Slot()-1, wsb.Block().ParentRoot(), service.cfg.ForkChoiceStore.CachedHeadRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))

	state, blkRoot, err = prepareForkchoiceState(ctx, wsb.Block().Slot(), newRoot, wsb.Block().ParentRoot(), [32]byte{}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	headState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, headState.SetSlot(1))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Slot: 1, Root: newRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveState(context.Background(), headState, newRoot))
	require.NoError(t, service.saveHead(context.Background(), newRoot, wsb, headState))

	rOnlyState, err := service.HeadStateReadOnly(ctx)
	require.NoError(t, err)

	assert.Equal(t, rOnlyState, service.head.state, "Head is not the same object")
}

func TestSaveOrphanedAtts(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.genesisTime = time.Now().Add(time.Duration(-10*int64(1)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	// Chain setup
	// 0 -- 1 -- 2 -- 3
	//  \-4
	st, keys := util.DeterministicGenesisStateZond(t, 256)
	blkG, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)

	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blk1, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 2)
	assert.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blk2, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 3)
	assert.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blk3, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 4)
	assert.NoError(t, err)
	blk3.Block.ParentRoot = r2[:]
	r3, err := blk3.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlockZond()
	blk4.Block.Slot = 5
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)
	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}

	for _, blk := range []*qrysmpb.SignedBeaconBlockZond{blkG, blk1, blk2, blk3, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r3, r4))
	require.Equal(t, 3, service.cfg.AttPool.AggregatedAttestationCount())
	wantAtts := []*qrysmpb.Attestation{
		blk3.Block.Body.Attestations[0],
		blk2.Block.Body.Attestations[0],
		blk1.Block.Body.Attestations[0],
	}
	atts := service.cfg.AttPool.AggregatedAttestations()
	sort.Slice(atts, func(i, j int) bool {
		return atts[i].Data.Slot > atts[j].Data.Slot
	})
	require.DeepEqual(t, wantAtts, atts)
}

func TestSaveOrphanedOps(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig()
	config.ShardCommitteePeriod = 0
	params.OverrideBeaconConfig(config)

	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.genesisTime = time.Now().Add(time.Duration(-10*int64(1)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	// Chain setup
	// 0 -- 1 -- 2 -- 3
	//  \-4
	st, keys := util.DeterministicGenesisStateZond(t, 256)
	service.head = &head{state: st}
	blkG, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)

	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blk1, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 2)
	assert.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blk2, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 3)
	assert.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blkConfig := util.DefaultBlockGenConfig()
	blkConfig.NumProposerSlashings = 1
	blkConfig.NumAttesterSlashings = 1
	blkConfig.NumVoluntaryExits = 1
	blk3, err := util.GenerateFullBlockZond(st, keys, blkConfig, 4)
	assert.NoError(t, err)
	blk3.Block.ParentRoot = r2[:]
	r3, err := blk3.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlockZond()
	blk4.Block.Slot = 5
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)
	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}

	for _, blk := range []*qrysmpb.SignedBeaconBlockZond{blkG, blk1, blk2, blk3, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r3, r4))
	require.Equal(t, 3, service.cfg.AttPool.AggregatedAttestationCount())
	wantAtts := []*qrysmpb.Attestation{
		blk3.Block.Body.Attestations[0],
		blk2.Block.Body.Attestations[0],
		blk1.Block.Body.Attestations[0],
	}
	atts := service.cfg.AttPool.AggregatedAttestations()
	sort.Slice(atts, func(i, j int) bool {
		return atts[i].Data.Slot > atts[j].Data.Slot
	})
	require.DeepEqual(t, wantAtts, atts)
	require.Equal(t, 1, len(service.cfg.SlashingPool.PendingProposerSlashings(ctx, st, false)))
	require.Equal(t, 1, len(service.cfg.SlashingPool.PendingAttesterSlashings(ctx, st, false)))
	exits, err := service.cfg.ExitPool.PendingExits()
	require.NoError(t, err)
	require.Equal(t, 1, len(exits))
}

func TestSaveOrphanedAtts_CanFilter(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.genesisTime = time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SlotsPerEpoch+2)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	// Chain setup
	// 0 -- 1 -- 2
	//  \-4
	st, keys := util.DeterministicGenesisStateZond(t, 256)
	blkConfig := util.DefaultBlockGenConfig()
	blkG, err := util.GenerateFullBlockZond(st, keys, blkConfig, 1)
	assert.NoError(t, err)
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blk1, err := util.GenerateFullBlockZond(st, keys, blkConfig, 2)
	assert.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blk2, err := util.GenerateFullBlockZond(st, keys, blkConfig, 3)
	assert.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlockZond()
	blk4.Block.Slot = 4
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)
	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}

	for _, blk := range []*qrysmpb.SignedBeaconBlockZond{blkG, blk1, blk2, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r2, r4))
	require.Equal(t, 1, service.cfg.AttPool.AggregatedAttestationCount())
}

func TestSaveOrphanedAtts_DoublyLinkedTrie(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	service.genesisTime = time.Now().Add(time.Duration(-10*int64(1)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	// Chain setup
	// 0 -- 1 -- 2 -- 3
	//  \-4
	st, keys := util.DeterministicGenesisStateZond(t, 256)
	blkG, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blk1, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 2)
	assert.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blk2, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 3)
	assert.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blk3, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 4)
	assert.NoError(t, err)
	blk3.Block.ParentRoot = r2[:]
	r3, err := blk3.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlockZond()
	blk4.Block.Slot = 5
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)

	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	for _, blk := range []*qrysmpb.SignedBeaconBlockZond{blkG, blk1, blk2, blk3, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r3, r4))
	require.Equal(t, 3, service.cfg.AttPool.AggregatedAttestationCount())
	wantAtts := []*qrysmpb.Attestation{
		blk3.Block.Body.Attestations[0],
		blk2.Block.Body.Attestations[0],
		blk1.Block.Body.Attestations[0],
	}
	atts := service.cfg.AttPool.AggregatedAttestations()
	sort.Slice(atts, func(i, j int) bool {
		return atts[i].Data.Slot > atts[j].Data.Slot
	})
	require.DeepEqual(t, wantAtts, atts)
}

func TestSaveOrphanedAtts_CanFilter_DoublyLinkedTrie(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	service := setupBeaconChain(t, beaconDB)
	// service.genesisTime = time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SlotsPerEpoch+2)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)
	service.genesisTime = time.Now().Add(time.Duration(-1*int64(params.BeaconConfig().SlotsPerEpoch+3)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	// Chain setup
	// 0 -- 1 -- 2
	//  \-4
	st, keys := util.DeterministicGenesisStateZond(t, 256)
	blkG, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, blkG)
	rG, err := blkG.Block.HashTreeRoot()
	require.NoError(t, err)

	blk1, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 2)
	assert.NoError(t, err)
	blk1.Block.ParentRoot = rG[:]
	r1, err := blk1.Block.HashTreeRoot()
	require.NoError(t, err)

	blk2, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), 3)
	assert.NoError(t, err)
	blk2.Block.ParentRoot = r1[:]
	r2, err := blk2.Block.HashTreeRoot()
	require.NoError(t, err)

	blk4 := util.NewBeaconBlockZond()
	blk4.Block.Slot = 5
	blk4.Block.ParentRoot = rG[:]
	r4, err := blk4.Block.HashTreeRoot()
	require.NoError(t, err)

	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	for _, blk := range []*qrysmpb.SignedBeaconBlockZond{blkG, blk1, blk2, blk4} {
		r, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		state, blkRoot, err := prepareForkchoiceState(ctx, blk.Block.Slot, r, bytesutil.ToBytes32(blk.Block.ParentRoot), [32]byte{}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
		util.SaveBlock(t, ctx, beaconDB, blk)
	}

	require.NoError(t, service.saveOrphanedOperations(ctx, r2, r4))
	require.Equal(t, 0, service.cfg.AttPool.AggregatedAttestationCount())
}

func TestUpdateHead_noSavedChanges(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB, fcs := tr.ctx, tr.db, tr.fcs

	ojp := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	st, blkRoot, err := prepareForkchoiceState(ctx, 0, [32]byte{}, [32]byte{}, [32]byte{}, ojp, ojp)
	require.NoError(t, err)
	require.NoError(t, fcs.InsertNode(ctx, st, blkRoot))

	bellatrixBlk := util.SaveBlock(t, ctx, beaconDB, util.NewBeaconBlockZond())
	bellatrixBlkRoot, err := bellatrixBlk.Block().HashTreeRoot()
	require.NoError(t, err)
	fcp := &qrysmpb.Checkpoint{
		Root:  bellatrixBlkRoot[:],
		Epoch: 0,
	}
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, bellatrixBlkRoot))

	bellatrixState, _ := util.DeterministicGenesisStateZond(t, 2)
	require.NoError(t, beaconDB.SaveState(ctx, bellatrixState, bellatrixBlkRoot))
	service.cfg.StateGen.SaveFinalizedState(0, bellatrixBlkRoot, bellatrixState)

	headRoot := service.headRoot()
	require.Equal(t, [32]byte{}, headRoot)

	st, blkRoot, err = prepareForkchoiceState(ctx, 0, bellatrixBlkRoot, [32]byte{}, [32]byte{}, fcp, fcp)
	require.NoError(t, err)
	require.NoError(t, fcs.InsertNode(ctx, st, blkRoot))
	fcs.SetBalancesByRooter(func(context.Context, [32]byte) ([]uint64, error) { return []uint64{1, 2}, nil })
	require.NoError(t, fcs.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{}))
	newRoot, err := service.cfg.ForkChoiceStore.Head(ctx)
	require.NoError(t, err)
	require.NotEqual(t, headRoot, newRoot)
	require.Equal(t, headRoot, service.headRoot())
}
