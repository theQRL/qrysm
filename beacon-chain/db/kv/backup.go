package kv

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/io/file"
	bolt "go.etcd.io/bbolt"
	"go.opencensus.io/trace"
)

const backupsDirectoryName = "backups"

// Backup the database to the datadir backup directory.
// Example for backup at slot 345: $DATADIR/backups/qrysm_beacondb_at_slot_0000345.backup
//
// The backup is one consistent snapshot of the database, taken in a single
// read transaction: copying key by key across transactions could record a
// finalized checkpoint whose block and state were written after the keys were
// listed, and such a backup cannot start a node. The snapshot is written to a
// temporary file, synced to disk and only then renamed to its final name, so a
// file with the final name is always complete; the directory is synced after the
// rename, so a completed backup survives a crash right after it is reported.
func (s *Store) Backup(ctx context.Context, outputDir string, permissionOverride bool) error {
	ctx, span := trace.StartSpan(ctx, "BeaconDB.Backup")
	defer span.End()

	var backupsDir string
	var err error
	if outputDir != "" {
		backupsDir, err = file.ExpandPath(outputDir)
		if err != nil {
			return err
		}
	} else {
		backupsDir = path.Join(s.databasePath, backupsDirectoryName)
	}
	head, err := s.HeadBlock(ctx)
	if err != nil {
		return err
	}
	if err := blocks.BeaconBlockIsNil(head); err != nil {
		return err
	}
	// Ensure the backups directory exists.
	if err := file.HandleBackupDir(backupsDir, permissionOverride); err != nil {
		return err
	}
	// State summaries are buffered in memory and only written to the database
	// periodically; flush them so the backup does not lack summaries for blocks
	// it contains.
	if err := s.saveCachedStateSummariesDB(ctx); err != nil {
		return errors.Wrap(err, "could not flush state summaries before backup")
	}
	backupPath := path.Join(backupsDir, fmt.Sprintf("qrysm_beacondb_at_slot_%07d.backup", head.Block().Slot()))
	log.WithField("backup", backupPath).Info("Writing backup database.")

	// Each backup writes its own temporary file. Two backups for the same head
	// slot can run at the same time (the webhook can be triggered twice), and a
	// shared temporary name would let one of them rename the other's unfinished
	// copy into place.
	tmpFile, err := os.CreateTemp(backupsDir, path.Base(backupPath)+".*.tmp")
	if err != nil {
		return errors.Wrap(err, "could not create temporary backup file")
	}
	tmpPath := tmpFile.Name()
	if err := s.writeSnapshot(ctx, tmpFile); err != nil {
		if closeErr := tmpFile.Close(); closeErr != nil {
			log.WithError(closeErr).Error("Failed to close temporary backup file")
		}
		if rmErr := os.Remove(tmpPath); rmErr != nil {
			log.WithError(rmErr).Error("Failed to remove incomplete backup file")
		}
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return errors.Wrap(err, "could not close backup file")
	}
	if err := os.Rename(tmpPath, backupPath); err != nil {
		return errors.Wrap(err, "could not move backup database into place")
	}
	// The rename is durable only once the directory is: a crash right after
	// this returns would otherwise leave the backup under its temporary name.
	if err := file.SyncDir(backupsDir); err != nil {
		return errors.Wrap(err, "could not sync the backups directory")
	}
	return nil
}

// writeSnapshot writes a consistent copy of the whole database into f, as of
// one read transaction, and syncs it to disk.
func (s *Store) writeSnapshot(ctx context.Context, f *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		_, err := tx.WriteTo(f)
		return err
	}); err != nil {
		return errors.Wrap(err, "could not write database snapshot")
	}
	if err := f.Sync(); err != nil {
		return errors.Wrap(err, "could not sync backup file")
	}
	return nil
}
