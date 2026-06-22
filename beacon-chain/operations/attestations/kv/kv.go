// Package kv includes a key-value store implementation
// of an attestation cache used to satisfy important use-cases
// such as aggregation in a beacon node runtime.
package kv

import (
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/crypto/hash"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

var hashFn = hash.Proto

// AttCaches defines the caches used to satisfy attestation pool interface.
// These caches are KV store for various attestations
// such are unaggregated, aggregated or attestations within a block.
type AttCaches struct {
	aggregatedAttLock  sync.RWMutex
	aggregatedAtt      map[[32]byte][]*qrysmpb.Attestation
	unAggregateAttLock sync.RWMutex
	unAggregatedAtt    map[[32]byte]*qrysmpb.Attestation
	forkchoiceAttLock  sync.RWMutex
	forkchoiceAtt      map[[32]byte]*qrysmpb.Attestation
	blockAttLock       sync.RWMutex
	blockAtt           map[[32]byte][]*qrysmpb.Attestation
	seenAtt            *cache.Cache
	seenAggregatedAtt  *cache.Cache
}

// NewAttCaches initializes a new attestation pool consists of multiple KV store in cache for
// various kind of attestations.
func NewAttCaches() *AttCaches {
	// Post EIP-7045, attestations from the previous epoch can still be
	// included in the current epoch's blocks, so dedup history is kept
	// for two epochs (matching the prune window in pruneExpiredAtts).
	secsInEpoch := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	c := cache.New(2*secsInEpoch*time.Second, 3*secsInEpoch*time.Second)
	pool := &AttCaches{
		unAggregatedAtt: make(map[[32]byte]*qrysmpb.Attestation),
		aggregatedAtt:   make(map[[32]byte][]*qrysmpb.Attestation),
		forkchoiceAtt:   make(map[[32]byte]*qrysmpb.Attestation),
		blockAtt:        make(map[[32]byte][]*qrysmpb.Attestation),
		seenAtt:         c,
		seenAggregatedAtt: cache.New(
			2*secsInEpoch*time.Second,
			3*secsInEpoch*time.Second,
		),
	}

	return pool
}
