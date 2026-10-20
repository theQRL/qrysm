package blockchain

import (
	"context"
	"fmt"
	"slices"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/db/kv"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	payloadattribute "github.com/theQRL/qrysm/consensus-types/payload-attribute"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
)

var defaultLatestValidHash = bytesutil.PadTo([]byte{0xff}, 32)

// notifyForkchoiceUpdateArg is the argument for the forkchoice update notification `notifyForkchoiceUpdate`.
type notifyForkchoiceUpdateArg struct {
	headState state.BeaconState
	headRoot  [32]byte
	headBlock interfaces.ReadOnlyBeaconBlock
}

// notifyForkchoiceUpdate signals execution engine the fork choice updates. Execution engine should:
// 1. Re-organizes the execution payload chain and corresponding state to make head_block_hash the head.
// 2. Applies finality to the execution state: it irreversibly persists the chain of all execution payloads and corresponding state, up to and including finalized_block_hash.
// The caller must hold the forkchoice write lock.
func (s *Service) notifyForkchoiceUpdate(ctx context.Context, arg *notifyForkchoiceUpdateArg) (*enginev1.PayloadIDBytes, error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.notifyForkchoiceUpdate")
	defer span.End()

	headBlk := arg.headBlock
	if headBlk == nil || headBlk.IsNil() || headBlk.Body().IsNil() {
		log.Error("Head block is nil")
		return nil, nil
	}
	isExecutionBlk, err := blocks.IsExecutionBlock(headBlk.Body())
	if err != nil {
		log.WithError(err).Error("Could not determine if head block is execution block")
		return nil, nil
	}
	if !isExecutionBlk {
		return nil, nil
	}
	headPayload, err := headBlk.Body().Execution()
	if err != nil {
		log.WithError(err).Error("Could not get execution payload for head block")
		return nil, nil
	}
	update := s.executionForkchoiceState(arg.headRoot)
	finalizedHash := update.finalizedHash
	justifiedHash := update.safeHash
	fcs := &enginev1.ForkchoiceState{
		HeadBlockHash:      headPayload.BlockHash(),
		SafeBlockHash:      justifiedHash[:],
		FinalizedBlockHash: finalizedHash[:],
	}
	// Guard against sending an FCU with a zero/empty HeadBlockHash at genesis.
	// When proposing the first non-genesis block, the head payload's BlockHash
	// can resolve to all-zeros; the EL rejects that and the resulting error
	// can cascade into forkchoice marking genesis invalid. Fall back to the
	// hash recorded in the genesis state's LatestExecutionPayloadHeader.
	if len(fcs.HeadBlockHash) != 32 || [32]byte(fcs.HeadBlockHash) == [32]byte{} {
		hash, err := s.hashForGenesisBlock(ctx, arg.headRoot)
		if errors.Is(err, errNotGenesisRoot) {
			log.Error("Sending nil head block hash to execution engine")
			return nil, nil
		}
		if err != nil {
			return nil, errors.Wrap(err, "could not get head block hash")
		}
		fcs.HeadBlockHash = hash
	}

	nextSlot := s.CurrentSlot() + 1 // Cache payload ID for next slot proposer.
	hasAttr, attr, proposerId := s.getPayloadAttribute(ctx, arg.headState, nextSlot, arg.headRoot[:])

	// A failed RPC may still have reached execution. Invalidate the previous
	// acknowledgement so even a return to those hashes gets notified again.
	s.lastForkchoiceUpdate = nil
	payloadID, lastValidHash, err := s.cfg.ExecutionEngineCaller.ForkchoiceUpdated(ctx, fcs, attr)
	if err != nil {
		switch err {
		case execution.ErrAcceptedSyncingPayloadStatus:
			// Retry even if the head and checkpoints stay unchanged: execution
			// may finish syncing and return VALID or INVALID on a later call.
			forkchoiceUpdatedOptimisticNodeCount.Inc()
			log.WithFields(logrus.Fields{
				"headSlot":                  headBlk.Slot(),
				"headPayloadBlockHash":      fmt.Sprintf("%#x", bytesutil.Trunc(headPayload.BlockHash())),
				"finalizedPayloadBlockHash": fmt.Sprintf("%#x", bytesutil.Trunc(finalizedHash[:])),
			}).Info("Called fork choice updated with optimistic block")
			return payloadID, nil
		case execution.ErrInvalidPayloadStatus:
			forkchoiceUpdatedInvalidNodeCount.Inc()
			headRoot := arg.headRoot
			if len(lastValidHash) == 0 {
				lastValidHash = defaultLatestValidHash
			}
			// An INVALID verdict outlives the request that obtained it. Finish
			// removing the branch even if cancellation races the engine response.
			invalidRoots, err := s.cfg.ForkChoiceStore.SetOptimisticToInvalid(context.WithoutCancel(ctx), headRoot, headBlk.ParentRoot(), bytesutil.ToBytes32(lastValidHash))
			invalid := invalidBlock{
				error:                ErrInvalidPayload,
				root:                 headRoot,
				lastValidHash:        bytesutil.ToBytes32(lastValidHash),
				invalidAncestorRoots: invalidRoots,
			}
			// Recovery failures must still reject this head. A nil error lets
			// the caller publish the block the execution engine just invalidated.
			if err != nil {
				invalid.error = fmt.Errorf("%w: could not set head root to invalid: %w", ErrInvalidPayload, err)
				return nil, invalid
			}
			if err := s.removeInvalidBlockAndState(ctx, invalidRoots); err != nil {
				invalid.error = fmt.Errorf("%w: could not remove invalid block and state: %w", ErrInvalidPayload, err)
				return nil, invalid
			}

			r, err := s.cfg.ForkChoiceStore.Head(ctx)
			if err != nil {
				log.WithFields(logrus.Fields{
					"slot":                 headBlk.Slot(),
					"blockRoot":            fmt.Sprintf("%#x", bytesutil.Trunc(headRoot[:])),
					"invalidChildrenCount": len(invalidRoots),
				}).Warn("Pruned invalid blocks, could not update head root")
				invalid.error = fmt.Errorf("%w: could not select head after pruning invalid blocks: %w", ErrInvalidPayload, err)
				return nil, invalid
			}
			b, err := s.getBlock(ctx, r)
			if err != nil {
				invalid.error = fmt.Errorf("%w: could not get head block: %w", ErrInvalidPayload, err)
				return nil, invalid
			}
			st, err := s.cfg.StateGen.StateByRoot(ctx, r)
			if err != nil {
				invalid.error = fmt.Errorf("%w: could not get head state: %w", ErrInvalidPayload, err)
				return nil, invalid
			}
			pid, err := s.notifyForkchoiceUpdate(ctx, &notifyForkchoiceUpdateArg{
				headState: st,
				headRoot:  r,
				headBlock: b.Block(),
			})
			if err != nil {
				// Recovery must retain the original rejection even if a later
				// checkpoint read fails or another branch is also invalidated.
				invalid.error = fmt.Errorf("%w: could not notify replacement head: %w", ErrInvalidPayload, err)
				additionalRoots := append([][32]byte{InvalidBlockRoot(err)}, InvalidAncestorRoots(err)...)
				for _, root := range additionalRoots {
					if root != [32]byte{} && !slices.Contains(invalid.invalidAncestorRoots, root) {
						invalid.invalidAncestorRoots = append(invalid.invalidAncestorRoots, root)
					}
				}
				return nil, invalid
			}

			if err := s.saveHead(ctx, r, b, st); err != nil {
				invalid.error = fmt.Errorf("%w: could not save head after pruning invalid blocks: %w", ErrInvalidPayload, err)
				return nil, invalid
			}

			log.WithFields(logrus.Fields{
				"slot":                 headBlk.Slot(),
				"blockRoot":            fmt.Sprintf("%#x", bytesutil.Trunc(headRoot[:])),
				"invalidChildrenCount": len(invalidRoots),
				"newHeadRoot":          fmt.Sprintf("%#x", bytesutil.Trunc(r[:])),
			}).Warn("Pruned invalid blocks")
			return pid, invalid

		default:
			log.WithError(err).Error(ErrUndefinedExecutionEngineError)
			return nil, nil
		}
	}
	forkchoiceUpdatedValidNodeCount.Inc()
	// A completed execution verdict outlives the request that obtained it.
	// Retain VALID before cancellation can stop the remaining local work.
	if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(context.WithoutCancel(ctx), arg.headRoot); err != nil {
		return nil, errors.Wrap(err, "could not set head root to valid")
	}
	s.refreshHeadOptimisticStatus()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// A VALID response can validate already-finalized ancestors even when no
	// block or epoch transition advances finality.
	finalized, err := s.cfg.BeaconDB.FinalizedCheckpoint(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.updateLastValidatedCheckpoint(ctx, finalized); err != nil {
		return nil, err
	}
	// If the forkchoice update call has an attribute, update the proposer payload ID cache.
	if hasAttr && payloadID != nil {
		var pId [8]byte
		copy(pId[:], payloadID[:])
		s.cfg.ProposerSlotIndexCache.SetProposerAndPayloadIDs(nextSlot, proposerId, pId, arg.headRoot)
	} else if hasAttr && payloadID == nil && !features.Get().PrepareAllPayloads {
		log.WithFields(logrus.Fields{
			"blockHash": fmt.Sprintf("%#x", headPayload.BlockHash()),
			"slot":      headBlk.Slot(),
		}).Error("Received nil payload ID on VALID engine response")
	}
	s.lastForkchoiceUpdate = &update
	return payloadID, nil
}

// getPayloadHash returns the payload hash given the block root.
func (s *Service) getPayloadHash(ctx context.Context, root []byte) ([32]byte, error) {
	blk, err := s.getBlock(ctx, s.ensureRootNotZeros(bytesutil.ToBytes32(root)))
	if err != nil {
		return [32]byte{}, err
	}
	payload, err := blk.Block().Body().Execution()
	if err != nil {
		return [32]byte{}, errors.Wrap(err, "could not get execution payload")
	}
	return bytesutil.ToBytes32(payload.BlockHash()), nil
}

// notifyNewPayload signals execution engine on a new payload.
// It returns true if the EL has returned VALID for the block
func (s *Service) notifyNewPayload(ctx context.Context,
	preStateHeader interfaces.ExecutionData, blk interfaces.ReadOnlySignedBeaconBlock) (bool, error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.notifyNewPayload")
	defer span.End()

	if blk == nil {
		return false, errors.New("signed beacon block can't be nil")
	}
	if err := consensusblocks.BeaconBlockIsNil(blk); err != nil {
		return false, err
	}
	body := blk.Block().Body()
	enabled, err := blocks.IsExecutionEnabledUsingHeader(preStateHeader, body)
	if err != nil {
		return false, errors.Wrap(invalidBlock{error: err}, "could not determine if execution is enabled")
	}
	if !enabled {
		return true, nil
	}
	payload, err := body.Execution()
	if err != nil {
		return false, errors.Wrap(invalidBlock{error: err}, "could not get execution payload")
	}

	lastValidHash, err := s.cfg.ExecutionEngineCaller.NewPayload(ctx, payload, []common.Hash{}, &common.Hash{} /*empty version hashes and root before Deneb*/)
	switch err {
	case nil:
		newPayloadValidNodeCount.Inc()
		return true, nil
	case execution.ErrAcceptedSyncingPayloadStatus:
		newPayloadOptimisticNodeCount.Inc()
		log.WithFields(logrus.Fields{
			"slot":             blk.Block().Slot(),
			"payloadBlockHash": fmt.Sprintf("%#x", bytesutil.Trunc(payload.BlockHash())),
		}).Info("Called new payload with optimistic block")
		return false, nil
	case execution.ErrInvalidPayloadStatus:
		lvh := bytesutil.ToBytes32(lastValidHash)
		return false, invalidBlock{
			error:         ErrInvalidPayload,
			lastValidHash: lvh,
		}
	case execution.ErrInvalidBlockHashPayloadStatus:
		// A malformed hash rejects this block without blaming its ancestors.
		return false, ErrInvalidBlockHashPayloadStatus
	default:
		return false, errors.WithMessage(ErrUndefinedExecutionEngineError, err.Error())
	}
}

// reportInvalidBlock deals with the event that an invalid block was detected by the execution layer
// pruneInvalidBlock marks the block and its invalid descendants in forkchoice
// and removes them from the DB and state caches. The caller MUST hold the
// forkchoice write lock: the forkchoice store method is called directly rather
// than through a lock-acquiring wrapper, so that this works when invoked from
// paths that already hold the lock (e.g. onBlockBatch under ReceiveBlockBatch),
// which would otherwise self-deadlock on the non-reentrant lock.
func (s *Service) pruneInvalidBlock(ctx context.Context, root, parentRoot, lvh [32]byte) error {
	newPayloadInvalidNodeCount.Inc()
	// Complete the in-memory removal independently of the import's deadline.
	// Storage cleanup below keeps the request context and quarantines roots for
	// retry if it is canceled, so the INVALID verdict cannot be lost.
	invalidRoots, err := s.cfg.ForkChoiceStore.SetOptimisticToInvalid(context.WithoutCancel(ctx), root, parentRoot, lvh)
	invalid := invalidBlock{
		error:                ErrInvalidPayload,
		root:                 root,
		lastValidHash:        lvh,
		invalidAncestorRoots: invalidRoots,
	}
	if err != nil {
		invalid.error = fmt.Errorf("%w: could not invalidate block: %w", ErrInvalidPayload, err)
		return invalid
	}
	if err := s.removeInvalidBlockAndState(ctx, invalidRoots); err != nil {
		invalid.error = fmt.Errorf("%w: could not remove invalid block and state: %w", ErrInvalidPayload, err)
		return invalid
	}
	log.WithFields(logrus.Fields{
		"blockRoot":            fmt.Sprintf("%#x", root),
		"invalidChildrenCount": len(invalidRoots),
	}).Warn("Pruned invalid blocks")
	return invalid
}

// handleInvalidBatchExecutionError follows pending blocks back to the latest
// valid payload or an imported ancestor, then invalidates the affected roots.
// blks is the linear batch prefix ending at the rejected block. The caller must
// hold the forkchoice write lock.
func (s *Service) handleInvalidBatchExecutionError(ctx context.Context, payloadErr error, blks []consensusblocks.ROBlock) error {
	lvh := InvalidBlockLVH(payloadErr)
	if !IsInvalidBlock(payloadErr) || lvh == [32]byte{} {
		return payloadErr
	}
	newPayloadInvalidNodeCount.Inc()
	last := len(blks) - 1
	invalid := invalidBlock{error: payloadErr, root: blks[last].Root(), lastValidHash: lvh}
	for i := last; i >= 0; i-- {
		b := blks[i]
		payload, err := b.Block().Body().Execution()
		if err != nil {
			invalid.error = fmt.Errorf("%w: could not read batch payload: %v", payloadErr, err)
			return invalid
		}
		if i < last && bytesutil.ToBytes32(payload.BlockHash()) == lvh {
			break
		}
		known := s.cfg.ForkChoiceStore.HasNode(b.Root())
		if known || s.cfg.ForkChoiceStore.HasNode(b.Block().ParentRoot()) {
			// As in single-block imports, cancellation must not discard a
			// completed execution rejection before quarantine is established.
			roots, err := s.cfg.ForkChoiceStore.SetOptimisticToInvalid(context.WithoutCancel(ctx), b.Root(), b.Block().ParentRoot(), lvh)
			if err != nil {
				invalid.error = fmt.Errorf("%w: could not invalidate batch ancestors: %v", payloadErr, err)
				return invalid
			}
			if !known && len(roots) == 0 && bytesutil.ToBytes32(payload.ParentHash()) != lvh {
				// Match forkchoice's fallback when the engine's latest valid hash
				// is on a different branch: only the rejected block is known invalid.
				invalid.invalidAncestorRoots = [][32]byte{invalid.root}
				break
			}
			if !known {
				invalid.invalidAncestorRoots = append(invalid.invalidAncestorRoots, b.Root())
			}
			invalid.invalidAncestorRoots = append(invalid.invalidAncestorRoots, roots...)
			break
		}
		invalid.invalidAncestorRoots = append(invalid.invalidAncestorRoots, b.Root())
	}
	if err := s.removeInvalidBlockAndState(ctx, invalid.invalidAncestorRoots); err != nil {
		invalid.error = fmt.Errorf("%w: could not remove invalid batch blocks: %w", payloadErr, err)
		return invalid
	}
	log.WithFields(logrus.Fields{
		"blockRoot":            fmt.Sprintf("%#x", invalid.root),
		"invalidChildrenCount": len(invalid.invalidAncestorRoots),
	}).Warn("Pruned invalid batch blocks")
	return invalid
}

// getPayloadAttributes returns the payload attributes for the given state and slot.
// The attribute is required to initiate a payload build process in the context of an `engine_forkchoiceUpdated` call.
func (s *Service) getPayloadAttribute(ctx context.Context, st state.BeaconState, slot primitives.Slot, headRoot []byte) (bool, payloadattribute.Attributer, primitives.ValidatorIndex) {
	emptyAttri := payloadattribute.EmptyWithVersion(st.Version())
	// Root is `[32]byte{}` since we are retrieving proposer ID of a given slot. During insertion at assignment the root was not known.
	proposerID, _, ok := s.cfg.ProposerSlotIndexCache.GetProposerPayloadIDs(slot, [32]byte{} /* root */)
	if !ok && !features.Get().PrepareAllPayloads { // There's no need to build attribute if there is no proposer for slot.
		return false, emptyAttri, 0
	}

	// Get previous randao.
	st = st.Copy()
	if slot > st.Slot() {
		var err error
		st, err = transition.ProcessSlotsUsingNextSlotCache(ctx, st, headRoot, slot)
		if err != nil {
			log.WithError(err).Error("Could not process slots to get payload attribute")
			return false, emptyAttri, 0
		}
	}
	prevRando, err := helpers.RandaoMix(st, time.CurrentEpoch(st))
	if err != nil {
		log.WithError(err).Error("Could not get randao mix to get payload attribute")
		return false, emptyAttri, 0
	}

	// Get fee recipient.
	feeRecipient := params.BeaconConfig().DefaultFeeRecipient
	recipient, err := s.cfg.BeaconDB.FeeRecipientByValidatorID(ctx, proposerID)
	switch {
	case errors.Is(err, kv.ErrNotFoundFeeRecipient):
		if feeRecipient.String() == params.BeaconConfig().QRLBurnAddress {
			logrus.WithFields(logrus.Fields{
				"validatorIndex": proposerID,
				"burnAddress":    params.BeaconConfig().QRLBurnAddress,
			}).Warn("Fee recipient is currently using the burn address, " +
				"you will not be rewarded transaction fees on this setting. " +
				"Please set a different qrl address as the fee recipient. " +
				"Please refer to our documentation for instructions")
		}
	case err != nil:
		log.WithError(err).Error("Could not get fee recipient to get payload attribute")
		return false, emptyAttri, 0
	default:
		feeRecipient = recipient
	}

	// Get timestamp.
	t, err := slots.ToTime(uint64(s.genesisTime.Unix()), slot)
	if err != nil {
		log.WithError(err).Error("Could not get timestamp to get payload attribute")
		return false, emptyAttri, 0
	}

	var attr payloadattribute.Attributer
	switch st.Version() {
	case version.Zond:
		withdrawals, err := st.ExpectedWithdrawals()
		if err != nil {
			log.WithError(err).Error("Could not get expected withdrawals to get payload attribute")
			return false, emptyAttri, 0
		}
		attr, err = payloadattribute.New(&enginev1.PayloadAttributesV2{
			Timestamp:             uint64(t.Unix()),
			PrevRandao:            prevRando,
			SuggestedFeeRecipient: feeRecipient.Bytes(),
			Withdrawals:           withdrawals,
		})
		if err != nil {
			log.WithError(err).Error("Could not get payload attribute")
			return false, emptyAttri, 0
		}
	default:
		log.WithField("version", st.Version()).Error("Could not get payload attribute due to unknown state version")
		return false, emptyAttri, 0
	}

	return true, attr, proposerID
}

// removeInvalidBlockAndState removes the invalid block, blob and its corresponding state from the cache and DB.
// The caller must hold the forkchoice write lock.
func (s *Service) removeInvalidBlockAndState(ctx context.Context, blkRoots [][32]byte) error {
	// Quarantine every root before fallible recovery or cleanup. Retain cached
	// copies privately so a failed ancestry read can retry after cache eviction.
	s.initSyncBlocksLock.Lock()
	if s.pendingInvalidBlocks == nil {
		s.pendingInvalidBlocks = make(map[[32]byte]interfaces.ReadOnlySignedBeaconBlock)
	}
	for _, root := range blkRoots {
		if _, pending := s.pendingInvalidBlocks[root]; !pending {
			s.pendingInvalidBlocks[root] = s.initSyncBlocks[root]
		}
		delete(s.initSyncBlocks, root)
	}
	s.initSyncBlocksLock.Unlock()
	return s.retryInvalidBlockCleanup(ctx)
}
