package blockchain

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/db"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

type finalityPayloadGate struct {
	*mockExecution.EngineClient
	hash    [32]byte
	entered chan struct{}
	release chan struct{}
}

func (e *finalityPayloadGate) NewPayload(ctx context.Context, payload interfaces.ExecutionData, hashes []common.Hash, root *common.Hash) ([]byte, error) {
	if bytesutil.ToBytes32(payload.BlockHash()) == e.hash {
		close(e.entered)
		<-e.release
	}
	return e.EngineClient.NewPayload(ctx, payload, hashes, root)
}

func TestReceiveBlock_RejectsBlockAtFinalizedSlot(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, mode := range []string{"serial gossip control", "batch", "gossip overlapping finality"} {
		t.Run(mode, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 11)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			var branch []blocks.ROBlock
			pre := f.states[11].Copy()
			// A skipped block at slot 12 means epoch-2 finality will require
			// checkpoint root A11 and forbid any child at slot 12.
			for slot := primitives.Slot(13); slot <= 23; slot++ {
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
			conflicting, _ := emptyBranchBlock(t, f, f.states[11], 12, 'x')
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 23, -30)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, branch))
				synctest.Wait()
				require.Equal(t, primitives.Epoch(0), f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				var received chan error
				var release chan struct{}
				if mode == "gossip overlapping finality" {
					payload, err := conflicting.Block().Body().Execution()
					require.NoError(t, err)
					gate := &finalityPayloadGate{
						EngineClient: f.engine, hash: bytesutil.ToBytes32(payload.BlockHash()),
						entered: make(chan struct{}), release: make(chan struct{}),
					}
					f.s.cfg.ExecutionEngineCaller = gate
					received, release = make(chan error, 1), gate.release
					go func() { received <- f.s.ReceiveBlock(f.ctx, conflicting, conflicting.Root()) }()
					<-gate.entered
					synctest.Wait() // The state transition finishes; newPayload is held.
				}
				driftGenesisTime(f.s, 24, -30)
				require.NoError(t, f.s.NewSlot(f.ctx, 24))
				synctest.Wait()
				fc := f.s.cfg.ForkChoiceStore.FinalizedCheckpoint()
				require.Equal(t, primitives.Epoch(2), fc.Epoch)
				require.Equal(t, f.blks[10].Root(), fc.Root)
				var err error
				switch mode {
				case "batch":
					err = f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{conflicting})
				case "gossip overlapping finality":
					close(release)
					err = <-received
				default:
					err = f.s.ReceiveBlock(f.ctx, conflicting, conflicting.Root())
				}
				synctest.Wait()
				t.Logf("error=%v imported=%t saved=%t", err, f.s.cfg.ForkChoiceStore.HasNode(conflicting.Root()), f.s.cfg.BeaconDB.HasBlock(f.ctx, conflicting.Root()))
				require.NotNil(t, err)
				require.ErrorContains(t, "equal or earlier than finalized", err)
				assert.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(conflicting.Root()))
				assert.Equal(t, false, f.s.cfg.BeaconDB.HasBlock(f.ctx, conflicting.Root()))
			})
		})
	}
}

type finalityHoldHeadRootDB struct {
	db.HeadAccessDatabase
	hold     bool
	failures int
}

func (d *finalityHoldHeadRootDB) SaveHeadBlockRoot(ctx context.Context, root [32]byte) error {
	if d.hold {
		d.failures++
		return errors.New("temporary head persistence failure")
	}
	return d.HeadAccessDatabase.SaveHeadBlockRoot(ctx, root)
}

func TestSaveHead_RecoversOperationsAfterFinalizationPrunedOldHead(t *testing.T) {
	setupEpochTransitionTest(t)
	cfg := params.BeaconConfig().Copy()
	cfg.ShardCommitteePeriod = 0
	params.OverrideBeaconConfig(cfg)
	for _, mode := range []string{"healthy control", "database failure until finalization", "batch finalizes before publication"} {
		t.Run(mode, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 24)
			f.s.cfg.SlashingPool = slashings.NewPool()
			f.s.cfg.ExitPool = voluntaryexits.NewPool()
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			pre := f.states[11]
			ps, err := util.GenerateProposerSlashingForValidator(pre, f.keys[0], 0)
			require.NoError(t, err)
			as, err := util.GenerateAttesterSlashingForValidator(pre, f.keys[1], 1)
			require.NoError(t, err)
			exit, err := util.GenerateVoluntaryExits(pre, f.keys[2], 2)
			require.NoError(t, err)
			pb, err := util.GenerateFullBlockZond(pre.Copy(), f.keys, &util.BlockGenConfig{}, 21)
			require.NoError(t, err)
			pb.Block.Body.ProposerSlashings = []*qrysmpb.ProposerSlashing{ps}
			pb.Block.Body.AttesterSlashings = []*qrysmpb.AttesterSlashing{as}
			pb.Block.Body.VoluntaryExits = []*qrysmpb.SignedVoluntaryExit{exit}
			sig, err := util.BlockSignature(pre.Copy(), pb.Block, f.keys)
			require.NoError(t, err)
			pb.Signature = sig.Marshal()
			signed, err := blocks.NewSignedBeaconBlock(pb)
			require.NoError(t, err)
			orphan, err := blocks.NewROBlock(signed)
			require.NoError(t, err)
			fresh := slashings.NewPool()
			require.NoError(t, fresh.InsertProposerSlashing(f.ctx, f.states[23], ps))
			require.NoError(t, fresh.InsertAttesterSlashing(f.ctx, f.states[23], as))
			require.NoError(t, f.s.cfg.SlashingPool.InsertProposerSlashing(f.ctx, pre, ps))
			require.NoError(t, f.s.cfg.SlashingPool.InsertAttesterSlashing(f.ctx, pre, as))
			f.s.cfg.ExitPool.InsertVoluntaryExit(exit)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 21, -30)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:11]))
				synctest.Wait()
				require.NoError(t, f.s.ReceiveBlock(f.ctx, orphan, orphan.Root()))
				synctest.Wait()
				published, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, orphan.Root(), bytesutil.ToBytes32(published))
				require.Equal(t, 0, len(f.s.cfg.SlashingPool.PendingProposerSlashings(f.ctx, f.states[23], true)))
				require.Equal(t, 0, len(f.s.cfg.SlashingPool.PendingAttesterSlashings(f.ctx, f.states[23], true)))
				exits, err := f.s.cfg.ExitPool.PendingExits()
				require.NoError(t, err)
				require.Equal(t, 0, len(exits))
				d := &finalityHoldHeadRootDB{HeadAccessDatabase: f.s.cfg.BeaconDB, hold: mode == "database failure until finalization"}
				f.s.cfg.BeaconDB = d
				events := make(chan *feed.Event, 128)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				if mode == "batch finalizes before publication" {
					driftGenesisTime(f.s, 24, -30)
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[11:23]))
				} else {
					driftGenesisTime(f.s, 23, -30)
					for _, b := range f.blks[11:23] {
						err := f.s.ReceiveBlock(f.ctx, b, b.Root())
						if err != nil {
							switch mode {
							case "database failure until finalization":
								require.ErrorContains(t, "temporary head persistence failure", err)
							default:
								t.Fatal(err)
							}
						}
					}
				}
				synctest.Wait()
				if d.hold {
					require.Equal(t, true, d.failures > 0)
				}
				if d.hold {
					published, err = f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					require.Equal(t, orphan.Root(), bytesutil.ToBytes32(published))
				}
				driftGenesisTime(f.s, 24, -30)
				require.NoError(t, f.s.NewSlot(f.ctx, 24))
				synctest.Wait()
				require.Equal(t, primitives.Epoch(2), f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(orphan.Root()))
				require.Equal(t, true, f.s.cfg.BeaconDB.HasBlock(f.ctx, orphan.Root()), "old block still exists in DB")
				d.hold = false
				f.s.UpdateHead(f.ctx, 24)
				synctest.Wait()
				published, err = f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, f.blks[22].Root(), bytesutil.ToBytes32(published))
				st, err := f.s.HeadState(f.ctx)
				require.NoError(t, err)
				psCount := len(f.s.cfg.SlashingPool.PendingProposerSlashings(f.ctx, st, true))
				asCount := len(f.s.cfg.SlashingPool.PendingAttesterSlashings(f.ctx, st, true))
				exits, err = f.s.cfg.ExitPool.ExitsForInclusion(st, st.Slot())
				require.NoError(t, err)
				t.Logf("recovered proposer=%d attester=%d exits=%d failed writes=%d", psCount, asCount, len(exits), d.failures)
				assert.Equal(t, 1, psCount)
				assert.Equal(t, 1, asCount)
				assert.Equal(t, 1, len(exits))
				if mode == "database failure until finalization" {
					found := false
					for len(events) > 0 {
						event := <-events
						if event.Type == statefeed.Reorg {
							found = true
							reorg := event.Data.(*qrlpb.EventChainReorg)
							t.Logf("reported depth=%d, expected depth=12 (fork at slot 11)", reorg.Depth)
							assert.Equal(t, uint64(12), reorg.Depth)
						}
					}
					require.Equal(t, true, found)
				}
			})
		})
	}
}

func TestReceiveBlockBatch_SkippedCheckpointSlot(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, skip := range []bool{false, true} {
		t.Run(map[bool]string{false: "checkpoint block present control", true: "checkpoint slot skipped"}[skip], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 11)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			branch := append([]blocks.ROBlock(nil), f.blks[2:]...)
			pre := f.states[11].Copy()
			start := primitives.Slot(12)
			if skip {
				start++
			}
			for slot := start; slot <= 23; slot++ {
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
				driftGenesisTime(f.s, 23, -30)
				err := f.s.ReceiveBlockBatch(f.ctx, branch)
				synctest.Wait()
				t.Logf("batch error=%v; checkpoint target block A11 saved=%t cached=%t", err, f.s.cfg.BeaconDB.HasBlock(f.ctx, f.blks[10].Root()), f.s.hasBlockInInitSyncOrDB(f.ctx, f.blks[10].Root()))
				require.NoError(t, err)
				published, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, branch[len(branch)-1].Root(), bytesutil.ToBytes32(published))
			})
		})
	}
}
