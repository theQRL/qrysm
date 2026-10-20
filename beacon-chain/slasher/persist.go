package slasher

import (
	"context"
	"time"

	"github.com/theQRL/qrysm/consensus-types/primitives"
)

// persistLatestEpochWritten flushes the latest epoch written per validator to
// disk once per epoch. The min/max span chunks are written as attestations are
// processed, but the epoch bookkeeping they depend on used to be saved only on a
// clean shutdown: after a crash the next start would reset every span written
// since the previous clean stop to the neutral element, blinding surround-vote
// detection for that window.
func (s *Service) persistLatestEpochWritten(ctx context.Context, currentEpoch primitives.Epoch) {
	if currentEpoch <= s.lastEpochWrittenPersisted {
		return
	}
	start := time.Now()
	if err := s.serviceCfg.Database.SaveLastEpochsWrittenForValidators(ctx, s.latestEpochWrittenForValidator); err != nil {
		log.WithError(err).Error("Could not persist the latest epoch written per validator")
		return
	}
	s.lastEpochWrittenPersisted = currentEpoch
	log.WithField("elapsed", time.Since(start)).WithField("epoch", currentEpoch).
		Debug("Persisted the latest epoch written per validator")
}
