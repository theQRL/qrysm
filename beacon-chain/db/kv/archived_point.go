package kv

import (
	"context"

	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/trace"
)

// LastArchivedSlot from the db.
func (s *Store) LastArchivedSlot(ctx context.Context) (primitives.Slot, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.LastArchivedSlot")
	defer span.End()
	var index primitives.Slot
	err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(stateSlotIndicesBucket)
		b, _ := bkt.Cursor().Last()
		index = bytesutil.BytesToSlotBigEndian(b)
		return nil
	})

	return index, err
}

// LastArchivedRoot from the db.
func (s *Store) LastArchivedRoot(ctx context.Context) [32]byte {
	_, span := trace.StartSpan(ctx, "BeaconDB.LastArchivedRoot")
	defer span.End()

	var blockRoot [32]byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(stateSlotIndicesBucket)
		_, v := bkt.Cursor().Last()
		if len(v) > 0 {
			blockRoot = preferFinalizedRoot(tx, v)
		}
		return nil
	}); err != nil {
		// Only a closed database fails a read-only view, which can happen to a
		// background routine during shutdown. Report no root rather than panic.
		log.WithError(err).Error("Could not read the last archived root")
	}

	return blockRoot
}

// preferFinalizedRoot picks the root to report for a state slot index value.
// A slot can index several states when the chain forked at that slot; the
// finalized (canonical) one is preferred, otherwise the first.
func preferFinalizedRoot(tx *bolt.Tx, packed []byte) [32]byte {
	roots, err := splitRoots(packed)
	if err != nil || len(roots) == 0 {
		return bytesutil.ToBytes32(packed)
	}
	if len(roots) > 1 {
		idx := tx.Bucket(finalizedBlockRootsIndexBucket)
		for _, r := range roots {
			if idx.Get(r[:]) != nil {
				return r
			}
		}
	}
	return roots[0]
}

// ArchivedPointRoot returns the block root of an archived point from the DB.
// This is essential for cold state management and to restore a cold state.
func (s *Store) ArchivedPointRoot(ctx context.Context, slot primitives.Slot) [32]byte {
	_, span := trace.StartSpan(ctx, "BeaconDB.ArchivedPointRoot")
	defer span.End()

	var blockRoot [32]byte
	if err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(stateSlotIndicesBucket)
		v := bucket.Get(bytesutil.SlotToBytesBigEndian(slot))
		if len(v) > 0 {
			blockRoot = preferFinalizedRoot(tx, v)
		}
		return nil
	}); err != nil {
		log.WithError(err).WithField("slot", slot).Error("Could not read the archived point root")
	}

	return blockRoot
}

// HasArchivedPoint returns true if an archived point exists in DB.
func (s *Store) HasArchivedPoint(ctx context.Context, slot primitives.Slot) bool {
	_, span := trace.StartSpan(ctx, "BeaconDB.HasArchivedPoint")
	defer span.End()
	var exists bool
	if err := s.db.View(func(tx *bolt.Tx) error {
		iBucket := tx.Bucket(stateSlotIndicesBucket)
		exists = iBucket.Get(bytesutil.SlotToBytesBigEndian(slot)) != nil
		return nil
	}); err != nil {
		log.WithError(err).WithField("slot", slot).Error("Could not check for an archived point")
		return false
	}
	return exists
}
