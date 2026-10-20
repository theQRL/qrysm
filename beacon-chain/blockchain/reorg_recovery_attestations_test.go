package blockchain

import (
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/go-bitfield"
	coreblocks "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestService_ReorgAttestationRecovery(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, aggregated := range []bool{false, true} {
		for _, mode := range []string{"gossip", "batch", "head publication retry"} {
			name := map[bool]string{false: "unaggregated", true: "aggregated"}[aggregated]
			t.Run(name+"/"+mode, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 3)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				pb, err := f.blks[2].PbZondBlock()
				require.NoError(t, err)
				a := qrysmpb.CopyAttestation(pb.Block.Body.Attestations[0])
				require.Equal(t, true, helpers.IsAggregated(a))
				if !aggregated {
					indices := a.AggregationBits.BitIndices()
					bits := bitfield.NewBitlist(a.AggregationBits.Len())
					bits.SetBitAt(uint64(indices[0]), true)
					a.AggregationBits = bits
					a.Signatures = a.Signatures[:1]
				}
				require.Equal(t, aggregated, helpers.IsAggregated(a))
				require.Equal(t, f.blks[1].Root(), bytesutil.ToBytes32(a.Data.BeaconBlockRoot))
				pb.Block.Body.Attestations = []*qrysmpb.Attestation{a}
				sig, err := util.BlockSignature(f.states[2].Copy(), pb.Block, f.keys)
				require.NoError(t, err)
				pb.Signature = sig.Marshal()
				signed, err := blocks.NewSignedBeaconBlock(pb)
				require.NoError(t, err)
				orphan, err := blocks.NewROBlock(signed)
				require.NoError(t, err)
				_, err = transition.ExecuteStateTransition(f.ctx, f.states[2].Copy(), orphan)
				require.NoError(t, err, "the old branch must be consensus valid")
				// Fork at slot 2: slot 3 includes a vote for the common parent;
				// the replacement at slot 4 leaves that vote available for inclusion.
				replacement, replacementState := emptyBranchBlock(t, f, f.states[2].Copy(), 4, 'r')
				proposalState, err := transition.ProcessSlots(f.ctx, replacementState.Copy(), 5)
				require.NoError(t, err)
				require.NoError(t, coreblocks.VerifyAttestationNoVerifySignatures(f.ctx, proposalState, a))
				require.NoError(t, coreblocks.VerifyAttestationSignatures(f.ctx, proposalState, a))
				save := f.s.cfg.AttPool.SaveUnaggregatedAttestation
				if aggregated {
					save = f.s.cfg.AttPool.SaveAggregatedAttestation
				}
				require.NoError(t, save(a))
				proposalAtts := func() []*qrysmpb.Attestation {
					t.Helper()
					atts, err := f.s.cfg.AttPool.UnaggregatedAttestations()
					require.NoError(t, err)
					return append(atts, f.s.cfg.AttPool.AggregatedAttestations()...)
				}
				require.Equal(t, 1, len(proposalAtts()))
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 3, 0)
					fc := f.s.cfg.ForkChoiceStore
					fc.Lock()
					err := fc.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot})
					fc.Unlock()
					require.NoError(t, err)
					require.NoError(t, f.s.ReceiveBlock(f.ctx, orphan, orphan.Root()))
					synctest.Wait()
					require.Equal(t, orphan.Root(), f.s.CachedHeadRoot())
					require.Equal(t, 0, len(proposalAtts()), "canonical inclusion removes the vote")
					driftGenesisTime(f.s, 4, 0)
					switch mode {
					case "batch":
						require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{replacement}))
					case "head publication retry":
						d := &headPublicationDB{
							HeadAccessDatabase: f.s.cfg.BeaconDB,
							root:               replacement.Root(),
							failure:            errors.New("temporary head write failure"),
							failures:           1,
						}
						f.s.cfg.BeaconDB = d
						require.ErrorIs(t, f.s.ReceiveBlock(f.ctx, replacement, replacement.Root()), d.failure)
						synctest.Wait()
						f.s.UpdateHead(f.ctx, 4)
						require.Equal(t, 2, d.attempts)
					default:
						require.NoError(t, f.s.ReceiveBlock(f.ctx, replacement, replacement.Root()))
					}
					synctest.Wait()
					head, err := f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					require.Equal(t, replacement.Root(), bytesutil.ToBytes32(head))
					recovered := proposalAtts()
					require.Equal(t, 1, len(recovered), "restore the valid orphaned vote for a later proposal")
					require.DeepSSZEqual(t, []*qrysmpb.Attestation{a}, recovered)
				})
			})
		}
	}
}
