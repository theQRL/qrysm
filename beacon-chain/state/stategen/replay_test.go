package stategen

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/db"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

type recordingBlockRootGetter struct {
	blocks map[[32]byte]interfaces.ReadOnlySignedBeaconBlock
	calls  [][32]byte
}

func (g *recordingBlockRootGetter) Block(_ context.Context, root [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	g.calls = append(g.calls, root)
	return g.blocks[root], nil
}

func TestExecuteStateTransitionStateGen_SyncCommitteeSignatures(t *testing.T) {
	ctx := context.Background()
	preState, keys := util.DeterministicGenesisStateZond(t, 32)
	block, err := util.GenerateFullBlockZond(preState, keys, &util.BlockGenConfig{FullSyncAggregate: true}, 1)
	require.NoError(t, err)
	signed, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)

	t.Run("valid replay matches verified state", func(t *testing.T) {
		verified, err := transition.ExecuteStateTransition(ctx, preState.Copy(), signed)
		require.NoError(t, err)
		replayed, err := executeStateTransitionStateGen(ctx, preState.Copy(), signed)
		require.NoError(t, err)
		verifiedRoot, err := verified.HashTreeRoot(ctx)
		require.NoError(t, err)
		replayedRoot, err := replayed.HashTreeRoot(ctx)
		require.NoError(t, err)
		require.Equal(t, verifiedRoot, replayedRoot)
	})

	t.Run("only replay skips sync signature verification", func(t *testing.T) {
		// Corrupt one signature to detect verification being performed during
		// replay. Production replay only receives previously verified blocks.
		corrupted := qrysmpb.CopySignedBeaconBlockZond(block)
		corrupted.Block.Body.SyncAggregate.SyncCommitteeSignatures[0][0] ^= 1
		signed, err := consensusblocks.NewSignedBeaconBlock(corrupted)
		require.NoError(t, err)

		_, err = transition.ExecuteStateTransition(ctx, preState.Copy(), signed)
		require.ErrorContains(t, "invalid sync committee signature[0]", err)
		_, err = executeStateTransitionStateGen(ctx, preState.Copy(), signed)
		require.NoError(t, err)
	})
}

func TestReplayBlockRoots_AllSkipSlots(t *testing.T) {

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	genesisBlock := blocks.NewGenesisBlock([]byte{})
	bodyRoot, err := genesisBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	err = beaconState.SetLatestBlockHeader(&qrysmpb.BeaconBlockHeader{
		Slot:       genesisBlock.Block.Slot,
		ParentRoot: genesisBlock.Block.ParentRoot,
		StateRoot:  params.BeaconConfig().ZeroHash[:],
		BodyRoot:   bodyRoot[:],
	})
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlashings(make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector)))
	cp := beaconState.CurrentJustifiedCheckpoint()
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	cp.Root = mockRoot[:]
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(cp))

	targetSlot := params.BeaconConfig().SlotsPerEpoch - 1
	newState, err := replayBlockRootsWithGetter(context.Background(), beaconState, nil, targetSlot, nil)
	require.NoError(t, err)
	assert.Equal(t, targetSlot, newState.Slot(), "Did not advance slots")
}

func TestReplayBlockRoots_SameSlot(t *testing.T) {

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	genesisBlock := blocks.NewGenesisBlock([]byte{})
	bodyRoot, err := genesisBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	err = beaconState.SetLatestBlockHeader(&qrysmpb.BeaconBlockHeader{
		Slot:       genesisBlock.Block.Slot,
		ParentRoot: genesisBlock.Block.ParentRoot,
		StateRoot:  params.BeaconConfig().ZeroHash[:],
		BodyRoot:   bodyRoot[:],
	})
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlashings(make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector)))
	cp := beaconState.CurrentJustifiedCheckpoint()
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	cp.Root = mockRoot[:]
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(cp))

	targetSlot := beaconState.Slot()
	newState, err := replayBlockRootsWithGetter(context.Background(), beaconState, nil, targetSlot, nil)
	require.NoError(t, err)
	assert.Equal(t, targetSlot, newState.Slot(), "Did not advance slots")
}

func TestReplayBlockRoots_LowerSlotBlock(t *testing.T) {

	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(1))
	genesisBlock := blocks.NewGenesisBlock([]byte{})
	bodyRoot, err := genesisBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	err = beaconState.SetLatestBlockHeader(&qrysmpb.BeaconBlockHeader{
		Slot:       genesisBlock.Block.Slot,
		ParentRoot: genesisBlock.Block.ParentRoot,
		StateRoot:  params.BeaconConfig().ZeroHash[:],
		BodyRoot:   bodyRoot[:],
	})
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlashings(make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector)))
	cp := beaconState.CurrentJustifiedCheckpoint()
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	cp.Root = mockRoot[:]
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(cp))

	targetSlot := beaconState.Slot()
	b := util.NewBeaconBlockZond()
	b.Block.Slot = beaconState.Slot() - 1
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root := [32]byte{1}
	getter := &recordingBlockRootGetter{blocks: map[[32]byte]interfaces.ReadOnlySignedBeaconBlock{root: wsb}}
	newState, err := replayBlockRootsWithGetter(context.Background(), beaconState, [][32]byte{root}, targetSlot, getter)
	require.NoError(t, err)
	assert.Equal(t, targetSlot, newState.Slot(), "Did not advance slots")
}

func TestLatestAncestorAndBlockRoots_FollowsCanonicalLineage(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name        string
		build       func(*testing.T, db.Database, []byte) ([][32]byte, []*qrysmpb.SignedBeaconBlockZond, error)
		target      int
		removeState []int
		want        []int
	}{
		{
			name:        "fork siblings are excluded",
			build:       tree1,
			target:      8,
			removeState: []int{1, 2, 3, 4, 6},
			want:        []int{1, 2, 4, 6, 8},
		},
		{
			name:        "same-slot siblings are excluded",
			build:       tree2,
			target:      6,
			removeState: []int{1, 5},
			want:        []int{1, 5, 6},
		},
		{
			name:        "selected end-slot sibling is retained",
			build:       tree3,
			target:      2,
			removeState: []int{1},
			want:        []int{1, 2},
		},
		{
			name:   "same-slot child of saved state",
			build:  tree4,
			target: 1,
			want:   []int{1},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			beaconDB := testDB.SetupDB(t)
			s := New(beaconDB, doublylinkedtree.New())
			roots, _, err := tc.build(t, beaconDB, bytesutil.PadTo([]byte{'A'}, 32))
			require.NoError(t, err)
			for _, i := range tc.removeState {
				require.NoError(t, beaconDB.DeleteState(ctx, roots[i]))
			}

			targetBlock, err := beaconDB.Block(ctx, roots[tc.target])
			require.NoError(t, err)
			ancestor, got, err := s.latestAncestorAndBlockRootsForSlot(ctx, roots[tc.target], targetBlock.Block().Slot())
			require.NoError(t, err)
			require.Equal(t, primitives.Slot(0), ancestor.Slot())
			require.Equal(t, len(tc.want), len(got))
			for i, wantIndex := range tc.want {
				require.Equal(t, roots[wantIndex], got[i])
			}
		})
	}
}

func TestLatestAncestorAndBlockRootsForSlot_ExcludesBlocksAfterTarget(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	s := New(beaconDB, doublylinkedtree.New())

	ancestorState, _ := util.DeterministicGenesisStateZond(t, 32)
	ancestorBlock := util.NewBeaconBlockZond()
	ancestorRoot, err := ancestorBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, s.epochBoundaryStateCache.put(ancestorRoot, ancestorState))

	beforeTarget := util.NewBeaconBlockZond()
	beforeTarget.Block.Slot = 5
	beforeTarget.Block.ParentRoot = ancestorRoot[:]
	beforeTargetRoot, err := beforeTarget.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, beforeTarget)

	afterTarget := util.NewBeaconBlockZond()
	afterTarget.Block.Slot = 11
	afterTarget.Block.ParentRoot = beforeTargetRoot[:]
	afterTargetRoot, err := afterTarget.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, afterTarget)

	ancestor, roots, err := s.latestAncestorAndBlockRootsForSlot(ctx, afterTargetRoot, 10)
	require.NoError(t, err)
	require.Equal(t, ancestorState.Slot(), ancestor.Slot())
	require.Equal(t, 1, len(roots))
	require.Equal(t, beforeTargetRoot, roots[0])
}

func TestReplayBlockRoots_RejectsTargetBeforeState(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(1))

	_, err := replayBlockRootsWithGetter(context.Background(), beaconState, nil, 0, nil)
	require.ErrorIs(t, err, ErrReplayTargetSlotExceeded)
}

func TestReplayBlockRoots_FetchesRootsInOrder(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, 32)
	require.NoError(t, beaconState.SetSlot(3))
	roots := [][32]byte{{1}, {2}, {3}}
	getter := &recordingBlockRootGetter{blocks: make(map[[32]byte]interfaces.ReadOnlySignedBeaconBlock)}
	for i, root := range roots {
		block := util.NewBeaconBlockZond()
		block.Block.Slot = primitives.Slot(i)
		signed, err := consensusblocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)
		getter.blocks[root] = signed
	}

	got, err := replayBlockRootsWithGetter(context.Background(), beaconState, roots, beaconState.Slot(), getter)
	require.NoError(t, err)
	require.Equal(t, beaconState.Slot(), got.Slot())
	require.Equal(t, len(roots), len(getter.calls))
	for i := range roots {
		require.Equal(t, roots[i], getter.calls[i])
	}
}

// tree1 constructs the following tree:
// B0 - B1 - - B3 -- B5
//
//	\- B2 -- B4 -- B6 ----- B8
//	                 \- B7
func tree1(t *testing.T, beaconDB db.Database, genesisRoot []byte) ([][32]byte, []*qrysmpb.SignedBeaconBlockZond, error) {
	b0 := util.NewBeaconBlockZond()
	b0.Block.Slot = 0
	b0.Block.ParentRoot = genesisRoot
	r0, err := b0.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b1 := util.NewBeaconBlockZond()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = r0[:]
	r1, err := b1.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b2 := util.NewBeaconBlockZond()
	b2.Block.Slot = 2
	b2.Block.ParentRoot = r1[:]
	r2, err := b2.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b3 := util.NewBeaconBlockZond()
	b3.Block.Slot = 3
	b3.Block.ParentRoot = r1[:]
	r3, err := b3.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b4 := util.NewBeaconBlockZond()
	b4.Block.Slot = 4
	b4.Block.ParentRoot = r2[:]
	r4, err := b4.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b5 := util.NewBeaconBlockZond()
	b5.Block.Slot = 5
	b5.Block.ParentRoot = r3[:]
	r5, err := b5.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b6 := util.NewBeaconBlockZond()
	b6.Block.Slot = 6
	b6.Block.ParentRoot = r4[:]
	r6, err := b6.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b7 := util.NewBeaconBlockZond()
	b7.Block.Slot = 7
	b7.Block.ParentRoot = r6[:]
	r7, err := b7.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b8 := util.NewBeaconBlockZond()
	b8.Block.Slot = 8
	b8.Block.ParentRoot = r6[:]
	r8, err := b8.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	returnedBlocks := make([]*qrysmpb.SignedBeaconBlockZond, 0)
	for _, b := range []*qrysmpb.SignedBeaconBlockZond{b0, b1, b2, b3, b4, b5, b6, b7, b8} {
		beaconBlock := util.NewBeaconBlockZond()
		beaconBlock.Block.Slot = b.Block.Slot
		beaconBlock.Block.ParentRoot = bytesutil.PadTo(b.Block.ParentRoot, 32)
		wsb, err := consensusblocks.NewSignedBeaconBlock(beaconBlock)
		require.NoError(t, err)
		if err := beaconDB.SaveBlock(context.Background(), wsb); err != nil {
			return nil, nil, err
		}
		if err := beaconDB.SaveState(context.Background(), st.Copy(), bytesutil.ToBytes32(beaconBlock.Block.ParentRoot)); err != nil {
			return nil, nil, err
		}
		returnedBlocks = append(returnedBlocks, beaconBlock)
	}
	return [][32]byte{r0, r1, r2, r3, r4, r5, r6, r7, r8}, returnedBlocks, nil
}

// tree2 constructs the following tree:
// B0 - B1
//
//	\- B2
//	\- B2
//	\- B2
//	\- B2 -- B3
func tree2(t *testing.T, beaconDB db.Database, genesisRoot []byte) ([][32]byte, []*qrysmpb.SignedBeaconBlockZond, error) {
	b0 := util.NewBeaconBlockZond()
	b0.Block.Slot = 0
	b0.Block.ParentRoot = genesisRoot
	r0, err := b0.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b1 := util.NewBeaconBlockZond()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = r0[:]
	r1, err := b1.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b21 := util.NewBeaconBlockZond()
	b21.Block.Slot = 2
	b21.Block.ParentRoot = r1[:]
	b21.Block.StateRoot = bytesutil.PadTo([]byte{'A'}, 32)
	r21, err := b21.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b22 := util.NewBeaconBlockZond()
	b22.Block.Slot = 2
	b22.Block.ParentRoot = r1[:]
	b22.Block.StateRoot = bytesutil.PadTo([]byte{'B'}, 32)
	r22, err := b22.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b23 := util.NewBeaconBlockZond()
	b23.Block.Slot = 2
	b23.Block.ParentRoot = r1[:]
	b23.Block.StateRoot = bytesutil.PadTo([]byte{'C'}, 32)
	r23, err := b23.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b24 := util.NewBeaconBlockZond()
	b24.Block.Slot = 2
	b24.Block.ParentRoot = r1[:]
	b24.Block.StateRoot = bytesutil.PadTo([]byte{'D'}, 32)
	r24, err := b24.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b3 := util.NewBeaconBlockZond()
	b3.Block.Slot = 3
	b3.Block.ParentRoot = r24[:]
	r3, err := b3.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	returnedBlocks := make([]*qrysmpb.SignedBeaconBlockZond, 0)
	for _, b := range []*qrysmpb.SignedBeaconBlockZond{b0, b1, b21, b22, b23, b24, b3} {
		beaconBlock := util.NewBeaconBlockZond()
		beaconBlock.Block.Slot = b.Block.Slot
		beaconBlock.Block.ParentRoot = bytesutil.PadTo(b.Block.ParentRoot, 32)
		beaconBlock.Block.StateRoot = bytesutil.PadTo(b.Block.StateRoot, 32)
		wsb, err := consensusblocks.NewSignedBeaconBlock(beaconBlock)
		require.NoError(t, err)
		if err := beaconDB.SaveBlock(context.Background(), wsb); err != nil {
			return nil, nil, err
		}
		if err := beaconDB.SaveState(context.Background(), st.Copy(), bytesutil.ToBytes32(beaconBlock.Block.ParentRoot)); err != nil {
			return nil, nil, err
		}
		returnedBlocks = append(returnedBlocks, beaconBlock)
	}
	return [][32]byte{r0, r1, r21, r22, r23, r24, r3}, returnedBlocks, nil
}

// tree3 constructs the following tree:
// B0 - B1
//
//	\- B2
//	\- B2
//	\- B2
//	\- B2
func tree3(t *testing.T, beaconDB db.Database, genesisRoot []byte) ([][32]byte, []*qrysmpb.SignedBeaconBlockZond, error) {
	b0 := util.NewBeaconBlockZond()
	b0.Block.Slot = 0
	b0.Block.ParentRoot = genesisRoot
	r0, err := b0.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b1 := util.NewBeaconBlockZond()
	b1.Block.Slot = 1
	b1.Block.ParentRoot = r0[:]
	r1, err := b1.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b21 := util.NewBeaconBlockZond()
	b21.Block.Slot = 2
	b21.Block.ParentRoot = r1[:]
	b21.Block.StateRoot = bytesutil.PadTo([]byte{'A'}, 32)
	r21, err := b21.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b22 := util.NewBeaconBlockZond()
	b22.Block.Slot = 2
	b22.Block.ParentRoot = r1[:]
	b22.Block.StateRoot = bytesutil.PadTo([]byte{'B'}, 32)
	r22, err := b22.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b23 := util.NewBeaconBlockZond()
	b23.Block.Slot = 2
	b23.Block.ParentRoot = r1[:]
	b23.Block.StateRoot = bytesutil.PadTo([]byte{'C'}, 32)
	r23, err := b23.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b24 := util.NewBeaconBlockZond()
	b24.Block.Slot = 2
	b24.Block.ParentRoot = r1[:]
	b24.Block.StateRoot = bytesutil.PadTo([]byte{'D'}, 32)
	r24, err := b24.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	returnedBlocks := make([]*qrysmpb.SignedBeaconBlockZond, 0)
	for _, b := range []*qrysmpb.SignedBeaconBlockZond{b0, b1, b21, b22, b23, b24} {
		beaconBlock := util.NewBeaconBlockZond()
		beaconBlock.Block.Slot = b.Block.Slot
		beaconBlock.Block.ParentRoot = bytesutil.PadTo(b.Block.ParentRoot, 32)
		beaconBlock.Block.StateRoot = bytesutil.PadTo(b.Block.StateRoot, 32)
		wsb, err := consensusblocks.NewSignedBeaconBlock(beaconBlock)
		require.NoError(t, err)
		if err := beaconDB.SaveBlock(context.Background(), wsb); err != nil {
			return nil, nil, err
		}
		if err := beaconDB.SaveState(context.Background(), st.Copy(), bytesutil.ToBytes32(beaconBlock.Block.ParentRoot)); err != nil {
			return nil, nil, err
		}
		returnedBlocks = append(returnedBlocks, beaconBlock)
	}

	return [][32]byte{r0, r1, r21, r22, r23, r24}, returnedBlocks, nil
}

// tree4 constructs the following tree:
// B0
//
//	\- B2
//	\- B2
//	\- B2
//	\- B2
func tree4(t *testing.T, beaconDB db.Database, genesisRoot []byte) ([][32]byte, []*qrysmpb.SignedBeaconBlockZond, error) {
	b0 := util.NewBeaconBlockZond()
	b0.Block.Slot = 0
	b0.Block.ParentRoot = genesisRoot
	r0, err := b0.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b21 := util.NewBeaconBlockZond()
	b21.Block.Slot = 2
	b21.Block.ParentRoot = r0[:]
	b21.Block.StateRoot = bytesutil.PadTo([]byte{'A'}, 32)
	r21, err := b21.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b22 := util.NewBeaconBlockZond()
	b22.Block.Slot = 2
	b22.Block.ParentRoot = r0[:]
	b22.Block.StateRoot = bytesutil.PadTo([]byte{'B'}, 32)
	r22, err := b22.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b23 := util.NewBeaconBlockZond()
	b23.Block.Slot = 2
	b23.Block.ParentRoot = r0[:]
	b23.Block.StateRoot = bytesutil.PadTo([]byte{'C'}, 32)
	r23, err := b23.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	b24 := util.NewBeaconBlockZond()
	b24.Block.Slot = 2
	b24.Block.ParentRoot = r0[:]
	b24.Block.StateRoot = bytesutil.PadTo([]byte{'D'}, 32)
	r24, err := b24.Block.HashTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	returnedBlocks := make([]*qrysmpb.SignedBeaconBlockZond, 0)
	for _, b := range []*qrysmpb.SignedBeaconBlockZond{b0, b21, b22, b23, b24} {
		beaconBlock := util.NewBeaconBlockZond()
		beaconBlock.Block.Slot = b.Block.Slot
		beaconBlock.Block.ParentRoot = bytesutil.PadTo(b.Block.ParentRoot, 32)
		beaconBlock.Block.StateRoot = bytesutil.PadTo(b.Block.StateRoot, 32)
		wsb, err := consensusblocks.NewSignedBeaconBlock(beaconBlock)
		require.NoError(t, err)
		if err := beaconDB.SaveBlock(context.Background(), wsb); err != nil {
			return nil, nil, err
		}
		if err := beaconDB.SaveState(context.Background(), st.Copy(), bytesutil.ToBytes32(beaconBlock.Block.ParentRoot)); err != nil {
			return nil, nil, err
		}
		returnedBlocks = append(returnedBlocks, beaconBlock)
	}

	return [][32]byte{r0, r21, r22, r23, r24}, returnedBlocks, nil
}

func TestLoadFinalizedBlocks(t *testing.T) {
	beaconDB := testDB.SetupDB(t)
	ctx := context.Background()
	s := &State{
		beaconDB: beaconDB,
	}
	gBlock := util.NewBeaconBlockZond()
	gRoot, err := gBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, beaconDB, gBlock)
	require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, [32]byte{}))
	roots, _, err := tree1(t, beaconDB, gRoot[:])
	require.NoError(t, err)

	filteredBlocks, err := s.loadFinalizedBlocks(ctx, 0, 8)
	require.NoError(t, err)
	require.Equal(t, 0, len(filteredBlocks))
	require.NoError(t, beaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: roots[8][:]}))

	require.NoError(t, s.beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: roots[8][:]}))
	filteredBlocks, err = s.loadFinalizedBlocks(ctx, 0, 8)
	require.NoError(t, err)
	require.Equal(t, 10, len(filteredBlocks))
}

// A checkpoint state can be a block's post-state advanced through empty slots
// up to the slot of the next block. Replaying that next block must apply it:
// the state is at the block's slot but has not applied it. A state that did
// apply the block (its latest header is the block's) is left alone.
func TestReplayBlockRoots_AppliesBlockAtPreStateSlot(t *testing.T) {
	ctx := context.Background()
	preState, keys := util.DeterministicGenesisStateZond(t, 32)
	block, err := util.GenerateFullBlockZond(preState, keys, util.DefaultBlockGenConfig(), 3)
	require.NoError(t, err)
	signed, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	root, err := signed.Block().HashTreeRoot()
	require.NoError(t, err)
	getter := &recordingBlockRootGetter{blocks: map[[32]byte]interfaces.ReadOnlySignedBeaconBlock{root: signed}}

	advanced, err := transition.ProcessSlots(ctx, preState.Copy(), 3)
	require.NoError(t, err)
	st, err := replayBlockRootsWithGetter(ctx, advanced, [][32]byte{root}, 3, getter)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(3), st.LatestBlockHeader().Slot, "the block at the pre-state's slot was not applied")

	post, err := executeStateTransitionStateGen(ctx, preState.Copy(), signed)
	require.NoError(t, err)
	postRoot, err := post.HashTreeRoot(ctx)
	require.NoError(t, err)
	replayedRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, postRoot, replayedRoot)

	again, err := replayBlockRootsWithGetter(ctx, post.Copy(), [][32]byte{root}, 3, getter)
	require.NoError(t, err)
	againRoot, err := again.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, postRoot, againRoot, "a block the state already applied must not be applied again")
}

// A replay stops at the next slot once its context is cancelled instead of
// running every remaining slot and epoch transition to the target.
func TestReplayProcessSlots_StopsOnCancelledContext(t *testing.T) {
	st, _ := util.DeterministicGenesisStateZond(t, 8)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ReplayProcessSlots(ctx, st, params.BeaconConfig().SlotsPerEpoch*2)
	require.ErrorIs(t, err, context.Canceled)
}
