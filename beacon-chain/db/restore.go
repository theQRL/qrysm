package db

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/db/kv"
	"github.com/theQRL/qrysm/cmd"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/io/file"
	"github.com/theQRL/qrysm/io/prompt"
	"github.com/urfave/cli/v2"
	bolt "go.etcd.io/bbolt"
)

const dbExistsYesNoPrompt = "A database file already exists in the target directory. " +
	"Are you sure that you want to overwrite it? [y/n]"

// Restore a beacon chain database.
//
// The source must be a readable database file that is not the target itself.
// The copy is written to a temporary file, synced and then renamed over the
// target, so an interrupted restore never leaves a truncated database under
// the database file name. The target is locked for the whole replacement: a
// running beacon node is refused, and one started during the restore cannot
// open the file that is about to be replaced; the replaced file is invalidated
// before the lock is released so that a node waiting for it fails to open.
func Restore(cliCtx *cli.Context) error {
	sourceFile := cliCtx.String(cmd.RestoreSourceFileFlag.Name)
	targetDir := cliCtx.String(cmd.RestoreTargetDirFlag.Name)

	restoreDir := path.Join(targetDir, kv.BeaconNodeDbDirName)
	// A symbolic link is followed: the database lives, and is locked, at the
	// link's target, so that is the file to replace (and the directory the
	// temporary file must be in for the rename). Replacing the link itself would
	// leave the real file in place for a node to open.
	targetFile, err := resolveTargetFile(path.Join(restoreDir, kv.DatabaseFileName))
	if err != nil {
		return err
	}
	restoreDir = filepath.Dir(targetFile)
	existed := file.FileExists(targetFile)
	if existed {
		same, err := sameFile(sourceFile, targetFile)
		if err != nil {
			return err
		}
		if same {
			// Copying a file onto itself truncates it before a byte is read.
			return errors.New("the restore source is the database file in the target directory")
		}
		resp, err := prompt.ValidatePrompt(
			os.Stdin, dbExistsYesNoPrompt, prompt.ValidateYesOrNo,
		)
		if err != nil {
			return errors.Wrap(err, "could not validate choice")
		}
		if strings.EqualFold(resp, "n") {
			log.Info("Restore aborted")
			return nil
		}
	}
	if err := verifyDatabaseFile(sourceFile); err != nil {
		return errors.Wrap(err, "the restore source is not a readable database file")
	}
	// Created only when missing, as the store does: an existing directory (the
	// target of a symbolic link, say) keeps whatever permissions it has.
	hasDir, err := file.HasDir(restoreDir)
	if err != nil {
		return err
	}
	if !hasDir {
		if err := file.MkdirAll(restoreDir); err != nil {
			return err
		}
	}
	// The lock is held until the restored file is in place. Taken only for an
	// in-use check, it would let a beacon node open the target while the backup
	// is copied; the rename would then pull the file out from under the running
	// node, whose writes would go to an unlinked file and be lost.
	unlock, pageSize, err := lockTargetDatabase(targetFile)
	if err != nil {
		return err
	}
	defer unlock()
	locked := pageSize > 0
	// The lock is on the file, not on the path. A node that opened the path
	// while the lock was held has a descriptor of the old file and retries the
	// lock for its open timeout, so releasing the lock after the rename would
	// hand it that file, by then unlinked, and its writes would be lost. The old
	// file is reached through a second descriptor and invalidated once the
	// restored file is in place, so such a node fails to open it instead.
	var oldFile *os.File
	closeOldFile := func() {
		if oldFile == nil {
			return
		}
		if err := oldFile.Close(); err != nil {
			log.WithError(err).Error("Could not close the replaced database file")
		}
		oldFile = nil
	}
	defer closeOldFile()
	if locked {
		oldFile, err = os.OpenFile(targetFile, os.O_RDWR, 0) // #nosec G304
		if err != nil {
			return errors.Wrap(err, "could not open the target database for invalidation")
		}
		// A file reachable through another hard link cannot be invalidated once
		// replaced (the other link is somebody's database or backup), so a node
		// waiting for the lock would open the stale database through it. Such a
		// target is refused before anything is changed; a placeholder created
		// above has a single link.
		if existed {
			info, err := oldFile.Stat()
			if err != nil {
				return errors.Wrap(err, "could not stat the target database")
			}
			if links, known := linkCount(info); known && links > 1 {
				return fmt.Errorf("the target database file has %d hard links; remove the other links before restoring, "+
					"or a node started during the restore could open the replaced file through them", links)
			}
		}
	}
	// A placeholder created for the lock is removed when the restore fails,
	// while the lock is still held: the removal is made durable and the file is
	// then invalidated, so that a node waiting for the lock (holding a
	// descriptor of it) fails to open it instead of running on a removed
	// database. Once the lock has been released (the rename fallback) the
	// placeholder may be a node's database and is left alone. Registered after
	// the lock release and the descriptor close, so it runs before them.
	failed, lockReleased := true, false
	defer func() {
		if !failed || existed || lockReleased {
			return
		}
		if rmErr := os.Remove(targetFile); rmErr != nil && !os.IsNotExist(rmErr) {
			log.WithError(rmErr).Error("Could not remove the placeholder database file")
			return
		}
		if oldFile == nil {
			return
		}
		if err := syncDirectory(restoreDir); err != nil {
			log.WithError(err).Warn("Could not sync the database directory; the removed placeholder is not invalidated")
			waitOutLockWaiters()
			return
		}
		if err := invalidateReplacedDatabase(oldFile, pageSize); err != nil {
			log.WithError(err).Warn("The removed placeholder was not invalidated")
			waitOutLockWaiters()
		}
	}()
	// A fresh temporary file: a fixed name could be the source itself (a
	// leftover of an interrupted restore), which the copy would truncate
	// before reading it.
	tmp, err := os.CreateTemp(restoreDir, kv.DatabaseFileName+".restore.*.tmp")
	if err != nil {
		return errors.Wrap(err, "could not create a temporary restore file")
	}
	tmpFile := tmp.Name()
	if err := tmp.Close(); err != nil {
		return errors.Wrap(err, "could not close the temporary restore file")
	}
	if err := copyDatabaseFile(sourceFile, tmpFile); err != nil {
		if rmErr := os.Remove(tmpFile); rmErr != nil && !os.IsNotExist(rmErr) {
			log.WithError(rmErr).Error("Could not remove incomplete restore file")
		}
		return err
	}
	renamedLocked := true
	if err := os.Rename(tmpFile, targetFile); err != nil {
		// A platform that refuses to replace an open file (Windows opens
		// without delete sharing): release the lock, close the second
		// descriptor too, and retry at once, which leaves the smallest window
		// possible. The old file is not invalidated then, as a node may own it
		// by now.
		unlock()
		closeOldFile()
		lockReleased, renamedLocked = true, false
		if err := os.Rename(tmpFile, targetFile); err != nil {
			return errors.Wrap(err, "could not move the restored database into place")
		}
	}
	// The restored file is in place: nothing is left to clean up.
	failed = false
	// The rename must be durable before the old file is touched: with the
	// zeros on disk but the directory change not yet, a power loss would leave
	// the path pointing at a file whose meta pages are cleared. When the old
	// file cannot be invalidated, the lock is held until every node that may be
	// waiting for it has given up.
	if err := syncDirectory(restoreDir); err != nil {
		if renamedLocked {
			waitOutLockWaiters()
		}
		return errors.Wrap(err, "the restored database is in place, but the directory sync failed, so the restore may not survive a power loss")
	}
	if renamedLocked && oldFile != nil {
		// Only reachable through descriptors opened before the rename: ours and
		// those of nodes waiting for the lock.
		if err := invalidateReplacedDatabase(oldFile, pageSize); err != nil {
			log.WithError(err).Warn("The replaced database file was not invalidated")
			waitOutLockWaiters()
		}
	}

	log.Info("Restore completed successfully")
	return nil
}

// invalidateReplacedDatabase overwrites both meta pages of the replaced
// database file so that a bolt open on it fails instead of serving the stale
// database. bolt recovers from either meta page, so both are cleared, at the
// page size the file was created with; the file keeps a non-zero size, so bolt
// does not initialize it as a new empty database either. The file must be
// unlinked: one still reachable through a hard link is someone's database or
// backup and is left intact (Restore refuses such a target up front).
func invalidateReplacedDatabase(f *os.File, pageSize int) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	links, known := linkCount(info)
	if !known {
		return errors.New("the link count of the replaced file is not known on this platform")
	}
	if links != 0 {
		return fmt.Errorf("the replaced file is still linked %d time(s) (a hard link, or the target of a symbolic link)", links)
	}
	n := int64(2 * pageSize)
	if n > info.Size() {
		n = info.Size()
	}
	if _, err := f.WriteAt(make([]byte, n), 0); err != nil {
		return err
	}
	return f.Sync()
}

// resolveTargetFile follows a symbolic link at the database file path.
func resolveTargetFile(p string) (string, error) {
	info, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return "", errors.Wrap(err, "could not stat the restore target")
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return p, nil
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", errors.Wrap(err, "the restore target is a symbolic link that cannot be resolved")
	}
	return resolved, nil
}

// sameFile reports whether the two paths name the same file.
func sameFile(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, errors.Wrap(err, "could not stat the restore source")
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false, errors.Wrap(err, "could not stat the restore target")
	}
	return os.SameFile(ai, bi), nil
}

// lockTargetDatabase takes the exclusive lock of the target database file,
// creating the file when there is none, and returns the function releasing it
// and the file's bolt page size, 0 when the lock was not taken. The lock is the
// one a beacon node takes, so a running node makes the restore refuse, and a
// node started while the lock is held fails to open the file. A file that
// cannot be opened as a database cannot be opened by a node either, so it is
// replaced unlocked. The release function may be called more than once.
func lockTargetDatabase(dbPath string) (release func(), pageSize int, err error) {
	db, err := bolt.Open(dbPath, params.BeaconIoConfig().ReadWritePermissions, &bolt.Options{Timeout: time.Second})
	if err != nil {
		if errors.Is(err, bolt.ErrTimeout) {
			return nil, 0, errors.New("the target database is in use by another process; stop the beacon node before restoring")
		}
		log.WithError(err).Warn("The target database file could not be locked; replacing it unlocked")
		return func() {}, 0, nil
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			if err := db.Close(); err != nil {
				log.WithError(err).Error("Could not release the target database lock")
			}
		})
	}, db.Info().PageSize, nil
}

// verifyDatabaseFile checks that the path opens as a bolt database.
func verifyDatabaseFile(dbPath string) error {
	if !file.FileExists(dbPath) {
		return errors.New("source file does not exist at provided path")
	}
	db, err := bolt.Open(dbPath, params.BeaconIoConfig().ReadWritePermissions, &bolt.Options{Timeout: time.Second, ReadOnly: true})
	if err != nil {
		if errors.Is(err, bolt.ErrTimeout) {
			return errors.New("the source database is in use by another process")
		}
		return err
	}
	return db.Close()
}

// waitOutLockWaiters is called with the lock of a replaced or removed database
// file still held when that file could not be invalidated. A node that opened
// the file's path while the lock was held retries the lock for OpenTimeout and
// then gives up; holding the lock for longer than that before releasing it sees
// every such node off, so none of them runs on the stale, unlinked file.
func waitOutLockWaiters() {
	wait := 2 * kv.OpenTimeout
	log.WithField("wait", wait).Warn("Holding the database lock until nodes that may be waiting for it have given up")
	time.Sleep(wait)
}

// syncDirectory makes a directory's entries durable; a variable so a test can
// make it fail.
var syncDirectory = file.SyncDir

// copyDatabaseFile copies the backup into the temporary file; a variable so a
// test can make the copy fail.
var copyDatabaseFile = snapshotDatabase

// snapshotDatabase writes a consistent snapshot of the source database to dst.
// The source is opened read-only, which holds its shared lock for the whole
// copy: a beacon node cannot open and change it meanwhile (a raw copy of a
// database being written to mixes pages of different transactions), and a
// source a node already has open is refused. The snapshot is written by bolt
// from one read transaction, and synced.
func snapshotDatabase(src, dst string) (err error) {
	db, err := bolt.Open(src, params.BeaconIoConfig().ReadWritePermissions, &bolt.Options{Timeout: time.Second, ReadOnly: true})
	if err != nil {
		if errors.Is(err, bolt.ErrTimeout) {
			return errors.New("the source database is in use by another process")
		}
		return errors.Wrap(err, "could not open the restore source")
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, params.BeaconIoConfig().ReadWritePermissions) // #nosec G304
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	if err := db.View(func(tx *bolt.Tx) error {
		_, err := tx.WriteTo(out)
		return err
	}); err != nil {
		return errors.Wrap(err, "could not write the database snapshot")
	}
	return out.Sync()
}
