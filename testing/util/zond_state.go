package util

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	b "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/db/iface"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/beacon-chain/state/stateutil"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
)

// DeterministicGenesisStateZondWithGenesisBlock creates a genesis state, saves the genesis block,
// genesis state and head block root. It returns the genesis state, genesis block's root and
// validator private keys.
func DeterministicGenesisStateZondWithGenesisBlock(
	t *testing.T,
	ctx context.Context,
	db iface.HeadAccessDatabase,
	numValidators uint64,
) (state.BeaconState, [32]byte, []ml_dsa_87.MLDSA87Key) {
	genesisState, privateKeys := DeterministicGenesisStateZond(t, numValidators)
	stateRoot, err := genesisState.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")

	genesis := b.NewGenesisBlock(stateRoot[:])
	SaveBlock(t, ctx, db, genesis)

	parentRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")
	require.NoError(t, db.SaveState(ctx, genesisState, parentRoot), "Could not save genesis state")
	require.NoError(t, db.SaveHeadBlockRoot(ctx, parentRoot), "Could not save genesis state")

	return genesisState, parentRoot, privateKeys
}

// DeterministicGenesisStateZond returns a genesis state in Zond format made using the deterministic deposits.
func DeterministicGenesisStateZond(t testing.TB, numValidators uint64) (state.BeaconState, []ml_dsa_87.MLDSA87Key) {
	deposits, privKeys, err := DeterministicDepositsAndKeys(numValidators)
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to get %d deposits", numValidators))
	}
	executionData, err := DeterministicExecutionData(len(deposits))
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to get executiondata for %d deposits", numValidators))
	}
	beaconState, err := GenesisBeaconStateZond(context.Background(), deposits, uint64(0), executionData)
	if err != nil {
		t.Fatal(errors.Wrapf(err, "failed to get genesis beacon state of %d validators", numValidators))
	}
	resetCache()
	return beaconState, privKeys
}

// GenesisBeaconStateZond returns the genesis beacon state.
func GenesisBeaconStateZond(ctx context.Context, deposits []*qrysmpb.Deposit, genesisTime uint64, executionData *qrysmpb.ExecutionData) (state.BeaconState, error) {
	st, err := emptyGenesisStateZond()
	if err != nil {
		return nil, err
	}

	// Process initial deposits.
	st, err = helpers.UpdateGenesisExecutionData(st, deposits, executionData)
	if err != nil {
		return nil, err
	}

	st, err = processPreGenesisDeposits(ctx, st, deposits)
	if err != nil {
		return nil, errors.Wrap(err, "could not process validator deposits")
	}

	return buildGenesisBeaconStateZond(genesisTime, st, st.ExecutionData())
}

// emptyGenesisStateZond returns an empty genesis state in Zond format.
func emptyGenesisStateZond() (state.BeaconState, error) {
	st := &qrysmpb.BeaconStateZond{
		// Misc fields.
		Slot: 0,
		Fork: &qrysmpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			Epoch:           0,
		},
		// Validator registry fields.
		Validators:       []*qrysmpb.Validator{},
		Balances:         []uint64{},
		InactivityScores: []uint64{},

		JustificationBits:          []byte{0},
		HistoricalRoots:            [][]byte{},
		CurrentEpochParticipation:  []byte{},
		PreviousEpochParticipation: []byte{},

		// Execution data.
		ExecutionData:         &qrysmpb.ExecutionData{},
		ExecutionDataVotes:    []*qrysmpb.ExecutionData{},
		ExecutionDepositIndex: 0,

		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderZond{},
	}
	return state_native.InitializeFromProtoZond(st)
}

func buildGenesisBeaconStateZond(genesisTime uint64, preState state.BeaconState, executionData *qrysmpb.ExecutionData) (state.BeaconState, error) {
	if executionData == nil {
		return nil, errors.New("no executiondata provided for genesis state")
	}

	randaoMixes := make([][]byte, fieldparams.RandaoMixesLength)
	for i := range randaoMixes {
		h := make([]byte, 32)
		copy(h, executionData.BlockHash)
		randaoMixes[i] = h
	}

	zeroHash := params.BeaconConfig().ZeroHash[:]

	activeIndexRoots := make([][]byte, fieldparams.RandaoMixesLength)
	for i := range activeIndexRoots {
		activeIndexRoots[i] = zeroHash
	}

	blockRoots := make([][]byte, fieldparams.BlockRootsLength)
	for i := range blockRoots {
		blockRoots[i] = zeroHash
	}

	stateRoots := make([][]byte, fieldparams.StateRootsLength)
	for i := range stateRoots {
		stateRoots[i] = zeroHash
	}

	slashings := make([]uint64, fieldparams.SlashingsLength)

	genesisValidatorsRoot, err := stateutil.ValidatorRegistryRoot(preState.Validators())
	if err != nil {
		return nil, errors.Wrapf(err, "could not hash tree root genesis validators %v", err)
	}

	prevEpochParticipation, err := preState.PreviousEpochParticipation()
	if err != nil {
		return nil, err
	}
	currEpochParticipation, err := preState.CurrentEpochParticipation()
	if err != nil {
		return nil, err
	}
	scores, err := preState.InactivityScores()
	if err != nil {
		return nil, err
	}
	st := &qrysmpb.BeaconStateZond{
		// Misc fields.
		Slot:                  0,
		GenesisTime:           genesisTime,
		GenesisValidatorsRoot: genesisValidatorsRoot[:],

		Fork: &qrysmpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			Epoch:           0,
		},

		// Validator registry fields.
		Validators:                 preState.Validators(),
		Balances:                   preState.Balances(),
		PreviousEpochParticipation: prevEpochParticipation,
		CurrentEpochParticipation:  currEpochParticipation,
		InactivityScores:           scores,

		// Randomness and committees.
		RandaoMixes: randaoMixes,

		// Finality.
		PreviousJustifiedCheckpoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		CurrentJustifiedCheckpoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		JustificationBits: []byte{0},
		FinalizedCheckpoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},

		HistoricalRoots: [][]byte{},
		BlockRoots:      blockRoots,
		StateRoots:      stateRoots,
		Slashings:       slashings,

		// Execution data.
		ExecutionData:         executionData,
		ExecutionDataVotes:    []*qrysmpb.ExecutionData{},
		ExecutionDepositIndex: preState.ExecutionDepositIndex(),
	}

	var scBits [fieldparams.SyncAggregateSyncCommitteeBytesLength]byte
	bodyRoot, err := (&qrysmpb.BeaconBlockBodyZond{
		RandaoReveal: make([]byte, fieldparams.MLDSA87SignatureLength),
		ExecutionData: &qrysmpb.ExecutionData{
			DepositRoot: make([]byte, 32),
			BlockHash:   make([]byte, 32),
		},
		Graffiti: make([]byte, 32),
		SyncAggregate: &qrysmpb.SyncAggregate{
			SyncCommitteeBits:       scBits[:],
			SyncCommitteeSignatures: [][]byte{},
		},
		ExecutionPayload: &enginev1.ExecutionPayloadZond{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, 64),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
		},
	}).HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "could not hash tree root empty block body")
	}

	st.LatestBlockHeader = &qrysmpb.BeaconBlockHeader{
		ParentRoot: zeroHash,
		StateRoot:  zeroHash,
		BodyRoot:   bodyRoot[:],
	}

	var pubKeys [][]byte
	vals := preState.Validators()
	for i := uint64(0); i < fieldparams.SyncCommitteeLength; i++ {
		j := i % uint64(len(vals))
		pubKeys = append(pubKeys, vals[j].PublicKey)
	}

	st.CurrentSyncCommittee = &qrysmpb.SyncCommittee{
		Pubkeys: pubKeys,
	}
	st.NextSyncCommittee = &qrysmpb.SyncCommittee{
		Pubkeys: pubKeys,
	}

	st.LatestExecutionPayloadHeader = &enginev1.ExecutionPayloadHeaderZond{
		ParentHash:       make([]byte, 32),
		FeeRecipient:     make([]byte, 64),
		StateRoot:        make([]byte, 32),
		ReceiptsRoot:     make([]byte, 32),
		LogsBloom:        make([]byte, 256),
		PrevRandao:       make([]byte, 32),
		BaseFeePerGas:    make([]byte, 32),
		BlockHash:        make([]byte, 32),
		TransactionsRoot: make([]byte, 32),
		WithdrawalsRoot:  make([]byte, 32),
	}

	return state_native.InitializeFromProtoZond(st)
}

// processPreGenesisDeposits processes a deposit for the beacon state Altair before chain start.
func processPreGenesisDeposits(
	ctx context.Context,
	beaconState state.BeaconState,
	deposits []*qrysmpb.Deposit,
) (state.BeaconState, error) {
	var err error
	beaconState, err = altair.ProcessDeposits(ctx, beaconState, deposits)
	if err != nil {
		return nil, errors.Wrap(err, "could not process deposit")
	}
	beaconState, err = blocks.ActivateValidatorWithEffectiveBalance(beaconState, deposits)
	if err != nil {
		return nil, err
	}
	return beaconState, nil
}
