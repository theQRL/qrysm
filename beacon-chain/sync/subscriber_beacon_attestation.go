package sync

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/beacon-chain/cache"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/container/slice"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/time/slots"
	"google.golang.org/protobuf/proto"
)

func (s *Service) committeeIndexBeaconAttestationSubscriber(_ context.Context, msg proto.Message) error {
	a, ok := msg.(*zond.Attestation)
	if !ok {
		return fmt.Errorf("message was not type *zond.Attestation, type=%T", msg)
	}

	if a.Data == nil {
		return errors.New("nil attestation")
	}
	s.setSeenCommitteeIndicesSlot(a.Data.Slot, a.Data.CommitteeIndex, a.AggregationBits)

	exists, err := s.cfg.attPool.HasAggregatedAttestation(a)
	if err != nil {
		return errors.Wrap(err, "Could not determine if attestation pool has this atttestation")
	}
	if exists {
		return nil
	}

	return s.cfg.attPool.SaveUnaggregatedAttestation(a)
}

func (_ *Service) persistentSubnetIndices() []uint64 {
	return cache.SubnetIDs.GetAllSubnets()
}

func (_ *Service) aggregatorSubnetIndices(currentSlot primitives.Slot) []uint64 {
	endEpoch := slots.ToEpoch(currentSlot) + 1
	endSlot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(endEpoch))
	var commIds []uint64
	for i := currentSlot; i <= endSlot; i++ {
		commIds = append(commIds, cache.SubnetIDs.GetAggregatorSubnetIDs(i)...)
	}
	return slice.SetUint64(commIds)
}

func (_ *Service) attesterSubnetIndices(currentSlot primitives.Slot) []uint64 {
	endEpoch := slots.ToEpoch(currentSlot) + 1
	endSlot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(endEpoch))
	var commIds []uint64
	for i := currentSlot; i <= endSlot; i++ {
		commIds = append(commIds, cache.SubnetIDs.GetAttesterSubnetIDs(i)...)
	}
	return slice.SetUint64(commIds)
}
