package doublylinkedtree

import (
	"context"
	"sort"
	"testing"

	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/require"
)

func TestSetOptimisticToInvalid_NoViableTips(t *testing.T) {
	for _, tt := range []struct {
		name             string
		checkpointSource primitives.Epoch
		remainingSources []primitives.Epoch
		wantOptimistic   bool
	}{
		{name: "stale checkpoint alone", checkpointSource: 1, wantOptimistic: true},
		{name: "stale remaining tip", checkpointSource: 1, remainingSources: []primitives.Epoch{1}, wantOptimistic: true},
		{name: "viable internal node with stale tip", checkpointSource: 1, remainingSources: []primitives.Epoch{2, 1}, wantOptimistic: true},
		{name: "viable sibling", checkpointSource: 1, remainingSources: []primitives.Epoch{2}},
		{name: "viable checkpoint alone", checkpointSource: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f := setup(0, 0)
			f.justifiedBalances = []uint64{100}
			epochSlots := params.BeaconConfig().SlotsPerEpoch
			driftGenesisTime(f, 4*epochSlots, 1)
			insert := func(slot primitives.Slot, root, parent [32]byte, source primitives.Epoch) {
				t.Helper()
				_, block, err := prepareForkchoiceState(ctx, slot, root, parent, root, source, 0)
				require.NoError(t, err)
				_, err = f.store.insert(ctx, block, source, 0)
				require.NoError(t, err)
			}
			checkpoint := indexToHash(1)
			insert(2*epochSlots, checkpoint, params.BeaconConfig().ZeroHash, tt.checkpointSource)
			require.NoError(t, f.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: 2, Root: checkpoint}))
			require.NoError(t, f.SetOptimisticToValid(ctx, checkpoint))
			remaining := checkpoint
			for i, source := range tt.remainingSources {
				root := indexToHash(uint64(i + 2))
				insert(3*epochSlots+primitives.Slot(i), root, remaining, source)
				remaining = root
			}
			require.NoError(t, f.SetOptimisticToValid(ctx, remaining))
			invalidRoot, invalidChild := indexToHash(90), indexToHash(91)
			insert(3*epochSlots, invalidRoot, checkpoint, 2)
			insert(3*epochSlots+1, invalidChild, invalidRoot, 2)
			f.ProcessAttestation(ctx, []uint64{0}, invalidChild, 3)
			head, err := f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, invalidChild, head)

			removed, err := f.SetOptimisticToInvalid(ctx, invalidRoot, checkpoint, checkpoint)
			require.NoError(t, err)
			require.DeepEqual(t, [][32]byte{invalidChild, invalidRoot}, removed)
			// Status must already be correct before Head recomputes cached
			// best descendants that used to point into the invalid branch.
			optimistic, err := f.IsOptimistic(checkpoint)
			require.NoError(t, err)
			require.Equal(t, tt.wantOptimistic, optimistic)
			head, err = f.Head(ctx)
			require.NoError(t, err)
			if tt.wantOptimistic {
				require.Equal(t, checkpoint, head)
			} else {
				require.Equal(t, remaining, head)
			}
			optimistic, err = f.IsOptimistic(head)
			require.NoError(t, err)
			require.Equal(t, tt.wantOptimistic, optimistic)
			if !tt.wantOptimistic {
				return
			}

			// Payload validation alone cannot recover an obsolete branch.
			staleReplacement := indexToHash(100)
			insert(3*epochSlots+2, staleReplacement, checkpoint, 1)
			require.NoError(t, f.SetOptimisticToValid(ctx, staleReplacement))
			head, err = f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, checkpoint, head)
			optimistic, err = f.IsOptimistic(head)
			require.NoError(t, err)
			require.Equal(t, true, optimistic)

			// A valid branch with the justified voting source ends recovery.
			replacement := indexToHash(101)
			insert(3*epochSlots+3, replacement, checkpoint, 2)
			require.NoError(t, f.SetOptimisticToValid(ctx, replacement))
			head, err = f.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, replacement, head)
			optimistic, err = f.IsOptimistic(head)
			require.NoError(t, err)
			require.Equal(t, false, optimistic)
		})
	}
}

func TestSetOptimisticToInvalid_JustifiedCheckpointRemoved(t *testing.T) {
	ctx := context.Background()
	f := setup(0, 0)
	epochSlots := params.BeaconConfig().SlotsPerEpoch
	driftGenesisTime(f, 4*epochSlots, 1)
	checkpoint := indexToHash(1)
	_, block, err := prepareForkchoiceState(ctx, 2*epochSlots, checkpoint, params.BeaconConfig().ZeroHash, checkpoint, 2, 0)
	require.NoError(t, err)
	_, err = f.store.insert(ctx, block, 2, 0)
	require.NoError(t, err)
	require.NoError(t, f.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: 2, Root: checkpoint}))
	require.NoError(t, f.SetOptimisticToValid(ctx, params.BeaconConfig().ZeroHash))
	_, err = f.SetOptimisticToInvalid(ctx, checkpoint, params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash)
	require.NoError(t, err)
	_, err = f.Head(ctx)
	require.ErrorContains(t, errUnknownJustifiedRoot.Error(), err)
	optimistic, err := f.IsOptimistic(params.BeaconConfig().ZeroHash)
	require.NoError(t, err)
	require.Equal(t, true, optimistic)
}

func TestSetOptimisticToInvalid_GenesisRootAlias(t *testing.T) {
	ctx := context.Background()
	f := New()
	driftGenesisTime(f, 2, 1)
	genesis, invalid := indexToHash(1), indexToHash(2)
	_, block, err := prepareForkchoiceState(ctx, 0, genesis, [32]byte{}, genesis, 0, 0)
	require.NoError(t, err)
	_, err = f.store.insert(ctx, block, 0, 0)
	require.NoError(t, err)
	require.NoError(t, f.SetOptimisticToValid(ctx, genesis))
	_, block, err = prepareForkchoiceState(ctx, 1, invalid, genesis, invalid, 0, 0)
	require.NoError(t, err)
	_, err = f.store.insert(ctx, block, 0, 0)
	require.NoError(t, err)
	_, err = f.SetOptimisticToInvalid(ctx, invalid, genesis, genesis)
	require.NoError(t, err)
	// Genesis checkpoints may use a zero root even though the actual tree
	// root is nonzero. Its validated, viable leaf must remain non-optimistic.
	require.Equal(t, params.BeaconConfig().ZeroHash, f.JustifiedCheckpoint().Root)
	optimistic, err := f.IsOptimistic(genesis)
	require.NoError(t, err)
	require.Equal(t, false, optimistic)
	head, err := f.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, genesis, head)
}

// We test the algorithm to update a node from SYNCING to INVALID
// We start with the same diagram as above:
//
//	              E -- F
//	             /
//	       C -- D
//	      /      \
//	A -- B        G -- H -- I
//	      \        \
//	       J        -- K -- L
//
// And every block in the Fork choice is optimistic.
func TestPruneInvalid(t *testing.T) {
	tests := []struct {
		root             [32]byte // the root of the new INVALID block
		parentRoot       [32]byte // the root of the parent block
		payload          [32]byte // the last valid hash
		wantedNodeNumber int
		wantedRoots      [][32]byte
		wantedErr        error
	}{
		{ // Bogus LVH, root not in forkchoice
			[32]byte{'x'},
			[32]byte{'i'},
			[32]byte{'R'},
			13,
			[][32]byte{},
			nil,
		},
		{
			// Bogus LVH
			[32]byte{'i'},
			[32]byte{'h'},
			[32]byte{'R'},
			12,
			[][32]byte{{'i'}},
			nil,
		},
		{
			[32]byte{'j'},
			[32]byte{'b'},
			[32]byte{'B'},
			12,
			[][32]byte{{'j'}},
			nil,
		},
		{
			[32]byte{'c'},
			[32]byte{'b'},
			[32]byte{'B'},
			4,
			[][32]byte{{'f'}, {'e'}, {'i'}, {'h'}, {'l'},
				{'k'}, {'g'}, {'d'}, {'c'}},
			nil,
		},
		{
			[32]byte{'i'},
			[32]byte{'h'},
			[32]byte{'H'},
			12,
			[][32]byte{{'i'}},
			nil,
		},
		{
			[32]byte{'h'},
			[32]byte{'g'},
			[32]byte{'G'},
			11,
			[][32]byte{{'i'}, {'h'}},
			nil,
		},
		{
			[32]byte{'g'},
			[32]byte{'d'},
			[32]byte{'D'},
			8,
			[][32]byte{{'i'}, {'h'}, {'l'}, {'k'}, {'g'}},
			nil,
		},
		{
			[32]byte{'i'},
			[32]byte{'h'},
			[32]byte{'D'},
			8,
			[][32]byte{{'i'}, {'h'}, {'l'}, {'k'}, {'g'}},
			nil,
		},
		{
			[32]byte{'f'},
			[32]byte{'e'},
			[32]byte{'D'},
			11,
			[][32]byte{{'f'}, {'e'}},
			nil,
		},
		{
			[32]byte{'h'},
			[32]byte{'g'},
			[32]byte{'C'},
			5,
			[][32]byte{
				{'f'},
				{'e'},
				{'i'},
				{'h'},
				{'l'},
				{'k'},
				{'g'},
				{'d'},
			},
			nil,
		},
		{
			[32]byte{'g'},
			[32]byte{'d'},
			[32]byte{'E'},
			8,
			[][32]byte{{'i'}, {'h'}, {'l'}, {'k'}, {'g'}},
			nil,
		},
		{
			[32]byte{'z'},
			[32]byte{'j'},
			[32]byte{'B'},
			12,
			[][32]byte{{'j'}},
			nil,
		},
		{
			[32]byte{'z'},
			[32]byte{'j'},
			[32]byte{'J'},
			13,
			[][32]byte{},
			nil,
		},
		{
			[32]byte{'j'},
			[32]byte{'a'},
			[32]byte{'B'},
			0,
			[][32]byte{},
			errInvalidParentRoot,
		},
		{
			[32]byte{'z'},
			[32]byte{'h'},
			[32]byte{'D'},
			8,
			[][32]byte{{'i'}, {'h'}, {'l'}, {'k'}, {'g'}},
			nil,
		},
		{
			[32]byte{'z'},
			[32]byte{'h'},
			[32]byte{'D'},
			8,
			[][32]byte{{'i'}, {'h'}, {'l'}, {'k'}, {'g'}},
			nil,
		},
	}
	for _, tc := range tests {
		ctx := context.Background()
		f := setup(1, 1)

		state, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'j'}, [32]byte{'b'}, [32]byte{'J'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 103, [32]byte{'d'}, [32]byte{'c'}, [32]byte{'D'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 104, [32]byte{'e'}, [32]byte{'d'}, [32]byte{'E'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 104, [32]byte{'g'}, [32]byte{'d'}, [32]byte{'G'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'f'}, [32]byte{'e'}, [32]byte{'F'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'h'}, [32]byte{'g'}, [32]byte{'H'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 105, [32]byte{'k'}, [32]byte{'g'}, [32]byte{'K'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 106, [32]byte{'i'}, [32]byte{'h'}, [32]byte{'I'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))
		state, blkRoot, err = prepareForkchoiceState(ctx, 106, [32]byte{'l'}, [32]byte{'k'}, [32]byte{'L'}, 1, 1)
		require.NoError(t, err)
		require.NoError(t, f.InsertNode(ctx, state, blkRoot))

		roots, err := f.store.setOptimisticToInvalid(context.Background(), tc.root, tc.parentRoot, tc.payload)
		if tc.wantedErr == nil {
			require.NoError(t, err)
			require.DeepEqual(t, tc.wantedRoots, roots)
			require.Equal(t, tc.wantedNodeNumber, f.NodeCount())
		} else {
			require.ErrorIs(t, tc.wantedErr, err)
		}
	}
}

// This is a regression test (10445)
func TestSetOptimisticToInvalid_ProposerBoost(t *testing.T) {
	ctx := context.Background()
	f := setup(1, 1)

	state, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	f.store.proposerBoostRoot = [32]byte{'c'}
	f.store.previousProposerBoostScore = 10
	f.store.previousProposerBoostRoot = [32]byte{'b'}

	_, err = f.SetOptimisticToInvalid(ctx, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'A'})
	require.NoError(t, err)
	require.Equal(t, uint64(0), f.store.previousProposerBoostScore)
	require.DeepEqual(t, [32]byte{}, f.store.proposerBoostRoot)
	require.DeepEqual(t, params.BeaconConfig().ZeroHash, f.store.previousProposerBoostRoot)
}

// This is a regression test (10565)
//     ----- C
//   /
//  A <- B
//   \
//     ----------D
// D is invalid

func TestSetOptimisticToInvalid_CorrectChildren(t *testing.T) {
	ctx := context.Background()
	f := setup(1, 1)

	state, blkRoot, err := prepareForkchoiceState(ctx, 100, [32]byte{'a'}, params.BeaconConfig().ZeroHash, [32]byte{'A'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 101, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 102, [32]byte{'c'}, [32]byte{'a'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))
	state, blkRoot, err = prepareForkchoiceState(ctx, 103, [32]byte{'d'}, [32]byte{'a'}, [32]byte{'D'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, blkRoot))

	_, err = f.store.setOptimisticToInvalid(ctx, [32]byte{'d'}, [32]byte{'a'}, [32]byte{'A'})
	require.NoError(t, err)
	require.Equal(t, 2, len(f.store.nodeByRoot[[32]byte{'a'}].children))

}

// Regression test for the upstream startup crash reported in #14892:
// setOptimisticToInvalid must not panic when the looked-up node has no parent.
func TestSetOptimisticToInvalid_NilParent(t *testing.T) {
	f := setup(1, 1)
	root := [32]byte{'n'}
	parentRoot := [32]byte{'p'}

	f.store.nodeByRoot[root] = &Node{
		root:       root,
		optimistic: true,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("setOptimisticToInvalid panicked with nil parent: %v", r)
		}
	}()

	_, err := f.store.setOptimisticToInvalid(context.Background(), root, parentRoot, [32]byte{'L'})
	require.ErrorIs(t, err, errInvalidParentRoot)
}

// Pow       |      Pos
//
//	CA -- A -- B -- C-----D
//	 \          \--------------E
//	  \
//	   ----------------------F -- G
//
// B is INVALID
func TestSetOptimisticToInvalid_ForkAtMerge(t *testing.T) {
	ctx := context.Background()
	f := setup(1, 1)

	st, root, err := prepareForkchoiceState(ctx, 100, [32]byte{'r'}, [32]byte{}, [32]byte{}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 101, [32]byte{'a'}, [32]byte{'r'}, [32]byte{}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 102, [32]byte{'b'}, [32]byte{'a'}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 103, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 104, [32]byte{'d'}, [32]byte{'c'}, [32]byte{'D'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 105, [32]byte{'e'}, [32]byte{'b'}, [32]byte{'E'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 106, [32]byte{'f'}, [32]byte{'r'}, [32]byte{'F'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 107, [32]byte{'g'}, [32]byte{'f'}, [32]byte{'G'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	roots, err := f.SetOptimisticToInvalid(ctx, [32]byte{'x'}, [32]byte{'d'}, [32]byte{})
	require.NoError(t, err)
	require.Equal(t, 4, len(roots))
	sort.Slice(roots, func(i, j int) bool {
		return bytesutil.BytesToUint64BigEndian(roots[i][:]) < bytesutil.BytesToUint64BigEndian(roots[j][:])
	})
	require.DeepEqual(t, roots, [][32]byte{{'b'}, {'c'}, {'d'}, {'e'}})
}

// Pow       |      Pos
//
//	CA -------- B -- C-----D
//	 \           \--------------E
//	  \
//	   --A -------------------------F -- G
//
// B is INVALID
func TestSetOptimisticToInvalid_ForkAtMerge_bis(t *testing.T) {
	ctx := context.Background()
	f := setup(1, 1)

	st, root, err := prepareForkchoiceState(ctx, 100, [32]byte{'r'}, [32]byte{}, [32]byte{}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 101, [32]byte{'a'}, [32]byte{'r'}, [32]byte{}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 102, [32]byte{'b'}, [32]byte{}, [32]byte{'B'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 103, [32]byte{'c'}, [32]byte{'b'}, [32]byte{'C'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 104, [32]byte{'d'}, [32]byte{'c'}, [32]byte{'D'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 105, [32]byte{'e'}, [32]byte{'b'}, [32]byte{'E'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 106, [32]byte{'f'}, [32]byte{'a'}, [32]byte{'F'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	st, root, err = prepareForkchoiceState(ctx, 107, [32]byte{'g'}, [32]byte{'f'}, [32]byte{'G'}, 1, 1)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, st, root))

	roots, err := f.SetOptimisticToInvalid(ctx, [32]byte{'x'}, [32]byte{'d'}, [32]byte{})
	require.NoError(t, err)
	require.Equal(t, 4, len(roots))
	sort.Slice(roots, func(i, j int) bool {
		return bytesutil.BytesToUint64BigEndian(roots[i][:]) < bytesutil.BytesToUint64BigEndian(roots[j][:])
	})
	require.DeepEqual(t, roots, [][32]byte{{'b'}, {'c'}, {'d'}, {'e'}})
}

func TestSetOptimisticToValid(t *testing.T) {
	f := setup(1, 1)
	op, err := f.IsOptimistic([32]byte{})
	require.NoError(t, err)
	require.Equal(t, true, op)
	require.NoError(t, f.SetOptimisticToValid(context.Background(), [32]byte{}))
	op, err = f.IsOptimistic([32]byte{})
	require.NoError(t, err)
	require.Equal(t, false, op)
}
