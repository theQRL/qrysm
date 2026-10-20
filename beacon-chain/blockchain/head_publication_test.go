package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type headPublicationDB struct {
	db.HeadAccessDatabase
	root     [32]byte
	failure  error
	attempts int
	failures int
}

func (d *headPublicationDB) SaveHeadBlockRoot(ctx context.Context, root [32]byte) error {
	if root == d.root {
		d.attempts++
		if d.attempts <= d.failures {
			return d.failure
		}
	}
	return d.HeadAccessDatabase.SaveHeadBlockRoot(ctx, root)
}

func TestService_HeadPublicationRetry(t *testing.T) {
	for _, mode := range []string{"extension", "competing branch", "invalidation rollback"} {
		for _, fail := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/healthy control", true: "/transient writes fail"}[fail], func(t *testing.T) {
				f := newBatchExecutionFixture(t, 3)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				incoming := f.blks[2]
				if mode == "competing branch" {
					incoming, _ = emptyBranchBlock(t, f, f.states[1], 3, 'r')
				}
				wantRoot := incoming.Root()
				if mode == "invalidation rollback" {
					wantRoot = f.blks[0].Root()
					payload, err := f.blks[0].Block().Body().Execution()
					require.NoError(t, err)
					f.engine.ErrNewPayload = execution.ErrAcceptedSyncingPayloadStatus
					f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
					f.engine.ForkChoiceUpdatedResp = payload.BlockHash()
					f.engine.OverrideValidHash = bytesutil.ToBytes32(payload.BlockHash())
				}
				oldRoot := f.blks[1].Root()
				require.NoError(t, f.s.cfg.BeaconDB.SaveHeadBlockRoot(f.ctx, oldRoot))
				d := &headPublicationDB{
					HeadAccessDatabase: f.s.cfg.BeaconDB,
					root:               wantRoot,
					failure:            errors.New("temporary head write failure"),
				}
				if fail {
					d.failures = 2
				}
				f.s.cfg.BeaconDB = d
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 3, 0)
					f.s.cfg.ForkChoiceStore.Lock()
					require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
					f.s.cfg.ForkChoiceStore.Unlock()
					events := make(chan *feed.Event, 16)
					sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
					defer sub.Unsubscribe()
					err := f.s.ReceiveBlock(f.ctx, incoming, incoming.Root())
					if fail {
						require.ErrorIs(t, err, d.failure, "head write failure must reach the block importer")
					} else if mode == "invalidation rollback" {
						require.Equal(t, true, IsInvalidBlock(err))
					} else {
						require.NoError(t, err)
					}
					synctest.Wait()
					assertUnpublished := func() {
						t.Helper()
						published, err := f.s.HeadRoot(f.ctx)
						require.NoError(t, err)
						require.Equal(t, oldRoot, bytesutil.ToBytes32(published))
						persisted, err := d.HeadBlockRoot()
						require.NoError(t, err)
						require.Equal(t, oldRoot, persisted)
						for len(events) > 0 {
							typ := (<-events).Type
							assert.NotEqual(t, statefeed.NewHead, typ)
							assert.NotEqual(t, statefeed.Reorg, typ)
						}
						if mode == "invalidation rollback" {
							require.Equal(t, 1, len(f.s.invalidatedHeadBlocks), "retain the old ancestry until publication succeeds")
						}
					}
					if fail {
						assertUnpublished()
					}
					if mode == "competing branch" {
						// Keep the replacement selected after its proposer boost expires.
						indices := make([]uint64, len(f.keys))
						for i := range indices {
							indices[i] = uint64(i)
						}
						f.s.cfg.ForkChoiceStore.Lock()
						f.s.cfg.ForkChoiceStore.ProcessAttestation(f.ctx, indices, wantRoot, 0)
						f.s.cfg.ForkChoiceStore.Unlock()
					}
					for slot := primitives.Slot(4); slot <= 6; slot++ {
						driftGenesisTime(f.s, int64(slot), 0)
						require.NoError(t, f.s.NewSlot(f.ctx, slot))
						f.s.UpdateHead(f.ctx, slot)
						synctest.Wait()
						if fail && slot == 4 {
							assertUnpublished()
						}
					}
					published, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					require.Equal(t, wantRoot, bytesutil.ToBytes32(published))
					persisted, err := d.HeadBlockRoot()
					require.NoError(t, err)
					require.Equal(t, wantRoot, persisted)
					require.Equal(t, true, d.HasBlock(f.ctx, persisted))
					require.Equal(t, d.failures+1, d.attempts)
					require.Equal(t, 0, len(f.s.invalidatedHeadBlocks))
					heads, reorgs := 0, 0
					for len(events) > 0 {
						event := <-events
						switch event.Type {
						case statefeed.NewHead:
							heads++
						case statefeed.Reorg:
							reorgs++
							reorg := event.Data.(*qrlpb.EventChainReorg)
							assert.Equal(t, oldRoot, bytesutil.ToBytes32(reorg.OldHeadBlock))
							assert.Equal(t, wantRoot, bytesutil.ToBytes32(reorg.NewHeadBlock))
						}
					}
					require.Equal(t, 1, heads, "publish the head event exactly once")
					wantReorgs := 1
					if mode == "extension" {
						wantReorgs = 0
					}
					require.Equal(t, wantReorgs, reorgs, "failed writes must not emit duplicate reorg events")
				})
			})
		}
	}
}
