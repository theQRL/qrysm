package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/cache"
	coreblocks "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/db"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

type reorgAttestationRetryDB struct {
	db.HeadAccessDatabase
	root            [32]byte
	failure         error
	failures, reads int
	cancelAfterRead context.CancelFunc
}

func (d *reorgAttestationRetryDB) State(ctx context.Context, root [32]byte) (state.BeaconState, error) {
	if root != d.root {
		return d.HeadAccessDatabase.State(ctx, root)
	}
	d.reads++
	if d.failures > 0 {
		d.failures--
		return nil, d.failure
	}
	st, err := d.HeadAccessDatabase.State(ctx, root)
	if err == nil && d.cancelAfterRead != nil {
		d.cancelAfterRead()
		d.cancelAfterRead = nil
	}
	return st, err
}

func TestService_ReorgAttestationRecoveryRetry(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, batch := range []bool{false, true} {
		for _, tc := range []struct {
			name          string
			failures      int
			cancel        bool
			nonCheckpoint bool
		}{
			{name: "healthy"},
			{name: "target state read failure", failures: 1},
			{name: "repeated target state read failures", failures: 2},
			{name: "cancelled verification", cancel: true},
			{name: "non-checkpoint target is discarded", nonCheckpoint: true},
		} {
			t.Run(map[bool]string{false: "gossip/", true: "batch/"}[batch]+tc.name, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 2)
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				orphan, _ := signedBatchSlashingBlock(t, f, f.states[2].Copy(), 6, 'o', nil)
				if tc.nonCheckpoint {
					// A block can contain a signed vote whose target is not a
					// checkpoint. Such a vote must not hold up head publication.
					pb, err := orphan.PbZondBlock()
					require.NoError(t, err)
					a := pb.Block.Body.Attestations[0]
					root := f.blks[0].Root() // Slot 1 cannot be an epoch-0 checkpoint.
					a.Data.Target.Root = root[:]
					committee, err := helpers.BeaconCommitteeFromState(f.ctx, f.states[2], a.Data.Slot, a.Data.CommitteeIndex)
					require.NoError(t, err)
					domain, err := signing.Domain(f.states[2].Fork(), a.Data.Target.Epoch, params.BeaconConfig().DomainBeaconAttester, f.states[2].GenesisValidatorsRoot())
					require.NoError(t, err)
					signingRoot, err := signing.ComputeSigningRoot(a.Data, domain)
					require.NoError(t, err)
					a.Signatures = nil
					for _, bit := range a.AggregationBits.BitIndices() {
						sig, err := f.keys[committee[bit]].Sign(signingRoot[:])
						require.NoError(t, err)
						a.Signatures = append(a.Signatures, sig.Marshal())
					}
					sig, err := util.BlockSignature(f.states[2].Copy(), pb.Block, f.keys)
					require.NoError(t, err)
					pb.Signature = sig.Marshal()
					signed, err := blocks.NewSignedBeaconBlock(pb)
					require.NoError(t, err)
					orphan, err = blocks.NewROBlock(signed)
					require.NoError(t, err)
					_, err = transition.ExecuteStateTransition(f.ctx, f.states[2].Copy(), orphan)
					require.NoError(t, err, "the containing block remains consensus-valid")
				}
				atts := orphan.Block().Body().Attestations()
				require.Equal(t, 1, len(atts))
				att := atts[0]
				require.Equal(t, primitives.Slot(5), att.Data.Slot)
				replacement, replacementState := emptyBranchBlock(t, f, f.states[2].Copy(), 7, 'r')
				proposalState, err := transition.ProcessSlots(f.ctx, replacementState.Copy(), 8)
				require.NoError(t, err)
				require.NoError(t, coreblocks.VerifyAttestationNoVerifySignatures(f.ctx, proposalState, att))
				require.NoError(t, coreblocks.VerifyAttestationSignatures(f.ctx, proposalState, att))
				if !tc.nonCheckpoint {
					require.NoError(t, f.s.cfg.AttPool.SaveAggregatedAttestation(att))
				}
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, 6, 0)
					fc := f.s.cfg.ForkChoiceStore
					fc.Lock()
					err := fc.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot})
					fc.Unlock()
					require.NoError(t, err)
					require.NoError(t, f.s.ReceiveBlock(f.ctx, orphan, orphan.Root()))
					synctest.Wait()
					require.Equal(t, orphan.Root(), f.s.CachedHeadRoot())
					require.Equal(t, 0, len(f.s.cfg.AttPool.AggregatedAttestations()))
					d := &reorgAttestationRetryDB{
						HeadAccessDatabase: f.s.cfg.BeaconDB, root: f.s.originBlockRoot,
						failure: errors.New("temporary target-state read failure"), failures: tc.failures,
					}
					require.NoError(t, f.s.saveInitSyncBlocks(f.ctx, true))
					f.s.cfg.BeaconDB = d
					// Evict the previous-epoch target while retaining the replacement's
					// pre-state, so only reorg recovery needs to regenerate its state.
					f.s.cfg.StateGen = stategen.New(d, fc)
					require.NoError(t, f.s.cfg.StateGen.SaveState(f.ctx, f.blks[1].Root(), f.states[2].Copy()))
					f.s.checkpointStateCache = cache.NewCheckpointStateCache()
					request, cancel := context.WithCancel(f.ctx)
					defer cancel()
					if tc.cancel {
						// The state read succeeds, then committee verification sees
						// cancellation. Gossip head publication uses the service context.
						d.cancelAfterRead = cancel
						f.s.ctx = request
					}
					driftGenesisTime(f.s, 7, 0)
					if batch {
						err = f.s.ReceiveBlockBatch(request, []blocks.ROBlock{replacement})
					} else {
						err = f.s.ReceiveBlock(request, replacement, replacement.Root())
					}
					synctest.Wait()
					f.s.ctx = f.ctx
					wantReads := 1
					if tc.nonCheckpoint {
						wantReads = 0
					}
					require.Equal(t, wantReads, d.reads)
					failed := tc.failures > 0 || tc.cancel
					if failed {
						wantErr := d.failure
						if tc.cancel {
							wantErr = context.Canceled
						}
						require.ErrorIs(t, err, wantErr)
						require.Equal(t, false, IsInvalidBlock(err))
						require.Equal(t, true, fc.HasNode(replacement.Root()))
						assertOldHead := func() {
							t.Helper()
							head, err := f.s.HeadRoot(f.ctx)
							require.NoError(t, err)
							require.Equal(t, orphan.Root(), bytesutil.ToBytes32(head))
							durable, err := d.HeadBlockRoot()
							require.NoError(t, err)
							require.Equal(t, orphan.Root(), durable)
							require.Equal(t, 0, len(f.s.cfg.AttPool.AggregatedAttestations()))
						}
						assertOldHead()
						// Duplicate imports skip the retained block; a later head update
						// must still revisit the old branch until recovery succeeds.
						require.NoError(t, f.s.ReceiveBlock(f.ctx, replacement, replacement.Root()))
						assertOldHead()
						if tc.failures > 1 {
							f.s.UpdateHead(f.ctx, 7)
							synctest.Wait()
							assertOldHead()
						}
					} else {
						require.NoError(t, err)
					}
					for range 2 {
						f.s.UpdateHead(f.ctx, 7)
						synctest.Wait()
						head, err := f.s.HeadRoot(f.ctx)
						require.NoError(t, err)
						require.Equal(t, replacement.Root(), bytesutil.ToBytes32(head))
						if tc.nonCheckpoint {
							require.Equal(t, 0, len(f.s.cfg.AttPool.AggregatedAttestations()))
						} else {
							require.DeepSSZEqual(t, atts, f.s.cfg.AttPool.AggregatedAttestations(), "recover the reusable vote exactly once")
						}
					}
					require.Equal(t, 0, d.failures)
				})
			})
		}
	}
}
