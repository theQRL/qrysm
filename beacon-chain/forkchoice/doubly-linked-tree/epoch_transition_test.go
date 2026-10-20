package doublylinkedtree

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

// epochPromotionTree starts at J1 with a heavily voted sibling of a branch
// carrying pending J2/F1. At epoch 3 the new checkpoint must exclude the sibling.
func epochPromotionTree(t *testing.T) (*ForkChoice, [32]byte) {
	t.Helper()
	ctx := context.Background()
	f := setup(0, 0)
	e := params.BeaconConfig().SlotsPerEpoch
	driftGenesisTime(f, 3*e-1, 30)
	base, checkpoint, tip, sibling := [32]byte{'a'}, [32]byte{'b'}, [32]byte{'t'}, [32]byte{'s'}
	z := &qrysmpb.Checkpoint{Root: make([]byte, 32)}
	cp1 := &qrysmpb.Checkpoint{Epoch: 1, Root: base[:]}
	cp2 := &qrysmpb.Checkpoint{Epoch: 2, Root: checkpoint[:]}
	f.justifiedBalances = []uint64{100}
	pending := checkpointBlock(t, 3*e-1, tip, checkpoint, cp1, z)
	pending.UnrealizedJustifiedCheckpoint = cp2
	pending.UnrealizedFinalizedCheckpoint = cp1
	require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{
		checkpointBlock(t, e, base, [32]byte{}, z, z),
		checkpointBlock(t, 2*e, checkpoint, base, cp1, z),
		pending,
	}))
	require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{
		checkpointBlock(t, 2*e+1, sibling, base, cp1, z),
	}))
	f.ProcessAttestation(ctx, []uint64{0}, sibling, 2)
	head, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, sibling, head)
	require.Equal(t, primitives.Epoch(1), f.JustifiedCheckpoint().Epoch)
	require.Equal(t, primitives.Epoch(2), f.store.unrealizedJustifiedCheckpoint.Epoch)
	return f, tip
}

func TestForkChoice_HeadBeforeEpochTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f, tip := epochPromotionTree(t)
		e := params.BeaconConfig().SlotsPerEpoch
		driftGenesisTime(f, 3*e, 0)
		beforeTick, err := f.Head(ctx)
		require.NoError(t, err)
		require.Equal(t, tip, beforeTick)
		require.Equal(t, primitives.Epoch(2), f.JustifiedCheckpoint().Epoch)
		require.Equal(t, primitives.Epoch(1), f.FinalizedCheckpoint().Epoch)
		require.Equal(t, f.FinalizedCheckpoint().Root, f.store.treeRootNode.root)
		require.Equal(t, false, f.HasNode([32]byte{}), "finalization must prune the old anchor")
		require.NoError(t, f.NewSlot(ctx, 3*e))
		afterTick, err := f.Head(ctx)
		require.NoError(t, err)
		require.Equal(t, beforeTick, afterTick)
	})
}

func TestForkChoice_FirstTickAfterEpochBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset primitives.Slot
	}{
		{name: "next slot", offset: 1},
		{name: "skipped epoch", offset: params.BeaconConfig().SlotsPerEpoch + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				f, tip := epochPromotionTree(t)
				e := params.BeaconConfig().SlotsPerEpoch
				// The ticker starts at the next slot after initial sync. It may
				// never have delivered the epoch boundary for this store.
				driftGenesisTime(f, 3*e+tc.offset, 0)
				require.NoError(t, f.NewSlot(ctx, 3*e+tc.offset))
				require.Equal(t, primitives.Epoch(2), f.JustifiedCheckpoint().Epoch)
				require.Equal(t, primitives.Epoch(1), f.FinalizedCheckpoint().Epoch)
				head, err := f.Head(ctx)
				require.NoError(t, err)
				require.Equal(t, tip, head)
			})
		})
	}
}

func TestForkChoice_HeadUsesPromotedBalances(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		f := setup(0, 0)
		e := params.BeaconConfig().SlotsPerEpoch
		driftGenesisTime(f, 3*e-1, 30)
		base, checkpoint, heavy, light := [32]byte{'a'}, [32]byte{'b'}, [32]byte{'h'}, [32]byte{'l'}
		z := &qrysmpb.Checkpoint{Root: make([]byte, 32)}
		cp1 := &qrysmpb.Checkpoint{Epoch: 1, Root: base[:]}
		cp2 := &qrysmpb.Checkpoint{Epoch: 2, Root: checkpoint[:]}
		f.justifiedBalances = []uint64{100, 1}
		require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{
			checkpointBlock(t, e, base, [32]byte{}, z, z),
			checkpointBlock(t, 2*e, checkpoint, base, cp1, z),
		}))
		for _, root := range [][32]byte{heavy, light} {
			tip := checkpointBlock(t, 3*e-1, root, checkpoint, cp1, z)
			tip.UnrealizedJustifiedCheckpoint = cp2
			require.NoError(t, f.InsertChain(ctx, []*forkchoicetypes.BlockAndCheckpoints{tip}))
		}
		f.ProcessAttestation(ctx, []uint64{0}, heavy, 2)
		f.ProcessAttestation(ctx, []uint64{1}, light, 2)
		head, err := f.Head(ctx)
		require.NoError(t, err)
		require.Equal(t, heavy, head)
		reads := 0
		f.SetBalancesByRooter(func(_ context.Context, cp *forkchoicetypes.Checkpoint) (*forkchoicetypes.JustifiedBalances, error) {
			reads++
			require.Equal(t, primitives.Epoch(2), cp.Epoch)
			return &forkchoicetypes.JustifiedBalances{Balances: []uint64{1, 100}, TotalActiveBalance: 101}, nil
		})
		driftGenesisTime(f, 3*e, 0)
		head, err = f.Head(ctx)
		require.NoError(t, err)
		require.Equal(t, light, head, "the first head must use the new checkpoint's balances")
		for _, slot := range []primitives.Slot{2 * e, 3 * e, 3*e + 1} {
			require.NoError(t, f.NewSlot(ctx, slot))
			head, err = f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, light, head)
		}
		require.Equal(t, 1, reads, "completed epochs must not reload balances")
	})
}

func TestForkChoice_HeadEpochPromotionCanRetry(t *testing.T) {
	for _, mode := range []string{"balance read fails", "cancelled head"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				f, tip := epochPromotionTree(t)
				e := params.BeaconConfig().SlotsPerEpoch
				before, err := f.ForkChoiceDump(ctx)
				require.NoError(t, err)
				readErr := errors.New("temporary balance read failure")
				reads := 0
				f.SetBalancesByRooter(func(context.Context, *forkchoicetypes.Checkpoint) (*forkchoicetypes.JustifiedBalances, error) {
					reads++
					if mode == "balance read fails" && reads == 1 {
						return nil, readErr
					}
					return &forkchoicetypes.JustifiedBalances{Balances: []uint64{100}, TotalActiveBalance: 100}, nil
				})
				requestCtx, cancel := context.WithCancel(ctx)
				defer cancel()
				wantErr := readErr
				if mode == "cancelled head" {
					cancel()
					wantErr = context.Canceled
				}
				driftGenesisTime(f, 3*e, 0)
				_, err = f.Head(requestCtx)
				require.ErrorIs(t, err, wantErr)
				after, err := f.ForkChoiceDump(ctx)
				require.NoError(t, err)
				require.DeepEqual(t, before, after, "failed promotion must preserve checkpoints, weights, and tree links")
				// Retry without replaying the epoch boundary or waiting for its ticker.
				driftGenesisTime(f, 3*e+1, 0)
				head, err := f.Head(ctx)
				require.NoError(t, err)
				require.Equal(t, tip, head)
				require.Equal(t, primitives.Epoch(2), f.JustifiedCheckpoint().Epoch)
				require.Equal(t, primitives.Epoch(1), f.FinalizedCheckpoint().Epoch)
			})
		})
	}
}
