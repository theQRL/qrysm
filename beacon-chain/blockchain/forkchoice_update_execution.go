package blockchain

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
)

func (s *Service) isNewProposer(slot primitives.Slot) bool {
	_, _, ok := s.cfg.ProposerSlotIndexCache.GetProposerPayloadIDs(slot, [32]byte{} /* root */)
	return ok || features.Get().PrepareAllPayloads
}

func (s *Service) isNewHead(r [32]byte) bool {
	s.headLock.RLock()
	defer s.headLock.RUnlock()

	currentHeadRoot := s.originBlockRoot
	if s.head != nil {
		currentHeadRoot = s.headRoot()
	}

	return r != currentHeadRoot || r == [32]byte{}
}

func (s *Service) getStateAndBlock(ctx context.Context, r [32]byte) (state.BeaconState, interfaces.ReadOnlySignedBeaconBlock, error) {
	if !s.hasBlockInInitSyncOrDB(ctx, r) {
		return nil, nil, errors.New("block does not exist")
	}
	newHeadBlock, err := s.getBlock(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	headState, err := s.cfg.StateGen.StateByRoot(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	return headState, newHeadBlock, nil
}

type executionForkchoice struct {
	headRoot      [32]byte
	safeHash      [32]byte
	finalizedHash [32]byte
}

// InvalidateForkchoiceUpdate forgets the engine's last acknowledged forkchoice.
// External FCU callers must call this before and after their RPC, including on
// error: an in-flight request can change execution after a chain-service FCU has
// completed. The next head update must then reconcile the selected head again.
// The caller must not hold the forkchoice lock.
func (s *Service) InvalidateForkchoiceUpdate() {
	s.cfg.ForkChoiceStore.Lock()
	defer s.cfg.ForkChoiceStore.Unlock()
	s.lastForkchoiceUpdate = nil
}

// executionForkchoiceState snapshots the execution update under the caller's
// forkchoice lock. A beacon head root also identifies its execution payload.
func (s *Service) executionForkchoiceState(headRoot [32]byte) executionForkchoice {
	return executionForkchoice{
		headRoot:      headRoot,
		safeHash:      s.cfg.ForkChoiceStore.UnrealizedJustifiedPayloadBlockHash(),
		finalizedHash: s.cfg.ForkChoiceStore.FinalizedPayloadBlockHash(),
	}
}

// forkchoiceUpdateWithExecution notifies execution when the head or checkpoint
// hashes change, or their validation is still pending. It returns true if the
// beacon head is updated. The caller must hold the forkchoice write lock.
func (s *Service) forkchoiceUpdateWithExecution(ctx context.Context, newHeadRoot [32]byte, proposingSlot primitives.Slot) (bool, error) {
	_, span := trace.StartSpan(ctx, "beacon-chain.blockchain.forkchoiceUpdateWithExecution")
	defer span.End()
	// Note: Use the service context here to avoid the parent context being ended during a forkchoice update.
	ctx = trace.NewContext(s.ctx, span)

	isNewHead := s.isNewHead(newHeadRoot)
	update := s.executionForkchoiceState(newHeadRoot)
	if !isNewHead && s.lastForkchoiceUpdate != nil && *s.lastForkchoiceUpdate == update {
		return false, nil
	}
	isNewProposer := s.isNewProposer(proposingSlot)
	if isNewHead && isNewProposer && !features.Get().DisableReorgLateBlocks {
		if s.shouldOverrideFCU(newHeadRoot, proposingSlot) {
			return false, nil
		}
	}
	headState, headBlock, err := s.getStateAndBlock(ctx, newHeadRoot)
	if err != nil {
		log.WithError(err).Error("Could not get forkchoice update argument")
		return false, nil
	}

	_, err = s.notifyForkchoiceUpdate(ctx, &notifyForkchoiceUpdateArg{
		headState: headState,
		headRoot:  newHeadRoot,
		headBlock: headBlock.Block(),
	})
	if err != nil {
		return false, errors.Wrap(err, "could not notify forkchoice update")
	}
	if !isNewHead {
		return false, nil
	}

	if err := s.saveHead(ctx, newHeadRoot, headBlock, headState); err != nil {
		return false, errors.Wrap(err, "could not save head")
	}

	// Only need to prune attestations from pool if the head has changed.
	if err := s.pruneAttsFromPool(headBlock); err != nil {
		log.WithError(err).Error("Could not prune attestations from pool")
	}
	return true, nil
}

// shouldOverrideFCU checks whether the incoming block is still subject to being
// reorged or not by the next proposer.
func (s *Service) shouldOverrideFCU(newHeadRoot [32]byte, proposingSlot primitives.Slot) bool {
	headWeight, err := s.cfg.ForkChoiceStore.Weight(newHeadRoot)
	if err != nil {
		log.WithError(err).WithField("root", fmt.Sprintf("%#x", newHeadRoot)).Warn("Could not determine node weight")
	}
	currentSlot := s.CurrentSlot()
	if proposingSlot == currentSlot {
		proposerHead := s.cfg.ForkChoiceStore.GetProposerHead()
		if proposerHead != newHeadRoot {
			return true
		}
		log.WithFields(logrus.Fields{
			"root":   fmt.Sprintf("%#x", newHeadRoot),
			"weight": headWeight,
		}).Infof("Attempted late block reorg aborted due to attestations at %d seconds",
			params.BeaconConfig().SecondsPerSlot)
		lateBlockFailedAttemptSecondThreshold.Inc()
	} else {
		if s.cfg.ForkChoiceStore.ShouldOverrideFCU() {
			return true
		}
		secs, err := slots.SecondsSinceSlotStart(currentSlot,
			uint64(s.genesisTime.Unix()), uint64(time.Now().Unix()))
		if err != nil {
			log.WithError(err).Error("Could not compute seconds since slot start")
		}
		if secs >= doublylinkedtree.ProcessAttestationsThreshold {
			log.WithFields(logrus.Fields{
				"root":   fmt.Sprintf("%#x", newHeadRoot),
				"weight": headWeight,
			}).Infof("Attempted late block reorg aborted due to attestations at %d seconds",
				doublylinkedtree.ProcessAttestationsThreshold)
			lateBlockFailedAttemptFirstThreshold.Inc()
		}
	}
	return false
}
