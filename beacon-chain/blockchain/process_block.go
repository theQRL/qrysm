package blockchain

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/epoch/precompute"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	coreTime "github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/monitoring/tracing"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
)

// A custom slot deadline for processing state slots in our cache.
const slotDeadline = 5 * time.Second

// A custom deadline for deposit trie insertion.
const depositDeadline = 20 * time.Second

// This defines size of the upper bound for initial sync block cache.
var initialSyncBlockCacheSize = uint64(2 * params.BeaconConfig().SlotsPerEpoch)

// postBlockProcess is called when a gossip block is received. This function performs
// several duties most importantly informing the engine if head was updated,
// saving the new head information to the blockchain package and
// handling attestations, slashings and similar included in the block.
func (s *Service) postBlockProcess(ctx context.Context, roblock consensusblocks.ROBlock, postState state.BeaconState, isValidPayload bool) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.onBlock")
	defer span.End()
	if err := consensusblocks.BeaconBlockIsNil(roblock); err != nil {
		return invalidBlock{error: err}
	}
	startTime := time.Now()

	if err := s.cfg.ForkChoiceStore.InsertNode(ctx, postState, roblock); err != nil {
		// Use a fresh background context for the rollback so it isn't aborted
		// by the same deadline that just failed the InsertNode call.
		rollbackCtx := trace.NewContext(context.Background(), span)
		s.rollbackBlock(rollbackCtx, roblock.Root())
		return errors.Wrapf(err, "could not insert block %d to fork choice store", roblock.Block().Slot())
	}
	// Local failures after insertion leave this block imported, and duplicate
	// imports skip it. Announce it on every exit unless execution removed it.
	defer func() {
		if s.cfg.ForkChoiceStore.HasNode(roblock.Root()) {
			reportAttestationInclusion(roblock.Block())
			s.sendStateFeedOnBlock(roblock)
		}
	}()
	// Execution already returned VALID. Preserve that verdict for the retained
	// node before any local failure; duplicate imports will skip this work.
	if isValidPayload {
		if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(context.WithoutCancel(ctx), roblock.Root()); err != nil {
			return errors.Wrap(err, "could not set optimistic block to valid")
		}
		s.refreshHeadOptimisticStatus()
	}
	if err := s.handleBlockAttestations(ctx, roblock.Block()); err != nil {
		return errors.Wrap(err, "could not handle block's attestations")
	}

	s.InsertSlashingsToForkChoiceStore(ctx, roblock.Block().Body().AttesterSlashings())
	if err := ctx.Err(); err != nil {
		return err
	}

	start := time.Now()
	headRoot, err := s.cfg.ForkChoiceStore.Head(ctx)
	if err != nil {
		log.WithError(err).Warn("Could not update head")
	}
	if roblock.Root() != headRoot {
		receivedWeight, err := s.cfg.ForkChoiceStore.Weight(roblock.Root())
		if err != nil {
			log.WithField("root", fmt.Sprintf("%#x", roblock.Root())).Warn("Could not determine node weight")
		}
		headWeight, err := s.cfg.ForkChoiceStore.Weight(headRoot)
		if err != nil {
			log.WithField("root", fmt.Sprintf("%#x", headRoot)).Warn("Could not determine node weight")
		}
		log.WithFields(logrus.Fields{
			"receivedRoot":   fmt.Sprintf("%#x", roblock.Root()),
			"receivedWeight": receivedWeight,
			"headRoot":       fmt.Sprintf("%#x", headRoot),
			"headWeight":     headWeight,
		}).Debug("Head block is not the received block")
	}
	newBlockHeadElapsedTime.Observe(float64(time.Since(start).Milliseconds()))

	if headRoot == roblock.Root() {
		// Updating next slot state cache can happen in the background
		// except in the epoch boundary in which case we lock to handle
		// the shuffling and proposer caches updates.
		// We handle these caches only on canonical
		// blocks, otherwise this will be handled by lateBlockTasks
		slot := postState.Slot()
		blockRoot := roblock.Root()
		if slots.IsEpochEnd(slot) {
			if err := transition.UpdateNextSlotCache(ctx, blockRoot[:], postState); err != nil {
				return errors.Wrap(err, "could not update next slot state cache")
			}
			if err := s.handleEpochBoundary(ctx, slot, postState, blockRoot[:]); err != nil {
				return errors.Wrap(err, "could not handle epoch boundary")
			}
		} else {
			// StateGen retains postState for a later batch to consume and mutate.
			// Snapshot it while holding the import lock, before the worker starts.
			cacheState := postState.Copy()
			go func() {
				slotCtx, cancel := context.WithTimeout(context.Background(), slotDeadline)
				defer cancel()
				if err := transition.UpdateNextSlotCache(slotCtx, blockRoot[:], cacheState); err != nil {
					log.WithError(err).Error("Could not update next slot state cache")
				}
			}()
		}
	}

	// verify conditions for FCU, notifies FCU, and saves the new head.
	// This function also prunes attestations, other similar operations happen in prunePostBlockOperationPools.
	if _, err := s.forkchoiceUpdateWithExecution(ctx, headRoot, s.CurrentSlot()+1); err != nil {
		return classifyForkchoiceError(err, []consensusblocks.ROBlock{roblock})
	}

	onBlockProcessingTime.Observe(float64(time.Since(startTime).Milliseconds()))
	return nil
}

// sendStateFeedOnBlock dispatches the block-processed state-feed event.
// The caller holds the forkchoice lock and has checked that the imported block
// survived execution processing, including any local failure during the import.
func (s *Service) sendStateFeedOnBlock(roblock consensusblocks.ROBlock) {
	optimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(roblock.Root())
	if err != nil {
		log.WithError(err).Debug("Could not check if block is optimistic")
		optimistic = true
	}
	s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.BlockProcessed,
		Data: &statefeed.BlockProcessedData{
			Slot:        roblock.Block().Slot(),
			BlockRoot:   roblock.Root(),
			SignedBlock: roblock,
			Verified:    true,
			Optimistic:  optimistic,
		},
	})
}

// sendStateFeedOnBatch reports execution status for a retained batch prefix,
// including ancestors pruned by finalization. The caller holds the store lock.
func (s *Service) sendStateFeedOnBatch(blks []consensusblocks.ROBlock, lastValidIndex int) error {
	optimistic := make([]bool, len(blks))
	descendantOptimistic := true
	for i := len(blks) - 1; i >= 0; i-- {
		status, err := s.cfg.ForkChoiceStore.IsOptimistic(blks[i].Root())
		if err != nil {
			if !errors.Is(err, doublylinkedtree.ErrNilNode) {
				return err
			}
			// Finalization may have pruned this prefix. A VALID payload response
			// or a validated descendant still establishes its execution validity.
			status = i > lastValidIndex && descendantOptimistic
		}
		optimistic[i], descendantOptimistic = status, status
	}
	for i, b := range blks {
		blockCopy, err := b.Copy()
		if err != nil {
			return err
		}
		s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.BlockProcessed,
			Data: &statefeed.BlockProcessedData{
				Slot:        blockCopy.Block().Slot(),
				BlockRoot:   b.Root(),
				SignedBlock: blockCopy,
				Verified:    true,
				Optimistic:  optimistic[i],
			},
		})
	}
	return nil
}

func getStateVersionAndPayload(st state.BeaconState) (int, interfaces.ExecutionData, error) {
	if st == nil {
		return 0, nil, errors.New("nil state")
	}
	var preStateHeader interfaces.ExecutionData
	var err error
	preStateVersion := st.Version()
	preStateHeader, err = st.LatestExecutionPayloadHeader()
	if err != nil {
		return 0, nil, err
	}
	return preStateVersion, preStateHeader, nil
}

func (s *Service) onBlockBatch(ctx context.Context, blks []consensusblocks.ROBlock) (retErr error) {
	ctx, span := trace.StartSpan(ctx, "blockChain.onBlockBatch")
	defer span.End()

	if len(blks) == 0 {
		return errors.New("no blocks provided")
	}

	// Check the whole batch before retrieving a mutable cached pre-state.
	// A known descendant proves that a linear prefix was already imported,
	// including ancestors that finalization has since pruned from forkchoice.
	firstNew := 0
	for i, blk := range blks {
		if err := consensusblocks.BeaconBlockIsNil(blk); err != nil {
			return invalidBlock{error: err}
		}
		if i > 0 && (blk.Block().ParentRoot() != blks[i-1].Root() || blk.Block().Slot() <= blks[i-1].Block().Slot()) {
			return invalidBlock{error: errors.New("block batch is not linear"), root: blk.Root()}
		}
		if err := slots.VerifyTime(uint64(s.genesisTime.Unix()), blk.Block().Slot(), params.BeaconNetworkConfig().MaximumGossipClockDisparity); err != nil {
			return err
		}
		if err := s.checkInvalidBlock(blk.Root(), blk.Block().ParentRoot()); err != nil {
			return err
		}
		if s.cfg.ForkChoiceStore.HasNode(blk.Root()) {
			firstNew = i + 1
		}
	}
	if firstNew == 0 {
		// Finalization may have pruned every block in a retry. The tail's
		// finalized ancestry proves that the whole linear batch was accepted.
		last := blks[len(blks)-1]
		imported, err := s.isImportedBlock(ctx, last.Root(), last.Block().Slot())
		if err != nil {
			return err
		}
		if imported {
			firstNew = len(blks)
		}
	}
	if err := s.retryInvalidBlockCleanup(ctx); err != nil {
		log.WithError(err).Error("Could not finish invalid block cleanup")
	}
	if firstNew == len(blks) {
		// A prior local failure can leave the whole batch accepted before its
		// checkpoints or head were saved. Retry those steps without replaying
		// finalized ancestors or publishing duplicate block notifications.
		return s.completeBatchImport(ctx, blks, nil)
	}
	blks = blks[firstNew:]
	// Apply the finalized-slot restriction only to new blocks. Already accepted
	// ancestors must remain retryable after a local checkpoint write failure.
	for _, blk := range blks {
		if err := s.verifyBlkFinalizedSlot(blk.Block()); err != nil {
			return err
		}
	}
	b := blks[0].Block()

	// Retrieve incoming block's pre state.
	if err := s.verifyBlkPreState(ctx, b); err != nil {
		return err
	}
	preState, err := s.cfg.StateGen.StateByRootInitialSync(ctx, b.ParentRoot())
	if err != nil {
		return err
	}
	if preState == nil || preState.IsNil() {
		return fmt.Errorf("nil pre state for slot %d", b.Slot())
	}

	// Fill in missing blocks
	if err := s.fillInForkChoiceMissingBlocks(ctx, blks[0].Block(), preState.FinalizedCheckpoint(), preState.CurrentJustifiedCheckpoint()); err != nil {
		return errors.Wrap(err, "could not fill in missing blocks to forkchoice")
	}

	pendingNodes := make([]*forkchoicetypes.BlockAndCheckpoints, len(blks))
	sigSet := ml_dsa_87.NewSet()
	type versionAndHeader struct {
		version int
		header  interfaces.ExecutionData
	}
	preVersionAndHeaders := make([]*versionAndHeader, len(blks))
	postVersionAndHeaders := make([]*versionAndHeader, len(blks))
	var set *ml_dsa_87.SignatureBatch
	boundaries := make(map[[32]byte]state.BeaconState)
	var pendingAttestations []*qrysmpb.Attestation
	for i, b := range blks {
		v, h, err := getStateVersionAndPayload(preState)
		if err != nil {
			return err
		}
		preVersionAndHeaders[i] = &versionAndHeader{
			version: v,
			header:  h,
		}

		set, preState, err = transition.ExecuteStateTransitionNoVerifyAnySig(ctx, preState, b)
		if err != nil {
			// Local cancellation must not mark valid peer data as invalid.
			if ctx.Err() != nil {
				return err
			}
			return invalidBlock{error: err}
		}
		// Authenticate votes against their target states after the batch is
		// validated and inserted, when those checkpoints are available.
		pendingAttestations = append(pendingAttestations, b.Block().Body().Attestations()...)
		// Save potential boundary states.
		if slots.IsEpochStart(preState.Slot()) {
			boundaries[b.Root()] = preState.Copy()
		}
		// Every epoch start before the next block uses this checkpoint root,
		// even if the next block starts a later epoch after an empty one. Keep
		// its state so forkchoice can weigh the checkpoint while the batch's
		// blocks are still only in the initial-sync cache.
		if i+1 < len(blks) {
			next := blks[i+1].Block().Slot()
			if slots.ToEpoch(next-1) > slots.ToEpoch(b.Block().Slot()) {
				boundaries[b.Root()] = preState.Copy()
			}
		}
		// Capture each block's checkpoints before advancing the state again.
		// Forkchoice will realize older blocks' observations at insertion time.
		uj, uf, err := precompute.UnrealizedCheckpoints(preState)
		if err != nil {
			return errors.Wrap(err, "could not compute batch unrealized checkpoints")
		}
		pendingNodes[i] = &forkchoicetypes.BlockAndCheckpoints{
			Block:                         b,
			JustifiedCheckpoint:           preState.CurrentJustifiedCheckpoint(),
			FinalizedCheckpoint:           preState.FinalizedCheckpoint(),
			UnrealizedJustifiedCheckpoint: uj,
			UnrealizedFinalizedCheckpoint: uf,
		}

		v, h, err = getStateVersionAndPayload(preState)
		if err != nil {
			return err
		}
		postVersionAndHeaders[i] = &versionAndHeader{
			version: v,
			header:  h,
		}
		sigSet.Join(set)
	}

	var verify bool
	if features.Get().EnableVerboseSigVerification {
		verify, err = sigSet.VerifyVerbosely()
	} else {
		verify, err = sigSet.Verify()
	}
	if err != nil {
		return invalidBlock{error: err}
	}
	if !verify {
		return invalidBlock{error: errors.New("batch block signature verification failed")}
	}

	// Check every payload before persisting the batch. A later INVALID response
	// can invalidate an earlier SYNCING payload and its checkpoint observations.
	lastValidIndex := -1
	for i, b := range blks {
		isValidPayload, err := s.notifyNewPayload(ctx,
			postVersionAndHeaders[i].header, b)
		if err != nil {
			return s.handleInvalidBatchExecutionError(ctx, err, blks[:i+1])
		}
		if isValidPayload {
			// The whole linear batch passed consensus validation. Its first
			// VALID payload also validates the already imported parent, even
			// if a later RPC, save, or insertion fails before retaining a node.
			parentRoot := blks[0].Block().ParentRoot()
			if lastValidIndex == -1 && s.cfg.ForkChoiceStore.HasNode(parentRoot) {
				if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(context.WithoutCancel(ctx), parentRoot); err != nil {
					return errors.Wrap(err, "could not set optimistic batch parent to valid")
				}
				s.refreshHeadOptimisticStatus()
			}
			lastValidIndex = i
		}
	}

	for _, b := range blks {
		root := b.Root()
		if err := s.saveInitSyncBlock(ctx, root, b); err != nil {
			tracing.AnnotateError(span, err)
			return err
		}
		if err := s.cfg.BeaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{
			Slot: b.Block().Slot(),
			Root: root[:],
		}); err != nil {
			tracing.AnnotateError(span, err)
			return err
		}
	}
	// Save boundary states that will be useful for forkchoice
	for r, st := range boundaries {
		if err := s.cfg.StateGen.SaveState(ctx, r, st); err != nil {
			return err
		}
	}
	lastB := blks[len(blks)-1]
	lastBR := lastB.Root()
	// Also saves the last post state which to be used as pre state for the next batch.
	if err := s.cfg.StateGen.SaveState(ctx, lastBR, preState); err != nil {
		return err
	}
	// Insertion can retain a prefix even when a later step fails or execution
	// rejects the tail. Persist its blocks first: an interior head may have no
	// cached state, and state regeneration cannot read the initial-sync cache.
	if err := s.saveInitSyncBlocks(ctx, false); err != nil {
		return err
	}
	// Record validated slashings before InsertChain can retain a prefix on
	// failure. Duplicate imports skip those blocks and cannot repair omissions.
	for _, b := range blks {
		s.InsertSlashingsToForkChoiceStore(ctx, b.Block().Body().AttesterSlashings())
	}
	// Announce newly inserted blocks, including a retained prefix on failure.
	defer func() {
		end := len(blks)
		for end > 0 && !s.cfg.ForkChoiceStore.HasNode(blks[end-1].Root()) {
			end--
		}
		if end == 0 {
			return
		}
		// Retained descendants establish that this linear prefix was inserted,
		// even when finalization has already pruned its oldest nodes.
		if err := s.sendStateFeedOnBatch(blks[:end], lastValidIndex); err != nil {
			if retErr == nil {
				retErr = err
			} else {
				log.WithError(err).Error("Could not announce imported batch")
			}
		}
	}()
	// Insert all nodes to forkchoice
	insertErr := s.cfg.ForkChoiceStore.InsertChain(ctx, pendingNodes)
	// Keep VALID verdicts for every retained ancestor, even if the validated
	// descendant was not inserted. Retries skip this prefix, and cancellation
	// cannot undo execution validation that has already completed.
	for i := lastValidIndex; i >= 0; i-- {
		if !s.cfg.ForkChoiceStore.HasNode(blks[i].Root()) {
			continue
		}
		if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(context.WithoutCancel(ctx), blks[i].Root()); err != nil {
			if insertErr == nil {
				insertErr = errors.Wrap(err, "could not set optimistic block to valid")
			} else {
				insertErr = fmt.Errorf("%w: could not set optimistic block to valid: %w", insertErr, err)
			}
		} else {
			s.refreshHeadOptimisticStatus()
		}
		break
	}
	if insertErr != nil {
		insertErr = errors.Wrap(insertErr, "could not insert batch to forkchoice")
		// A completed prefix survives insertion errors and duplicate imports
		// skip its blocks. Account for its votes even if insertion was cancelled.
		recoveryCtx := context.WithoutCancel(ctx)
		for _, b := range blks {
			if !s.cfg.ForkChoiceStore.HasNode(b.Root()) {
				continue
			}
			if err := s.handleBlockAttestations(recoveryCtx, b.Block()); err != nil {
				insertErr = fmt.Errorf("%w: could not handle retained block attestations: %w", insertErr, err)
			}
		}
		return insertErr
	}
	if err := s.applyBlockAttestations(ctx, pendingAttestations); err != nil {
		return errors.Wrap(err, "could not handle batch attestations")
	}
	return s.completeBatchImport(ctx, blks, preState)
}

// completeBatchImport publishes the selected head and persists checkpoints,
// including retries after the blocks were already accepted. lastState may be
// nil when retrying; the supplied batch is nonempty and the caller holds the
// forkchoice write lock.
func (s *Service) completeBatchImport(ctx context.Context, blks []consensusblocks.ROBlock, lastState state.BeaconState) error {
	// Establish the selected head before publishing it to the engine and the
	// service cache. A competing branch can win over the last block in the batch.
	headRoot, err := s.cfg.ForkChoiceStore.Head(ctx)
	if err != nil {
		return errors.Wrap(err, "could not select head after batch import")
	}
	lastB := blks[len(blks)-1]
	var headBlock interfaces.ReadOnlySignedBeaconBlock = lastB
	headState := lastState
	if headRoot != lastB.Root() || headState == nil {
		headState, headBlock, err = s.getStateAndBlock(ctx, headRoot)
		if err != nil {
			return errors.Wrap(err, "could not get selected head after batch import")
		}
	}
	arg := &notifyForkchoiceUpdateArg{
		headState: headState,
		headRoot:  headRoot,
		headBlock: headBlock.Block(),
	}
	if _, err := s.notifyForkchoiceUpdate(ctx, arg); err != nil {
		return classifyForkchoiceError(err, blks)
	}
	// Persist the accepted store checkpoints, including changes observed by
	// the first block or pulled up from older epochs. Execution processing must
	// finish first so finalization records the checkpoint's validation status.
	if _, err := s.updateCheckpoints(ctx); err != nil {
		return err
	}
	optimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(headRoot)
	if err != nil {
		return errors.Wrap(err, "could not get selected head optimistic status")
	}
	if err := s.saveHeadNoDB(ctx, headBlock, headRoot, headState, optimistic); err != nil {
		return err
	}
	return nil
}

func (s *Service) updateEpochBoundaryCaches(ctx context.Context, st state.BeaconState) error {
	e := coreTime.CurrentEpoch(st)
	if err := helpers.UpdateCommitteeCache(ctx, st, e); err != nil {
		return errors.Wrap(err, "could not update committee cache")
	}
	if err := helpers.UpdateProposerIndicesInCache(ctx, st, e); err != nil {
		return errors.Wrap(err, "could not update proposer index cache")
	}
	go func(ep primitives.Epoch) {
		// Use a custom deadline here, since this method runs asynchronously.
		// We ignore the parent method's context and instead create a new one
		// with a custom deadline, therefore using the background context instead.
		slotCtx, cancel := context.WithTimeout(context.Background(), slotDeadline)
		defer cancel()
		if err := helpers.UpdateCommitteeCache(slotCtx, st, ep+1); err != nil {
			log.WithError(err).Warn("Could not update committee cache")
		}
		// The proposer-indices cache for epoch ep+1 is intentionally not warmed
		// here: its key is the state root at the end of epoch ep, a future slot
		// not yet recorded in this state, so precomputing it could only cache
		// under the wrong key. It is populated on demand once that root exists.
	}(e)
	return nil
}

// Epoch boundary tasks: it copies the headState and updates the epoch boundary
// caches.
func (s *Service) handleEpochBoundary(ctx context.Context, slot primitives.Slot, headState state.BeaconState, blockRoot []byte) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.handleEpochBoundary")
	defer span.End()
	// return early if we are advancing to a past epoch
	if slot < headState.Slot() {
		return nil
	}
	if !slots.IsEpochEnd(slot) {
		return nil
	}
	copied := headState.Copy()
	copied, err := transition.ProcessSlotsUsingNextSlotCache(ctx, copied, blockRoot, slot+1)
	if err != nil {
		return err
	}
	return s.updateEpochBoundaryCaches(ctx, copied)
}

// handleBlockAttestations authenticates included votes against their target
// states before applying them. The caller must hold the forkchoice write lock.
func (s *Service) handleBlockAttestations(ctx context.Context, blk interfaces.ReadOnlyBeaconBlock) error {
	return s.applyBlockAttestations(ctx, blk.Body().Attestations())
}

func (s *Service) applyBlockAttestations(ctx context.Context, atts []*qrysmpb.Attestation) error {
	for _, a := range atts {
		// A batch may finalize and prune earlier attested blocks. Their old
		// votes cannot affect head, and need not be saved to the pending pool.
		if a.Data.Target.Epoch < s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch {
			continue
		}
		// The containing block is already in forkchoice, so a duplicate import
		// will not replay its votes. Keep every unprocessed vote on cancellation.
		if ctx.Err() != nil {
			if err := s.cfg.AttPool.SaveBlockAttestation(a); err != nil {
				return err
			}
			continue
		}
		r := bytesutil.ToBytes32(a.Data.BeaconBlockRoot)
		if s.cfg.ForkChoiceStore.HasNode(r) {
			if err := s.verifyAttestationForkchoice(a); err != nil {
				// Ignore an invalid forkchoice vote without rejecting its
				// consensus-valid containing block.
				continue
			}
			targetState, err := s.getAttPreState(ctx, a.Data.Target)
			if err != nil {
				// State regeneration can fail temporarily. Defer the vote
				// without rejecting its already validated containing block.
				if err := s.cfg.AttPool.SaveBlockAttestation(a); err != nil {
					return err
				}
				continue
			}
			indices, err := verifiedAttestingIndices(ctx, targetState, a)
			if err != nil {
				// Committee lookups can observe cancellation after target-state
				// retrieval succeeds. Such votes still need authentication later.
				if ctx.Err() != nil {
					if err := s.cfg.AttPool.SaveBlockAttestation(a); err != nil {
						return err
					}
				}
				continue
			}
			s.cfg.ForkChoiceStore.ProcessAttestation(ctx, indices, r, a.Data.Target.Epoch)
			// The vote is authenticated in its target state: later gossip for
			// these participants adds nothing, and the pool may skip it.
			if err := s.cfg.AttPool.MarkAppliedAttestation(a); err != nil {
				log.WithError(err).Debug("Could not mark applied included vote as seen")
			}
		} else if err := s.cfg.AttPool.SaveBlockAttestation(a); err != nil {
			return err
		}
	}
	return nil
}

// InsertSlashingsToForkChoiceStore inserts attester slashing indices to fork choice store.
// To call this function, it's caller's responsibility to ensure the slashing object is valid.
// This function requires a write lock on forkchoice.
func (s *Service) InsertSlashingsToForkChoiceStore(ctx context.Context, slashings []*qrysmpb.AttesterSlashing) {
	for _, slashing := range slashings {
		indices := blocks.SlashableAttesterIndices(slashing)
		for _, index := range indices {
			s.cfg.ForkChoiceStore.InsertSlashedIndex(ctx, primitives.ValidatorIndex(index))
		}
	}
}

// This saves post state info to DB or cache. This also saves post state info to fork choice store.
// Post state info consists of processed block and state. Do not call this method unless the block and state are verified.
func (s *Service) savePostStateInfo(ctx context.Context, r [32]byte, b interfaces.ReadOnlySignedBeaconBlock, st state.BeaconState) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.savePostStateInfo")
	defer span.End()
	if err := s.cfg.BeaconDB.SaveBlock(ctx, b); err != nil {
		return errors.Wrapf(err, "could not save block from slot %d", b.Block().Slot())
	}
	if err := s.cfg.StateGen.SaveState(ctx, r, st); err != nil {
		// Do not use parent context in the event it deadlined.
		ctx = trace.NewContext(context.Background(), span)
		log.Warnf("Rolling back insertion of block with root %#x", r)
		// The state save fills the hot or epoch-boundary cache before it writes
		// to the database, so a failure can leave the state cached for a block
		// that is about to be deleted; drop it, as rollbackBlock does.
		if cacheErr := s.cfg.StateGen.DeleteStateFromCaches(ctx, r); cacheErr != nil {
			log.WithError(cacheErr).Errorf("Could not delete state from caches with block root %#x", r)
		}
		if deleteErr := s.cfg.BeaconDB.DeleteBlock(ctx, r); deleteErr != nil {
			log.WithError(deleteErr).Errorf("Could not delete block with block root %#x", r)
		}
		return errors.Wrap(err, "could not save state")
	}
	return nil
}

// This removes the attestations in block `b` from the attestation mem pool.
// Only the pool entries they cover are marked seen: those were authenticated
// in their target state, whereas a block only proves its votes valid in its
// own state. applyBlockAttestations marks an included vote once fork choice
// verifies it, so a vote rejected there never shadows a genuine one.
func (s *Service) pruneAttsFromPool(headBlock interfaces.ReadOnlySignedBeaconBlock) error {
	atts := headBlock.Block().Body().Attestations()
	for _, att := range atts {
		if helpers.IsAggregated(att) {
			if err := s.cfg.AttPool.DeleteAggregatedAttestation(att); err != nil {
				return err
			}
		} else {
			if err := s.cfg.AttPool.DeleteUnaggregatedAttestation(att); err != nil {
				return err
			}
		}
	}
	return nil
}

// This routine checks if there is a cached proposer payload ID available for the next slot proposer.
// If there is not, it will call forkchoice updated with the correct payload attribute then cache the payload ID.
func (s *Service) runLateBlockTasks() {
	if err := s.waitForSync(); err != nil {
		log.WithError(err).Error("Failed to wait for initial sync")
		return
	}

	attThreshold := params.BeaconConfig().SecondsPerSlot / 3
	ticker := slots.NewSlotTickerWithOffset(s.genesisTime, time.Duration(attThreshold)*time.Second, params.BeaconConfig().SecondsPerSlot)
	for {
		select {
		case <-ticker.C():
			s.lateBlockTasks(s.ctx)
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting routine")
			ticker.Done()
			return
		}
	}
}

// lateBlockTasks  is called 4 seconds into the slot and performs tasks
// related to late blocks. It emits a MissedSlot state feed event.
// It calls FCU and sets the right attributes if we are proposing next slot
// it also updates the next slot cache and the proposer index cache to deal with skipped slots.
func (s *Service) lateBlockTasks(ctx context.Context) {
	currentSlot := s.CurrentSlot()
	if s.CurrentSlot() == s.HeadSlot() {
		return
	}
	s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.MissedSlot,
	})

	s.headLock.RLock()
	headRoot := s.headRoot()
	headState := s.headState(ctx)
	s.headLock.RUnlock()
	lastRoot, lastState := transition.LastCachedState()
	if lastState == nil {
		lastRoot, lastState = headRoot[:], headState
	}
	// Copy all the field tries in our cached state in the event of late
	// blocks.
	lastState.CopyAllTries()
	if err := transition.UpdateNextSlotCache(ctx, lastRoot, lastState); err != nil {
		log.WithError(err).Debug("Could not update next slot state cache")
	}
	if err := s.handleEpochBoundary(ctx, currentSlot, headState, headRoot[:]); err != nil {
		log.WithError(err).Error("lateBlockTasks: could not update epoch boundary caches")
	}
	// Head root should be empty when retrieving proposer index for the next slot.
	_, id, has := s.cfg.ProposerSlotIndexCache.GetProposerPayloadIDs(s.CurrentSlot()+1, [32]byte{} /* head root */)
	// There exists proposer for next slot, but we haven't called fcu w/ payload attribute yet.
	if (!has && !features.Get().PrepareAllPayloads) || id != [8]byte{} {
		return
	}

	// Serialize the head snapshot and FCU with block imports. The head may
	// have changed while preparing caches or waiting for the forkchoice lock.
	// FCU also needs the write lock to update validation status or prune nodes.
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.headLock.RLock()
	headRoot = s.headRoot()
	headState = s.headState(ctx)
	headBlock, err := s.headBlock()
	s.headLock.RUnlock()
	if err != nil {
		log.WithError(err).Debug("could not perform late block tasks: failed to retrieve head block")
		return
	}
	_, err = s.notifyForkchoiceUpdate(ctx, &notifyForkchoiceUpdateArg{
		headState: headState,
		headRoot:  headRoot,
		headBlock: headBlock.Block(),
	})
	if err != nil {
		log.WithError(err).Debug("could not perform late block tasks: failed to update forkchoice with engine")
	}
}

// waitForSync blocks until the node is synced to the head.
func (s *Service) waitForSync() error {
	select {
	case <-s.syncComplete:
		return nil
	case <-s.ctx.Done():
		return errors.New("context closed, exiting goroutine")
	}
}

// handleInvalidExecutionError applies an execution rejection to a consensus-valid
// block. The caller must hold the forkchoice write lock.
func (s *Service) handleInvalidExecutionError(ctx context.Context, err error, blockRoot [32]byte, parentRoot [32]byte) error {
	if IsInvalidBlock(err) && InvalidBlockLVH(err) != [32]byte{} {
		return s.pruneInvalidBlock(ctx, blockRoot, parentRoot, InvalidBlockLVH(err))
	}
	return err
}

// rollbackBlock undoes the on-disk and in-cache state changes made for a block
// whose post-processing failed (for example, an InsertNode error on fork
// choice). This keeps the DB, StateGen caches and forkchoice store consistent
// so the node does not retain a block that was never inserted into the chain.
func (s *Service) rollbackBlock(ctx context.Context, blockRoot [32]byte) {
	log.Warnf("Rolling back insertion of block with root %#x due to processing error", blockRoot)
	if err := s.cfg.StateGen.DeleteStateFromCaches(ctx, blockRoot); err != nil {
		log.WithError(err).Errorf("Could not delete state from caches with block root %#x", blockRoot)
	}
	if err := s.cfg.BeaconDB.DeleteBlock(ctx, blockRoot); err != nil {
		log.WithError(err).Errorf("Could not delete block with block root %#x", blockRoot)
	}
}
