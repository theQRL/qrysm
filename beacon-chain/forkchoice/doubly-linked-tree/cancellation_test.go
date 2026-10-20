package doublylinkedtree

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/require"
)

// cancelOnCheckContext delivers cancellation at a deterministic point in a
// traversal, without racing the tree's readers and writers in the test.
type cancelOnCheckContext struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (ctx *cancelOnCheckContext) Err() error {
	ctx.remaining--
	if ctx.remaining == 0 {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func TestSetOptimisticToInvalid_CancellationIsAtomic(t *testing.T) {
	for _, cancelAt := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("cancel at check %d", cancelAt), func(t *testing.T) {
			ctx := context.Background()
			f := setup(0, 0)
			// D is the current-slot block so its boost survives head calculation.
			driftGenesisTime(f, 3, 0)
			f.justifiedBalances = []uint64{100}
			a, b, c, d, sibling := indexToHash(1), indexToHash(2), indexToHash(3), indexToHash(4), indexToHash(5)
			// The removed subtree has multiple children. A cancelled walk must
			// not delete an earlier child or detach its parent from the tree.
			for _, block := range []struct {
				slot         primitives.Slot
				root, parent [32]byte
			}{{1, a, [32]byte{}}, {2, b, a}, {2, c, a}, {3, d, b}, {1, sibling, [32]byte{}}} {
				st, ro, err := prepareForkchoiceState(ctx, block.slot, block.root, block.parent, block.root, 0, 0)
				require.NoError(t, err)
				require.NoError(t, f.InsertNode(ctx, st, ro))
			}
			require.NoError(t, f.SetOptimisticToValid(ctx, sibling))
			f.ProcessAttestation(ctx, []uint64{0}, d, 0)
			f.store.proposerBoostRoot = d
			f.store.committeeWeight = 100
			head, err := f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, d, head)
			before, err := f.ForkChoiceDump(ctx)
			require.NoError(t, err)

			cancellable, cancel := context.WithCancel(ctx)
			defer cancel()
			removed, err := f.SetOptimisticToInvalid(&cancelOnCheckContext{Context: cancellable, cancel: cancel, remaining: cancelAt}, a, [32]byte{}, [32]byte{})
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, 0, len(removed))
			after, err := f.ForkChoiceDump(ctx)
			require.NoError(t, err)
			require.DeepEqual(t, before, after)
			require.Equal(t, len(after.ForkChoiceNodes), f.NodeCount())
			require.Equal(t, f.NodeCount(), len(f.store.nodeByPayload))
			require.Equal(t, false, f.store.allTipsAreInvalid)
			require.Equal(t, d, f.store.proposerBoostRoot)
			require.Equal(t, d, f.store.previousProposerBoostRoot)
			head, err = f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, d, head)

			// A fresh request must remove the complete subtree, clear its boost,
			// and select the surviving, validated branch.
			removed, err = f.SetOptimisticToInvalid(ctx, a, [32]byte{}, [32]byte{})
			require.NoError(t, err)
			require.DeepEqual(t, [][32]byte{d, b, c, a}, removed)
			for _, root := range removed {
				require.Equal(t, false, f.HasNode(root))
				_, exists := f.store.nodeByPayload[root]
				require.Equal(t, false, exists)
			}
			require.Equal(t, 2, f.NodeCount())
			require.Equal(t, [32]byte{}, f.store.proposerBoostRoot)
			require.Equal(t, [32]byte{}, f.store.previousProposerBoostRoot)
			require.Equal(t, uint64(0), f.store.previousProposerBoostScore)
			head, err = f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, sibling, head)
		})
	}
}

func TestSetOptimisticToValid_CancellationCanRetry(t *testing.T) {
	for _, cancelAt := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("cancel at check %d", cancelAt), func(t *testing.T) {
			ctx := context.Background()
			f := setup(0, 0)
			driftGenesisTime(f, 10, 30)
			parent := [32]byte{}
			roots := [][32]byte{parent, indexToHash(1), indexToHash(2), indexToHash(3)}
			for i, root := range roots[1:] {
				st, block, err := prepareForkchoiceState(ctx, primitives.Slot(i+1), root, parent, root, 0, 0)
				require.NoError(t, err)
				require.NoError(t, f.InsertNode(ctx, st, block))
				parent = root
			}
			cancellable, cancel := context.WithCancel(ctx)
			defer cancel()
			err := f.SetOptimisticToValid(&cancelOnCheckContext{Context: cancellable, cancel: cancel, remaining: cancelAt}, parent)
			require.ErrorIs(t, err, context.Canceled)
			for _, root := range roots {
				optimistic, err := f.IsOptimistic(root)
				require.NoError(t, err)
				require.Equal(t, true, optimistic)
			}
			require.NoError(t, f.SetOptimisticToValid(ctx, parent))
			for _, root := range roots {
				optimistic, err := f.IsOptimistic(root)
				require.NoError(t, err)
				require.Equal(t, false, optimistic)
			}
		})
	}
}
