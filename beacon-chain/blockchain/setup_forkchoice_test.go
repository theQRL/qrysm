package blockchain

import (
	"context"
	"errors"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func Test_setupForkchoiceCheckpoints_BalanceFailure(t *testing.T) {
	service, tr := minimalTestService(t)
	st, _ := util.DeterministicGenesisStateZond(t, 64)
	require.NoError(t, service.saveGenesisData(tr.ctx, st))
	root := service.originBlockRoot
	require.NoError(t, service.cfg.BeaconDB.SaveJustifiedCheckpoint(tr.ctx, &qrysmpb.Checkpoint{Epoch: 1, Root: root[:]}))
	previous := service.cfg.ForkChoiceStore.JustifiedCheckpoint()
	attempts := 0
	service.cfg.ForkChoiceStore.SetBalancesByRooter(func(context.Context, *forkchoicetypes.Checkpoint) (*forkchoicetypes.JustifiedBalances, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("temporary checkpoint-state read failure")
		}
		return &forkchoicetypes.JustifiedBalances{Balances: []uint64{32}, TotalActiveBalance: 32}, nil
	})
	require.ErrorContains(t, "could not initialize justified checkpoint and balances", service.setupForkchoiceCheckpoints())
	require.DeepEqual(t, previous, service.cfg.ForkChoiceStore.JustifiedCheckpoint())

	require.NoError(t, service.setupForkchoiceCheckpoints())
	require.Equal(t, 2, attempts)
	require.DeepEqual(t, &forkchoicetypes.Checkpoint{Epoch: 1, Root: root}, service.cfg.ForkChoiceStore.JustifiedCheckpoint())
}

func Test_startupHeadRoot(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	hook := logTest.NewGlobal()
	cp := service.FinalizedCheckpt()
	require.DeepEqual(t, cp.Root, params.BeaconConfig().ZeroHash[:])
	gr := [32]byte{'r', 'o', 'o', 't'}
	service.originBlockRoot = gr
	require.NoError(t, service.cfg.BeaconDB.SaveGenesisBlockRoot(ctx, gr))
	t.Run("start from finalized", func(t *testing.T) {
		require.Equal(t, service.startupHeadRoot(), gr)
	})
	t.Run("head requested, error path", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{
			ForceHead: "head",
		})
		defer resetCfg()
		require.Equal(t, service.startupHeadRoot(), gr)
		require.LogsContain(t, hook, "Could not get head block root, starting with justified block as head")
	})

	st, _ := util.DeterministicGenesisStateZond(t, 64)
	hr := [32]byte{'h', 'e', 'a', 'd'}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st, hr), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, hr), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, hr))

	t.Run("start from head", func(t *testing.T) {
		resetCfg := features.InitWithReset(&features.Flags{
			ForceHead: "head",
		})
		defer resetCfg()
		require.Equal(t, service.startupHeadRoot(), hr)
	})
}

func Test_setupForkchoiceTree_Finalized(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	st, _ := util.DeterministicGenesisStateZond(t, 64)
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")

	require.NoError(t, service.saveGenesisData(ctx, st))

	genesis := blocks.NewGenesisBlock(stateRoot[:])
	wsb, err := consensusblocks.NewSignedBeaconBlock(genesis)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb), "Could not save genesis block")
	parentRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st, parentRoot), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, parentRoot), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: parentRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: parentRoot[:]}))
	require.NoError(t, service.setupForkchoiceTree(st))
	require.Equal(t, 1, service.cfg.ForkChoiceStore.NodeCount())
}

func Test_setupForkchoiceTree_Head(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	resetCfg := features.InitWithReset(&features.Flags{
		ForceHead: "head",
	})
	defer resetCfg()

	genesisState, keys := util.DeterministicGenesisStateZond(t, 64)
	stateRoot, err := genesisState.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")
	genesis := blocks.NewGenesisBlock(stateRoot[:])
	wsb, err := consensusblocks.NewSignedBeaconBlock(genesis)
	require.NoError(t, err)
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb), "Could not save genesis block")
	require.NoError(t, service.saveGenesisData(ctx, genesisState))

	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, genesisState, genesisRoot), "Could not save genesis state")
	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, genesisRoot), "Could not save genesis state")

	st, err := service.HeadState(ctx)
	require.NoError(t, err)
	b, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(1))
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	preState, err := service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err := service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, root, wsb, postState))

	b, err = util.GenerateFullBlockZond(postState, keys, util.DefaultBlockGenConfig(), primitives.Slot(2))
	require.NoError(t, err)
	wsb, err = consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	root, err = b.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, root, wsb, preState))

	require.NoError(t, service.cfg.BeaconDB.SaveHeadBlockRoot(ctx, root))
	cp := service.FinalizedCheckpt()
	fRoot := service.ensureRootNotZeros([32]byte(cp.Root))
	require.NotEqual(t, fRoot, root)
	require.Equal(t, root, service.startupHeadRoot())
	require.NoError(t, service.setupForkchoiceTree(st))
	require.Equal(t, 3, service.cfg.ForkChoiceStore.NodeCount())
}

func Test_setupForkchoice_SelectedHead(t *testing.T) {
	for _, forceHead := range []string{"", "head"} {
		t.Run("sync-from="+forceHead, func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{ForceHead: forceHead})
			defer reset()
			s, tr := minimalTestService(t)
			ctx := tr.ctx
			genesis, keys := util.DeterministicGenesisStateZond(t, 64)
			s.genesisTime = time.Unix(int64(genesis.GenesisTime()), 0)
			require.NoError(t, s.saveGenesisData(ctx, genesis))
			b, err := util.GenerateFullBlockZond(genesis, keys, util.DefaultBlockGenConfig(), 1)
			require.NoError(t, err)
			block, err := consensusblocks.NewSignedBeaconBlock(b)
			require.NoError(t, err)
			root, err := block.Block().HashTreeRoot()
			require.NoError(t, err)
			post, err := s.validateStateTransition(ctx, genesis.Copy(), block)
			require.NoError(t, err)
			require.NoError(t, s.savePostStateInfo(ctx, root, block, post))
			require.NoError(t, tr.db.SaveState(ctx, post, root))
			require.NoError(t, tr.db.SaveHeadBlockRoot(ctx, root))
			if forceHead == "" {
				require.NoError(t, tr.db.SaveJustifiedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 1, Root: root[:]}))
			}

			// Rebuild both caches from the database, as on restart.
			s.head = nil
			s.cfg.ForkChoiceStore = doublylinkedtree.New()
			s.cfg.StateGen = stategen.New(tr.db, s.cfg.ForkChoiceStore)
			s.cfg.ForkChoiceStore.SetBalancesByRooter(s.cfg.StateGen.BalancesByCheckpoint)
			require.NoError(t, s.setupForkchoice(genesis))
			serviceRoot, err := s.HeadRoot(ctx)
			require.NoError(t, err)
			require.DeepEqual(t, root[:], serviceRoot)
			require.Equal(t, root, s.CachedHeadRoot())
			canonical, err := s.IsCanonical(ctx, root)
			require.NoError(t, err)
			require.Equal(t, true, canonical)
			if forceHead == "" {
				require.DeepEqual(t, &forkchoicetypes.Checkpoint{Epoch: 1, Root: root}, s.cfg.ForkChoiceStore.JustifiedCheckpoint())
			}
		})
	}
}

func Test_setupForkchoice_OptimisticHead(t *testing.T) {
	for _, tt := range []struct {
		name            string
		forceHead       string
		startOptimistic bool
		wantOptimistic  bool
	}{
		{name: "recent saved head", forceHead: "head", wantOptimistic: true},
		{name: "recent saved head with startup optimism", forceHead: "head", startOptimistic: true, wantOptimistic: true},
		{name: "validated finalized anchor"},
		{name: "optimistic finalized anchor", startOptimistic: true, wantOptimistic: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reset := features.InitWithReset(&features.Flags{ForceHead: tt.forceHead, EnableStartOptimistic: tt.startOptimistic})
			defer reset()
			s, tr := minimalTestService(t)
			ctx := tr.ctx
			genesis, keys := util.DeterministicGenesisStateZond(t, 64)
			genesisTime := uint64(time.Now().Unix()) - 2*params.BeaconConfig().SecondsPerSlot
			require.NoError(t, genesis.SetGenesisTime(genesisTime))
			s.genesisTime = time.Unix(int64(genesisTime), 0)
			require.NoError(t, s.saveGenesisData(ctx, genesis))
			wantRoot := s.originBlockRoot
			require.NoError(t, tr.db.SaveLastValidatedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: wantRoot[:]}))
			if tt.forceHead == "head" {
				b, err := util.GenerateFullBlockZond(genesis.Copy(), keys, util.DefaultBlockGenConfig(), 1)
				require.NoError(t, err)
				block, err := consensusblocks.NewSignedBeaconBlock(b)
				require.NoError(t, err)
				wantRoot, err = block.Block().HashTreeRoot()
				require.NoError(t, err)
				post, err := s.validateStateTransition(ctx, genesis.Copy(), block)
				require.NoError(t, err)
				require.NoError(t, s.savePostStateInfo(ctx, wantRoot, block, post))
				require.NoError(t, tr.db.SaveState(ctx, post, wantRoot))
				require.NoError(t, tr.db.SaveHeadBlockRoot(ctx, wantRoot))
			}

			s.head = nil
			s.cfg.ForkChoiceStore = doublylinkedtree.New()
			s.cfg.StateGen = stategen.New(tr.db, s.cfg.ForkChoiceStore)
			require.NoError(t, s.setupForkchoice(genesis))
			root, err := s.HeadRoot(ctx)
			require.NoError(t, err)
			require.DeepEqual(t, wantRoot[:], root)
			optimistic, err := s.cfg.ForkChoiceStore.IsOptimistic(wantRoot)
			require.NoError(t, err)
			require.Equal(t, tt.wantOptimistic, optimistic)
			// Exercise the recent-head path, which trusts the service cache.
			require.Equal(t, true, s.head.slot+2 >= s.CurrentSlot())
			optimistic, err = s.IsOptimistic(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.wantOptimistic, optimistic)
		})
	}
}

func Test_setupForkchoice_FallbackBalanceFailure(t *testing.T) {
	s, tr := minimalTestService(t)
	ctx := tr.ctx
	st, _ := util.DeterministicGenesisStateZond(t, 64)
	s.genesisTime = time.Unix(int64(st.GenesisTime()), 0)
	require.NoError(t, s.saveGenesisData(ctx, st))
	genesisRoot := s.originBlockRoot
	missing := [32]byte{'m', 'i', 's', 's', 'i', 'n', 'g'}
	require.NoError(t, tr.db.SaveState(ctx, st, missing))
	require.NoError(t, tr.db.SaveJustifiedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 1, Root: missing[:]}))

	s.head = nil
	s.cfg.ForkChoiceStore = doublylinkedtree.New()
	failRead := true
	s.cfg.ForkChoiceStore.SetBalancesByRooter(func(_ context.Context, cp *forkchoicetypes.Checkpoint) (*forkchoicetypes.JustifiedBalances, error) {
		if cp.Root == missing {
			return &forkchoicetypes.JustifiedBalances{Balances: []uint64{64}, TotalActiveBalance: 64}, nil
		}
		require.DeepEqual(t, &forkchoicetypes.Checkpoint{Root: genesisRoot}, cp)
		if failRead {
			return nil, errors.New("temporary finalized balance read failure")
		}
		return &forkchoicetypes.JustifiedBalances{Balances: []uint64{32}, TotalActiveBalance: 32}, nil
	})
	require.ErrorContains(t, "could not reset justified checkpoint to finalized checkpoint", s.setupForkchoice(st))
	require.Equal(t, true, s.head == nil)
	require.DeepEqual(t, &forkchoicetypes.Checkpoint{Epoch: 1, Root: missing}, s.cfg.ForkChoiceStore.JustifiedCheckpoint())

	failRead = false
	require.NoError(t, s.setupForkchoice(st))
	require.DeepEqual(t, s.cfg.ForkChoiceStore.FinalizedCheckpoint(), s.cfg.ForkChoiceStore.JustifiedCheckpoint())
	require.Equal(t, genesisRoot, s.CachedHeadRoot())
	// Votes must use the finalized checkpoint's balances after the fallback.
	s.cfg.ForkChoiceStore.ProcessAttestation(ctx, []uint64{0}, genesisRoot, 1)
	root, err := s.cfg.ForkChoiceStore.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, genesisRoot, root)
	weight, err := s.cfg.ForkChoiceStore.Weight(root)
	require.NoError(t, err)
	require.Equal(t, uint64(32), weight)
}

// Regression test: the justified checkpoint in the DB references a root whose
// block is absent (BeaconDB.Block returns nil, nil for it). Startup must fall
// back to the finalized block as head instead of panicking on the nil block.
func Test_setupForkchoiceTree_MissingHeadBlock(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	hook := logTest.NewGlobal()

	st, _ := util.DeterministicGenesisStateZond(t, 64)
	stateRoot, err := st.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")
	require.NoError(t, service.saveGenesisData(ctx, st))

	genesis := blocks.NewGenesisBlock(stateRoot[:])
	wsb, err := consensusblocks.NewSignedBeaconBlock(genesis)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb), "Could not save genesis block")
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st, genesisRoot), "Could not save genesis state")

	// The justified checkpoint points to a root whose state exists but whose
	// block is not in the DB, e.g. left behind by an unclean stop.
	missingRoot := [32]byte{'m', 'i', 's', 's', 'i', 'n', 'g'}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, st, missingRoot), "Could not save state")
	require.NoError(t, service.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 1, Root: missingRoot[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: genesisRoot[:]}))
	require.NoError(t, service.setupForkchoiceCheckpoints())

	blk, err := service.cfg.BeaconDB.Block(ctx, missingRoot)
	require.NoError(t, err)
	require.Equal(t, true, blk == nil)
	require.Equal(t, missingRoot, service.startupHeadRoot())

	require.NoError(t, service.setupForkchoiceTree(st))
	require.LogsContain(t, hook, "starting with finalized block as head")
	require.Equal(t, 1, service.cfg.ForkChoiceStore.NodeCount())
	require.NoError(t, service.initializeHead(ctx, st))
	require.DeepEqual(t, service.cfg.ForkChoiceStore.FinalizedCheckpoint(), service.cfg.ForkChoiceStore.JustifiedCheckpoint())
	require.Equal(t, genesisRoot, service.CachedHeadRoot())
	require.Equal(t, true, service.cfg.ForkChoiceStore.IsCanonical(genesisRoot))
	require.NoError(t, service.NewSlot(ctx, params.BeaconConfig().SlotsPerEpoch))
	root, err := service.cfg.ForkChoiceStore.Head(ctx)
	require.NoError(t, err)
	require.Equal(t, genesisRoot, root)
}

// Regression test: the head block is in the DB but an ancestor between it and
// the finalized block is not. The chain walk must surface an error so startup
// falls back to the finalized block as head instead of panicking.
func Test_setupForkchoiceTree_MissingAncestor(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	hook := logTest.NewGlobal()

	genesisState, keys := util.DeterministicGenesisStateZond(t, 64)
	stateRoot, err := genesisState.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")
	genesis := blocks.NewGenesisBlock(stateRoot[:])
	wsb, err := consensusblocks.NewSignedBeaconBlock(genesis)
	require.NoError(t, err)
	genesisRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb), "Could not save genesis block")
	require.NoError(t, service.saveGenesisData(ctx, genesisState))
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, genesisState, genesisRoot), "Could not save genesis state")

	// Block 1 is processed to derive its post state but never saved to the DB.
	st, err := service.HeadState(ctx)
	require.NoError(t, err)
	b1, err := util.GenerateFullBlockZond(st, keys, util.DefaultBlockGenConfig(), primitives.Slot(1))
	require.NoError(t, err)
	wsb1, err := consensusblocks.NewSignedBeaconBlock(b1)
	require.NoError(t, err)
	preState, err := service.getBlockPreState(ctx, wsb1.Block())
	require.NoError(t, err)
	postState, err := service.validateStateTransition(ctx, preState, wsb1)
	require.NoError(t, err)

	// Block 2, child of the missing block 1, is the startup head.
	b2, err := util.GenerateFullBlockZond(postState, keys, util.DefaultBlockGenConfig(), primitives.Slot(2))
	require.NoError(t, err)
	wsb2, err := consensusblocks.NewSignedBeaconBlock(b2)
	require.NoError(t, err)
	root2, err := b2.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb2))
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, postState, root2))

	require.NoError(t, service.cfg.BeaconDB.SaveJustifiedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 1, Root: root2[:]}))
	require.NoError(t, service.cfg.BeaconDB.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Root: genesisRoot[:]}))
	require.NoError(t, service.setupForkchoiceCheckpoints())
	require.Equal(t, root2, service.startupHeadRoot())

	require.NoError(t, service.setupForkchoiceTree(genesisState))
	require.LogsContain(t, hook, "Could not build forkchoice chain, starting with finalized block as head")
	require.Equal(t, 1, service.cfg.ForkChoiceStore.NodeCount())
	require.NoError(t, service.initializeHead(ctx, genesisState))
	require.DeepEqual(t, service.cfg.ForkChoiceStore.FinalizedCheckpoint(), service.cfg.ForkChoiceStore.JustifiedCheckpoint())
	require.Equal(t, genesisRoot, service.CachedHeadRoot())
	require.Equal(t, true, service.cfg.ForkChoiceStore.IsCanonical(genesisRoot))
}
