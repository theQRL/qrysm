package blockchain

import (
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestService_IncludedAttestationForkchoiceValidation(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, mode := range []string{"gossip", "batch", "deferred"} {
		for _, kind := range []string{"valid control", "inconsistent target", "unknown target", "future voted block"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 2)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				// A=slot 2 and B=slot 3 are competing children of slot 1.
				branchPB, err := util.GenerateFullBlockZond(f.states[1].Copy(), f.keys, &util.BlockGenConfig{}, 3)
				require.NoError(t, err)
				branchSigned, err := blocks.NewSignedBeaconBlock(branchPB)
				require.NoError(t, err)
				branch, err := blocks.NewROBlock(branchSigned)
				require.NoError(t, err)
				branchRoot := branch.Root()
				attSlot := primitives.Slot(3)
				if kind == "future voted block" {
					attSlot = 2
				}
				atts, err := util.GenerateAttestations(f.states[2].Copy(), f.keys, 1, attSlot+1, false)
				require.NoError(t, err)
				require.Equal(t, 1, len(atts))
				a := atts[0]
				require.Equal(t, attSlot, a.Data.Slot)
				a.Data.BeaconBlockRoot = branchRoot[:]
				if kind == "inconsistent target" {
					wrongTarget := f.blks[1].Root() // Known block, but not B's epoch-0 target.
					a.Data.Target.Root = wrongTarget[:]
				}
				if kind == "unknown target" {
					a.Data.Target.Root = bytesutil.PadTo([]byte{'u'}, 32)
				}
				committee, err := helpers.BeaconCommitteeFromState(f.ctx, f.states[2], a.Data.Slot, a.Data.CommitteeIndex)
				require.NoError(t, err)
				domain, err := signing.Domain(f.states[2].Fork(), a.Data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester, f.states[2].GenesisValidatorsRoot())
				require.NoError(t, err)
				dataRoot, err := signing.ComputeSigningRoot(a.Data, domain)
				require.NoError(t, err)
				a.Signatures = nil
				for i, index := range committee {
					if a.AggregationBits.BitAt(uint64(i)) {
						sig, err := f.keys[index].Sign(dataRoot[:])
						require.NoError(t, err)
						a.Signatures = append(a.Signatures, sig.Marshal())
					}
				}
				// The containing block extends A. The attestation is valid for
				// the state transition even when invalid for forkchoice.
				pb, err := util.GenerateFullBlockZond(f.states[2].Copy(), f.keys, &util.BlockGenConfig{}, 4)
				require.NoError(t, err)
				pb.Block.Body.Attestations = atts
				sig, err := util.BlockSignature(f.states[2].Copy(), pb.Block, f.keys)
				require.NoError(t, err)
				pb.Signature = sig.Marshal()
				signed, err := blocks.NewSignedBeaconBlock(pb)
				require.NoError(t, err)
				containing, err := blocks.NewROBlock(signed)
				require.NoError(t, err)
				_, err = transition.ExecuteStateTransition(f.ctx, f.states[2].Copy(), containing)
				require.NoError(t, err, "fully signed containing block must be consensus-valid")
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 5, -30) // No proposer boost.
					require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forkchoicetypes.Checkpoint{Root: f.s.originBlockRoot}))
					if mode != "deferred" {
						require.NoError(t, f.s.ReceiveBlock(f.ctx, branch, branch.Root()))
						synctest.Wait()
					}
					voteCommittee, err := helpers.BeaconCommitteeFromState(f.ctx, f.states[2], 4, 0)
					require.NoError(t, err)
					f.s.cfg.ForkChoiceStore.ProcessAttestation(f.ctx, []uint64{uint64(voteCommittee[0])}, f.blks[1].Root(), 0)
					f.s.UpdateHead(f.ctx, 5)
					synctest.Wait()
					require.Equal(t, f.blks[1].Root(), f.s.CachedHeadRoot())
					events := make(chan *feed.Event, 16)
					sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
					defer sub.Unsubscribe()
					if mode == "batch" {
						require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{containing}))
					} else {
						require.NoError(t, f.s.ReceiveBlock(f.ctx, containing, containing.Root()))
					}
					synctest.Wait()
					if mode == "deferred" {
						// The voted block was unknown during import. Exercise the
						// pool path once the missing branch becomes available.
						pending := f.s.cfg.AttPool.BlockAttestations()
						require.Equal(t, 1, len(pending))
						require.NoError(t, f.s.ReceiveBlock(f.ctx, branch, branch.Root()))
						synctest.Wait()
						f.s.UpdateHead(f.ctx, 5)
						synctest.Wait()
						require.Equal(t, 0, len(f.s.cfg.AttPool.ForkchoiceAttestations()))
						wantPending := 0
						if kind == "unknown target" {
							wantPending = 1
						}
						require.Equal(t, wantPending, len(f.s.cfg.AttPool.BlockAttestations()))
					}
					weight, err := f.s.cfg.ForkChoiceStore.Weight(branch.Root())
					require.NoError(t, err)
					head, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					reorgs := 0
					for len(events) > 0 {
						if ev := <-events; ev.Type == statefeed.Reorg {
							reorgs++
						}
					}
					if kind == "valid control" {
						require.Equal(t, true, weight > 0)
						require.Equal(t, branch.Root(), bytesutil.ToBytes32(head))
					} else {
						assert.Equal(t, uint64(0), weight, "invalid forkchoice attestations must not create vote weight")
						assert.Equal(t, containing.Root(), bytesutil.ToBytes32(head), "ignore the invalid vote and retain the heavier A branch")
						assert.Equal(t, 0, reorgs)
					}
				})
			})
		}
	}
}
