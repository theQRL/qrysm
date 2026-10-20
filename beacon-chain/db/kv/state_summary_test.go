package kv

import (
	"context"
	"sync"
	"testing"

	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestStateSummary_CanSaveRetrieve(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	r1 := bytesutil.ToBytes32([]byte{'A'})
	r2 := bytesutil.ToBytes32([]byte{'B'})
	s1 := &qrysmpb.StateSummary{Slot: 1, Root: r1[:]}

	// State summary should not exist yet.
	require.Equal(t, false, db.HasStateSummary(ctx, r1), "State summary should not be saved")
	require.NoError(t, db.SaveStateSummary(ctx, s1))
	require.Equal(t, true, db.HasStateSummary(ctx, r1), "State summary should be saved")

	saved, err := db.StateSummary(ctx, r1)
	require.NoError(t, err)
	assert.DeepEqual(t, s1, saved, "State summary does not equal")

	// Save a new state summary.
	s2 := &qrysmpb.StateSummary{Slot: 2, Root: r2[:]}

	// State summary should not exist yet.
	require.Equal(t, false, db.HasStateSummary(ctx, r2), "State summary should not be saved")
	require.NoError(t, db.SaveStateSummary(ctx, s2))
	require.Equal(t, true, db.HasStateSummary(ctx, r2), "State summary should be saved")

	saved, err = db.StateSummary(ctx, r2)
	require.NoError(t, err)
	assert.DeepEqual(t, s2, saved, "State summary does not equal")
}

func TestStateSummary_CacheToDB(t *testing.T) {
	db := setupDB(t)

	summaries := make([]*qrysmpb.StateSummary, stateSummaryCachePruneCount-1)
	for i := range summaries {
		summaries[i] = &qrysmpb.StateSummary{Slot: primitives.Slot(i), Root: bytesutil.PadTo(bytesutil.Uint64ToBytesLittleEndian(uint64(i)), 32)}
	}

	require.NoError(t, db.SaveStateSummaries(context.Background(), summaries))
	require.Equal(t, db.stateSummaryCache.len(), stateSummaryCachePruneCount-1)

	require.NoError(t, db.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Slot: 1000, Root: []byte{'a', 'b'}}))
	require.Equal(t, db.stateSummaryCache.len(), stateSummaryCachePruneCount)

	require.NoError(t, db.SaveStateSummary(context.Background(), &qrysmpb.StateSummary{Slot: 1001, Root: []byte{'c', 'd'}}))
	require.Equal(t, db.stateSummaryCache.len(), 1)

	for i := range summaries {
		r := bytesutil.Uint64ToBytesLittleEndian(uint64(i))
		require.Equal(t, true, db.HasStateSummary(context.Background(), bytesutil.ToBytes32(r)))
	}
}

func TestStateSummary_CanDelete(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	r1 := bytesutil.ToBytes32([]byte{'A'})
	s1 := &qrysmpb.StateSummary{Slot: 1, Root: r1[:]}

	require.Equal(t, false, db.HasStateSummary(ctx, r1), "State summary should not be saved")
	require.NoError(t, db.SaveStateSummary(ctx, s1))
	require.Equal(t, true, db.HasStateSummary(ctx, r1), "State summary should be saved")

	require.NoError(t, db.deleteStateSummary(r1))
	require.Equal(t, false, db.HasStateSummary(ctx, r1), "State summary should be deleted")
}

// A flush of the state summary cache used to snapshot the cache, write the
// snapshot and then drop the whole cache, losing every summary put while the
// write was in flight.
func TestStateSummary_ConcurrentFlushKeepsEveryPut(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	const writers = 4
	const perWriter = 1500

	root := func(g, i int) [32]byte {
		var r [32]byte
		r[0] = byte(g + 1)
		r[1] = byte(i >> 8)
		r[2] = byte(i)
		return r
	}

	var wg sync.WaitGroup
	for g := 0; g < writers; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				r := root(g, i)
				if err := db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: primitives.Slot(i), Root: r[:]}); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	require.NoError(t, db.saveCachedStateSummariesDB(ctx))
	require.Equal(t, 0, db.stateSummaryCache.len())

	for g := 0; g < writers; g++ {
		for i := 0; i < perWriter; i++ {
			require.Equal(t, true, db.HasStateSummary(ctx, root(g, i)), "summary %d of writer %d was lost", i, g)
		}
	}
}

// A deleted summary must not be resurrected by a flush whose snapshot predates the deletion.

func TestStateSummary_DeleteIsNotUndoneByFlush(t *testing.T) {
	db := setupDB(t)
	ctx := context.Background()
	r := [32]byte{'d'}
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: 3, Root: r[:]}))
	require.NoError(t, db.deleteStateSummary(r))
	require.NoError(t, db.saveCachedStateSummariesDB(ctx))
	require.Equal(t, false, db.HasStateSummary(ctx, r))
}

// GenesisState used to open a second read transaction (through unmarshalState)
// inside its own. bbolt blocks new read transactions while a writer waits to
// remap the file, so the inner read, the outer read and the writer deadlocked.
// The test fails by timing out if that nesting comes back.
