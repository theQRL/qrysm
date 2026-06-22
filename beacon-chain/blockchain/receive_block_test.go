package blockchain

import (
	"context"
	"sync"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	blockchainTesting "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/operations/voluntaryexits"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
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
	require.Equal(t, true, s.HasBlock(context.Background(), r))
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
