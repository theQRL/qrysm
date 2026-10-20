package kv

import (
	"context"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/trace"
)

// LastValidatedCheckpoint returns the latest fully validated checkpoint in beacon chain.
func (s *Store) LastValidatedCheckpoint(ctx context.Context) (*qrysmpb.Checkpoint, error) {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.LastValidatedCheckpoint")
	defer span.End()
	var checkpoint *qrysmpb.Checkpoint
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(checkpointBucket)
		enc := bkt.Get(lastValidatedCheckpointKey)
		if enc == nil {
			// Finality does not imply execution validity. Without an explicit
			// validation marker, only genesis is known to be valid.
			checkpoint = &qrysmpb.Checkpoint{Root: bytesutil.SafeCopyBytes(params.BeaconConfig().ZeroHash[:])}
			if r := tx.Bucket(blocksBucket).Get(genesisBlockRootKey); r != nil {
				checkpoint.Root = bytesutil.SafeCopyBytes(r)
			}
			return nil
		}
		checkpoint = &qrysmpb.Checkpoint{}
		return decode(ctx, enc, checkpoint)
	})
	return checkpoint, err
}

// SaveLastValidatedCheckpoint saves the last validated checkpoint in beacon chain.
func (s *Store) SaveLastValidatedCheckpoint(ctx context.Context, checkpoint *qrysmpb.Checkpoint) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.SaveLastValidatedCheckpoint")
	defer span.End()

	return s.saveCheckpoint(ctx, lastValidatedCheckpointKey, checkpoint)
}
