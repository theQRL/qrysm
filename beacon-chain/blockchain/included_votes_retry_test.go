package blockchain

import (
	"bytes"
	"context"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

// includedVoteCancelContext starts counting cancellation checks only after
// insertion completes, so consensus validation runs with a healthy context.
type includedVoteCancelContext struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (c *includedVoteCancelContext) Err() error {
	if c.remaining > 0 {
		c.remaining--
		if c.remaining == 0 {
			c.cancel()
		}
	}
	return c.Context.Err()
}

type includedVoteCancelStore struct {
	forkchoice.ForkChoicer
	request *includedVoteCancelContext
	checks  int
}

func (s *includedVoteCancelStore) InsertNode(ctx context.Context, st state.BeaconState, b blocks.ROBlock) error {
	if err := s.ForkChoicer.InsertNode(ctx, st, b); err != nil {
		return err
	}
	s.request.remaining = s.checks
	return nil
}

func (s *includedVoteCancelStore) InsertChain(ctx context.Context, chain []*forktypes.BlockAndCheckpoints) error {
	if err := s.ForkChoicer.InsertChain(ctx, chain); err != nil {
		return err
	}
	s.request.remaining = s.checks
	return nil
}

func TestService_IncludedVotesSurviveCancelledImport(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, batch := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			checks int
		}{
			{name: "healthy control"},
			{name: "before verification", checks: 1},
			{name: "during committee lookup", checks: 2},
		} {
			t.Run(map[bool]string{false: "gossip/", true: "batch/"}[batch]+tc.name, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 2)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				a, aState := emptyBranchBlock(t, f, f.states[2], 6, 'a')
				b, bState := emptyBranchBlock(t, f, f.states[2], 6, 'b')
				aRoot, bRoot := a.Root(), b.Root()
				if bytes.Compare(aRoot[:], bRoot[:]) > 0 {
					a, b, aState = b, a, bState
				}
				child, _ := signedBatchSlashingBlock(t, f, aState, 7, 'c', nil)
				atts := child.Block().Body().Attestations()
				require.Equal(t, true, len(atts) > 0)
				var wantWeight uint64
				for _, att := range atts {
					require.Equal(t, a.Root(), bytesutil.ToBytes32(att.Data.BeaconBlockRoot))
					indices, err := verifiedAttestingIndices(f.ctx, aState, att)
					require.NoError(t, err)
					for _, index := range indices {
						v, err := f.states[0].ValidatorAtIndexReadOnly(primitives.ValidatorIndex(index))
						require.NoError(t, err)
						wantWeight += v.EffectiveBalance()
					}
				}
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 8, 0)
					fc := f.s.cfg.ForkChoiceStore
					fc.Lock()
					require.NoError(t, fc.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
					fc.Unlock()
					require.NoError(t, f.s.ReceiveBlock(f.ctx, a, a.Root()))
					require.NoError(t, f.s.ReceiveBlock(f.ctx, b, b.Root()))
					synctest.Wait()
					require.Equal(t, b.Root(), f.s.CachedHeadRoot())
					// A target-state cache hit leaves cancellation to the later
					// committee lookup, exercising verification's retry path too.
					for _, att := range atts {
						require.NoError(t, f.s.checkpointStateCache.AddCheckpointState(att.Data.Target, aState))
					}
					ctx, cancel := context.WithCancel(f.ctx)
					defer cancel()
					request := &includedVoteCancelContext{Context: ctx, cancel: cancel}
					f.s.cfg.ForkChoiceStore = &includedVoteCancelStore{ForkChoicer: fc, request: request, checks: tc.checks}
					var err error
					if batch {
						err = f.s.ReceiveBlockBatch(request, []blocks.ROBlock{child})
					} else {
						err = f.s.ReceiveBlock(request, child, child.Root())
					}
					if tc.checks == 0 {
						require.NoError(t, err)
					} else {
						require.ErrorIs(t, err, context.Canceled)
						require.DeepSSZEqual(t, atts, f.s.cfg.AttPool.BlockAttestations(), "retain votes from the imported block")
						seen, err := f.s.cfg.AttPool.HasAggregatedAttestation(atts[0])
						require.NoError(t, err)
						require.Equal(t, false, seen, "unauthenticated votes must not suppress gossip")
					}
					require.Equal(t, true, fc.HasNode(child.Root()))
					synctest.Wait()
					require.NoError(t, f.s.ReceiveBlock(f.ctx, child, child.Root()), "duplicate imports skip the retained block")
					f.s.UpdateHead(f.ctx, 8)
					synctest.Wait()
					weight, err := fc.Weight(a.Root())
					require.NoError(t, err)
					assert.Equal(t, wantWeight, weight, "included votes must survive cancellation")
					assert.Equal(t, child.Root(), f.s.CachedHeadRoot(), "the voted branch must beat the larger-root competitor")
					assert.Equal(t, 0, len(f.s.cfg.AttPool.BlockAttestations()))
					// Repeated head updates must not count recovered votes twice.
					f.s.UpdateHead(f.ctx, 8)
					weight, err = fc.Weight(a.Root())
					require.NoError(t, err)
					assert.Equal(t, wantWeight, weight)
				})
			})
		}
	}
}
