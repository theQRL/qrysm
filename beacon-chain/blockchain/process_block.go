package blockchain

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	coreTime "github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
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
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
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
	if err := s.handleBlockAttestations(ctx, roblock.Block(), postState); err != nil {
		return errors.Wrap(err, "could not handle block's attestations")
	}

	s.InsertSlashingsToForkChoiceStore(ctx, roblock.Block().Body().AttesterSlashings())
	if isValidPayload {
		if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(ctx, roblock.Root()); err != nil {
			return errors.Wrap(err, "could not set optimistic block to valid")
		}
	}

	defer s.sendStateFeedOnBlock(roblock) // only send event after successful insertion

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
			go func() {
				slotCtx, cancel := context.WithTimeout(context.Background(), slotDeadline)
				defer cancel()
				if err := transition.UpdateNextSlotCache(slotCtx, blockRoot[:], postState); err != nil {
					log.WithError(err).Error("Could not update next slot state cache")
				}
			}()
		}
	}

	// verify conditions for FCU, notifies FCU, and saves the new head.
	// This function also prunes attestations, other similar operations happen in prunePostBlockOperationPools.
	if _, err := s.forkchoiceUpdateWithExecution(ctx, headRoot, s.CurrentSlot()+1); err != nil {
		return err
	}

	defer reportAttestationInclusion(roblock.Block())
	onBlockProcessingTime.Observe(float64(time.Since(startTime).Milliseconds()))
	return nil
}

// sendStateFeedOnBlock dispatches the block-processed state-feed event.
// It is invoked via defer once the block has been successfully imported into
// fork choice, so subscribers do not observe blocks that fail insertion.
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

func (s *Service) onBlockBatch(ctx context.Context, blks []consensusblocks.ROBlock) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.onBlockBatch")
	defer span.End()

	if len(blks) == 0 {
		return errors.New("no blocks provided")
	}

	if err := consensusblocks.BeaconBlockIsNil(blks[0]); err != nil {
		return invalidBlock{error: err}
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

	jCheckpoints := make([]*qrysmpb.Checkpoint, len(blks))
	fCheckpoints := make([]*qrysmpb.Checkpoint, len(blks))
	sigSet := ml_dsa_87.NewSet()
	type versionAndHeader struct {
		version int
		header  interfaces.ExecutionData
	}
	preVersionAndHeaders := make([]*versionAndHeader, len(blks))
	postVersionAndHeaders := make([]*versionAndHeader, len(blks))
	var set *ml_dsa_87.SignatureBatch
	boundaries := make(map[[32]byte]state.BeaconState)
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
			return invalidBlock{error: err}
		}
		// Save potential boundary states.
		if slots.IsEpochStart(preState.Slot()) {
			boundaries[b.Root()] = preState.Copy()
		}
		jCheckpoints[i] = preState.CurrentJustifiedCheckpoint()
		fCheckpoints[i] = preState.FinalizedCheckpoint()

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

	// blocks have been verified, save them and call the engine
	pendingNodes := make([]*forkchoicetypes.BlockAndCheckpoints, len(blks))
	var isValidPayload bool
	for i, b := range blks {
		root := b.Root()
		isValidPayload, err = s.notifyNewPayload(ctx,
			postVersionAndHeaders[i].header, b)
		if err != nil {
			return s.handleInvalidExecutionError(ctx, err, root, b.Block().ParentRoot())
		}

		args := &forkchoicetypes.BlockAndCheckpoints{Block: b,
			JustifiedCheckpoint: jCheckpoints[i],
			FinalizedCheckpoint: fCheckpoints[i]}
		pendingNodes[i] = args
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
		if i > 0 && jCheckpoints[i].Epoch > jCheckpoints[i-1].Epoch {
			if err := s.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, jCheckpoints[i]); err != nil {
				tracing.AnnotateError(span, err)
				return err
			}
		}
		if i > 0 && fCheckpoints[i].Epoch > fCheckpoints[i-1].Epoch {
			if err := s.updateFinalized(ctx, fCheckpoints[i]); err != nil {
				tracing.AnnotateError(span, err)
				return err
			}
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
	// Insert all nodes to forkchoice
	if err := s.cfg.ForkChoiceStore.InsertChain(ctx, pendingNodes); err != nil {
		return errors.Wrap(err, "could not insert batch to forkchoice")
	}
	// Set their optimistic status
	if isValidPayload {
		if err := s.cfg.ForkChoiceStore.SetOptimisticToValid(ctx, lastBR); err != nil {
			return errors.Wrap(err, "could not set optimistic block to valid")
		}
	}
	arg := &notifyForkchoiceUpdateArg{
		headState: preState,
		headRoot:  lastBR,
		headBlock: lastB.Block(),
	}
	if _, err := s.notifyForkchoiceUpdate(ctx, arg); err != nil {
		return err
	}
	return s.saveHeadNoDB(ctx, lastB, lastBR, preState, !isValidPayload)
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
		if err := helpers.UpdateProposerIndicesInCache(slotCtx, st, ep+1); err != nil {
			log.WithError(err).Warn("Failed to cache next epoch proposers")
		}
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

// This feeds in the attestations included in the block to fork choice store. It's allows fork choice store
// to gain information on the most current chain.
func (s *Service) handleBlockAttestations(ctx context.Context, blk interfaces.ReadOnlyBeaconBlock, st state.BeaconState) error {
	// Feed in block's attestations to fork choice store.
	for _, a := range blk.Body().Attestations() {
		committee, err := helpers.BeaconCommitteeFromState(ctx, st, a.Data.Slot, a.Data.CommitteeIndex)
		if err != nil {
			return err
		}
		indices, err := attestation.AttestingIndices(a.AggregationBits, committee)
		if err != nil {
			return err
		}
		r := bytesutil.ToBytes32(a.Data.BeaconBlockRoot)
		if s.cfg.ForkChoiceStore.HasNode(r) {
			s.cfg.ForkChoiceStore.ProcessAttestation(ctx, indices, r, a.Data.Target.Epoch)
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
		if deleteErr := s.cfg.BeaconDB.DeleteBlock(ctx, r); deleteErr != nil {
			log.WithError(deleteErr).Errorf("Could not delete block with block root %#x", r)
		}
		return errors.Wrap(err, "could not save state")
	}
	return nil
}

// This removes the attestations in block `b` from the attestation mem pool.
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

	s.headLock.RLock()
	headBlock, err := s.headBlock()
	if err != nil {
		s.headLock.RUnlock()
		log.WithError(err).Debug("could not perform late block tasks: failed to retrieve head block")
		return
	}
	s.headLock.RUnlock()
	s.cfg.ForkChoiceStore.RLock()
	_, err = s.notifyForkchoiceUpdate(ctx, &notifyForkchoiceUpdateArg{
		headState: headState,
		headRoot:  headRoot,
		headBlock: headBlock.Block(),
	})
	s.cfg.ForkChoiceStore.RUnlock()
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
