package blockchain

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func signedBatchSlashingBlock(t *testing.T, f *batchExecutionFixture, pre state.BeaconState, slot primitives.Slot, graffiti byte, slashing *qrysmpb.AttesterSlashing) (blocks.ROBlock, state.BeaconState) {
	t.Helper()
	pb, err := util.GenerateFullBlockZond(pre.Copy(), f.keys, util.DefaultBlockGenConfig(), slot)
	require.NoError(t, err)
	pb.Block.Body.Graffiti[0] = graffiti
	if slashing != nil {
		pb.Block.Body.AttesterSlashings = []*qrysmpb.AttesterSlashing{slashing}
	}
	sig, err := util.BlockSignature(pre.Copy(), pb.Block, f.keys)
	require.NoError(t, err)
	pb.Signature = sig.Marshal()
	signed, err := blocks.NewSignedBeaconBlock(pb)
	require.NoError(t, err)
	ro, err := blocks.NewROBlock(signed)
	require.NoError(t, err)
	post, err := transition.ExecuteStateTransition(f.ctx, pre.Copy(), ro)
	require.NoError(t, err)
	return ro, post
}

func TestService_ReceiveBlockBatch_PrefixSlashings(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, mode := range []string{"successful batch control", "failed batch", "failed batch replay proof control"} {
		t.Run(mode, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 12)
			f.s.cfg.SlashingPool = slashings.NewPool()
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			pre := f.states[12].Copy()
			committee, err := helpers.BeaconCommitteeFromState(f.ctx, pre, 14, 0)
			require.NoError(t, err)
			proposers := make(map[primitives.ValidatorIndex]bool)
			for slot := primitives.Slot(13); slot <= 17; slot++ {
				atSlot, err := transition.ProcessSlots(f.ctx, pre.Copy(), slot)
				require.NoError(t, err)
				proposer, err := helpers.BeaconProposerIndex(f.ctx, atSlot)
				require.NoError(t, err)
				proposers[proposer] = true
			}
			position := 0
			for proposers[committee[position]] {
				position++
			}
			victim := committee[position]
			proof, err := util.GenerateAttesterSlashingForValidator(pre, f.keys[victim], victim)
			require.NoError(t, err)
			// Both justified balance snapshots predate this slashing, so
			// forkchoice must also retain the proof's vote exclusion.
			common, commonState := signedBatchSlashingBlock(t, f, pre, 13, 's', proof)
			validator, err := commonState.ValidatorAtIndexReadOnly(victim)
			require.NoError(t, err)
			require.Equal(t, true, validator.Slashed())
			a, aState := signedBatchSlashingBlock(t, f, commonState, 14, 'a', nil)
			b, bState := signedBatchSlashingBlock(t, f, commonState, 14, 'b', nil)
			low, high, lowState, highState := a, b, aState, bState
			aRoot, bRoot := a.Root(), b.Root()
			if bytes.Compare(aRoot[:], bRoot[:]) > 0 {
				low, high, lowState, highState = b, a, bState, aState
			}
			branch := append(append([]blocks.ROBlock(nil), f.blks[2:]...), common, high)
			for slot := primitives.Slot(15); slot <= 17; slot++ {
				blk, post := signedBatchSlashingBlock(t, f, highState, slot, 'h', nil)
				highState = post
				branch = append(branch, blk)
			}
			atts, err := util.GenerateAttestations(lowState.Copy(), f.keys, 1, 15, false)
			require.NoError(t, err)
			require.Equal(t, 1, len(atts))
			att := atts[0]
			bits := bitfield.NewBitlist(att.AggregationBits.Len())
			bits.SetBitAt(uint64(position), true)
			att.AggregationBits, att.Signatures = bits, att.Signatures[position:position+1]
			indices, err := verifiedAttestingIndices(f.ctx, pre, att)
			require.NoError(t, err)
			require.DeepEqual(t, []uint64{uint64(victim)}, indices)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 18, 0)
				fc := f.s.cfg.ForkChoiceStore
				fc.Lock()
				err := fc.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot})
				fc.Unlock()
				require.NoError(t, err)
				readErr := errors.New("temporary justified balance read failure")
				if mode != "successful batch control" {
					// Slot 17 advances justification to epoch 2. Failing its
					// balance read leaves the earlier blocks in forkchoice.
					fc.SetBalancesByRooter(func(ctx context.Context, cp *forktypes.Checkpoint) (*forktypes.JustifiedBalances, error) {
						if cp.Epoch == 2 {
							return nil, readErr
						}
						return f.s.cfg.StateGen.BalancesByCheckpoint(ctx, cp)
					})
				}
				err = f.s.ReceiveBlockBatch(f.ctx, branch)
				if mode == "successful batch control" {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, readErr)
					require.Equal(t, primitives.Epoch(1), fc.JustifiedCheckpoint().Epoch)
					require.Equal(t, true, fc.HasNode(branch[len(branch)-2].Root()), "slot 16 survives")
					require.Equal(t, false, fc.HasNode(branch[len(branch)-1].Root()), "slot 17 failed its checkpoint update")
				}
				synctest.Wait()
				fc.SetBalancesByRooter(f.s.cfg.StateGen.BalancesByCheckpoint)
				require.Equal(t, true, fc.HasNode(common.Root()), "the slashing block survives the failed batch")
				require.Equal(t, true, fc.HasNode(high.Root()), "a competing descendant survives")
				require.NoError(t, f.s.saveInitSyncBlocks(f.ctx, true))
				require.NoError(t, f.s.ReceiveBlock(f.ctx, common, common.Root()), "duplicate import cannot repair dropped operations")
				if mode == "failed batch replay proof control" {
					f.s.ReceiveAttesterSlashing(f.ctx, proof)
				}
				require.NoError(t, f.s.ReceiveBlock(f.ctx, low, low.Root()))
				synctest.Wait()
				fc.Lock()
				before, beforeErr := fc.Head(f.ctx)
				err = f.s.OnAttestation(f.ctx, att, 0)
				after, headErr := fc.Head(f.ctx)
				weight, weightErr := fc.Weight(low.Root())
				fc.Unlock()
				require.NoError(t, beforeErr)
				require.NoError(t, err)
				require.NoError(t, headErr)
				require.NoError(t, weightErr)
				assert.Equal(t, uint64(0), weight, "retained slashing blocks must exclude their equivocators")
				assert.Equal(t, before, after, "a slashed validator must not switch head")
			})
		})
	}
}
