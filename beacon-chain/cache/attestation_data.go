package cache

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"k8s.io/client-go/tools/cache"
)

var (
	// Delay parameters
	minDelay    = float64(10)        // 10 nanoseconds
	maxDelay    = float64(100000000) // 0.1 second
	delayFactor = 1.1

	// Metrics
	attestationCacheMiss = promauto.NewCounter(prometheus.CounterOpts{
		Name: "attestation_cache_miss",
		Help: "The number of attestation data requests that aren't present in the cache.",
	})
	attestationCacheHit = promauto.NewCounter(prometheus.CounterOpts{
		Name: "attestation_cache_hit",
		Help: "The number of attestation data requests that are present in the cache.",
	})
	attestationCacheSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "attestation_cache_size",
		Help: "The number of attestation data in the attestations cache",
	})
)

// ErrAlreadyInProgress appears when attempting to mark a cache as in progress while it is
// already in progress. The client should handle this error and wait for the in progress
// data to resolve via Get.
var ErrAlreadyInProgress = errors.New("already in progress")

// ErrAttestationDataStale means the head changed while a response was being
// computed. The caller must release its in-progress request and retry.
var ErrAttestationDataStale = errors.New("attestation data invalidated by head change")

// AttestationCache is used to store the cached results of an AttestationData request.
type AttestationCache struct {
	cache      *cache.FIFO
	lock       sync.RWMutex
	inProgress map[string]uint64
	generation uint64
}

// NewAttestationCache initializes the map and underlying cache.
func NewAttestationCache() *AttestationCache {
	return &AttestationCache{
		cache:      cache.NewFIFO(wrapperToKey),
		inProgress: make(map[string]uint64),
	}
}

// Get waits for any in progress calculation to complete before returning a
// cached response, if any.
func (c *AttestationCache) Get(ctx context.Context, req *qrysmpb.AttestationDataRequest) (*qrysmpb.AttestationData, error) {
	if req == nil {
		return nil, errors.New("nil attestation data request")
	}

	s, e := reqToKey(req)
	if e != nil {
		return nil, e
	}

	delay := minDelay

	// Another identical request may be in progress already. Let's wait until
	// any in progress request resolves or our timeout is exceeded.
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		c.lock.RLock()
		if _, pending := c.inProgress[s]; !pending {
			// Keep the read and its copy atomic with Clear and Put.
			defer c.lock.RUnlock()
			break
		}
		c.lock.RUnlock()

		// This increasing backoff is to decrease the CPU cycles while waiting
		// for the in progress boolean to flip to false.
		time.Sleep(time.Duration(delay) * time.Nanosecond)
		delay *= delayFactor
		delay = math.Min(delay, maxDelay)
	}

	item, exists, err := c.cache.GetByKey(s)
	if err != nil {
		return nil, err
	}

	if exists && item != nil && item.(*attestationReqResWrapper).res != nil {
		attestationCacheHit.Inc()
		return qrysmpb.CopyAttestationData(item.(*attestationReqResWrapper).res), nil
	}
	attestationCacheMiss.Inc()
	return nil, nil
}

// MarkInProgress a request so that any other similar requests will block on
// Get until MarkNotInProgress is called.
func (c *AttestationCache) MarkInProgress(req *qrysmpb.AttestationDataRequest) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s, e := reqToKey(req)
	if e != nil {
		return e
	}
	if _, pending := c.inProgress[s]; pending {
		return ErrAlreadyInProgress
	}
	c.inProgress[s] = c.generation
	return nil
}

// MarkNotInProgress will release the lock on a given request. This should be
// called after put.
func (c *AttestationCache) MarkNotInProgress(req *qrysmpb.AttestationDataRequest) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s, e := reqToKey(req)
	if e != nil {
		return e
	}
	delete(c.inProgress, s)
	return nil
}

// Put the response in the cache.
func (c *AttestationCache) Put(_ context.Context, req *qrysmpb.AttestationDataRequest, res *qrysmpb.AttestationData) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	key, err := reqToKey(req)
	if err != nil {
		return err
	}
	if generation, pending := c.inProgress[key]; pending && generation != c.generation {
		return ErrAttestationDataStale
	}
	data := &attestationReqResWrapper{
		req,
		res,
	}
	if err := c.cache.AddIfNotPresent(data); err != nil {
		return err
	}
	trim(c.cache, maxCacheSize)

	attestationCacheSize.Set(float64(len(c.cache.List())))
	return nil
}

// Clear evicts every cached response. It is called when the head changes:
// entries are keyed by slot only, so a response produced against the previous
// head would otherwise keep being served for the rest of the slot after a
// block for that slot has been imported. Pending producers retain their slot
// reservation, but their writes are rejected so they cannot restore old data.
func (c *AttestationCache) Clear() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.generation++
	for _, item := range c.cache.List() {
		// Delete only fails when the key function does, which cannot happen
		// for items that were accepted by AddIfNotPresent.
		_ = c.cache.Delete(item)
	}
	attestationCacheSize.Set(float64(len(c.cache.List())))
}

func wrapperToKey(i any) (string, error) {
	w, ok := i.(*attestationReqResWrapper)
	if !ok {
		return "", errors.New("key is not of type *attestationReqResWrapper")
	}
	if w == nil {
		return "", errors.New("nil wrapper")
	}
	if w.req == nil {
		return "", errors.New("nil wrapper.request")
	}
	return reqToKey(w.req)
}

func reqToKey(req *qrysmpb.AttestationDataRequest) (string, error) {
	if req == nil {
		return "", errors.New("nil attestation data request")
	}
	return fmt.Sprintf("%d", req.Slot), nil
}

type attestationReqResWrapper struct {
	req *qrysmpb.AttestationDataRequest
	res *qrysmpb.AttestationData
}
