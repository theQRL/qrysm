//go:build !fuzz

package cache

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	lruwrpr "github.com/theQRL/qrysm/cache/lru"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/container/slice"
	mathutil "github.com/theQRL/qrysm/math"
)

const (
	// maxCommitteesCacheSize bounds the cached seed and active-validator combinations.
	// Due to reorgs and long finality, it's good to keep the old cache around for quickly switch over.
	maxCommitteesCacheSize = int(32)
)

var (
	// CommitteeCacheMiss tracks the number of committee requests that aren't present in the cache.
	CommitteeCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "committee_cache_miss",
		Help: "The number of committee requests that aren't present in the cache.",
	})
	// CommitteeCacheHit tracks the number of committee requests that are in the cache.
	CommitteeCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "committee_cache_hit",
		Help: "The number of committee requests that are present in the cache.",
	})
)

// CommitteeCache looks up shuffled indices by seed and active-validator membership.
type CommitteeCache struct {
	CommitteeCache *lru.Cache
	lock           sync.RWMutex
	inProgress     map[string]bool
}

// committeeKeyFn includes both the seed and the ordered active indices.
func committeeKeyFn(obj any) (string, error) {
	info, ok := obj.(*Committees)
	if !ok {
		return "", ErrNotCommittee
	}
	return committeeKey(NewCommitteeKey(info.Seed, info.SortedIndices)), nil
}

// NewCommitteesCache creates a new committee cache for storing/accessing shuffled indices of a committee.
func NewCommitteesCache() *CommitteeCache {
	cc := &CommitteeCache{}
	cc.Clear()
	return cc
}

// Clear resets the CommitteeCache to its initial state
func (c *CommitteeCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.CommitteeCache = lruwrpr.New(maxCommitteesCacheSize)
	c.inProgress = make(map[string]bool)
}

// Committee fetches the shuffled indices by slot and committee index. Every list of indices
// represent one committee. Returns true if the list exists with slot and committee index. Otherwise returns false, nil.
func (c *CommitteeCache) Committee(ctx context.Context, slot primitives.Slot, cacheKey CommitteeKey, index primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error) {
	if err := c.checkInProgress(ctx, cacheKey); err != nil {
		return nil, err
	}

	obj, exists := c.CommitteeCache.Get(committeeKey(cacheKey))
	if exists {
		CommitteeCacheHit.Inc()
	} else {
		CommitteeCacheMiss.Inc()
		return nil, nil
	}

	item, ok := obj.(*Committees)
	if !ok {
		return nil, ErrNotCommittee
	}

	committeeCountPerSlot := uint64(1)
	if item.CommitteeCount/uint64(params.BeaconConfig().SlotsPerEpoch) > 1 {
		committeeCountPerSlot = item.CommitteeCount / uint64(params.BeaconConfig().SlotsPerEpoch)
	}
	// Mirror the spec's data.index < committees_per_slot bound so an index
	// equal to the per-slot count cannot resolve to the next slot's committee.
	if uint64(index) >= committeeCountPerSlot {
		return nil, fmt.Errorf("requested index out of bound: committee index %d >= committees per slot %d", index, committeeCountPerSlot)
	}

	indexOffSet, err := mathutil.Add64(uint64(index), uint64(slot.ModSlot(params.BeaconConfig().SlotsPerEpoch).Mul(committeeCountPerSlot)))
	if err != nil {
		return nil, err
	}
	// startEndIndices multiplies the validator count by the offset in uint64;
	// bound the offset first so a wrapped product cannot yield the positions
	// of a legitimate committee.
	if indexOffSet >= item.CommitteeCount {
		return nil, fmt.Errorf("requested index out of bound: committee offset %d >= committee count %d", indexOffSet, item.CommitteeCount)
	}
	start, end := startEndIndices(item, indexOffSet)

	if end > uint64(len(item.ShuffledIndices)) || end < start {
		return nil, errors.New("requested index out of bound")
	}

	return item.ShuffledIndices[start:end], nil
}

// AddCommitteeShuffledList adds Committee shuffled list object to the cache. T
// his method also trims the least recently list if the cache size has ready the max cache size limit.
func (c *CommitteeCache) AddCommitteeShuffledList(ctx context.Context, committees *Committees) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := committeeKeyFn(committees)
	if err != nil {
		return err
	}
	_ = c.CommitteeCache.Add(key, committees)
	return nil
}

// ActiveIndices returns the active indices for a seed and membership combination.
func (c *CommitteeCache) ActiveIndices(ctx context.Context, cacheKey CommitteeKey) ([]primitives.ValidatorIndex, error) {
	if err := c.checkInProgress(ctx, cacheKey); err != nil {
		return nil, err
	}
	obj, exists := c.CommitteeCache.Get(committeeKey(cacheKey))

	if exists {
		CommitteeCacheHit.Inc()
	} else {
		CommitteeCacheMiss.Inc()
		return nil, nil
	}

	item, ok := obj.(*Committees)
	if !ok {
		return nil, ErrNotCommittee
	}

	return item.SortedIndices, nil
}

// ActiveIndicesCount returns the count for a seed and membership combination.
func (c *CommitteeCache) ActiveIndicesCount(ctx context.Context, cacheKey CommitteeKey) (int, error) {
	if err := c.checkInProgress(ctx, cacheKey); err != nil {
		return 0, err
	}

	obj, exists := c.CommitteeCache.Get(committeeKey(cacheKey))
	if exists {
		CommitteeCacheHit.Inc()
	} else {
		CommitteeCacheMiss.Inc()
		return 0, nil
	}

	item, ok := obj.(*Committees)
	if !ok {
		return 0, ErrNotCommittee
	}

	return len(item.SortedIndices), nil
}

// HasEntry returns true if the committee cache has a value.
func (c *CommitteeCache) HasEntry(cacheKey CommitteeKey) bool {
	_, ok := c.CommitteeCache.Get(committeeKey(cacheKey))
	return ok
}

// MarkInProgress a request so that any other similar requests will block on
// Get until MarkNotInProgress is called.
func (c *CommitteeCache) MarkInProgress(cacheKey CommitteeKey) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s := committeeKey(cacheKey)
	if c.inProgress[s] {
		return ErrAlreadyInProgress
	}
	c.inProgress[s] = true
	return nil
}

// MarkNotInProgress will release the lock on a given request. This should be
// called after put.
func (c *CommitteeCache) MarkNotInProgress(cacheKey CommitteeKey) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s := committeeKey(cacheKey)
	delete(c.inProgress, s)
	return nil
}

func startEndIndices(c *Committees, index uint64) (uint64, uint64) {
	validatorCount := uint64(len(c.ShuffledIndices))
	start := slice.SplitOffset(validatorCount, c.CommitteeCount, index)
	end := slice.SplitOffset(validatorCount, c.CommitteeCount, index+1)
	return start, end
}

func key(root [32]byte) string {
	return string(root[:])
}

func committeeKey(cacheKey CommitteeKey) string {
	return string(cacheKey.seed[:]) + string(cacheKey.indicesRoot[:])
}

func (c *CommitteeCache) checkInProgress(ctx context.Context, cacheKey CommitteeKey) error {
	delay := minDelay
	// Another identical request may be in progress already. Let's wait until
	// any in progress request resolves or our timeout is exceeded.
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		c.lock.RLock()
		if !c.inProgress[committeeKey(cacheKey)] {
			c.lock.RUnlock()
			break
		}
		c.lock.RUnlock()

		// This increasing backoff is to decrease the CPU cycles while waiting
		// for the in progress boolean to flip to false.
		time.Sleep(time.Duration(delay) * time.Nanosecond)
		delay *= delayFactor
		delay = math.Min(delay, maxDelay)
	}
	return nil
}
