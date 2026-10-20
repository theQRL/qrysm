package slasherkv

import (
	"bytes"
	"context"
	"encoding/binary"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/time/slots"
	bolt "go.etcd.io/bbolt"
)

var errTimeOut = errors.New("operation timed out")

// PruneAttestationsAtEpoch deletes all attestations from the slasher DB with target epoch
// less than or equal to the specified epoch.
//
// The deletion is bounded in time. When the deadline passes, the deletes made so
// far are committed (returning an error from the transaction would roll all of
// them back and make no progress at all) and the next pruning tick continues
// from the lowest remaining epoch. The validator index and the records are
// pruned in separate transactions and each decides on its own whether it still
// holds expired entries, so an interruption between the two cannot strand
// expired records behind an already pruned index.
func (s *Store) PruneAttestationsAtEpoch(
	ctx context.Context, maxEpoch primitives.Epoch,
) (numPruned uint, err error) {
	// In some cases, pruning may take a very long time and consume significant memory in the
	// open Update transaction. Therefore, we impose a 1 minute timeout on this operation.
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	// We can prune everything less than the current epoch - history length.
	encodedEndPruneEpoch := make([]byte, 8)
	binary.BigEndian.PutUint64(encodedEndPruneEpoch, uint64(maxEpoch))

	// Both buckets are keyed by the target epoch; only the ones whose lowest
	// epoch is at or below the pruning epoch hold work.
	var indexLowest, recordsLowest primitives.Epoch
	var indexHasData, recordsHasData bool
	if err = s.db.View(func(tx *bolt.Tx) error {
		indexLowest, indexHasData = lowestEpoch(tx.Bucket(attestationDataRootsBucket))
		recordsLowest, recordsHasData = lowestEpoch(tx.Bucket(attestationRecordsBucket))
		return nil
	}); err != nil {
		return
	}
	// If there is no data stored, just exit early.
	if !indexHasData && !recordsHasData {
		return
	}
	indexHasWork := indexHasData && indexLowest <= maxEpoch
	recordsHasWork := recordsHasData && recordsLowest <= maxEpoch
	if !indexHasWork && !recordsHasWork {
		lowest := indexLowest
		if !indexHasData || (recordsHasData && recordsLowest < lowest) {
			lowest = recordsLowest
		}
		log.Debugf("Lowest epoch %d is > pruning epoch %d, nothing to prune", lowest, maxEpoch)
		return
	}

	var deleted uint
	var timedOut bool
	if indexHasWork {
		err = s.db.Update(func(tx *bolt.Tx) error {
			signingRootsBkt := tx.Bucket(attestationDataRootsBucket)
			c := signingRootsBkt.Cursor()
			// We begin a pruning iteration starting from the first item in the bucket.
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				if ctx.Err() != nil {
					// Deadline reached: commit what was deleted so far.
					timedOut = true
					return nil
				}
				// We check the epoch from the current key in the database.
				// If we have hit an epoch that is greater than the end epoch of the pruning process,
				// we then completely exit the process as we are done.
				if uint64PrefixGreaterThan(k, encodedEndPruneEpoch) {
					return nil
				}
				// Index entries in the database look like this:
				//  (target_epoch ++ validator_index) => data root
				// so it is possible we have a few adjacent objects that have the same epoch.
				if err := signingRootsBkt.Delete(k); err != nil {
					return errors.Wrap(err, "delete attestation signing root")
				}
				deleted++
			}
			return nil
		})
		if err != nil {
			log.WithError(err).Error("Failed to prune attestations")
			return
		}
	}
	if recordsHasWork && !timedOut {
		// Records are keyed by target epoch as well, so every record of a pruned
		// epoch is removed, including records that no index entry points at any
		// more (a validator that voted twice at the same target keeps only the
		// latest root in its index entry).
		err = s.db.Update(func(tx *bolt.Tx) error {
			attRecordsBkt := tx.Bucket(attestationRecordsBucket)
			c := attRecordsBkt.Cursor()
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				if ctx.Err() != nil {
					timedOut = true
					return nil
				}
				if uint64PrefixGreaterThan(k, encodedEndPruneEpoch) {
					return nil
				}
				if err := attRecordsBkt.Delete(k); err != nil {
					return errors.Wrap(err, "delete attestation record")
				}
			}
			return nil
		})
		if err != nil {
			log.WithError(err).Error("Failed to prune attestation records")
			return
		}
	}

	// Only committed deletions count.
	numPruned = deleted
	slasherAttestationsPrunedTotal.Add(float64(deleted))
	if timedOut {
		log.WithField("numPruned", deleted).Warning("Pruning deadline reached, committed partial progress; continuing on the next tick")
		err = errTimeOut
	}
	return
}

// lowestEpoch returns the epoch of the bucket's first key (keys are ordered by
// their 8-byte big-endian epoch prefix) and whether the bucket holds any.
func lowestEpoch(bkt *bolt.Bucket) (primitives.Epoch, bool) {
	k, _ := bkt.Cursor().First()
	if len(k) < 8 {
		return 0, false
	}
	return primitives.Epoch(binary.BigEndian.Uint64(k[:8])), true
}

// PruneProposalsAtEpoch deletes all proposals from the slasher DB with epoch
// less than or equal to the specified epoch. A cancelled context commits the
// deletes made so far instead of rolling them back.
func (s *Store) PruneProposalsAtEpoch(
	ctx context.Context, maxEpoch primitives.Epoch,
) (numPruned uint, err error) {
	var endPruneSlot primitives.Slot
	endPruneSlot, err = slots.EpochEnd(maxEpoch)
	if err != nil {
		return
	}
	encodedEndPruneSlot := make([]byte, 8)
	binary.BigEndian.PutUint64(encodedEndPruneSlot, uint64(endPruneSlot))

	// We retrieve the lowest stored slot in the proposals bucket.
	var lowestSlot primitives.Slot
	var hasData bool
	if err = s.db.View(func(tx *bolt.Tx) error {
		proposalBkt := tx.Bucket(proposalRecordsBucket)
		c := proposalBkt.Cursor()
		k, _ := c.First()
		if k == nil {
			return nil
		}
		hasData = true
		lowestSlot = slotFromProposalKey(k)
		return nil
	}); err != nil {
		return
	}

	// If there is no data stored, just exit early.
	if !hasData {
		return
	}

	// If the lowest slot is greater than the end pruning slot,
	// there is nothing to prune, so we return early.
	if lowestSlot > endPruneSlot {
		log.Debugf("Lowest slot %d is > pruning slot %d, nothing to prune", lowestSlot, endPruneSlot)
		return
	}

	var deleted uint
	var cancelled bool
	if err = s.db.Update(func(tx *bolt.Tx) error {
		proposalBkt := tx.Bucket(proposalRecordsBucket)
		c := proposalBkt.Cursor()
		// We begin a pruning iteration starting from the first item in the bucket.
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if ctx.Err() != nil {
				cancelled = true
				return nil
			}
			// We check the slot from the current key in the database.
			// If we have hit a slot that is greater than the end slot of the pruning process,
			// we then completely exit the process as we are done.
			if uint64PrefixGreaterThan(k, encodedEndPruneSlot) {
				return nil
			}
			// Proposals in the database look like this:
			//  (slot ++ validatorIndex) => encode(proposal)
			// so it is possible we have a few adjacent objects that have the same slot, such as
			//  (slot = 3 ++ validatorIndex = 0) => ...
			//  (slot = 3 ++ validatorIndex = 1) => ...
			//  (slot = 3 ++ validatorIndex = 2) => ...
			if err := proposalBkt.Delete(k); err != nil {
				return err
			}
			deleted++
		}
		return nil
	}); err != nil {
		return
	}
	numPruned = deleted
	slasherProposalsPrunedTotal.Add(float64(deleted))
	if cancelled {
		err = ctx.Err()
	}
	return
}

func slotFromProposalKey(key []byte) primitives.Slot {
	return primitives.Slot(binary.BigEndian.Uint64(key[:8]))
}

func uint64PrefixGreaterThan(key, lessThan []byte) bool {
	enc := key[:8]
	return bytes.Compare(enc, lessThan) > 0
}
