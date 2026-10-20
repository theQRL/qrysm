package blockchain

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	coreTime "github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/monitoring/tracing"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
	"golang.org/x/sync/errgroup"
)

// This defines how many epochs since finality the run time will begin to save hot state on to the DB.
var epochsSinceFinalitySaveHotStateDB = primitives.Epoch(100)

// BlockReceiver interface defines the methods of chain service for receiving and processing new blocks.
type BlockReceiver interface {
	ReceiveBlock(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte) error
	ReceiveBlockBatch(ctx context.Context, blocks []blocks.ROBlock) error
	HasBlock(ctx context.Context, root [32]byte) bool
	RecentBlockSlot(root [32]byte) (primitives.Slot, error)
	BlockBeingSynced([32]byte) bool
}

// SlashingReceiver interface defines the methods of chain service for receiving validated slashing over the wire.
type SlashingReceiver interface {
	ReceiveAttesterSlashing(ctx context.Context, slashings *qrysmpb.AttesterSlashing)
}

// ReceiveBlock is a function that defines the operations (minus pubsub)
// that are performed on a received block. The operations consist of:
//  1. Validate block, apply state transition and update checkpoints
//  2. Apply fork choice to the processed block
//  3. Save latest head info
func (s *Service) ReceiveBlock(ctx context.Context, block interfaces.ReadOnlySignedBeaconBlock, blockRoot [32]byte) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.ReceiveBlock")
	defer span.End()
	receivedTime := time.Now()
	if err := s.checkInvalidBlock(blockRoot); err != nil {
		return err
	}
	// Skip blocks already imported into forkchoice. Without this check a
	// gossip rebroadcast of an already-processed block would re-run prestate
	// fetch, block copy, and full state-transition validation.
	if s.InForkchoice(blockRoot) {
		log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot)).Debug("Ignoring block already in forkchoice")
		return nil
	}
	err := s.blockBeingSynced.set(blockRoot)
	if errors.Is(err, errBlockBeingSynced) {
		log.WithField("blockRoot", fmt.Sprintf("%#x", blockRoot)).Debug("Ignoring block currently being synced")
		return nil
	}
	defer s.blockBeingSynced.unset(blockRoot)

	blockCopy, err := block.Copy()
	if err != nil {
		return err
	}

	preState, err := s.getBlockPreState(ctx, blockCopy.Block())
	if err != nil {
		// A concurrent import may have finalized this block and pruned its
		// pre-state ancestry before this request could retrieve it.
		s.cfg.ForkChoiceStore.RLock()
		imported, lookupErr := s.isImportedBlock(ctx, blockRoot, blockCopy.Block().Slot())
		s.cfg.ForkChoiceStore.RUnlock()
		if lookupErr != nil {
			return lookupErr
		}
		if imported {
			return nil
		}
		return errors.Wrap(err, "could not get block's prestate")
	}
	currentEpoch := coreTime.CurrentEpoch(preState)
	roblock, err := blocks.NewROBlockWithRoot(blockCopy, blockRoot)
	if err != nil {
		return errors.Wrap(err, "new ro block with root")
	}

	_, preStateHeader, err := getStateVersionAndPayload(preState)
	if err != nil {
		return err
	}
	var eg errgroup.Group
	var postState state.BeaconState
	eg.Go(func() error {
		var err error
		postState, err = s.validateStateTransition(ctx, preState, blockCopy)
		if errors.Is(err, ErrNotDescendantOfFinalized) {
			return invalidBlock{error: err, root: blockRoot}
		}
		if err != nil {
			return errors.Wrap(err, "failed to validate consensus state transition function")
		}
		return nil
	})
	var isValidPayload bool
	var payloadErr error
	eg.Go(func() error {
		isValidPayload, payloadErr = s.notifyNewPayload(ctx, preStateHeader, blockCopy)
		// Consensus errors take precedence. Until the transition succeeds, the
		// payload's execution ancestry may not match the block's beacon ancestry.
		return nil
	})
	consensusErr := eg.Wait()

	// The rest of block processing takes a lock on forkchoice.
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	if err := s.checkInvalidBlock(blockRoot, blockCopy.Block().ParentRoot()); err != nil {
		return err
	}
	// Accepted blocks have already passed consensus validation. Finalization
	// can remove their parents while this request's transition is starting.
	imported := false
	if consensusErr != nil {
		imported, err = s.isImportedBlock(ctx, blockRoot, blockCopy.Block().Slot())
		if err != nil {
			return err
		}
		if !imported {
			return consensusErr
		}
	}
	if payloadErr != nil {
		err = s.handleInvalidExecutionError(ctx, payloadErr, blockRoot, blockCopy.Block().ParentRoot())
		return errors.Wrap(err, "could not notify the engine of the new payload")
	}
	// Consensus now establishes that the execution and beacon ancestors agree.
	// Preserve their completed VALID verdict before this block's import can
	// fail, including when cancellation raced the execution response.
	parentRoot := blockCopy.Block().ParentRoot()
	if isValidPayload && s.cfg.ForkChoiceStore.HasNode(parentRoot) {
		if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(context.WithoutCancel(ctx), parentRoot); err != nil {
			return errors.Wrap(err, "could not set optimistic parent to valid")
		}
		s.refreshHeadOptimisticStatus()
	}
	// A batch may have imported this block while validation ran without the
	// lock. Do not repeat persistence: a failed state save would roll back
	// the batch's accepted block. Keep any new execution validation, though.
	if !imported {
		imported, err = s.isImportedBlock(ctx, blockRoot, blockCopy.Block().Slot())
		if err != nil {
			return err
		}
	}
	if imported {
		if isValidPayload && s.cfg.ForkChoiceStore.HasNode(blockRoot) {
			if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(context.WithoutCancel(ctx), blockRoot); err != nil {
				return errors.Wrap(err, "could not set optimistic block to valid")
			}
			s.refreshHeadOptimisticStatus()
			if err := ctx.Err(); err != nil {
				return err
			}
			newFinalized, err := s.updateCheckpoints(ctx)
			if newFinalized {
				s.notifyFinalized()
			}
			return err
		}
		return nil
	}
	// Finality may have advanced while the state transition and the payload
	// verification ran without the lock. Only new blocks need this check.
	if err := s.verifyBlkFinalizedSlot(blockCopy.Block()); err != nil {
		return errors.Wrap(err, "block conflicts with the finalized checkpoint")
	}
	if err := s.savePostStateInfo(ctx, blockRoot, blockCopy, postState); err != nil {
		return errors.Wrap(err, "could not save post state info")
	}
	if features.Get().EnableSlasher {
		// Local errors after insertion leave the block accepted, and retries
		// skip it. Deliver its attestations if execution retained the block.
		defer func() {
			if s.cfg.ForkChoiceStore.HasNode(blockRoot) {
				// The transition may have advanced preState in place, making it
				// the object StateGen retains for a later batch to mutate.
				// Snapshot it under the import lock, before the worker starts.
				go s.sendBlockAttestationsToSlasher(blockCopy, preState.Copy())
			}
		}()
	}
	if err := s.postBlockProcess(ctx, roblock, postState, isValidPayload); err != nil {
		err := errors.Wrap(err, "could not process block")
		tracing.AnnotateError(span, err)
		return err
	}
	s.reportEpochMetrics(postState, currentEpoch)
	newFinalized, err := s.updateCheckpoints(ctx)
	// Persisted finality still needs notification if its validation write failed.
	if newFinalized {
		s.notifyFinalized()
	}
	if err != nil {
		return errors.Wrap(err, "could not update checkpoints")
	}

	// Handle post block operations such as pruning exits and ml-dsa-87 messages if incoming block is the head
	if err := s.prunePostBlockOperationPools(ctx, blockCopy, blockRoot); err != nil {
		log.WithError(err).Error("Could not prune canonical objects from pool ")
	}

	// Have we been finalizing? Should we start saving hot states to db?
	if err := s.checkSaveHotStateDB(ctx); err != nil {
		log.WithError(err).Error("Could not check save hot state DB")
	}

	// Reports on block and fork choice metrics.
	cp := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	finalized := &qrysmpb.Checkpoint{Epoch: cp.Epoch, Root: bytesutil.SafeCopyBytes(cp.Root[:])}
	reportSlotMetrics(blockCopy.Block().Slot(), s.HeadSlot(), s.CurrentSlot(), finalized)

	// Log block sync status.
	cp = s.cfg.ForkChoiceStore.JustifiedCheckpoint()
	justified := &qrysmpb.Checkpoint{Epoch: cp.Epoch, Root: bytesutil.SafeCopyBytes(cp.Root[:])}
	if err := logBlockSyncStatus(blockCopy.Block(), blockRoot, justified, finalized, receivedTime, uint64(s.genesisTime.Unix())); err != nil {
		log.WithError(err).Error("Unable to log block sync status")
	}
	// Log payload data
	if err := logPayload(blockCopy.Block()); err != nil {
		log.WithError(err).Error("Unable to log debug block payload data")
	}
	// Log state transition data.
	if err := logStateTransitionData(blockCopy.Block()); err != nil {
		log.WithError(err).Error("Unable to log state transition data")
	}

	chainServiceProcessingTime.Observe(float64(time.Since(receivedTime).Milliseconds()))

	return nil
}

// ReceiveBlockBatch processes the whole block batch at once, assuming the block batch is linear ,transitioning
// the state, performing batch verification of all collected signatures and then performing the appropriate
// actions for a block post-transition.
func (s *Service) ReceiveBlockBatch(ctx context.Context, blocks []blocks.ROBlock) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.ReceiveBlockBatch")
	defer span.End()

	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()

	// Apply state transition on the incoming newly received block batches, one by one.
	if err := s.onBlockBatch(ctx, blocks); err != nil {
		err := errors.Wrap(err, "could not process block in batch")
		tracing.AnnotateError(span, err)
		return err
	}

	for _, b := range blocks {
		// Reports on block and fork choice metrics.
		cp := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
		finalized := &qrysmpb.Checkpoint{Epoch: cp.Epoch, Root: bytesutil.SafeCopyBytes(cp.Root[:])}
		reportSlotMetrics(b.Block().Slot(), s.HeadSlot(), s.CurrentSlot(), finalized)
	}

	return nil
}

// HasBlock reports imported blocks and stored history at or before finality.
// Recent blocks must also be in forkchoice: a batch can persist its blocks and
// states before insertion fails. Those blocks must remain retryable, and their
// children must wait for the parent import instead of failing verification.
func (s *Service) HasBlock(ctx context.Context, root [32]byte) bool {
	s.cfg.ForkChoiceStore.RLock()
	defer s.cfg.ForkChoiceStore.RUnlock()
	// A batch may have completed this import while duplicate gossip is still
	// waiting. Check the accepted block before considering the in-flight request.
	if s.cfg.ForkChoiceStore.HasNode(root) {
		return s.hasBlockInInitSyncOrDB(ctx, root)
	}
	if s.BlockBeingSynced(root) {
		return false
	}
	b, err := s.getBlock(ctx, root)
	if err != nil {
		return false
	}
	finalizedSlot, err := slots.EpochStart(s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
	return err == nil && b.Block().Slot() <= finalizedSlot
}

// ReceiveAttesterSlashing receives an attester slashing and inserts it to forkchoice
func (s *Service) ReceiveAttesterSlashing(ctx context.Context, slashing *qrysmpb.AttesterSlashing) {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.InsertSlashingsToForkChoiceStore(ctx, []*qrysmpb.AttesterSlashing{slashing})
}

// prunePostBlockOperationPools only runs on new head otherwise should return a nil.
func (s *Service) prunePostBlockOperationPools(ctx context.Context, blk interfaces.ReadOnlySignedBeaconBlock, root [32]byte) error {
	headRoot, err := s.HeadRoot(ctx)
	if err != nil {
		return err
	}
	// By comparing the current headroot, that has already gone through forkchoice,
	// we can assume that if equal the current block root is canonical.
	if !bytes.Equal(headRoot, root[:]) {
		return nil
	}

	// Mark block exits as seen so we don't include same ones in future blocks.
	for _, e := range blk.Block().Body().VoluntaryExits() {
		s.cfg.ExitPool.MarkIncluded(e)
	}

	// Mark slashings as seen so we don't include same ones in future blocks.
	for _, as := range blk.Block().Body().AttesterSlashings() {
		s.cfg.SlashingPool.MarkIncludedAttesterSlashing(as)
	}
	for _, ps := range blk.Block().Body().ProposerSlashings() {
		s.cfg.SlashingPool.MarkIncludedProposerSlashing(ps)
	}

	return nil
}

// This checks whether it's time to start saving hot state to DB.
// It's time when there's `epochsSinceFinalitySaveHotStateDB` epochs of non-finality.
// Requires a read lock on forkchoice
func (s *Service) checkSaveHotStateDB(ctx context.Context) error {
	currentEpoch := slots.ToEpoch(s.CurrentSlot())
	// Prevent `sinceFinality` going underflow.
	var sinceFinality primitives.Epoch
	finalized := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	if finalized == nil {
		return errNilFinalizedInStore
	}
	if currentEpoch > finalized.Epoch {
		sinceFinality = currentEpoch - finalized.Epoch
	}

	if sinceFinality >= epochsSinceFinalitySaveHotStateDB {
		s.cfg.StateGen.EnableSaveHotStateToDB(ctx)
		return nil
	}

	return s.cfg.StateGen.DisableSaveHotStateToDB(ctx)
}

// This performs the state transition function and returns the poststate or an
// error if the block fails to verify the consensus rules
func (s *Service) validateStateTransition(ctx context.Context, preState state.BeaconState, signed interfaces.ReadOnlySignedBeaconBlock) (state.BeaconState, error) {
	b := signed.Block()
	// Verify that the parent block is in forkchoice
	parentRoot := b.ParentRoot()
	if !s.InForkchoice(parentRoot) {
		return nil, ErrNotDescendantOfFinalized
	}
	stateTransitionStartTime := time.Now()
	postState, err := transition.ExecuteStateTransition(ctx, preState, signed)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		return nil, invalidBlock{error: err}
	}
	stateTransitionProcessingTime.Observe(float64(time.Since(stateTransitionStartTime).Milliseconds()))
	return postState, nil
}

// notifyFinalized starts finalization notifications and deposit processing.
// The caller must hold the forkchoice lock and have persisted new finality.
func (s *Service) notifyFinalized() {
	finalized := *s.cfg.ForkChoiceStore.FinalizedCheckpoint()
	// Snapshot this checkpoint's execution status under the store lock;
	// the head and finalization may advance before the event goroutine runs.
	optimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(finalized.Root)
	if err != nil {
		log.WithError(err).Error("Could not get finalized checkpoint optimistic status")
		optimistic = true
	}
	// A request deadline cannot undo persisted finality. Let its notification
	// finish under the service context, just like state migration.
	go s.sendNewFinalizedEvent(s.ctx, &finalized, optimistic)
	depCtx, cancel := context.WithTimeout(context.Background(), depositDeadline)
	go func() {
		defer cancel()
		s.insertFinalizedDeposits(depCtx, finalized.Root)
	}()
}

// reportEpochMetrics asynchronously reports validator metrics on epoch
// boundaries. The work iterates the full validator set and was previously
// blocking ReceiveBlock; running it in a goroutine keeps the block-import
// path under the slot deadline.
func (s *Service) reportEpochMetrics(postState state.BeaconState, prevEpoch primitives.Epoch) {
	if coreTime.CurrentEpoch(postState) <= prevEpoch {
		return
	}
	// StateGen retains postState for a later batch to consume and mutate.
	// Snapshot it while holding the import lock, before the worker starts.
	snapshot := postState.Copy()
	go func() {
		headSt, err := s.HeadState(s.ctx)
		if err != nil {
			log.WithError(err).Error("Could not get head state for epoch metrics")
			return
		}
		if err := reportEpochMetrics(s.ctx, snapshot, headSt); err != nil {
			log.WithError(err).Error("Could not report epoch metrics")
		}
	}()
}

// sendNewFinalizedEvent sends a new finalization checkpoint event over the
// event feed. It needs to be called on the background.
//
// The event's state root must be the state root of the *finalized* block, not
// of the block whose processing triggered finalization: consumers of the
// finalized_checkpoint SSE use it to address the finalized state.
func (s *Service) sendNewFinalizedEvent(ctx context.Context, finalized *forkchoicetypes.Checkpoint, optimistic bool) {
	finalizedRoot := finalized.Root
	blk, err := s.getBlock(ctx, finalizedRoot)
	if err != nil {
		log.WithError(err).WithField("root", fmt.Sprintf("%#x", finalizedRoot)).
			Error("Could not retrieve block for finalized checkpoint root. Finalized event will not be emitted")
		return
	}
	stateRoot := blk.Block().StateRoot()

	// Send an event regarding the new finalized checkpoint over a common event feed.
	s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.FinalizedCheckpoint,
		Data: &qrlpb.EventFinalizedCheckpoint{
			Epoch:               finalized.Epoch,
			Block:               finalizedRoot[:],
			State:               stateRoot[:],
			ExecutionOptimistic: optimistic,
		},
	})
}

// sendBlockAttestationsToSlasher sends the incoming block's attestation to the slasher
func (s *Service) sendBlockAttestationsToSlasher(signed interfaces.ReadOnlySignedBeaconBlock, preState state.BeaconState) {
	// Feed the indexed attestation to slasher if enabled. This action
	// is done in the background to avoid adding more load to this critical code path.
	ctx := s.ctx
	for _, att := range signed.Block().Body().Attestations() {
		committee, err := helpers.BeaconCommitteeFromState(ctx, preState, att.Data.Slot, att.Data.CommitteeIndex)
		if err != nil {
			log.WithError(err).Error("Could not get attestation committee")
			continue
		}
		indexedAtt, err := attestation.ConvertToIndexed(ctx, att, committee)
		if err != nil {
			log.WithError(err).Error("Could not convert to indexed attestation")
			continue
		}
		s.cfg.SlasherAttestationsFeed.Send(indexedAtt)
	}
}
