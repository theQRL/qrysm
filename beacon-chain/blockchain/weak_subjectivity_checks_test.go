package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	testDB "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestService_VerifyWeakSubjectivityRoot(t *testing.T) {
	b := util.NewBeaconBlockZond()
	startSlot, err := slots.EpochStart(100)
	require.NoError(t, err)
	b.Block.Slot = startSlot
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	// A block at the same slot that never becomes part of the finalized
	// canonical chain.
	forkBlock := util.NewBeaconBlockZond()
	forkBlock.Block.Slot = b.Block.Slot
	forkBlock.Block.ProposerIndex = 1
	forkRoot, err := forkBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	blockEpoch := slots.ToEpoch(b.Block.Slot)
	childSlot, err := slots.EpochStart(blockEpoch + 1)
	require.NoError(t, err)
	childBlock := util.NewBeaconBlockZond()
	childBlock.Block.Slot = childSlot
	childBlock.Block.ParentRoot = r[:]
	childRoot, err := childBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	tests := []struct {
		wsVerified     bool
		disabled       bool
		wantErr        error
		checkpt        *qrysmpb.Checkpoint
		finalizedEpoch primitives.Epoch
		name           string
	}{
		{
			name:     "nil root and epoch",
			disabled: true,
		},
		{
			name:           "not yet to verify, ws epoch higher than finalized epoch",
			checkpt:        &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte{'a'}, 32), Epoch: blockEpoch},
			finalizedEpoch: blockEpoch - 1,
		},
		{
			name:           "can't find the block in DB",
			checkpt:        &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte{'a'}, fieldparams.RootLength), Epoch: 1},
			finalizedEpoch: blockEpoch + 1,
			wantErr:        errWSBlockNotFound,
		},
		{
			name:           "can't find the block corresponds to ws epoch in DB",
			checkpt:        &qrysmpb.Checkpoint{Root: r[:], Epoch: blockEpoch - 2},
			finalizedEpoch: blockEpoch - 1,
			wantErr:        errWSCheckpointMismatch,
		},
		{
			name:           "block in db but not canonical",
			checkpt:        &qrysmpb.Checkpoint{Root: forkRoot[:], Epoch: blockEpoch},
			finalizedEpoch: blockEpoch + 1,
			wantErr:        errWSBlockNotCanonical,
		},
		{
			name:           "canonical block from next epoch fails epoch range",
			checkpt:        &qrysmpb.Checkpoint{Root: childRoot[:], Epoch: blockEpoch},
			finalizedEpoch: blockEpoch + 1,
			wantErr:        errWSCheckpointMismatch,
		},
		{
			name:           "can verify and pass",
			checkpt:        &qrysmpb.Checkpoint{Root: r[:], Epoch: blockEpoch},
			finalizedEpoch: blockEpoch + 1,
		},
		{
			name:           "not yet to verify, equal epoch",
			checkpt:        &qrysmpb.Checkpoint{Root: r[:], Epoch: blockEpoch},
			finalizedEpoch: blockEpoch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beaconDB := testDB.SetupDB(t)
			ctx := context.Background()
			util.SaveBlock(t, ctx, beaconDB, b)
			util.SaveBlock(t, ctx, beaconDB, forkBlock)
			util.SaveBlock(t, ctx, beaconDB, childBlock)
			require.NoError(t, beaconDB.SaveGenesisBlockRoot(ctx, bytesutil.ToBytes32(b.Block.ParentRoot)))
			// Finalizing through the child indexes b and childBlock as part of
			// the finalized canonical chain; forkBlock stays non-canonical.
			require.NoError(t, beaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: childRoot[:], Epoch: blockEpoch + 1}))
			wv, err := NewWeakSubjectivityVerifier(tt.checkpt, beaconDB)
			require.NoError(t, err)
			require.Equal(t, !tt.disabled, wv.enabled)
			fcs := doublylinkedtree.New()
			s := &Service{
				cfg:        &config{BeaconDB: beaconDB, WeakSubjectivityCheckpt: tt.checkpt, ForkChoiceStore: fcs},
				wsVerifier: wv,
			}
			require.NoError(t, fcs.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Epoch: tt.finalizedEpoch}))
			cp := s.cfg.ForkChoiceStore.FinalizedCheckpoint()
			err = s.wsVerifier.VerifyWeakSubjectivity(context.Background(), cp.Epoch)
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

func TestWeakSubjectivity_CheckpointBoundary(t *testing.T) {
	setupEpochTransitionTest(t)
	db := testDB.SetupDB(t)
	ctx := context.Background()
	roots := make(map[primitives.Slot][32]byte)
	var parent [32]byte
	for _, slot := range []primitives.Slot{0, 5, 6, 8, 17, 25, 30} {
		b := util.NewBeaconBlockZond()
		b.Block.Slot, b.Block.ParentRoot = slot, parent[:]
		root, err := b.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, b)
		roots[slot], parent = root, root
	}
	genesis := roots[0]
	require.NoError(t, db.SaveGenesisBlockRoot(ctx, genesis))
	// Orphans at empty checkpoint slots must not hide the older canonical root.
	for _, slot := range []primitives.Slot{12, 24} {
		b := util.NewBeaconBlockZond()
		b.Block.Slot = slot
		util.SaveBlock(t, ctx, db, b)
	}
	finalized := roots[30]
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 5, Root: finalized[:]}))
	for _, tc := range []struct {
		name  string
		epoch primitives.Epoch
		slot  primitives.Slot
		valid bool
	}{
		{name: "boundary block", epoch: 1, slot: 6, valid: true},
		{name: "skipped boundary", epoch: 2, slot: 8, valid: true},
		{name: "skipped epoch", epoch: 4, slot: 17, valid: true},
		{name: "older canonical block", epoch: 2, slot: 6},
		{name: "later block in same epoch", epoch: 1, slot: 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := roots[tc.slot]
			v, err := NewWeakSubjectivityVerifier(&qrysmpb.Checkpoint{Epoch: tc.epoch, Root: root[:]}, db)
			require.NoError(t, err)
			err = v.VerifyWeakSubjectivity(ctx, 5)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errWSCheckpointMismatch)
			}
			assert.Equal(t, tc.valid, v.verified)
		})
	}
}

type weakSubjectivityReadFailure struct {
	weakSubjectivityDB
	failure error
	cancel  context.CancelFunc
}

func (d *weakSubjectivityReadFailure) HighestRootsBelowSlot(ctx context.Context, slot primitives.Slot) (primitives.Slot, [][32]byte, error) {
	if d.cancel != nil {
		d.cancel()
		return 0, nil, ctx.Err()
	}
	if d.failure != nil {
		return 0, nil, d.failure
	}
	return d.weakSubjectivityDB.HighestRootsBelowSlot(ctx, slot)
}

func TestService_DeferredWeakSubjectivity(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, mode := range []string{"tick", "gossip", "batch"} {
		for _, outcome := range []string{"valid", "mismatch", "read failure", "cancelled lookup"} {
			t.Run(mode+"/"+outcome, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 24)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				root := f.blks[5].Root()
				if outcome == "mismatch" {
					// Canonical, but later than the epoch-1 checkpoint boundary.
					root = f.blks[6].Root()
				}
				var err error
				f.s.wsVerifier, err = NewWeakSubjectivityVerifier(&qrysmpb.Checkpoint{Epoch: 1, Root: root[:]}, f.s.cfg.BeaconDB)
				require.NoError(t, err)
				db := &weakSubjectivityReadFailure{weakSubjectivityDB: f.s.cfg.BeaconDB}
				f.s.wsVerifier.db = db
				exits := 0
				exit := log.Logger.ExitFunc
				log.Logger.ExitFunc = func(code int) {
					require.Equal(t, 1, code)
					exits++
				}
				defer func() { log.Logger.ExitFunc = exit }()
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 23, 0)
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:23]))
					require.Equal(t, false, f.s.wsVerifier.verified)
					ctx, cancel := context.WithCancel(f.ctx)
					defer cancel()
					readErr := errors.New("temporary checkpoint index read failure")
					if outcome == "read failure" {
						db.failure = readErr
					} else if outcome == "cancelled lookup" {
						db.cancel = cancel
					}
					driftGenesisTime(f.s, 24, 0)
					switch mode {
					case "tick":
						err = f.s.NewSlot(ctx, 24)
					case "gossip":
						err = f.s.ReceiveBlock(ctx, f.blks[23], f.blks[23].Root())
					case "batch":
						err = f.s.ReceiveBlockBatch(ctx, f.blks[23:])
					}
					synctest.Wait()
					saved, savedErr := f.s.cfg.BeaconDB.FinalizedCheckpoint(f.ctx)
					require.NoError(t, savedErr)
					require.Equal(t, primitives.Epoch(2), saved.Epoch)
					switch outcome {
					case "valid":
						require.NoError(t, err)
						require.Equal(t, true, f.s.wsVerifier.verified)
					case "mismatch":
						require.ErrorIs(t, err, errWSCheckpointMismatch)
						require.Equal(t, 1, exits, "a conflicting trust anchor must stop the node")
						return
					case "read failure":
						require.ErrorIs(t, err, readErr)
					case "cancelled lookup":
						require.ErrorIs(t, err, context.Canceled)
					}
					require.Equal(t, 0, exits, "local failures must remain retryable")
					db.failure, db.cancel = nil, nil
					driftGenesisTime(f.s, 25, 0)
					require.NoError(t, f.s.NewSlot(f.ctx, 25))
					synctest.Wait()
					assert.Equal(t, true, f.s.wsVerifier.verified, "retry even when persisted finality is unchanged")
					assert.Equal(t, primitives.Epoch(2), f.s.FinalizedCheckpt().Epoch)
				})
			})
		}
	}
}

func TestWeakSubjectivity_ConsensusCheckpointWithSkippedSlot(t *testing.T) {
	setupEpochTransitionTest(t)
	f := newBatchExecutionFixture(t, 11)
	f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
	pre := f.states[11].Copy()
	var branch []blocks.ROBlock
	// Epoch 2 begins at slot 12, which is empty. Consensus uses slot 11's root.
	for slot := primitives.Slot(13); slot <= 29; slot++ {
		pb, err := util.GenerateFullBlockZond(pre.Copy(), f.keys, util.DefaultBlockGenConfig(), slot)
		require.NoError(t, err)
		signed, err := blocks.NewSignedBeaconBlock(pb)
		require.NoError(t, err)
		b, err := blocks.NewROBlock(signed)
		require.NoError(t, err)
		pre, err = transition.ExecuteStateTransition(f.ctx, pre.Copy(), b)
		require.NoError(t, err)
		branch = append(branch, b)
	}
	synctest.Test(t, func(t *testing.T) {
		t.Cleanup(synctest.Wait)
		driftGenesisTime(f.s, 23, 0)
		require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
		require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, branch[:11]))
		driftGenesisTime(f.s, 24, 0)
		require.NoError(t, f.s.NewSlot(f.ctx, 24))
		synctest.Wait()
		checkpoint := f.s.FinalizedCheckpt()
		require.Equal(t, primitives.Epoch(2), checkpoint.Epoch)
		require.Equal(t, f.blks[10].Root(), bytesutil.ToBytes32(checkpoint.Root))
		var err error
		f.s.wsVerifier, err = NewWeakSubjectivityVerifier(checkpoint, f.s.cfg.BeaconDB)
		require.NoError(t, err)
		driftGenesisTime(f.s, 29, 0)
		require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, branch[11:]))
		driftGenesisTime(f.s, 30, 0)
		require.NoError(t, f.s.NewSlot(f.ctx, 30))
		synctest.Wait()
		require.Equal(t, primitives.Epoch(3), f.s.FinalizedCheckpt().Epoch)
		assert.Equal(t, true, f.s.wsVerifier.verified)
	})
}
