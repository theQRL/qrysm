package blockchain

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	"github.com/theQRL/qrysm/beacon-chain/state"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
)

// UpdateAndSaveHeadWithBalances updates the beacon state head after getting justified balanced from cache.
// This function is only used in spec-tests, it does save the head after updating it.
func (s *Service) UpdateAndSaveHeadWithBalances(ctx context.Context) error {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	headRoot, err := s.cfg.ForkChoiceStore.Head(ctx)
	if err != nil {
		return errors.Wrap(err, "could not update head")
	}
	headBlock, err := s.getBlock(ctx, headRoot)
	if err != nil {
		return err
	}
	headState, err := s.cfg.StateGen.StateByRoot(ctx, headRoot)
	if err != nil {
		return errors.Wrap(err, "could not retrieve head state in DB")
	}
	return s.saveHead(ctx, headRoot, headBlock, headState)
}

// This defines the current chain service's view of head.
type head struct {
	root       [32]byte                             // current head root.
	block      interfaces.ReadOnlySignedBeaconBlock // current head block.
	state      state.BeaconState                    // current head state.
	slot       primitives.Slot                      // the head block slot number
	optimistic bool                                 // optimistic status when saved head
}

// This saves head info to the local service cache, it also saves the
// new head root to the DB.
// Caller of the method MUST acquire a lock on forkchoice.
func (s *Service) saveHead(ctx context.Context, newHeadRoot [32]byte, headBlock interfaces.ReadOnlySignedBeaconBlock, headState state.BeaconState) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.saveHead")
	defer span.End()

	// Do nothing if head hasn't changed.
	if !s.isNewHead(newHeadRoot) {
		return nil
	}

	if err := blocks.BeaconBlockIsNil(headBlock); err != nil {
		return err
	}
	if headState == nil || headState.IsNil() {
		return errors.New("cannot save nil head state")
	}

	// If the head state is not available, just return nil.
	// There's nothing to cache
	if !s.cfg.BeaconDB.HasStateSummary(ctx, newHeadRoot) {
		return nil
	}

	s.headLock.RLock()
	oldHeadBlock, err := s.headBlock()
	if err != nil {
		s.headLock.RUnlock()
		return errors.Wrap(err, "could not get old head block")
	}
	oldStateRoot := oldHeadBlock.Block().StateRoot()
	s.headLock.RUnlock()
	headSlot := s.HeadSlot()
	newHeadSlot := headBlock.Block().Slot()
	newStateRoot := headBlock.Block().StateRoot()

	r, err := s.HeadRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get old head root")
	}
	oldHeadRoot := bytesutil.ToBytes32(r)
	isOptimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(newHeadRoot)
	if err != nil {
		log.WithError(err).Error("Could not check if node is optimistically synced")
	}
	var commonRoot [32]byte
	var forkSlot primitives.Slot
	isReorg := headBlock.Block().ParentRoot() != oldHeadRoot
	if isReorg {
		commonRoot, forkSlot, err = s.commonAncestorForReorg(ctx, oldHeadRoot, newHeadRoot)
		if err != nil {
			if !errors.Is(err, forkchoice.ErrUnknownCommonAncestor) {
				return errors.Wrap(err, "could not find common ancestor root")
			}
			log.WithError(err).Error("Could not find common ancestor root")
			commonRoot = params.BeaconConfig().ZeroHash
		} else {
			// Head publication can skip intermediate blocks. Advancing to a
			// descendant of the old head does not orphan any blocks.
			isReorg = commonRoot != oldHeadRoot
		}
	}
	// Prepare the snapshot before persistence, so publishing it cannot fail
	// after the durable head has changed.
	bCp, err := headBlock.Copy()
	if err != nil {
		return errors.Wrap(err, "could not copy head block")
	}
	newHead := &head{
		root:       newHeadRoot,
		block:      bCp,
		state:      headState.Copy(),
		optimistic: isOptimistic,
		slot:       newHeadSlot,
	}
	if isReorg {
		if err := s.saveOrphanedOperations(ctx, oldHeadRoot, newHeadRoot, newHead.state); err != nil {
			return err
		}
	}
	// Leave the published head and its retained ancestry intact on failure.
	// The next head update can retry even if execution already validated it.
	if err := s.cfg.BeaconDB.SaveHeadBlockRoot(ctx, newHeadRoot); err != nil {
		return errors.Wrap(err, "could not save head root in DB")
	}
	s.publishHead(newHead)
	s.invalidatedHeadBlocks = nil

	if isReorg {
		// A chain re-org occurred, so we fire an event notifying the rest of the services.
		dis := headSlot + newHeadSlot - 2*forkSlot
		dep := max(uint64(headSlot-forkSlot), uint64(newHeadSlot-forkSlot))
		oldWeight, err := s.cfg.ForkChoiceStore.Weight(oldHeadRoot)
		if err != nil {
			log.WithField("root", fmt.Sprintf("%#x", oldHeadRoot)).Warn("Could not determine node weight")
		}
		newWeight, err := s.cfg.ForkChoiceStore.Weight(newHeadRoot)
		if err != nil {
			log.WithField("root", fmt.Sprintf("%#x", newHeadRoot)).Warn("Could not determine node weight")
		}
		log.WithFields(logrus.Fields{
			"newSlot":            fmt.Sprintf("%d", newHeadSlot),
			"newRoot":            fmt.Sprintf("%#x", newHeadRoot),
			"newWeight":          newWeight,
			"oldSlot":            fmt.Sprintf("%d", headSlot),
			"oldRoot":            fmt.Sprintf("%#x", oldHeadRoot),
			"oldWeight":          oldWeight,
			"commonAncestorRoot": fmt.Sprintf("%#x", commonRoot),
			"distance":           dis,
			"depth":              dep,
		}).Info("Chain reorg occurred")
		reorgDistance.Observe(float64(dis))
		reorgDepth.Observe(float64(dep))

		s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
			Type: statefeed.Reorg,
			Data: &qrlpb.EventChainReorg{
				Slot:                newHeadSlot,
				Depth:               max(uint64(headSlot-forkSlot), uint64(newHeadSlot-forkSlot)),
				OldHeadBlock:        oldHeadRoot[:],
				NewHeadBlock:        newHeadRoot[:],
				OldHeadState:        oldStateRoot[:],
				NewHeadState:        newStateRoot[:],
				Epoch:               slots.ToEpoch(newHeadSlot),
				ExecutionOptimistic: isOptimistic,
			},
		})

		reorgCount.Inc()
	}

	// Keep the execution status with this head's snapshot. The service may
	// switch branches before the asynchronous notification runs.
	go func() {
		if err := s.notifyNewHeadEvent(ctx, newHeadSlot, headState, newStateRoot[:], newHeadRoot[:], isOptimistic); err != nil {
			log.WithError(err).Error("Could not notify event feed of new chain head")
		}
	}()

	return nil
}

// This gets called to update canonical root mapping. It does not save head block
// root in DB. With the inception of initial-sync-cache-state flag, it uses finalized
// check point as anchors to resume sync therefore head is no longer needed to be saved on per slot basis.
// The caller must hold the forkchoice write lock.
func (s *Service) saveHeadNoDB(ctx context.Context, b interfaces.ReadOnlySignedBeaconBlock, r [32]byte, hs state.BeaconState, optimistic bool) error {
	if err := blocks.BeaconBlockIsNil(b); err != nil {
		return err
	}
	cachedHeadRoot, err := s.HeadRoot(ctx)
	if err != nil {
		return errors.Wrap(err, "could not get head root from cache")
	}
	if bytes.Equal(r[:], cachedHeadRoot) {
		return nil
	}

	bCp, err := b.Copy()
	if err != nil {
		return err
	}
	// Batch imports can select a competing branch without execution
	// invalidation. Recover its orphaned operations before replacing the head.
	if b.Block().ParentRoot() != bytesutil.ToBytes32(cachedHeadRoot) {
		if err := s.saveOrphanedOperations(ctx, bytesutil.ToBytes32(cachedHeadRoot), r, hs); err != nil {
			return err
		}
	}
	if err := s.setHeadInitialSync(r, bCp, hs, optimistic); err != nil {
		return errors.Wrap(err, "could not set head")
	}
	s.invalidatedHeadBlocks = nil
	return nil
}

// refreshHeadOptimisticStatus updates the cached status even when the selected
// head has not changed. Validating a descendant also validates its ancestors.
// The caller must hold the forkchoice lock.
func (s *Service) refreshHeadOptimisticStatus() {
	s.headLock.Lock()
	defer s.headLock.Unlock()
	if s.head == nil || !s.cfg.ForkChoiceStore.HasNode(s.head.root) {
		return
	}
	optimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(s.head.root)
	if err != nil {
		log.WithError(err).Error("Could not get head optimistic status")
		return
	}
	s.head.optimistic = optimistic
}

// This sets head view object which is used to track the head slot, root, block, state and optimistic status
func (s *Service) setHead(newHead *head) error {
	// This does a full copy of the block and state.
	bCp, err := newHead.block.Copy()
	if err != nil {
		return err
	}
	s.publishHead(&head{
		root:       newHead.root,
		block:      bCp,
		state:      newHead.state.Copy(),
		optimistic: newHead.optimistic,
		slot:       newHead.slot,
	})
	return nil
}

// setHeadInitialSync snapshots the published head. StateGen can return the input
// state without a copy for the next batch, which may mutate it before validation
// fails. The cached head must retain the last successfully imported state.
func (s *Service) setHeadInitialSync(root [32]byte, block interfaces.ReadOnlySignedBeaconBlock, state state.BeaconState, optimistic bool) error {
	// Copy both the block and state to isolate the head from later batches.
	bCp, err := block.Copy()
	if err != nil {
		return err
	}
	s.publishHead(&head{
		root:       root,
		block:      bCp,
		state:      state.Copy(),
		optimistic: optimistic,
	})
	return nil
}

// publishHead invalidates attestation calculations before exposing a new head.
// Holding headLock across both operations prevents a reader from observing the
// replacement while the cache still belongs to its predecessor.
func (s *Service) publishHead(snapshot *head) {
	s.headLock.Lock()
	defer s.headLock.Unlock()
	if s.head == nil || s.head.root != snapshot.root {
		if s.cfg != nil && s.cfg.AttestationCache != nil {
			s.cfg.AttestationCache.Clear()
		}
	}
	s.head = snapshot
}

// This returns the head slot.
// This is a lock free version.
func (s *Service) headSlot() primitives.Slot {
	if s.head == nil || s.head.block == nil || s.head.block.Block() == nil {
		return 0
	}
	return s.head.block.Block().Slot()
}

// This returns the head root.
// It does a full copy on head root for immutability.
// This is a lock free version.
func (s *Service) headRoot() [32]byte {
	if s.head == nil {
		return params.BeaconConfig().ZeroHash
	}

	return s.head.root
}

// This returns the head block.
// It does a full copy on head block for immutability.
// This is a lock free version.
func (s *Service) headBlock() (interfaces.ReadOnlySignedBeaconBlock, error) {
	return s.head.block.Copy()
}

// This returns the head state.
// It does a full copy on head state for immutability.
// This is a lock free version.
func (s *Service) headState(ctx context.Context) state.BeaconState {
	_, span := trace.StartSpan(ctx, "blockChain.headState")
	defer span.End()

	return s.head.state.Copy()
}

// This returns a read only version of the head state.
// It does not perform a copy of the head state.
// This is a lock free version.
func (s *Service) headStateReadOnly(ctx context.Context) state.ReadOnlyBeaconState {
	_, span := trace.StartSpan(ctx, "blockChain.headStateReadOnly")
	defer span.End()

	return s.head.state
}

// This returns the genesis validators root of the head state.
// This is a lock free version.
func (s *Service) headGenesisValidatorsRoot() [32]byte {
	return bytesutil.ToBytes32(s.head.state.GenesisValidatorsRoot())
}

// This returns the validator referenced by the provided index in
// the head state.
// This is a lock free version.
func (s *Service) headValidatorAtIndex(index primitives.ValidatorIndex) (state.ReadOnlyValidator, error) {
	return s.head.state.ValidatorAtIndexReadOnly(index)
}

// This returns the validator index referenced by the provided pubkey in
// the head state.
// This is a lock free version.
func (s *Service) headValidatorIndexAtPubkey(pubKey [field_params.MLDSA87PubkeyLength]byte) (primitives.ValidatorIndex, bool) {
	return s.head.state.ValidatorIndexByPubkey(pubKey)
}

// Returns true if head state exists.
// This is the lock free version.
func (s *Service) hasHeadState() bool {
	return s.head != nil && s.head.state != nil
}

// Notifies a common event feed of a new chain head event. Called right after a new
// chain head is determined, set, and saved to disk.
func (s *Service) notifyNewHeadEvent(
	ctx context.Context,
	newHeadSlot primitives.Slot,
	newHeadState state.BeaconState,
	newHeadStateRoot,
	newHeadRoot []byte,
	isOptimistic bool,
) error {
	previousDutyDependentRoot := s.originBlockRoot[:]
	currentDutyDependentRoot := s.originBlockRoot[:]

	var previousDutyEpoch primitives.Epoch
	currentDutyEpoch := slots.ToEpoch(newHeadSlot)
	if currentDutyEpoch > 0 {
		previousDutyEpoch = currentDutyEpoch.Sub(1)
	}
	currentDutySlot, err := slots.EpochStart(currentDutyEpoch)
	if err != nil {
		return errors.Wrap(err, "could not get duty slot")
	}
	previousDutySlot, err := slots.EpochStart(previousDutyEpoch)
	if err != nil {
		return errors.Wrap(err, "could not get duty slot")
	}
	if currentDutySlot > 0 {
		currentDutyDependentRoot, err = helpers.BlockRootAtSlot(newHeadState, currentDutySlot-1)
		if err != nil {
			return errors.Wrap(err, "could not get duty dependent root")
		}
		if bytes.Equal(currentDutyDependentRoot, params.BeaconConfig().ZeroHash[:]) {
			currentDutyDependentRoot = s.originBlockRoot[:]
		}
	}
	if previousDutySlot > 0 {
		previousDutyDependentRoot, err = helpers.BlockRootAtSlot(newHeadState, previousDutySlot-1)
		if err != nil {
			return errors.Wrap(err, "could not get duty dependent root")
		}
		if bytes.Equal(previousDutyDependentRoot, params.BeaconConfig().ZeroHash[:]) {
			previousDutyDependentRoot = s.originBlockRoot[:]
		}
	}
	parentRoot := bytesutil.ToBytes32(newHeadState.LatestBlockHeader().ParentRoot)
	epochTransition, err := s.headEpochTransition(newHeadSlot, newHeadState, parentRoot)
	if err != nil {
		return err
	}

	s.cfg.StateNotifier.StateFeed().Send(&feed.Event{
		Type: statefeed.NewHead,
		Data: &qrlpb.EventHead{
			Slot:                      newHeadSlot,
			Block:                     newHeadRoot,
			State:                     newHeadStateRoot,
			EpochTransition:           epochTransition,
			PreviousDutyDependentRoot: previousDutyDependentRoot,
			CurrentDutyDependentRoot:  currentDutyDependentRoot,
			ExecutionOptimistic:       isOptimistic,
		},
	})
	return nil
}

// headEpochTransition reports whether the head at newHeadSlot crossed an epoch
// boundary relative to its parent block, i.e. whether the parent sits in a
// strictly earlier epoch. Comparing parent vs head epochs (rather than checking
// IsEpochStart on the head slot) correctly handles skipped epoch-boundary
// slots, where the head lands several slots into the new epoch.
func (s *Service) headEpochTransition(newHeadSlot primitives.Slot, newHeadState state.ReadOnlyBeaconState, parentRoot [32]byte) (bool, error) {
	// The genesis block has no parent; consider it an epoch transition.
	if newHeadSlot == 0 {
		return true, nil
	}
	parentSlot, err := s.RecentBlockSlot(parentRoot)
	if err == nil {
		return slots.ToEpoch(newHeadSlot) > slots.ToEpoch(parentSlot), nil
	}
	// The parent is not in forkchoice. This happens when the head is the
	// forkchoice tree root itself (the finalized or checkpoint-sync origin
	// block, e.g. after reverting to it because its descendants were INVALID),
	// whose parent was pruned at finalization. Rather than dropping the head
	// event, fall back to the head state: every slot between the parent and
	// the head is empty, so the parent sits in an earlier epoch iff its root
	// is the block root recorded at the last slot of the previous epoch.
	epochStart, err := slots.EpochStart(slots.ToEpoch(newHeadSlot))
	if err != nil {
		return false, errors.Wrap(err, "could not get epoch start slot")
	}
	if epochStart == 0 {
		// Both the head and its parent are in epoch 0.
		return false, nil
	}
	root, err := helpers.BlockRootAtSlot(newHeadState, epochStart-1)
	if err != nil {
		return false, errors.Wrap(err, "could not obtain parent slot in forkchoice or head state")
	}
	return bytes.Equal(root, parentRoot[:]), nil
}

// saveOrphanedOperations recovers operations from the old branch, validating
// slashings against the replacement state. The caller holds the forkchoice lock.
// Attestations older than their inclusion window are discarded.
func (s *Service) saveOrphanedOperations(ctx context.Context, orphanedRoot [32]byte, newHeadRoot [32]byte, newHeadState state.ReadOnlyBeaconState) error {
	commonAncestorRoot, _, err := s.commonAncestorForReorg(ctx, orphanedRoot, newHeadRoot)
	switch {
	// Exit early if there's no common ancestor and root doesn't exist, there would be nothing to save.
	case errors.Is(err, forkchoice.ErrUnknownCommonAncestor):
		return nil
	case err != nil:
		return err
	}
	for orphanedRoot != commonAncestorRoot {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		orphanedBlk, invalidated := s.invalidatedHeadBlocks[orphanedRoot]
		if !invalidated {
			orphanedBlk, err = s.getBlock(ctx, orphanedRoot)
			if err != nil {
				return err
			}
		}
		// Slashings and exits can remain valid after the attestation inclusion
		// window expires. Walk the whole orphaned branch, filtering only atts.
		for _, a := range orphanedBlk.Block().Body().Attestations() {
			if helpers.ValidateNilAttestation(a) != nil {
				continue
			}
			// Execution-invalid blocks can contain reusable votes, but votes
			// for the removed branch must not return to the attestation pool.
			if invalidated && s.verifyAttestationForkchoice(a) != nil {
				continue
			}
			// if the attestation is one epoch older, it wouldn't been useful to save it.
			if a.Data.Slot+params.BeaconConfig().SlotsPerEpoch < s.CurrentSlot() {
				continue
			}
			// The orphaned block only proves the vote valid in its own state.
			// The pool it joins gates gossip and proposals on every entry
			// having been authenticated in the voted target state, so verify
			// it here like any other entry. Committees are fixed two epochs
			// ahead, so this is also what makes it includable on the new
			// branch. The age check above bounds this work to one epoch.
			targetState, err := s.getAttPreState(ctx, a.Data.Target)
			if err != nil {
				if errors.Is(err, ErrNotCheckpoint) {
					continue
				}
				// Keep the old head and its ancestry until recovery can finish.
				// Publishing the replacement would prevent later head updates
				// from retrying this attestation after a temporary read failure.
				return errors.Wrap(err, "could not get target state of orphaned attestation")
			}
			if _, err := verifiedAttestingIndices(ctx, targetState, a); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				continue
			}
			if err := s.cfg.AttPool.RecoverAttestation(a); err != nil {
				return err
			}
			saveOrphanedAttCount.Inc()
		}
		for _, as := range orphanedBlk.Block().Body().AttesterSlashings() {
			if err := s.cfg.SlashingPool.RecoverAttesterSlashing(ctx, newHeadState, as); err != nil {
				log.WithError(err).Error("Could not insert reorg attester slashing")
			}
		}
		for _, vs := range orphanedBlk.Block().Body().ProposerSlashings() {
			if err := s.cfg.SlashingPool.RecoverProposerSlashing(ctx, newHeadState, vs); err != nil {
				log.WithError(err).Error("Could not insert reorg proposer slashing")
			}
		}
		for _, v := range orphanedBlk.Block().Body().VoluntaryExits() {
			s.cfg.ExitPool.InsertVoluntaryExit(v)
		}
		parentRoot := orphanedBlk.Block().ParentRoot()
		orphanedRoot = bytesutil.ToBytes32(parentRoot[:])
	}
	return nil
}
