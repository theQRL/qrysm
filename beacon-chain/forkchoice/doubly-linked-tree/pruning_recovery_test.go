package doublylinkedtree

import (
	"context"
	"fmt"
	"testing"

	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func checkpointBlock(t *testing.T, slot primitives.Slot, root, parent [32]byte, jc, fc *qrysmpb.Checkpoint) *forkchoicetypes.BlockAndCheckpoints {
	t.Helper()
	_, block, err := prepareForkchoiceState(context.Background(), slot, root, parent, root, jc.Epoch, fc.Epoch)
	require.NoError(t, err)
	return &forkchoicetypes.BlockAndCheckpoints{Block: block, JustifiedCheckpoint: jc, FinalizedCheckpoint: fc}
}

func TestInsertChain_CancelledFinalizationCanRetry(t *testing.T) {
	for _, cancelAt := range []int{7, 10} {
		t.Run(fmt.Sprintf("cancel at check %d", cancelAt), func(t *testing.T) {
			ctx := context.Background()
			f := setup(0, 0)
			e := params.BeaconConfig().SlotsPerEpoch
			driftGenesisTime(f, 3*e+3, 30)
			f.justifiedBalances = []uint64{100}
			p, finalized, justified := indexToHash(1), indexToHash(2), indexToHash(3)
			oldTip, nextTip, side := indexToHash(4), indexToHash(5), indexToHash(6)
			z := &qrysmpb.Checkpoint{Root: make([]byte, 32)}
			require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{
				checkpointBlock(t, 1, p, [32]byte{}, z, z),
				checkpointBlock(t, e, finalized, p, z, z),
				checkpointBlock(t, 2*e, justified, finalized, z, z),
			}))
			require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{checkpointBlock(t, 2, side, [32]byte{}, z, z)}))
			jc := &qrysmpb.Checkpoint{Epoch: 2, Root: justified[:]}
			fc := &qrysmpb.Checkpoint{Epoch: 1, Root: finalized[:]}
			pending := []*forkchoicetypes.BlockAndCheckpoints{checkpointBlock(t, 3*e, oldTip, justified, jc, fc)}
			cancellable, cancel := context.WithCancel(ctx)
			defer cancel()
			// The first six checks visit the inserted tree. Cancellation
			// after that interrupts finalization before or during pruning.
			request := &cancelOnCheckContext{Context: cancellable, cancel: cancel, remaining: cancelAt}
			require.ErrorIs(t, f.InsertChain(request, pending), context.Canceled)
			require.Equal(t, primitives.Epoch(0), f.JustifiedCheckpoint().Epoch)
			require.Equal(t, primitives.Epoch(0), f.FinalizedCheckpoint().Epoch)
			require.Equal(t, [32]byte{}, f.FinalizedPayloadBlockHash())
			require.Equal(t, false, f.HasNode(oldTip))
			require.Equal(t, true, f.HasNode(p))
			require.Equal(t, true, f.HasNode(side))
			dump, err := f.ForkChoiceDump(ctx)
			require.NoError(t, err)
			require.Equal(t, 5, f.NodeCount())
			require.Equal(t, f.NodeCount(), len(dump.ForkChoiceNodes))

			require.NoError(t, f.InsertChain(ctx, pending))
			require.Equal(t, false, f.HasNode(p))
			require.Equal(t, false, f.HasNode(side))
			require.Equal(t, finalized, f.store.treeRootNode.root)
			require.Equal(t, finalized, f.FinalizedPayloadBlockHash())
			require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{checkpointBlock(t, 3*e+1, nextTip, justified, jc, fc)}))
			f.ProcessAttestation(ctx, []uint64{0}, nextTip, 3)
			head, err := f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, nextTip, head)
			dump, err = f.ForkChoiceDump(ctx)
			require.NoError(t, err)
			require.Equal(t, f.NodeCount(), len(dump.ForkChoiceNodes))
		})
	}
}

func TestStore_PruneCancellationPreservesIncompatibleChildren(t *testing.T) {
	for _, cancelAt := range []int{3, 6} {
		t.Run(fmt.Sprintf("cancel at check %d", cancelAt), func(t *testing.T) {
			ctx := context.Background()
			f := setup(0, 0)
			e := params.BeaconConfig().SlotsPerEpoch
			driftGenesisTime(f, 4*e, 30)
			finalized, bad, good, otherBad, leaf := indexToHash(1), indexToHash(2), indexToHash(3), indexToHash(4), indexToHash(5)
			z := &qrysmpb.Checkpoint{Root: make([]byte, 32)}
			for _, block := range []*forkchoicetypes.BlockAndCheckpoints{
				checkpointBlock(t, e-1, finalized, [32]byte{}, z, z),
				checkpointBlock(t, e+1, bad, finalized, z, z),
				checkpointBlock(t, 2*e+1, good, finalized, z, z),
				checkpointBlock(t, e+2, otherBad, finalized, z, z),
				checkpointBlock(t, e+3, leaf, bad, z, z),
			} {
				require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{block}))
			}
			require.NoError(t, f.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: 1, Root: finalized}))
			require.NoError(t, f.store.prune(ctx))
			require.NoError(t, f.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: 2, Root: finalized}))
			before, err := f.ForkChoiceDump(ctx)
			require.NoError(t, err)
			cancellable, cancel := context.WithCancel(ctx)
			defer cancel()
			require.ErrorIs(t, f.store.prune(&cancelOnCheckContext{Context: cancellable, cancel: cancel, remaining: cancelAt}), context.Canceled)
			after, err := f.ForkChoiceDump(ctx)
			require.NoError(t, err)
			require.DeepEqual(t, before, after)
			require.Equal(t, 5, f.NodeCount())
			require.Equal(t, 5, len(f.store.nodeByPayload))
			require.NoError(t, f.store.prune(ctx))
			require.Equal(t, 2, f.NodeCount())
			require.Equal(t, 1, len(f.store.treeRootNode.children))
			head, err := f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, good, head)
		})
	}
}

func TestNewSlot_CancelledFinalizationCanRetry(t *testing.T) {
	ctx := context.Background()
	f := setup(0, 0)
	e := params.BeaconConfig().SlotsPerEpoch
	driftGenesisTime(f, 4*e, 30)
	p, finalized, justified, tip, side := indexToHash(1), indexToHash(2), indexToHash(3), indexToHash(4), indexToHash(5)
	z := &qrysmpb.Checkpoint{Root: make([]byte, 32)}
	require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{
		checkpointBlock(t, 1, p, [32]byte{}, z, z),
		checkpointBlock(t, e, finalized, p, z, z),
		checkpointBlock(t, 2*e, justified, finalized, z, z),
		checkpointBlock(t, 3*e, tip, justified, z, z),
	}))
	require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{checkpointBlock(t, 2, side, [32]byte{}, z, z)}))
	f.store.nodeByRoot[tip].unrealizedJustifiedEpoch = 2
	f.store.nodeByRoot[tip].unrealizedFinalizedEpoch = 1
	f.store.nodeByRoot[tip].unrealizedJustifiedRoot = justified
	f.store.nodeByRoot[tip].unrealizedFinalizedRoot = finalized
	f.store.unrealizedJustifiedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 2, Root: justified}
	f.store.unrealizedFinalizedCheckpoint = &forkchoicetypes.Checkpoint{Epoch: 1, Root: finalized}
	before, err := f.ForkChoiceDump(ctx)
	require.NoError(t, err)
	cancellable, cancel := context.WithCancel(ctx)
	defer cancel()
	require.ErrorIs(t, f.NewSlot(&cancelOnCheckContext{Context: cancellable, cancel: cancel, remaining: 4}, 4*e), context.Canceled)
	after, err := f.ForkChoiceDump(ctx)
	require.NoError(t, err)
	require.DeepEqual(t, before, after)
	require.Equal(t, [32]byte{}, f.FinalizedPayloadBlockHash())
	require.Equal(t, f.NodeCount(), len(after.ForkChoiceNodes))
	require.NoError(t, f.NewSlot(ctx, 4*e))
	require.Equal(t, primitives.Epoch(2), f.JustifiedCheckpoint().Epoch)
	require.Equal(t, primitives.Epoch(1), f.FinalizedCheckpoint().Epoch)
	require.Equal(t, finalized, f.store.treeRootNode.root)
	require.Equal(t, finalized, f.FinalizedPayloadBlockHash())
	head, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, tip, head)
}
