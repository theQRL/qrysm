package sync

import (
	"bytes"
	"context"
	"encoding/hex"
	"slices"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/async"
	"github.com/theQRL/qrysm/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/rand"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
)

// This defines how often a node cleans up and processes pending attestations in the queue.
var processPendingAttsPeriod = slots.DivideSlotBy(2 /* twice per slot */)
var pendingAttsLimit = 10000

// aggregatorIndexFilter defines how aggregator index should be handled in equality checks.
type aggregatorIndexFilter int

const (
	// ignoreAggregatorIndex means aggregates differing only by aggregator index are considered equal.
	ignoreAggregatorIndex aggregatorIndexFilter = iota
	// includeAggregatorIndex means aggregator index must also match for aggregates to be considered equal.
	includeAggregatorIndex
)

// This processes pending attestation queues on every `processPendingAttsPeriod`.
func (s *Service) processPendingAttsQueue() {
	// Prevents multiple queue processing goroutines (invoked by RunEvery) from contending for data.
	mutex := new(sync.Mutex)
	async.RunEvery(s.ctx, processPendingAttsPeriod, func() {
		mutex.Lock()
		if err := s.processPendingAtts(s.ctx); err != nil {
			log.WithError(err).Debugf("Could not process pending attestation: %v", err)
		}
		mutex.Unlock()
	})
}

// This defines how pending attestations are processed. It contains features:
// 1. Clean up invalid pending attestations from the queue.
// 2. Check if pending attestations can be processed when the block has arrived.
// 3. Request block from a random peer if unable to proceed step 2.
func (s *Service) processPendingAtts(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "processPendingAtts")
	defer span.End()

	// Before a node processes pending attestations queue, it verifies
	// the attestations in the queue are still valid. Attestations will
	// be deleted from the queue if invalid (ie. getting staled from falling too many slots behind).
	s.validatePendingAtts(ctx, s.cfg.clock.CurrentSlot())

	s.pendingAttsLock.RLock()
	roots := make([][32]byte, 0, len(s.blkRootToPendingAtts))
	for br := range s.blkRootToPendingAtts {
		roots = append(roots, br)
	}
	s.pendingAttsLock.RUnlock()

	var pendingRoots [][32]byte
	randGen := rand.NewGenerator()
	for _, bRoot := range roots {
		// has the pending attestation's missing block arrived and the node processed block yet?
		// Also require the block to still be in fork choice — attestations for blocks pruned
		// out of forkchoice can never enter the pool, so decoding/sig-verifying them is wasted work.
		if s.cfg.beaconDB.HasBlock(ctx, bRoot) &&
			(s.cfg.beaconDB.HasState(ctx, bRoot) || s.cfg.beaconDB.HasStateSummary(ctx, bRoot)) &&
			s.cfg.chain.InForkchoice(bRoot) {
			if err := s.processPendingAttsForBlock(ctx, bRoot); err != nil {
				log.WithError(err).Debug("Failed to process pending attestations for block")
			}
		} else {
			s.pendingAttsLock.RLock()
			attestations := s.blkRootToPendingAtts[bRoot]
			s.pendingAttsLock.RUnlock()
			s.pendingQueueLock.RLock()
			seen := s.seenPendingBlocks[bRoot]
			s.pendingQueueLock.RUnlock()
			if !seen && len(attestations) > 0 {
				// Pending attestation's missing block has not arrived yet.
				log.WithFields(logrus.Fields{
					"currentSlot": s.cfg.clock.CurrentSlot(),
					"attSlot":     attestations[0].Message.Aggregate.Data.Slot,
					"attCount":    len(attestations),
					"blockRoot":   hex.EncodeToString(bytesutil.Trunc(bRoot[:])),
				}).Debug("Requesting block for pending attestation")
				pendingRoots = append(pendingRoots, bRoot)
			}
		}
	}
	return s.sendBatchRootRequest(ctx, pendingRoots, randGen)
}

// processPendingAttsForBlock drains the pending-attestation queue for a single
// block root. Called both from the periodic processPendingAtts ticker and from
// pending_blocks_queue.go immediately after a pending block lands in the DB,
// so attestations referencing that block don't have to wait for the next tick.
// The caller must have already verified the block (and state) is in the DB.
func (s *Service) processPendingAttsForBlock(ctx context.Context, blkRoot [32]byte) error {
	s.pendingAttsLock.RLock()
	attestations := s.blkRootToPendingAtts[blkRoot]
	s.pendingAttsLock.RUnlock()

	if len(attestations) == 0 {
		return nil
	}

	s.processAttestations(ctx, attestations)
	log.WithFields(logrus.Fields{
		"blockRoot":        hex.EncodeToString(bytesutil.Trunc(blkRoot[:])),
		"pendingAttsCount": len(attestations),
	}).Debug("Verified and saved pending attestations to pool")

	// Delete the missing block root key from pending attestation queue so a node
	// will not request the block again.
	s.pendingAttsLock.Lock()
	delete(s.blkRootToPendingAtts, blkRoot)
	s.pendingAttsLock.Unlock()
	return nil
}

func (s *Service) processAttestations(ctx context.Context, attestations []*qrysmpb.SignedAggregateAttestationAndProof) {
	validAggregates := make([]*qrysmpb.SignedAggregateAttestationAndProof, 0, len(attestations))
	for _, signedAtt := range attestations {
		att := signedAtt.Message
		// The pending attestations can arrive in both aggregated and unaggregated forms,
		// each from has distinct validation steps.
		if helpers.IsAggregated(att.Aggregate) {
			// Avoid processing multiple aggregates only differing by aggregator index;
			// validating and broadcasting more than one would be wasted work.
			if slices.ContainsFunc(validAggregates, func(other *qrysmpb.SignedAggregateAttestationAndProof) bool {
				return pendingAggregatesAreEqual(signedAtt, other, ignoreAggregatorIndex)
			}) {
				continue
			}
			// Skip if we've already processed an aggregate from this aggregator
			// in this target epoch — avoids redundant validation and broadcast.
			if s.hasSeenAggregatorIndexEpoch(att.Aggregate.Data.Target.Epoch, att.AggregatorIndex) {
				continue
			}
			// Save the pending aggregated attestation to the pool if it passes the aggregated
			// validation steps.
			valRes, err := s.validateAggregatedAtt(ctx, signedAtt)
			if err != nil {
				log.WithError(err).Debug("Pending aggregated attestation failed validation")
			}
			aggValid := pubsub.ValidationAccept == valRes
			if s.validateBlockInAttestation(ctx, signedAtt) && aggValid {
				if err := s.cfg.attPool.SaveAggregatedAttestation(att.Aggregate); err != nil {
					log.WithError(err).Debug("Could not save aggregate attestation")
					continue
				}
				if first := s.setAggregatorIndexEpochSeen(att.Aggregate.Data.Target.Epoch, att.AggregatorIndex); !first {
					continue
				}

				// Broadcasting the signed attestation again once a node is able to process it.
				if err := s.cfg.p2p.Broadcast(ctx, signedAtt); err != nil {
					log.WithError(err).Debug("Could not broadcast")
				}
				validAggregates = append(validAggregates, signedAtt)
			}
		} else {
			// This is an important validation before retrieving attestation pre state to defend against
			// attestation's target intentionally reference checkpoint that's long ago.
			// Verify current finalized checkpoint is an ancestor of the block defined by the attestation's beacon block root.
			if !s.cfg.chain.InForkchoice(bytesutil.ToBytes32(att.Aggregate.Data.BeaconBlockRoot)) {
				log.WithError(blockchain.ErrNotDescendantOfFinalized).Debug("Could not verify finalized consistency")
				continue
			}
			if err := s.cfg.chain.VerifyLmdFfgConsistency(ctx, att.Aggregate); err != nil {
				log.WithError(err).Debug("Could not verify FFG consistency")
				continue
			}
			preState, err := s.cfg.chain.AttestationTargetState(ctx, att.Aggregate.Data.Target)
			if err != nil {
				log.WithError(err).Debug("Could not retrieve attestation prestate")
				continue
			}

			valid, err := s.validateUnaggregatedAttWithState(ctx, att.Aggregate, preState)
			if err != nil {
				log.WithError(err).Debug("Pending unaggregated attestation failed validation")
				continue
			}
			if valid == pubsub.ValidationAccept {
				if err := s.cfg.attPool.SaveUnaggregatedAttestation(att.Aggregate); err != nil {
					log.WithError(err).Debug("Could not save unaggregated attestation")
					continue
				}
				_ = s.setSeenCommitteeIndicesSlot(att.Aggregate.Data.Slot, att.Aggregate.Data.CommitteeIndex, att.Aggregate.AggregationBits)

				valCount, err := helpers.ActiveValidatorCount(ctx, preState, slots.ToEpoch(att.Aggregate.Data.Slot))
				if err != nil {
					log.WithError(err).Debug("Could not retrieve active validator count")
					continue
				}
				// Broadcasting the signed attestation again once a node is able to process it.
				if err := s.cfg.p2p.BroadcastAttestation(ctx, helpers.ComputeSubnetForAttestation(valCount, signedAtt.Message.Aggregate), signedAtt.Message.Aggregate); err != nil {
					log.WithError(err).Debug("Could not broadcast")
				}
			}
		}
	}
}

// This defines how pending attestations is saved in the map. The key is the
// root of the missing block. The value is the list of pending attestations
// that voted for that block root.
func (s *Service) savePendingAtt(att *qrysmpb.SignedAggregateAttestationAndProof) {
	root := bytesutil.ToBytes32(att.Message.Aggregate.Data.BeaconBlockRoot)

	s.pendingAttsLock.Lock()
	defer s.pendingAttsLock.Unlock()

	numOfPendingAtts := 0
	for _, v := range s.blkRootToPendingAtts {
		numOfPendingAtts += len(v)
	}
	// Exit early if we exceed the pending attestations limit.
	if numOfPendingAtts >= pendingAttsLimit {
		return
	}

	_, ok := s.blkRootToPendingAtts[root]
	if !ok {
		s.blkRootToPendingAtts[root] = []*qrysmpb.SignedAggregateAttestationAndProof{att}
		return
	}

	// Skip if the attestation from the same aggregator already exists in
	// the pending queue.
	for _, a := range s.blkRootToPendingAtts[root] {
		if pendingAggregatesAreEqual(att, a, includeAggregatorIndex) {
			return
		}
	}
	s.blkRootToPendingAtts[root] = append(s.blkRootToPendingAtts[root], att)
}

// pendingAggregatesAreEqual checks if two pending aggregate attestations are equal.
// The filter parameter controls whether aggregator index is considered in the equality check.
func pendingAggregatesAreEqual(a, b *qrysmpb.SignedAggregateAttestationAndProof, filter aggregatorIndexFilter) bool {
	if filter == includeAggregatorIndex {
		if a.Signature != nil {
			return b.Signature != nil && a.Message.AggregatorIndex == b.Message.AggregatorIndex
		}
		if b.Signature != nil {
			return false
		}
	}
	if a.Message.Aggregate.Data.Slot != b.Message.Aggregate.Data.Slot {
		return false
	}
	if a.Message.Aggregate.Data.CommitteeIndex != b.Message.Aggregate.Data.CommitteeIndex {
		return false
	}
	return bytes.Equal(a.Message.Aggregate.AggregationBits, b.Message.Aggregate.AggregationBits)
}

// This validates the pending attestations in the queue are still valid.
// If not valid, a node will remove it in the queue in place. The validity
// check specifies the pending attestation could not fall one epoch behind
// of the current slot.
func (s *Service) validatePendingAtts(ctx context.Context, slot primitives.Slot) {
	_, span := trace.StartSpan(ctx, "validatePendingAtts")
	defer span.End()

	s.pendingAttsLock.Lock()
	defer s.pendingAttsLock.Unlock()

	for bRoot, atts := range s.blkRootToPendingAtts {
		for i := len(atts) - 1; i >= 0; i-- {
			if slot >= atts[i].Message.Aggregate.Data.Slot+params.BeaconConfig().SlotsPerEpoch {
				// Remove the pending attestation from the list in place.
				atts = append(atts[:i], atts[i+1:]...)
			}
		}
		s.blkRootToPendingAtts[bRoot] = atts

		// If the pending attestations list of a given block root is empty,
		// a node will remove the key from the map to avoid dangling keys.
		if len(s.blkRootToPendingAtts[bRoot]) == 0 {
			delete(s.blkRootToPendingAtts, bRoot)
		}
	}
}
