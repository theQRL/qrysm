package blockchain

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
)

// This saves a beacon block to the initial sync blocks cache. It rate limits how many blocks
// the cache keeps in memory (2 epochs worth of blocks) and saves them to DB when it hits this limit.
func (s *Service) saveInitSyncBlock(ctx context.Context, r [32]byte, b interfaces.ReadOnlySignedBeaconBlock) error {
	s.initSyncBlocksLock.Lock()
	if _, pending := s.pendingInvalidBlocks[r]; pending {
		s.initSyncBlocksLock.Unlock()
		return invalidBlock{error: ErrInvalidPayload, root: r}
	}
	s.initSyncBlocks[r] = b
	numBlocks := len(s.initSyncBlocks)
	s.initSyncBlocksLock.Unlock()
	if uint64(numBlocks) > initialSyncBlockCacheSize {
		return s.saveInitSyncBlocks(ctx, true)
	}
	return nil
}

// This checks if a beacon block exists in the initial sync blocks cache using the root
// of the block.
func (s *Service) hasInitSyncBlock(r [32]byte) bool {
	s.initSyncBlocksLock.RLock()
	defer s.initSyncBlocksLock.RUnlock()
	_, ok := s.initSyncBlocks[r]
	return ok
}

// Returns true if a block for root `r` exists in the initial sync blocks cache or the DB.
func (s *Service) hasBlockInInitSyncOrDB(ctx context.Context, r [32]byte) bool {
	s.initSyncBlocksLock.RLock()
	defer s.initSyncBlocksLock.RUnlock()
	if _, pending := s.pendingInvalidBlocks[r]; pending {
		return false
	}
	if _, ok := s.initSyncBlocks[r]; ok {
		return true
	}
	return s.cfg.BeaconDB.HasBlock(ctx, r)
}

// Returns block for a given root `r` from either the initial sync blocks cache or the DB.
// Error is returned if the block is not found in either cache or DB.
func (s *Service) getBlock(ctx context.Context, r [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	s.initSyncBlocksLock.RLock()
	defer s.initSyncBlocksLock.RUnlock()
	if _, pending := s.pendingInvalidBlocks[r]; pending {
		return nil, invalidBlock{error: ErrInvalidPayload, root: r}
	}

	// Check cache first because it's faster.
	b, ok := s.initSyncBlocks[r]
	var err error
	if !ok {
		b, err = s.cfg.BeaconDB.Block(ctx, r)
		if err != nil {
			return nil, errors.Wrap(err, "could not retrieve block from db")
		}
	}
	if err := blocks.BeaconBlockIsNil(b); err != nil {
		return nil, errBlockNotFoundInCacheOrDB
	}
	return b, nil
}

// saveInitSyncBlocks writes a cache snapshot while excluding invalid-block
// deletion. The save lock covers snapshot creation through persistence, so an
// older snapshot cannot restore a block after cleanup has deleted it.
func (s *Service) saveInitSyncBlocks(ctx context.Context, clearCache bool) error {
	s.initSyncBlocksSaveLock.Lock()
	defer s.initSyncBlocksSaveLock.Unlock()

	s.initSyncBlocksLock.RLock()
	blks := make([]interfaces.ReadOnlySignedBeaconBlock, 0, len(s.initSyncBlocks))
	roots := make([][32]byte, 0, len(s.initSyncBlocks))
	for root, b := range s.initSyncBlocks {
		blks = append(blks, b)
		roots = append(roots, root)
	}
	s.initSyncBlocksLock.RUnlock()

	if len(blks) == 0 {
		return nil
	}
	if err := s.cfg.BeaconDB.SaveBlocks(ctx, blks); err != nil {
		return err
	}
	if clearCache {
		// Imports may add blocks during the write. Evict only the entries
		// that were saved, and retain the whole snapshot on a failed write.
		s.initSyncBlocksLock.Lock()
		for _, root := range roots {
			delete(s.initSyncBlocks, root)
		}
		s.initSyncBlocksLock.Unlock()
	}
	return nil
}
