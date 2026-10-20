package doublylinkedtree

import (
	"context"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/epoch/precompute"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestStore_SetUnrealizedEpochs(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	require.Equal(t, primitives.Epoch(1), f.store.nodeByRoot[[32]byte{'b'}].unrealizedJustifiedEpoch)
	require.Equal(t, primitives.Epoch(1), f.store.nodeByRoot[[32]byte{'b'}].unrealizedFinalizedEpoch)
	require.NoError(t, f.store.setUnrealizedJustifiedEpoch([32]byte{'b'}, 2))
	require.NoError(t, f.store.setUnrealizedFinalizedEpoch([32]byte{'b'}, 2))
	require.Equal(t, primitives.Epoch(2), f.store.nodeByRoot[[32]byte{'b'}].unrealizedJustifiedEpoch)
	require.Equal(t, primitives.Epoch(2), f.store.nodeByRoot[[32]byte{'b'}].unrealizedFinalizedEpoch)

	require.ErrorIs(t, errInvalidUnrealizedJustifiedEpoch, f.store.setUnrealizedJustifiedEpoch([32]byte{'b'}, 0))
	require.ErrorIs(t, errInvalidUnrealizedFinalizedEpoch, f.store.setUnrealizedFinalizedEpoch([32]byte{'b'}, 0))
}

func TestStore_UpdateUnrealizedCheckpoints(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	state, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

}

// Epoch 2    |   Epoch 3
//
//	    |
//	  C |
//	/   |
//
// A <-- B    |
//
//	\   |
//	  ---- D
//
// B is the first block that justifies A.
func TestStore_LongFork(t *testing.T) {
	f := setup(1, 1)
	ctx := context.Background()
	e := params.BeaconConfig().SlotsPerEpoch
	driftGenesisTime(f, 3*e-1, 30)
	state, blkRoot, err := prepareForkchoiceState(ctx, 2*e, [32]byte{'a'}, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 2*e+1, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	require.NoError(t, f.store.setUnrealizedJustifiedEpoch([32]byte{'b'}, 2))
	state, blkRoot, err = prepareForkchoiceState(ctx, 3*e-1, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	require.NoError(t, f.store.setUnrealizedJustifiedEpoch([32]byte{'c'}, 2))

	// Add an attestation to c, it is head
	f.ProcessAttestation(ctx, []uint64{0}, [32]byte{'c'}, 1)
	f.justifiedBalances = []uint64{100}
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, [32]byte{'c'}, headRoot)

	// C's pulled-up source keeps it viable even before the epoch tick.
	ha := [32]byte{'a'}
	driftGenesisTime(f, 3*e, 30)
	state, blkRoot, err = prepareForkchoiceState(ctx, 3*e, [32]byte{'d'}, [32]byte{'b'}, [32]byte{'D'}, 2, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	require.NoError(t, f.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: 2, Root: ha}))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, [32]byte{'c'}, headRoot)
	require.Equal(t, uint64(0), f.store.nodeByRoot[[32]byte{'d'}].weight)
	require.Equal(t, uint64(100), f.store.nodeByRoot[[32]byte{'c'}].weight)

	// Realizing justification during the tick must preserve the head.
	require.NoError(t, f.updateUnrealizedCheckpoints(ctx, 3))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, [32]byte{'c'}, headRoot)
}

//	Epoch 1                Epoch 2               Epoch 3
//	                |                      |
//	                |                      |
//
// A <-- B <-- C <-- D <-- E <-- F <-- G <-- H |
//
//	|        \             |
//	|         --------------- I
//	|                      |
//
// E justifies A. G justifies E.
func TestStore_NoDeadLock(t *testing.T) {
	f := setup(0, 0)
	ctx := context.Background()
	e := params.BeaconConfig().SlotsPerEpoch
	driftGenesisTime(f, 5*e+7, 30)
	a, b := [32]byte{'a'}, [32]byte{'b'}
	z := &qrysmpb.Checkpoint{Root: make([]byte, 32)}
	cp1 := &qrysmpb.Checkpoint{Epoch: 1, Root: a[:]}
	cp2 := &qrysmpb.Checkpoint{Epoch: 2, Root: b[:]}
	// Keep the observations in the current epoch until the final head call.
	// Checkpoints refer to actual earlier blocks, so realizing them can prune.
	parent := [32]byte{}
	for i, name := range []byte{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'} {
		root := [32]byte{name}
		slot := primitives.Slot(i+1) * e
		if i >= 2 {
			slot = 5*e + primitives.Slot(i-2)
		}
		block := checkpointBlock(t, slot, root, parent, z, z)
		if i >= 6 {
			block.UnrealizedJustifiedCheckpoint = cp2
			block.UnrealizedFinalizedCheckpoint = cp1
		} else if i >= 4 {
			block.UnrealizedJustifiedCheckpoint = cp1
		}
		require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{block}))
		parent = root
	}
	f.ProcessAttestation(ctx, []uint64{0}, [32]byte{'h'}, 5)
	f.justifiedBalances = []uint64{100}
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, [32]byte{'h'}, headRoot)
	require.Equal(t, primitives.Epoch(0), f.JustifiedCheckpoint().Epoch)

	// Insert Block I, it becomes Head
	hr := [32]byte{'i'}
	require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{
		checkpointBlock(t, 5*e+6, hr, [32]byte{'f'}, cp1, z),
	}))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, [32]byte{'i'}, headRoot)
	require.Equal(t, primitives.Epoch(1), f.JustifiedCheckpoint().Epoch)
	require.Equal(t, primitives.Epoch(0), f.FinalizedCheckpoint().Epoch)

	// Realize the current epoch's checkpoints; H becomes head again.
	driftGenesisTime(f, 6*e, 30)
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, [32]byte{'h'}, headRoot)
	require.Equal(t, primitives.Epoch(2), f.JustifiedCheckpoint().Epoch)
	require.Equal(t, primitives.Epoch(1), f.FinalizedCheckpoint().Epoch)
}

//	  Epoch  2       |         Epoch 3
//	                 |
//	            -- D (late)
//	           /     |
//	A <- B <- C      |
//	           \     |
//	            -- -- -- E <- F <- G <- H
//	                 |
//
// D justifies and comes late.
func TestStore_ForkNextEpoch(t *testing.T) {
	f := setup(1, 0)
	ctx := context.Background()

	// Epoch 1 blocks (D does not arrive)
	state, blkRoot, err := prepareForkchoiceState(ctx, 380, [32]byte{'a'}, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 381, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 382, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	// Epoch 2 blocks
	state, blkRoot, err = prepareForkchoiceState(ctx, 384, [32]byte{'e'}, [32]byte{'c'}, [32]byte{'E'}, 1, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 385, [32]byte{'f'}, [32]byte{'e'}, [32]byte{'F'}, 1, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 386, [32]byte{'g'}, [32]byte{'f'}, [32]byte{'G'}, 1, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 387, [32]byte{'h'}, [32]byte{'g'}, [32]byte{'H'}, 1, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	// Insert an attestation to H, H is head
	f.ProcessAttestation(ctx, []uint64{0}, [32]byte{'h'}, 1)
	f.justifiedBalances = []uint64{100}
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, [32]byte{'h'}, headRoot)
	require.Equal(t, primitives.Epoch(1), f.JustifiedCheckpoint().Epoch)

	// D arrives late, D is head
	state, blkRoot, err = prepareForkchoiceState(ctx, 383, [32]byte{'d'}, [32]byte{'c'}, [32]byte{'D'}, 1, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	require.NoError(t, f.store.setUnrealizedJustifiedEpoch([32]byte{'d'}, 2))
	f.store.unrealizedJustifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 2}
	require.NoError(t, f.updateUnrealizedCheckpoints(ctx, 3))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, [32]byte{'d'}, headRoot)
	require.Equal(t, primitives.Epoch(2), f.JustifiedCheckpoint().Epoch)
	require.Equal(t, uint64(0), f.store.nodeByRoot[[32]byte{'d'}].weight)
	require.Equal(t, uint64(100), f.store.nodeByRoot[[32]byte{'h'}].weight)
	// Set current epoch to 3, and H's unrealized checkpoint. Check it's head
	driftGenesisTime(f, 387, 0)
	require.NoError(t, f.store.setUnrealizedJustifiedEpoch([32]byte{'h'}, 2))
	headRoot, err = f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, [32]byte{'h'}, headRoot)
	require.Equal(t, primitives.Epoch(2), f.JustifiedCheckpoint().Epoch)
	require.Equal(t, uint64(0), f.store.nodeByRoot[[32]byte{'d'}].weight)
	require.Equal(t, uint64(100), f.store.nodeByRoot[[32]byte{'h'}].weight)
}

func TestStore_PullTips_Heuristics(t *testing.T) {
	ctx := context.Background()
	t.Run("Current epoch is justified", func(tt *testing.T) {
		f := setup(1, 1)
		st, root, err := prepareForkchoiceState(ctx, 257, [32]byte{'p'}, [32]byte{}, [32]byte{}, 1, 1)
		require.NoError(tt, err)
		require.NoError(tt, f.InsertNode(ctx, st, root))
		f.store.nodeByRoot[[32]byte{'p'}].unrealizedJustifiedEpoch = primitives.Epoch(2)
		driftGenesisTime(f, 258, 0)

		st, root, err = prepareForkchoiceState(ctx, 258, [32]byte{'h'}, [32]byte{'p'}, [32]byte{}, 1, 1)
		require.NoError(tt, err)
		require.NoError(tt, f.InsertNode(ctx, st, root))
		// A parent that justified the current epoch does not settle the
		// child's finalization, so the checkpoints are still computed from
		// the state, which here yields the bogus value the fixture carries.
		require.Equal(tt, primitives.Epoch(1), f.store.nodeByRoot[[32]byte{'h'}].unrealizedJustifiedEpoch)
		require.Equal(tt, primitives.Epoch(1), f.store.nodeByRoot[[32]byte{'h'}].unrealizedFinalizedEpoch)
	})

	t.Run("Previous Epoch is justified and early in the current epoch", func(tt *testing.T) {
		f := setup(1, 1)
		st, root, err := prepareForkchoiceState(ctx, 383, [32]byte{'p'}, [32]byte{}, [32]byte{}, 1, 1)
		require.NoError(tt, err)
		require.NoError(tt, f.InsertNode(ctx, st, root))
		f.store.nodeByRoot[[32]byte{'p'}].unrealizedJustifiedEpoch = primitives.Epoch(2)
		driftGenesisTime(f, 384, 0)

		st, root, err = prepareForkchoiceState(ctx, 384, [32]byte{'h'}, [32]byte{'p'}, [32]byte{}, 1, 1)
		require.NoError(tt, err)
		require.NoError(tt, f.InsertNode(ctx, st, root))
		// Being early in the epoch is no reason to inherit the parent's
		// checkpoints: the unrealized justification is computed from the
		// state, which here yields the bogus value the fixture carries.
		require.Equal(tt, primitives.Epoch(1), f.store.nodeByRoot[[32]byte{'h'}].unrealizedJustifiedEpoch)
		require.Equal(tt, primitives.Epoch(1), f.store.nodeByRoot[[32]byte{'h'}].unrealizedFinalizedEpoch)
	})
	t.Run("Previous Epoch is justified and not too early for current", func(tt *testing.T) {
		f := setup(1, 1)
		st, root, err := prepareForkchoiceState(ctx, 384, [32]byte{'p'}, [32]byte{}, [32]byte{}, 1, 1)
		require.NoError(tt, err)
		require.NoError(tt, f.InsertNode(ctx, st, root))
		f.store.nodeByRoot[[32]byte{'p'}].unrealizedJustifiedEpoch = primitives.Epoch(2)
		driftGenesisTime(f, 511, 0)

		st, root, err = prepareForkchoiceState(ctx, 511, [32]byte{'h'}, [32]byte{'p'}, [32]byte{}, 1, 1)
		require.NoError(tt, err)
		require.NoError(tt, f.InsertNode(ctx, st, root))
		// Check that the justification point is not the parent's.
		// This tests that the heuristics in pullTips did not apply and
		// the test continues to compute a bogus unrealized
		// justification
		require.Equal(tt, primitives.Epoch(1), f.store.nodeByRoot[[32]byte{'h'}].unrealizedJustifiedEpoch)
	})
	t.Run("Block from previous Epoch", func(tt *testing.T) {
		f := setup(1, 1)
		st, root, err := prepareForkchoiceState(ctx, 382, [32]byte{'p'}, [32]byte{}, [32]byte{}, 1, 1)
		require.NoError(tt, err)
		require.NoError(tt, f.InsertNode(ctx, st, root))
		f.store.nodeByRoot[[32]byte{'p'}].unrealizedJustifiedEpoch = primitives.Epoch(2)
		driftGenesisTime(f, 384, 0)

		st, root, err = prepareForkchoiceState(ctx, 383, [32]byte{'h'}, [32]byte{'p'}, [32]byte{}, 1, 1)
		require.NoError(tt, err)
		require.NoError(tt, f.InsertNode(ctx, st, root))
		// Check that the justification point is not the parent's.
		// This tests that the heuristics in pullTips did not apply and
		// the test continues to compute a bogus unrealized
		// justification
		require.Equal(tt, primitives.Epoch(1), f.store.nodeByRoot[[32]byte{'h'}].unrealizedJustifiedEpoch)
	})
	t.Run("Previous Epoch is not justified", func(tt *testing.T) {
		f := setup(1, 1)
		st, root, err := prepareForkchoiceState(ctx, 512, [32]byte{'p'}, [32]byte{}, [32]byte{}, 2, 1)
		require.NoError(tt, err)
		require.NoError(tt, f.InsertNode(ctx, st, root))
		driftGenesisTime(f, 513, 0)

		st, root, err = prepareForkchoiceState(ctx, 513, [32]byte{'h'}, [32]byte{'p'}, [32]byte{}, 2, 1)
		require.NoError(tt, err)
		require.NoError(tt, f.InsertNode(ctx, st, root))
		// Check that the justification point is not the parent's.
		// This tests that the heuristics in pullTips did not apply and
		// the test continues to compute a bogus unrealized
		// justification
		require.Equal(tt, primitives.Epoch(2), f.store.nodeByRoot[[32]byte{'h'}].unrealizedJustifiedEpoch)
	})
}

// Fewer than two thirds of an epoch's slots can carry two thirds of the active
// balance when effective balances are uneven, so a block early in the epoch
// can already justify it. pullTips must compute that instead of assuming the
// epoch is too young to be justified.
func TestStore_PullTips_EarlyQuorum(t *testing.T) {
	ctx := context.Background()
	cfg := params.BeaconConfig()
	f := setup(2, 1)
	epochStart := 3 * cfg.SlotsPerEpoch
	parentSlot := epochStart + 84
	st, root, err := prepareForkchoiceState(ctx, parentSlot, [32]byte{'p'}, [32]byte{}, [32]byte{}, 2, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))
	parent := f.store.nodeByRoot[[32]byte{'p'}]
	parent.unrealizedJustifiedEpoch, parent.unrealizedFinalizedEpoch = 2, 1

	// The child sits before the two-thirds mark of epoch 3, yet its
	// participation already holds a target quorum for that epoch.
	childSlot := epochStart + 85
	require.Equal(t, true, uint64(childSlot-epochStart)*3 < uint64(cfg.SlotsPerEpoch)*2)
	child, _ := util.DeterministicGenesisStateZond(t, 128)
	require.NoError(t, child.SetSlot(childSlot))
	require.NoError(t, child.SetPreviousJustifiedCheckpoint(&qrysmpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{1}, 32)}))
	require.NoError(t, child.SetCurrentJustifiedCheckpoint(&qrysmpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{2}, 32)}))
	require.NoError(t, child.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{1}, 32)}))
	target := [32]byte{'t'}
	require.NoError(t, child.UpdateBlockRootAtIndex(uint64(epochStart%cfg.SlotsPerHistoricalRoot), target))
	flags := make([]byte, child.NumValidators())
	for i := 0; i < 100; i++ {
		flags[i] = 1 << cfg.TimelyTargetFlagIndex
	}
	require.NoError(t, child.SetCurrentParticipationBits(flags))
	require.NoError(t, child.SetPreviousParticipationBits(make([]byte, child.NumValidators())))
	wantJ, wantF, err := precompute.UnrealizedCheckpoints(child)
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(3), wantJ.Epoch)
	require.DeepEqual(t, target[:], wantJ.Root)
	require.Equal(t, primitives.Epoch(1), wantF.Epoch)

	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = childSlot
	blk.Block.ParentRoot = bytesutil.PadTo([]byte{'p'}, 32)
	sb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	rb, err := blocks.NewROBlockWithRoot(sb, [32]byte{'h'})
	require.NoError(t, err)
	driftGenesisTime(f, childSlot, 0)
	require.NoError(t, f.InsertNode(ctx, child, rb))

	node := f.store.nodeByRoot[[32]byte{'h'}]
	require.Equal(t, primitives.Epoch(3), node.unrealizedJustifiedEpoch)
	require.Equal(t, primitives.Epoch(1), node.unrealizedFinalizedEpoch)
	require.Equal(t, primitives.Epoch(3), f.store.unrealizedJustifiedCheckpoint.Epoch)
	require.Equal(t, target, f.store.unrealizedJustifiedCheckpoint.Root)
	// At the next epoch boundary the store pulls the checkpoint up.
	require.NoError(t, f.updateUnrealizedCheckpoints(ctx, 4))
	require.Equal(t, primitives.Epoch(3), f.store.justifiedCheckpoint.Epoch)
	require.Equal(t, target, f.store.justifiedCheckpoint.Root)
}

// Once the parent has justified the current epoch, a late previous-epoch
// attestation in the child can still justify the previous epoch and thereby
// finalize an epoch the parent did not. pullTips must compute the child's
// checkpoints instead of inheriting the parent's finalization.
func TestStore_PullTips_ChildFinalizesAfterParentJustified(t *testing.T) {
	ctx := context.Background()
	cfg := params.BeaconConfig()
	f := setup(1, 0)
	epochStart := 3 * cfg.SlotsPerEpoch
	parentSlot := epochStart + 100
	st, root, err := prepareForkchoiceState(ctx, parentSlot, [32]byte{'p'}, [32]byte{}, [32]byte{}, 1, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))
	parent := f.store.nodeByRoot[[32]byte{'p'}]
	parent.unrealizedJustifiedEpoch, parent.unrealizedFinalizedEpoch = 3, 0

	// Epoch 1 is justified, epoch 2 is not yet. The child carries a quorum of
	// target votes for both the previous and the current epoch, so epochs 2
	// and 3 become justified and epoch 1 finalizes.
	childSlot := parentSlot + 1
	anchor, target := [32]byte{'a'}, [32]byte{'t'}
	child, _ := util.DeterministicGenesisStateZond(t, 128)
	require.NoError(t, child.SetSlot(childSlot))
	require.NoError(t, child.SetPreviousJustifiedCheckpoint(&qrysmpb.Checkpoint{Epoch: 1, Root: anchor[:]}))
	require.NoError(t, child.SetCurrentJustifiedCheckpoint(&qrysmpb.Checkpoint{Epoch: 1, Root: anchor[:]}))
	require.NoError(t, child.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)}))
	require.NoError(t, child.SetJustificationBits(bitfield.Bitvector4{0x02}))
	require.NoError(t, child.UpdateBlockRootAtIndex(uint64(epochStart%cfg.SlotsPerHistoricalRoot), target))
	flags := make([]byte, child.NumValidators())
	for i := 0; i < 100; i++ {
		flags[i] = 1 << cfg.TimelyTargetFlagIndex
	}
	require.NoError(t, child.SetCurrentParticipationBits(flags))
	require.NoError(t, child.SetPreviousParticipationBits(flags))
	wantJ, wantF, err := precompute.UnrealizedCheckpoints(child)
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(3), wantJ.Epoch)
	require.DeepEqual(t, target[:], wantJ.Root)
	require.Equal(t, primitives.Epoch(1), wantF.Epoch)
	require.DeepEqual(t, anchor[:], wantF.Root)

	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = childSlot
	blk.Block.ParentRoot = bytesutil.PadTo([]byte{'p'}, 32)
	sb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	rb, err := blocks.NewROBlockWithRoot(sb, [32]byte{'h'})
	require.NoError(t, err)
	driftGenesisTime(f, childSlot, 0)
	require.NoError(t, f.InsertNode(ctx, child, rb))

	node := f.store.nodeByRoot[[32]byte{'h'}]
	require.Equal(t, primitives.Epoch(3), node.unrealizedJustifiedEpoch)
	require.Equal(t, primitives.Epoch(1), node.unrealizedFinalizedEpoch)
	require.Equal(t, primitives.Epoch(1), f.store.unrealizedFinalizedCheckpoint.Epoch)
	require.Equal(t, anchor, f.store.unrealizedFinalizedCheckpoint.Root)
	// At the next epoch boundary the store realizes both checkpoints.
	require.NoError(t, f.updateUnrealizedCheckpoints(ctx, 4))
	require.Equal(t, primitives.Epoch(3), f.store.justifiedCheckpoint.Epoch)
	require.Equal(t, primitives.Epoch(1), f.store.finalizedCheckpoint.Epoch)
	require.Equal(t, anchor, f.store.finalizedCheckpoint.Root)
}

func TestStore_PullTips_CheckpointArrivalOrder(t *testing.T) {
	ctx := context.Background()
	cfg := params.BeaconConfig()
	epochStart := 4 * cfg.SlotsPerEpoch
	anchor, oldTarget, common, newTarget := [32]byte{'a'}, [32]byte{'b'}, [32]byte{'c'}, [32]byte{'d'}
	newer, older := [32]byte{'n'}, [32]byte{'o'}

	// Both branches start epoch 4 with epoch 2 justified. One observes a
	// current-epoch quorum (J4/F0); the other observes a previous-epoch
	// quorum (J3/F2). The checkpoints must advance independently.
	base, _ := util.DeterministicGenesisStateZond(t, 128)
	require.NoError(t, base.SetPreviousJustifiedCheckpoint(&qrysmpb.Checkpoint{Epoch: 2, Root: anchor[:]}))
	require.NoError(t, base.SetCurrentJustifiedCheckpoint(&qrysmpb.Checkpoint{Epoch: 2, Root: anchor[:]}))
	require.NoError(t, base.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{Root: make([]byte, 32)}))
	require.NoError(t, base.SetJustificationBits(bitfield.Bitvector4{0x06}))
	require.NoError(t, base.UpdateBlockRootAtIndex(uint64((epochStart-cfg.SlotsPerEpoch)%cfg.SlotsPerHistoricalRoot), oldTarget))
	require.NoError(t, base.UpdateBlockRootAtIndex(uint64(epochStart%cfg.SlotsPerHistoricalRoot), newTarget))
	flags := make([]byte, base.NumValidators())
	for i := 0; i < 100; i++ {
		flags[i] = 1 << cfg.TimelyTargetFlagIndex
	}
	newState := base.Copy()
	require.NoError(t, newState.SetSlot(epochStart+cfg.SlotsPerEpoch-1))
	require.NoError(t, newState.SetPreviousParticipationBits(make([]byte, base.NumValidators())))
	require.NoError(t, newState.SetCurrentParticipationBits(flags))
	oldState := base.Copy()
	require.NoError(t, oldState.SetSlot(epochStart+1))
	require.NoError(t, oldState.UpdateBlockRootAtIndex(uint64(epochStart%cfg.SlotsPerHistoricalRoot), common))
	require.NoError(t, oldState.SetPreviousParticipationBits(flags))
	require.NoError(t, oldState.SetCurrentParticipationBits(make([]byte, base.NumValidators())))
	newJ, newF, err := precompute.UnrealizedCheckpoints(newState)
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(4), newJ.Epoch)
	require.Equal(t, primitives.Epoch(0), newF.Epoch)
	oldJ, oldF, err := precompute.UnrealizedCheckpoints(oldState)
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(3), oldJ.Epoch)
	require.Equal(t, primitives.Epoch(2), oldF.Epoch)

	for _, tc := range []struct {
		name       string
		olderFirst bool
	}{
		{"newer justification first", false},
		{"newer finalization first", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(2, 0)
			driftGenesisTime(f, newState.Slot(), cfg.SecondsPerSlot/2)
			// anchor -> oldTarget -> common -> newTarget -> newer
			//                           +-> older
			for _, b := range []struct {
				slot         primitives.Slot
				root, parent [32]byte
			}{
				{2 * cfg.SlotsPerEpoch, anchor, params.BeaconConfig().ZeroHash},
				{3 * cfg.SlotsPerEpoch, oldTarget, anchor},
				{epochStart - 1, common, oldTarget},
				{epochStart, newTarget, common},
			} {
				st, blk, err := prepareForkchoiceState(ctx, b.slot, b.root, b.parent, b.root, 2, 0)
				require.NoError(t, err)
				require.NoError(t, f.InsertNode(ctx, st, blk))
			}
			f.justifiedBalances = []uint64{100}
			require.NoError(t, f.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: 2, Root: anchor}))
			_, newBlock, err := prepareForkchoiceState(ctx, newState.Slot(), newer, newTarget, newer, 2, 0)
			require.NoError(t, err)
			_, oldBlock, err := prepareForkchoiceState(ctx, oldState.Slot(), older, common, older, 2, 0)
			require.NoError(t, err)
			if tc.olderFirst {
				require.NoError(t, f.InsertNode(ctx, oldState.Copy(), oldBlock))
				require.NoError(t, f.InsertNode(ctx, newState.Copy(), newBlock))
			} else {
				require.NoError(t, f.InsertNode(ctx, newState.Copy(), newBlock))
				require.NoError(t, f.InsertNode(ctx, oldState.Copy(), oldBlock))
			}
			wantJ := &forkchoicetypes.Checkpoint{Epoch: 4, Root: newTarget}
			wantF := &forkchoicetypes.Checkpoint{Epoch: 2, Root: anchor}
			assert.DeepEqual(t, wantJ, f.store.unrealizedJustifiedCheckpoint)
			assert.DeepEqual(t, wantF, f.store.unrealizedFinalizedCheckpoint)
			f.ProcessAttestation(ctx, []uint64{0}, newer, 4)
			head, err := f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, newer, head)

			// Realization must retain J4 while also advancing finalization.
			boundary := epochStart + cfg.SlotsPerEpoch
			driftGenesisTime(f, boundary, 0)
			require.NoError(t, f.NewSlot(ctx, boundary))
			assert.DeepEqual(t, wantJ, f.JustifiedCheckpoint())
			assert.DeepEqual(t, wantF, f.FinalizedCheckpoint())
			// Even newer votes for the sibling must not move the head
			// outside the highest justified checkpoint's subtree.
			f.ProcessAttestation(ctx, []uint64{0}, older, 5)
			head, err = f.Head(ctx)
			require.NoError(t, err)
			assert.Equal(t, newer, head)
		})
	}
}
