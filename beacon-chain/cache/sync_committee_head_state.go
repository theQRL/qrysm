package cache

import (
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/theQRL/qrysm/beacon-chain/state"
	lruwrpr "github.com/theQRL/qrysm/cache/lru"
	"github.com/theQRL/qrysm/consensus-types/primitives"
)

// SyncCommitteeHeadStateCache for the latest head state requested by a sync committee participant.
type SyncCommitteeHeadStateCache struct {
	cache *lru.Cache
	lock  sync.RWMutex
}

type syncCommitteeHeadStateKey struct {
	headRoot [32]byte
	slot     primitives.Slot
}

// NewSyncCommitteeHeadState initializes a LRU cache for `SyncCommitteeHeadState` with size of 1.
func NewSyncCommitteeHeadState() *SyncCommitteeHeadStateCache {
	c := lruwrpr.New(1) // only need size of 1 to avoid redundant state copies, hashing, and slot processing.
	return &SyncCommitteeHeadStateCache{cache: c}
}

// Put caches the state advanced to slot on the branch identified by headRoot.
func (c *SyncCommitteeHeadStateCache) Put(headRoot [32]byte, slot primitives.Slot, st state.BeaconState) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	// Make sure that the provided state is non nil
	// and is of the correct type.
	if st == nil || st.IsNil() {
		return ErrNilValueProvided
	}

	c.cache.Add(syncCommitteeHeadStateKey{headRoot: headRoot, slot: slot}, st)
	return nil
}

// Get returns the state for the requested head and slot, or ErrNotFound.
func (c *SyncCommitteeHeadStateCache) Get(headRoot [32]byte, slot primitives.Slot) (state.BeaconState, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	val, exists := c.cache.Get(syncCommitteeHeadStateKey{headRoot: headRoot, slot: slot})
	if !exists {
		return nil, ErrNotFound
	}
	st, ok := val.(state.BeaconState)
	if !ok {
		return nil, ErrIncorrectType
	}

	return st, nil
}
