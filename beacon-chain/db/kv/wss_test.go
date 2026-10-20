package kv

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/beacon-chain/state/genesis"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	bolt "go.etcd.io/bbolt"
)

// alignLatestBlockHeader records blk as the block that produced st, which is
// what SaveOrigin verifies.
func alignLatestBlockHeader(t *testing.T, st state.BeaconState, blk *qrysmpb.SignedBeaconBlockZond) {
	bodyRoot, err := blk.Block.Body.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, st.SetLatestBlockHeader(&qrysmpb.BeaconBlockHeader{
		Slot:          blk.Block.Slot,
		ProposerIndex: blk.Block.ProposerIndex,
		ParentRoot:    blk.Block.ParentRoot,
		StateRoot:     make([]byte, 32),
		BodyRoot:      bodyRoot[:],
	}))
	stateRoot, err := st.HashTreeRoot(context.Background())
	require.NoError(t, err)
	blk.Block.StateRoot = stateRoot[:]
}

func TestSaveOrigin(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	// Embedded Genesis works with Mainnet config
	params.OverrideBeaconConfig(params.MainnetConfig().Copy())

	ctx := context.Background()
	db := setupDB(t)

	st, err := genesis.State(params.MainnetName)
	require.NoError(t, err)

	sb, err := st.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, db.LoadGenesis(ctx, sb))

	// this is necessary for mainnet, because LoadGenesis is short-circuited by the embedded state,
	// so the genesis root key is never written to the db.
	require.NoError(t, db.EnsureEmbeddedGenesis(ctx))

	cst, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	// The checkpoint belongs to the chain of the embedded genesis.
	require.NoError(t, cst.SetGenesisValidatorsRoot(st.GenesisValidatorsRoot()))
	require.NoError(t, cst.SetGenesisTime(st.GenesisTime()))
	cb := util.NewBeaconBlockZond()
	alignLatestBlockHeader(t, cst, cb)
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, db.SaveOrigin(ctx, csb, cbb))

	broot, err := scb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, true, db.IsFinalizedBlock(ctx, broot))
	validated, err := db.LastValidatedCheckpoint(ctx)
	require.NoError(t, err)
	require.DeepEqual(t, broot[:], validated.Root)
}

func TestSaveOrigin_ActiveValidatorCapacity(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	if fieldparams.Preset == "minimal" {
		cfg := params.MinimalSpecConfig().Copy()
		cfg.ConfigName = params.MainnetTestName
		params.FillTestVersions(cfg, 128)
		require.NoError(t, params.SetActive(cfg))
	}
	cfg := params.BeaconConfig()
	capacity, err := cfg.MaxActiveValidators()
	require.NoError(t, err)
	ctx := context.Background()
	const currentEpoch = primitives.Epoch(10)
	for _, tc := range []struct {
		name            string
		active          uint64
		scheduled       uint64
		inactive        uint64
		activationEpoch primitives.Epoch
		exitEpoch       primitives.Epoch // Optional exit for the first active validator.
		wantEpoch       primitives.Epoch // Zero means the import should succeed.
	}{
		{name: "at capacity", active: capacity},
		{name: "above capacity", active: capacity + 1, wantEpoch: currentEpoch},
		{name: "inactive records beyond capacity", active: capacity, inactive: 2},
		{name: "exit at checkpoint epoch", active: capacity + 1, exitEpoch: currentEpoch},
		{name: "scheduled activations reach capacity", active: capacity - 1, scheduled: 1, activationEpoch: currentEpoch + 1},
		{name: "scheduled activations exceed capacity", active: capacity, scheduled: 1, activationEpoch: currentEpoch + 1, wantEpoch: currentEpoch + 1},
		{name: "same epoch replacement at capacity", active: capacity, scheduled: 1, activationEpoch: currentEpoch + 1, exitEpoch: currentEpoch + 1},
		{name: "earlier exit frees capacity", active: capacity, scheduled: 1, activationEpoch: currentEpoch + 2, exitEpoch: currentEpoch + 1},
		{name: "overflow before later exit", active: capacity, scheduled: 1, activationEpoch: currentEpoch + 1, exitEpoch: currentEpoch + 2, wantEpoch: currentEpoch + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupDB(t)
			require.NoError(t, db.SaveGenesisData(ctx, genesisStateWithValidatorCounts(t, 1, 0)))
			genesisRoot, err := db.GenesisBlockRoot(ctx)
			require.NoError(t, err)

			cst := genesisStateWithValidatorCounts(t, tc.active, tc.scheduled+tc.inactive)
			require.NoError(t, cst.SetSlot(primitives.Slot(currentEpoch)*cfg.SlotsPerEpoch))
			for i := uint64(0); i < tc.active+tc.scheduled; i++ {
				validator, err := cst.ValidatorAtIndex(primitives.ValidatorIndex(i))
				require.NoError(t, err)
				// A genesis-only check would miss every active validator in this checkpoint.
				validator.ActivationEpoch = 5
				if i >= tc.active {
					validator.ActivationEpoch = tc.activationEpoch
					validator.ActivationEligibilityEpoch = 4
				}
				if i == 0 && tc.exitEpoch != 0 {
					validator.ExitEpoch = tc.exitEpoch
				}
				require.NoError(t, cst.UpdateValidatorAtIndex(primitives.ValidatorIndex(i), validator))
			}
			cb := util.NewBeaconBlockZond()
			cb.Block.Slot = cst.Slot()
			cb.Block.ParentRoot = genesisRoot[:]
			alignLatestBlockHeader(t, cst, cb)
			csb, err := cst.MarshalSSZ()
			require.NoError(t, err)
			stateRoot, err := cst.HashTreeRoot(ctx)
			require.NoError(t, err)
			cb.Block.StateRoot = stateRoot[:]
			scb, err := blocks.NewSignedBeaconBlock(cb)
			require.NoError(t, err)
			cbb, err := scb.MarshalSSZ()
			require.NoError(t, err)
			blockRoot, err := scb.Block().HashTreeRoot()
			require.NoError(t, err)

			var transactionID int
			require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
				transactionID = tx.ID()
				return nil
			}))
			err = db.SaveOrigin(ctx, csb, cbb)
			if tc.wantEpoch != 0 {
				require.ErrorContains(t, fmt.Sprintf("checkpoint active validator count %d at epoch %d exceeds committee capacity %d", capacity+1, tc.wantEpoch, capacity), err)
				// An unchanged transaction ID proves no write was committed to any
				// bucket, including the backfill marker that used to be saved first.
				require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
					require.Equal(t, transactionID, tx.ID(), "rejected checkpoint modified the database")
					return nil
				}))
				_, err = db.BackfillBlockRoot(ctx)
				require.ErrorIs(t, err, ErrNotFoundBackfillBlockRoot)
				_, err = db.OriginCheckpointBlockRoot(ctx)
				require.ErrorIs(t, err, ErrNotFoundOriginBlockRoot)
				headRoot, err := db.HeadBlockRoot()
				require.NoError(t, err)
				require.Equal(t, genesisRoot, headRoot)
				require.Equal(t, false, db.HasStateSummary(ctx, blockRoot))
				return
			}
			require.NoError(t, err)
			originRoot, err := db.OriginCheckpointBlockRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, blockRoot, originRoot)
			backfillRoot, err := db.BackfillBlockRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, genesisRoot, backfillRoot)
			require.Equal(t, true, db.IsFinalizedBlock(ctx, blockRoot))
			validated, err := db.LastValidatedCheckpoint(ctx)
			require.NoError(t, err)
			require.Equal(t, currentEpoch, validated.Epoch)
			require.DeepEqual(t, blockRoot[:], validated.Root)
			loaded, err := db.State(ctx, blockRoot)
			require.NoError(t, err)
			require.NotNil(t, loaded)
			loadedRoot, err := loaded.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, stateRoot, loadedRoot)
		})
	}
}

func TestSaveOrigin_RejectsMismatchedBlock(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	if fieldparams.Preset == "minimal" {
		cfg := params.MinimalSpecConfig().Copy()
		cfg.ConfigName = params.MainnetTestName
		params.FillTestVersions(cfg, 128)
		require.NoError(t, params.SetActive(cfg))
	}
	ctx := context.Background()
	db := setupDB(t)
	require.NoError(t, db.SaveGenesisData(ctx, genesisStateWithValidatorCounts(t, 1, 0)))
	genesisRoot, err := db.GenesisBlockRoot(ctx)
	require.NoError(t, err)

	cst := genesisStateWithValidatorCounts(t, 1, 0)
	require.NoError(t, cst.SetSlot(5))
	cb := util.NewBeaconBlockZond()
	cb.Block.Slot = 5
	cb.Block.ParentRoot = genesisRoot[:]
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)
	blockRoot, err := scb.Block().HashTreeRoot()
	require.NoError(t, err)

	// The state was not produced by this block: nothing may be persisted.
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	require.ErrorIs(t, db.SaveOrigin(ctx, csb, cbb), errOriginBlockMismatch)
	_, err = db.OriginCheckpointBlockRoot(ctx)
	require.ErrorIs(t, err, ErrNotFoundOriginBlockRoot)
	_, err = db.BackfillBlockRoot(ctx)
	require.ErrorIs(t, err, ErrNotFoundBackfillBlockRoot)
	require.Equal(t, false, db.HasBlock(ctx, blockRoot))
	require.Equal(t, false, db.HasState(ctx, blockRoot))

	// A block whose only difference is its state root is refused as well:
	// it has a different root, and the real descendants would never attach.
	alignLatestBlockHeader(t, cst, cb)
	csb, err = cst.MarshalSSZ()
	require.NoError(t, err)
	wrongStateRoot := util.NewBeaconBlockZond()
	wrongStateRoot.Block = cb.Block
	wrongStateRoot.Block.StateRoot = bytesutil.PadTo([]byte("wrong"), 32)
	wsrb, err := blocks.NewSignedBeaconBlock(wrongStateRoot)
	require.NoError(t, err)
	wsrbb, err := wsrb.MarshalSSZ()
	require.NoError(t, err)
	require.ErrorIs(t, db.SaveOrigin(ctx, csb, wsrbb), errOriginBlockMismatch)
	_, err = db.OriginCheckpointBlockRoot(ctx)
	require.ErrorIs(t, err, ErrNotFoundOriginBlockRoot)

	// The matching pair is accepted.
	cb.Block.StateRoot = make([]byte, 32)
	alignLatestBlockHeader(t, cst, cb)
	csb, err = cst.MarshalSSZ()
	require.NoError(t, err)
	scb, err = blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err = scb.MarshalSSZ()
	require.NoError(t, err)
	blockRoot, err = scb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveOrigin(ctx, csb, cbb))
	originRoot, err := db.OriginCheckpointBlockRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, blockRoot, originRoot)
}

// A checkpoint state may be the block's post-state advanced through empty
// slots, even beyond the history ring; its latest block header then carries
// the block's state root.
func TestSaveOrigin_AcceptsStateAdvancedPastBlock(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	if fieldparams.Preset == "minimal" {
		cfg := params.MinimalSpecConfig().Copy()
		cfg.ConfigName = params.MainnetTestName
		params.FillTestVersions(cfg, 128)
		require.NoError(t, params.SetActive(cfg))
	}
	ctx := context.Background()
	db := setupDB(t)
	require.NoError(t, db.SaveGenesisData(ctx, genesisStateWithValidatorCounts(t, 1, 0)))
	genesisRoot, err := db.GenesisBlockRoot(ctx)
	require.NoError(t, err)

	cst := genesisStateWithValidatorCounts(t, 1, 0)
	require.NoError(t, cst.SetSlot(5))
	cb := util.NewBeaconBlockZond()
	cb.Block.Slot = 5
	cb.Block.ParentRoot = genesisRoot[:]
	alignLatestBlockHeader(t, cst, cb) // sets the block's state root to the state's root at slot 5
	// Advance the state past the block, further than the history ring reaches,
	// the way process_slot records the block's state root in the header.
	hdr := cst.LatestBlockHeader()
	require.NoError(t, cst.SetSlot(5+params.BeaconConfig().SlotsPerHistoricalRoot+5))
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)
	blockRoot, err := scb.Block().HashTreeRoot()
	require.NoError(t, err)

	// A header whose recorded state root differs from the block's is refused,
	// and nothing is persisted.
	hdr.StateRoot = bytesutil.PadTo([]byte("other"), 32)
	require.NoError(t, cst.SetLatestBlockHeader(hdr))
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	require.ErrorIs(t, db.SaveOrigin(ctx, csb, cbb), errOriginBlockMismatch)
	_, err = db.OriginCheckpointBlockRoot(ctx)
	require.ErrorIs(t, err, ErrNotFoundOriginBlockRoot)

	// The header carrying the block's state root is accepted.
	hdr.StateRoot = cb.Block.StateRoot
	require.NoError(t, cst.SetLatestBlockHeader(hdr))
	csb, err = cst.MarshalSSZ()
	require.NoError(t, err)
	require.NoError(t, db.SaveOrigin(ctx, csb, cbb))
	originRoot, err := db.OriginCheckpointBlockRoot(ctx)
	require.NoError(t, err)
	require.Equal(t, blockRoot, originRoot)
}

func TestSaveOrigin_RejectsDatabaseWithFinalizedHistory(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	if fieldparams.Preset == "minimal" {
		cfg := params.MinimalSpecConfig().Copy()
		cfg.ConfigName = params.MainnetTestName
		params.FillTestVersions(cfg, 128)
		require.NoError(t, params.SetActive(cfg))
	}
	ctx := context.Background()
	db := setupDB(t)
	require.NoError(t, db.SaveGenesisData(ctx, genesisStateWithValidatorCounts(t, 1, 0)))
	genesisRoot, err := db.GenesisBlockRoot(ctx)
	require.NoError(t, err)

	// The database finalized epoch 1.
	fb := util.NewBeaconBlockZond()
	fb.Block.Slot = params.BeaconConfig().SlotsPerEpoch
	fb.Block.ParentRoot = genesisRoot[:]
	wfb, err := blocks.NewSignedBeaconBlock(fb)
	require.NoError(t, err)
	fRoot, err := wfb.Block().HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wfb))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Slot: fb.Block.Slot, Root: fRoot[:]}))
	require.NoError(t, db.SaveFinalizedCheckpoint(ctx, &qrysmpb.Checkpoint{Epoch: 1, Root: fRoot[:]}))

	cst := genesisStateWithValidatorCounts(t, 1, 0)
	require.NoError(t, cst.SetSlot(5))
	cb := util.NewBeaconBlockZond()
	cb.Block.Slot = 5
	cb.Block.ParentRoot = genesisRoot[:]
	alignLatestBlockHeader(t, cst, cb)
	scb, err := blocks.NewSignedBeaconBlock(cb)
	require.NoError(t, err)
	cbb, err := scb.MarshalSSZ()
	require.NoError(t, err)
	csb, err := cst.MarshalSSZ()
	require.NoError(t, err)
	blockRoot, err := scb.Block().HashTreeRoot()
	require.NoError(t, err)

	require.ErrorIs(t, db.SaveOrigin(ctx, csb, cbb), errOriginOnSyncedDatabase)
	_, err = db.OriginCheckpointBlockRoot(ctx)
	require.ErrorIs(t, err, ErrNotFoundOriginBlockRoot)
	require.Equal(t, false, db.HasBlock(ctx, blockRoot))
}

// Two backups for the same head may run at the same time. Each must write its
// own temporary file, so that neither renames the other's unfinished copy into
// place, and the published file must be complete.

// A checkpoint whose genesis validators root or genesis time differs from the
// local genesis comes from another chain and must not become the trusted
// anchor, whatever its fork version says.
func TestSaveOrigin_RejectsDifferentGenesisIdentity(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	if fieldparams.Preset == "minimal" {
		cfg := params.MinimalSpecConfig().Copy()
		cfg.ConfigName = params.MainnetTestName
		params.FillTestVersions(cfg, 128)
		require.NoError(t, params.SetActive(cfg))
	}
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		mutate func(st state.BeaconState) error
	}{
		{name: "genesis validators root", mutate: func(st state.BeaconState) error {
			return st.SetGenesisValidatorsRoot(bytesutil.PadTo([]byte("other chain"), 32))
		}},
		{name: "genesis time", mutate: func(st state.BeaconState) error {
			return st.SetGenesisTime(st.GenesisTime() + 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupDB(t)
			require.NoError(t, db.SaveGenesisData(ctx, genesisStateWithValidatorCounts(t, 1, 0)))
			genesisRoot, err := db.GenesisBlockRoot(ctx)
			require.NoError(t, err)

			cst := genesisStateWithValidatorCounts(t, 1, 0)
			require.NoError(t, cst.SetSlot(5))
			require.NoError(t, tc.mutate(cst))
			cb := util.NewBeaconBlockZond()
			cb.Block.Slot = 5
			cb.Block.ParentRoot = genesisRoot[:]
			alignLatestBlockHeader(t, cst, cb)
			scb, err := blocks.NewSignedBeaconBlock(cb)
			require.NoError(t, err)
			cbb, err := scb.MarshalSSZ()
			require.NoError(t, err)
			csb, err := cst.MarshalSSZ()
			require.NoError(t, err)

			require.ErrorIs(t, db.SaveOrigin(ctx, csb, cbb), errOriginGenesisMismatch)
			_, err = db.OriginCheckpointBlockRoot(ctx)
			require.ErrorIs(t, err, ErrNotFoundOriginBlockRoot, "nothing may be persisted")
			blockRoot, err := scb.Block().HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, false, db.HasBlock(ctx, blockRoot))
		})
	}
}
