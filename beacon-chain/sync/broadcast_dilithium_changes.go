package sync

import (
	"context"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	types "github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/rand"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

const broadcastDilithiumChangesRateLimit = 128

// This routine broadcasts known Dilithium changes at the Capella fork.
func (s *Service) broadcastDilithiumChanges(currSlot types.Slot) {
	capellaSlotStart := primitives.Slot(0)
	if currSlot != capellaSlotStart {
		return
	}
	changes, err := s.cfg.dilithiumToExecPool.PendingDilithiumToExecChanges()
	if err != nil {
		log.WithError(err).Error("could not get Dilithium to execution changes")
	}
	if len(changes) == 0 {
		return
	}
	source := rand.NewGenerator()
	length := len(changes)
	broadcastChanges := make([]*zondpb.SignedDilithiumToExecutionChange, length)
	for i := 0; i < length; i++ {
		idx := source.Intn(len(changes))
		broadcastChanges[i] = changes[idx]
		changes = append(changes[:idx], changes[idx+1:]...)
	}

	go s.rateDilithiumChanges(s.ctx, broadcastChanges)
}

func (s *Service) broadcastDilithiumBatch(ctx context.Context, ptr *[]*zondpb.SignedDilithiumToExecutionChange) {
	limit := broadcastDilithiumChangesRateLimit
	if len(*ptr) < broadcastDilithiumChangesRateLimit {
		limit = len(*ptr)
	}
	st, err := s.cfg.chain.HeadStateReadOnly(ctx)
	if err != nil {
		log.WithError(err).Error("could not get head state")
		return
	}
	for _, ch := range (*ptr)[:limit] {
		if ch != nil {
			_, err := blocks.ValidateDilithiumToExecutionChange(st, ch)
			if err != nil {
				log.WithError(err).Error("could not validate Dilithium to execution change")
				continue
			}
			if err := s.cfg.p2p.Broadcast(ctx, ch); err != nil {
				log.WithError(err).Error("could not broadcast Dilithium to execution changes.")
			}
		}
	}
	*ptr = (*ptr)[limit:]
}

func (s *Service) rateDilithiumChanges(ctx context.Context, changes []*zondpb.SignedDilithiumToExecutionChange) {
	s.broadcastDilithiumBatch(ctx, &changes)
	if len(changes) == 0 {
		return
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.broadcastDilithiumBatch(ctx, &changes)
			if len(changes) == 0 {
				return
			}
		}
	}
}
