package blockchain

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type forkchoiceRecoveryDB struct {
	db.HeadAccessDatabase
	failure   error
	remaining int
}

func (d *forkchoiceRecoveryDB) FinalizedCheckpoint(ctx context.Context) (*qrysmpb.Checkpoint, error) {
	if d.remaining > 0 {
		d.remaining--
		return nil, d.failure
	}
	return d.HeadAccessDatabase.FinalizedCheckpoint(ctx)
}

func TestService_RecursiveInvalidRecovery(t *testing.T) {
	for _, competing := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("competing=%v/read failure=%v", competing, fail), func(t *testing.T) {
				f := newBatchExecutionFixture(t, 3)
				other, _ := emptyBranchBlock(t, f, f.states[1], 2, 'r')
				payload, err := f.blks[0].Block().Body().Execution()
				require.NoError(t, err)
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 4, 0)
					wantInvalid := [][32]byte{f.blks[1].Root(), f.blks[2].Root()}
					if competing {
						// Keep B2/C3 selected until INVALID removes that branch.
						// The independent sibling is then rejected by the next FCU.
						voteForRoot(t, f, f.blks[1].Root(), 0)
						require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{other}))
						wantInvalid = append(wantInvalid, other.Root())
					}
					synctest.Wait()
					f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
					f.engine.ForkChoiceUpdatedResp = payload.BlockHash()
					f.engine.OverrideValidHash = bytesutil.ToBytes32(payload.BlockHash())
					d := &forkchoiceRecoveryDB{
						HeadAccessDatabase: f.s.cfg.BeaconDB,
						failure:            errors.New("fallback finalized checkpoint read failed"),
					}
					if fail {
						d.remaining = 1
					}
					f.s.cfg.BeaconDB = d
					err := f.s.ReceiveBlock(f.ctx, f.blks[2], f.blks[2].Root())
					synctest.Wait()
					require.NotNil(t, err)
					if fail {
						require.ErrorIs(t, err, d.failure)
					}
					assert.Equal(t, true, IsInvalidBlock(err))
					assert.Equal(t, f.blks[2].Root(), InvalidBlockRoot(err), "retain the original rejected head")
					assert.Equal(t, bytesutil.ToBytes32(payload.BlockHash()), InvalidBlockLVH(err))
					roots := InvalidAncestorRoots(err)
					assert.Equal(t, len(wantInvalid), len(roots))
					for _, root := range wantInvalid {
						assert.Equal(t, true, slices.Contains(roots, root), "return every invalidated branch to sync")
						require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(root))
					}
					f.s.UpdateHead(f.ctx, 4)
					synctest.Wait()
					published, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					require.Equal(t, f.blks[0].Root(), bytesutil.ToBytes32(published))
				})
			})
		}
	}
}
