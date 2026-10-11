package db

import (
	"errors"
	"time"

	"context"
	"flag"
	"os"
	"path"
	"testing"

	"github.com/theQRL/qrysm/config/params"
	bolt "go.etcd.io/bbolt"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/beacon-chain/db/kv"
	"github.com/theQRL/qrysm/cmd"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/urfave/cli/v2"
)

func TestRestore(t *testing.T) {
	logHook := logTest.NewGlobal()
	ctx := context.Background()

	backupDb, err := kv.NewKVStore(context.Background(), t.TempDir())
	require.NoError(t, err)
	head := util.NewBeaconBlockZond()
	head.Block.Slot = 5000
	wsb, err := blocks.NewSignedBeaconBlock(head)
	require.NoError(t, err)
	require.NoError(t, backupDb.SaveBlock(ctx, wsb))
	root, err := head.Block.HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, backupDb.SaveState(ctx, st, root))
	require.NoError(t, backupDb.SaveHeadBlockRoot(ctx, root))
	require.NoError(t, err)
	require.NoError(t, backupDb.Close())
	// We rename the backup file so that we can later verify
	// whether the restored db has been renamed correctly.
	require.NoError(t, os.Rename(
		path.Join(backupDb.DatabasePath(), kv.DatabaseFileName),
		path.Join(backupDb.DatabasePath(), "backup.db")))

	restoreDir := t.TempDir()
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.RestoreSourceFileFlag.Name, "", "")
	set.String(cmd.RestoreTargetDirFlag.Name, "", "")
	require.NoError(t, set.Set(cmd.RestoreSourceFileFlag.Name, path.Join(backupDb.DatabasePath(), "backup.db")))
	require.NoError(t, set.Set(cmd.RestoreTargetDirFlag.Name, restoreDir))
	cliCtx := cli.NewContext(&app, set, nil)

	assert.NoError(t, Restore(cliCtx))

	files, err := os.ReadDir(path.Join(restoreDir, kv.BeaconNodeDbDirName))
	require.NoError(t, err)
	assert.Equal(t, 1, len(files))
	assert.Equal(t, kv.DatabaseFileName, files[0].Name())
	restoredDb, err := kv.NewKVStore(context.Background(), path.Join(restoreDir, kv.BeaconNodeDbDirName))
	defer func() {
		require.NoError(t, restoredDb.Close())
	}()
	require.NoError(t, err)
	headBlock, err := restoredDb.HeadBlock(ctx)
	require.NoError(t, err)
	assert.Equal(t, primitives.Slot(5000), headBlock.Block().Slot(), "Restored database has incorrect data")
	assert.LogsContain(t, logHook, "Restore completed successfully")

}

// Restoring a database onto itself used to truncate it before a byte was read.
func TestRestore_RefusesToCopyDatabaseOntoItself(t *testing.T) {
	dataDir := t.TempDir()
	dbDir := path.Join(dataDir, kv.BeaconNodeDbDirName)
	store, err := kv.NewKVStore(context.Background(), dbDir)
	require.NoError(t, err)
	require.NoError(t, store.Close())
	dbFile := path.Join(dbDir, kv.DatabaseFileName)

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.RestoreSourceFileFlag.Name, "", "")
	set.String(cmd.RestoreTargetDirFlag.Name, "", "")
	require.NoError(t, set.Set(cmd.RestoreSourceFileFlag.Name, dbFile))
	require.NoError(t, set.Set(cmd.RestoreTargetDirFlag.Name, dataDir))
	cliCtx := cli.NewContext(&app, set, nil)

	require.ErrorContains(t, "the restore source is the database file in the target directory", Restore(cliCtx))

	// The database is intact.
	store, err = kv.NewKVStore(context.Background(), dbDir)
	require.NoError(t, err)
	require.NoError(t, store.Close())
}

// A source living in the target directory under a temporary-looking name (a
// leftover of an interrupted restore) must be copied, not truncated: the
// restore uses a fresh temporary file of its own.

func TestRestore_SourceInTargetDirectoryIsNotTruncated(t *testing.T) {
	dataDir := t.TempDir()
	dbDir := path.Join(dataDir, kv.BeaconNodeDbDirName)
	store, err := kv.NewKVStore(context.Background(), dbDir)
	require.NoError(t, err)
	head := util.NewBeaconBlockZond()
	head.Block.Slot = 77
	wsb, err := blocks.NewSignedBeaconBlock(head)
	require.NoError(t, err)
	require.NoError(t, store.SaveBlock(context.Background(), wsb))
	root, err := head.Block.HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, store.SaveState(context.Background(), st, root))
	require.NoError(t, store.SaveHeadBlockRoot(context.Background(), root))
	require.NoError(t, store.Close())
	// The only copy of the database sits under a leftover temporary name.
	source := path.Join(dbDir, kv.DatabaseFileName+".restore.tmp")
	require.NoError(t, os.Rename(path.Join(dbDir, kv.DatabaseFileName), source))

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.RestoreSourceFileFlag.Name, "", "")
	set.String(cmd.RestoreTargetDirFlag.Name, "", "")
	require.NoError(t, set.Set(cmd.RestoreSourceFileFlag.Name, source))
	require.NoError(t, set.Set(cmd.RestoreTargetDirFlag.Name, dataDir))
	cliCtx := cli.NewContext(&app, set, nil)
	require.NoError(t, Restore(cliCtx))

	restored, err := kv.NewKVStore(context.Background(), dbDir)
	require.NoError(t, err)
	defer func() { require.NoError(t, restored.Close()) }()
	headBlock, err := restored.HeadBlock(context.Background())
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(77), headBlock.Block().Slot())
	info, err := os.Stat(source)
	require.NoError(t, err)
	require.NotEqual(t, int64(0), info.Size(), "the source was truncated")
}

// A source that is not a database is refused before the target is touched.

func TestRestore_RefusesNonDatabaseSource(t *testing.T) {
	dataDir := t.TempDir()
	source := path.Join(t.TempDir(), "not-a-db")
	require.NoError(t, os.WriteFile(source, []byte("hello"), 0o600))

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.RestoreSourceFileFlag.Name, "", "")
	set.String(cmd.RestoreTargetDirFlag.Name, "", "")
	require.NoError(t, set.Set(cmd.RestoreSourceFileFlag.Name, source))
	require.NoError(t, set.Set(cmd.RestoreTargetDirFlag.Name, dataDir))
	cliCtx := cli.NewContext(&app, set, nil)

	require.ErrorContains(t, "not a readable database file", Restore(cliCtx))
}

// The target stays locked for the whole replacement: a beacon node (which takes
// the same lock) cannot open it until the restore releases it.
func TestLockTargetDatabase_HoldsTheLockUntilReleased(t *testing.T) {
	target := path.Join(t.TempDir(), kv.DatabaseFileName)
	unlock, pageSize, err := lockTargetDatabase(target)
	require.NoError(t, err)
	require.Equal(t, true, pageSize > 0, "the lock reports the file's page size")

	opts := &bolt.Options{Timeout: 200 * time.Millisecond}
	_, err = bolt.Open(target, params.BeaconIoConfig().ReadWritePermissions, opts)
	require.ErrorIs(t, err, bolt.ErrTimeout, "the target must stay locked")
	_, _, err = lockTargetDatabase(target)
	require.ErrorContains(t, "in use", err, "a second restore must see the target as in use")

	unlock()
	unlock() // releasing twice is harmless
	db, err := bolt.Open(target, params.BeaconIoConfig().ReadWritePermissions, opts)
	require.NoError(t, err)
	require.NoError(t, db.Close())
}

// A restore into a directory without a database takes the lock by creating the
// file; when the copy then fails, neither that placeholder nor the temporary
// file may be left behind.
func TestRestore_FailedCopyLeavesNoPlaceholder(t *testing.T) {
	ctx := context.Background()
	backupDb, err := kv.NewKVStore(ctx, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, backupDb.Close())
	source := path.Join(backupDb.DatabasePath(), kv.DatabaseFileName)

	original := copyDatabaseFile
	copyDatabaseFile = func(_, _ string) error { return errors.New("disk full") }
	defer func() { copyDatabaseFile = original }()

	restoreDir := t.TempDir()
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.RestoreSourceFileFlag.Name, "", "")
	set.String(cmd.RestoreTargetDirFlag.Name, "", "")
	require.NoError(t, set.Set(cmd.RestoreSourceFileFlag.Name, source))
	require.NoError(t, set.Set(cmd.RestoreTargetDirFlag.Name, restoreDir))
	cliCtx := cli.NewContext(&app, set, nil)

	require.ErrorContains(t, "disk full", Restore(cliCtx))
	entries, err := os.ReadDir(path.Join(restoreDir, kv.BeaconNodeDbDirName))
	require.NoError(t, err)
	require.Equal(t, 0, len(entries), "no database or temporary file may be left behind")
}

// answerPrompt feeds the answer to the overwrite prompt through os.Stdin.
func answerPrompt(t *testing.T, answer string) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(answer + "\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		require.NoError(t, r.Close())
	})
}

// newDatabaseWithHead creates a database in dir whose head block sits at slot
// and returns the database file path.
func newDatabaseWithHead(t *testing.T, dir string, slot primitives.Slot) string {
	ctx := context.Background()
	store, err := kv.NewKVStore(ctx, dir)
	require.NoError(t, err)
	head := util.NewBeaconBlockZond()
	head.Block.Slot = slot
	wsb, err := blocks.NewSignedBeaconBlock(head)
	require.NoError(t, err)
	require.NoError(t, store.SaveBlock(ctx, wsb))
	root, err := head.Block.HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, store.SaveState(ctx, st, root))
	require.NoError(t, store.SaveHeadBlockRoot(ctx, root))
	require.NoError(t, store.Close())
	return path.Join(store.DatabasePath(), kv.DatabaseFileName)
}

// headSlot opens the database in dir and returns its head block slot.
func headSlot(t *testing.T, dir string) primitives.Slot {
	store, err := kv.NewKVStore(context.Background(), dir)
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()
	head, err := store.HeadBlock(context.Background())
	require.NoError(t, err)
	return head.Block().Slot()
}

func restoreContext(t *testing.T, source, targetDir string) *cli.Context {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(cmd.RestoreSourceFileFlag.Name, "", "")
	set.String(cmd.RestoreTargetDirFlag.Name, "", "")
	require.NoError(t, set.Set(cmd.RestoreSourceFileFlag.Name, source))
	require.NoError(t, set.Set(cmd.RestoreTargetDirFlag.Name, targetDir))
	return cli.NewContext(&app, set, nil)
}

// restoreWithWaitingNode runs the restore while a "node" opens the target
// path during the copy, which is while the restore holds the lock, with a
// lock timeout long enough to be handed the lock after the rename. When
// failCopy is set the copy fails instead of copying. It returns the restore's
// result and the node's open result: a nil node result means the node opened a
// database.
func restoreWithWaitingNode(t *testing.T, source, targetDir, target string, nodeOptions *bolt.Options, failCopy bool) (restoreErr, nodeErr error) {
	nodeOpen := make(chan error, 1)
	original := copyDatabaseFile
	copyDatabaseFile = func(src, dst string) error {
		go func() {
			db, err := bolt.Open(target, params.BeaconIoConfig().ReadWritePermissions, nodeOptions)
			if err == nil {
				err = db.Close()
			}
			nodeOpen <- err
		}()
		time.Sleep(500 * time.Millisecond) // let the node reach its lock retry loop
		if failCopy {
			return errors.New("disk full")
		}
		return original(src, dst)
	}
	defer func() { copyDatabaseFile = original }()
	restoreErr = Restore(restoreContext(t, source, targetDir))
	select {
	case nodeErr = <-nodeOpen:
	case <-time.After(15 * time.Second):
		t.Fatal("the waiting node neither opened the replaced file nor failed")
	}
	return restoreErr, nodeErr
}

// A beacon node that opened the target path while the restore held the lock
// keeps a descriptor of the old file and retries the lock for its open timeout.
// Once the restored file is in place, the old file is unlinked: the node must
// fail to open it rather than run on the stale, unlinked database.
func TestRestore_NodeWaitingForTheLockCannotOpenTheReplacedFile(t *testing.T) {
	source := newDatabaseWithHead(t, t.TempDir(), 77)
	restoreDir := t.TempDir()
	target := path.Join(restoreDir, kv.BeaconNodeDbDirName, kv.DatabaseFileName)

	restoreErr, nodeErr := restoreWithWaitingNode(t, source, restoreDir, target, &bolt.Options{Timeout: 10 * time.Second}, false)
	require.NoError(t, restoreErr)
	require.ErrorIs(t, nodeErr, bolt.ErrInvalid, "the waiting node must not open the replaced file")
	require.Equal(t, primitives.Slot(77), headSlot(t, path.Join(restoreDir, kv.BeaconNodeDbDirName)))
}

// When the restore into an empty directory fails, the placeholder created for
// the lock is removed. A node waiting for the lock holds a descriptor of it and
// must not be able to open the removed file and write into it.
func TestRestore_FailedCopyInvalidatesPlaceholderForWaitingNode(t *testing.T) {
	source := newDatabaseWithHead(t, t.TempDir(), 77)
	restoreDir := t.TempDir()
	dbDir := path.Join(restoreDir, kv.BeaconNodeDbDirName)
	target := path.Join(dbDir, kv.DatabaseFileName)

	restoreErr, nodeErr := restoreWithWaitingNode(t, source, restoreDir, target, &bolt.Options{Timeout: 10 * time.Second}, true)
	require.ErrorContains(t, "disk full", restoreErr)
	require.ErrorIs(t, nodeErr, bolt.ErrInvalid, "the waiting node must not open the removed placeholder")
	entries, err := os.ReadDir(dbDir)
	require.NoError(t, err)
	require.Equal(t, 0, len(entries), "no database or temporary file may be left behind")
}

// When the directory sync after the rename fails, the old file is not
// invalidated (the zeros could outlive the rename on a power loss). The lock
// is then held past the node's open timeout, so a waiting node gives up
// instead of opening the replaced file; the restore reports the failed sync.
func TestRestore_DirectorySyncFailureSeesOffWaitingNodes(t *testing.T) {
	original := syncDirectory
	syncDirectory = func(string) error { return errors.New("sync failed") }
	defer func() { syncDirectory = original }()
	source := newDatabaseWithHead(t, t.TempDir(), 77)
	restoreDir := t.TempDir()
	dbDir := path.Join(restoreDir, kv.BeaconNodeDbDirName)
	target := path.Join(dbDir, kv.DatabaseFileName)

	restoreErr, nodeErr := restoreWithWaitingNode(t, source, restoreDir, target, &bolt.Options{Timeout: kv.OpenTimeout}, false)
	require.ErrorContains(t, "directory sync failed", restoreErr)
	require.ErrorIs(t, nodeErr, bolt.ErrTimeout, "a node waiting with the node's open timeout must give up")
	require.Equal(t, primitives.Slot(77), headSlot(t, dbDir), "the restored database is in place")
}

// The same when the restore fails and the placeholder is removed: with the
// directory sync failing the placeholder is not invalidated, and the lock is
// held until a waiting node has given up.
func TestRestore_FailedCopyWithDirectorySyncFailureSeesOffWaitingNodes(t *testing.T) {
	original := syncDirectory
	syncDirectory = func(string) error { return errors.New("sync failed") }
	defer func() { syncDirectory = original }()
	source := newDatabaseWithHead(t, t.TempDir(), 77)
	restoreDir := t.TempDir()
	dbDir := path.Join(restoreDir, kv.BeaconNodeDbDirName)
	target := path.Join(dbDir, kv.DatabaseFileName)

	restoreErr, nodeErr := restoreWithWaitingNode(t, source, restoreDir, target, &bolt.Options{Timeout: kv.OpenTimeout}, true)
	require.ErrorContains(t, "disk full", restoreErr)
	require.ErrorIs(t, nodeErr, bolt.ErrTimeout, "a node waiting with the node's open timeout must give up")
	entries, err := os.ReadDir(dbDir)
	require.NoError(t, err)
	require.Equal(t, 0, len(entries), "no database or temporary file may be left behind")
}

// bolt recovers from either meta page. On a system with 64 KiB pages the
// second meta page sits at 64 KiB, so clearing less than two pages would let a
// waiting node recover the stale database from it.
func TestRestore_NodeWaitingForTheLockCannotOpenTheReplacedFile_LargePages(t *testing.T) {
	source := newDatabaseWithHead(t, t.TempDir(), 77)
	restoreDir := t.TempDir()
	dbDir := path.Join(restoreDir, kv.BeaconNodeDbDirName)
	require.NoError(t, os.MkdirAll(dbDir, 0700))
	target := path.Join(dbDir, kv.DatabaseFileName)
	// The existing target, created on a 64 KiB-page system.
	largePages := &bolt.Options{Timeout: 10 * time.Second, PageSize: 64 << 10}
	existing, err := bolt.Open(target, params.BeaconIoConfig().ReadWritePermissions, largePages)
	require.NoError(t, err)
	require.NoError(t, existing.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("stale"))
		return err
	}))
	require.NoError(t, existing.Close())
	answerPrompt(t, "y")

	restoreErr, nodeErr := restoreWithWaitingNode(t, source, restoreDir, target, largePages, false)
	require.NoError(t, restoreErr)
	require.ErrorIs(t, nodeErr, bolt.ErrInvalid, "the waiting node must not recover the replaced file from its second meta page")
	require.Equal(t, primitives.Slot(77), headSlot(t, dbDir))
}

// A hard link to the database shares the replaced file, which then cannot be
// invalidated: a node waiting for the lock would open the stale database
// through the link. Such a target is refused before anything is changed.
func TestRestore_RefusesHardLinkedTarget(t *testing.T) {
	dataDir := t.TempDir()
	dbDir := path.Join(dataDir, kv.BeaconNodeDbDirName)
	dbFile := newDatabaseWithHead(t, dbDir, 11)
	linkDir := path.Join(dataDir, "backup")
	require.NoError(t, os.MkdirAll(linkDir, 0700))
	require.NoError(t, os.Link(dbFile, path.Join(linkDir, kv.DatabaseFileName)))
	source := newDatabaseWithHead(t, t.TempDir(), 5000)
	answerPrompt(t, "y")

	require.ErrorContains(t, "hard links", Restore(restoreContext(t, source, dataDir)))

	require.Equal(t, primitives.Slot(11), headSlot(t, dbDir), "the database must be untouched")
	require.Equal(t, primitives.Slot(11), headSlot(t, linkDir), "the link must be untouched")
	entries, err := os.ReadDir(dbDir)
	require.NoError(t, err)
	require.Equal(t, 1, len(entries), "no temporary file may be left behind")

	// With the other link gone the restore proceeds.
	require.NoError(t, os.Remove(path.Join(linkDir, kv.DatabaseFileName)))
	answerPrompt(t, "y")
	require.NoError(t, Restore(restoreContext(t, source, dataDir)))
	require.Equal(t, primitives.Slot(5000), headSlot(t, dbDir))
}

// A database file that is a symbolic link is restored at the link's target,
// which stays intact as a database; the link is not replaced by a plain file
// while the real file is overwritten.
func TestRestore_SymlinkedDatabaseIsRestoredAtItsTarget(t *testing.T) {
	realDir := t.TempDir()
	realFile := newDatabaseWithHead(t, realDir, 11)
	dataDir := t.TempDir()
	dbDir := path.Join(dataDir, kv.BeaconNodeDbDirName)
	require.NoError(t, os.MkdirAll(dbDir, 0700))
	link := path.Join(dbDir, kv.DatabaseFileName)
	require.NoError(t, os.Symlink(realFile, link))
	source := newDatabaseWithHead(t, t.TempDir(), 5000)
	answerPrompt(t, "y")

	require.NoError(t, Restore(restoreContext(t, source, dataDir)))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotEqual(t, os.FileMode(0), info.Mode()&os.ModeSymlink, "the link must remain a link")
	require.Equal(t, primitives.Slot(5000), headSlot(t, realDir), "the restore must land at the link's target")
	require.Equal(t, primitives.Slot(5000), headSlot(t, dbDir))
}

// The copy is a bolt snapshot taken under the source's shared lock: a source
// that a node has open is refused instead of being copied while it changes,
// and the snapshot opens as a database with the source's content.
func TestSnapshotDatabase(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	source := newDatabaseWithHead(t, srcDir, 77)

	// A node has the source open.
	node, err := kv.NewKVStore(ctx, srcDir)
	require.NoError(t, err)
	require.ErrorContains(t, "in use", snapshotDatabase(source, path.Join(t.TempDir(), "copy.db")))
	require.NoError(t, node.Close())

	dstDir := path.Join(t.TempDir(), kv.BeaconNodeDbDirName)
	require.NoError(t, os.MkdirAll(dstDir, 0700))
	require.NoError(t, snapshotDatabase(source, path.Join(dstDir, kv.DatabaseFileName)))
	require.Equal(t, primitives.Slot(77), headSlot(t, dstDir))
}
