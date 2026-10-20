package kv

import (
	"sync"

	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

const stateSummaryCachePruneCount = 128

// stateSummaryCache caches state summary object.
type stateSummaryCache struct {
	initSyncStateSummaries     map[[32]byte]*qrysmpb.StateSummary
	initSyncStateSummariesLock sync.RWMutex
	// flushLock serializes a flush of the cache to the database with any
	// deletion of a summary. A flush writes a snapshot of the cache and must
	// not race a delete (the snapshot would resurrect the deleted summary), and
	// two flushes must not overlap.
	flushLock sync.Mutex
}

// newStateSummaryCache creates a new state summary cache.
func newStateSummaryCache() *stateSummaryCache {
	return &stateSummaryCache{
		initSyncStateSummaries: make(map[[32]byte]*qrysmpb.StateSummary),
	}
}

// put saves a state summary to the initial sync state summaries cache.
func (c *stateSummaryCache) put(r [32]byte, b *qrysmpb.StateSummary) {
	c.initSyncStateSummariesLock.Lock()
	defer c.initSyncStateSummariesLock.Unlock()
	c.initSyncStateSummaries[r] = b
}

// has checks if a state summary exists in the initial sync state summaries cache using the root
// of the block.
func (c *stateSummaryCache) has(r [32]byte) bool {
	c.initSyncStateSummariesLock.RLock()
	defer c.initSyncStateSummariesLock.RUnlock()
	_, ok := c.initSyncStateSummaries[r]
	return ok
}

// delete state summary in cache.
func (c *stateSummaryCache) delete(r [32]byte) {
	c.initSyncStateSummariesLock.Lock()
	defer c.initSyncStateSummariesLock.Unlock()
	delete(c.initSyncStateSummaries, r)
}

// get retrieves a state summary from the initial sync state summaries cache using the root of
// the block.
func (c *stateSummaryCache) get(r [32]byte) *qrysmpb.StateSummary {
	c.initSyncStateSummariesLock.RLock()
	defer c.initSyncStateSummariesLock.RUnlock()
	b := c.initSyncStateSummaries[r]
	return b
}

// len retrieves the state summary count from the state summaries cache.
func (c *stateSummaryCache) len() int {
	c.initSyncStateSummariesLock.RLock()
	defer c.initSyncStateSummariesLock.RUnlock()
	return len(c.initSyncStateSummaries)
}

// GetAll retrieves all the beacon state summaries from the initial sync state summaries cache, the returned
// state summaries are unordered.
func (c *stateSummaryCache) getAll() []*qrysmpb.StateSummary {
	c.initSyncStateSummariesLock.RLock()
	defer c.initSyncStateSummariesLock.RUnlock()

	summaries := make([]*qrysmpb.StateSummary, 0, len(c.initSyncStateSummaries))
	for _, b := range c.initSyncStateSummaries {
		summaries = append(summaries, b)
	}
	return summaries
}

// clear drops every cached summary. Only for tests; a flush must use deleteMany
// so that summaries added during the flush survive.
func (c *stateSummaryCache) clear() {
	c.initSyncStateSummariesLock.Lock()
	defer c.initSyncStateSummariesLock.Unlock()
	c.initSyncStateSummaries = make(map[[32]byte]*qrysmpb.StateSummary)
}

// deleteMany removes the given summaries from the cache. Unlike dropping the
// whole map, this keeps summaries that were added after the caller took its
// snapshot, so a flush never discards a summary it did not write.
func (c *stateSummaryCache) deleteMany(summaries []*qrysmpb.StateSummary) {
	c.initSyncStateSummariesLock.Lock()
	defer c.initSyncStateSummariesLock.Unlock()
	for _, s := range summaries {
		delete(c.initSyncStateSummaries, bytesutil.ToBytes32(s.Root))
	}
}
