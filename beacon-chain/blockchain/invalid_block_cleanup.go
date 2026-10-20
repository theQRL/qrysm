package blockchain

import (
	"context"
	"errors"
	"fmt"

	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
)

// checkInvalidBlock rejects roots quarantined after execution invalidation,
// including imports that were already in flight when their root was removed.
func (s *Service) checkInvalidBlock(roots ...[32]byte) error {
	s.initSyncBlocksLock.RLock()
	defer s.initSyncBlocksLock.RUnlock()
	for _, root := range roots {
		if _, pending := s.pendingInvalidBlocks[root]; pending {
			return invalidBlock{error: ErrInvalidPayload, root: root, invalidAncestorRoots: [][32]byte{root}}
		}
	}
	return nil
}

// retryInvalidBlockCleanup finishes pending invalidations without making the
// roots available to ordinary block lookups. The caller holds the forkchoice
// write lock. A storage failure must not discard the remaining cleanup work.
func (s *Service) retryInvalidBlockCleanup(ctx context.Context) error {
	s.initSyncBlocksLock.RLock()
	roots := make([][32]byte, 0, len(s.pendingInvalidBlocks))
	for root := range s.pendingInvalidBlocks {
		roots = append(roots, root)
	}
	s.initSyncBlocksLock.RUnlock()
	if len(roots) == 0 {
		return nil
	}
	// Quarantine is already visible to readers. Let any older cache write
	// finish before deletion, and prevent later writes from taking a stale
	// snapshot while cleanup is in progress.
	s.initSyncBlocksSaveLock.Lock()
	defer s.initSyncBlocksSaveLock.Unlock()

	// Preserve the whole removed head prefix before deleting any of it. A
	// failed ancestry read leaves both the quarantine and cached copies intact.
	if err := s.preserveInvalidatedHead(ctx, roots); err != nil {
		return fmt.Errorf("could not preserve invalidated head ancestry: %w", err)
	}
	var cleanupErr error
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return errors.Join(cleanupErr, err)
		}
		if err := s.cfg.StateGen.DeleteStateFromCaches(ctx, root); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
			continue
		}
		// DeleteBlock also removes the state and state summary.
		if err := s.cfg.BeaconDB.DeleteBlock(ctx, root); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
			continue
		}
		// An import may have obtained its pre-state and SYNCING response
		// before this invalidation. Keep it quarantined through its insertion
		// check even when the database deletion has already succeeded.
		if s.blockBeingSynced != nil && s.BlockBeingSynced(root) {
			continue
		}
		s.initSyncBlocksLock.Lock()
		delete(s.pendingInvalidBlocks, root)
		s.initSyncBlocksLock.Unlock()
	}
	return cleanupErr
}

// invalidBlockForRecovery is the only lookup allowed to read quarantined
// blocks. It is used under the forkchoice lock solely to recover operations.
func (s *Service) invalidBlockForRecovery(ctx context.Context, root [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	s.initSyncBlocksLock.RLock()
	b := s.pendingInvalidBlocks[root]
	s.initSyncBlocksLock.RUnlock()
	if b == nil {
		var err error
		b, err = s.cfg.BeaconDB.Block(ctx, root)
		if err != nil {
			return nil, err
		}
	}
	if err := blocks.BeaconBlockIsNil(b); err != nil {
		return nil, errBlockNotFoundInCacheOrDB
	}
	return b, nil
}
