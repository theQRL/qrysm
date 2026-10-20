package kv

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"testing"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/theQRL/qrysm/beacon-chain/db/iface"
	dbIface "github.com/theQRL/qrysm/beacon-chain/db/iface"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	bolt "go.etcd.io/bbolt"
)

func TestStore_SaveGenesisData(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)

	gs, err := util.NewBeaconStateZond()
	assert.NoError(t, err)

	assert.NoError(t, db.SaveGenesisData(ctx, gs))

	testGenesisDataSaved(t, db)
}

func TestStore_GenesisActiveValidatorCapacity(t *testing.T) {
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

	for _, tc := range []struct {
		name            string
		active          uint64
		inactive        uint64
		scheduled       uint64
		activationEpoch primitives.Epoch
		exitEpoch       primitives.Epoch // Optional exit for the first active validator.
		wantErr         bool
	}{
		{name: "at capacity", active: capacity},
		{name: "above capacity", active: capacity + 1, wantErr: true},
		{name: "inactive records beyond capacity", active: capacity, inactive: 1},
		{name: "scheduled activations reach capacity", active: capacity - 1, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 1},
		{name: "scheduled activations exceed capacity", active: capacity, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 1, wantErr: true},
		{name: "same epoch replacement at capacity", active: capacity, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 1, exitEpoch: cfg.GenesisEpoch + 1},
		{name: "earlier exit frees capacity", active: capacity, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 2, exitEpoch: cfg.GenesisEpoch + 1},
		{name: "overflow before later exit", active: capacity, scheduled: 1, activationEpoch: cfg.GenesisEpoch + 1, exitEpoch: cfg.GenesisEpoch + 2, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gs := genesisStateWithValidatorCounts(t, tc.active, tc.scheduled+tc.inactive)
			for i := tc.active; i < tc.active+tc.scheduled; i++ {
				validator, err := gs.ValidatorAtIndex(primitives.ValidatorIndex(i))
				require.NoError(t, err)
				validator.ActivationEpoch = tc.activationEpoch
				validator.ActivationEligibilityEpoch = cfg.GenesisEpoch
				require.NoError(t, gs.UpdateValidatorAtIndex(primitives.ValidatorIndex(i), validator))
			}
			if tc.exitEpoch != 0 {
				validator, err := gs.ValidatorAtIndex(0)
				require.NoError(t, err)
				validator.ExitEpoch = tc.exitEpoch
				require.NoError(t, gs.UpdateValidatorAtIndex(0, validator))
			}
			// Marshal even the over-cap state: SSZ alone does not constrain
			// the number of active validators, only the much larger registry.
			sb, err := gs.MarshalSSZ()
			require.NoError(t, err)
			for _, mode := range []string{"save", "import"} {
				t.Run(mode, func(t *testing.T) {
					db := setupDB(t)
					var transactionID int
					require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
						transactionID = tx.ID()
						return nil
					}))
					var err error
					if mode == "save" {
						err = db.SaveGenesisData(ctx, gs)
					} else {
						err = db.LoadGenesis(ctx, sb)
					}
					if tc.wantErr {
						wantErr := fmt.Sprintf("genesis active validator count %d exceeds committee capacity %d", tc.active, capacity)
						if tc.scheduled != 0 {
							wantErr = fmt.Sprintf("genesis active validator count %d at epoch %d exceeds committee capacity %d", tc.active+tc.scheduled, tc.activationEpoch, capacity)
						}
						require.ErrorContains(t, wantErr, err)
						// Reject before persisting blocks, state, or metadata.
						require.NoError(t, db.db.View(func(tx *bolt.Tx) error {
							require.Equal(t, transactionID, tx.ID(), "rejected genesis modified the database")
							for _, bucket := range [][]byte{blocksBucket, stateBucket, stateSummaryBucket, stateValidatorsBucket} {
								require.Equal(t, 0, tx.Bucket(bucket).Stats().KeyN, "unexpected writes to %s", bucket)
							}
							return nil
						}))
						return
					}
					require.NoError(t, err)
					testGenesisDataSaved(t, db)
					loaded, err := db.GenesisState(ctx)
					require.NoError(t, err)
					require.Equal(t, int(tc.active+tc.scheduled+tc.inactive), loaded.NumValidators())
					// Preserve idempotent imports for valid genesis states.
					require.NoError(t, db.LoadGenesis(ctx, sb))
				})
			}
		})
	}

	for _, scheduled := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing genesis is also validated/scheduled=%t", scheduled), func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			db := setupDB(t)
			gs := genesisStateWithValidatorCounts(t, capacity, 0)
			if scheduled {
				for i := primitives.ValidatorIndex(1); i < primitives.ValidatorIndex(capacity); i++ {
					validator, err := gs.ValidatorAtIndex(i)
					require.NoError(t, err)
					validator.ActivationEpoch = cfg.GenesisEpoch + 1
					require.NoError(t, gs.UpdateValidatorAtIndex(i, validator))
				}
			}
			require.NoError(t, db.SaveGenesisData(ctx, gs))
			sb, err := gs.MarshalSSZ()
			require.NoError(t, err)

			// Model a genesis persisted before the cap was enforced by lowering
			// the capacity after saving it. Reimporting must not bypass validation
			// merely because the stored and supplied states have the same root.
			cfg := params.BeaconConfig().Copy()
			cfg.MaxValidatorsPerCommittee--
			params.OverrideBeaconConfig(cfg)
			require.ErrorContains(t, "exceeds committee capacity", db.LoadGenesis(ctx, sb))
		})
	}
}

func genesisStateWithValidatorCounts(t *testing.T, active, inactive uint64) state.BeaconState {
	t.Helper()
	cfg := params.BeaconConfig()
	gs, err := util.NewBeaconStateZond(func(st *qrysmpb.BeaconStateZond) error {
		st.Fork.PreviousVersion = cfg.GenesisForkVersion
		st.Fork.CurrentVersion = cfg.GenesisForkVersion
		count := active + inactive
		st.Validators = make([]*qrysmpb.Validator, count)
		st.Balances = make([]uint64, count)
		st.InactivityScores = make([]uint64, count)
		st.PreviousEpochParticipation = make([]byte, count)
		st.CurrentEpochParticipation = make([]byte, count)
		for i := range st.Validators {
			pubkey := make([]byte, fieldparams.MLDSA87PubkeyLength)
			binary.LittleEndian.PutUint64(pubkey, uint64(i))
			val := &qrysmpb.Validator{
				PublicKey:           pubkey,
				WithdrawalRecipient: make([]byte, fieldparams.WithdrawalRecipientLength),
				EffectiveBalance:    cfg.MaxEffectiveBalance,
				RandaoCommitment:    make([]byte, fieldparams.RandaoCommitmentLength),
				ActivationEpoch:     cfg.GenesisEpoch,
				ExitEpoch:           cfg.FarFutureEpoch,
				WithdrawableEpoch:   cfg.FarFutureEpoch,
			}
			if uint64(i) >= active {
				val.ActivationEpoch = cfg.FarFutureEpoch
				val.ActivationEligibilityEpoch = cfg.FarFutureEpoch
			}
			st.Validators[i] = val
			st.Balances[i] = cfg.MaxEffectiveBalance
		}
		return nil
	})
	require.NoError(t, err)
	return gs
}

func testGenesisDataSaved(t *testing.T, db iface.Database) {
	ctx := context.Background()

	gb, err := db.GenesisBlock(ctx)
	require.NoError(t, err)
	require.NotNil(t, gb)

	gbHTR, err := gb.Block().HashTreeRoot()
	require.NoError(t, err)

	gss, err := db.StateSummary(ctx, gbHTR)
	require.NoError(t, err)
	require.NotNil(t, gss)

	head, err := db.HeadBlock(ctx)
	require.NoError(t, err)
	require.NotNil(t, head)

	headHTR, err := head.Block().HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, gbHTR, headHTR, "head block does not match genesis block")
}

func TestLoadGenesisFromFile(t *testing.T) {
	// for this test to work, we need the active config to have these properties:
	// - fork version schedule that matches mainnnet.genesis.ssz
	// - name that does not match params.MainnetName - otherwise we'll trigger the codepath that loads the state
	//   from the compiled binary.
	// to do that, first we need to rewrite the mainnet fork schedule so it won't conflict with a renamed config that
	// uses the mainnet fork schedule. construct the differently named mainnet config and set it active.
	// finally, revert all this at the end of the test.

	// first get the real mainnet out of the way by overwriting its schedule.
	cfg, err := params.ByName(params.MainnetName)
	require.NoError(t, err)
	cfg = cfg.Copy()
	reversioned := cfg.Copy()
	params.FillTestVersions(reversioned, 127)
	undo, err := params.SetActiveWithUndo(reversioned)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, undo())
	}()

	// then set up a new config, which uses the real mainnet schedule, and activate it
	cfg.ConfigName = "genesis-test"
	undo2, err := params.SetActiveWithUndo(cfg)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, undo2())
	}()

	fp := "testdata/mainnet.genesis.ssz"
	rfp, err := bazel.Runfile(fp)
	if err == nil {
		fp = rfp
	}
	sb, err := os.ReadFile(fp)
	require.NoError(t, err)

	db := setupDB(t)
	require.NoError(t, db.LoadGenesis(context.Background(), sb))
	testGenesisDataSaved(t, db)

	// Loading the same genesis again should not throw an error
	require.NoError(t, err)
	require.NoError(t, db.LoadGenesis(context.Background(), sb))
	testGenesisDataSaved(t, db)
}

func TestLoadGenesisFromFile_mismatchedForkVersion(t *testing.T) {
	fp := "testdata/altona.genesis.ssz"
	rfp, err := bazel.Runfile(fp)
	if err == nil {
		fp = rfp
	}
	sb, err := os.ReadFile(fp)
	assert.NoError(t, err)

	// Loading a genesis with the wrong fork version as beacon config should throw an error.
	db := setupDB(t)
	assert.ErrorContains(t, "not found in any known fork choice schedule", db.LoadGenesis(context.Background(), sb))
}

func TestEnsureEmbeddedGenesis(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	// Embedded Genesis works with Mainnet config
	cfg := params.MainnetConfig().Copy()
	cfg.SecondsPerSlot = 1
	undo, err := params.SetActiveWithUndo(cfg)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, undo())
	}()

	ctx := context.Background()
	db := setupDB(t)

	gb, err := db.GenesisBlock(ctx)
	assert.NoError(t, err)
	if gb != nil && !gb.IsNil() {
		t.Fatal("Genesis block exists already")
	}

	gs, err := db.GenesisState(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, gs, "an embedded genesis state does not exist")

	assert.NoError(t, db.EnsureEmbeddedGenesis(ctx))

	gb, err = db.GenesisBlock(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, gb)

	testGenesisDataSaved(t, db)
}

func TestEnsureEmbeddedGenesis_RejectsDifferentStoredGenesis(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MainnetConfig().Copy()
	cfg.SecondsPerSlot = 1
	undo, err := params.SetActiveWithUndo(cfg)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, undo())
	}()

	ctx := context.Background()
	db := setupDB(t)
	embedded, err := db.GenesisState(ctx)
	require.NoError(t, err)
	require.NotNil(t, embedded, "an embedded genesis state does not exist")

	other, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisData(ctx, other))

	require.ErrorIs(t, db.EnsureEmbeddedGenesis(ctx), dbIface.ErrEmbeddedGenesisMismatch)
}

// The genesis block carries the zero execution block hash, which no execution
// client can resolve, so it is stored with its full payload even in a database
// that stores blinded blocks.

func TestSaveGenesisData_StoresGenesisBlockUnblinded(t *testing.T) {
	ctx := context.Background()
	db := setupDB(t)
	blinded, err := db.shouldSaveBlinded(ctx)
	require.NoError(t, err)
	require.Equal(t, true, blinded, "a fresh database stores blinded blocks")

	gs, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisData(ctx, gs))
	gb, err := db.GenesisBlock(ctx)
	require.NoError(t, err)
	require.Equal(t, false, gb.IsBlinded())

	// Later blocks keep the blinded storage format.
	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.NoError(t, db.SaveBlock(ctx, wsb))
	root, err := wsb.Block().HashTreeRoot()
	require.NoError(t, err)
	stored, err := db.Block(ctx, root)
	require.NoError(t, err)
	require.Equal(t, true, stored.IsBlinded())
}

// A checkpoint can only be the origin of a chain without finalized history.

// A genesis block that an older release stored blinded is rewritten with its
// empty execution payload when the database is opened, so it can be served.
func TestNewKVStore_RepairsBlindedGenesisBlock(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := NewKVStore(ctx, dir)
	require.NoError(t, err)
	gs, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveGenesisData(ctx, gs))
	gb, err := db.GenesisBlock(ctx)
	require.NoError(t, err)
	root, err := gb.Block().HashTreeRoot()
	require.NoError(t, err)

	// Store it the way an older release did.
	blindedEnc, err := marshalBlockBlinded(ctx, gb)
	require.NoError(t, err)
	require.NoError(t, db.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(blocksBucket).Put(root[:], blindedEnc)
	}))
	db.blockCache.Del(string(root[:]))
	stored, err := db.GenesisBlock(ctx)
	require.NoError(t, err)
	require.Equal(t, true, stored.IsBlinded())
	require.NoError(t, db.Close())

	reopened, err := NewKVStore(ctx, dir)
	require.NoError(t, err)
	defer func() { require.NoError(t, reopened.Close()) }()
	repaired, err := reopened.GenesisBlock(ctx)
	require.NoError(t, err)
	require.Equal(t, false, repaired.IsBlinded(), "the genesis block must be stored with its payload again")
	repairedRoot, err := repaired.Block().HashTreeRoot()
	require.NoError(t, err)
	require.Equal(t, root, repairedRoot)
	byRoot, err := reopened.Block(ctx, root)
	require.NoError(t, err)
	require.Equal(t, false, byRoot.IsBlinded())

	// A database whose genesis block is already full is left alone.
	require.NoError(t, reopened.repairBlindedGenesisBlock(ctx))
	again, err := reopened.GenesisBlock(ctx)
	require.NoError(t, err)
	require.Equal(t, false, again.IsBlinded())
}
