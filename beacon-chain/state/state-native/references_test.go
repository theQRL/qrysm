package state_native

import (
	"reflect"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/state-native/types"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestStateReferenceSharing_Finalizer_Capella(t *testing.T) {
	// This test showcases the logic on the RandaoMixes field with the GC finalizer.

	s, err := InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{RandaoMixes: [][]byte{[]byte("foo")}})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.RandaoMixes].Refs(), "Expected a single reference for RANDAO mixes")

	func() {
		// Create object in a different scope for GC
		b := a.Copy()
		assert.Equal(t, uint(2), a.sharedFieldReferences[types.RandaoMixes].Refs(), "Expected 2 references to RANDAO mixes")
		_ = b
	}()

	runtime.GC() // Should run finalizer on object b
	assert.Equal(t, uint(1), a.sharedFieldReferences[types.RandaoMixes].Refs(), "Expected 1 shared reference to RANDAO mixes!")

	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assert.Equal(t, uint(2), b.sharedFieldReferences[types.RandaoMixes].Refs(), "Expected 2 shared references to RANDAO mixes")
	require.NoError(t, b.UpdateRandaoMixesAtIndex(0, bytesutil.ToBytes32([]byte("bar"))))
	if b.sharedFieldReferences[types.RandaoMixes].Refs() != 1 || a.sharedFieldReferences[types.RandaoMixes].Refs() != 1 {
		t.Error("Expected 1 shared reference to RANDAO mix for both a and b")
	}
}

func TestStateReferenceCopy_NoUnexpectedRootsMutation_Capella(t *testing.T) {
	root1, root2 := bytesutil.ToBytes32([]byte("foo")), bytesutil.ToBytes32([]byte("bar"))
	s, err := InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{
		BlockRoots: [][]byte{
			root1[:],
		},
		StateRoots: [][]byte{
			root1[:],
		},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	require.NoError(t, err)
	assertRefCount(t, a, types.BlockRoots, 1)
	assertRefCount(t, a, types.StateRoots, 1)

	// Copy, increases reference count.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assertRefCount(t, a, types.BlockRoots, 2)
	assertRefCount(t, a, types.StateRoots, 2)
	assertRefCount(t, b, types.BlockRoots, 2)
	assertRefCount(t, b, types.StateRoots, 2)

	// Assert shared state.
	blockRootsA := a.BlockRoots()
	stateRootsA := a.StateRoots()
	blockRootsB := b.BlockRoots()
	stateRootsB := b.StateRoots()
	assertValFound(t, blockRootsA, root1[:])
	assertValFound(t, blockRootsB, root1[:])
	assertValFound(t, stateRootsA, root1[:])
	assertValFound(t, stateRootsB, root1[:])

	// Mutator should only affect calling state: a.
	require.NoError(t, a.UpdateBlockRootAtIndex(0, root2))
	require.NoError(t, a.UpdateStateRootAtIndex(0, root2))

	// Assert no shared state mutation occurred only on state a (copy on write).
	assertValNotFound(t, a.BlockRoots(), root1[:])
	assertValNotFound(t, a.StateRoots(), root1[:])
	assertValFound(t, a.BlockRoots(), root2[:])
	assertValFound(t, a.StateRoots(), root2[:])
	assertValFound(t, b.BlockRoots(), root1[:])
	assertValFound(t, b.StateRoots(), root1[:])
	assert.DeepEqual(t, root2[:], a.BlockRoots()[0], "Expected mutation not found")
	assert.DeepEqual(t, root2[:], a.StateRoots()[0], "Expected mutation not found")
	assert.DeepEqual(t, root1[:], blockRootsB[0], "Unexpected mutation found")
	assert.DeepEqual(t, root1[:], stateRootsB[0], "Unexpected mutation found")

	// Copy on write happened, reference counters are reset.
	assertRefCount(t, a, types.BlockRoots, 1)
	assertRefCount(t, a, types.StateRoots, 1)
	assertRefCount(t, b, types.BlockRoots, 1)
	assertRefCount(t, b, types.StateRoots, 1)
}

func TestStateReferenceCopy_NoUnexpectedRandaoMutation_Capella(t *testing.T) {
	val1, val2 := bytesutil.ToBytes32([]byte("foo")), bytesutil.ToBytes32([]byte("bar"))
	s, err := InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{
		RandaoMixes: [][]byte{
			val1[:],
		},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)
	require.NoError(t, err)
	assertRefCount(t, a, types.RandaoMixes, 1)

	// Copy, increases reference count.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)
	assertRefCount(t, a, types.RandaoMixes, 2)
	assertRefCount(t, b, types.RandaoMixes, 2)

	// Assert shared state.
	mixesA := a.RandaoMixes()
	mixesB := b.RandaoMixes()
	assertValFound(t, mixesA, val1[:])
	assertValFound(t, mixesB, val1[:])

	// Mutator should only affect calling state: a.
	require.NoError(t, a.UpdateRandaoMixesAtIndex(0, val2))

	// Assert no shared state mutation occurred only on state a (copy on write).
	assertValFound(t, a.RandaoMixes(), val2[:])
	assertValNotFound(t, a.RandaoMixes(), val1[:])
	assertValFound(t, b.RandaoMixes(), val1[:])
	assertValNotFound(t, b.RandaoMixes(), val2[:])
	assertValFound(t, mixesB, val1[:])
	assertValNotFound(t, mixesB, val2[:])
	assert.DeepEqual(t, val2[:], a.RandaoMixes()[0], "Expected mutation not found")
	assert.DeepEqual(t, val1[:], mixesB[0], "Unexpected mutation found")

	// Copy on write happened, reference counters are reset.
	assertRefCount(t, a, types.RandaoMixes, 1)
	assertRefCount(t, b, types.RandaoMixes, 1)
}

func TestValidatorReferences_RemainsConsistent_Capella(t *testing.T) {
	s, err := InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{
		Validators: []*zondpb.Validator{
			{PublicKey: []byte{'A'}},
			{PublicKey: []byte{'B'}},
			{PublicKey: []byte{'C'}},
			{PublicKey: []byte{'D'}},
			{PublicKey: []byte{'E'}},
		},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)

	// Create a second state.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)

	// Update First Validator.
	assert.NoError(t, a.UpdateValidatorAtIndex(0, &zondpb.Validator{PublicKey: []byte{'Z'}}))

	assert.DeepNotEqual(t, a.Validators()[0], b.Validators()[0], "validators are equal when they are supposed to be different")
	// Modify all validators from copied state.
	assert.NoError(t, b.ApplyToEveryValidator(func(idx int, val *zondpb.Validator) (bool, *zondpb.Validator, error) {
		return true, &zondpb.Validator{PublicKey: []byte{'V'}}, nil
	}))

	// Ensure reference is properly accounted for.
	assert.NoError(t, a.ReadFromEveryValidator(func(idx int, val state.ReadOnlyValidator) error {
		assert.NotEqual(t, bytesutil.ToBytes48([]byte{'V'}), val.PublicKey())
		return nil
	}))
}

func TestValidatorReferences_ApplyValidator_BalancesRead(t *testing.T) {
	resetCfg := features.InitWithReset(&features.Flags{
		EnableExperimentalState: true,
	})
	defer resetCfg()
	s, err := InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{
		Validators: []*zondpb.Validator{
			{PublicKey: []byte{'A'}},
			{PublicKey: []byte{'B'}},
			{PublicKey: []byte{'C'}},
			{PublicKey: []byte{'D'}},
			{PublicKey: []byte{'E'}},
		},
		Balances: []uint64{0, 0, 0, 0, 0},
	})
	require.NoError(t, err)
	a, ok := s.(*BeaconState)
	require.Equal(t, true, ok)

	// Create a second state.
	copied := a.Copy()
	b, ok := copied.(*BeaconState)
	require.Equal(t, true, ok)

	// Modify all validators from copied state, it should not deadlock.
	assert.NoError(t, b.ApplyToEveryValidator(func(idx int, val *zondpb.Validator) (bool, *zondpb.Validator, error) {
		b, err := b.BalanceAtIndex(0)
		if err != nil {
			return false, nil, err
		}
		newVal := zondpb.CopyValidator(val)
		newVal.EffectiveBalance += b
		val.EffectiveBalance += b
		return true, val, nil
	}))
}

// assertRefCount checks whether reference count for a given state
// at a given index is equal to expected amount.
func assertRefCount(t *testing.T, b *BeaconState, idx types.FieldIndex, want uint) {
	if cnt := b.sharedFieldReferences[idx].Refs(); cnt != want {
		t.Errorf("Unexpected count of references for index %d, want: %v, got: %v", idx, want, cnt)
	}
}

// assertValFound checks whether item with a given value exists in list.
func assertValFound(t *testing.T, vals [][]byte, val []byte) {
	for i := range vals {
		if reflect.DeepEqual(vals[i], val) {
			return
		}
	}
	t.Log(string(debug.Stack()))
	t.Fatalf("Expected value not found (%v), want: %v", vals, val)
}

// assertValNotFound checks whether item with a given value doesn't exist in list.
func assertValNotFound(t *testing.T, vals [][]byte, val []byte) {
	for i := range vals {
		if reflect.DeepEqual(vals[i], val) {
			t.Log(string(debug.Stack()))
			t.Errorf("Unexpected value found (%v),: %v", vals, val)
			return
		}
	}
}
