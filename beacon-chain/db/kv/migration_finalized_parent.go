package kv

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pkg/errors"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	bolt "go.etcd.io/bbolt"
)

var migrationFinalizedParent = []byte("parent_bug_32fb183")

// migrateFinalizedParent repairs corrupt FinalizedBlockRootContainer.ParentRoot
// entries left behind by the (since-reverted) PR 13486 algorithm, which wrote
// the concatenated children-list for a parent into ParentRoot instead of the
// canonical single 32-byte parent root.
//
// Walks the whole finalizedBlockRootsIndexBucket; for each entry whose
// ParentRoot equals its own key (a hallmark of the corruption), looks up the
// real block and replaces ParentRoot with block.ParentRoot. The bucket is keyed
// by block root, so its iteration order is unrelated to slot order and the scan
// cannot stop early after a run of clean entries: the migration runs once (it
// records completion) and the scan is a read of the index.
func migrateFinalizedParent(ctx context.Context, db *bolt.DB) error {
	if updateErr := db.Update(func(tx *bolt.Tx) error {
		mb := tx.Bucket(migrationsBucket)
		if b := mb.Get(migrationFinalizedParent); bytes.Equal(b, migrationCompleted) {
			return nil // Migration already completed.
		}

		bkt := tx.Bucket(finalizedBlockRootsIndexBucket)
		if bkt == nil {
			return fmt.Errorf("unable to read %s bucket for migration", finalizedBlockRootsIndexBucket)
		}
		bb := tx.Bucket(blocksBucket)
		if bb == nil {
			return fmt.Errorf("unable to read %s bucket for migration", blocksBucket)
		}

		c := bkt.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			// check if context is cancelled in between
			if ctx.Err() != nil {
				return ctx.Err()
			}

			// The index bucket holds more than encoded containers: the
			// previous finalized checkpoint under its own key, and the raw
			// "needs reindexing" sentinel for recently finalized non-canonical
			// blocks (see updateFinalizedBlockRoots). Neither decodes as a
			// container; treating them as corruption aborted the migration
			// and, since completion is only recorded on success, kept the
			// node from starting on every restart.
			if bytes.Equal(k, previousFinalizedCheckpointKey) {
				continue
			}
			if bytes.Equal(v, containerFinalizedButNotCanonical) {
				continue
			}

			idxEntry := &qrysmpb.FinalizedBlockRootContainer{}
			if err := decode(ctx, v, idxEntry); err != nil {
				// An undecodable entry is not one this migration knows how to
				// repair; skipping it is preferable to refusing to start.
				log.WithError(err).WithField("root", fmt.Sprintf("%#x", k)).
					Warn("Skipping finalized index entry that could not be decoded during parent root repair")
				continue
			}
			// Not one of the corrupt values
			if !bytes.Equal(idxEntry.ParentRoot, k) {
				continue
			}
			log.WithField("root", fmt.Sprintf("%#x", k)).Debug("found index entry with incorrect parent root")

			// Look up full block to get the correct parent root.
			encBlk := bb.Get(k)
			if encBlk == nil {
				return errors.Wrapf(ErrNotFound, "could not find block for corrupt finalized index entry %#x", k)
			}
			blk, err := unmarshalBlock(ctx, encBlk)
			if err != nil {
				return errors.Wrapf(err, "unable to decode block for root=%#x", k)
			}
			// Replace parent root in the index with the correct value and write it back.
			pr := blk.Block().ParentRoot()
			idxEntry.ParentRoot = pr[:]
			idxEnc, err := encode(ctx, idxEntry)
			if err != nil {
				return errors.Wrapf(err, "failed to encode finalized index entry for root=%#x", k)
			}
			if err := bkt.Put(k, idxEnc); err != nil {
				return errors.Wrapf(err, "failed to update finalized index entry for root=%#x", k)
			}
			log.WithField("root", fmt.Sprintf("%#x", k)).
				WithField("parentRoot", fmt.Sprintf("%#x", idxEntry.ParentRoot)).
				Debug("updated corrupt index entry with correct parent")
		}
		// Mark migration complete.
		return mb.Put(migrationFinalizedParent, migrationCompleted)
	}); updateErr != nil {
		log.WithError(updateErr).Errorf("could not run finalized parent root index repair migration")
		return updateErr
	}
	return nil
}
