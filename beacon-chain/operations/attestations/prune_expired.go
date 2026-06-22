package attestations

import (
	"time"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmTime "github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/time/slots"
)

// pruneAttsPool prunes attestations pool on every slot, firing near the end of
// the slot so the deletes don't race with slot-boundary metric updates.
func (s *Service) pruneAttsPool() {
	secondsPerSlot := params.BeaconConfig().SecondsPerSlot
	offset := time.Duration(secondsPerSlot-1) * time.Second
	slotTicker := slots.NewSlotTickerWithOffset(time.Unix(int64(s.genesisTime), 0), offset, secondsPerSlot)
	defer slotTicker.Done()
	for {
		select {
		case <-slotTicker.C():
			s.pruneExpiredAtts()
			s.updateMetrics()
		case <-s.ctx.Done():
			log.Debug("Context closed, exiting routine")
			return
		}
	}
}

// This prunes expired attestations from the pool.
func (s *Service) pruneExpiredAtts() {
	aggregatedAtts := s.cfg.Pool.AggregatedAttestations()
	for _, att := range aggregatedAtts {
		if s.expired(att.Data.Slot) {
			if err := s.cfg.Pool.DeleteAggregatedAttestation(att); err != nil {
				log.WithError(err).Error("Could not delete expired aggregated attestation")
			}
			expiredAggregatedAtts.Inc()
		}
	}

	if _, err := s.cfg.Pool.DeleteSeenUnaggregatedAttestations(); err != nil {
		log.WithError(err).Error("Cannot delete seen attestations")
	}
	unAggregatedAtts, err := s.cfg.Pool.UnaggregatedAttestations()
	if err != nil {
		log.WithError(err).Error("Could not get unaggregated attestations")
		return
	}
	for _, att := range unAggregatedAtts {
		if s.expired(att.Data.Slot) {
			if err := s.cfg.Pool.DeleteUnaggregatedAttestation(att); err != nil {
				log.WithError(err).Error("Could not delete expired unaggregated attestation")
			}
			expiredUnaggregatedAtts.Inc()
		}
	}

	blockAtts := s.cfg.Pool.BlockAttestations()
	for _, att := range blockAtts {
		if s.expired(att.Data.Slot) {
			if err := s.cfg.Pool.DeleteBlockAttestation(att); err != nil {
				log.WithError(err).Error("Could not delete expired block attestation")
			}
			expiredBlockAtts.Inc()
		}
	}
}

// Return true if the input slot has been expired.
// Post EIP-7045, attestations from the previous epoch can still be included
// in the current epoch's blocks, so the inclusion window is two epochs.
func (s *Service) expired(slot primitives.Slot) bool {
	expirationSlot := slot + params.BeaconConfig().SlotsPerEpoch*2
	expirationTime := s.genesisTime + uint64(expirationSlot.Mul(params.BeaconConfig().SecondsPerSlot))
	currentTime := uint64(qrysmTime.Now().Unix())
	return currentTime >= expirationTime
}
