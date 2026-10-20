package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/time/slots"
)

func (s *Service) setupForkchoice(st state.BeaconState) error {
	if err := s.setupForkchoiceCheckpoints(); err != nil {
		return errors.Wrap(err, "could not set up forkchoice checkpoints")
	}
	if err := s.setupForkchoiceTree(st); err != nil {
		return errors.Wrap(err, "could not set up forkchoice tree")
	}
	if err := s.initializeHead(s.ctx, st); err != nil {
		return errors.Wrap(err, "could not initialize head from db")
	}
	return nil
}

func (s *Service) setupForkchoiceCheckpoints() error {
	justified, err := s.cfg.BeaconDB.JustifiedCheckpoint(s.ctx)
	if err != nil {
		return errors.Wrap(err, "could not get justified checkpoint")
	}
	if justified == nil {
		return errNilJustifiedCheckpoint
	}
	finalized, err := s.cfg.BeaconDB.FinalizedCheckpoint(s.ctx)
	if err != nil {
		return errors.Wrap(err, "could not get finalized checkpoint")
	}
	if finalized == nil {
		return errNilFinalizedCheckpoint
	}

	fRoot := s.ensureRootNotZeros(bytesutil.ToBytes32(finalized.Root))
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	if err := s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(s.ctx, &forkchoicetypes.Checkpoint{Epoch: justified.Epoch,
		Root: bytesutil.ToBytes32(justified.Root)}); err != nil {
		return errors.Wrap(err, "could not initialize justified checkpoint and balances")
	}
	if err := s.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: finalized.Epoch,
		Root: fRoot}); err != nil {
		log.WithError(err).Error("Could not update forkchoice's finalized checkpoint")
	}
	s.cfg.ForkChoiceStore.SetGenesisTime(uint64(s.genesisTime.Unix()))

	return nil
}

func (s *Service) startupHeadRoot() [32]byte {
	headStr := features.Get().ForceHead
	jp := s.CurrentJustifiedCheckpt()
	jRoot := s.ensureRootNotZeros([32]byte(jp.Root))
	if headStr == "" {
		return jRoot
	}
	if headStr == "head" {
		root, err := s.cfg.BeaconDB.HeadBlockRoot()
		if err != nil {
			log.WithError(err).Error("Could not get head block root, starting with justified block as head")
			return jRoot
		}
		log.Infof("Using Head root of %#x", root)
		return root
	}
	root, err := bytesutil.DecodeHexWithLength(headStr, 32)
	if err != nil {
		log.WithError(err).Error("Could not parse head root, starting with justified block as head")
		return jRoot
	}
	return [32]byte(root)
}

func (s *Service) setupForkchoiceRoot(st state.BeaconState) error {
	cp := s.FinalizedCheckpt()
	fRoot := s.ensureRootNotZeros([32]byte(cp.Root))
	finalizedBlock, err := s.cfg.BeaconDB.Block(s.ctx, fRoot)
	if err != nil {
		return errors.Wrap(err, "could not get finalized checkpoint block")
	}
	roblock, err := blocks.NewROBlockWithRoot(finalizedBlock, fRoot)
	if err != nil {
		return err
	}
	if err := s.cfg.ForkChoiceStore.InsertNode(s.ctx, st, roblock); err != nil {
		return errors.Wrap(err, "could not insert finalized block to forkchoice")
	}
	// Re-seed the cached finalized payload hash now that the finalized node is
	// in the tree. setupForkchoiceCheckpoints already ran UpdateFinalizedCheckpoint,
	// but that was before this insert, when the node was absent from nodeByRoot, so
	// the cache is still zero. This insert does not advance finalization or trigger
	// pruning. Without this, every forkchoice update sends a zero finalized hash
	// to the execution layer until pruning next refreshes the cache.
	if err := s.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: cp.Epoch, Root: fRoot}); err != nil {
		return errors.Wrap(err, "could not re-seed finalized checkpoint after forkchoice insert")
	}
	if !features.Get().EnableStartOptimistic {
		lastValidatedCheckpoint, err := s.cfg.BeaconDB.LastValidatedCheckpoint(s.ctx)
		if err != nil {
			return errors.Wrap(err, "could not get last validated checkpoint")
		}
		if bytes.Equal(fRoot[:], lastValidatedCheckpoint.Root) {
			if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(s.ctx, fRoot); err != nil {
				return errors.Wrap(err, "could not set finalized block as validated")
			}
		}
	}
	return nil
}

func (s *Service) setupForkchoiceTree(st state.BeaconState) error {
	headRoot := s.startupHeadRoot()
	cp := s.FinalizedCheckpt()
	fRoot := s.ensureRootNotZeros([32]byte(cp.Root))
	if err := s.setupForkchoiceRoot(st); err != nil {
		return errors.Wrap(err, "could not set up forkchoice root")
	}
	if headRoot == fRoot {
		return s.setupForkchoiceFinalizedHead()
	}
	blk, err := s.cfg.BeaconDB.Block(s.ctx, headRoot)
	if err != nil {
		log.WithError(err).Error("Could not get head block, starting with finalized block as head")
		return s.setupForkchoiceFinalizedHead()
	}
	if err := blocks.BeaconBlockIsNil(blk); err != nil {
		log.WithError(err).WithField("headRoot", fmt.Sprintf("%#x", headRoot)).Error("Head block is nil, starting with finalized block as head")
		return s.setupForkchoiceFinalizedHead()
	}
	if slots.ToEpoch(blk.Block().Slot()) < cp.Epoch {
		log.WithField("headRoot", fmt.Sprintf("%#x", headRoot)).Error("Head block is older than finalized block, starting with finalized block as head")
		return s.setupForkchoiceFinalizedHead()
	}
	chain, err := s.buildForkchoiceChain(s.ctx, blk)
	if err != nil {
		log.WithError(err).Error("Could not build forkchoice chain, starting with finalized block as head")
		return s.setupForkchoiceFinalizedHead()
	}

	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	return s.restoreForkchoiceChain(s.ctx, chain)
}

// restoreForkchoiceChain restores previously validated database blocks, their
// votes, and slashing exclusions. The caller must hold the forkchoice write lock.
func (s *Service) restoreForkchoiceChain(ctx context.Context, chain []*forkchoicetypes.BlockAndCheckpoints) error {
	// InsertChain can retain a prefix on failure. Restore exclusions first so
	// retries that skip those now-known blocks cannot lose their slashings.
	for _, b := range chain {
		s.InsertSlashingsToForkChoiceStore(ctx, b.Block.Block().Body().AttesterSlashings())
	}
	restoreErr := s.cfg.ForkChoiceStore.InsertChain(ctx, chain)
	if restoreErr != nil {
		ctx = context.WithoutCancel(ctx)
	}
	// Restored ancestors are skipped by later imports. Recover votes from
	// every retained block, including a prefix left by failed insertion.
	// The regular included-vote handler authenticates them against their
	// target states and queues votes whose dependencies are still unavailable.
	for _, b := range chain {
		if !s.cfg.ForkChoiceStore.HasNode(b.Block.Root()) {
			continue
		}
		if err := s.handleBlockAttestations(ctx, b.Block.Block()); err != nil {
			if restoreErr == nil {
				restoreErr = errors.Wrap(err, "could not handle restored block attestations")
			} else {
				restoreErr = fmt.Errorf("%w: could not handle restored block attestations: %w", restoreErr, err)
			}
		}
	}
	return restoreErr
}

func (s *Service) setupForkchoiceFinalizedHead() error {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()

	jc := s.cfg.ForkChoiceStore.JustifiedCheckpoint()
	fc := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	// Only the finalized anchor was restored. A missing justified checkpoint
	// would make Head fail, so replace it and its balances together. Preserve a
	// newer justified epoch at the same root when checkpoint slots were skipped.
	if jc.Epoch < fc.Epoch || !s.cfg.ForkChoiceStore.HasNode(s.ensureRootNotZeros(jc.Root)) {
		if err := s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(s.ctx, &forkchoicetypes.Checkpoint{Epoch: fc.Epoch, Root: fc.Root}); err != nil {
			return errors.Wrap(err, "could not reset justified checkpoint to finalized checkpoint")
		}
	}
	return nil
}

func (s *Service) buildForkchoiceChain(ctx context.Context, head interfaces.ReadOnlySignedBeaconBlock) ([]*forkchoicetypes.BlockAndCheckpoints, error) {
	chain := []*forkchoicetypes.BlockAndCheckpoints{}
	cp := s.FinalizedCheckpt()
	fRoot := s.ensureRootNotZeros([32]byte(cp.Root))
	jp := s.CurrentJustifiedCheckpt()
	root, err := head.Block().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not get head block root")
	}
	for {
		roblock, err := blocks.NewROBlockWithRoot(head, root)
		if err != nil {
			return nil, err
		}
		// This chain sets the justified checkpoint for every block, including some that are older than jp.
		// This should be however safe for forkchoice at startup. An alternative would be to hook during the
		// block processing pipeline when setting the head state, to compute the right states for the justified
		// checkpoint.
		chain = append(chain, &forkchoicetypes.BlockAndCheckpoints{Block: roblock, JustifiedCheckpoint: jp, FinalizedCheckpoint: cp})
		root = head.Block().ParentRoot()
		if root == fRoot {
			break
		}
		head, err = s.cfg.BeaconDB.Block(s.ctx, root)
		if err != nil {
			return nil, errors.Wrap(err, "could not get block")
		}
		if err := blocks.BeaconBlockIsNil(head); err != nil {
			return nil, errors.Wrapf(err, "missing block %#x between head and finalized", root)
		}
		if slots.ToEpoch(head.Block().Slot()) < cp.Epoch {
			return nil, errors.New("head block is not a descendant of the finalized checkpoint")
		}
	}
	slices.Reverse(chain)
	return chain, nil
}
