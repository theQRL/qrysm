package doublylinkedtree

import (
	"context"
	"errors"
	"testing"

	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

func TestForkChoice_EpochPromotionRetryAtLaterSlot(t *testing.T) {
	for _, mode := range []string{"healthy", "balance read fails", "cancelled boundary"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			f := setup(0, 0)
			e := params.BeaconConfig().SlotsPerEpoch
			driftGenesisTime(f, 2*e-1, 30)
			a, tip, sibling := [32]byte{'a'}, [32]byte{'t'}, [32]byte{'s'}
			z := &qrysmpb.Checkpoint{Root: make([]byte, 32)}
			pending := checkpointBlock(t, 2*e-1, tip, a, z, z)
			pending.UnrealizedJustifiedCheckpoint = &qrysmpb.Checkpoint{Epoch: 1, Root: a[:]}
			require.NoError(t, f.InsertChain(ctx, []*forktypes.BlockAndCheckpoints{
				checkpointBlock(t, e, a, [32]byte{}, z, z), pending,
			}))
			require.NoError(t, f.InsertChain(ctx, []*forktypes.BlockAndCheckpoints{
				checkpointBlock(t, e+1, sibling, [32]byte{}, z, z),
			}))
			f.justifiedBalances = []uint64{100}
			f.ProcessAttestation(ctx, []uint64{0}, sibling, 1)
			head, err := f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, sibling, head)
			attempts := 0
			readErr := errors.New("temporary checkpoint balance failure")
			f.SetBalancesByRooter(func(context.Context, *forktypes.Checkpoint) (*forktypes.JustifiedBalances, error) {
				attempts++
				if mode == "balance read fails" && attempts == 1 {
					return nil, readErr
				}
				return &forktypes.JustifiedBalances{Balances: []uint64{100}, TotalActiveBalance: 100}, nil
			})
			driftGenesisTime(f, 2*e, 30)
			boundaryCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			if mode == "cancelled boundary" {
				cancel()
			}
			err = f.NewSlot(boundaryCtx, 2*e)
			switch mode {
			case "balance read fails":
				require.ErrorIs(t, err, readErr)
			case "cancelled boundary":
				require.ErrorIs(t, err, context.Canceled)
			default:
				require.NoError(t, err)
			}
			// Real slot ticks advance; they do not replay the failed boundary.
			for slot := 2*e + 1; slot <= 2*e+2; slot++ {
				driftGenesisTime(f, slot, 30)
				require.NoError(t, f.NewSlot(ctx, slot))
				head, err = f.Head(ctx)
				require.NoError(t, err)
				require.Equal(t, primitives.Epoch(1), f.JustifiedCheckpoint().Epoch)
				require.Equal(t, tip, head)
			}
			wantAttempts := 1
			if mode == "balance read fails" {
				wantAttempts++
			}
			require.Equal(t, wantAttempts, attempts, "successful realization must not repeat on ordinary ticks")
		})
	}
}

func TestForkChoice_EpochPromotionRetryKeepsNewerObservationsPending(t *testing.T) {
	for _, skipToNextEpoch := range []bool{false, true} {
		t.Run(map[bool]string{false: "retry in same epoch", true: "retry in next epoch"}[skipToNextEpoch], func(t *testing.T) {
			ctx := context.Background()
			f := setup(0, 0)
			e := params.BeaconConfig().SlotsPerEpoch
			a, b, c, older, newer := [32]byte{'a'}, [32]byte{'b'}, [32]byte{'c'}, [32]byte{'o'}, [32]byte{'n'}
			z := &qrysmpb.Checkpoint{Root: make([]byte, 32)}
			driftGenesisTime(f, 2*e+2, 30)
			oldTip := checkpointBlock(t, 2*e+1, older, b, z, z)
			oldTip.UnrealizedJustifiedCheckpoint = &qrysmpb.Checkpoint{Epoch: 2, Root: b[:]}
			oldTip.UnrealizedFinalizedCheckpoint = &qrysmpb.Checkpoint{Epoch: 1, Root: a[:]}
			require.NoError(t, f.InsertChain(ctx, []*forktypes.BlockAndCheckpoints{
				checkpointBlock(t, e, a, [32]byte{}, z, z),
				checkpointBlock(t, 2*e, b, a, z, z), oldTip,
			}))
			// These observations arrive before their epoch's tick takes the lock.
			driftGenesisTime(f, 3*e+2, 30)
			newTip := checkpointBlock(t, 3*e+1, newer, c, z, z)
			newTip.UnrealizedJustifiedCheckpoint = &qrysmpb.Checkpoint{Epoch: 3, Root: c[:]}
			newTip.UnrealizedFinalizedCheckpoint = &qrysmpb.Checkpoint{Epoch: 2, Root: b[:]}
			require.NoError(t, f.InsertChain(ctx, []*forktypes.BlockAndCheckpoints{
				checkpointBlock(t, 3*e, c, b, z, z), newTip,
			}))
			f.justifiedBalances = []uint64{100}
			f.ProcessAttestation(ctx, []uint64{0}, newer, 3)
			attempts := 0
			readErr := errors.New("temporary checkpoint balance failure")
			f.SetBalancesByRooter(func(context.Context, *forktypes.Checkpoint) (*forktypes.JustifiedBalances, error) {
				attempts++
				if attempts <= 2 {
					return nil, readErr
				}
				return &forktypes.JustifiedBalances{Balances: []uint64{100}, TotalActiveBalance: 100}, nil
			})
			require.ErrorIs(t, f.NewSlot(ctx, 3*e), readErr)
			require.NoError(t, f.NewSlot(ctx, 2*e), "older delayed ticks must not consume the pending epoch")
			require.Equal(t, 1, attempts)
			require.ErrorIs(t, f.NewSlot(ctx, 3*e+1), readErr)
			require.Equal(t, primitives.Epoch(0), f.JustifiedCheckpoint().Epoch)
			require.Equal(t, primitives.Epoch(0), f.FinalizedCheckpoint().Epoch)
			if !skipToNextEpoch {
				require.NoError(t, f.NewSlot(ctx, 3*e+2))
				require.Equal(t, primitives.Epoch(2), f.JustifiedCheckpoint().Epoch)
				require.Equal(t, primitives.Epoch(1), f.FinalizedCheckpoint().Epoch)
				head, err := f.Head(ctx)
				require.NoError(t, err)
				require.Equal(t, older, head)
			}
			driftGenesisTime(f, 4*e, 30)
			require.NoError(t, f.NewSlot(ctx, 4*e))
			require.Equal(t, primitives.Epoch(3), f.JustifiedCheckpoint().Epoch)
			require.Equal(t, primitives.Epoch(2), f.FinalizedCheckpoint().Epoch)
			require.Equal(t, false, f.HasNode(a))
			head, err := f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, newer, head)
		})
	}
}
