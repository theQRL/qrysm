package blockchain

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/execution"
	"github.com/theQRL/qrysm/beacon-chain/verification"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestReceiveBlock_ForkchoiceErrorAttribution(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, tc := range []struct {
			name         string
			parent       int
			reject       bool
			cleanupFails bool
			punishable   bool
		}{
			{name: "valid existing head", parent: 1},
			{name: "unrelated head rejected", parent: 1, reject: true},
			{name: "unrelated cleanup fails", parent: 1, reject: true, cleanupFails: true},
			{name: "shared ancestor rejected", parent: 2, reject: true, punishable: true},
			{name: "received head rejected", parent: 3, reject: true, punishable: true},
			{name: "replacement also rejected", parent: 1, reject: true, punishable: true},
		} {
			name := map[bool]string{false: "gossip/", true: "batch/"}[batch] + tc.name
			t.Run(name, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 3)
				incoming, _ := emptyBranchBlock(t, f, f.states[tc.parent], 4, 'r')
				validPayload, err := f.blks[0].Block().Body().Execution()
				require.NoError(t, err)
				incomingPayload, err := incoming.Block().Body().Execution()
				require.NoError(t, err)
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 5, 0)
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
					voteForRoot(t, f, f.blks[2].Root(), 1)
					f.engine.ErrForkchoiceUpdated = nil
					if !tc.punishable {
						f.engine.ErrNewPayload = nil
					}
					if tc.reject {
						f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
						f.engine.ForkChoiceUpdatedResp = validPayload.BlockHash()
						f.engine.OverrideValidHash = bytesutil.ToBytes32(validPayload.BlockHash())
						if !tc.punishable {
							f.engine.OverrideValidHash = bytesutil.ToBytes32(incomingPayload.BlockHash())
						}
					}
					d := &invalidCleanupRetryDB{
						HeadAccessDatabase: f.s.cfg.BeaconDB, root: f.blks[1].Root(),
						err: errors.New("temporary block deletion failure"),
					}
					if tc.cleanupFails {
						d.remaining = 1
					}
					f.s.cfg.BeaconDB = d
					if batch {
						err = f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{incoming})
					} else {
						err = f.s.ReceiveBlock(f.ctx, incoming, incoming.Root())
					}
					synctest.Wait()
					if tc.reject {
						require.NotNil(t, err)
						// Further wrapping must preserve both the peer verdict
						// and the invalid roots consumed by regular sync.
						err = fmt.Errorf("import: %w", err)
						require.Equal(t, true, IsInvalidBlock(err))
						wantRoot := f.blks[2].Root()
						if tc.parent == 3 {
							wantRoot = incoming.Root()
						}
						require.Equal(t, wantRoot, InvalidBlockRoot(err))
						require.Equal(t, bytesutil.ToBytes32(validPayload.BlockHash()), InvalidBlockLVH(err))
						roots := InvalidAncestorRoots(err)
						require.Equal(t, true, slices.Contains(roots, f.blks[1].Root()))
						require.Equal(t, true, slices.Contains(roots, f.blks[2].Root()))
						require.Equal(t, tc.punishable, slices.Contains(roots, incoming.Root()))
						if tc.cleanupFails {
							require.ErrorIs(t, err, d.err, "retain the cleanup failure for diagnostics and retries")
						}
					} else {
						require.NoError(t, err)
					}
					assert.Equal(t, tc.punishable, errors.Is(err, verification.ErrInvalid), "attribute rejection to the supplied branch")
					assert.Equal(t, !tc.punishable, f.s.cfg.ForkChoiceStore.HasNode(incoming.Root()))
					if !tc.punishable {
						optimistic, statusErr := f.s.IsOptimisticForRoot(f.ctx, incoming.Root())
						require.NoError(t, statusErr)
						require.Equal(t, false, optimistic, "the received block passed consensus and execution validation")
					}
				})
			})
		}
	}
}
