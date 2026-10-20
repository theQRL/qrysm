package blockchain

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/db"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

type delayedInitSyncSaveDB struct {
	db.HeadAccessDatabase
	entered, release chan struct{}
	delayed          bool
	failure          error
}

func (d *delayedInitSyncSaveDB) SaveBlocks(ctx context.Context, blks []interfaces.ReadOnlySignedBeaconBlock) error {
	if !d.delayed {
		d.delayed = true
		close(d.entered)
		<-d.release
		if d.failure != nil {
			return d.failure
		}
	}
	return d.HeadAccessDatabase.SaveBlocks(ctx, blks)
}

func TestService_SaveInitSyncBlocksSnapshot(t *testing.T) {
	for _, clearCache := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("clear=%v/fail=%v", clearCache, fail), func(t *testing.T) {
				ctx := context.Background()
				d := &delayedInitSyncSaveDB{
					HeadAccessDatabase: testDB.SetupDB(t),
					entered:            make(chan struct{}), release: make(chan struct{}),
				}
				if fail {
					d.failure = errors.New("temporary cache write failure")
				}
				s := &Service{cfg: &config{BeaconDB: d}, initSyncBlocks: make(map[[32]byte]interfaces.ReadOnlySignedBeaconBlock)}
				var blks []blocks.ROBlock
				for i := 0; i < 2; i++ {
					pb := util.NewBeaconBlockZond()
					pb.Block.Body.Graffiti[0] = byte(i)
					b, err := blocks.NewSignedBeaconBlock(pb)
					require.NoError(t, err)
					ro, err := blocks.NewROBlock(b)
					require.NoError(t, err)
					blks = append(blks, ro)
				}
				require.NoError(t, s.saveInitSyncBlock(ctx, blks[0].Root(), blks[0]))
				result := make(chan error, 1)
				go func() { result <- s.saveInitSyncBlocks(ctx, clearCache) }()
				<-d.entered
				// This block is added after the writer took its snapshot.
				insertErr := s.saveInitSyncBlock(ctx, blks[1].Root(), blks[1])
				close(d.release)
				err := <-result
				require.NoError(t, insertErr)
				if fail {
					require.ErrorIs(t, err, d.failure)
				} else {
					require.NoError(t, err)
				}
				require.Equal(t, fail || !clearCache, s.hasInitSyncBlock(blks[0].Root()))
				require.Equal(t, true, s.hasInitSyncBlock(blks[1].Root()), "must keep blocks added during the write")
				require.Equal(t, !fail, d.HasBlock(ctx, blks[0].Root()))
				require.Equal(t, false, d.HasBlock(ctx, blks[1].Root()))
				require.NoError(t, s.saveInitSyncBlocks(ctx, true))
				for _, b := range blks {
					require.Equal(t, true, d.HasBlock(ctx, b.Root()), "a later save must persist every remaining block")
					require.Equal(t, false, s.hasInitSyncBlock(b.Root()))
				}
			})
		}
	}
}

func TestService_InvalidBlockCleanupDuringCacheSave(t *testing.T) {
	f := newBatchExecutionFixture(t, 6)
	alt, _ := emptyBranchBlock(t, f, f.states[3], 4, 'z')
	payload, err := f.blks[1].Block().Body().Execution()
	require.NoError(t, err)
	require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:5]))
	parent := f.blks[2].Root()
	hasState, err := f.s.cfg.StateGen.HasState(f.ctx, parent)
	require.NoError(t, err)
	require.Equal(t, false, hasState, "C3 naturally has no cached batch state")
	require.Equal(t, true, f.s.hasInitSyncBlock(parent))
	d := &delayedInitSyncSaveDB{
		HeadAccessDatabase: f.s.cfg.BeaconDB,
		entered:            make(chan struct{}), release: make(chan struct{}),
	}
	f.s.cfg.BeaconDB = d
	result := make(chan error, 1)
	go func() { result <- f.s.ReceiveBlock(f.ctx, alt, alt.Root()) }()
	<-d.entered
	invalidResult := make(chan error, 1)
	go func() {
		// Exercise the same invalidation used for an INVALID NewPayload
		// response, while the gossip pre-state lookup is saving old blocks.
		f.s.cfg.ForkChoiceStore.Lock()
		err := f.s.pruneInvalidBlock(f.ctx, f.blks[5].Root(), f.blks[4].Root(), bytesutil.ToBytes32(payload.BlockHash()))
		f.s.cfg.ForkChoiceStore.Unlock()
		invalidResult <- err
	}()
	// Cleanup must wait for the write. Use a bounded wait rather than
	// synctest.Wait: blocking on a mutex is not durably blocked in synctest.
	completed := false
	select {
	case err = <-invalidResult:
		completed = true
	case <-time.After(200 * time.Millisecond):
	}
	close(d.release)
	if !completed {
		err = <-invalidResult
	}
	require.Equal(t, true, IsInvalidBlock(err))
	assert.Equal(t, false, completed, "cleanup must not finish before the older write")
	require.NotNil(t, <-result, "gossip must not import a child of the removed branch")
	f.s.UpdateHead(f.ctx, f.s.CurrentSlot())
	for _, b := range f.blks[2:5] {
		assert.Equal(t, false, d.HasBlock(f.ctx, b.Root()), "the old snapshot must not restore invalid blocks")
		assert.Equal(t, false, f.s.HasBlock(f.ctx, b.Root()))
		assert.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(b.Root()))
	}
	err = f.s.ReceiveBlockBatch(f.ctx, f.blks[3:5])
	assert.NotNil(t, err, "batch must not backfill invalid C3 after the write completes")
	assert.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(parent))
	published, err := f.s.HeadRoot(f.ctx)
	require.NoError(t, err)
	assert.Equal(t, f.blks[1].Root(), bytesutil.ToBytes32(published))
}
