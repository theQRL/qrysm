package sync

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	opfeed "github.com/theQRL/qrysm/beacon-chain/core/feed/operation"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func (s *Service) dilithiumToExecutionChangeSubscriber(_ context.Context, msg proto.Message) error {
	dilithiumMsg, ok := msg.(*zondpb.SignedDilithiumToExecutionChange)
	if !ok {
		return errors.Errorf("incorrect type of message received, wanted %T but got %T", &zondpb.SignedDilithiumToExecutionChange{}, msg)
	}
	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.DilithiumToExecutionChangeReceived,
		Data: &opfeed.DilithiumToExecutionChangeReceivedData{
			Change: dilithiumMsg,
		},
	})
	s.cfg.dilithiumToExecPool.InsertDilithiumToExecChange(dilithiumMsg)
	return nil
}
