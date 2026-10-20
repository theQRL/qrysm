package blockchain

import (
	"context"
	"runtime/debug"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

// delayedParentStateDB pauses the first database read of one state so that a
// test can remove that state while the read is in flight.
type delayedParentStateDB struct {
	db.HeadAccessDatabase
	root             [32]byte
	entered, release chan struct{}
	once             sync.Once
}

func (d *delayedParentStateDB) State(ctx context.Context, root [32]byte) (state.BeaconState, error) {
	if root == d.root {
		d.once.Do(func() {
			close(d.entered)
			<-d.release
		})
	}
	return d.HeadAccessDatabase.State(ctx, root)
}

// TestReceiveBlock_ParentStateRemovedDuringRead covers a parent state that is
// deleted from the database between the state loader's existence check and its
// read. getBlockPreState runs before ReceiveBlock takes the forkchoice lock, so
// invalid-block pruning (or the hot-state DB mode being switched off) under the
// lock can remove the state mid-lookup. The database then reports a nil state
// without an error; the import must fail with an error instead of
// dereferencing the nil state.
func TestReceiveBlock_ParentStateRemovedDuringRead(t *testing.T) {
	for _, invalidate := range []bool{false, true} {
		name := "control"
		if invalidate {
			name = "parent invalidated during read"
		}
		t.Run(name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 3)
			parent := f.blks[1].Root()
			require.NoError(t, f.s.cfg.BeaconDB.SaveState(f.ctx, f.states[2], parent))
			d := &delayedParentStateDB{
				HeadAccessDatabase: f.s.cfg.BeaconDB,
				root:               parent,
				entered:            make(chan struct{}),
				release:            make(chan struct{}),
			}
			f.s.cfg.BeaconDB = d
			// Fresh, empty state caches so that the parent state is read
			// through the HasState -> State database path.
			f.s.cfg.StateGen = stategen.New(d, f.s.cfg.ForkChoiceStore)
			payload, err := f.blks[0].Block().Body().Execution()
			require.NoError(t, err)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 4, 0)
				type outcome struct {
					err        error
					panicValue any
					stack      []byte
				}
				result := make(chan outcome, 1)
				go func() {
					var got outcome
					defer func() {
						if p := recover(); p != nil {
							got.panicValue, got.stack = p, debug.Stack()
						}
						result <- got
					}()
					got.err = f.s.ReceiveBlock(f.ctx, f.blks[2], f.blks[2].Root())
				}()
				<-d.entered
				if invalidate {
					f.s.cfg.ForkChoiceStore.Lock()
					pruneErr := f.s.pruneInvalidBlock(f.ctx, f.blks[2].Root(), parent, bytesutil.ToBytes32(payload.BlockHash()))
					f.s.cfg.ForkChoiceStore.Unlock()
					require.Equal(t, true, IsInvalidBlock(pruneErr))
					require.Equal(t, false, d.HasState(f.ctx, parent), "the parent state must be gone before the read resumes")
				}
				close(d.release)
				got := <-result
				if got.panicValue != nil {
					t.Fatalf("ReceiveBlock panicked: %v\n%s", got.panicValue, got.stack)
				}
				if !invalidate {
					require.NoError(t, got.err)
					return
				}
				require.NotNil(t, got.err, "a parent state removed mid-read must fail the import")
				require.ErrorContains(t, "could not get block's prestate", got.err)
			})
		})
	}
}

// parentSummaryRecoveryDB exposes the gap between DeleteBlock removing a
// summary and removing the block itself. Only StateGen uses this wrapper.
type parentSummaryRecoveryDB struct {
	db.HeadAccessDatabase
	parent          [32]byte
	summaryReads    int
	beforeBlockRead func()
}

func (d *parentSummaryRecoveryDB) StateSummary(ctx context.Context, root [32]byte) (*qrysmpb.StateSummary, error) {
	if root == d.parent {
		d.summaryReads++
		return nil, nil
	}
	return d.HeadAccessDatabase.StateSummary(ctx, root)
}

func (d *parentSummaryRecoveryDB) Block(ctx context.Context, root [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	if root == d.parent && d.beforeBlockRead != nil {
		beforeRead := d.beforeBlockRead
		d.beforeBlockRead = nil
		beforeRead()
	}
	return d.HeadAccessDatabase.Block(ctx, root)
}

func TestReceiveBlock_ParentRemovedDuringSummaryRecovery(t *testing.T) {
	for _, invalidate := range []bool{false, true} {
		name := "control"
		if invalidate {
			name = "parent invalidated during summary recovery"
		}
		t.Run(name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 3)
			parent := f.blks[1].Root()
			d := &parentSummaryRecoveryDB{HeadAccessDatabase: f.s.cfg.BeaconDB, parent: parent}
			require.Equal(t, false, d.HasState(f.ctx, parent))
			// Keep the preceding state, but force the parent through summary
			// recovery and replay instead of serving its cached state.
			f.s.cfg.StateGen = stategen.New(d, f.s.cfg.ForkChoiceStore)
			require.NoError(t, f.s.cfg.StateGen.SaveState(f.ctx, f.blks[0].Root(), f.states[1].Copy()))
			payload, err := f.blks[0].Block().Body().Execution()
			require.NoError(t, err)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 4, 0)
				if invalidate {
					d.beforeBlockRead = func() {
						require.Equal(t, 1, d.summaryReads)
						require.Equal(t, true, d.HasBlock(f.ctx, parent))
						f.s.cfg.ForkChoiceStore.Lock()
						pruneErr := f.s.pruneInvalidBlock(f.ctx, f.blks[2].Root(), parent, bytesutil.ToBytes32(payload.BlockHash()))
						f.s.cfg.ForkChoiceStore.Unlock()
						require.Equal(t, true, IsInvalidBlock(pruneErr))
						require.Equal(t, false, d.HasBlock(f.ctx, parent))
					}
				}
				err := f.s.ReceiveBlock(f.ctx, f.blks[2], f.blks[2].Root())
				require.Equal(t, 1, d.summaryReads)
				if !invalidate {
					require.NoError(t, err)
					require.Equal(t, true, f.s.InForkchoice(f.blks[2].Root()))
					return
				}
				require.ErrorContains(t, "could not find block in DB", err)
				require.Equal(t, false, IsInvalidBlock(err), "a failed state read must not blame the incoming block")
				require.Equal(t, false, f.s.InForkchoice(f.blks[2].Root()))
				require.Equal(t, false, d.HasStateSummary(f.ctx, parent), "must not recreate a deleted parent's summary")
			})
		})
	}
}
