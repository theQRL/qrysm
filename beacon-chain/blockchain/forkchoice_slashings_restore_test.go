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
	"github.com/theQRL/qrysm/beacon-chain/forkchoice"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

// interruptedSlashingRestore retains a prefix, as InsertChain does when a
// later node fails. Retrying the backfill then skips those restored ancestors.
type interruptedSlashingRestore struct {
	forkchoice.ForkChoicer
	stopRoot    [32]byte
	restoreErr  error
	interrupted bool
}

func (f *interruptedSlashingRestore) InsertChain(ctx context.Context, chain []*forktypes.BlockAndCheckpoints) error {
	if !f.interrupted {
		for i, b := range chain {
			if b.Block.Root() == f.stopRoot {
				f.interrupted = true
				if err := f.ForkChoicer.InsertChain(ctx, chain[:i+1]); err != nil {
					return err
				}
				return f.restoreErr
			}
		}
	}
	return f.ForkChoicer.InsertChain(ctx, chain)
}

func TestService_RestoreForkchoiceSlashings(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, slashed := range []bool{false, true} {
		for _, mode := range []string{"live", "saved head", "batch backfill", "batch backfill retry"} {
			name := map[bool]string{false: "unslashed control", true: "slashed"}[slashed] + "/" + mode
			t.Run(name, func(t *testing.T) {
				flags := &features.Flags{}
				if mode == "saved head" {
					flags.ForceHead = "head"
				}
				reset := features.InitWithReset(flags)
				t.Cleanup(reset)
				f := newBatchExecutionFixture(t, 2)
				f.s.cfg.SlashingPool = slashings.NewPool()
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				pre := f.states[2].Copy()
				committee, err := helpers.BeaconCommitteeFromState(f.ctx, pre, 4, 0)
				require.NoError(t, err)
				at4, err := transition.ProcessSlots(f.ctx, pre.Copy(), 4)
				require.NoError(t, err)
				proposer, err := helpers.BeaconProposerIndex(f.ctx, at4)
				require.NoError(t, err)
				position := 0
				for committee[position] == proposer {
					position++
				}
				victim := committee[position]
				pb, err := util.GenerateFullBlockZond(pre.Copy(), f.keys, &util.BlockGenConfig{}, 3)
				require.NoError(t, err)
				if slashed {
					as, err := util.GenerateAttesterSlashingForValidator(pre, f.keys[victim], victim)
					require.NoError(t, err)
					pb.Block.Body.AttesterSlashings = []*qrysmpb.AttesterSlashing{as}
				}
				sig, err := util.BlockSignature(pre.Copy(), pb.Block, f.keys)
				require.NoError(t, err)
				pb.Signature = sig.Marshal()
				signed, err := blocks.NewSignedBeaconBlock(pb)
				require.NoError(t, err)
				common, err := blocks.NewROBlock(signed)
				require.NoError(t, err)
				commonState, err := transition.ExecuteStateTransition(f.ctx, pre.Copy(), common)
				require.NoError(t, err)
				v, err := commonState.ValidatorAtIndexReadOnly(victim)
				require.NoError(t, err)
				require.Equal(t, slashed, v.Slashed())
				a, aState := emptyBranchBlock(t, f, commonState, 4, 'a')
				b, bState := emptyBranchBlock(t, f, commonState, 4, 'b')
				aRoot, bRoot := a.Root(), b.Root()
				var low, high blocks.ROBlock
				var lowState, highState state.BeaconState
				if bytes.Compare(aRoot[:], bRoot[:]) < 0 {
					low, high, lowState, highState = a, b, aState, bState
				} else {
					low, high, lowState, highState = b, a, bState, aState
				}
				var retryHead blocks.ROBlock
				if mode == "batch backfill retry" {
					retryHead, _ = emptyBranchBlock(t, f, highState, 5, 'h')
				}
				atts, err := util.GenerateAttestations(lowState.Copy(), f.keys, 1, 5, false)
				require.NoError(t, err)
				require.Equal(t, 1, len(atts))
				att := atts[0]
				bits := bitfield.NewBitlist(att.AggregationBits.Len())
				bits.SetBitAt(uint64(position), true)
				att.AggregationBits, att.Signatures = bits, att.Signatures[position:position+1]
				indices, err := verifiedAttestingIndices(f.ctx, f.states[0], att)
				require.NoError(t, err, "the vote has a genuine signature from the slashed validator")
				require.DeepEqual(t, []uint64{uint64(victim)}, indices)
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 5, 0)
					fc := f.s.cfg.ForkChoiceStore
					fc.Lock()
					err := fc.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot})
					fc.Unlock()
					require.NoError(t, err)
					for _, blk := range []blocks.ROBlock{common, low, high} {
						require.NoError(t, f.s.ReceiveBlock(f.ctx, blk, blk.Root()))
						synctest.Wait()
					}
					require.Equal(t, high.Root(), f.s.CachedHeadRoot())
					saved, err := f.s.cfg.BeaconDB.HeadBlockRoot()
					require.NoError(t, err)
					require.Equal(t, high.Root(), saved)
					if mode != "live" {
						require.NoError(t, f.s.saveInitSyncBlocks(f.ctx, true))
						f.s.head = nil
						f.s.cfg.ForkChoiceStore = doublylinkedtree.New()
						f.s.cfg.StateGen = stategen.New(f.s.cfg.BeaconDB, f.s.cfg.ForkChoiceStore)
						f.s.cfg.ForkChoiceStore.SetBalancesByRooter(f.s.cfg.StateGen.BalancesByCheckpoint)
						require.NoError(t, f.s.setupForkchoice(f.states[0].Copy()))
						if mode == "batch backfill" || mode == "batch backfill retry" {
							require.Equal(t, f.s.originBlockRoot, f.s.CachedHeadRoot())
							// The normal startup mode begins at justification.
							// Batch import backfills the known parent chain from DB.
							if mode == "batch backfill retry" {
								driftGenesisTime(f.s, 6, 0)
								restoreErr := errors.New("interrupted ancestor insertion")
								f.s.cfg.ForkChoiceStore = &interruptedSlashingRestore{
									ForkChoicer: f.s.cfg.ForkChoiceStore, stopRoot: common.Root(), restoreErr: restoreErr,
								}
								require.ErrorIs(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{retryHead}), restoreErr)
								require.Equal(t, true, f.s.cfg.ForkChoiceStore.HasNode(common.Root()))
								require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(high.Root()))
								require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{retryHead}))
							} else {
								require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{high}))
							}
							synctest.Wait()
						}
						// Re-sending the known slashing block takes the duplicate
						// fast path; importing its sibling child has no slashings.
						require.NoError(t, f.s.ReceiveBlock(f.ctx, common, common.Root()))
						require.NoError(t, f.s.ReceiveBlock(f.ctx, low, low.Root()))
						synctest.Wait()
					}
					wantHead := high.Root()
					if mode == "batch backfill retry" {
						wantHead = retryHead.Root()
					}
					require.Equal(t, wantHead, f.s.CachedHeadRoot())
					headState, err := f.s.HeadStateReadOnly(f.ctx)
					require.NoError(t, err)
					headValidator, err := headState.ValidatorAtIndexReadOnly(victim)
					require.NoError(t, err)
					require.Equal(t, slashed, headValidator.Slashed(), "the restored head state still records the slashing")
					fc = f.s.cfg.ForkChoiceStore
					fc.Lock()
					err = f.s.OnAttestation(f.ctx, att, 0)
					if err != nil {
						fc.Unlock()
						t.Fatal(err)
					}
					gotHead, err := fc.Head(f.ctx)
					weight, weightErr := fc.Weight(low.Root())
					fc.Unlock()
					require.NoError(t, err)
					require.NoError(t, weightErr)
					if slashed {
						require.Equal(t, uint64(0), weight, "restart must preserve the attester slashing's vote exclusion")
						require.Equal(t, wantHead, gotHead)
					} else {
						require.Equal(t, true, weight > 0)
						require.Equal(t, low.Root(), gotHead)
					}
				})
			})
		}
	}
}
