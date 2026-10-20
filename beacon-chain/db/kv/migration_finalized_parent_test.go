package kv

import (
	"bytes"
	"context"
	"testing"

	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	bolt "go.etcd.io/bbolt"
)

// TestMigrateFinalizedParent_SkipsSentinelAndCheckpointEntries is the
// regression test for the migration aborting on finalized-index entries that
// are not encoded containers: the raw "recent block needs reindexing"
// sentinel written for non-canonical blocks of the latest finalized epoch, and
// the previous finalized checkpoint stored under its own key. Both exist in
// every database that has finalized at least once; hitting one made decode
// fail with "snappy: corrupt input", the migration error out, and - as
// completion is only recorded on success - the node refuse to start on every
// restart. The migration must skip them and still repair the corrupt entries.
func TestMigrateFinalizedParent_SkipsSentinelAndCheckpointEntries(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)

	// A block whose index entry carries the corruption this migration repairs
	// (ParentRoot == its own root).
	corruptBlk := util.NewBeaconBlockZond()
	corruptBlk.Block.Slot = 10
	corruptBlk.Block.ParentRoot = bytesutil.PadTo([]byte("real parent"), 32)
	wsb, err := blocks.NewSignedBeaconBlock(corruptBlk)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))
	corruptRoot, err := corruptBlk.Block.HashTreeRoot()
	require.NoError(t, err)

	healthyRoot := bytesutil.ToBytes32(bytesutil.PadTo([]byte("healthy"), 32))
	healthyParent := bytesutil.PadTo([]byte("healthy parent"), 32)
	// Roots spread across the key space so that the sentinel and checkpoint
	// entries are interleaved with container entries in cursor order.
	sentinelRoots := [][32]byte{
		bytesutil.ToBytes32(bytesutil.PadTo([]byte{0x01}, 32)),
		bytesutil.ToBytes32(bytesutil.PadTo([]byte{0x80}, 32)),
		bytesutil.ToBytes32(bytesutil.PadTo([]byte{0xff}, 32)),
	}

	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(finalizedBlockRootsIndexBucket)
		corruptEnc, err := encode(ctx, &qrysmpb.FinalizedBlockRootContainer{ParentRoot: corruptRoot[:]})
		if err != nil {
			return err
		}
		if err := bkt.Put(corruptRoot[:], corruptEnc); err != nil {
			return err
		}
		healthyEnc, err := encode(ctx, &qrysmpb.FinalizedBlockRootContainer{ParentRoot: healthyParent})
		if err != nil {
			return err
		}
		if err := bkt.Put(healthyRoot[:], healthyEnc); err != nil {
			return err
		}
		for _, r := range sentinelRoots {
			if err := bkt.Put(r[:], containerFinalizedButNotCanonical); err != nil {
				return err
			}
		}
		cpEnc, err := encode(ctx, &qrysmpb.Checkpoint{Epoch: 3, Root: healthyRoot[:]})
		if err != nil {
			return err
		}
		return bkt.Put(previousFinalizedCheckpointKey, cpEnc)
	}))

	require.NoError(t, migrateFinalizedParent(ctx, db.db))

	require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(finalizedBlockRootsIndexBucket)

		// The corrupt entry was repaired with the block's real parent root.
		repaired := &qrysmpb.FinalizedBlockRootContainer{}
		require.NoError(t, decode(ctx, bkt.Get(corruptRoot[:]), repaired))
		require.DeepEqual(t, corruptBlk.Block.ParentRoot, repaired.ParentRoot)

		// Healthy, sentinel and checkpoint entries are untouched.
		healthy := &qrysmpb.FinalizedBlockRootContainer{}
		require.NoError(t, decode(ctx, bkt.Get(healthyRoot[:]), healthy))
		require.DeepEqual(t, healthyParent, healthy.ParentRoot)
		for _, r := range sentinelRoots {
			require.Equal(t, true, bytes.Equal(bkt.Get(r[:]), containerFinalizedButNotCanonical), "sentinel entry was modified")
		}
		cp := &qrysmpb.Checkpoint{}
		require.NoError(t, decode(ctx, bkt.Get(previousFinalizedCheckpointKey), cp))
		require.DeepEqual(t, healthyRoot[:], cp.Root)

		// And the migration is recorded as complete, so it does not run again.
		require.DeepEqual(t, migrationCompleted, tx.Bucket(migrationsBucket).Get(migrationFinalizedParent))
		return nil
	}))
}

// TestMigrateFinalizedParent_SkipsUndecodableEntries checks that an index
// value that is neither a container nor a known sentinel is skipped rather
// than aborting the migration.
func TestMigrateFinalizedParent_SkipsUndecodableEntries(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)

	garbageRoot := bytesutil.ToBytes32(bytesutil.PadTo([]byte("garbage"), 32))
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(finalizedBlockRootsIndexBucket).Put(garbageRoot[:], []byte("not snappy, not a container"))
	}))

	require.NoError(t, migrateFinalizedParent(ctx, db.db))
	require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
		require.DeepEqual(t, migrationCompleted, tx.Bucket(migrationsBucket).Get(migrationFinalizedParent))
		return nil
	}))
}
