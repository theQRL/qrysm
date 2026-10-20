package blockchain

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/db"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/protobuf/proto"
)

// Ensure Service implements chain info interface.
var _ ChainInfoFetcher = (*Service)(nil)
var _ TimeFetcher = (*Service)(nil)
var _ ForkFetcher = (*Service)(nil)

// prepareForkchoiceState prepares a beacon state with the given data to mock
// insert into forkchoice
func prepareForkchoiceState(
	_ context.Context,
	slot primitives.Slot,
	blockRoot [32]byte,
	parentRoot [32]byte,
	payloadHash [32]byte,
	justified *qrysmpb.Checkpoint,
	finalized *qrysmpb.Checkpoint,
) (state.BeaconState, blocks.ROBlock, error) {
	blockHeader := &qrysmpb.BeaconBlockHeader{
		ParentRoot: parentRoot[:],
	}

	executionHeader := &enginev1.ExecutionPayloadHeaderZond{
		BlockHash: payloadHash[:],
	}

	base := &qrysmpb.BeaconStateZond{
		Slot:                         slot,
		RandaoMixes:                  make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		BlockRoots:                   make([][]byte, 1),
		CurrentJustifiedCheckpoint:   justified,
		FinalizedCheckpoint:          finalized,
		LatestExecutionPayloadHeader: executionHeader,
		LatestBlockHeader:            blockHeader,
	}

	base.BlockRoots[0] = append(base.BlockRoots[0], blockRoot[:]...)
	st, err := state_native.InitializeFromProtoZond(base)
	if err != nil {
		return nil, blocks.ROBlock{}, err
	}

	blk := &qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			Slot:       slot,
			ParentRoot: parentRoot[:],
			Body: &qrysmpb.BeaconBlockBodyZond{
				ExecutionPayload: &enginev1.ExecutionPayloadZond{
					BlockHash: payloadHash[:],
				},
			},
		},
	}
	signed, err := blocks.NewSignedBeaconBlock(blk)
	if err != nil {
		return nil, blocks.ROBlock{}, err
	}
	roblock, err := blocks.NewROBlockWithRoot(signed, blockRoot)
	return st, roblock, err
}

func TestHeadRoot_Nil(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	c := setupBeaconChain(t, beaconDB)
	headRoot, err := c.HeadRoot(context.Background())
	require.NoError(t, err)
	assert.DeepEqual(t, params.BeaconConfig().ZeroHash[:], headRoot, "Incorrect pre chain start value")
}

func TestFinalizedCheckpt_GenesisRootOk(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, fcs := tr.ctx, tr.fcs

	gs, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))
	cp := service.FinalizedCheckpt()
	assert.DeepEqual(t, [32]byte{}, bytesutil.ToBytes32(cp.Root))
	cp = service.CurrentJustifiedCheckpt()
	assert.DeepEqual(t, [32]byte{}, bytesutil.ToBytes32(cp.Root))
	// check that forkchoice has the right genesis root as the node root
	root, err := fcs.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, service.originBlockRoot, root)

}

func TestCurrentJustifiedCheckpt_CanRetrieve(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB, fcs := tr.ctx, tr.db, tr.fcs

	jroot := [32]byte{'j'}
	cp := &forkchoicetypes.Checkpoint{Epoch: 6, Root: jroot}
	bState, _ := util.DeterministicGenesisStateZond(t, 10)
	require.NoError(t, beaconDB.SaveState(ctx, bState, jroot))

	require.NoError(t, fcs.UpdateJustifiedCheckpoint(ctx, cp))
	jp := service.CurrentJustifiedCheckpt()
	assert.Equal(t, cp.Epoch, jp.Epoch, "Unexpected justified epoch")
	require.Equal(t, cp.Root, bytesutil.ToBytes32(jp.Root))
}

func TestFinalizedBlockHash(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB, fcs := tr.ctx, tr.db, tr.fcs

	r := [32]byte{'f'}
	cp := &forkchoicetypes.Checkpoint{Epoch: 6, Root: r}
	bState, _ := util.DeterministicGenesisStateZond(t, 10)
	require.NoError(t, beaconDB.SaveState(ctx, bState, r))

	require.NoError(t, fcs.UpdateFinalizedCheckpoint(cp))
	h := service.FinalizedBlockHash()
	require.Equal(t, params.BeaconConfig().ZeroHash, h)
	require.Equal(t, r, fcs.FinalizedCheckpoint().Root)
}

func TestUnrealizedJustifiedBlockHash(t *testing.T) {
	ctx := context.Background()
	service := &Service{cfg: &config{ForkChoiceStore: doublylinkedtree.New()}}
	ojc := &qrysmpb.Checkpoint{Root: []byte{'j'}}
	ofc := &qrysmpb.Checkpoint{Root: []byte{'f'}}
	st, blkRoot, err := prepareForkchoiceState(ctx, 0, [32]byte{}, [32]byte{}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
	service.cfg.ForkChoiceStore.SetBalancesByRooter(func(_ context.Context, _ *forkchoicetypes.Checkpoint) (*forkchoicetypes.JustifiedBalances, error) {
		return &forkchoicetypes.JustifiedBalances{}, nil
	})
	require.NoError(t, service.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: 6, Root: [32]byte{'j'}}))

	h := service.UnrealizedJustifiedPayloadBlockHash()
	require.Equal(t, params.BeaconConfig().ZeroHash, h)
	require.Equal(t, [32]byte{'j'}, service.cfg.ForkChoiceStore.JustifiedCheckpoint().Root)
}

func TestService_ShouldIgnoreData(t *testing.T) {
	ctx := context.Background()
	service := &Service{cfg: &config{ForkChoiceStore: doublylinkedtree.New()}}

	// Drive CurrentSlot to be in epoch 2.
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	currentSlot := primitives.Slot(2 * slotsPerEpoch)
	service.genesisTime = time.Now().Add(-time.Duration(uint64(currentSlot)*params.BeaconConfig().SecondsPerSlot) * time.Second)
	service.cfg.ForkChoiceStore.SetGenesisTime(uint64(service.genesisTime.Unix()))

	zeroHash := params.BeaconConfig().ZeroHash
	ojc := &qrysmpb.Checkpoint{Root: zeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: zeroHash[:]}

	// Build chain in forkchoice:
	//   genesis (slot 0, epoch 0)
	//      └── nodeA (slot 1, epoch 0)
	//            └── nodeB (slot=slotsPerEpoch, epoch 1)
	stRoot, robRoot, err := prepareForkchoiceState(ctx, 0, zeroHash, [32]byte{}, zeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, stRoot, robRoot))

	nodeARoot := [32]byte{1}
	stA, robA, err := prepareForkchoiceState(ctx, 1, nodeARoot, zeroHash, [32]byte{10}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, stA, robA))

	nodeBRoot := [32]byte{2}
	stB, robB, err := prepareForkchoiceState(ctx, primitives.Slot(slotsPerEpoch), nodeBRoot, nodeARoot, [32]byte{11}, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, stB, robB))

	service.cfg.ForkChoiceStore.SetBalancesByRooter(func(_ context.Context, _ *forkchoicetypes.Checkpoint) (*forkchoicetypes.JustifiedBalances, error) {
		return &forkchoicetypes.JustifiedBalances{}, nil
	})
	// Justify nodeB (epoch 1).
	require.NoError(t, service.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(
		ctx, &forkchoicetypes.Checkpoint{Epoch: 1, Root: nodeBRoot}))
	headRoot, err := service.cfg.ForkChoiceStore.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, nodeBRoot, headRoot)

	t.Run("data from a past epoch is not ignored", func(t *testing.T) {
		// dataSlot in epoch 1 < currentEpoch 2.
		pastSlot := primitives.Slot(slotsPerEpoch)
		require.Equal(t, false, service.ShouldIgnoreData(nodeARoot, pastSlot))
	})

	t.Run("parent not in forkchoice is not ignored", func(t *testing.T) {
		require.Equal(t, false, service.ShouldIgnoreData([32]byte{99}, currentSlot))
	})

	t.Run("parent epoch >= justified is not ignored", func(t *testing.T) {
		// nodeB at epoch 1, justified epoch 1, so parentEpoch >= justified.
		require.Equal(t, false, service.ShouldIgnoreData(nodeBRoot, currentSlot))
	})

	t.Run("canonical parent before justified is ignored", func(t *testing.T) {
		// nodeA at epoch 0 < justified epoch 1, and is on the canonical chain.
		require.Equal(t, true, service.ShouldIgnoreData(nodeARoot, currentSlot))
	})

	t.Run("non-canonical parent before justified is not ignored", func(t *testing.T) {
		// Branch nodeD at slot 2 (epoch 0) off nodeA — not on the canonical chain.
		nodeDRoot := [32]byte{4}
		stD, robD, err := prepareForkchoiceState(ctx, 2, nodeDRoot, nodeARoot, [32]byte{13}, ojc, ofc)
		require.NoError(t, err)
		require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, stD, robD))

		require.Equal(t, false, service.ShouldIgnoreData(nodeDRoot, currentSlot))
	})
}

func TestService_ShouldIgnoreData_SkippedCheckpointSlot(t *testing.T) {
	for _, tc := range []struct {
		name  string
		epoch primitives.Epoch
	}{
		{name: "skipped checkpoint slot", epoch: 1},
		{name: "empty epoch", epoch: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store := doublylinkedtree.New()
			store.SetBalancesByRooter(func(context.Context, *forkchoicetypes.Checkpoint) (*forkchoicetypes.JustifiedBalances, error) {
				return &forkchoicetypes.JustifiedBalances{}, nil
			})
			currentSlot := primitives.Slot(uint64(tc.epoch+1) * uint64(params.BeaconConfig().SlotsPerEpoch))
			service := &Service{cfg: &config{ForkChoiceStore: store}}
			service.genesisTime = time.Now().Add(-time.Duration(uint64(currentSlot)*params.BeaconConfig().SecondsPerSlot) * time.Second)
			store.SetGenesisTime(uint64(service.genesisTime.Unix()))
			zero := params.BeaconConfig().ZeroHash
			cp := &qrysmpb.Checkpoint{Root: zero[:]}
			genesisState, genesisBlock, err := prepareForkchoiceState(ctx, 0, zero, zero, zero, cp, cp)
			require.NoError(t, err)
			require.NoError(t, store.InsertNode(ctx, genesisState, genesisBlock))

			// Skipping the checkpoint slot, or an entire epoch, leaves the
			// justified root at the last block from epoch zero.
			checkpointRoot := [32]byte{'c'}
			checkpointSlot := primitives.Slot(params.BeaconConfig().SlotsPerEpoch - 1)
			checkpointState, checkpointBlock, err := prepareForkchoiceState(ctx, checkpointSlot, checkpointRoot, zero, [32]byte{'e'}, cp, cp)
			require.NoError(t, err)
			require.NoError(t, store.InsertNode(ctx, checkpointState, checkpointBlock))
			require.NoError(t, store.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: tc.epoch, Root: checkpointRoot}))
			headRoot, err := store.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, checkpointRoot, headRoot)

			// An earlier ancestor still forks before justification, while
			// building directly on the justified block preserves it.
			require.Equal(t, true, service.ShouldIgnoreData(zero, currentSlot))
			require.Equal(t, false, service.ShouldIgnoreData(checkpointRoot, currentSlot))
		})
	}
}

func TestHeadSlot_CanRetrieve(t *testing.T) {
	c := &Service{}
	s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	b, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
	require.NoError(t, err)
	b.SetSlot(100)
	c.head = &head{block: b, state: s}
	assert.Equal(t, primitives.Slot(100), c.HeadSlot())
}

func TestHeadRoot_CanRetrieve(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	gs, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, service.saveGenesisData(ctx, gs))

	r, err := service.HeadRoot(ctx)
	require.NoError(t, err)
	assert.Equal(t, service.originBlockRoot, bytesutil.ToBytes32(r))
}

func TestHeadRoot_UseDB(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB := tr.ctx, tr.db

	service.head = &head{root: params.BeaconConfig().ZeroHash}
	b := util.NewBeaconBlockZond()
	br, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveBlock(ctx, wsb))
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: br[:]}))
	require.NoError(t, beaconDB.SaveHeadBlockRoot(ctx, br))
	r, err := service.HeadRoot(ctx)
	require.NoError(t, err)
	assert.Equal(t, br, bytesutil.ToBytes32(r))
}

func TestHeadBlock_CanRetrieve(t *testing.T) {
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	c := &Service{}
	c.head = &head{block: wsb, state: s}

	received, err := c.HeadBlock(context.Background())
	require.NoError(t, err)
	pb, err := received.Proto()
	require.NoError(t, err)
	assert.DeepEqual(t, b, pb, "Incorrect head block received")
}

func TestHeadState_CanRetrieve(t *testing.T) {
	s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Slot: 2, GenesisValidatorsRoot: params.BeaconConfig().ZeroHash[:]})
	require.NoError(t, err)
	c := &Service{}
	c.head = &head{state: s}
	headState, err := c.HeadState(context.Background())
	require.NoError(t, err)
	assert.DeepEqual(t, headState.ToProtoUnsafe(), s.ToProtoUnsafe(), "Incorrect head state received")
}

func TestHeadStateAndRoot_Publication(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "gossip", true: "batch"}[batch], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 3)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			c := cache.NewAttestationCache()
			f.s.cfg.AttestationCache = c
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 3, 0)
				oldState, oldRoot, err := f.s.HeadStateAndRoot(f.ctx)
				require.NoError(t, err)
				req := &qrysmpb.AttestationDataRequest{Slot: 3}
				oldData := &qrysmpb.AttestationData{Slot: 3, BeaconBlockRoot: oldRoot}
				require.NoError(t, c.Put(f.ctx, req, oldData))
				pending := &qrysmpb.AttestationDataRequest{Slot: 2}
				require.NoError(t, c.MarkInProgress(pending))
				if batch {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				} else {
					require.NoError(t, f.s.ReceiveBlock(f.ctx, f.blks[2], f.blks[2].Root()))
				}
				synctest.Wait()
				stale, err := c.Get(f.ctx, req)
				require.NoError(t, err)
				assert.Equal(t, (*qrysmpb.AttestationData)(nil), stale)
				require.ErrorIs(t, c.Put(f.ctx, pending, oldData), cache.ErrAttestationDataStale)
				require.NoError(t, c.MarkNotInProgress(pending))
				st, root, err := f.s.HeadStateAndRoot(f.ctx)
				require.NoError(t, err)
				assert.Equal(t, f.blks[2].Root(), bytesutil.ToBytes32(root))
				hash, err := st.HashTreeRoot(f.ctx)
				require.NoError(t, err)
				assert.Equal(t, f.blks[2].Block().StateRoot(), hash)
				assert.Equal(t, f.blks[1].Root(), bytesutil.ToBytes32(oldRoot))
				assert.Equal(t, primitives.Slot(2), oldState.Slot())
				// Both pieces returned to the RPC must be independent copies.
				root[0] ^= 1
				require.NoError(t, st.SetSlot(100))
				st, root, err = f.s.HeadStateAndRoot(f.ctx)
				require.NoError(t, err)
				assert.Equal(t, primitives.Slot(3), st.Slot())
				assert.Equal(t, f.blks[2].Root(), bytesutil.ToBytes32(root))
				if !batch {
					// The fallback must load the state for the durable head root,
					// even when the in-memory head has not been initialized.
					f.s.head = nil
					st, root, err = f.s.HeadStateAndRoot(f.ctx)
					require.NoError(t, err)
					assert.Equal(t, f.blks[2].Root(), bytesutil.ToBytes32(root))
					hash, err = st.HashTreeRoot(f.ctx)
					require.NoError(t, err)
					assert.Equal(t, f.blks[2].Block().StateRoot(), hash)
				}
			})
		})
	}
}

func TestGenesisTime_CanRetrieve(t *testing.T) {
	c := &Service{genesisTime: time.Unix(999, 0)}
	wanted := time.Unix(999, 0)
	assert.Equal(t, wanted, c.GenesisTime(), "Did not get wanted genesis time")
}

func TestCurrentFork_CanRetrieve(t *testing.T) {
	f := &qrysmpb.Fork{Epoch: 999}
	s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Fork: f})
	require.NoError(t, err)
	c := &Service{}
	c.head = &head{state: s}
	if !proto.Equal(c.CurrentFork(), f) {
		t.Error("Received incorrect fork version")
	}
}

func TestCurrentFork_NilHeadSTate(t *testing.T) {
	f := &qrysmpb.Fork{
		PreviousVersion: params.BeaconConfig().GenesisForkVersion,
		CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
	}
	c := &Service{}
	if !proto.Equal(c.CurrentFork(), f) {
		t.Error("Received incorrect fork version")
	}
}

func TestGenesisValidatorsRoot_CanRetrieve(t *testing.T) {
	// Should not panic if head state is nil.
	c := &Service{}
	assert.Equal(t, [32]byte{}, c.GenesisValidatorsRoot(), "Did not get correct genesis validators root")

	s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{GenesisValidatorsRoot: []byte{'a'}})
	require.NoError(t, err)
	c.head = &head{state: s}
	assert.Equal(t, [32]byte{'a'}, c.GenesisValidatorsRoot(), "Did not get correct genesis validators root")
}

func TestHeadExecutionData_Nil(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	c := setupBeaconChain(t, beaconDB)
	assert.DeepEqual(t, &qrysmpb.ExecutionData{}, c.HeadExecutionData(), "Incorrect pre chain start value")
}

func TestHeadExecutionData_CanRetrieve(t *testing.T) {
	d := &qrysmpb.ExecutionData{DepositCount: 999}
	s, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{ExecutionData: d})
	require.NoError(t, err)
	c := &Service{}
	c.head = &head{state: s}
	if !proto.Equal(c.HeadExecutionData(), d) {
		t.Error("Received incorrect execution data")
	}
}

func TestIsCanonical_Ok(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	c := setupBeaconChain(t, beaconDB)

	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = 0
	root, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, blk)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, root))
	can, err := c.IsCanonical(ctx, root)
	require.NoError(t, err)
	assert.Equal(t, true, can)

	can, err = c.IsCanonical(ctx, [32]byte{'a'})
	require.NoError(t, err)
	assert.Equal(t, false, can)
}

func TestService_HeadValidatorsIndices(t *testing.T) {
	s, _ := util.DeterministicGenesisStateZond(t, 10)
	c := &Service{}

	c.head = &head{}
	indices, err := c.HeadValidatorsIndices(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, 0, len(indices))

	c.head = &head{state: s}
	indices, err = c.HeadValidatorsIndices(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, 10, len(indices))
}

func TestService_HeadGenesisValidatorsRoot(t *testing.T) {
	s, _ := util.DeterministicGenesisStateZond(t, 1)
	c := &Service{}

	c.head = &head{}
	root := c.HeadGenesisValidatorsRoot()
	require.Equal(t, [32]byte{}, root)

	c.head = &head{state: s}
	root = c.HeadGenesisValidatorsRoot()
	require.DeepEqual(t, root[:], s.GenesisValidatorsRoot())
}

//
//  A <- B <- C
//   \    \
//    \    ---------- E
//     ---------- D

func TestService_ChainHeads(t *testing.T) {
	ctx := context.Background()
	c := &Service{cfg: &config{ForkChoiceStore: doublylinkedtree.New()}}
	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	st, blkRoot, err := prepareForkchoiceState(ctx, 0, [32]byte{}, [32]byte{}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, c.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
	st, blkRoot, err = prepareForkchoiceState(ctx, 100, [32]byte{'a'}, [32]byte{}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, c.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
	st, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, c.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
	st, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'c'}, [32]byte{'b'}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, c.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
	st, blkRoot, err = prepareForkchoiceState(ctx, 103, [32]byte{'d'}, [32]byte{'a'}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, c.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
	st, blkRoot, err = prepareForkchoiceState(ctx, 104, [32]byte{'e'}, [32]byte{'b'}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, c.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))

	roots, slots := c.ChainHeads()
	require.Equal(t, 3, len(roots))
	rootMap := map[[32]byte]primitives.Slot{{'c'}: 102, {'d'}: 103, {'e'}: 104}
	for i, root := range roots {
		slot, ok := rootMap[root]
		require.Equal(t, true, ok)
		require.Equal(t, slot, slots[i])
	}
}

func TestService_HeadPublicKeyToValidatorIndex(t *testing.T) {
	s, _ := util.DeterministicGenesisStateZond(t, 10)
	c := &Service{}
	c.head = &head{state: s}

	_, e := c.HeadPublicKeyToValidatorIndex([field_params.MLDSA87PubkeyLength]byte{})
	require.Equal(t, false, e)

	v, err := s.ValidatorAtIndex(0)
	require.NoError(t, err)

	i, e := c.HeadPublicKeyToValidatorIndex(bytesutil.ToBytes2592(v.PublicKey))
	require.Equal(t, true, e)
	require.Equal(t, primitives.ValidatorIndex(0), i)
}

func TestService_HeadPublicKeyToValidatorIndexNil(t *testing.T) {
	c := &Service{}
	c.head = nil

	idx, e := c.HeadPublicKeyToValidatorIndex([field_params.MLDSA87PubkeyLength]byte{})
	require.Equal(t, false, e)
	require.Equal(t, primitives.ValidatorIndex(0), idx)

	c.head = &head{state: nil}
	i, e := c.HeadPublicKeyToValidatorIndex([field_params.MLDSA87PubkeyLength]byte{})
	require.Equal(t, false, e)
	require.Equal(t, primitives.ValidatorIndex(0), i)
}

func TestService_HeadValidatorIndexToPublicKey(t *testing.T) {
	s, _ := util.DeterministicGenesisStateZond(t, 10)
	c := &Service{}
	c.head = &head{state: s}

	p, err := c.HeadValidatorIndexToPublicKey(context.Background(), 0)
	require.NoError(t, err)

	v, err := s.ValidatorAtIndex(0)
	require.NoError(t, err)

	require.Equal(t, bytesutil.ToBytes2592(v.PublicKey), p)
}

func TestService_HeadValidatorIndexToPublicKeyNil(t *testing.T) {
	c := &Service{}
	c.head = nil

	p, err := c.HeadValidatorIndexToPublicKey(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, [field_params.MLDSA87PubkeyLength]byte{}, p)

	c.head = &head{state: nil}
	p, err = c.HeadValidatorIndexToPublicKey(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, [field_params.MLDSA87PubkeyLength]byte{}, p)
}

func TestService_IsOptimistic(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	ctx := context.Background()
	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	c := &Service{cfg: &config{ForkChoiceStore: doublylinkedtree.New()}, head: &head{root: [32]byte{'b'}}}
	st, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, [32]byte{}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, c.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
	st, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, c.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))

	opt, err := c.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), c.CurrentSlot())
	require.Equal(t, false, opt)

	c.SetGenesisTime(time.Now().Add(-time.Second * time.Duration(4*params.BeaconConfig().SecondsPerSlot)))
	opt, err = c.IsOptimistic(ctx)
	require.NoError(t, err)
	require.Equal(t, true, opt)
}

func TestService_IsOptimisticForRoot(t *testing.T) {
	ctx := context.Background()
	c := &Service{cfg: &config{ForkChoiceStore: doublylinkedtree.New()}, head: &head{root: [32]byte{'b'}}}
	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	st, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, [32]byte{}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, c.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
	st, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, c.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))

	opt, err := c.IsOptimisticForRoot(ctx, [32]byte{'a'})
	require.NoError(t, err)
	require.Equal(t, true, opt)
}

func TestService_IsOptimisticForRoot_DB(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := context.Background()
	c := &Service{cfg: &config{BeaconDB: beaconDB, ForkChoiceStore: doublylinkedtree.New()}, head: &head{root: [32]byte{'b'}}}
	c.head = &head{root: params.BeaconConfig().ZeroHash}
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 10
	br, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b)
	require.NoError(t, beaconDB.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Root: br[:], Slot: 10}))

	optimisticBlock := util.NewBeaconBlockZond()
	optimisticBlock.Block.Slot = 97
	optimisticRoot, err := optimisticBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, optimisticBlock)

	validatedBlock := util.NewBeaconBlockZond()
	validatedBlock.Block.Slot = 9
	validatedRoot, err := validatedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, validatedBlock)

	validatedCheckpoint := &qrysmpb.Checkpoint{Root: br[:]}
	require.NoError(t, beaconDB.SaveLastValidatedCheckpoint(ctx, validatedCheckpoint))

	optimistic, err := c.IsOptimisticForRoot(ctx, optimisticRoot)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)

	require.NoError(t, beaconDB.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Root: validatedRoot[:], Slot: 9}))
	cp := &qrysmpb.Checkpoint{
		Epoch: 1,
		Root:  validatedRoot[:],
	}
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, validatedRoot))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, cp))
	validated, err := c.IsOptimisticForRoot(ctx, validatedRoot)
	require.NoError(t, err)
	require.Equal(t, false, validated)

	// Before the first finalized epoch, finalized root could be zeros.
	validatedCheckpoint = &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, br))
	require.NoError(t, beaconDB.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Root: params.BeaconConfig().ZeroHash[:], Slot: 10}))
	require.NoError(t, beaconDB.SaveLastValidatedCheckpoint(ctx, validatedCheckpoint))

	require.NoError(t, beaconDB.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Root: optimisticRoot[:], Slot: 11}))
	optimistic, err = c.IsOptimisticForRoot(ctx, optimisticRoot)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)
}

func TestService_IsOptimisticForRoot_DB_non_canonical(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := context.Background()
	c := &Service{cfg: &config{BeaconDB: beaconDB, ForkChoiceStore: doublylinkedtree.New()}, head: &head{root: [32]byte{'b'}}}
	c.head = &head{root: params.BeaconConfig().ZeroHash}
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 10
	br, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b)
	require.NoError(t, beaconDB.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Root: br[:], Slot: 10}))

	optimisticBlock := util.NewBeaconBlockZond()
	optimisticBlock.Block.Slot = 97
	optimisticRoot, err := optimisticBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, optimisticBlock)

	validatedBlock := util.NewBeaconBlockZond()
	validatedBlock.Block.Slot = 9
	validatedRoot, err := validatedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, validatedBlock)

	validatedCheckpoint := &qrysmpb.Checkpoint{Root: br[:]}
	require.NoError(t, beaconDB.SaveLastValidatedCheckpoint(ctx, validatedCheckpoint))

	require.NoError(t, beaconDB.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Root: optimisticRoot[:], Slot: 11}))
	optimistic, err := c.IsOptimisticForRoot(ctx, optimisticRoot)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)

	require.NoError(t, beaconDB.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Root: validatedRoot[:], Slot: 9}))
	validated, err := c.IsOptimisticForRoot(ctx, validatedRoot)
	require.NoError(t, err)
	require.Equal(t, true, validated)

}

// Regression test for upstream PR #16969: when the validated checkpoint's
// state summary is missing, recovery must use the checkpoint root, not the
// queried root, so the optimism slot comparison is done against the correct
// summary.
func TestService_IsOptimisticForRoot_RecoverLastValidated(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := context.Background()
	c := &Service{cfg: &config{BeaconDB: beaconDB, ForkChoiceStore: doublylinkedtree.New()}, head: &head{root: params.BeaconConfig().ZeroHash}}

	cpBlock := util.NewBeaconBlockZond()
	cpBlock.Block.Slot = 1
	cpRoot, err := cpBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, cpBlock)
	// Save a full state (not a summary) for the checkpoint root so the
	// checkpoint save below does not create a summary on its own — the summary
	// must be absent at query time to exercise the recovery path.
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveState(ctx, st, cpRoot))
	require.NoError(t, beaconDB.SaveLastValidatedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: cpRoot[:]}))

	// The queried block is canonical and one slot past the validated
	// checkpoint, so it must be reported as optimistic.
	qBlock := util.NewBeaconBlockZond()
	qBlock.Block.Slot = 2
	qRoot, err := qBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, qBlock)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: qRoot[:], Slot: 2}))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, qRoot))

	optimistic, err := c.IsOptimisticForRoot(ctx, qRoot)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)
}

func TestService_IsOptimisticForRoot_StateSummaryRecovered(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := context.Background()
	c := &Service{cfg: &config{BeaconDB: beaconDB, ForkChoiceStore: doublylinkedtree.New()}, head: &head{root: [32]byte{'b'}}}
	c.head = &head{root: params.BeaconConfig().ZeroHash}
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 10
	br, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b)
	cpRoot := [32]byte{'v'}
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: cpRoot[:], Slot: 0}))
	require.NoError(t, beaconDB.SaveLastValidatedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: cpRoot[:]}))
	_, err = c.IsOptimisticForRoot(ctx, br)
	assert.NoError(t, err)
	summ, err := beaconDB.StateSummary(ctx, br)
	assert.NoError(t, err)
	assert.NotNil(t, summ)
	assert.Equal(t, 10, int(summ.Slot))
	assert.DeepEqual(t, br[:], summ.Root)
}

type deleteOnSummaryRecoveryDB struct {
	db.HeadAccessDatabase
}

func (d *deleteOnSummaryRecoveryDB) Block(ctx context.Context, root [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	// Reproduce cleanup completing just before the summary-recovery read
	// reaches the database.
	if err := d.HeadAccessDatabase.DeleteBlock(ctx, root); err != nil {
		return nil, err
	}
	return d.HeadAccessDatabase.Block(ctx, root)
}

func TestService_IsOptimisticForRoot_BlockDeletedDuringSummaryRecovery(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	c := &Service{
		cfg: &config{
			BeaconDB:        &deleteOnSummaryRecoveryDB{HeadAccessDatabase: beaconDB},
			ForkChoiceStore: doublylinkedtree.New(),
		},
		head: &head{root: [32]byte{'h'}},
	}
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 10
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b)
	require.Equal(t, true, beaconDB.HasBlock(ctx, root))
	require.Equal(t, false, beaconDB.HasStateSummary(ctx, root))

	optimistic, err := c.IsOptimisticForRoot(ctx, root)
	require.ErrorIs(t, err, errBlockDoesNotExist)
	require.Equal(t, true, optimistic)
	require.Equal(t, false, beaconDB.HasBlock(ctx, root))
	require.Equal(t, false, beaconDB.HasStateSummary(ctx, root))
}

func TestService_IsFinalized(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := context.Background()
	c := &Service{cfg: &config{BeaconDB: beaconDB, ForkChoiceStore: doublylinkedtree.New()}}
	r1 := [32]byte{'a'}
	require.NoError(t, c.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{
		Root: r1,
	}))
	b := util.NewBeaconBlockZond()
	br, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, b)
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: br[:], Slot: 10}))
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, br))
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{
		Root: br[:],
	}))
	require.Equal(t, true, c.IsFinalized(ctx, r1))
	require.Equal(t, true, c.IsFinalized(ctx, br))
	require.Equal(t, false, c.IsFinalized(ctx, [32]byte{'c'}))
}

func TestService_FinalizedIndexExcludesRecentForks(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	store := doublylinkedtree.New()
	c := &Service{cfg: &config{BeaconDB: beaconDB, ForkChoiceStore: store}, head: &head{root: [32]byte{'h'}}}
	var roots [][32]byte
	checkpointSlot := primitives.Slot(params.BeaconConfig().SlotsPerEpoch)
	for i, slot := range []primitives.Slot{0, 1, checkpointSlot, checkpointSlot, checkpointSlot + 1} {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = slot
		b.Block.Body.Graffiti[0] = byte(i)
		if i > 0 {
			parent := roots[0]
			if i > 1 {
				parent = roots[1]
			}
			b.Block.ParentRoot = parent[:]
		}
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, beaconDB, b)
		require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: slot, Root: root[:]}))
		roots = append(roots, root)
	}
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, roots[0]))
	cp := &qrysmpb.Checkpoint{Epoch: 1, Root: roots[2][:]}
	require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, cp))
	require.NoError(t, beaconDB.SaveLastValidatedCheckpoint(ctx, cp))

	for _, tc := range []struct {
		name  string
		epoch primitives.Epoch
	}{
		{name: "persisted finality", epoch: 1},
		{name: "forkchoice ahead of persistence", epoch: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, store.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: tc.epoch, Root: roots[2]}))
			for i, name := range []string{"genesis", "ancestor", "checkpoint", "fork at checkpoint slot", "fork after checkpoint slot"} {
				t.Run(name, func(t *testing.T) {
					root := roots[i]
					// The DB index protects every recent block from deletion. It is
					// not proof that the block belongs to the finalized chain.
					require.Equal(t, true, beaconDB.IsFinalizedBlock(ctx, root))
					wantFinalized := i < 3
					canonical, err := c.IsCanonical(ctx, root)
					require.NoError(t, err)
					assert.Equal(t, wantFinalized, canonical)
					assert.Equal(t, wantFinalized, c.IsFinalized(ctx, root))
					optimistic, err := c.IsOptimisticForRoot(ctx, root)
					require.NoError(t, err)
					assert.Equal(t, !wantFinalized, optimistic)
				})
			}
		})
	}
}

func Test_hashForGenesisBlock(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := context.Background()
	c := setupBeaconChain(t, beaconDB)
	st, _ := util.DeterministicGenesisStateZond(t, 10)
	require.NoError(t, c.cfg.BeaconDB.SaveGenesisData(ctx, st))
	root, err := beaconDB.GenesisBlockRoot(ctx)
	require.NoError(t, err)

	// Non-genesis root should return errNotGenesisRoot.
	got, err := c.hashForGenesisBlock(ctx, [32]byte{'a'})
	require.ErrorIs(t, err, errNotGenesisRoot)
	require.Equal(t, 0, len(got))

	// Genesis root should return the BlockHash from the genesis state's
	// LatestExecutionPayloadHeader. For DeterministicGenesisStateZond this
	// is all-zeros, but the call must not error.
	got, err = c.hashForGenesisBlock(ctx, root)
	require.NoError(t, err)
	require.Equal(t, [32]byte{}, [32]byte(got))
}
