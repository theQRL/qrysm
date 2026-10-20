package blockchain

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type restoredVoteSummaryDB struct {
	db.HeadAccessDatabase
	root    [32]byte
	failure error
	fail    bool
}

func (d *restoredVoteSummaryDB) SaveStateSummary(ctx context.Context, summary *qrysmpb.StateSummary) error {
	if d.fail && bytesutil.ToBytes32(summary.Root) == d.root {
		d.fail = false
		return d.failure
	}
	return d.HeadAccessDatabase.SaveStateSummary(ctx, summary)
}

type interruptedAttestationRestore struct {
	forkchoice.ForkChoicer
	stopRoot           [32]byte
	restoreErr         error
	cancel             context.CancelFunc
	cancelAfterSuccess bool
	interrupted        bool
}

func (f *interruptedAttestationRestore) InsertChain(ctx context.Context, chain []*forktypes.BlockAndCheckpoints) error {
	if f.interrupted {
		return f.ForkChoicer.InsertChain(ctx, chain)
	}
	f.interrupted = true
	if f.cancelAfterSuccess {
		if err := f.ForkChoicer.InsertChain(ctx, chain); err != nil {
			return err
		}
		f.cancel()
		return nil
	}
	for i, b := range chain {
		if b.Block.Root() != f.stopRoot {
			continue
		}
		if err := f.ForkChoicer.InsertChain(ctx, chain[:i+1]); err != nil {
			return err
		}
		if f.cancel != nil {
			f.cancel()
			// Exercise the store's actual rollback of a cancelled suffix.
			return f.ForkChoicer.InsertChain(ctx, chain[i+1:])
		}
		return f.restoreErr
	}
	return f.ForkChoicer.InsertChain(ctx, chain)
}

func TestService_RestoreForkchoiceAttestations(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tc := range []struct {
		name               string
		healthy            bool
		partial            bool
		cancel             bool
		cancelAfterSuccess bool
		currentSlot        int64
	}{
		{name: "healthy control", healthy: true, currentSlot: 10},
		{name: "failed persistence", currentSlot: 10},
		{name: "historical votes", currentSlot: 18},
		{name: "retained prefix", partial: true, currentSlot: 10},
		{name: "cancelled suffix", partial: true, cancel: true, currentSlot: 10},
		{name: "cancelled vote processing", cancelAfterSuccess: true, currentSlot: 18},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 2)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			a, aState := emptyBranchBlock(t, f, f.states[2], 6, 'a')
			b, bState := emptyBranchBlock(t, f, f.states[2], 6, 'b')
			aRoot, bRoot := a.Root(), b.Root()
			if bytes.Compare(aRoot[:], bRoot[:]) > 0 {
				a, b, aState = b, a, bState
			}
			firstVoteBlock, firstVoteState := signedBatchSlashingBlock(t, f, aState, 7, 'v', nil)
			lastVoteBlock, lastVoteState := signedBatchSlashingBlock(t, f, firstVoteState, 8, 'v', nil)
			tip, _ := emptyBranchBlock(t, f, lastVoteState, 9, 't')
			branch := []blocks.ROBlock{a, firstVoteBlock, lastVoteBlock}
			var wantWeight, prefixWeight uint64
			var atts []*qrysmpb.Attestation
			for _, blk := range branch[1:] {
				require.Equal(t, true, len(blk.Block().Body().Attestations()) > 0)
				atts = append(atts, blk.Block().Body().Attestations()...)
				for _, att := range blk.Block().Body().Attestations() {
					indices, err := verifiedAttestingIndices(f.ctx, aState, att)
					require.NoError(t, err, "restored votes have genuine target-state signatures")
					for _, index := range indices {
						v, err := f.states[0].ValidatorAtIndexReadOnly(primitives.ValidatorIndex(index))
						require.NoError(t, err)
						wantWeight += v.EffectiveBalance()
						if blk.Root() == firstVoteBlock.Root() {
							prefixWeight += v.EffectiveBalance()
						}
					}
				}
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, tc.currentSlot, 30)
				fc := f.s.cfg.ForkChoiceStore
				fc.Lock()
				require.NoError(t, fc.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
				fc.Unlock()
				d := &restoredVoteSummaryDB{
					HeadAccessDatabase: f.s.cfg.BeaconDB,
					root:               lastVoteBlock.Root(),
					failure:            errors.New("temporary summary write failure"),
					fail:               !tc.healthy,
				}
				f.s.cfg.BeaconDB = d
				err := f.s.ReceiveBlockBatch(f.ctx, branch)
				if tc.healthy {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, d.failure)
					for _, blk := range branch {
						require.Equal(t, false, fc.HasNode(blk.Root()))
					}
					require.Equal(t, 0, len(f.s.cfg.AttPool.BlockAttestations()))
				}
				// A successful competing batch flushes the validated blocks left
				// by the failed write, without inserting them into forkchoice.
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{b}))
				require.Equal(t, true, d.HasBlock(f.ctx, lastVoteBlock.Root()))
				request, cancel := context.WithCancel(f.ctx)
				defer cancel()
				restoreErr := errors.New("interrupted ancestor restoration")
				if tc.partial || tc.cancelAfterSuccess {
					interrupted := &interruptedAttestationRestore{
						ForkChoicer: fc, stopRoot: firstVoteBlock.Root(), restoreErr: restoreErr,
						cancelAfterSuccess: tc.cancelAfterSuccess,
					}
					if tc.cancel || tc.cancelAfterSuccess {
						interrupted.cancel = cancel
						restoreErr = context.Canceled
					}
					f.s.cfg.ForkChoiceStore = interrupted
				}
				// Importing a child regenerates its pre-state and restores the
				// missing parent chain from DB, including both voting blocks.
				err = f.s.ReceiveBlockBatch(request, []blocks.ROBlock{tip})
				if tc.partial || tc.cancelAfterSuccess {
					require.ErrorIs(t, err, restoreErr)
					require.Equal(t, true, fc.HasNode(firstVoteBlock.Root()))
					require.Equal(t, !tc.partial, fc.HasNode(lastVoteBlock.Root()))
					require.Equal(t, false, fc.HasNode(tip.Root()))
					if tc.cancelAfterSuccess {
						pending := f.s.cfg.AttPool.BlockAttestations()
						slices.SortFunc(pending, func(a, b *qrysmpb.Attestation) int {
							return cmp.Compare(a.Data.Slot, b.Data.Slot)
						})
						require.DeepSSZEqual(t, atts, pending, "cancellation must queue every restored vote")
					}
					require.NoError(t, f.s.ReceiveBlock(f.ctx, firstVoteBlock, firstVoteBlock.Root()), "known restored blocks skip reprocessing")
					f.s.UpdateHead(f.ctx, f.s.CurrentSlot())
					synctest.Wait()
					weight, err := fc.Weight(a.Root())
					require.NoError(t, err)
					wantBeforeRetry, headBeforeRetry := wantWeight, lastVoteBlock.Root()
					if tc.partial {
						wantBeforeRetry, headBeforeRetry = prefixWeight, firstVoteBlock.Root()
					}
					require.Equal(t, wantBeforeRetry, weight, "only retained blocks contribute votes")
					require.Equal(t, headBeforeRetry, f.s.CachedHeadRoot())
					// The retry skips the retained prefix and must recover the
					// suffix's votes without double-counting earlier ones.
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{tip}))
				} else {
					require.NoError(t, err)
				}
				for _, blk := range branch {
					require.Equal(t, true, fc.HasNode(blk.Root()))
					require.NoError(t, f.s.ReceiveBlock(f.ctx, blk, blk.Root()))
				}
				for range 2 {
					f.s.UpdateHead(f.ctx, f.s.CurrentSlot())
					synctest.Wait()
					weight, err := fc.Weight(a.Root())
					require.NoError(t, err)
					published, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					assert.Equal(t, wantWeight, weight, "restored blocks retain their included votes")
					assert.Equal(t, tip.Root(), f.s.CachedHeadRoot(), "the voted branch beats the root tie-break")
					assert.Equal(t, tip.Root(), bytesutil.ToBytes32(published), "publish the voted branch")
					assert.Equal(t, 0, len(f.s.cfg.AttPool.BlockAttestations()))
				}
			})
		})
	}
}
