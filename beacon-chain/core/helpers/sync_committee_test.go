package helpers

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/cache"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestIsCurrentEpochSyncCommittee_UsingCache(t *testing.T) {
	ClearCache()
	defer ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	r := [32]byte{'a'}
	require.NoError(t, err, syncCommitteeCache.UpdatePositionsInCommittee(r, state))

	ok, err := IsCurrentPeriodSyncCommittee(state, 0)
	require.NoError(t, err)
	require.Equal(t, true, ok)
}

func TestIsCurrentEpochSyncCommittee_UsingCommittee(t *testing.T) {
	ClearCache()
	defer ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	ok, err := IsCurrentPeriodSyncCommittee(state, 0)
	require.NoError(t, err)
	require.Equal(t, true, ok)
}

func TestIsCurrentEpochSyncCommittee_DoesNotExist(t *testing.T) {
	ClearCache()
	defer ClearCache()
	params.SetupTestConfigCleanup(t)
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))
	// Every test state at the genesis slot shares the ZeroHash sync-period
	// cache key, so an async cache fill spawned by an earlier test can land
	// after this test's ClearCache and satisfy the lookup from the cache
	// (returning no error). A slot in a distinct sync period gives this test
	// its own cache key, forcing the state lookup path deterministically.
	uniquePeriodSlot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerSyncCommitteePeriod) * 7)
	require.NoError(t, state.SetSlot(uniquePeriodSlot+1))

	ok, err := IsCurrentPeriodSyncCommittee(state, 12390192)
	require.ErrorContains(t, "validator index 12390192 does not exist", err)
	require.Equal(t, false, ok)
}

func TestIsNextEpochSyncCommittee_UsingCache(t *testing.T) {
	ClearCache()
	defer ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	r := [32]byte{'a'}
	require.NoError(t, err, syncCommitteeCache.UpdatePositionsInCommittee(r, state))

	ok, err := IsNextPeriodSyncCommittee(state, 0)
	require.NoError(t, err)
	require.Equal(t, true, ok)
}

func TestIsNextEpochSyncCommittee_UsingCommittee(t *testing.T) {
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	ok, err := IsNextPeriodSyncCommittee(state, 0)
	require.NoError(t, err)
	require.Equal(t, true, ok)
}

func TestIsNextEpochSyncCommittee_DoesNotExist(t *testing.T) {
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	ok, err := IsNextPeriodSyncCommittee(state, 120391029)
	require.ErrorContains(t, "validator index 120391029 does not exist", err)
	require.Equal(t, false, ok)
}

func TestCurrentEpochSyncSubcommitteeIndices_UsingCache(t *testing.T) {
	ClearCache()
	defer ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	r := [32]byte{'a'}
	require.NoError(t, err, syncCommitteeCache.UpdatePositionsInCommittee(r, state))

	index, err := CurrentPeriodSyncSubcommitteeIndices(state, 0)
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, index)
}

func TestCurrentEpochSyncSubcommitteeIndices_UsingCommittee(t *testing.T) {
	ClearCache()
	defer ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))
	root, err := syncPeriodBoundaryRoot(state)
	require.NoError(t, err)

	// Test that cache was empty.
	_, err = syncCommitteeCache.CurrentPeriodIndexPosition(root, 0)
	require.Equal(t, cache.ErrNonExistingSyncCommitteeKey, err)

	// Test that helper can retrieve the index given empty cache.
	index, err := CurrentPeriodSyncSubcommitteeIndices(state, 0)
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, index)

	// Test that cache was able to fill on miss.
	time.Sleep(100 * time.Millisecond)
	index, err = syncCommitteeCache.CurrentPeriodIndexPosition(root, 0)
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, index)
}

func TestCurrentEpochSyncSubcommitteeIndices_DoesNotExist(t *testing.T) {
	ClearCache()
	defer ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	index, err := CurrentPeriodSyncSubcommitteeIndices(state, 129301923)
	require.ErrorContains(t, "validator index 129301923 does not exist", err)
	require.DeepEqual(t, []primitives.CommitteeIndex(nil), index)
}

func TestNextEpochSyncSubcommitteeIndices_UsingCache(t *testing.T) {
	ClearCache()
	defer ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	r := [32]byte{'a'}
	require.NoError(t, err, syncCommitteeCache.UpdatePositionsInCommittee(r, state))

	index, err := NextPeriodSyncSubcommitteeIndices(state, 0)
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, index)
}

func TestNextEpochSyncSubcommitteeIndices_UsingCommittee(t *testing.T) {
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	index, err := NextPeriodSyncSubcommitteeIndices(state, 0)
	require.NoError(t, err)
	require.DeepEqual(t, []primitives.CommitteeIndex{0}, index)
}

func TestNextEpochSyncSubcommitteeIndices_DoesNotExist(t *testing.T) {
	ClearCache()
	defer ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, 48)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, 48))
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	index, err := NextPeriodSyncSubcommitteeIndices(state, 21093019)
	require.ErrorContains(t, "validator index 21093019 does not exist", err)
	require.DeepEqual(t, []primitives.CommitteeIndex(nil), index)
}

func TestUpdateSyncCommitteeCache_BadSlot(t *testing.T) {
	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: 1,
	})
	require.NoError(t, err)
	err = UpdateSyncCommitteeCache(state)
	require.ErrorContains(t, "not at the end of the epoch to update cache", err)

	state, err = state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot: params.BeaconConfig().SlotsPerEpoch - 1,
	})
	require.NoError(t, err)
	err = UpdateSyncCommitteeCache(state)
	require.ErrorContains(t, "not at sync committee period boundary to update cache", err)
}

func TestUpdateSyncCommitteeCache_BadRoot(t *testing.T) {
	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot:              primitives.Slot(params.BeaconConfig().EpochsPerSyncCommitteePeriod)*params.BeaconConfig().SlotsPerEpoch - 1,
		LatestBlockHeader: &qrysmpb.BeaconBlockHeader{StateRoot: params.BeaconConfig().ZeroHash[:]},
	})
	require.NoError(t, err)
	err = UpdateSyncCommitteeCache(state)
	require.ErrorContains(t, "zero hash state root can't be used to update cache", err)
}

func TestIsCurrentEpochSyncCommittee_SameBlockRoot(t *testing.T) {
	ClearCache()
	defer ClearCache()
	validators := make([]*qrysmpb.Validator, params.BeaconConfig().SyncCommitteeSize)
	syncCommittee := &qrysmpb.SyncCommittee{}
	for i := range validators {
		k := make([]byte, field_params.MLDSA87PubkeyLength)
		copy(k, strconv.Itoa(i))
		validators[i] = &qrysmpb.Validator{
			PublicKey: k,
		}
		syncCommittee.Pubkeys = append(syncCommittee.Pubkeys, bytesutil.PadTo(k, field_params.MLDSA87PubkeyLength))
	}

	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	for i := range blockRoots {
		blockRoots[i] = make([]byte, 32)
	}
	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Validators: validators,
		BlockRoots: blockRoots,
	})
	require.NoError(t, err)
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	require.NoError(t, state.SetNextSyncCommittee(syncCommittee))

	comIdxs, err := CurrentPeriodSyncSubcommitteeIndices(state, 10)
	require.NoError(t, err)

	wantedSlot := params.BeaconConfig().EpochsPerSyncCommitteePeriod.Mul(uint64(params.BeaconConfig().SlotsPerEpoch))
	assert.NoError(t, state.SetSlot(primitives.Slot(wantedSlot)))
	syncCommittee, err = state.CurrentSyncCommittee()
	assert.NoError(t, err)
	rand.Shuffle(len(syncCommittee.Pubkeys), func(i, j int) {
		syncCommittee.Pubkeys[i], syncCommittee.Pubkeys[j] = syncCommittee.Pubkeys[j], syncCommittee.Pubkeys[i]
	})
	require.NoError(t, state.SetCurrentSyncCommittee(syncCommittee))
	newIdxs, err := CurrentPeriodSyncSubcommitteeIndices(state, 10)
	require.NoError(t, err)
	require.DeepNotEqual(t, comIdxs, newIdxs)
}

// The rotation-time cache write must land on the key that lookups compute for
// the new period and must leave the previous period's entry alone. Keyed by
// the latest block's own slot, it only matched lookups when a block landed on
// the boundary slot, and after a block-less period it overwrote the previous
// period's entry with the rotated committees.
func TestUpdateSyncCommitteeCache_KeyMatchesBoundaryLookup(t *testing.T) {
	cfg := params.BeaconConfig()
	boundary := primitives.Slot(cfg.EpochsPerSyncCommitteePeriod) * cfg.SlotsPerEpoch
	for _, headerSlot := range []primitives.Slot{0, boundary / 2} {
		t.Run(fmt.Sprintf("latest block at slot %d", headerSlot), func(t *testing.T) {
			ClearCache()
			t.Cleanup(ClearCache)
			validators := make([]*qrysmpb.Validator, cfg.SyncCommitteeSize)
			keys := make([][]byte, len(validators))
			for i := range validators {
				keys[i] = bytesutil.PadTo([]byte(strconv.Itoa(i)), field_params.MLDSA87PubkeyLength)
				validators[i] = &qrysmpb.Validator{PublicKey: keys[i]}
			}
			header := &qrysmpb.BeaconBlockHeader{
				Slot:       headerSlot,
				ParentRoot: make([]byte, 32),
				StateRoot:  bytesutil.PadTo([]byte{1}, 32),
				BodyRoot:   make([]byte, 32),
			}
			headerRoot, err := header.HashTreeRoot()
			require.NoError(t, err)
			// With a later block, the root recorded at slot 0 differs from the
			// latest header. Without one, both are the genesis block.
			genesisRoot := headerRoot
			if headerSlot != 0 {
				genesisRoot = [32]byte{2}
			}
			blockRoots := make([][]byte, cfg.SlotsPerHistoricalRoot)
			for i := range blockRoots {
				blockRoots[i] = make([]byte, 32)
			}
			blockRoots[0] = genesisRoot[:]
			// process_slot records the latest header root before process_epoch.
			blockRoots[uint64((boundary-1)%cfg.SlotsPerHistoricalRoot)] = headerRoot[:]
			st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
				Slot:              boundary - 1,
				Validators:        validators,
				BlockRoots:        blockRoots,
				LatestBlockHeader: header,
			})
			require.NoError(t, err)
			reversed := make([][]byte, len(keys))
			rotated := make([][]byte, len(keys))
			for i := range keys {
				reversed[i] = keys[len(keys)-1-i]
				rotated[i] = keys[(i+1)%len(keys)]
			}
			committees := []*qrysmpb.SyncCommittee{{Pubkeys: keys}, {Pubkeys: reversed}, {Pubkeys: rotated}}
			require.NoError(t, st.SetCurrentSyncCommittee(committees[0]))
			require.NoError(t, st.SetNextSyncCommittee(committees[1]))
			oldKey, err := syncPeriodBoundaryRoot(st)
			require.NoError(t, err)
			require.NoError(t, syncCommitteeCache.UpdatePositionsInCommittee(oldKey, st))

			// Rotate as process_epoch does at the last slot of the period.
			require.NoError(t, st.SetCurrentSyncCommittee(committees[1]))
			require.NoError(t, st.SetNextSyncCommittee(committees[2]))
			require.NoError(t, UpdateSyncCommitteeCache(st))

			advanced := st.Copy()
			require.NoError(t, advanced.SetSlot(boundary))
			newKey, err := syncPeriodBoundaryRoot(advanced)
			require.NoError(t, err)
			require.NotEqual(t, oldKey, newKey)
			for _, tc := range []struct {
				name     string
				cacheKey [32]byte
				current  *qrysmpb.SyncCommittee
				next     *qrysmpb.SyncCommittee
			}{
				{"previous period entry is intact", oldKey, committees[0], committees[1]},
				{"new period entry is warm", newKey, committees[1], committees[2]},
			} {
				for i, key := range keys {
					idx := primitives.ValidatorIndex(i)
					current, err := syncCommitteeCache.CurrentPeriodIndexPosition(tc.cacheKey, idx)
					require.NoError(t, err, tc.name)
					require.DeepEqual(t, findSubCommitteeIndices(key, tc.current.Pubkeys), current, tc.name)
					next, err := syncCommitteeCache.NextPeriodIndexPosition(tc.cacheKey, idx)
					require.NoError(t, err, tc.name)
					require.DeepEqual(t, findSubCommitteeIndices(key, tc.next.Pubkeys), next, tc.name)
				}
			}
		})
	}
}
