package sync

import (
	"context"

	"github.com/cyyber/qrysm/v4/beacon-chain/core/feed"
	opfeed "github.com/cyyber/qrysm/v4/beacon-chain/core/feed/operation"
	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

func (s *Service) dilithiumToExecutionChangeSubscriber(_ context.Context, msg proto.Message) error {
	blsMsg, ok := msg.(*ethpb.SignedDilithiumToExecutionChange)
	if !ok {
		return errors.Errorf("incorrect type of message received, wanted %T but got %T", &ethpb.SignedDilithiumToExecutionChange{}, msg)
	}
	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.DilithiumToExecutionChangeReceived,
		Data: &opfeed.DilithiumToExecutionChangeReceivedData{
			Change: blsMsg,
		},
	})
	s.cfg.dilithiumToExecPool.InsertDilithiumToExecChange(blsMsg)
	return nil
}
