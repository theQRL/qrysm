package blockchain

import (
	"context"

	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

// SendNewBlobEvent sends a message to the BlobNotifier channel that the blob
// for the blocroot `root` is ready in the database
func (s *Service) sendNewBlobEvent(root [32]byte, index uint64) {
	s.blobNotifiers.forRoot(root) <- index
}

// ReceiveBlob saves the blob to database and sends the new event
func (s *Service) ReceiveBlob(ctx context.Context, b *zondpb.BlobSidecar) error {
	if err := s.cfg.BeaconDB.SaveBlobSidecar(ctx, []*zondpb.BlobSidecar{b}); err != nil {
		return err
	}

	s.sendNewBlobEvent([32]byte(b.BlockRoot), b.Index)
	return nil
}
