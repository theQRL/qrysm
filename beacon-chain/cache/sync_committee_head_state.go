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

// NewSyncCommitteeHeadState initializes a LRU cache for `SyncCommitteeHeadState` with size of 1.
func NewSyncCommitteeHeadState() *SyncCommitteeHeadStateCache {
	c := lruwrpr.New(1) // only need size of 1 to avoid redundant state copies, hashing, and slot processing.
	return &SyncCommitteeHeadStateCache{cache: c}
}

// Put `slot` as key and `state` as value onto the cache.
func (c *SyncCommitteeHeadStateCache) Put(slot primitives.Slot, st state.BeaconState) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	// Make sure that the provided state is non nil
	// and is of the correct type.
	if st == nil || st.IsNil() {
		return ErrNilValueProvided
	}

	c.cache.Add(slot, st)
	return nil
}

// Get `state` using `slot` as key. Return nil if nothing is found.
func (c *SyncCommitteeHeadStateCache) Get(slot primitives.Slot) (state.BeaconState, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	val, exists := c.cache.Get(slot)
	if !exists {
		return nil, ErrNotFound
	}
	st, ok := val.(state.BeaconState)
	if !ok {
		return nil, ErrIncorrectType
	}

	return st, nil
}
