package blockchain

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/require"
)

func TestService_ReceiveBlockBatch_FutureSlot(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tc := range []struct {
		name                      string
		batch, prefix, withinSkew bool
	}{
		{name: "gossip control"},
		{name: "single future block", batch: true},
		{name: "future suffix", batch: true, prefix: true},
		{name: "gossip clock tolerance", withinSkew: true},
		{name: "batch clock tolerance", batch: true, withinSkew: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 2)
			pre := f.states[2].Copy()
			var incoming []blocks.ROBlock
			if tc.prefix {
				b, st := emptyBranchBlock(t, f, pre, 8, 'p')
				incoming, pre = append(incoming, b), st
			}
			future, _ := emptyBranchBlock(t, f, pre, 9, 'f')
			incoming = append(incoming, future)
			receive := func() error {
				if tc.batch {
					return f.s.ReceiveBlockBatch(f.ctx, incoming)
				}
				return f.s.ReceiveBlock(f.ctx, future, future.Root())
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 8, -30)
				if tc.withinSkew {
					// Stay just before slot 9, within the same clock tolerance
					// used by the single-block import path.
					driftGenesisTime(f.s, 9, 1)
					time.Sleep(time.Second - params.BeaconNetworkConfig().MaximumGossipClockDisparity/2)
				}
				require.Equal(t, primitives.Slot(8), f.s.CurrentSlot())
				err := receive()
				synctest.Wait()
				if !tc.withinSkew {
					require.ErrorContains(t, "slot from the future", err)
					require.Equal(t, false, IsInvalidBlock(err), "the block remains eligible for a later retry")
					for _, b := range incoming {
						require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(b.Root()))
						require.Equal(t, false, f.s.HasBlock(f.ctx, b.Root()))
						require.Equal(t, false, f.s.cfg.BeaconDB.HasStateSummary(f.ctx, b.Root()))
					}
					require.Equal(t, primitives.Slot(2), f.s.HeadSlot())
					require.Equal(t, f.blks[1].Root(), f.s.CachedHeadRoot())
					cached, cacheErr := f.s.cfg.StateGen.StateByRoot(f.ctx, f.blks[1].Root())
					require.NoError(t, cacheErr)
					root, rootErr := cached.HashTreeRoot(f.ctx)
					require.NoError(t, rootErr)
					require.Equal(t, f.blks[1].Block().StateRoot(), root, "reject the whole batch before mutating the cached parent")
					driftGenesisTime(f.s, 9, 0)
					err = receive()
					synctest.Wait()
				}
				require.NoError(t, err)
				require.Equal(t, future.Root(), f.s.CachedHeadRoot())
				require.Equal(t, primitives.Slot(9), f.s.HeadSlot())
			})
		})
	}
}
