package blockchain

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/db"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

type attestationReadFailureDB struct {
	db.HeadAccessDatabase
	root          [32]byte
	blockFailures int
	stateFailures int
	blockReads    int
	stateReads    int
	cancel        context.CancelFunc
}

func (d *attestationReadFailureDB) Block(ctx context.Context, root [32]byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	if root == d.root {
		d.blockReads++
		if d.cancel != nil {
			d.cancel()
			d.cancel = nil
			return nil, ctx.Err()
		}
		if d.blockFailures > 0 {
			d.blockFailures--
			return nil, errors.New("temporary voted-block read failure")
		}
	}
	return d.HeadAccessDatabase.Block(ctx, root)
}

func (d *attestationReadFailureDB) State(ctx context.Context, root [32]byte) (state.BeaconState, error) {
	if root == d.root {
		d.stateReads++
		if d.stateFailures > 0 {
			d.stateFailures--
			return nil, errors.New("temporary target-state read failure")
		}
	}
	return d.HeadAccessDatabase.State(ctx, root)
}

func TestService_AttestationProcessingRetry(t *testing.T) {
	for _, included := range []bool{false, true} {
		t.Run(map[bool]string{false: "gossip", true: "included"}[included], func(t *testing.T) {
			testAttestationProcessingRetry(t, included)
		})
	}
}

func testAttestationProcessingRetry(t *testing.T, included bool) {
	setupEpochTransitionTest(t)
	for _, mode := range []string{
		"healthy", "block read failures", "state read failures", "cancelled before processing",
		"cancelled during read", "future vote", "invalid signature", "expired dependency", "expired unknown block",
	} {
		t.Run(mode, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 2)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			// Two competing epoch-boundary blocks. The smaller root wins only
			// after the queued vote is processed successfully.
			a, aState := emptyBranchBlock(t, f, f.states[2], 6, 'a')
			b, bState := emptyBranchBlock(t, f, f.states[2], 6, 'b')
			aRoot, bRoot := a.Root(), b.Root()
			if bytes.Compare(aRoot[:], bRoot[:]) > 0 {
				a, b, aState = b, a, bState
			}
			atts, err := util.GenerateAttestations(aState.Copy(), f.keys, 1, 7, false)
			require.NoError(t, err)
			require.Equal(t, 1, len(atts))
			require.Equal(t, a.Root(), bytesutil.ToBytes32(atts[0].Data.BeaconBlockRoot))
			indices, err := verifiedAttestingIndices(f.ctx, aState, atts[0])
			require.NoError(t, err)
			require.Equal(t, true, len(indices) > 0)
			if mode == "invalid signature" {
				atts[0].Signatures[0][0] ^= 1
			}
			if mode == "expired unknown block" {
				atts[0].Data.BeaconBlockRoot = bytesutil.PadTo([]byte("unknown"), 32)
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 7, -30)
				f.s.cfg.ForkChoiceStore.Lock()
				require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
				f.s.cfg.ForkChoiceStore.Unlock()
				require.NoError(t, f.s.ReceiveBlock(f.ctx, a, a.Root()))
				require.NoError(t, f.s.ReceiveBlock(f.ctx, b, b.Root()))
				synctest.Wait()
				require.Equal(t, b.Root(), f.s.CachedHeadRoot())
				pendingCount := f.s.cfg.AttPool.ForkchoiceAttestationCount
				if included {
					require.NoError(t, f.s.cfg.AttPool.SaveBlockAttestation(atts[0]))
					pendingCount = func() int { return len(f.s.cfg.AttPool.BlockAttestations()) }
				} else {
					require.NoError(t, f.s.cfg.AttPool.SaveForkchoiceAttestations(atts))
				}
				d := &attestationReadFailureDB{HeadAccessDatabase: f.s.cfg.BeaconDB, root: a.Root()}
				f.s.cfg.BeaconDB = d
				if mode == "block read failures" || mode == "expired dependency" {
					d.blockFailures = 2
				}
				if mode == "state read failures" {
					// Recreate the state reader to exercise DB retrieval of the
					// competing target, which cannot use the canonical head state.
					require.NoError(t, d.SaveState(f.ctx, aState, a.Root()))
					f.s.cfg.StateGen = stategen.New(d, f.s.cfg.ForkChoiceStore)
					d.stateFailures = 2
				}
				request, cancel := context.WithCancel(f.ctx)
				defer cancel()
				if mode == "cancelled during read" {
					d.cancel = cancel
				}
				if mode == "cancelled before processing" {
					cancel()
				}
				if mode == "future vote" {
					driftGenesisTime(f.s, 6, -30)
				}
				f.s.cfg.ForkChoiceStore.Lock()
				defer f.s.cfg.ForkChoiceStore.Unlock()
				f.s.processAttestations(request, 0)
				wantPending := 1
				if mode == "healthy" || mode == "invalid signature" {
					wantPending = 0
				}
				require.Equal(t, wantPending, pendingCount())
				if mode == "block read failures" || mode == "state read failures" {
					f.s.processAttestations(f.ctx, 0)
					require.Equal(t, 1, pendingCount(), "repeated read failures must preserve the vote")
				}
				if mode == "future vote" {
					driftGenesisTime(f.s, 7, -30)
				}
				if mode == "expired dependency" || mode == "expired unknown block" {
					// Only gossip votes expire when epoch 3 starts. Included
					// votes must survive until their dependency is available.
					driftGenesisTime(f.s, 18, -30)
				}
				f.s.processAttestations(f.ctx, 0)
				if included && mode == "expired dependency" {
					require.Equal(t, 1, pendingCount(), "included votes survive repeated read failures after the gossip window")
					f.s.processAttestations(f.ctx, 0)
				}
				wantPending = 0
				if included && mode == "expired unknown block" {
					wantPending = 1
				}
				require.Equal(t, wantPending, pendingCount())
				head, err := f.s.cfg.ForkChoiceStore.Head(f.ctx)
				require.NoError(t, err)
				wantHead := a.Root()
				if mode == "invalid signature" || (!included && mode == "expired dependency") || mode == "expired unknown block" {
					wantHead = b.Root()
				}
				require.Equal(t, wantHead, head)
				if mode == "block read failures" {
					require.Equal(t, 3, d.blockReads)
				}
				if mode == "state read failures" {
					require.Equal(t, 3, d.stateReads)
				}
				if included && mode == "expired unknown block" {
					require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forktypes.Checkpoint{Epoch: 2, Root: a.Root()}))
					f.s.processAttestations(f.ctx, 0)
					require.Equal(t, 0, pendingCount(), "finality expires obsolete included votes even when their dependency never arrives")
				}
			})
		})
	}
}
