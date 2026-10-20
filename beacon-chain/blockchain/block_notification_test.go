package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type notificationInterruptedStore struct {
	forkchoice.ForkChoicer
	cancel  context.CancelFunc
	failure error
}

func (f *notificationInterruptedStore) InsertNode(ctx context.Context, st state.BeaconState, b blocks.ROBlock) error {
	err := f.ForkChoicer.InsertNode(ctx, st, b)
	if err == nil && f.cancel != nil {
		f.cancel()
	}
	return err
}

func (f *notificationInterruptedStore) InsertChain(ctx context.Context, chain []*forktypes.BlockAndCheckpoints) error {
	if f.failure != nil {
		if err := f.ForkChoicer.InsertChain(ctx, chain[:1]); err != nil {
			return err
		}
		return f.failure
	}
	err := f.ForkChoicer.InsertChain(ctx, chain)
	if err == nil && f.cancel != nil {
		f.cancel()
	}
	return err
}

func processedBlockCounts(events chan *feed.Event) map[[32]byte]int {
	counts := make(map[[32]byte]int)
	for len(events) != 0 {
		event := <-events
		if event.Type == statefeed.BlockProcessed {
			counts[event.Data.(*statefeed.BlockProcessedData).BlockRoot]++
		}
	}
	return counts
}

func TestReceiveBlock_RetainedNotificationAfterCancellation(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, batch := range []bool{false, true} {
		for _, cancelImport := range []bool{false, true} {
			name := map[bool]string{false: "gossip", true: "batch"}[batch] + "/" + map[bool]string{false: "healthy control", true: "cancelled after insertion"}[cancelImport]
			t.Run(name, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 3)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 4, 0)
					ctx, cancel := context.WithCancel(f.ctx)
					defer cancel()
					fc := f.s.cfg.ForkChoiceStore
					if cancelImport {
						f.s.cfg.ForkChoiceStore = &notificationInterruptedStore{ForkChoicer: fc, cancel: cancel}
					}
					events := make(chan *feed.Event, 32)
					sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
					defer sub.Unsubscribe()
					b := f.blks[2]
					var err error
					if batch {
						err = f.s.ReceiveBlockBatch(ctx, f.blks[2:])
					} else {
						err = f.s.ReceiveBlock(ctx, b, b.Root())
					}
					if cancelImport {
						require.ErrorIs(t, err, context.Canceled)
					} else {
						require.NoError(t, err)
					}
					synctest.Wait() // Finish head notifications before restoring the store.
					f.s.cfg.ForkChoiceStore = fc
					require.Equal(t, true, fc.HasNode(b.Root()))
					f.s.UpdateHead(f.ctx, 4)
					synctest.Wait()
					require.NoError(t, f.s.ReceiveBlock(f.ctx, b, b.Root()))
					synctest.Wait()
					head, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					require.Equal(t, b.Root(), bytesutil.ToBytes32(head))
					assert.Equal(t, 1, processedBlockCounts(events)[b.Root()], "retained imports must be announced exactly once")
				})
			})
		}
	}
}

func TestReceiveBlockBatch_RetainedPrefixNotifications(t *testing.T) {
	for _, invalidTail := range []bool{false, true} {
		t.Run(map[bool]string{false: "insertion failure and retry", true: "execution rejects tail"}[invalidTail], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 4)
			fc := f.s.cfg.ForkChoiceStore
			failure := errors.New("temporary insertion failure")
			if invalidTail {
				payload, err := f.blks[2].Block().Body().Execution()
				require.NoError(t, err)
				f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
				f.engine.ForkChoiceUpdatedResp = payload.BlockHash()
				f.engine.OverrideValidHash = bytesutil.ToBytes32(payload.BlockHash())
			} else {
				f.s.cfg.ForkChoiceStore = &notificationInterruptedStore{ForkChoicer: fc, failure: failure}
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 5, 0)
				events := make(chan *feed.Event, 32)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				err := f.s.ReceiveBlockBatch(f.ctx, f.blks[2:])
				if invalidTail {
					require.Equal(t, true, IsInvalidBlock(err))
					require.Equal(t, f.blks[3].Root(), InvalidBlockRoot(err))
				} else {
					require.ErrorIs(t, err, failure)
				}
				require.Equal(t, true, fc.HasNode(f.blks[2].Root()))
				require.Equal(t, false, fc.HasNode(f.blks[3].Root()))
				synctest.Wait()
				counts := processedBlockCounts(events)
				assert.Equal(t, 1, counts[f.blks[2].Root()], "announce the retained prefix")
				assert.Equal(t, 0, counts[f.blks[3].Root()], "never announce the rejected tail")
				if invalidTail {
					return
				}
				f.s.cfg.ForkChoiceStore = fc
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				synctest.Wait()
				counts = processedBlockCounts(events)
				assert.Equal(t, 0, counts[f.blks[2].Root()], "a retried prefix must not be announced twice")
				assert.Equal(t, 1, counts[f.blks[3].Root()])
			})
		})
	}
}
