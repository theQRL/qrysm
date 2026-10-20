package stategen

import (
	"context"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/beacon-chain/state"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestBlockForSlotFuture(t *testing.T) {
	ch := &CanonicalHistory{
		cs: &mockCurrentSlotter{Slot: 0},
	}
	_, err := ch.BlockRootForSlot(context.Background(), 1)
	require.ErrorIs(t, err, ErrFutureSlotRequested)
}

func TestChainForSlotFuture(t *testing.T) {
	ch := &CanonicalHistory{
		cs: &mockCurrentSlotter{Slot: 0},
	}
	_, _, err := ch.chainForSlot(context.Background(), 1)
	require.ErrorIs(t, err, ErrFutureSlotRequested)
}

func TestBestForSlot(t *testing.T) {
	derp := errors.New("fake hash tree root method no hash good")
	var goodHTR [32]byte
	copy(goodHTR[:], []byte{23})
	var betterHTR [32]byte
	copy(betterHTR[:], []byte{42})

	cases := []struct {
		name  string
		err   error
		roots [][32]byte
		root  [32]byte
		cc    CanonicalChecker
	}{
		{
			name:  "empty list",
			err:   ErrNoCanonicalBlockForSlot,
			roots: [][32]byte{},
		},
		{
			name:  "IsCanonical fail",
			roots: [][32]byte{goodHTR, betterHTR},
			cc:    &mockCanonicalChecker{is: true, err: derp},
			err:   derp,
		},
		{
			name:  "all non-canonical",
			err:   ErrNoCanonicalBlockForSlot,
			roots: [][32]byte{goodHTR, betterHTR},
			cc:    &mockCanonicalChecker{is: false},
		},
		{
			name:  "one canonical",
			cc:    &mockCanonicalChecker{is: true},
			root:  goodHTR,
			roots: [][32]byte{goodHTR},
		},
		{
			name:  "all canonical",
			cc:    &mockCanonicalChecker{is: true},
			root:  betterHTR,
			roots: [][32]byte{betterHTR, goodHTR},
		},
		{
			name:  "first wins",
			cc:    &mockCanonicalChecker{is: true},
			root:  goodHTR,
			roots: [][32]byte{goodHTR, betterHTR},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chk := CanonicalChecker(&mockCanonicalChecker{is: true})
			if c.cc != nil {
				chk = c.cc
			}
			ch := &CanonicalHistory{cc: chk}
			r, err := ch.bestForSlot(context.Background(), c.roots)
			if c.err == nil {
				require.NoError(t, err)
				require.Equal(t, c.root, r)
			} else {
				require.ErrorIs(t, err, c.err)
			}
		})
	}
}

// happy path tests
func TestCanonicalBlockForSlotHappy(t *testing.T) {
	ctx := context.Background()
	var begin, middle, end primitives.Slot = 100, 150, 155
	specs := []mockHistorySpec{
		{slot: begin},
		{slot: middle, savedState: true},
		{slot: end, canonicalBlock: true},
	}
	hist := newMockHistory(t, specs, end+1)
	ch := &CanonicalHistory{h: hist, cc: hist, cs: hist}

	// since only the end block and genesis are canonical, once the slot drops below
	// end, we should always get genesis
	cases := []struct {
		slot    primitives.Slot
		highest primitives.Slot
		canon   primitives.Slot
		name    string
	}{
		{slot: hist.current, highest: end, canon: end, name: "slot > end"},
		{slot: end, highest: end, canon: end, name: "slot == end"},
		{slot: end - 1, highest: middle, canon: 0, name: "middle < slot < end"},
		{slot: middle, highest: middle, canon: 0, name: "slot == middle"},
		{slot: middle - 1, highest: begin, canon: 0, name: "begin < slot < middle"},
		{slot: begin, highest: begin, canon: 0, name: "slot == begin"},
		{slot: begin - 1, highest: 0, canon: 0, name: "genesis < slot < begin"},
		{slot: 0, highest: 0, canon: 0, name: "slot == genesis"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, rs, err := hist.HighestRootsBelowSlot(ctx, c.slot+1)
			require.NoError(t, err)
			require.Equal(t, len(rs), 1)
			require.Equal(t, hist.slotMap[c.highest], rs[0])
			cr, err := ch.BlockRootForSlot(ctx, c.slot)
			require.NoError(t, err)
			require.Equal(t, hist.slotMap[c.canon], cr)
		})
	}
}

func TestCanonicalBlockForSlotNonHappy(t *testing.T) {
	ctx := context.Background()
	var begin, middle, end primitives.Slot = 100, 150, 155
	specs := []mockHistorySpec{
		{slot: begin},
		{slot: middle, savedState: true},
		{slot: end, canonicalBlock: true},
	}
	hist := newMockHistory(t, specs, end+1)

	genesis, err := hist.GenesisBlockRoot(ctx)
	require.NoError(t, err)
	slotOrderObserved := make([]primitives.Slot, 0)
	derp := errors.New("HighestRootsBelowSlot don't work")
	// since only the end block and genesis are canonical, once the slot drops below
	// end, we should always get genesis
	cases := []struct {
		name              string
		slot              primitives.Slot
		canon             CanonicalChecker
		overrideHighest   func(context.Context, primitives.Slot) (primitives.Slot, [][32]byte, error)
		slotOrderExpected []primitives.Slot
		err               error
		root              [32]byte
	}{
		{
			name: "HighestRootsBelowSlot not called for genesis",
			overrideHighest: func(_ context.Context, _ primitives.Slot) (primitives.Slot, [][32]byte, error) {
				return 0, [][32]byte{}, derp
			},
			root: hist.slotMap[0],
		},
		{
			name: "wrapped error from HighestRootsBelowSlot returned",
			err:  derp,
			overrideHighest: func(_ context.Context, _ primitives.Slot) (primitives.Slot, [][32]byte, error) {
				return 0, [][32]byte{}, derp
			},
			slot: end,
		},
		{
			name: "HighestRootsBelowSlot empty list",
			err:  ErrNoBlocksBelowSlot,
			overrideHighest: func(_ context.Context, _ primitives.Slot) (primitives.Slot, [][32]byte, error) {
				return 0, [][32]byte{}, nil
			},
			slot: end,
		},
		{
			name:  "HighestRootsBelowSlot no canonical",
			canon: &mockCanonicalChecker{is: false},
			slot:  end,
			root:  genesis,
		},
		{
			name: "slot ordering correct - only genesis canonical",
			canon: &mockCanonicalChecker{isCanon: func(root [32]byte) (bool, error) {
				if root == hist.slotMap[0] {
					return true, nil
				}
				return false, nil
			}},
			overrideHighest: func(_ context.Context, s primitives.Slot) (primitives.Slot, [][32]byte, error) {
				slotOrderObserved = append(slotOrderObserved, s)
				// this allows the mock HighestRootsBelowSlot to continue to execute now that we've recorded
				// the slot in our channel
				return 0, nil, errFallThroughOverride
			},
			slotOrderExpected: []primitives.Slot{156, 155, 150, 100},
			slot:              end,
			root:              hist.slotMap[0],
		},
		{
			name: "slot ordering correct - slot 100 canonical",
			canon: &mockCanonicalChecker{isCanon: func(root [32]byte) (bool, error) {
				if root == hist.slotMap[100] {
					return true, nil
				}
				return false, nil
			}},
			overrideHighest: func(_ context.Context, s primitives.Slot) (primitives.Slot, [][32]byte, error) {
				slotOrderObserved = append(slotOrderObserved, s)
				// this allows the mock HighestRootsBelowSlot to continue to execute now that we've recorded
				// the slot in our channel
				return 0, nil, errFallThroughOverride
			},
			slotOrderExpected: []primitives.Slot{156, 155, 150},
			slot:              end,
			root:              hist.slotMap[100],
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var canon CanonicalChecker = hist
			if c.canon != nil {
				canon = c.canon
			}
			ch := &CanonicalHistory{h: hist, cc: canon, cs: hist}
			hist.overrideHighestSlotBlocksBelow = c.overrideHighest
			r, err := ch.BlockRootForSlot(ctx, c.slot)
			if c.err == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, c.err)
			}
			if len(c.slotOrderExpected) > 0 {
				require.Equal(t, len(c.slotOrderExpected), len(slotOrderObserved), "HighestRootsBelowSlot not called the expected number of times")
				for i := range c.slotOrderExpected {
					require.Equal(t, c.slotOrderExpected[i], slotOrderObserved[i])
				}
			}
			if c.root != [32]byte{} {
				require.Equal(t, c.root, r)
			}
			slotOrderObserved = make([]primitives.Slot, 0)
		})
	}
}

type mockCurrentSlotter struct {
	Slot primitives.Slot
}

func (c *mockCurrentSlotter) CurrentSlot() primitives.Slot {
	return c.Slot
}

var _ CurrentSlotter = &mockCurrentSlotter{}

func TestAncestorChainCache(t *testing.T) {
	ctx := context.Background()
	var begin, middle, end primitives.Slot = 100, 150, 155
	specs := []mockHistorySpec{
		{slot: begin, canonicalBlock: true},
		{slot: middle, canonicalBlock: true},
		{slot: end, canonicalBlock: true},
	}
	hist := newMockHistory(t, specs, end+1)
	ch := &CanonicalHistory{h: hist, cc: hist, cs: hist}

	// should only contain the genesis block
	require.Equal(t, 1, len(hist.states))

	endRoot := hist.slotMap[end]
	st, bs, err := ch.ancestorChain(ctx, endRoot, end)
	require.NoError(t, err)
	require.Equal(t, 3, len(bs))
	expectedHTR, err := hist.states[hist.slotMap[0]].HashTreeRoot(ctx)
	require.NoError(t, err)
	actualHTR, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedHTR, actualHTR)

	// now populate the cache, we should get the cached state instead of genesis
	ch.cache = &mockCachedGetter{
		cache: map[[32]byte]state.BeaconState{
			hist.slotMap[end]: hist.hiddenStates[hist.slotMap[end]],
		},
	}
	st, bs, err = ch.ancestorChain(ctx, endRoot, end)
	require.NoError(t, err)
	require.Equal(t, 0, len(bs))
	expectedHTR, err = hist.hiddenStates[hist.slotMap[end]].HashTreeRoot(ctx)
	require.NoError(t, err)
	actualHTR, err = st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedHTR, actualHTR)

	// populate cache with a different state for good measure
	ch.cache = &mockCachedGetter{
		cache: map[[32]byte]state.BeaconState{
			hist.slotMap[begin]: hist.hiddenStates[hist.slotMap[begin]],
		},
	}
	st, bs, err = ch.ancestorChain(ctx, endRoot, end)
	require.NoError(t, err)
	require.Equal(t, 2, len(bs))
	expectedHTR, err = hist.hiddenStates[hist.slotMap[begin]].HashTreeRoot(ctx)
	require.NoError(t, err)
	actualHTR, err = st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedHTR, actualHTR)

	// rebuild history w/ last state saved, make sure we get that instead of cache
	specs[2].savedState = true
	hist = newMockHistory(t, specs, end+1)
	ch = &CanonicalHistory{h: hist, cc: hist, cs: hist}
	endRoot = hist.slotMap[end]
	ch.cache = &mockCachedGetter{
		cache: map[[32]byte]state.BeaconState{
			hist.slotMap[begin]: hist.hiddenStates[hist.slotMap[begin]],
		},
	}
	st, bs, err = ch.ancestorChain(ctx, endRoot, end)
	require.NoError(t, err)
	require.Equal(t, 0, len(bs))
	expectedHTR, err = hist.states[hist.slotMap[end]].HashTreeRoot(ctx)
	require.NoError(t, err)
	actualHTR, err = st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedHTR, actualHTR)
}

func TestAncestorChainOK(t *testing.T) {
	ctx := context.Background()
	var begin, middle, end primitives.Slot = 100, 150, 155
	specs := []mockHistorySpec{
		{slot: begin},
		{slot: middle, savedState: true},
		{slot: end, canonicalBlock: true},
	}
	hist := newMockHistory(t, specs, end+1)
	ch := &CanonicalHistory{h: hist, cc: hist, cs: hist}

	endRoot := hist.slotMap[end]
	st, bs, err := ch.ancestorChain(ctx, endRoot, end)
	require.NoError(t, err)

	// middle is the most recent slot where savedState == true
	require.Equal(t, 1, len(bs))
	require.Equal(t, endRoot, bs[0])
	expectedHTR, err := hist.states[hist.slotMap[middle]].HashTreeRoot(ctx)
	require.NoError(t, err)
	actualHTR, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedHTR, actualHTR)

	st, bs, err = ch.ancestorChain(ctx, hist.slotMap[middle], middle)
	require.NoError(t, err)
	actualHTR, err = st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, len(bs))
	require.Equal(t, expectedHTR, actualHTR)
}

func TestChainForSlot(t *testing.T) {
	ctx := context.Background()
	var zero, one, two, three primitives.Slot = 50, 51, 150, 151
	specs := []mockHistorySpec{
		{slot: zero, canonicalBlock: true, savedState: true},
		{slot: one, canonicalBlock: true},
		{slot: two},
		{slot: three, canonicalBlock: true},
	}
	hist := newMockHistory(t, specs, three+10)
	ch := &CanonicalHistory{h: hist, cc: hist, cs: hist}
	firstNonGenesisRoot := hist.slotMap[zero]
	nonGenesisStateRoot, err := hist.states[firstNonGenesisRoot].HashTreeRoot(ctx)
	require.NoError(t, err)

	cases := []struct {
		name       string
		slot       primitives.Slot
		stateRoot  [32]byte
		blockRoots [][32]byte
	}{
		{
			name:       "above latest slot (but before current slot)",
			slot:       three + 1,
			stateRoot:  nonGenesisStateRoot,
			blockRoots: [][32]byte{hist.slotMap[one], hist.slotMap[two], hist.slotMap[three]},
		},
		{
			name:       "last canonical slot - two treated as canonical because it is parent of three",
			slot:       three,
			stateRoot:  nonGenesisStateRoot,
			blockRoots: [][32]byte{hist.slotMap[one], hist.slotMap[two], hist.slotMap[three]},
		},
		{
			name:       "non-canonical slot skipped",
			slot:       two,
			stateRoot:  nonGenesisStateRoot,
			blockRoots: [][32]byte{hist.slotMap[one]},
		},
		{
			name:       "first canonical slot",
			slot:       one,
			stateRoot:  nonGenesisStateRoot,
			blockRoots: [][32]byte{hist.slotMap[one]},
		},
		{
			name:       "slot at saved state",
			slot:       zero,
			stateRoot:  nonGenesisStateRoot,
			blockRoots: [][32]byte{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, roots, err := ch.chainForSlot(ctx, c.slot)
			require.NoError(t, err)
			actualStRoot, err := st.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, c.stateRoot, actualStRoot)
			require.Equal(t, len(c.blockRoots), len(roots))
			for i, root := range roots {
				require.Equal(t, c.blockRoots[i], root)
			}
		})
	}
}

func TestAncestorChainOrdering(t *testing.T) {
	ctx := context.Background()
	var zero, one, two, three, four, five primitives.Slot = 50, 51, 150, 151, 152, 200
	specs := []mockHistorySpec{
		{slot: zero},
		{slot: one, savedState: true},
		{slot: two},
		{slot: three},
		{slot: four},
		{slot: five},
	}

	hist := newMockHistory(t, specs, five+1)
	endRoot := hist.slotMap[specs[len(specs)-1].slot]
	ch := &CanonicalHistory{h: hist, cc: hist, cs: hist}
	st, bs, err := ch.ancestorChain(ctx, endRoot, five)
	require.NoError(t, err)
	expectedRoot, err := hist.states[hist.slotMap[one]].HashTreeRoot(ctx)
	require.NoError(t, err)
	actualRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedRoot, actualRoot)
	// we asked for the chain leading up to five
	// one has the savedState. one is applied to the savedState, so it should be omitted
	// that means we should get two, three, four, five (length of 4)
	require.Equal(t, 4, len(bs))
	for i, slot := range []primitives.Slot{two, three, four, five} {
		require.Equal(t, hist.slotMap[slot], bs[i], fmt.Sprintf("wrong value at index %d", i))
	}

	// do the same query, but with the final state saved
	// we should just get the final state w/o block to apply
	specs[5].savedState = true
	hist = newMockHistory(t, specs, five+1)
	endRoot = hist.slotMap[specs[len(specs)-1].slot]

	ch = &CanonicalHistory{h: hist, cc: hist, cs: hist}
	st, bs, err = ch.ancestorChain(ctx, endRoot, five)
	require.NoError(t, err)
	expectedRoot, err = hist.states[endRoot].HashTreeRoot(ctx)
	require.NoError(t, err)
	actualRoot, err = st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedRoot, actualRoot)
	require.Equal(t, 0, len(bs))

	// Slice off the last element for an odd-size list to cover both paths in reverseBlockRoots.
	specs = specs[:len(specs)-1]
	require.Equal(t, 5, len(specs))
	hist = newMockHistory(t, specs, five+1)

	ch = &CanonicalHistory{h: hist, cc: hist, cs: hist}
	endRoot = hist.slotMap[specs[len(specs)-1].slot]
	st, bs, err = ch.ancestorChain(ctx, endRoot, five)
	require.NoError(t, err)
	expectedRoot, err = hist.states[hist.slotMap[one]].HashTreeRoot(ctx)
	require.NoError(t, err)
	actualRoot, err = st.HashTreeRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, expectedRoot, actualRoot)
	require.Equal(t, 3, len(bs))
	for i, slot := range []primitives.Slot{two, three, four} {
		require.Equal(t, hist.slotMap[slot], bs[i], fmt.Sprintf("wrong value at index %d", i))
	}
}

type mockCanonicalChecker struct {
	isCanon func([32]byte) (bool, error)
	is      bool
	err     error
}

func (m *mockCanonicalChecker) IsCanonical(_ context.Context, root [32]byte) (bool, error) {
	if m.isCanon != nil {
		return m.isCanon(root)
	}
	return m.is, m.err
}

func TestReverseBlockRoots(t *testing.T) {
	// test 0,1,2,3 elements to handle: zero case; single element; even number; odd number
	for i := range 4 {
		t.Run(fmt.Sprintf("reverseBlockRoots with %d elements", i), func(t *testing.T) {
			actual := mockRoots(i, incrFwd)
			expected := mockRoots(i, incrBwd)
			reverseBlockRoots(actual)
			if len(actual) != len(expected) {
				t.Errorf("different list lengths")
			}
			for i := range actual {
				require.Equal(t, expected[i], actual[i])
			}
		})
	}
}

func incrBwd(n int, c chan uint32) {
	for i := n - 1; i >= 0; i-- {
		c <- uint32(i)
	}
	close(c)
}

func incrFwd(n int, c chan uint32) {
	for i := range n {
		c <- uint32(i)
	}
	close(c)
}

func mockRoots(n int, iter func(int, chan uint32)) [][32]byte {
	bchan := make(chan uint32)
	go iter(n, bchan)
	roots := make([][32]byte, 0)
	for i := range bchan {
		var root [32]byte
		binary.LittleEndian.PutUint32(root[:], i)
		roots = append(roots, root)
	}
	return roots
}

// A node started from a checkpoint has no history between the backfill
// position and the origin block. The slot replayer must refuse such slots
// instead of falling through to genesis and fabricating a state from the
// genesis state and empty slots.
func TestBlockRootForSlot_RefusesSlotsBeforeOrigin(t *testing.T) {
	ctx := context.Background()
	var backfilled, origin, head primitives.Slot = 20, 100, 150
	specs := []mockHistorySpec{
		{slot: backfilled, canonicalBlock: true},
		{slot: origin, canonicalBlock: true, savedState: true},
		{slot: head, canonicalBlock: true},
	}
	hist := newMockHistory(t, specs, head+1)
	originRoot := hist.slotMap[origin]
	hist.originRoot = &originRoot
	backfillRoot := hist.slotMap[backfilled]
	hist.backfillRoot = &backfillRoot
	ch := &CanonicalHistory{h: hist, cc: hist, cs: hist}

	for _, slot := range []primitives.Slot{backfilled + 1, origin - 1} {
		_, err := ch.BlockRootForSlot(ctx, slot)
		require.ErrorIs(t, err, ErrNoDataForSlot)
		_, err = ch.ReplayerForSlot(slot).ReplayBlocks(ctx)
		require.ErrorIs(t, err, ErrNoDataForSlot)
	}
	for _, c := range []struct {
		slot primitives.Slot
		root [32]byte
	}{
		{slot: 0, root: hist.slotMap[0]},
		{slot: backfilled, root: backfillRoot},
		{slot: origin, root: originRoot},
		{slot: origin + 1, root: originRoot},
		{slot: head, root: hist.slotMap[head]},
	} {
		r, err := ch.BlockRootForSlot(ctx, c.slot)
		require.NoError(t, err)
		require.Equal(t, c.root, r)
	}
}

// Same guarantee against the real database after a checkpoint import: the
// origin and backfill roots written by SaveOrigin bound the served history.
func TestReplayer_CheckpointSyncedDBRefusesPreOriginSlots(t *testing.T) {
	ctx := context.Background()
	beaconDB := testDB.SetupDB(t)
	genesisState, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, beaconDB.SaveGenesisData(ctx, genesisState))
	genesisRoot, err := beaconDB.GenesisBlockRoot(ctx)
	require.NoError(t, err)

	originSlot := primitives.Slot(64)
	originState := genesisState.Copy()
	require.NoError(t, originState.SetSlot(originSlot))
	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = originSlot
	blk.Block.ParentRoot = genesisRoot[:]
	bodyRoot, err := blk.Block.Body.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, originState.SetLatestBlockHeader(&qrysmpb.BeaconBlockHeader{
		Slot:          originSlot,
		ProposerIndex: blk.Block.ProposerIndex,
		ParentRoot:    genesisRoot[:],
		StateRoot:     make([]byte, 32),
		BodyRoot:      bodyRoot[:],
	}))
	stateRoot, err := originState.HashTreeRoot(ctx)
	require.NoError(t, err)
	blk.Block.StateRoot = stateRoot[:]
	sb, err := originState.MarshalSSZ()
	require.NoError(t, err)
	wb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	bb, err := wb.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, beaconDB.SaveOrigin(ctx, sb, bb))

	ch := NewCanonicalHistory(beaconDB, &mockCanonicalChecker{is: true}, &mockCurrentSlotter{Slot: 200})
	_, err = ch.ReplayerForSlot(10).ReplayBlocks(ctx)
	require.ErrorIs(t, err, ErrNoDataForSlot)
	st, err := ch.ReplayerForSlot(originSlot).ReplayBlocks(ctx)
	require.NoError(t, err)
	require.Equal(t, originSlot, st.Slot())
	// Genesis itself is covered.
	st, err = ch.ReplayerForSlot(0).ReplayBlocks(ctx)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), st.Slot())
}

// A checkpoint state may be the origin block's post-state advanced through
// empty slots. It must serve replays for targets at or after its slot instead
// of being rejected (which would send the replay to the unavailable pre-origin
// parent), and must not serve targets before its slot.
func TestAncestorChain_AdvancedCheckpointState(t *testing.T) {
	ctx := context.Background()
	var origin primitives.Slot = 100
	specs := []mockHistorySpec{
		{slot: origin, canonicalBlock: true, savedState: true},
	}
	hist := newMockHistory(t, specs, origin+20)
	originRoot := hist.slotMap[origin]
	advanced := hist.states[originRoot].Copy()
	require.NoError(t, advanced.SetSlot(origin+5))
	hist.states[originRoot] = advanced
	ch := &CanonicalHistory{h: hist, cc: hist, cs: hist}

	st, bs, err := ch.ancestorChain(ctx, originRoot, origin+10)
	require.NoError(t, err)
	require.Equal(t, 0, len(bs))
	require.Equal(t, origin+5, st.Slot())

	st, bs, err = ch.ancestorChain(ctx, originRoot, origin+5)
	require.NoError(t, err)
	require.Equal(t, origin+5, st.Slot())
	require.Equal(t, 0, len(bs))

	// Before the saved state's slot the chain has to come from further back.
	st, bs, err = ch.ancestorChain(ctx, originRoot, origin+2)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(0), st.Slot())
	require.Equal(t, 1, len(bs))
	require.Equal(t, originRoot, bs[0])
}
