package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	"github.com/theQRL/qrysm/beacon-chain/verification"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type cancelBatchOnParentLookup struct {
	forkchoice.ForkChoicer
	parent [32]byte
	before func()
}

func (f *cancelBatchOnParentLookup) HasNode(root [32]byte) bool {
	has := f.ForkChoicer.HasNode(root)
	if root == f.parent && has && f.before != nil {
		before := f.before
		f.before = nil
		before()
	}
	return has
}

func TestReceiveBlockBatch_ContextErrorsAreNotInvalid(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "canceled", true: "deadline exceeded"}[deadline], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 3)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 4, 0)
				ctx, cancel := context.WithCancel(f.ctx)
				wantErr := context.Canceled
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(f.ctx, time.Second)
					wantErr = context.DeadlineExceeded
				}
				defer cancel()
				before := func() { cancel() }
				if deadline {
					// Sleep past the deadline on the bubble clock: at exactly the
					// deadline the timer and the sleeper wake at the same instant
					// and the import could observe a still-live context.
					before = func() { time.Sleep(2 * time.Second) }
				}
				// Cancel after retrieving the parent state, just before the
				// consensus transition starts, to exercise its error handling.
				fc := f.s.cfg.ForkChoiceStore
				f.s.cfg.ForkChoiceStore = &cancelBatchOnParentLookup{
					ForkChoicer: fc, parent: f.blks[1].Root(), before: before,
				}
				err := f.s.ReceiveBlockBatch(ctx, f.blks[2:])
				f.s.cfg.ForkChoiceStore = fc
				require.ErrorIs(t, err, wantErr)
				assert.Equal(t, false, IsInvalidBlock(err))
				assert.Equal(t, false, errors.Is(err, verification.ErrInvalid), "local interruption must not penalize the peer")
				assert.Equal(t, false, f.s.HasBlock(f.ctx, f.blks[2].Root()))
				assert.Equal(t, false, fc.HasNode(f.blks[2].Root()))
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]), "the same valid batch must remain retryable")
			})
		})
	}
}

func TestReceiveBlock_ExecutionHashErrorClassification(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, invalidHash := range []bool{false, true} {
			name := map[bool]string{false: "gossip", true: "batch"}[batch] + "/" +
				map[bool]string{false: "RPC failure", true: "INVALID_BLOCK_HASH"}[invalidHash]
			t.Run(name, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 3)
				f.engine.ErrNewPayload = errors.New("temporary execution RPC failure")
				if invalidHash {
					f.engine.ErrNewPayload = execution.ErrInvalidBlockHashPayloadStatus
				}
				// Even if a hash-rejection response includes an ancestor hash,
				// it does not establish that the block's ancestry is invalid.
				payload, err := f.blks[0].Block().Body().Execution()
				require.NoError(t, err)
				f.engine.NewPayloadResp = payload.BlockHash()
				receive := func() error {
					if batch {
						return f.s.ReceiveBlockBatch(f.ctx, f.blks[2:])
					}
					return f.s.ReceiveBlock(f.ctx, f.blks[2], f.blks[2].Root())
				}
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 4, 0)
					err := receive()
					require.NotNil(t, err)
					assert.Equal(t, invalidHash, IsInvalidBlock(err))
					assert.Equal(t, invalidHash, errors.Is(err, verification.ErrInvalid))
					assert.Equal(t, [32]byte{}, InvalidBlockLVH(err))
					assert.Equal(t, 0, len(InvalidAncestorRoots(err)))
					assert.Equal(t, false, f.s.HasBlock(f.ctx, f.blks[2].Root()))
					assert.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(f.blks[2].Root()))
					for _, b := range f.blks[:2] {
						assert.Equal(t, true, f.s.cfg.ForkChoiceStore.HasNode(b.Root()), "ancestors must remain available")
						assert.Equal(t, true, f.s.cfg.BeaconDB.HasBlock(f.ctx, b.Root()))
					}
					f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
					require.NoError(t, receive())
				})
			})
		}
	}
}
