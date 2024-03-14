package monitor

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation"
)

// canUpdateAttestedValidator returns true if the validator is tracked and if the
// given slot is different than the last attested slot from this validator.
// It assumes that a read lock is held on the monitor service.
func (s *Service) canUpdateAttestedValidator(idx primitives.ValidatorIndex, slot primitives.Slot) bool {
	if !s.trackedIndex(idx) {
		return false
	}

	if lp, ok := s.latestPerformance[idx]; ok {
		return lp.attestedSlot != slot
	}
	return false
}

// attestingIndices returns the indices of validators that participated in the given aggregated attestation.
func attestingIndices(ctx context.Context, state state.BeaconState, att *zondpb.Attestation) ([]uint64, error) {
	committee, err := helpers.BeaconCommitteeFromState(ctx, state, att.Data.Slot, att.Data.CommitteeIndex)
	if err != nil {
		return nil, err
	}
	return attestation.AttestingIndices(att.AggregationBits, committee)
}

// logMessageTimelyFlagsForIndex returns the log message with performance info for the attestation (head, source, target)
func logMessageTimelyFlagsForIndex(idx primitives.ValidatorIndex, data *zondpb.AttestationData) logrus.Fields {
	return logrus.Fields{
		"ValidatorIndex": idx,
		"Slot":           data.Slot,
		"Source":         fmt.Sprintf("%#x", bytesutil.Trunc(data.Source.Root)),
		"Target":         fmt.Sprintf("%#x", bytesutil.Trunc(data.Target.Root)),
		"Head":           fmt.Sprintf("%#x", bytesutil.Trunc(data.BeaconBlockRoot)),
	}
}

// processAttestations logs the event for the tracked validators' attestations inclusion in block
func (s *Service) processAttestations(ctx context.Context, state state.BeaconState, blk interfaces.ReadOnlyBeaconBlock) {
	if blk == nil || blk.Body() == nil {
		return
	}
	for _, att := range blk.Body().Attestations() {
		s.processIncludedAttestation(ctx, state, att)
	}
}

// processIncludedAttestation logs in the event for the tracked validators' and their latest attestation gets processed.
func (s *Service) processIncludedAttestation(ctx context.Context, state state.BeaconState, att *zondpb.Attestation) {
	attestingIndices, err := attestingIndices(ctx, state, att)
	if err != nil {
		log.WithError(err).Error("Could not get attesting indices")
		return
	}
	s.Lock()
	defer s.Unlock()
	for _, idx := range attestingIndices {
		if s.canUpdateAttestedValidator(primitives.ValidatorIndex(idx), att.Data.Slot) {
			logFields := logMessageTimelyFlagsForIndex(primitives.ValidatorIndex(idx), att.Data)
			balance, err := state.BalanceAtIndex(primitives.ValidatorIndex(idx))
			if err != nil {
				log.WithError(err).Error("Could not get balance")
				return
			}

			aggregatedPerf := s.aggregatedPerformance[primitives.ValidatorIndex(idx)]
			aggregatedPerf.totalAttestedCount++
			aggregatedPerf.totalRequestedCount++

			latestPerf := s.latestPerformance[primitives.ValidatorIndex(idx)]
			balanceChg := int64(balance - latestPerf.balance)
			latestPerf.balanceChange = balanceChg
			latestPerf.balance = balance
			latestPerf.attestedSlot = att.Data.Slot
			latestPerf.inclusionSlot = state.Slot()
			inclusionSlotGauge.WithLabelValues(fmt.Sprintf("%d", idx)).Set(float64(latestPerf.inclusionSlot))
			aggregatedPerf.totalDistance += uint64(latestPerf.inclusionSlot - latestPerf.attestedSlot)

			logFields["CorrectHead"] = latestPerf.timelyHead
			logFields["CorrectSource"] = latestPerf.timelySource
			logFields["CorrectTarget"] = latestPerf.timelyTarget
			logFields["InclusionSlot"] = latestPerf.inclusionSlot
			logFields["NewBalance"] = balance
			logFields["BalanceChange"] = balanceChg

			s.latestPerformance[primitives.ValidatorIndex(idx)] = latestPerf
			s.aggregatedPerformance[primitives.ValidatorIndex(idx)] = aggregatedPerf
			log.WithFields(logFields).Info("Attestation included")
		}
	}
}

// processUnaggregatedAttestation logs when the beacon node observes an unaggregated attestation from tracked validator.
func (s *Service) processUnaggregatedAttestation(ctx context.Context, att *zondpb.Attestation) {
	s.RLock()
	defer s.RUnlock()
	root := bytesutil.ToBytes32(att.Data.BeaconBlockRoot)
	st := s.config.StateGen.StateByRootIfCachedNoCopy(root)
	if st == nil {
		log.WithField("BeaconBlockRoot", fmt.Sprintf("%#x", bytesutil.Trunc(root[:]))).Debug(
			"Skipping unaggregated attestation due to state not found in cache")
		return
	}
	attestingIndices, err := attestingIndices(ctx, st, att)
	if err != nil {
		log.WithError(err).Error("Could not get attesting indices")
		return
	}
	for _, idx := range attestingIndices {
		if s.canUpdateAttestedValidator(primitives.ValidatorIndex(idx), att.Data.Slot) {
			logFields := logMessageTimelyFlagsForIndex(primitives.ValidatorIndex(idx), att.Data)
			log.WithFields(logFields).Info("Processed unaggregated attestation")
		}
	}
}

// processUnaggregatedAttestation logs when the beacon node observes an aggregated attestation from tracked validator.
func (s *Service) processAggregatedAttestation(ctx context.Context, att *zondpb.AggregateAttestationAndProof) {
	s.Lock()
	defer s.Unlock()
	if s.trackedIndex(att.AggregatorIndex) {
		log.WithFields(logrus.Fields{
			"AggregatorIndex": att.AggregatorIndex,
			"Slot":            att.Aggregate.Data.Slot,
			"BeaconBlockRoot": fmt.Sprintf("%#x", bytesutil.Trunc(
				att.Aggregate.Data.BeaconBlockRoot)),
			"SourceRoot": fmt.Sprintf("%#x", bytesutil.Trunc(
				att.Aggregate.Data.Source.Root)),
			"TargetRoot": fmt.Sprintf("%#x", bytesutil.Trunc(
				att.Aggregate.Data.Target.Root)),
		}).Info("Processed attestation aggregation")
		aggregatedPerf := s.aggregatedPerformance[att.AggregatorIndex]
		aggregatedPerf.totalAggregations++
		s.aggregatedPerformance[att.AggregatorIndex] = aggregatedPerf
		aggregationCounter.WithLabelValues(fmt.Sprintf("%d", att.AggregatorIndex)).Inc()
	}

	var root [32]byte
	copy(root[:], att.Aggregate.Data.BeaconBlockRoot)
	st := s.config.StateGen.StateByRootIfCachedNoCopy(root)
	if st == nil {
		log.WithField("BeaconBlockRoot", fmt.Sprintf("%#x", bytesutil.Trunc(root[:]))).Debug(
			"Skipping aggregated attestation due to state not found in cache")
		return
	}
	attestingIndices, err := attestingIndices(ctx, st, att.Aggregate)
	if err != nil {
		log.WithError(err).Error("Could not get attesting indices")
		return
	}
	for _, idx := range attestingIndices {
		if s.canUpdateAttestedValidator(primitives.ValidatorIndex(idx), att.Aggregate.Data.Slot) {
			logFields := logMessageTimelyFlagsForIndex(primitives.ValidatorIndex(idx), att.Aggregate.Data)
			log.WithFields(logFields).Info("Processed aggregated attestation")
		}
	}
}
