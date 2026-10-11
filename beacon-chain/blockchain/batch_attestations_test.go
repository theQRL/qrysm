package blockchain

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"

	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestService_ReceiveBlockBatch_PrefixAttestations(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tc := range []struct {
		name         string
		fail, cancel bool
		replay       bool
		currentSlot  primitives.Slot
	}{
		{name: "successful batch control", currentSlot: 18},
		{name: "failed batch", fail: true, currentSlot: 18},
		{name: "cancelled insertion", fail: true, cancel: true, currentSlot: 18},
		{name: "failed historical batch", fail: true, currentSlot: 30},
		{name: "failed batch replay votes control", fail: true, replay: true, currentSlot: 18},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 12)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			pre := f.states[12].Copy()
			a, aState := signedBatchSlashingBlock(t, f, pre, 13, 'a', nil)
			b, bState := signedBatchSlashingBlock(t, f, pre, 13, 'b', nil)
			aRoot, bRoot := a.Root(), b.Root()
			if bytes.Compare(aRoot[:], bRoot[:]) > 0 {
				a, b, aState = b, a, bState
			}
			branch := []blocks.ROBlock{a}
			var fullWeight, retainedWeight uint64
			for slot := primitives.Slot(14); slot <= 17; slot++ {
				blk, post := signedBatchSlashingBlock(t, f, aState, slot, 'a', nil)
				aState = post
				branch = append(branch, blk)
				for _, att := range blk.Block().Body().Attestations() {
					indices, err := verifiedAttestingIndices(f.ctx, pre, att)
					require.NoError(t, err, "included votes have genuine target-state signatures")
					// These disjoint slot committees vote for A's branch. The
					// rolled-back slot-17 block must not contribute its votes.
					for _, index := range indices {
						validator, err := f.states[6].ValidatorAtIndexReadOnly(primitives.ValidatorIndex(index))
						require.NoError(t, err)
						fullWeight += validator.EffectiveBalance()
						if slot < 17 {
							retainedWeight += validator.EffectiveBalance()
						}
					}
				}
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, int64(tc.currentSlot), 0)
				fc := f.s.cfg.ForkChoiceStore
				fc.Lock()
				err := fc.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot})
				fc.Unlock()
				require.NoError(t, err)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				require.NoError(t, f.s.ReceiveBlock(f.ctx, b, b.Root()))
				synctest.Wait()
				require.Equal(t, b.Root(), f.s.CachedHeadRoot())
				readErr := errors.New("temporary epoch-2 balance read failure")
				batchCtx, cancel := context.WithCancel(f.ctx)
				defer cancel()
				if tc.fail {
					fc.SetBalancesByRooter(func(ctx context.Context, cp *forktypes.Checkpoint) (*forktypes.JustifiedBalances, error) {
						if cp.Epoch == 2 {
							if tc.cancel {
								cancel()
								return nil, ctx.Err()
							}
							return nil, readErr
						}
						return f.s.cfg.StateGen.BalancesByCheckpoint(ctx, cp)
					})
				}
				err = f.s.ReceiveBlockBatch(batchCtx, branch)
				wantHead := branch[len(branch)-1].Root()
				wantWeight := fullWeight
				if !tc.fail {
					require.NoError(t, err)
				} else {
					wantErr := readErr
					if tc.cancel {
						wantErr = context.Canceled
					}
					require.ErrorIs(t, err, wantErr)
					require.Equal(t, primitives.Epoch(1), fc.JustifiedCheckpoint().Epoch)
					wantHead = branch[len(branch)-2].Root()
					wantWeight = retainedWeight
					require.Equal(t, true, fc.HasNode(wantHead), "slot 16 survives")
					require.Equal(t, false, fc.HasNode(branch[len(branch)-1].Root()), "slot 17 fails")
				}
				synctest.Wait()
				fc.SetBalancesByRooter(f.s.cfg.StateGen.BalancesByCheckpoint)
				require.NoError(t, f.s.saveInitSyncBlocks(f.ctx, true))
				for _, blk := range branch[:len(branch)-1] {
					require.Equal(t, true, fc.HasNode(blk.Root()))
					require.NoError(t, f.s.ReceiveBlock(f.ctx, blk, blk.Root()), "duplicate import must not lose included votes")
				}
				if tc.replay {
					fc.Lock()
					for _, blk := range branch[:len(branch)-1] {
						for _, att := range blk.Block().Body().Attestations() {
							if err := f.s.OnAttestation(f.ctx, att, 0); err != nil {
								fc.Unlock()
								t.Fatal(err)
							}
						}
					}
					fc.Unlock()
				}
				f.s.UpdateHead(f.ctx, tc.currentSlot)
				synctest.Wait()
				fc.Lock()
				weight, err := fc.Weight(a.Root())
				head, headErr := fc.Head(f.ctx)
				fc.Unlock()
				require.NoError(t, err)
				require.NoError(t, headErr)
				assert.Equal(t, wantWeight, weight, "only retained blocks must contribute their included votes")
				assert.Equal(t, wantHead, head, "the attested branch must beat the larger-root competitor")
				assert.Equal(t, wantHead, f.s.CachedHeadRoot(), "the wrong branch must not be published")
			})
		})
	}
}
