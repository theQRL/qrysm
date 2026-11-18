package doublylinkedtree

import (
	"context"
	"testing"
	"time"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/require"
)

func TestForkChoice_ShouldOverrideFCU(t *testing.T) {
	f := setup(0, 0)
	f.numActiveValidators = 640
	f.justifiedBalances = make([]uint64, f.numActiveValidators)
	for i := range f.justifiedBalances {
		f.justifiedBalances[i] = uint64(10)
		f.store.committeeWeight += uint64(10)
	}
	f.store.committeeWeight /= uint64(params.BeaconConfig().SlotsPerEpoch)
	ctx := context.Background()
	driftGenesisTime(f, 1, 0)
	st, blk, err := prepareForkchoiceState(ctx, 1, [32]byte{'a'}, [32]byte{}, [32]byte{'A'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	attesters := make([]uint64, f.numActiveValidators-64)
	for i := range attesters {
		attesters[i] = uint64(i + 64)
	}
	f.ProcessAttestation(ctx, attesters, blk.Root(), 0)

	orphanLateBlockFirstThreshold := params.BeaconConfig().SecondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
	st, blk, err = prepareForkchoiceState(ctx, 2, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, blk.Root(), headRoot)
	t.Run("head is weak", func(t *testing.T) {
		require.Equal(t, true, f.ShouldOverrideFCU())

	})
	t.Run("head is nil", func(t *testing.T) {
		saved := f.store.headNode
		f.store.headNode = nil
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode = saved
	})
	t.Run("head is not from current slot", func(t *testing.T) {
		driftGenesisTime(f, 3, 0)
		require.Equal(t, false, f.ShouldOverrideFCU())
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
	})
	t.Run("head is from epoch boundary", func(t *testing.T) {
		saved := f.store.headNode.slot
		driftGenesisTime(f, params.BeaconConfig().SlotsPerEpoch-1, 0)
		f.store.headNode.slot = params.BeaconConfig().SlotsPerEpoch - 1
		require.Equal(t, false, f.ShouldOverrideFCU())
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
		f.store.headNode.slot = saved
	})
	t.Run("head is early", func(t *testing.T) {
		saved := f.store.headNode.timestamp
		f.store.headNode.timestamp = saved - 2
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.timestamp = saved
	})
	t.Run("chain not finalizing", func(t *testing.T) {
		saved := f.store.headNode.slot
		f.store.headNode.slot = 97
		driftGenesisTime(f, 97, orphanLateBlockFirstThreshold+1)
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.slot = saved
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
	})
	t.Run("Not single block reorg", func(t *testing.T) {
		saved := f.store.headNode.parent.slot
		f.store.headNode.parent.slot = 0
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.parent.slot = saved
	})
	t.Run("head advanced justification", func(t *testing.T) {
		saved := f.store.headNode.unrealizedJustifiedEpoch
		f.store.headNode.unrealizedJustifiedEpoch = f.store.headNode.parent.unrealizedJustifiedEpoch + 1
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.unrealizedJustifiedEpoch = saved
	})
	t.Run("parent is nil", func(t *testing.T) {
		saved := f.store.headNode.parent
		f.store.headNode.parent = nil
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.parent = saved
	})
	t.Run("parent is weak early call", func(t *testing.T) {
		saved := f.store.headNode.parent.weight
		f.store.headNode.parent.weight = 0
		require.Equal(t, true, f.ShouldOverrideFCU())
		f.store.headNode.parent.weight = saved
	})
	t.Run("parent is weak late call", func(t *testing.T) {
		saved := f.store.headNode.parent.weight
		driftGenesisTime(f, 2, ProcessAttestationsThreshold+1)
		f.store.headNode.parent.weight = 0
		require.Equal(t, false, f.ShouldOverrideFCU())
		f.store.headNode.parent.weight = saved
		driftGenesisTime(f, 2, orphanLateBlockFirstThreshold+1)
	})
	t.Run("Head is strong", func(t *testing.T) {
		f.store.headNode.weight = f.store.committeeWeight
		require.Equal(t, false, f.ShouldOverrideFCU())
	})
}

func TestForkChoice_GetProposerHead(t *testing.T) {
	f := setup(0, 0)
	f.numActiveValidators = 640
	f.justifiedBalances = make([]uint64, f.numActiveValidators)
	for i := range f.justifiedBalances {
		f.justifiedBalances[i] = uint64(10)
		f.store.committeeWeight += uint64(10)
	}
	f.store.committeeWeight /= uint64(params.BeaconConfig().SlotsPerEpoch)
	ctx := context.Background()
	driftGenesisTime(f, 1, 0)
	parentRoot := [32]byte{'a'}
	st, blk, err := prepareForkchoiceState(ctx, 1, parentRoot, [32]byte{}, [32]byte{'A'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	attesters := make([]uint64, f.numActiveValidators-64)
	for i := range attesters {
		attesters[i] = uint64(i + 64)
	}
	f.ProcessAttestation(ctx, attesters, blk.Root(), 0)

	driftGenesisTime(f, 3, 0)
	childRoot := [32]byte{'b'}
	st, blk, err = prepareForkchoiceState(ctx, 2, childRoot, [32]byte{'a'}, [32]byte{'B'}, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, blk))
	headRoot, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, blk.Root(), headRoot)
	secondsPerSlot := params.BeaconConfig().SecondsPerSlot
	slotsPerEpoch := params.BeaconConfig().SlotsPerEpoch
	orphanLateBlockFirstThreshold := secondsPerSlot / params.BeaconConfig().IntervalsPerSlot
	// lateArrival is how many seconds into its own slot a late head arrived:
	// one second past the voting window.
	lateArrival := orphanLateBlockFirstThreshold + 1
	// proposerHeadArrived returns GetProposerHead as computed delay seconds
	// into slot, for a head that arrived headArrival seconds into its own
	// slot. GetProposerHead reads the wall clock, so the clock is pinned by
	// moving genesis, and a result is only accepted when it was computed
	// entirely within the pinned second. The head's arrival is stamped
	// relative to the same genesis. Each case below is therefore independent
	// of how long the test takes and differs from "head is weak" only in the
	// condition it is named after, so that condition decides its result.
	// Cases restore what they change with t.Cleanup, so a failed assertion
	// does not leak into the cases after it.
	proposerHeadArrived := func(t *testing.T, slot primitives.Slot, delay, headArrival uint64) [32]byte {
		for attempt := 0; attempt < 10; attempt++ {
			now := uint64(time.Now().Unix())
			genesis := now - uint64(slot)*secondsPerSlot - delay
			f.SetGenesisTime(genesis)
			head := f.store.headNode
			head.timestamp = genesis + uint64(head.slot)*secondsPerSlot + headArrival
			got := f.GetProposerHead()
			if uint64(time.Now().Unix()) == now {
				return got
			}
		}
		t.Fatal("wall clock crossed a second boundary on every attempt")
		return [32]byte{}
	}
	proposerHead := func(t *testing.T, slot primitives.Slot, delay uint64) [32]byte {
		return proposerHeadArrived(t, slot, delay, lateArrival)
	}
	// setSlots moves the head and its parent for the duration of a case.
	setSlots := func(t *testing.T, headSlot, parentSlot primitives.Slot) {
		head := f.store.headNode
		parent := head.parent
		savedHead, savedParent := head.slot, parent.slot
		t.Cleanup(func() { head.slot, parent.slot = savedHead, savedParent })
		head.slot, parent.slot = headSlot, parentSlot
	}
	t.Run("head is weak", func(t *testing.T) {
		require.Equal(t, parentRoot, proposerHead(t, 3, 0))
	})
	t.Run("head is nil", func(t *testing.T) {
		saved := f.store.headNode
		t.Cleanup(func() { f.store.headNode = saved })
		f.store.headNode = nil
		require.Equal(t, [32]byte{}, f.GetProposerHead())
	})
	t.Run("head is not from previous slot", func(t *testing.T) {
		require.Equal(t, childRoot, proposerHead(t, 4, 0))
	})
	t.Run("not proposing early", func(t *testing.T) {
		require.Equal(t, childRoot, proposerHead(t, 3, orphanLateBlockProposingEarly))
	})
	t.Run("head is from epoch boundary", func(t *testing.T) {
		setSlots(t, slotsPerEpoch-1, slotsPerEpoch-2)
		require.Equal(t, childRoot, proposerHead(t, slotsPerEpoch, 0))
	})
	t.Run("head is early", func(t *testing.T) {
		require.Equal(t, childRoot, proposerHeadArrived(t, 3, 0, orphanLateBlockFirstThreshold-1))
	})
	t.Run("chain not finalizing", func(t *testing.T) {
		// Finalized epoch is 0. limit is the last proposal slot that is not
		// more than ReorgMaxEpochsSinceFinalization epochs past it. The next
		// slot is an epoch boundary, so the first proposal slot past the
		// window that only this guard rejects is limit+2.
		limit := primitives.Slot(params.BeaconConfig().ReorgMaxEpochsSinceFinalization+1)*slotsPerEpoch - 1
		setSlots(t, limit-1, limit-2)
		require.Equal(t, parentRoot, proposerHead(t, limit, 0))
		setSlots(t, limit+1, limit)
		require.Equal(t, childRoot, proposerHead(t, limit+2, 0))
	})
	t.Run("Not single block reorg", func(t *testing.T) {
		setSlots(t, f.store.headNode.slot, 0)
		require.Equal(t, childRoot, proposerHead(t, 3, 0))
	})
	t.Run("head advanced justification", func(t *testing.T) {
		head := f.store.headNode
		saved := head.unrealizedJustifiedEpoch
		t.Cleanup(func() { head.unrealizedJustifiedEpoch = saved })
		head.unrealizedJustifiedEpoch = head.parent.unrealizedJustifiedEpoch + 1
		require.Equal(t, childRoot, proposerHead(t, 3, 0))
	})
	t.Run("parent is nil", func(t *testing.T) {
		head := f.store.headNode
		saved := head.parent
		t.Cleanup(func() { head.parent = saved })
		head.parent = nil
		require.Equal(t, childRoot, proposerHead(t, 3, 0))
	})
	t.Run("parent is weak", func(t *testing.T) {
		parent := f.store.headNode.parent
		saved := parent.weight
		t.Cleanup(func() { parent.weight = saved })
		parent.weight = 0
		require.Equal(t, childRoot, proposerHead(t, 3, 0))
	})
	t.Run("Head is strong", func(t *testing.T) {
		head := f.store.headNode
		saved := head.weight
		t.Cleanup(func() { head.weight = saved })
		head.weight = f.store.committeeWeight
		require.Equal(t, childRoot, proposerHead(t, 3, 0))
	})
}
