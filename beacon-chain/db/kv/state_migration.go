package kv

import (
	"context"

	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/trace"
)

// stateMigrationCursorKey holds the slot up to which the cold state migration
// has archived every point. It is kept separately from the archived states
// themselves: a finalized state can be saved at an archived slot by other
// paths (a forced checkpoint at shutdown, a hot state during non-finality)
// without the points below it having been written.
var stateMigrationCursorKey = []byte("state-migration-cursor")

// StateMigrationCursor returns the slot up to which the cold state migration
// completed, and whether one was recorded.
func (s *Store) StateMigrationCursor(ctx context.Context) (primitives.Slot, bool, error) {
	_, span := trace.StartSpan(ctx, "BeaconDB.StateMigrationCursor")
	defer span.End()
	var slot primitives.Slot
	var found bool
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(chainMetadataBucket).Get(stateMigrationCursorKey)
		if len(v) != 8 {
			return nil
		}
		slot = bytesutil.BytesToSlotBigEndian(v)
		found = true
		return nil
	})
	return slot, found, err
}

// SaveStateMigrationCursor records the slot up to which the cold state
// migration completed. The cursor never moves back: migrations run in the
// background and complete in any order, and an older one finishing after a
// newer one must not undo the recorded progress.
func (s *Store) SaveStateMigrationCursor(ctx context.Context, slot primitives.Slot) error {
	_, span := trace.StartSpan(ctx, "BeaconDB.SaveStateMigrationCursor")
	defer span.End()
	return s.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(chainMetadataBucket)
		if v := bkt.Get(stateMigrationCursorKey); len(v) == 8 && bytesutil.BytesToSlotBigEndian(v) >= slot {
			return nil
		}
		return bkt.Put(stateMigrationCursorKey, bytesutil.SlotToBytesBigEndian(slot))
	})
}
