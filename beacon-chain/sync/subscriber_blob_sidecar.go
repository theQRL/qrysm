package sync

import (
	"context"
	"fmt"

	"github.com/theQRL/qrysm/v4/beacon-chain/core/feed"
	opfeed "github.com/theQRL/qrysm/v4/beacon-chain/core/feed/operation"
	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"google.golang.org/protobuf/proto"
)

func (s *Service) blobSubscriber(ctx context.Context, msg proto.Message) error {
	b, ok := msg.(*zond.SignedBlobSidecar)
	if !ok {
		return fmt.Errorf("message was not type *eth.SignedBlobSidecar, type=%T", msg)
	}

	s.setSeenBlobIndex(b.Message.Blob, b.Message.Index)

	if err := s.cfg.chain.ReceiveBlob(ctx, b.Message); err != nil {
		return err
	}

	s.cfg.operationNotifier.OperationFeed().Send(&feed.Event{
		Type: opfeed.BlobSidecarReceived,
		Data: &opfeed.BlobSidecarReceivedData{
			Blob: b,
		},
	})
	return nil
}
