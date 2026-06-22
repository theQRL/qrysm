package kv

import (
	"bytes"
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
			var finErr error
			checkpoint, finErr = s.FinalizedCheckpoint(ctx)
			if finErr != nil {
				return finErr
			}
			// Before the first finalized epoch the finalized checkpoint root is the
			// zero hash. Callers comparing this root against a real block root
			// (e.g. setup_forkchoice's SetOptimisticToValid) would otherwise miss
			// genesis. Fall back to the genesis block root when present.
			if bytes.Equal(checkpoint.Root, params.BeaconConfig().ZeroHash[:]) {
				bkt = tx.Bucket(blocksBucket)
				r := bkt.Get(genesisBlockRootKey)
				if r != nil {
					checkpoint.Root = bytesutil.SafeCopyBytes(r)
				}
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
