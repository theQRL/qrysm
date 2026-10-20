package blockchain

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-qrl/common"
	blockchainTesting "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/db"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	mockExecution "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/operations/slashings"
	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/beacon-chain/verification"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestService_ReceiveBlock(t *testing.T) {
	ctx := context.Background()

	genesis, keys := util.DeterministicGenesisStateZond(t, 64)
	copiedGen := genesis.Copy()
	genFullBlock := func(t *testing.T, conf *util.BlockGenConfig, slot primitives.Slot) *qrysmpb.SignedBeaconBlockZond {
		blk, err := util.GenerateFullBlockZond(copiedGen.Copy(), keys, conf, slot)
		require.NoError(t, err)
		return blk
	}
	//params.SetupTestConfigCleanupWithLock(t)
	bc := params.BeaconConfig().Copy()
	bc.ShardCommitteePeriod = 0 // Required for voluntary exits test in reasonable time.
	params.OverrideBeaconConfig(bc)

	type args struct {
		block *qrysmpb.SignedBeaconBlockZond
	}
	tests := []struct {
		name      string
		args      args
		wantedErr string
		check     func(*testing.T, *Service)
	}{
		{
			name: "applies block with state transition",
			args: args{
				block: genFullBlock(t, util.DefaultBlockGenConfig(), 2),
			},
			check: func(t *testing.T, s *Service) {
				if hs := s.head.state.Slot(); hs != 2 {
					t.Errorf("Unexpected state slot. Got %d but wanted %d", hs, 2)
				}
				if bs := s.head.block.Block().Slot(); bs != 2 {
					t.Errorf("Unexpected head block slot. Got %d but wanted %d", bs, 2)
				}
			},
		},
		{
			name: "saves attestations to pool",
			args: args{
				block: genFullBlock(t,
					&util.BlockGenConfig{
						NumProposerSlashings: 0,
						NumAttesterSlashings: 0,
						NumAttestations:      2,
						NumDeposits:          0,
						NumVoluntaryExits:    0,
					},
					1,
				),
			},
			check: func(t *testing.T, s *Service) {
				if baCount := len(s.cfg.AttPool.BlockAttestations()); baCount != 0 {
					t.Errorf("Did not get the correct number of block attestations saved to the pool. "+
						"Got %d but wanted %d", baCount, 0)
				}
			},
		},
		{
			name: "updates exit pool",
			args: args{
				block: genFullBlock(t, &util.BlockGenConfig{
					NumProposerSlashings: 0,
					NumAttesterSlashings: 0,
					NumAttestations:      0,
					NumDeposits:          0,
					NumVoluntaryExits:    3,
				},
					1,
				),
			},
			check: func(t *testing.T, s *Service) {
				pending, err := s.cfg.ExitPool.PendingExits()
				require.NoError(t, err)
				if len(pending) != 0 {
					t.Errorf(
						"Did not mark the correct number of exits. Got %d pending but wanted %d",
						len(pending),
						0,
					)
				}
			},
		},
		{
			name: "notifies block processed on state feed",
			args: args{
				block: genFullBlock(t, util.DefaultBlockGenConfig(), 1),
			},
			check: func(t *testing.T, s *Service) {
				// Poll until the state-feed event arrives, rather than sleeping
				// for a fixed 100ms (which is racy on slow CI).
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					if recvd := len(s.cfg.StateNotifier.(*blockchainTesting.MockStateNotifier).ReceivedEvents()); recvd >= 1 {
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
				t.Errorf("Did not receive any state notifications within 5s")
			},
		},
	}

	wg := new(sync.WaitGroup)
	for _, tt := range tests {
		wg.Add(1)
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				wg.Done()
			}()
			genesis = genesis.Copy()
			s, tr := minimalTestService(t,
				WithFinalizedStateAtStartUp(genesis),
				WithExitPool(voluntaryexits.NewPool()),
				WithStateNotifier(&blockchainTesting.MockStateNotifier{RecordEvents: true}))

			beaconDB := tr.db
			genesisBlockRoot := bytesutil.ToBytes32(nil)
			require.NoError(t, beaconDB.SaveState(ctx, genesis, genesisBlockRoot))

			// Initialize it here.
			_ = s.cfg.StateNotifier.StateFeed()
			require.NoError(t, s.saveGenesisData(ctx, genesis))
			root, err := tt.args.block.Block.HashTreeRoot()
			require.NoError(t, err)
			wsb, err := blocks.NewSignedBeaconBlock(tt.args.block)
			require.NoError(t, err)
			err = s.ReceiveBlock(ctx, wsb, root)
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				require.NoError(t, err)
				tt.check(t, s)
			}
		})
	}
	wg.Wait()
}

func TestService_ReceiveBlockUpdateHead(t *testing.T) {
	s, tr := minimalTestService(t,
		WithExitPool(voluntaryexits.NewPool()),
		WithStateNotifier(&blockchainTesting.MockStateNotifier{RecordEvents: true}))
	ctx, beaconDB := tr.ctx, tr.db
	genesis, keys := util.DeterministicGenesisStateZond(t, 64)
	b, err := util.GenerateFullBlockZond(genesis, keys, util.DefaultBlockGenConfig(), 1)
	assert.NoError(t, err)
	genesisBlockRoot := bytesutil.ToBytes32(nil)
	require.NoError(t, beaconDB.SaveState(ctx, genesis, genesisBlockRoot))

	// Initialize it here.
	_ = s.cfg.StateNotifier.StateFeed()
	require.NoError(t, s.saveGenesisData(ctx, genesis))
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wg := sync.WaitGroup{}
	wg.Go(func() {
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		require.NoError(t, s.ReceiveBlock(ctx, wsb, root))
	})
	wg.Wait()
	// Poll for the state-feed event instead of sleeping.
	deadline := time.Now().Add(5 * time.Second)
	got := false
	for time.Now().Before(deadline) {
		if recvd := len(s.cfg.StateNotifier.(*blockchainTesting.MockStateNotifier).ReceivedEvents()); recvd >= 1 {
			got = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !got {
		t.Errorf("Did not receive any state notifications within 5s")
	}
	// Verify fork choice has processed the block. (Genesis block and the new block)
	assert.Equal(t, 2, s.cfg.ForkChoiceStore.NodeCount())
}

// A rebroadcast of an already-imported block must short-circuit before
// running prestate fetch / block copy / state-transition validation.
func TestService_ReceiveBlock_AlreadyInForkchoiceShortCircuits(t *testing.T) {
	s, tr := minimalTestService(t,
		WithExitPool(voluntaryexits.NewPool()),
		WithStateNotifier(&blockchainTesting.MockStateNotifier{RecordEvents: true}))
	ctx, beaconDB := tr.ctx, tr.db
	genesis, keys := util.DeterministicGenesisStateZond(t, 64)
	b, err := util.GenerateFullBlockZond(genesis, keys, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	genesisBlockRoot := bytesutil.ToBytes32(nil)
	require.NoError(t, beaconDB.SaveState(ctx, genesis, genesisBlockRoot))
	_ = s.cfg.StateNotifier.StateFeed()
	require.NoError(t, s.saveGenesisData(ctx, genesis))

	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)

	// First receive: full import. Forkchoice ends with genesis + new block.
	require.NoError(t, s.ReceiveBlock(ctx, wsb, root))
	require.Equal(t, true, s.InForkchoice(root))
	require.Equal(t, 2, s.cfg.ForkChoiceStore.NodeCount())

	// Second receive of the same block: must early-return via the new
	// InForkchoice short-circuit and emit its debug log.
	hook := logTest.NewGlobal()
	require.NoError(t, s.ReceiveBlock(ctx, wsb, root))
	require.LogsContain(t, hook, "Ignoring block already in forkchoice")
	// Forkchoice unchanged — the second call did not re-import.
	require.Equal(t, 2, s.cfg.ForkChoiceStore.NodeCount())
}

func TestService_ReceiveBlockBatch(t *testing.T) {
	ctx := context.Background()

	genesis, keys := util.DeterministicGenesisStateZond(t, 64)
	genFullBlock := func(t *testing.T, conf *util.BlockGenConfig, slot primitives.Slot) *qrysmpb.SignedBeaconBlockZond {
		blk, err := util.GenerateFullBlockZond(genesis, keys, conf, slot)
		assert.NoError(t, err)
		return blk
	}

	type args struct {
		block *qrysmpb.SignedBeaconBlockZond
	}
	tests := []struct {
		name      string
		args      args
		wantedErr string
		check     func(*testing.T, *Service)
	}{
		{
			name: "applies block with state transition",
			args: args{
				block: genFullBlock(t, util.DefaultBlockGenConfig(), 2 /*slot*/),
			},
			check: func(t *testing.T, s *Service) {
				assert.Equal(t, primitives.Slot(2), s.head.state.Slot(), "Incorrect head state slot")
				assert.Equal(t, primitives.Slot(2), s.head.block.Block().Slot(), "Incorrect head block slot")
			},
		},
		{
			name: "notifies block processed on state feed",
			args: args{
				block: genFullBlock(t, util.DefaultBlockGenConfig(), 1 /*slot*/),
			},
			check: func(t *testing.T, s *Service) {
				time.Sleep(100 * time.Millisecond)
				if recvd := len(s.cfg.StateNotifier.(*blockchainTesting.MockStateNotifier).ReceivedEvents()); recvd < 1 {
					t.Errorf("Received %d state notifications, expected at least 1", recvd)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := minimalTestService(t, WithStateNotifier(&blockchainTesting.MockStateNotifier{RecordEvents: true}))
			err := s.saveGenesisData(ctx, genesis)
			require.NoError(t, err)
			wsb, err := blocks.NewSignedBeaconBlock(tt.args.block)
			require.NoError(t, err)
			rwsb, err := blocks.NewROBlock(wsb)
			require.NoError(t, err)
			err = s.ReceiveBlockBatch(ctx, []blocks.ROBlock{rwsb})
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
				tt.check(t, s)
			}
		})
	}
}

func TestService_ReceiveBlockBatch_HeadSelection(t *testing.T) {
	for _, competing := range []bool{false, true} {
		t.Run(map[bool]string{false: "linear chain", true: "competing branch wins"}[competing], func(t *testing.T) {
			s, tr := minimalTestService(t)
			ctx := tr.ctx
			genesis, keys := util.DeterministicGenesisStateZond(t, 64)
			s.genesisTime = time.Unix(int64(genesis.GenesisTime()), 0)
			require.NoError(t, s.saveGenesisData(ctx, genesis))
			b, err := util.GenerateFullBlockZond(genesis.Copy(), keys, util.DefaultBlockGenConfig(), 2)
			require.NoError(t, err)
			block, err := blocks.NewSignedBeaconBlock(b)
			require.NoError(t, err)
			batchBlock, err := blocks.NewROBlock(block)
			require.NoError(t, err)
			wantRoot := batchBlock.Root()
			wantSlot := batchBlock.Block().Slot()
			wantStateRoot := batchBlock.Block().StateRoot()
			if competing {
				// Store another branch without publishing it as the service head.
				b, err := util.GenerateFullBlockZond(genesis.Copy(), keys, util.DefaultBlockGenConfig(), 1)
				require.NoError(t, err)
				block, err := blocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				branch, err := blocks.NewROBlock(block)
				require.NoError(t, err)
				post, err := s.validateStateTransition(ctx, genesis.Copy(), block)
				require.NoError(t, err)
				require.NoError(t, s.savePostStateInfo(ctx, branch.Root(), block, post))
				require.NoError(t, s.cfg.ForkChoiceStore.InsertNode(ctx, post, branch))
				require.NoError(t, s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Root: s.originBlockRoot}))
				s.cfg.ForkChoiceStore.ProcessAttestation(ctx, []uint64{0}, branch.Root(), 1)
				wantRoot, wantSlot, wantStateRoot = branch.Root(), branch.Block().Slot(), branch.Block().StateRoot()
				// Only an FCU for the winning branch will mark it valid.
				payload, err := branch.Block().Body().Execution()
				require.NoError(t, err)
				s.cfg.ExecutionEngineCaller.(*mockExecution.EngineClient).OverrideValidHash = bytesutil.ToBytes32(payload.BlockHash())
			}

			require.NoError(t, s.ReceiveBlockBatch(ctx, []blocks.ROBlock{batchBlock}))
			serviceRoot, err := s.HeadRoot(ctx)
			require.NoError(t, err)
			require.DeepEqual(t, wantRoot[:], serviceRoot)
			require.Equal(t, wantRoot, s.CachedHeadRoot())
			require.Equal(t, wantSlot, s.HeadSlot())
			headState, err := s.HeadState(ctx)
			require.NoError(t, err)
			stateRoot, err := headState.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, wantStateRoot, stateRoot)
			canonical, err := s.IsCanonical(ctx, wantRoot)
			require.NoError(t, err)
			require.Equal(t, true, canonical)
			canonical, err = s.IsCanonical(ctx, batchBlock.Root())
			require.NoError(t, err)
			require.Equal(t, !competing, canonical)
			optimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(wantRoot)
			require.NoError(t, err)
			require.Equal(t, !competing, optimistic)
			require.Equal(t, optimistic, s.head.optimistic)
		})
	}
}

func TestService_ReceiveBlockBatch_AttestationsSelectHead(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.SlotsPerEpoch = 6
	params.OverrideBeaconConfig(config)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	transition.SkipSlotCache.Disable()
	t.Cleanup(transition.SkipSlotCache.Enable)
	// One existing vote favors A, while the batch carries a full committee's
	// votes for B, including one slashed member. Both import paths must select
	// B's tip with the same weight, excluding the slashed validator's vote.
	weights := make(map[string]uint64)
	for _, mode := range []string{"batch", "gossip"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := minimalTestService(t, WithSlashingPool(slashings.NewPool()))
			synctest.Test(t, func(t *testing.T) {
				// Drain background cache and head-event work before closing the DB
				// or restoring the global caches and configuration.
				t.Cleanup(synctest.Wait)
				ctx := context.Background()
				genesis, keys := util.DeterministicGenesisStateZond(t, 64)
				genesisTime := uint64(time.Now().Unix()) - 5*params.BeaconConfig().SecondsPerSlot
				require.NoError(t, genesis.SetGenesisTime(genesisTime))
				makeBlock := func(parent state.BeaconState, slot primitives.Slot, conf *util.BlockGenConfig) blocks.ROBlock {
					t.Helper()
					b, err := util.GenerateFullBlockZond(parent.Copy(), keys, conf, slot)
					require.NoError(t, err)
					signed, err := blocks.NewSignedBeaconBlock(b)
					require.NoError(t, err)
					ro, err := blocks.NewROBlock(signed)
					require.NoError(t, err)
					return ro
				}
				branchA := makeBlock(genesis, 1, &util.BlockGenConfig{})
				branchB := makeBlock(genesis, 2, &util.BlockGenConfig{})
				postB, err := transition.ExecuteStateTransition(ctx, genesis.Copy(), branchB)
				require.NoError(t, err)
				tipConfig := util.DefaultBlockGenConfig()
				tipConfig.NumAttesterSlashings = 1
				tipB := makeBlock(postB, 3, tipConfig)
				require.Equal(t, 1, len(tipB.Block().Body().Attestations()))
				require.Equal(t, 1, len(tipB.Block().Body().AttesterSlashings()))
				require.Equal(t, branchB.Root(), bytesutil.ToBytes32(tipB.Block().Body().Attestations()[0].Data.BeaconBlockRoot))
				committee, err := helpers.BeaconCommitteeFromState(ctx, genesis, 1, 0)
				require.NoError(t, err)
				require.Equal(t, true, len(committee) > 0)
				s.SetGenesisTime(time.Unix(int64(genesisTime), 0))
				require.NoError(t, s.saveGenesisData(ctx, genesis.Copy()))
				require.NoError(t, s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Root: s.originBlockRoot}))
				require.NoError(t, s.ReceiveBlock(ctx, branchA, branchA.Root()))
				s.cfg.ForkChoiceStore.ProcessAttestation(ctx, []uint64{uint64(committee[0])}, branchA.Root(), 0)
				s.UpdateHead(ctx, s.CurrentSlot())
				if mode == "batch" {
					require.NoError(t, s.ReceiveBlockBatch(ctx, []blocks.ROBlock{branchB, tipB}))
				} else {
					require.NoError(t, s.ReceiveBlock(ctx, branchB, branchB.Root()))
					require.NoError(t, s.ReceiveBlock(ctx, tipB, tipB.Root()))
				}
				require.Equal(t, tipB.Root(), s.CachedHeadRoot())
				serviceRoot, err := s.HeadRoot(ctx)
				require.NoError(t, err)
				require.Equal(t, tipB.Root(), bytesutil.ToBytes32(serviceRoot))
				weights[mode], err = s.cfg.ForkChoiceStore.Weight(branchB.Root())
				require.NoError(t, err)
				wantWeight := (tipB.Block().Body().Attestations()[0].AggregationBits.Count() - 1) * params.BeaconConfig().MaxEffectiveBalance
				require.Equal(t, wantWeight, weights[mode])
				require.Equal(t, true, weights[mode] > params.BeaconConfig().MaxEffectiveBalance)
			})
		})
	}
	require.Equal(t, weights["gossip"], weights["batch"])
}

func TestService_ReceiveBlockBatch_CrossEpochVotes(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.SlotsPerEpoch = 6
	params.OverrideBeaconConfig(config)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	weights := make(map[string][]uint64)
	for _, mode := range []string{"batch", "gossip"} {
		t.Run(mode, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 8)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 20, 0)
				require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forkchoicetypes.Checkpoint{Root: f.s.originBlockRoot}))
				if mode == "batch" {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				} else {
					for _, b := range f.blks[2:] {
						require.NoError(t, f.s.ReceiveBlock(f.ctx, b, b.Root()))
					}
				}
				require.Equal(t, f.blks[7].Root(), f.s.CachedHeadRoot())
				for _, b := range f.blks {
					weight, err := f.s.cfg.ForkChoiceStore.Weight(b.Root())
					require.NoError(t, err)
					weights[mode] = append(weights[mode], weight)
				}
				require.Equal(t, true, weights[mode][6] > 0, "count votes from the second epoch")
				require.Equal(t, 0, len(f.s.cfg.AttPool.BlockAttestations()))
			})
		})
	}
	require.DeepEqual(t, weights["gossip"], weights["batch"])
}

func TestService_ReceiveBlockBatch_UnrealizedCheckpoints(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.SlotsPerEpoch = 6
	params.OverrideBeaconConfig(config)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	for _, tt := range []struct {
		name              string
		currentSlot       int64
		initialJustified  primitives.Epoch
		importedJustified primitives.Epoch
	}{
		{name: "current epoch", currentSlot: 17},
		{name: "older epoch", currentSlot: 25, initialJustified: 1, importedJustified: 2},
	} {
		for _, mode := range []string{"batch", "gossip"} {
			t.Run(tt.name+"/"+mode, func(t *testing.T) {
				f := newBatchExecutionFixture(t, 17)
				synctest.Test(t, func(t *testing.T) {
					t.Cleanup(synctest.Wait)
					driftGenesisTime(f.s, tt.currentSlot, 0)
					require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forkchoicetypes.Checkpoint{Root: f.s.originBlockRoot}))
					require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Root: f.s.originBlockRoot}))
					for _, b := range f.blks[2:12] {
						require.NoError(t, f.s.ReceiveBlock(f.ctx, b, b.Root()))
					}
					require.Equal(t, tt.initialJustified, f.s.cfg.ForkChoiceStore.JustifiedCheckpoint().Epoch)
					if mode == "batch" {
						require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[12:]))
					} else {
						for _, b := range f.blks[12:] {
							require.NoError(t, f.s.ReceiveBlock(f.ctx, b, b.Root()))
						}
					}
					require.Equal(t, f.blks[16].Root(), f.s.CachedHeadRoot(), "a valid extension must remain eligible for head")
					require.Equal(t, tt.importedJustified, f.s.cfg.ForkChoiceStore.JustifiedCheckpoint().Epoch)
					// Current-epoch observations must survive until the epoch tick;
					// older observations must already have been realized on import.
					nextEpochSlot := (tt.currentSlot/6 + 1) * 6
					synctest.Wait()
					driftGenesisTime(f.s, nextEpochSlot, 0)
					require.NoError(t, f.s.NewSlot(f.ctx, primitives.Slot(nextEpochSlot)))
					f.s.UpdateHead(f.ctx, primitives.Slot(nextEpochSlot))
					require.Equal(t, f.blks[16].Root(), f.s.CachedHeadRoot())
					require.Equal(t, primitives.Epoch(2), f.s.cfg.ForkChoiceStore.JustifiedCheckpoint().Epoch)
					require.Equal(t, f.blks[11].Root(), f.s.cfg.ForkChoiceStore.JustifiedCheckpoint().Root)
				})
			})
		}
	}
}

func TestService_ReceiveBlockBatch_FinalizedVotes(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.SlotsPerEpoch = 6
	params.OverrideBeaconConfig(config)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	for _, validateHead := range []bool{false, true} {
		t.Run(map[bool]string{false: "optimistic suffix", true: "FCU validates suffix"}[validateHead], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 26)
			payload, err := f.blks[2].Block().Body().Execution()
			require.NoError(t, err)
			f.s.cfg.ExecutionEngineCaller = &batchMixedValidationEngine{EngineClient: f.engine, validHash: bytesutil.ToBytes32(payload.BlockHash())}
			validThrough := primitives.Slot(3)
			if validateHead {
				f.engine.ErrForkchoiceUpdated = nil
				validThrough = 26
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 28, 0)
				events := make(chan *feed.Event, 32)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				require.NoError(t, f.s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(f.ctx, &forkchoicetypes.Checkpoint{Root: f.s.originBlockRoot}))
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				require.Equal(t, primitives.Epoch(2), f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(f.blks[1].Root()), "the batch prunes early attested blocks")
				require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(f.blks[2].Root()), "the validated prefix was also pruned")
				require.Equal(t, 0, len(f.s.cfg.AttPool.BlockAttestations()), "do not queue obsolete votes for pruned blocks")
				require.Equal(t, f.blks[25].Root(), f.s.CachedHeadRoot())
				weight, err := f.s.cfg.ForkChoiceStore.Weight(f.blks[24].Root())
				require.NoError(t, err)
				require.Equal(t, true, weight > 0, "retain votes for blocks after finality")
				optimistic, err := f.s.cfg.ForkChoiceStore.IsOptimistic(f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Root)
				require.NoError(t, err)
				require.Equal(t, !validateHead, optimistic, "validating a pruned ancestor must not validate the surviving descendants")
				requireBlockEventOptimism(t, events, f.blks[2:], validThrough)
			})
		})
	}
}

func requireBlockEventOptimism(t *testing.T, events <-chan *feed.Event, blks []blocks.ROBlock, validThrough primitives.Slot) {
	t.Helper()
	var processed []*statefeed.BlockProcessedData
	for len(events) > 0 {
		event := <-events
		if event.Type == statefeed.BlockProcessed {
			processed = append(processed, event.Data.(*statefeed.BlockProcessedData))
		}
	}
	require.Equal(t, len(blks), len(processed))
	for i, data := range processed {
		require.Equal(t, blks[i].Root(), data.BlockRoot, "preserve block event ordering")
		require.Equal(t, blks[i].Block().Slot(), data.Slot)
		require.Equal(t, true, data.Verified)
		require.Equal(t, data.Slot > validThrough, data.Optimistic, "use this block's execution status")
	}
}

type batchMixedValidationEngine struct {
	*mockExecution.EngineClient
	validHash [32]byte
}

func (e *batchMixedValidationEngine) NewPayload(_ context.Context, payload interfaces.ExecutionData, _ []common.Hash, _ *common.Hash) ([]byte, error) {
	if bytesutil.ToBytes32(payload.BlockHash()) == e.validHash {
		return payload.BlockHash(), nil
	}
	return nil, execution.ErrAcceptedSyncingPayloadStatus
}

func TestService_ReceiveBlockBatch_ValidatedPrefix(t *testing.T) {
	for _, mode := range []string{"batch", "gossip"} {
		t.Run(mode, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 4)
			payload, err := f.blks[2].Block().Body().Execution()
			require.NoError(t, err)
			f.s.cfg.ExecutionEngineCaller = &batchMixedValidationEngine{EngineClient: f.engine, validHash: bytesutil.ToBytes32(payload.BlockHash())}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 20, 0)
				events := make(chan *feed.Event, 16)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				if mode == "batch" {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				} else {
					for _, b := range f.blks[2:] {
						require.NoError(t, f.s.ReceiveBlock(f.ctx, b, b.Root()))
					}
				}
				require.Equal(t, f.blks[3].Root(), f.s.CachedHeadRoot())
				for i, b := range f.blks {
					optimistic, err := f.s.IsOptimisticForRoot(f.ctx, b.Root())
					require.NoError(t, err)
					require.Equal(t, i == 3, optimistic, "only the SYNCING suffix should remain optimistic")
				}
				synctest.Wait()
				requireBlockEventOptimism(t, events, f.blks[2:], 3)
			})
		})
	}
}

func TestService_ReceiveBlockBatch_FailurePreservesHeadState(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.SlotsPerEpoch = 6
	params.OverrideBeaconConfig(config)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	// Initial sync disables this cache, so transitions mutate the batch pre-state.
	transition.SkipSlotCache.Disable()
	defer transition.SkipSlotCache.Enable()
	for _, failure := range []string{"signature", "execution RPC"} {
		t.Run(failure, func(t *testing.T) {
			s, tr := minimalTestService(t)
			ctx := tr.ctx
			genesis, keys := util.DeterministicGenesisStateZond(t, 64)
			// Use a distinct genesis to avoid next-slot states cached by other tests.
			genesisTime := uint64(time.Now().Unix()) - 10*params.BeaconConfig().SecondsPerSlot
			require.NoError(t, genesis.SetGenesisTime(genesisTime))
			s.genesisTime = time.Unix(int64(genesisTime), 0)
			require.NoError(t, s.saveGenesisData(ctx, genesis))
			require.NoError(t, s.cfg.ForkChoiceStore.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Root: s.originBlockRoot}))
			b1, err := util.GenerateFullBlockZond(genesis.Copy(), keys, util.DefaultBlockGenConfig(), 1)
			require.NoError(t, err)
			block1, err := blocks.NewSignedBeaconBlock(b1)
			require.NoError(t, err)
			ro1, err := blocks.NewROBlock(block1)
			require.NoError(t, err)
			require.NoError(t, s.ReceiveBlockBatch(ctx, []blocks.ROBlock{ro1}))
			beforeWeight, err := s.cfg.ForkChoiceStore.Weight(ro1.Root())
			require.NoError(t, err)
			before, err := s.HeadState(ctx)
			require.NoError(t, err)
			readOnlyHead, err := s.HeadStateReadOnly(ctx)
			require.NoError(t, err)
			beforeRoot, err := before.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, ro1.Block().StateRoot(), beforeRoot)
			b2, err := util.GenerateFullBlockZond(before.Copy(), keys, util.DefaultBlockGenConfig(), 2)
			require.NoError(t, err)
			signature := bytesutil.SafeCopyBytes(b2.Signature)
			engine := s.cfg.ExecutionEngineCaller.(*mockExecution.EngineClient)
			previousPayloadError := engine.ErrNewPayload
			wantError := "batch block signature verification failed"
			if failure == "signature" {
				b2.Signature[0] ^= 1
			} else {
				wantError = "temporary execution RPC failure"
				engine.ErrNewPayload = errors.New(wantError)
			}
			block2, err := blocks.NewSignedBeaconBlock(b2)
			require.NoError(t, err)
			ro2, err := blocks.NewROBlock(block2)
			require.NoError(t, err)
			events := make(chan *feed.Event, 4)
			sub := s.cfg.StateNotifier.StateFeed().Subscribe(events)
			defer sub.Unsubscribe()
			require.ErrorContains(t, wantError, s.ReceiveBlockBatch(ctx, []blocks.ROBlock{ro2}))
			require.Equal(t, 0, len(events), "a rejected batch must not emit processed-block events")
			head, err := s.cfg.ForkChoiceStore.Head(ctx)
			require.NoError(t, err)
			require.Equal(t, ro1.Root(), head)
			afterWeight, err := s.cfg.ForkChoiceStore.Weight(ro1.Root())
			require.NoError(t, err)
			require.Equal(t, beforeWeight, afterWeight, "a rejected batch must not publish votes")
			require.Equal(t, 0, len(s.cfg.AttPool.BlockAttestations()))
			require.Equal(t, false, s.cfg.ForkChoiceStore.HasNode(ro2.Root()))
			require.Equal(t, ro1.Root(), s.CachedHeadRoot())
			root, err := s.HeadRoot(ctx)
			require.NoError(t, err)
			root1 := ro1.Root()
			require.DeepEqual(t, root1[:], root)
			after, err := s.HeadState(ctx)
			require.NoError(t, err)
			afterRoot, err := after.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, beforeRoot, afterRoot)
			require.Equal(t, before.Slot(), after.Slot())
			require.Equal(t, before.Slot(), s.HeadSlot())
			require.Equal(t, before.Slot(), readOnlyHead.Slot())

			// A valid retry must still advance the selected head and its state.
			b2.Signature = signature
			engine.ErrNewPayload = previousPayloadError
			block2, err = blocks.NewSignedBeaconBlock(b2)
			require.NoError(t, err)
			ro2, err = blocks.NewROBlock(block2)
			require.NoError(t, err)
			require.NoError(t, s.ReceiveBlockBatch(ctx, []blocks.ROBlock{ro2}))
			afterWeight, err = s.cfg.ForkChoiceStore.Weight(ro1.Root())
			require.NoError(t, err)
			require.Equal(t, true, afterWeight > beforeWeight, "a valid retry must apply the votes")
			root, err = s.HeadRoot(ctx)
			require.NoError(t, err)
			root2 := ro2.Root()
			require.DeepEqual(t, root2[:], root)
			require.Equal(t, root2, s.CachedHeadRoot())
			after, err = s.HeadState(ctx)
			require.NoError(t, err)
			afterRoot, err = after.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, ro2.Block().StateRoot(), afterRoot)
			require.Equal(t, ro2.Block().Slot(), after.Slot())
			require.Equal(t, before.Slot(), readOnlyHead.Slot())
			requireBlockEventOptimism(t, events, []blocks.ROBlock{ro2}, 0)
		})
	}
}

type batchExecutionFixture struct {
	s      *Service
	ctx    context.Context
	engine *mockExecution.EngineClient
	keys   []ml_dsa_87.MLDSA87Key
	blks   []blocks.ROBlock
	states []state.BeaconState
}

func newBatchExecutionFixture(t *testing.T, blockCount int) *batchExecutionFixture {
	t.Helper()
	transition.SkipSlotCache.Disable()
	t.Cleanup(transition.SkipSlotCache.Enable)
	engine := &mockExecution.EngineClient{}
	s, tr := minimalTestService(t, WithExecutionEngineCaller(engine))
	genesis, keys := util.DeterministicGenesisStateZond(t, 64)
	genesisTime := uint64(time.Now().Unix()) - uint64(max(20, blockCount+2))*params.BeaconConfig().SecondsPerSlot
	require.NoError(t, genesis.SetGenesisTime(genesisTime))
	s.genesisTime = time.Unix(int64(genesisTime), 0)
	require.NoError(t, s.saveGenesisData(tr.ctx, genesis))
	f := &batchExecutionFixture{s: s, ctx: tr.ctx, engine: engine, keys: keys, states: []state.BeaconState{genesis}}
	for i := 1; i <= blockCount; i++ {
		b, err := util.GenerateFullBlockZond(f.states[i-1].Copy(), keys, util.DefaultBlockGenConfig(), primitives.Slot(i))
		require.NoError(t, err)
		block, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		ro, err := blocks.NewROBlock(block)
		require.NoError(t, err)
		post, err := transition.ExecuteStateTransition(tr.ctx, f.states[i-1].Copy(), ro)
		require.NoError(t, err)
		f.blks = append(f.blks, ro)
		f.states = append(f.states, post)
	}
	// A is validated and B is imported optimistically; the rest are pending.
	// Import the prefix before later epochs have been processed. Individual
	// tests then advance the clock to their own epoch boundary scenarios.
	driftGenesisTime(s, 3, 0)
	require.NoError(t, s.ReceiveBlockBatch(tr.ctx, f.blks[:1]))
	engine.ErrNewPayload = execution.ErrAcceptedSyncingPayloadStatus
	engine.ErrForkchoiceUpdated = execution.ErrAcceptedSyncingPayloadStatus
	require.NoError(t, s.ReceiveBlockBatch(tr.ctx, f.blks[1:2]))
	require.Equal(t, f.blks[1].Root(), s.CachedHeadRoot())
	s.SetGenesisTime(time.Unix(int64(genesisTime), 0))
	s.cfg.ForkChoiceStore.SetGenesisTime(genesisTime)
	return f
}

type batchPayloadErrorEngine struct {
	*mockExecution.EngineClient
	failureHash [32]byte
	payloadErr  error
	lastValid   []byte
	calls       int
}

func (e *batchPayloadErrorEngine) NewPayload(_ context.Context, payload interfaces.ExecutionData, _ []common.Hash, _ *common.Hash) ([]byte, error) {
	e.calls++
	if bytesutil.ToBytes32(payload.BlockHash()) == e.failureHash {
		return e.lastValid, e.payloadErr
	}
	return nil, execution.ErrAcceptedSyncingPayloadStatus
}

func TestService_ReceiveBlockBatch_InvalidPayload(t *testing.T) {
	for _, tt := range []struct {
		name        string
		rejected    int
		lastValid   int
		wantHead    int
		wantInvalid []int
		rpcFailure  bool
	}{
		{name: "first payload rejects imported ancestor", rejected: 3, lastValid: 1, wantHead: 1, wantInvalid: []int{3, 2}},
		{name: "later payload rejects imported ancestor", rejected: 5, lastValid: 1, wantHead: 1, wantInvalid: []int{5, 4, 3, 2}},
		{name: "latest valid is the batch parent", rejected: 5, lastValid: 2, wantHead: 2, wantInvalid: []int{5, 4, 3}},
		{name: "latest valid is pending", rejected: 5, lastValid: 3, wantHead: 2, wantInvalid: []int{5, 4}},
		{name: "latest valid is on another branch", rejected: 5, wantHead: 2, wantInvalid: []int{5}},
		{name: "later RPC failure preserves ancestors", rejected: 5, wantHead: 2, rpcFailure: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 5)
			rejectedPayload, err := f.blks[tt.rejected-1].Block().Body().Execution()
			require.NoError(t, err)
			lvh := [32]byte{'x'}
			if tt.lastValid > 0 {
				payload, err := f.blks[tt.lastValid-1].Block().Body().Execution()
				require.NoError(t, err)
				lvh = bytesutil.ToBytes32(payload.BlockHash())
			}
			engine := &batchPayloadErrorEngine{
				EngineClient: f.engine,
				failureHash:  bytesutil.ToBytes32(rejectedPayload.BlockHash()),
				payloadErr:   execution.ErrInvalidPayloadStatus,
				lastValid:    lvh[:],
			}
			wantError := "received an INVALID payload"
			if tt.rpcFailure {
				wantError = "temporary execution RPC failure"
				engine.payloadErr = errors.New(wantError)
			}
			f.s.cfg.ExecutionEngineCaller = engine
			err = f.s.ReceiveBlockBatch(f.ctx, f.blks[2:])
			require.ErrorContains(t, wantError, err)
			require.Equal(t, tt.rejected-2, engine.calls)
			require.Equal(t, !tt.rpcFailure, IsInvalidBlock(err))
			require.Equal(t, !tt.rpcFailure, errors.Is(err, verification.ErrInvalid))
			if !tt.rpcFailure {
				require.Equal(t, f.blks[tt.rejected-1].Root(), InvalidBlockRoot(err))
				require.Equal(t, lvh, InvalidBlockLVH(err))
			}
			wantRoots := make([][32]byte, 0, len(tt.wantInvalid))
			for _, i := range tt.wantInvalid {
				wantRoots = append(wantRoots, f.blks[i-1].Root())
			}
			require.DeepEqual(t, wantRoots, InvalidAncestorRoots(err))
			// No part of an execution-rejected batch should have been persisted.
			for _, b := range f.blks[2:] {
				require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(b.Root()))
				require.Equal(t, false, f.s.HasBlock(f.ctx, b.Root()))
				require.Equal(t, false, f.s.cfg.BeaconDB.HasStateSummary(f.ctx, b.Root()))
			}
			for i, b := range f.blks[:2] {
				wantPresent := i < tt.wantHead
				require.Equal(t, wantPresent, f.s.cfg.ForkChoiceStore.HasNode(b.Root()))
				require.Equal(t, wantPresent, f.s.HasBlock(f.ctx, b.Root()))
			}
			f.s.cfg.ForkChoiceStore.Lock()
			head, err := f.s.cfg.ForkChoiceStore.Head(f.ctx)
			f.s.cfg.ForkChoiceStore.Unlock()
			require.NoError(t, err)
			require.Equal(t, f.blks[tt.wantHead-1].Root(), head)
			if tt.lastValid == 3 || tt.rpcFailure {
				// A valid pending prefix or a transient RPC failure remains retryable.
				f.s.cfg.ExecutionEngineCaller = f.engine
				f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
				end := 3
				if tt.rpcFailure {
					end = len(f.blks)
				}
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:end]))
				require.Equal(t, f.blks[end-1].Root(), f.s.CachedHeadRoot())
			}
		})
	}
}

func TestService_ReceiveBlockBatch_InvalidationEvictsCache(t *testing.T) {
	for _, invalidFCU := range []bool{false, true} {
		t.Run(map[bool]string{false: "NewPayload", true: "ForkchoiceUpdated"}[invalidFCU], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 3)
			validPayload, err := f.blks[0].Block().Body().Execution()
			require.NoError(t, err)
			if invalidFCU {
				f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
				f.engine.ForkChoiceUpdatedResp = validPayload.BlockHash()
				f.engine.OverrideValidHash = bytesutil.ToBytes32(validPayload.BlockHash())
			} else {
				f.engine.ErrNewPayload = execution.ErrInvalidPayloadStatus
				f.engine.NewPayloadResp = validPayload.BlockHash()
			}
			// C identifies B as invalid, removing both from every block store.
			require.ErrorContains(t, "received an INVALID payload", f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
			checkRemoved := func() {
				t.Helper()
				for _, b := range f.blks[1:] {
					require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(b.Root()))
					require.Equal(t, false, f.s.hasInitSyncBlock(b.Root()))
					require.Equal(t, false, f.s.HasBlock(f.ctx, b.Root()))
					require.Equal(t, false, f.s.cfg.BeaconDB.HasBlock(f.ctx, b.Root()))
				}
			}
			checkRemoved()
			// A valid sibling of B flushes the initial-sync cache to disk.
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			b, err := util.GenerateFullBlockZond(f.states[1].Copy(), f.keys, util.DefaultBlockGenConfig(), 4)
			require.NoError(t, err)
			block, err := blocks.NewSignedBeaconBlock(b)
			require.NoError(t, err)
			ro, err := blocks.NewROBlock(block)
			require.NoError(t, err)
			require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{ro}))
			require.Equal(t, ro.Root(), f.s.CachedHeadRoot())
			checkRemoved()
			// A descendant must not backfill B before its own signature check.
			bad, err := f.blks[2].Copy()
			require.NoError(t, err)
			sig := bad.Signature()
			sig[0] ^= 1
			bad.(interfaces.SignedBeaconBlock).SetSignature(sig[:])
			badRO, err := blocks.NewROBlock(bad)
			require.NoError(t, err)
			require.ErrorContains(t, "could not reconstruct parent state", f.s.ReceiveBlockBatch(f.ctx, []blocks.ROBlock{badRO}))
			checkRemoved()
		})
	}
}

type invalidHeadCleanupDB struct {
	db.HeadAccessDatabase
	root [32]byte
	err  error
}

func (d *invalidHeadCleanupDB) DeleteBlock(ctx context.Context, root [32]byte) error {
	if root == d.root {
		return d.err
	}
	return d.HeadAccessDatabase.DeleteBlock(ctx, root)
}

func TestService_ReceiveBlock_InvalidHeadCleanup(t *testing.T) {
	for _, failDelete := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleanup succeeds", true: "cleanup fails"}[failDelete], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 3)
			validPayload, err := f.blks[0].Block().Body().Execution()
			require.NoError(t, err)
			f.engine.ErrForkchoiceUpdated = execution.ErrInvalidPayloadStatus
			f.engine.ForkChoiceUpdatedResp = validPayload.BlockHash()
			f.engine.OverrideValidHash = bytesutil.ToBytes32(validPayload.BlockHash())
			deleteErr := errors.New("temporary block deletion failure")
			if failDelete {
				f.s.cfg.BeaconDB = &invalidHeadCleanupDB{HeadAccessDatabase: f.s.cfg.BeaconDB, root: f.blks[2].Root(), err: deleteErr}
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 20, 0)
				events := make(chan *feed.Event, 16)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				err := f.s.ReceiveBlock(f.ctx, f.blks[2], f.blks[2].Root())
				require.ErrorContains(t, "received an INVALID payload", err)
				require.Equal(t, true, IsInvalidBlock(err))
				require.Equal(t, true, errors.Is(err, verification.ErrInvalid))
				require.Equal(t, f.blks[2].Root(), InvalidBlockRoot(err))
				require.Equal(t, bytesutil.ToBytes32(validPayload.BlockHash()), InvalidBlockLVH(err))
				roots := InvalidAncestorRoots(err)
				require.Equal(t, 2, len(roots))
				for _, b := range f.blks[1:] {
					require.Equal(t, true, roots[0] == b.Root() || roots[1] == b.Root())
					require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(b.Root()))
				}
				cachedRoot, rootErr := f.s.HeadRoot(f.ctx)
				require.NoError(t, rootErr)
				cached := bytesutil.ToBytes32(cachedRoot)
				persisted, rootErr := f.s.cfg.BeaconDB.HeadBlockRoot()
				require.NoError(t, rootErr)
				require.Equal(t, false, cached == f.blks[2].Root(), "must not publish the rejected block as the service head")
				require.Equal(t, false, persisted == f.blks[2].Root(), "must not persist the rejected block as the database head")
				if failDelete {
					require.Equal(t, true, errors.Is(err, deleteErr), "the cleanup error must reach the caller")
				} else {
					require.Equal(t, f.blks[0].Root(), cached)
					require.Equal(t, cached, persisted)
				}
				synctest.Wait()
				for len(events) > 0 {
					event := <-events
					require.Equal(t, false, event.Type == statefeed.BlockProcessed, "cleanup failures must not announce the rejected block as processed")
				}
			})
		})
	}
}

func TestService_ReceiveBlock_DeferredHeadExtension(t *testing.T) {
	for _, deferHead := range []bool{false, true} {
		t.Run(map[bool]string{false: "publish each head", true: "defer late head"}[deferHead], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 4)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				events := make(chan *feed.Event, 16)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				// A late slot-3 block may have its head update deferred while
				// deciding whether the local slot-4 proposer can orphan it.
				driftGenesisTime(f.s, 3, -30)
				if deferHead {
					f.s.cfg.ProposerSlotIndexCache.SetProposerAndPayloadIDs(4, 0, [8]byte{}, [32]byte{})
				}
				require.NoError(t, f.s.ReceiveBlock(f.ctx, f.blks[2], f.blks[2].Root()))
				wantHead := f.blks[2].Root()
				if deferHead {
					wantHead = f.blks[1].Root()
				}
				cachedRoot, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, wantHead, bytesutil.ToBytes32(cachedRoot))
				synctest.Wait()
				driftGenesisTime(f.s, 4, 0)
				require.NoError(t, f.s.ReceiveBlock(f.ctx, f.blks[3], f.blks[3].Root()))
				cachedRoot, err = f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				require.Equal(t, f.blks[3].Root(), bytesutil.ToBytes32(cachedRoot))
				common, _, err := f.s.cfg.ForkChoiceStore.CommonAncestor(f.ctx, f.blks[1].Root(), f.blks[3].Root())
				require.NoError(t, err)
				require.Equal(t, f.blks[1].Root(), common, "the old head remains on the new head's branch")
				synctest.Wait()
				for len(events) > 0 {
					event := <-events
					require.Equal(t, false, event.Type == statefeed.Reorg, "a linear head advance must not emit a reorg event")
				}
			})
		})
	}
}

func TestService_ReceiveBlock_ProcessedEvent(t *testing.T) {
	for _, tt := range []struct {
		name          string
		newPayloadErr error
		forkchoiceErr error
		invalid       bool
		optimistic    bool
	}{
		{name: "validated"},
		{
			name:          "optimistic",
			newPayloadErr: execution.ErrAcceptedSyncingPayloadStatus,
			forkchoiceErr: execution.ErrAcceptedSyncingPayloadStatus,
			optimistic:    true,
		},
		{
			name:          "rejected by NewPayload",
			newPayloadErr: execution.ErrInvalidPayloadStatus,
			invalid:       true,
		},
		{
			name:          "rejected by ForkchoiceUpdated",
			newPayloadErr: execution.ErrAcceptedSyncingPayloadStatus,
			forkchoiceErr: execution.ErrInvalidPayloadStatus,
			invalid:       true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 3)
			payload, err := f.blks[0].Block().Body().Execution()
			require.NoError(t, err)
			f.engine.ErrNewPayload = tt.newPayloadErr
			f.engine.ErrForkchoiceUpdated = tt.forkchoiceErr
			f.engine.NewPayloadResp = payload.BlockHash()
			f.engine.ForkChoiceUpdatedResp = payload.BlockHash()
			f.engine.OverrideValidHash = bytesutil.ToBytes32(payload.BlockHash())
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 20, 0)
				events := make(chan *feed.Event, 16)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				err := f.s.ReceiveBlock(f.ctx, f.blks[2], f.blks[2].Root())
				if tt.invalid {
					require.Equal(t, true, IsInvalidBlock(err))
					require.Equal(t, false, f.s.cfg.ForkChoiceStore.HasNode(f.blks[2].Root()))
					require.Equal(t, false, f.s.HasBlock(f.ctx, f.blks[2].Root()))
				} else {
					require.NoError(t, err)
				}
				synctest.Wait()
				processed := 0
				for len(events) > 0 {
					event := <-events
					if event.Type != statefeed.BlockProcessed {
						continue
					}
					processed++
					data := event.Data.(*statefeed.BlockProcessedData)
					require.Equal(t, false, tt.invalid, "execution-invalid blocks must not be announced as processed")
					require.Equal(t, f.blks[2].Root(), data.BlockRoot)
					require.Equal(t, true, data.Verified)
					require.Equal(t, tt.optimistic, data.Optimistic)
				}
				wantEvents := 1
				if tt.invalid {
					wantEvents = 0
				}
				require.Equal(t, wantEvents, processed)
			})
		})
	}
}

func TestService_HasBlock(t *testing.T) {
	s, _ := minimalTestService(t)
	r := [32]byte{'a'}
	if s.HasBlock(context.Background(), r) {
		t.Error("Should not have block")
	}
	wsb, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
	require.NoError(t, err)
	require.NoError(t, s.saveInitSyncBlock(context.Background(), r, wsb))
	if !s.HasBlock(context.Background(), r) {
		t.Error("Should have block")
	}
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	util.SaveBlock(t, context.Background(), s.cfg.BeaconDB, b)
	r, err = b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, false, s.HasBlock(context.Background(), r), "a stored recent block still needs importing")
	require.NoError(t, s.blockBeingSynced.set(r))
	require.Equal(t, false, s.HasBlock(context.Background(), r))
}

func TestCheckSaveHotStateDB_Enabling(t *testing.T) {
	hook := logTest.NewGlobal()
	s, _ := minimalTestService(t)
	st := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epochsSinceFinalitySaveHotStateDB))
	s.genesisTime = time.Now().Add(time.Duration(-1*int64(st)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)

	require.NoError(t, s.checkSaveHotStateDB(context.Background()))
	assert.LogsContain(t, hook, "Entering mode to save hot states in DB")
}

func TestCheckSaveHotStateDB_Disabling(t *testing.T) {
	hook := logTest.NewGlobal()

	s, _ := minimalTestService(t)

	st := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(epochsSinceFinalitySaveHotStateDB))
	s.genesisTime = time.Now().Add(time.Duration(-1*int64(st)*int64(params.BeaconConfig().SecondsPerSlot)) * time.Second)
	require.NoError(t, s.checkSaveHotStateDB(context.Background()))
	s.genesisTime = time.Now()

	require.NoError(t, s.checkSaveHotStateDB(context.Background()))
	assert.LogsContain(t, hook, "Exiting mode to save hot states in DB")
}

func TestCheckSaveHotStateDB_Overflow(t *testing.T) {
	hook := logTest.NewGlobal()
	s, _ := minimalTestService(t)
	s.genesisTime = time.Now()

	require.NoError(t, s.checkSaveHotStateDB(context.Background()))
	assert.LogsDoNotContain(t, hook, "Entering mode to save hot states in DB")
}

// Regression test (upstream prysm #13842): the finalized_checkpoint event must
// carry the state root of the *finalized* block, not the state root of the
// block whose processing triggered finalization.
func Test_sendNewFinalizedEvent(t *testing.T) {
	s, _ := minimalTestService(t)
	notifier := &blockchainTesting.MockStateNotifier{RecordEvents: true}
	s.cfg.StateNotifier = notifier

	// The finalized block, stored in the DB, whose state root the event must report.
	finalizedSt, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	finalizedStRoot, err := finalizedSt.HashTreeRoot(s.ctx)
	require.NoError(t, err)
	b := util.NewBeaconBlockZond()
	b.Block.StateRoot = finalizedStRoot[:]
	sbb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	sbbRoot, err := sbb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, s.cfg.BeaconDB.SaveBlock(s.ctx, sbb))

	// The post state of the block that triggered finalization. It points at the
	// finalized block via its finalized checkpoint but has a different state root.
	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch*200))
	require.NoError(t, st.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{
		Epoch: 123,
		Root:  sbbRoot[:],
	}))
	triggeringStRoot, err := st.HashTreeRoot(s.ctx)
	require.NoError(t, err)
	require.NotEqual(t, finalizedStRoot, triggeringStRoot, "test setup: state roots must differ")

	checkpoint := st.FinalizedCheckpoint()
	s.sendNewFinalizedEvent(s.ctx, &forkchoicetypes.Checkpoint{Epoch: checkpoint.Epoch, Root: bytesutil.ToBytes32(checkpoint.Root)}, false)

	require.Eventually(t, func() bool {
		return len(notifier.ReceivedEvents()) == 1
	}, 5*time.Second, 10*time.Millisecond, "Expected exactly 1 state notification")
	e := notifier.ReceivedEvents()[0]
	assert.Equal(t, statefeed.FinalizedCheckpoint, int(e.Type))
	fc, ok := e.Data.(*qrlpb.EventFinalizedCheckpoint)
	require.Equal(t, true, ok, "event has wrong data type")
	assert.Equal(t, primitives.Epoch(123), fc.Epoch)
	assert.DeepEqual(t, sbbRoot[:], fc.Block)
	assert.DeepEqual(t, finalizedStRoot[:], fc.State)
	assert.Equal(t, false, fc.ExecutionOptimistic)
}

// If the finalized block cannot be found, no event is emitted (rather than
// emitting one with a bogus state root).
func Test_sendNewFinalizedEvent_UnknownFinalizedBlock(t *testing.T) {
	hook := logTest.NewGlobal()
	s, _ := minimalTestService(t)
	notifier := &blockchainTesting.MockStateNotifier{RecordEvents: true}
	s.cfg.StateNotifier = notifier

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{
		Epoch: 123,
		Root:  bytesutil.PadTo([]byte("missing"), 32),
	}))

	checkpoint := st.FinalizedCheckpoint()
	s.sendNewFinalizedEvent(s.ctx, &forkchoicetypes.Checkpoint{Epoch: checkpoint.Epoch, Root: bytesutil.ToBytes32(checkpoint.Root)}, false)

	require.Equal(t, 0, len(notifier.ReceivedEvents()))
	assert.LogsContain(t, hook, "Could not retrieve block for finalized checkpoint root")
}

func setupEpochTransitionTest(t *testing.T) {
	t.Helper()
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.SlotsPerEpoch = 6
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)
	transition.SkipSlotCache.Disable()
	t.Cleanup(transition.SkipSlotCache.Enable)
}

func TestService_NewSlot_GenesisCheckpoint(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, normalize := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal genesis alias", true: "normalized root control"}[normalize], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 17)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 17, 0)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				fc := f.s.cfg.ForkChoiceStore
				require.Equal(t, primitives.Epoch(0), fc.FinalizedCheckpoint().Epoch)
				require.Equal(t, [32]byte{}, fc.FinalizedCheckpoint().Root)
				require.Equal(t, primitives.Epoch(0), fc.JustifiedCheckpoint().Epoch)
				dump, err := fc.ForkChoiceDump(f.ctx)
				require.NoError(t, err)
				require.Equal(t, primitives.Epoch(2), dump.UnrealizedJustifiedCheckpoint.Epoch)
				if normalize {
					require.NoError(t, fc.UpdateFinalizedCheckpoint(&forkchoicetypes.Checkpoint{Root: f.s.originBlockRoot}))
				}
				driftGenesisTime(f.s, 18, 0)
				err = f.s.NewSlot(f.ctx, 18)
				assert.NoError(t, err)
				assert.Equal(t, primitives.Epoch(2), fc.JustifiedCheckpoint().Epoch)
			})
		})
	}
}

func TestService_NewSlot_BlockArrivalOrder(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tickFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "tick before block", false: "block before tick"}[tickFirst], func(t *testing.T) {
			helpers.ClearCache()
			engine := &mockExecution.EngineClient{}
			s, tr := minimalTestService(t, WithExecutionEngineCaller(engine))
			genesis, keys := util.DeterministicGenesisStateZond(t, 64)
			require.NoError(t, genesis.SetGenesisTime(uint64(time.Now().Unix())-40*params.BeaconConfig().SecondsPerSlot))
			require.NoError(t, s.saveGenesisData(tr.ctx, genesis))
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				for slot := primitives.Slot(1); slot <= 30; slot++ {
					synctest.Wait()
					driftGenesisTime(s, int64(slot), 0)
					if slot == 30 && tickFirst {
						require.NoError(t, s.NewSlot(tr.ctx, slot))
					}
					parent, err := s.HeadState(tr.ctx)
					require.NoError(t, err)
					cfg := util.DefaultBlockGenConfig()
					// Withhold two committees in epoch 4. The attestation for
					// slot 29 carried by block 30 then completes its target quorum.
					if slot == 28 || slot == 29 {
						cfg.NumAttestations = 0
					}
					pb, err := util.GenerateFullBlockZond(parent, keys, cfg, slot)
					require.NoError(t, err)
					block, err := blocks.NewSignedBeaconBlock(pb)
					require.NoError(t, err)
					ro, err := blocks.NewROBlock(block)
					require.NoError(t, err)
					require.NoError(t, s.ReceiveBlock(tr.ctx, ro, ro.Root()))
				}
				synctest.Wait()
				st, err := s.HeadState(tr.ctx)
				require.NoError(t, err)
				require.Equal(t, primitives.Epoch(3), st.CurrentJustifiedCheckpoint().Epoch)
				require.Equal(t, primitives.Epoch(2), st.FinalizedCheckpoint().Epoch)
				dump, err := s.cfg.ForkChoiceStore.ForkChoiceDump(tr.ctx)
				require.NoError(t, err)
				require.Equal(t, primitives.Epoch(4), dump.UnrealizedJustifiedCheckpoint.Epoch)
				if !tickFirst {
					require.NoError(t, s.NewSlot(tr.ctx, 30))
				}
				dump, err = s.cfg.ForkChoiceStore.ForkChoiceDump(tr.ctx)
				require.NoError(t, err)
				assert.Equal(t, primitives.Epoch(3), dump.JustifiedCheckpoint.Epoch)
				assert.Equal(t, primitives.Epoch(2), dump.FinalizedCheckpoint.Epoch)
				require.Equal(t, 19, len(dump.ForkChoiceNodes), "retain blocks from finalized slot 12 through slot 30")
				wantHead, err := s.HeadRoot(tr.ctx)
				require.NoError(t, err)
				// Delayed and repeated ticks must not realize block 30's
				// observations or prune another epoch of ancestors.
				for _, tick := range []primitives.Slot{24, 30, 30} {
					require.NoError(t, s.NewSlot(tr.ctx, tick))
					repeated, err := s.cfg.ForkChoiceStore.ForkChoiceDump(tr.ctx)
					require.NoError(t, err)
					require.DeepEqual(t, dump, repeated)
				}
				head, err := s.cfg.ForkChoiceStore.Head(tr.ctx)
				require.NoError(t, err)
				require.Equal(t, bytesutil.ToBytes32(wantHead), head)
				// The same observations become eligible at the next epoch.
				driftGenesisTime(s, 36, 0)
				require.NoError(t, s.NewSlot(tr.ctx, 36))
				require.Equal(t, primitives.Epoch(4), s.cfg.ForkChoiceStore.JustifiedCheckpoint().Epoch)
				require.Equal(t, primitives.Epoch(3), s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				require.Equal(t, 13, s.cfg.ForkChoiceStore.NodeCount())
				head, err = s.cfg.ForkChoiceStore.Head(tr.ctx)
				require.NoError(t, err)
				require.Equal(t, bytesutil.ToBytes32(wantHead), head)
			})
		})
	}
}

func TestService_ReceiveBlock_FinalizedEventExecutionStatus(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tt := range []struct {
		name           string
		finalizedValid bool
		headValid      bool
	}{
		{name: "validated finalized block and optimistic head", finalizedValid: true},
		{name: "both optimistic"},
		{name: "both validated", finalizedValid: true, headValid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 24)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 12, 0)
				if tt.finalizedValid {
					f.engine.ErrNewPayload = nil
					f.engine.ErrForkchoiceUpdated = nil
				}
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:12]))
				if !tt.headValid {
					f.engine.ErrNewPayload = execution.ErrAcceptedSyncingPayloadStatus
					f.engine.ErrForkchoiceUpdated = execution.ErrAcceptedSyncingPayloadStatus
				}
				driftGenesisTime(f.s, 23, 0)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[12:23]))
				require.Equal(t, primitives.Epoch(0), f.s.cfg.ForkChoiceStore.FinalizedCheckpoint().Epoch)
				events := make(chan *feed.Event, 16)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				driftGenesisTime(f.s, 24, 0)
				require.NoError(t, f.s.ReceiveBlock(f.ctx, f.blks[23], f.blks[23].Root()))
				synctest.Wait()
				count := 0
				for len(events) > 0 {
					ev := <-events
					if ev.Type != statefeed.FinalizedCheckpoint {
						continue
					}
					count++
					data := ev.Data.(*qrlpb.EventFinalizedCheckpoint)
					require.Equal(t, primitives.Epoch(2), data.Epoch)
					require.Equal(t, f.blks[11].Root(), bytesutil.ToBytes32(data.Block))
					actual, err := f.s.cfg.ForkChoiceStore.IsOptimistic(bytesutil.ToBytes32(data.Block))
					require.NoError(t, err)
					require.Equal(t, !tt.finalizedValid, actual)
					assert.Equal(t, actual, data.ExecutionOptimistic)
				}
				require.Equal(t, 1, count, "missing or duplicate finalized event")
			})
		})
	}
}

func TestService_ReceiveBlockBatch_CheckpointPersistence(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, split := range []bool{false, true} {
		t.Run(map[bool]string{false: "transitions inside batch", true: "transitions start batches"}[split], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 24)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 24, 0)
				if split {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:17]))
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[17:23]))
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[23:]))
				} else {
					require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				}
				synctest.Wait()
				fc := f.s.cfg.ForkChoiceStore
				require.Equal(t, primitives.Epoch(3), fc.JustifiedCheckpoint().Epoch)
				require.Equal(t, primitives.Epoch(2), fc.FinalizedCheckpoint().Epoch)
				jc, err := f.s.cfg.BeaconDB.JustifiedCheckpoint(f.ctx)
				require.NoError(t, err)
				cp, err := f.s.cfg.BeaconDB.FinalizedCheckpoint(f.ctx)
				require.NoError(t, err)

				assert.Equal(t, fc.JustifiedCheckpoint().Epoch, jc.Epoch)
				assert.Equal(t, fc.JustifiedCheckpoint().Root, bytesutil.ToBytes32(jc.Root))
				assert.Equal(t, fc.FinalizedCheckpoint().Epoch, cp.Epoch)
				assert.Equal(t, fc.FinalizedCheckpoint().Root, bytesutil.ToBytes32(cp.Root))
				validated, err := f.s.cfg.BeaconDB.LastValidatedCheckpoint(f.ctx)
				require.NoError(t, err)
				assert.Equal(t, cp.Epoch, validated.Epoch)
				assert.DeepEqual(t, cp.Root, validated.Root)
			})
		})
	}
}

func TestService_ReceiveBlockBatch_FinalizedExecutionStatusRestart(t *testing.T) {
	setupEpochTransitionTest(t)
	reset := features.InitWithReset(&features.Flags{})
	t.Cleanup(reset)
	for _, tt := range []struct {
		name           string
		seedValidated  bool
		validPayload   bool
		validHead      bool
		wantOptimistic bool
	}{
		{name: "optimistic without validation marker", wantOptimistic: true},
		{name: "optimistic with validated genesis", seedValidated: true, wantOptimistic: true},
		{name: "validated by new payload", validPayload: true},
		{name: "validated by forkchoice update", validHead: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newBatchExecutionFixture(t, 24)
			if tt.seedValidated {
				require.NoError(t, f.s.cfg.BeaconDB.SaveLastValidatedCheckpoint(f.ctx, &qrysmpb.Checkpoint{Root: f.s.originBlockRoot[:]}))
			}
			if tt.validPayload {
				f.engine.ErrNewPayload = nil
			}
			if tt.validHead {
				f.engine.ErrForkchoiceUpdated = nil
			}
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 24, 0)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:]))
				synctest.Wait()
				finalized := *f.s.cfg.ForkChoiceStore.FinalizedCheckpoint()
				require.Equal(t, primitives.Epoch(2), finalized.Epoch)
				optimistic, err := f.s.cfg.ForkChoiceStore.IsOptimistic(finalized.Root)
				require.NoError(t, err)
				require.Equal(t, tt.wantOptimistic, optimistic)
				validated, err := f.s.cfg.BeaconDB.LastValidatedCheckpoint(f.ctx)
				require.NoError(t, err)
				if tt.wantOptimistic {
					assert.Equal(t, primitives.Epoch(0), validated.Epoch)
					assert.Equal(t, f.s.originBlockRoot, bytesutil.ToBytes32(validated.Root))
				} else {
					assert.Equal(t, finalized.Epoch, validated.Epoch)
					assert.Equal(t, finalized.Root, bytesutil.ToBytes32(validated.Root))
				}
				// Slot 11 is pruned from forkchoice but finalized in the DB.
				historical, err := f.s.IsOptimisticForRoot(f.ctx, f.blks[10].Root())
				require.NoError(t, err)
				assert.Equal(t, tt.wantOptimistic, historical)
				// Rebuild the same forkchoice root from the DB, using the normal
				// startup path without receiving any further engine responses.
				f.s.head = nil
				f.s.cfg.ForkChoiceStore = doublylinkedtree.New()
				f.s.cfg.StateGen = stategen.New(f.s.cfg.BeaconDB, f.s.cfg.ForkChoiceStore)
				f.s.cfg.ForkChoiceStore.SetBalancesByRooter(f.s.cfg.StateGen.BalancesByCheckpoint)
				require.NoError(t, f.s.setupForkchoice(f.states[12].Copy()))
				optimistic, err = f.s.cfg.ForkChoiceStore.IsOptimistic(finalized.Root)
				require.NoError(t, err)
				assert.Equal(t, tt.wantOptimistic, optimistic, "restart must preserve the finalized root's execution status")
			})
		})
	}
}

func TestService_ReceiveBlock_EpochTickOrder(t *testing.T) {
	setupEpochTransitionTest(t)
	for _, tickFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "tick before block", false: "block before tick"}[tickFirst], func(t *testing.T) {
			f := newBatchExecutionFixture(t, 17)
			f.engine.ErrNewPayload, f.engine.ErrForkchoiceUpdated = nil, nil
			// Create an independently signed sibling with the same participation
			// and checkpoints. Only its graffiti and derived roots differ.
			pb, err := util.GenerateFullBlockZond(f.states[16].Copy(), f.keys, util.DefaultBlockGenConfig(), 17)
			require.NoError(t, err)
			pb.Block.Body.Graffiti[0] = 0xab
			sig, err := util.BlockSignature(f.states[16].Copy(), pb.Block, f.keys)
			require.NoError(t, err)
			pb.Signature = sig.Marshal()
			signed, err := blocks.NewSignedBeaconBlock(pb)
			require.NoError(t, err)
			sibling, err := blocks.NewROBlock(signed)
			require.NoError(t, err)
			siblingState, err := transition.ExecuteStateTransition(f.ctx, f.states[16].Copy(), sibling)
			require.NoError(t, err)
			heavy, light := f.blks[16], sibling
			heavyState, lightState := f.states[17], siblingState
			h, l := heavy.Root(), light.Root()
			if bytes.Compare(h[:], l[:]) < 0 {
				heavy, light = light, heavy
				heavyState, lightState = lightState, heavyState
			}
			nextPB, err := util.GenerateFullBlockZond(lightState.Copy(), f.keys, &util.BlockGenConfig{}, 18)
			require.NoError(t, err)
			signed, err = blocks.NewSignedBeaconBlock(nextPB)
			require.NoError(t, err)
			next, err := blocks.NewROBlock(signed)
			require.NoError(t, err)
			synctest.Test(t, func(t *testing.T) {
				t.Cleanup(synctest.Wait)
				driftGenesisTime(f.s, 17, -30)
				require.NoError(t, f.s.ReceiveBlockBatch(f.ctx, f.blks[2:16]))
				require.NoError(t, f.s.ReceiveBlock(f.ctx, heavy, heavy.Root()))
				require.NoError(t, f.s.ReceiveBlock(f.ctx, light, light.Root()))
				synctest.Wait()
				require.Equal(t, heavy.Root(), f.s.CachedHeadRoot())
				driftGenesisTime(f.s, 18, -30)
				if tickFirst {
					require.NoError(t, f.s.NewSlot(f.ctx, 18))
				}
				committee, err := helpers.BeaconCommitteeFromState(f.ctx, heavyState, 17, 0)
				require.NoError(t, err)
				indices := make([]uint64, len(committee))
				for i, index := range committee {
					indices[i] = uint64(index)
				}
				fc := f.s.cfg.ForkChoiceStore
				fc.ProcessAttestation(f.ctx, indices, heavy.Root(), 2)
				events := make(chan *feed.Event, 16)
				sub := f.s.cfg.StateNotifier.StateFeed().Subscribe(events)
				defer sub.Unsubscribe()
				require.NoError(t, f.s.ReceiveBlock(f.ctx, next, next.Root()))
				synctest.Wait()
				weight, err := fc.Weight(heavy.Root())
				require.NoError(t, err)
				nextWeight, err := fc.Weight(next.Root())
				require.NoError(t, err)
				require.Equal(t, true, weight > nextWeight)
				got, err := f.s.HeadRoot(f.ctx)
				require.NoError(t, err)
				count := 0
				for len(events) > 0 {
					if ev := <-events; ev.Type == statefeed.Reorg {
						count++
					}
				}

				assert.Equal(t, heavy.Root(), bytesutil.ToBytes32(got))
				assert.Equal(t, 0, count)
				if !tickFirst {
					require.NoError(t, f.s.NewSlot(f.ctx, 18))
					f.s.UpdateHead(f.ctx, 18)
					synctest.Wait()
					got, err = f.s.HeadRoot(f.ctx)
					require.NoError(t, err)
					require.Equal(t, heavy.Root(), bytesutil.ToBytes32(got))
				}
			})
		})
	}
}
