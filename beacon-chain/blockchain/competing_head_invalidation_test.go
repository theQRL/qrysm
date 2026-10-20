package blockchain

import (
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/beacon-chain/verification"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

// TestReceiveBlock_CompetingHeadInvalidated covers an import whose forkchoice
// update rejects a previously imported head on another branch. The received
// block stays imported, becomes the head, and must still be announced, while
// the returned error names the other branch and carries no peer penalty. A
// block that is itself rejected must not be announced.
func TestReceiveBlock_CompetingHeadInvalidated(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, selfInvalid := range []bool{false, true} {
			name := map[bool]string{false: "gossip", true: "batch"}[batch] + "/" +
				map[bool]string{false: "other branch rejected", true: "received block rejected"}[selfInvalid]
			t.Run(name, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 3)
				// The received block is either the optimistic head's own child, or
				// a late sibling of that head (a child of blks[0] at slot 3).
				received := f.blks[2]
				if !selfInvalid {
					received, _ = emptyBranchBlock(t, f, f.states[1], 3, 'r')
				}
				receivedPayload, err := received.Block().Body().Execution()
				require.NoError(t, err)
				validPayload, err := f.blks[0].Block().Body().Execution()
				require.NoError(t, err)
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 4, 0)
					// Keep the optimistic head blks[1] selected over a late sibling.
					voteForRoot(t, f, f.blks[1].Root(), 0)
					// Execution rejects the head's branch back to blks[0]. The
					// replacement head's payload is accepted.
					f.engine.ErrNewPayload = execution.ErrAcceptedSyncingPayloadStatus
					f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
					f.engine.ForkChoiceUpdatedResp = validPayload.BlockHash()
					f.engine.OverrideValidHash = bytesutil.ToBytes32(validPayload.BlockHash())
					if !selfInvalid {
						f.engine.OverrideValidHash = bytesutil.ToBytes32(receivedPayload.BlockHash())
					}
					events := make(chan *feed.Event, 32)
					sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
					defer sub.Unsubscribe()

					if batch {
						err = f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{received})
					} else {
						err = f.s.ReceiveBlock(f.ctx, received, received.Root())
					}
					synctest.Wait()
					require.NotNil(t, err)
					require.Equal(t, true, IsInvalidBlock(err))
					require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(f.blks[1].Root()), "the rejected head must be removed")

					processed := 0
					for len(events) > 0 {
						ev := <-events
						if ev.Type != statefeed.BlockProcessed {
							continue
						}
						data := ev.Data.(*statefeed.BlockProcessedData)
						if data.BlockRoot == received.Root() {
							processed++
							assert.Equal(t, false, data.Optimistic, "execution validated the announced block")
						}
					}
					if selfInvalid {
						assert.Equal(t, received.Root(), InvalidBlockRoot(err))
						assert.Equal(t, false, IsUnrelatedBlockError(err))
						assert.Equal(t, true, errors.Is(err, verification.ErrInvalid), "a rejected block penalizes the peer")
						require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(received.Root()))
						assert.Equal(t, 0, processed, "a rejected block must not be announced")
						return
					}
					assert.Equal(t, f.blks[1].Root(), InvalidBlockRoot(err), "the error names the other branch")
					assert.Equal(t, true, IsUnrelatedBlockError(err))
					assert.Equal(t, false, errors.Is(err, verification.ErrInvalid), "no peer penalty for an accepted block")
					require.Equal(t, true, f.s.cfg.ForkChoiceStore.HasNode(received.Root()), "the received block stays imported")
					published, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					assert.Equal(t, received.Root(), bytesutil.ToBytes32(published), "the received block becomes the head")
					assert.Equal(t, 1, processed, "the imported block must be announced exactly once")
				})
			})
		}
	}
}
