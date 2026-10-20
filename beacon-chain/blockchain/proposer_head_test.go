package blockchain

import (
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestService_ProposerHeadBeforeTick(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tickFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "tick first", false: "proposal request first"}[tickFirst], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 2)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			competitor, _ := emptyBranchBlock(t, f, f.states[1].Copy(), 2, 'c')
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 2, 0)
				require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forkchoicetypes.Checkpoint{Root: f.s.originBlockRoot}))
				require.NoError(t, f.s.ReceiveBlock(f.ctx, competitor, competitor.Root()))
				synctest.Wait()
				require.Equal(t, competitor.Root(), f.s.CachedHeadRoot())
				require.Equal(t, competitor.Root(), f.s.ProposerBoost())
				// Votes from slot 2 become eligible at slot 3. They outweigh the
				// competitor once its previous-slot boost has expired.
				driftGenesisTime(f.s, 3, 0)
				committee, err := helpers.BeaconCommitteeFromState(f.ctx, f.states[2], 2, 0)
				require.NoError(t, err)
				require.Equal(t, true, len(committee) >= 3)
				f.s.cfg.ForkChoiceStore.Lock()
				f.s.cfg.ForkChoiceStore.ProcessAttestation(f.ctx, []uint64{uint64(committee[0]), uint64(committee[1]), uint64(committee[2])}, f.blks[1].Root(), 0)
				f.s.cfg.ForkChoiceStore.Unlock()
				if tickFirst {
					require.NoError(t, f.s.NewSlot(f.ctx, 3))
				}
				// Match the validator RPC's proposal parent selection order.
				f.s.UpdateHead(f.ctx, f.s.CurrentSlot())
				parent := f.s.GetProposerHead()
				synctest.Wait()
				weight, err := f.s.cfg.ForkChoiceStore.Weight(competitor.Root())
				require.NoError(t, err)
				assert.Equal(t, uint64(0), weight)
				assert.Equal(t, [32]byte{}, f.s.ProposerBoost())
				assert.Equal(t, f.blks[1].Root(), f.s.CachedHeadRoot())
				assert.Equal(t, f.blks[1].Root(), parent, "a proposal must not use the previous slot's proposer boost")
			})
		})
	}
}
