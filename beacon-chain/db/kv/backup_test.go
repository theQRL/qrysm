package kv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestStore_Backup(t *testing.T) {
	db, err := NewKVStore(context.Background(), t.TempDir())
	require.NoError(t, err, "Failed to instantiate DB")
	ctx := context.Background()

	head := util.NewBeaconBlockZond()
	head.Block.Slot = 5000

	wsb, err := blocks.NewSignedBeaconBlock(head)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))
	root, err := head.Block.HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveHeadBlockRoot(ctx, root))

	require.NoError(t, db.Backup(ctx, "", false))

	backupsPath := filepath.Join(db.databasePath, backupsDirectoryName)
	files, err := os.ReadDir(backupsPath)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(files), "No backups created")
	require.NoError(t, db.Close(), "Failed to close database")

	oldFilePath := filepath.Join(backupsPath, files[0].Name())
	newFilePath := filepath.Join(backupsPath, DatabaseFileName)
	// We rename the file to match the database file name
	// our NewKVStore function expects when opening a database.
	require.NoError(t, os.Rename(oldFilePath, newFilePath))

	backedDB, err := NewKVStore(ctx, backupsPath)
	require.NoError(t, err, "Failed to instantiate DB")
	t.Cleanup(func() {
		require.NoError(t, backedDB.Close(), "Failed to close database")
	})
	require.Equal(t, true, backedDB.HasState(ctx, root))
}

func TestStore_BackupMultipleBuckets(t *testing.T) {
	db, err := NewKVStore(context.Background(), t.TempDir())
	require.NoError(t, err, "Failed to instantiate DB")
	ctx := context.Background()

	startSlot := primitives.Slot(5000)

	for i := startSlot; i < 5200; i++ {
		head := util.NewBeaconBlockZond()
		head.Block.Slot = i
		wsb, err := blocks.NewSignedBeaconBlock(head)
		require.NoError(t, err)
		require.NoError(t, db.SaveBlock(ctx, wsb))
		root, err := head.Block.HashTreeRoot()
		require.NoError(t, err)
		st, err := util.NewBeaconStateZond()
		require.NoError(t, st.SetSlot(i))
		require.NoError(t, err)
		require.NoError(t, db.SaveState(ctx, st, root))
		require.NoError(t, db.SaveHeadBlockRoot(ctx, root))
	}

	require.NoError(t, db.Backup(ctx, "", false))

	backupsPath := filepath.Join(db.databasePath, backupsDirectoryName)
	files, err := os.ReadDir(backupsPath)
	require.NoError(t, err)
	require.NotEqual(t, 0, len(files), "No backups created")
	require.NoError(t, db.Close(), "Failed to close database")

	oldFilePath := filepath.Join(backupsPath, files[0].Name())
	newFilePath := filepath.Join(backupsPath, DatabaseFileName)
	// We rename the file to match the database file name
	// our NewKVStore function expects when opening a database.
	require.NoError(t, os.Rename(oldFilePath, newFilePath))

	backedDB, err := NewKVStore(ctx, backupsPath)
	require.NoError(t, err, "Failed to instantiate DB")
	t.Cleanup(func() {
		require.NoError(t, backedDB.Close(), "Failed to close database")
	})
	for i := startSlot; i < 5200; i++ {
		head := util.NewBeaconBlockZond()
		head.Block.Slot = i
		root, err := head.Block.HashTreeRoot()
		require.NoError(t, err)
		nBlock, err := backedDB.Block(ctx, root)
		require.NoError(t, err)
		require.NotNil(t, nBlock)
		require.Equal(t, nBlock.Block().Slot(), i)
		nState, err := backedDB.State(ctx, root)
		require.NoError(t, err)
		require.NotNil(t, nState)
		require.Equal(t, nState.Slot(), i)
	}
}

func TestStore_Backup_ConcurrentBackupsPublishCompleteFile(t *testing.T) {
	db, err := NewKVStore(context.Background(), t.TempDir())
	require.NoError(t, err)
	ctx := context.Background()

	head := util.NewBeaconBlockZond()
	head.Block.Slot = 5000
	wsb, err := blocks.NewSignedBeaconBlock(head)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))
	root, err := head.Block.HashTreeRoot()
	require.NoError(t, err)
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, st, root))
	require.NoError(t, db.SaveHeadBlockRoot(ctx, root))

	const runs = 3
	errs := make([]error, runs)
	var wg sync.WaitGroup
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = db.Backup(ctx, "", false)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "backup %d failed", i)
	}

	backupsPath := filepath.Join(db.databasePath, backupsDirectoryName)
	files, err := os.ReadDir(backupsPath)
	require.NoError(t, err)
	require.Equal(t, 1, len(files), "expected exactly the published backup, got %v", files)
	require.Equal(t, false, strings.HasSuffix(files[0].Name(), ".tmp"))
	require.NoError(t, db.Close())

	require.NoError(t, os.Rename(filepath.Join(backupsPath, files[0].Name()), filepath.Join(backupsPath, DatabaseFileName)))
	backedDB, err := NewKVStore(ctx, backupsPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, backedDB.Close())
	})
	require.Equal(t, true, backedDB.HasBlock(ctx, root))
	require.Equal(t, true, backedDB.HasState(ctx, root))
	headBlock, err := backedDB.HeadBlock(ctx)
	require.NoError(t, err)
	require.NotNil(t, headBlock)
}

// Deleting a block removes its state and state summary in the same transaction.
