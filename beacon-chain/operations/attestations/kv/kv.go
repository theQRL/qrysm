// Package kv includes a key-value store implementation
// of an attestation cache used to satisfy important use-cases
// such as aggregation in a beacon node runtime.
package kv

import (
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/crypto/hash"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

var hashFn = hash.HashProto

// AttCaches defines the caches used to satisfy attestation pool interface.
// These caches are KV store for various attestations
// such are unaggregated, aggregated or attestations within a block.
type AttCaches struct {
	aggregatedAttLock  sync.RWMutex
	aggregatedAtt      map[[32]byte][]*zondpb.Attestation
	unAggregateAttLock sync.RWMutex
	unAggregatedAtt    map[[32]byte]*zondpb.Attestation
	forkchoiceAttLock  sync.RWMutex
	forkchoiceAtt      map[[32]byte]*zondpb.Attestation
	blockAttLock       sync.RWMutex
	blockAtt           map[[32]byte][]*zondpb.Attestation
	seenAtt            *cache.Cache
}

// NewAttCaches initializes a new attestation pool consists of multiple KV store in cache for
// various kind of attestations.
func NewAttCaches() *AttCaches {
	secsInEpoch := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	c := cache.New(secsInEpoch*time.Second, 2*secsInEpoch*time.Second)
	pool := &AttCaches{
		unAggregatedAtt: make(map[[32]byte]*zondpb.Attestation),
		aggregatedAtt:   make(map[[32]byte][]*zondpb.Attestation),
		forkchoiceAtt:   make(map[[32]byte]*zondpb.Attestation),
		blockAtt:        make(map[[32]byte][]*zondpb.Attestation),
		seenAtt:         c,
	}

	return pool
}
