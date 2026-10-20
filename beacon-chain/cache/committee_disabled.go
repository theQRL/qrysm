//go:build fuzz

// This file is used in fuzzer builds to bypass global committee caches.
package cache

import (
	"context"

	"github.com/theQRL/qrysm/consensus-types/primitives"
)

// FakeCommitteeCache bypasses lookups by seed and active-validator membership.
type FakeCommitteeCache struct {
}

// NewCommitteesCache creates a new committee cache for storing/accessing shuffled indices of a committee.
func NewCommitteesCache() *FakeCommitteeCache {
	return &FakeCommitteeCache{}
}

// Committee fetches the shuffled indices by slot and committee index. Every list of indices
// represent one committee. Returns true if the list exists with slot and committee index. Otherwise returns false, nil.
func (c *FakeCommitteeCache) Committee(ctx context.Context, slot primitives.Slot, cacheKey CommitteeKey, index primitives.CommitteeIndex) ([]primitives.ValidatorIndex, error) {
	return nil, nil
}

// AddCommitteeShuffledList adds Committee shuffled list object to the cache. T
// his method also trims the least recently list if the cache size has ready the max cache size limit.
func (c *FakeCommitteeCache) AddCommitteeShuffledList(ctx context.Context, committees *Committees) error {
	return nil
}

// AddProposerIndicesList updates the committee shuffled list with proposer indices.
func (c *FakeCommitteeCache) AddProposerIndicesList(seed [32]byte, indices []primitives.ValidatorIndex) error {
	return nil
}

// ActiveIndices is a stub.
func (c *FakeCommitteeCache) ActiveIndices(ctx context.Context, cacheKey CommitteeKey) ([]primitives.ValidatorIndex, error) {
	return nil, nil
}

// ActiveIndicesCount is a stub.
func (c *FakeCommitteeCache) ActiveIndicesCount(ctx context.Context, cacheKey CommitteeKey) (int, error) {
	return 0, nil
}

// ActiveBalance returns the active balance of a given seed stored in cache.
func (c *FakeCommitteeCache) ActiveBalance(seed [32]byte) (uint64, error) {
	return 0, nil
}

// ProposerIndices returns the proposer indices of a given seed.
func (c *FakeCommitteeCache) ProposerIndices(seed [32]byte) ([]primitives.ValidatorIndex, error) {
	return nil, nil
}

// HasEntry returns true if the committee cache has a value.
func (c *FakeCommitteeCache) HasEntry(CommitteeKey) bool {
	return false
}

// MarkInProgress is a stub.
func (c *FakeCommitteeCache) MarkInProgress(cacheKey CommitteeKey) error {
	return nil
}

// MarkNotInProgress is a stub.
func (c *FakeCommitteeCache) MarkNotInProgress(cacheKey CommitteeKey) error {
	return nil
}

// Clear is a stub.
func (c *FakeCommitteeCache) Clear() {
	return
}
