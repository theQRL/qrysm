package blockchain

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
)

// preserveInvalidatedHead retains the removed portion of the published head's
// branch for reorg recovery. These blocks must stay out of the regular caches:
// they are invalid and must never be served or flushed back to the database.
// The caller must hold the forkchoice write lock.
func (s *Service) preserveInvalidatedHead(ctx context.Context, invalidRoots [][32]byte) error {
	s.headLock.RLock()
	if s.head == nil {
		s.headLock.RUnlock()
		return nil
	}
	headRoot, headBlock := s.head.root, s.head.block
	s.headLock.RUnlock()

	invalid := make(map[[32]byte]bool, len(invalidRoots))
	for _, root := range invalidRoots {
		invalid[root] = true
	}
	for root := headRoot; ; {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, retained := s.invalidatedHeadBlocks[root]
		if !retained {
			if !invalid[root] {
				return nil
			}
			if root == headRoot {
				b = headBlock
			} else {
				var err error
				b, err = s.invalidBlockForRecovery(ctx, root)
				if err != nil {
					return err
				}
			}
			if err := blocks.BeaconBlockIsNil(b); err != nil {
				return err
			}
			if s.invalidatedHeadBlocks == nil {
				s.invalidatedHeadBlocks = make(map[[32]byte]interfaces.ReadOnlySignedBeaconBlock)
			}
			s.invalidatedHeadBlocks[root] = b
		}
		// An earlier invalidation may already have removed the head prefix.
		// Continue through it to retain ancestors rejected by a later response.
		root = b.Block().ParentRoot()
	}
}

// commonAncestorForReorg bridges any removed old-head nodes to the surviving
// tree before asking forkchoice for the shared ancestor of the two branches.
// The caller must hold the forkchoice lock.
func (s *Service) commonAncestorForReorg(ctx context.Context, oldRoot, newRoot [32]byte) ([32]byte, primitives.Slot, error) {
	for {
		if err := ctx.Err(); err != nil {
			return [32]byte{}, 0, err
		}
		b, ok := s.invalidatedHeadBlocks[oldRoot]
		if !ok {
			root, slot, err := s.cfg.ForkChoiceStore.CommonAncestor(ctx, oldRoot, newRoot)
			if errors.Is(err, forkchoice.ErrUnknownCommonAncestor) {
				// Finalization prunes a competing branch from fork choice
				// before the head moves off it, but its blocks are still
				// stored, so the fork point can be found from the blocks.
				return s.commonAncestorFromBlocks(ctx, oldRoot, newRoot)
			}
			return root, slot, err
		}
		oldRoot = b.Block().ParentRoot()
	}
}

// commonAncestorFromBlocks finds the fork point of two branches by walking
// their stored blocks towards genesis, one step at a time from whichever side
// is higher. It returns ErrUnknownCommonAncestor when a block is missing or
// the branches never meet. Read failures are returned so recovery can retry.
func (s *Service) commonAncestorFromBlocks(ctx context.Context, oldRoot, newRoot [32]byte) ([32]byte, primitives.Slot, error) {
	readBlock := func(root [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
		b, err := s.getBlock(ctx, root)
		if errors.Is(err, errBlockNotFoundInCacheOrDB) {
			return nil, forkchoice.ErrUnknownCommonAncestor
		}
		return b, err
	}
	oldBlock, err := readBlock(oldRoot)
	if err != nil {
		return [32]byte{}, 0, err
	}
	newBlock, err := readBlock(newRoot)
	if err != nil {
		return [32]byte{}, 0, err
	}
	for oldRoot != newRoot {
		if err := ctx.Err(); err != nil {
			return [32]byte{}, 0, err
		}
		if oldBlock.Block().Slot() == 0 && newBlock.Block().Slot() == 0 {
			return [32]byte{}, 0, forkchoice.ErrUnknownCommonAncestor
		}
		if oldBlock.Block().Slot() >= newBlock.Block().Slot() {
			oldRoot = oldBlock.Block().ParentRoot()
			if oldBlock, err = readBlock(oldRoot); err != nil {
				return [32]byte{}, 0, err
			}
		} else {
			newRoot = newBlock.Block().ParentRoot()
			if newBlock, err = readBlock(newRoot); err != nil {
				return [32]byte{}, 0, err
			}
		}
	}
	return oldRoot, oldBlock.Block().Slot(), nil
}
