package state_native

import (
	"fmt"
	"reflect"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/state-native/types"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestStateTrieReferences_Released(t *testing.T) {
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%v", experimental), func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{EnableExperimentalState: experimental})
			defer reset()
			for _, operation := range []string{"finalize", "rebuild", "transferred"} {
				t.Run(operation, func(t *testing.T) {
					st, err := InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Balances: []uint64{1}})
					require.NoError(t, err)
					a := st.(*BeaconState)
					_, err = a.balancesRootSelector(types.Balances)
					require.NoError(t, err)
					b := a.Copy().(*BeaconState)
					shared := a.stateFieldLeaves[types.Balances]
					require.Equal(t, uint(2), shared.FieldReference().Refs())
					var c *BeaconState
					switch operation {
					case "finalize":
						runtime.SetFinalizer(b, nil)
						finalizerCleanup(b)
						require.Equal(t, uint(1), a.valMapHandler.Refs())
					case "rebuild":
						require.NoError(t, b.SetBalances([]uint64{2}))
						_, err = b.balancesRootSelector(types.Balances)
						require.NoError(t, err)
					case "transferred":
						require.NoError(t, b.UpdateBalancesAtIndex(0, 2))
						_, err = b.balancesRootSelector(types.Balances)
						require.NoError(t, err)
						c = a.Copy().(*BeaconState)
						require.NoError(t, a.UpdateBalancesAtIndex(0, 3))
						_, err = a.balancesRootSelector(types.Balances)
						require.NoError(t, err)
					}
					require.Equal(t, uint(1), shared.FieldReference().Refs())
					runtime.KeepAlive(a)
					runtime.KeepAlive(b)
					runtime.KeepAlive(c)
				})
			}
		})
	}
}

func TestMultiValueValidatorReferences_Released(t *testing.T) {
	reset := features.InitWithReset(&features.Flags{EnableExperimentalState: true})
	defer reset()
	for _, appended := range []bool{false, true} {
		for _, finalize := range []bool{false, true} {
			t.Run(fmt.Sprintf("appended=%v/finalize=%v", appended, finalize), func(t *testing.T) {
				st, err := InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
					Validators: []*qrysmpb.Validator{{PublicKey: []byte{1}}},
				})
				require.NoError(t, err)
				original := st.(*BeaconState)
				branch := original.Copy().(*BeaconState)
				collected := make(chan struct{}, 1)
				func() {
					val, err := original.ValidatorAtIndex(0)
					require.NoError(t, err)
					val.EffectiveBalance = 2
					runtime.SetFinalizer(val, func(*qrysmpb.Validator) { collected <- struct{}{} })
					if appended {
						val.PublicKey[0] = 2
						require.NoError(t, branch.AppendValidator(val))
					} else {
						require.NoError(t, branch.UpdateValidatorAtIndex(0, val))
					}
				}()
				snapshot := branch.Copy().(*BeaconState)
				release := func(b *BeaconState) {
					if finalize {
						runtime.SetFinalizer(b, nil)
						finalizerCleanup(b)
					} else {
						require.NoError(t, b.SetValidators(original.Validators()))
					}
				}
				// The copied branch must keep the validator alive after its source releases it.
				release(branch)
				runtime.GC()
				runtime.Gosched()
				select {
				case <-collected:
					t.Fatal("validator was collected while a snapshot still owns it")
				default:
				}
				wantCount := 1
				if appended {
					wantCount = 2
				}
				require.Equal(t, wantCount, snapshot.NumValidators())
				val, err := snapshot.ValidatorAtIndex(primitives.ValidatorIndex(wantCount - 1))
				require.NoError(t, err)
				require.Equal(t, uint64(2), val.EffectiveBalance)
				// The original state still keeps the shared container reachable.
				release(snapshot)
				wasCollected := false
				for i := 0; i < 20 && !wasCollected; i++ {
					runtime.GC()
					runtime.Gosched()
					select {
					case <-collected:
						wasCollected = true
					default:
					}
				}
				require.Equal(t, true, wasCollected, "released validator is retained by shared storage")
				require.Equal(t, 1, original.NumValidators())
				runtime.KeepAlive(original)
				runtime.KeepAlive(branch)
				runtime.KeepAlive(snapshot)
			})
		}
	}
}

func TestStateReferenceSharing_Finalizer_Zond(t *testing.T) {
	// This test showcases the logic on the RandaoMixes field with the GC finalizer.

	s, err := InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{RandaoMixes: [][]byte{[]byte("foo")}})
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

func TestStateReferenceCopy_NoUnexpectedRootsMutation_Zond(t *testing.T) {
	root1, root2 := bytesutil.ToBytes32([]byte("foo")), bytesutil.ToBytes32([]byte("bar"))
	s, err := InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{
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

func TestStateReferenceCopy_NoUnexpectedRandaoMutation_Zond(t *testing.T) {
	val1, val2 := bytesutil.ToBytes32([]byte("foo")), bytesutil.ToBytes32([]byte("bar"))
	s, err := InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{
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

func TestValidatorReferences_RemainsConsistent_Zond(t *testing.T) {
	s, err := InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{
		Validators: []*qrysmpb.Validator{
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
	assert.NoError(t, a.UpdateValidatorAtIndex(0, &qrysmpb.Validator{PublicKey: []byte{'Z'}}))

	assert.DeepNotEqual(t, a.Validators()[0], b.Validators()[0], "validators are equal when they are supposed to be different")
	// Modify all validators from copied state.
	assert.NoError(t, b.ApplyToEveryValidator(func(idx int, val *qrysmpb.Validator) (bool, *qrysmpb.Validator, error) {
		return true, &qrysmpb.Validator{PublicKey: []byte{'V'}}, nil
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
	s, err := InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{
		Validators: []*qrysmpb.Validator{
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
	assert.NoError(t, b.ApplyToEveryValidator(func(idx int, val *qrysmpb.Validator) (bool, *qrysmpb.Validator, error) {
		b, err := b.BalanceAtIndex(0)
		if err != nil {
			return false, nil, err
		}
		newVal := qrysmpb.CopyValidator(val)
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
