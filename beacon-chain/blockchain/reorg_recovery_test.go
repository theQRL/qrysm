package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	forktypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	payloadattribute "github.com/theQRL/qrysm/consensus-types/payload-attribute"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestService_ReorgSlashingRecovery(t *testing.T) {
	for _, mode := range []string{"gossip", "batch", "execution rollback", "batch replacement after invalidation", "still slashed on replacement"} {
		t.Run(mode, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 2)
			f.s.cfg.SlashingPool = slashings.NewPool()
			if mode != "execution rollback" && mode != "batch replacement after invalidation" {
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			}
			pre := f.states[2]
			ps, err := util.GenerateProposerSlashingForValidator(pre, f.keys[0], 0)
			require.NoError(t, err)
			as, err := util.GenerateAttesterSlashingForValidator(pre, f.keys[1], 1)
			require.NoError(t, err)
			// Sign two competing children of the slot-2 block. The original
			// head slashes validators 0 and 1; the replacement usually does not.
			pb, err := util.GenerateFullBlockZond(pre.Copy(), f.keys, &util.BlockGenConfig{}, 3)
			require.NoError(t, err)
			pb.Block.Body.ProposerSlashings = []*qrysmpb.ProposerSlashing{ps}
			pb.Block.Body.AttesterSlashings = []*qrysmpb.AttesterSlashing{as}
			sig, err := util.BlockSignature(pre.Copy(), pb.Block, f.keys)
			require.NoError(t, err)
			pb.Signature = sig.Marshal()
			signed, err := blocks.NewSignedBeaconBlock(pb)
			require.NoError(t, err)
			orphan, err := blocks.NewROBlock(signed)
			require.NoError(t, err)
			orphanState, err := transition.ExecuteStateTransition(f.ctx, pre.Copy(), orphan)
			require.NoError(t, err)
			var invalidChild blocks.ROBlock
			if mode == "batch replacement after invalidation" {
				invalidChild, _ = emptyBranchBlock(t, f, orphanState, 4, 'x')
			}
			replacement, _ := emptyBranchBlock(t, f, pre, 4, 'r')
			if mode == "still slashed on replacement" {
				pb, err := replacement.PbZondBlock()
				require.NoError(t, err)
				pb.Block.Body.ProposerSlashings = []*qrysmpb.ProposerSlashing{ps}
				pb.Block.Body.AttesterSlashings = []*qrysmpb.AttesterSlashing{as}
				sig, err := util.BlockSignature(pre.Copy(), pb.Block, f.keys)
				require.NoError(t, err)
				pb.Signature = sig.Marshal()
				signed, err := blocks.NewSignedBeaconBlock(pb)
				require.NoError(t, err)
				replacement, err = blocks.NewROBlock(signed)
				require.NoError(t, err)
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 3, 0)
				f.s.cfg.ForkChoiceStore.Lock()
				require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forktypes.Checkpoint{Root: f.s.originBlockRoot}))
				f.s.cfg.ForkChoiceStore.Unlock()
				if mode == "batch" {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{orphan}))
				} else {
					require.NoError(t, f.s.ReceiveBlock(f.ctx, orphan, orphan.Root()))
				}
				synctest.Wait()
				head, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, orphan.Root(), bytesutil.ToBytes32(head))
				driftGenesisTime(f.s, 4, 0)
				wantRoot := replacement.Root()
				if mode == "execution rollback" || mode == "batch replacement after invalidation" {
					payload, err := f.blks[1].Block().Body().Execution()
					require.NoError(t, err)
					if mode == "execution rollback" {
						wantRoot = f.blks[1].Root()
						f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
						f.engine.ForkChoiceUpdatedResp = payload.BlockHash()
						f.engine.OverrideValidHash = bytesutil.ToBytes32(payload.BlockHash())
						f.s.UpdateHead(f.ctx, 4)
					} else {
						f.engine.ErrNewPayload = execution.ErrInvalidPayloadStatus
						f.engine.NewPayloadResp = payload.BlockHash()
						err = f.s.ReceiveBlock(f.ctx, invalidChild, invalidChild.Root())
						require.Equal(t, true, IsInvalidBlock(err))
						require.Equal(t, 1, len(f.s.invalidatedHeadBlocks))
						f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
						require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{replacement}))
					}
				} else {
					require.NoError(t, f.s.ReceiveBlock(f.ctx, replacement, replacement.Root()))
				}
				synctest.Wait()
				head, err = f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, wantRoot, bytesutil.ToBytes32(head))
				newState, err := f.s.HeadState(f.ctx)
				require.NoError(t, err)
				want := 1
				if mode == "still slashed on replacement" {
					want = 0
				}
				assert.Equal(t, want, len(f.s.cfg.SlashingPool.PendingProposerSlashings(f.ctx, newState, true)))
				assert.Equal(t, want, len(f.s.cfg.SlashingPool.PendingAttesterSlashings(f.ctx, newState, true)))
			})
		})
	}
}

func TestService_InvalidationReorgRecovery(t *testing.T) {
	for _, tc := range []struct {
		name          string
		oldHeadSlot   int
		batch         bool
		invalidFCU    bool
		unchangedHead bool
		failCleanup   bool
	}{
		{name: "NewPayload", oldHeadSlot: 2},
		{name: "batch NewPayload", oldHeadSlot: 3, batch: true},
		{name: "ForkchoiceUpdated", oldHeadSlot: 3, invalidFCU: true},
		{name: "unchanged optimistic head", oldHeadSlot: 2, invalidFCU: true, unchangedHead: true},
		{name: "cleanup failure before recovery", oldHeadSlot: 3, failCleanup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, tc.oldHeadSlot+1)
			validPayload, err := f.blks[0].Block().Body().Execution()
			require.NoError(t, err)
			atts := f.blks[1].Block().Body().Attestations()
			require.Equal(t, 1, len(atts))
			require.Equal(t, f.blks[0].Root(), bytesutil.ToBytes32(atts[0].Data.BeaconBlockRoot))
			_, err = verifiedAttestingIndices(f.ctx, f.states[0], atts[0])
			require.NoError(t, err, "the vote for the surviving ancestor is reusable")
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 5, 0)
				if tc.oldHeadSlot > 2 {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:tc.oldHeadSlot]))
				}
				synctest.Wait()
				require.Equal(t, f.blks[tc.oldHeadSlot-1].Root(), f.s.CachedHeadRoot())
				events := make(chan *feed.Event, 16)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				f.engine.ErrForkchoiceUpdated = nil
				if tc.invalidFCU {
					f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
					f.engine.ForkChoiceUpdatedResp = validPayload.BlockHash()
					f.engine.OverrideValidHash = bytesutil.ToBytes32(validPayload.BlockHash())
				} else {
					f.engine.ErrNewPayload = execution.ErrInvalidPayloadStatus
					f.engine.NewPayloadResp = validPayload.BlockHash()
				}
				wantError := "received an INVALID payload"
				if tc.failCleanup {
					wantError = "temporary block deletion failure"
					f.s.cfg.BeaconDB = &invalidHeadCleanupDB{
						HeadAccessDatabase: f.s.cfg.BeaconDB,
						root:               f.blks[1].Root(),
						err:                errors.New(wantError),
					}
				}
				if !tc.unchangedHead {
					if tc.batch {
						err = f.s.ReceiveBlockBatch(f.ctx, f.blks[tc.oldHeadSlot:])
					} else {
						b := f.blks[tc.oldHeadSlot]
						err = f.s.ReceiveBlock(f.ctx, b, b.Root())
					}
					require.ErrorContains(t, wantError, err)
					if !tc.invalidFCU {
						// Only the published head's removed ancestry is retained;
						// it remains inaccessible through the normal block cache.
						require.Equal(t, tc.oldHeadSlot-1, len(f.s.invalidatedHeadBlocks))
						for _, b := range f.blks[1:tc.oldHeadSlot] {
							assert.Equal(t, false, f.s.hasInitSyncBlock(b.Root()))
							if !tc.failCleanup {
								_, err := f.s.getBlock(f.ctx, b.Root())
								require.ErrorIs(t, errBlockNotFoundInCacheOrDB, err)
							}
						}
					}
				}
				f.s.UpdateHead(f.ctx, 5)
				synctest.Wait()
				require.Equal(t, f.blks[0].Root(), f.s.CachedHeadRoot())
				published, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, f.blks[0].Root(), bytesutil.ToBytes32(published))
				requireInvalidationReorg(t, events, f.blks[tc.oldHeadSlot-1].Root(), f.blks[0].Root(), uint64(tc.oldHeadSlot-1))
				requireRecoveredAncestorVote(t, f)
				require.Equal(t, 0, len(f.s.invalidatedHeadBlocks), "release the removed branch after recovery")
				for _, b := range f.blks[1:tc.oldHeadSlot] {
					assert.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(b.Root()))
				}
			})
		})
	}
}

func TestService_InvalidationReorgCompetingBranch(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "gossip replacement", true: "batch replacement"}[batch], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 4)
			replacement, _ := emptyBranchBlock(t, f, f.states[1], 4, 'r')
			validPayload, err := f.blks[0].Block().Body().Execution()
			require.NoError(t, err)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 5, 0)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:3]))
				synctest.Wait()
				f.engine.ErrNewPayload = execution.ErrInvalidPayloadStatus
				f.engine.NewPayloadResp = validPayload.BlockHash()
				require.ErrorContains(t, "received an INVALID payload", f.s.ReceiveBlock(f.ctx, f.blks[3], f.blks[3].Root()))
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				events := make(chan *feed.Event, 16)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				// The next import can replace the invalid head before UpdateHead
				// runs. Both publication paths must recover its reusable votes.
				if batch {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{replacement}))
				} else {
					require.NoError(t, f.s.ReceiveBlock(f.ctx, replacement, replacement.Root()))
				}
				synctest.Wait()
				require.Equal(t, replacement.Root(), f.s.CachedHeadRoot())
				published, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, replacement.Root(), bytesutil.ToBytes32(published))
				if !batch {
					requireInvalidationReorg(t, events, f.blks[2].Root(), replacement.Root(), 3)
				}
				requireRecoveredAncestorVote(t, f)
				require.Equal(t, 0, len(f.s.invalidatedHeadBlocks))
				for _, b := range f.blks[1:3] {
					assert.Equal(t, false, f.s.HasBlock(f.ctx, b.Root()), "replacement import must not resurrect removed blocks")
				}
			})
		})
	}
}

type recursiveInvalidationEngine struct {
	*mockExecution.EngineClient
	latestValid map[[32]byte][32]byte
}

func (e *recursiveInvalidationEngine) ForkchoiceUpdated(ctx context.Context, fcs *enginev1.ForkchoiceState, attr payloadattribute.Attributer) (*enginev1.PayloadIDBytes, []byte, error) {
	if valid, ok := e.latestValid[bytesutil.ToBytes32(fcs.HeadBlockHash)]; ok {
		return nil, valid[:], execution.ErrInvalidPayloadStatus
	}
	return e.EngineClient.ForkchoiceUpdated(ctx, fcs, attr)
}

func TestService_InvalidationReorgRecursiveRecovery(t *testing.T) {
	f := newBatchExecutionFixture(t, 3)
	var hashes [][32]byte
	for _, b := range f.blks {
		payload, err := b.Block().Body().Execution()
		require.NoError(t, err)
		hashes = append(hashes, bytesutil.ToBytes32(payload.BlockHash()))
	}
	synctest.Test(t, func(t *testing.T) {
		t.Cleanup(synctest.Wait)
		driftGenesisTime(f.s, 4, 0)
		require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
		synctest.Wait()
		f.engine.ErrForkchoiceUpdated = nil
		// C is removed first; when recovery queries B, execution rejects it
		// too. The retained ancestry must extend through both invalidations.
		f.s.cfg.ExecutionEngineCaller = &recursiveInvalidationEngine{
			EngineClient: f.engine,
			latestValid:  map[[32]byte][32]byte{hashes[2]: hashes[1], hashes[1]: hashes[0]},
		}
		events := make(chan *feed.Event, 16)
		sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
		defer sub.Unsubscribe()
		f.s.UpdateHead(f.ctx, 4)
		synctest.Wait()
		published, err := f.s.HeadRoot(f.ctx)
		require.NoError(t, err)
		require.Equal(t, f.blks[0].Root(), bytesutil.ToBytes32(published))
		requireInvalidationReorg(t, events, f.blks[2].Root(), f.blks[0].Root(), 2)
		requireRecoveredAncestorVote(t, f)
		require.Equal(t, 0, len(f.s.invalidatedHeadBlocks))
	})
}

func requireInvalidationReorg(t *testing.T, events chan *feed.Event, oldRoot, newRoot [32]byte, depth uint64) {
	t.Helper()
	count := 0
	for len(events) > 0 {
		event := <-events
		if event.Type != statefeed.Reorg {
			continue
		}
		count++
		reorg := event.Data.(*qrlpb.EventChainReorg)
		assert.Equal(t, depth, reorg.Depth)
		assert.Equal(t, oldRoot, bytesutil.ToBytes32(reorg.OldHeadBlock))
		assert.Equal(t, newRoot, bytesutil.ToBytes32(reorg.NewHeadBlock))
	}
	require.Equal(t, 1, count)
}

func requireRecoveredAncestorVote(t *testing.T, f *batchExecutionFixture) {
	t.Helper()
	atts, err := f.s.cfg.AttPool.UnaggregatedAttestations()
	require.NoError(t, err)
	atts = append(atts, f.s.cfg.AttPool.AggregatedAttestations()...)
	require.Equal(t, 1, len(atts), "recover the surviving vote, excluding votes for execution-invalid blocks")
	require.DeepEqual(t, f.blks[1].Block().Body().Attestations()[0], atts[0])
}
