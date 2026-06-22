package transition

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	b "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/beacon-chain/state/stateutil"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// GenesisBeaconStateZond gets called when MinGenesisActiveValidatorCount count of
// full deposits were made to the deposit contract and the ChainStart log gets emitted.
//
// Spec pseudocode definition:
//
//	def initialize_beacon_state_from_execution(execution_block_hash: Bytes32,
//	                                    execution_timestamp: uint64,
//	                                    deposits: Sequence[Deposit]) -> BeaconState:
//	  fork = Fork(
//	      previous_version=GENESIS_FORK_VERSION,
//	      current_version=GENESIS_FORK_VERSION,
//	      epoch=GENESIS_EPOCH,
//	  )
//	  state = BeaconState(
//	      genesis_time=execution_timestamp + GENESIS_DELAY,
//	      fork=fork,
//	      execution_data=ExecutionData(block_hash=execution_block_hash, deposit_count=uint64(len(deposits))),
//	      latest_block_header=BeaconBlockHeader(body_root=hash_tree_root(BeaconBlockBody())),
//	      randao_mixes=[execution_block_hash] * EPOCHS_PER_HISTORICAL_VECTOR,  # Seed RANDAO with Eth1 entropy
//	  )
//
//	  # Process deposits
//	  leaves = list(map(lambda deposit: deposit.data, deposits))
//	  for index, deposit in enumerate(deposits):
//	      deposit_data_list = List[DepositData, 2**DEPOSIT_CONTRACT_TREE_DEPTH](*leaves[:index + 1])
//	      state.execution_data.deposit_root = hash_tree_root(deposit_data_list)
//	      process_deposit(state, deposit)
//
//	  # Process activations
//	  for index, validator in enumerate(state.validators):
//	      balance = state.balances[index]
//	      validator.effective_balance = min(balance - balance % EFFECTIVE_BALANCE_INCREMENT, MAX_EFFECTIVE_BALANCE)
//	      if validator.effective_balance == MAX_EFFECTIVE_BALANCE:
//	          validator.activation_eligibility_epoch = GENESIS_EPOCH
//	          validator.activation_epoch = GENESIS_EPOCH
//
//	  # Set genesis validators root for domain separation and chain versioning
//	  state.genesis_validators_root = hash_tree_root(state.validators)
//
//	  return state
//
// This method differs from the spec so as to process deposits beforehand instead of the end of the function.
func GenesisBeaconStateZond(ctx context.Context, deposits []*qrysmpb.Deposit, genesisTime uint64, executionData *qrysmpb.ExecutionData, ep *enginev1.ExecutionPayloadZond) (state.BeaconState, error) {
	st, err := EmptyGenesisStateZond()
	if err != nil {
		return nil, err
	}

	// Process initial deposits.
	st, err = helpers.UpdateGenesisExecutionData(st, deposits, executionData)
	if err != nil {
		return nil, err
	}

	st, err = b.ProcessPreGenesisDeposits(ctx, st, deposits)
	if err != nil {
		return nil, errors.Wrap(err, "could not process validator deposits")
	}

	// After deposits have been processed, overwrite executionData to what is passed in. This allows us to "pre-mine" validators
	// without the deposit root and count mismatching the real deposit contract.
	if err := st.SetExecutionData(executionData); err != nil {
		return nil, err
	}
	if err := st.SetExecutionDepositIndex(executionData.DepositCount); err != nil {
		return nil, err
	}

	return OptimizedGenesisBeaconStateZond(genesisTime, st, st.ExecutionData(), ep)
}

// OptimizedGenesisBeaconStateZond is used to create a state that has already processed deposits. This is to efficiently
// create a mainnet state at chainstart.
func OptimizedGenesisBeaconStateZond(genesisTime uint64, preState state.BeaconState, executionData *qrysmpb.ExecutionData, ep *enginev1.ExecutionPayloadZond) (state.BeaconState, error) {
	if executionData == nil {
		return nil, errors.New("no executionData provided for genesis state")
	}

	randaoMixes := make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector)
	for i := range randaoMixes {
		h := make([]byte, 32)
		copy(h, executionData.BlockHash)
		randaoMixes[i] = h
	}

	zeroHash := params.BeaconConfig().ZeroHash[:]

	activeIndexRoots := make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector)
	for i := range activeIndexRoots {
		activeIndexRoots[i] = zeroHash
	}

	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	for i := range blockRoots {
		blockRoots[i] = zeroHash
	}

	stateRoots := make([][]byte, params.BeaconConfig().SlotsPerHistoricalRoot)
	for i := range stateRoots {
		stateRoots[i] = zeroHash
	}

	slashings := make([]uint64, params.BeaconConfig().EpochsPerSlashingsVector)

	genesisValidatorsRoot, err := stateutil.ValidatorRegistryRoot(preState.Validators())
	if err != nil {
		return nil, errors.Wrapf(err, "could not hash tree root genesis validators %v", err)
	}

	scores, err := preState.InactivityScores()
	if err != nil {
		return nil, err
	}
	scoresMissing := len(preState.Validators()) - len(scores)
	if scoresMissing > 0 {
		scores = append(scores, make([]uint64, scoresMissing)...)
	}
	wep, err := blocks.WrappedExecutionPayloadZond(ep, 0)
	if err != nil {
		return nil, err
	}
	eph, err := blocks.PayloadToHeaderZond(wep)
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
		Validators: preState.Validators(),
		Balances:   preState.Balances(),

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
		ExecutionData:                executionData,
		ExecutionDataVotes:           []*qrysmpb.ExecutionData{},
		ExecutionDepositIndex:        preState.ExecutionDepositIndex(),
		LatestExecutionPayloadHeader: eph,
		InactivityScores:             scores,
	}

	bodyRoot, err := (&qrysmpb.BeaconBlockBodyZond{
		RandaoReveal: make([]byte, fieldparams.MLDSA87SignatureLength),
		ExecutionData: &qrysmpb.ExecutionData{
			DepositRoot: make([]byte, 32),
			BlockHash:   make([]byte, 32),
		},
		Graffiti: make([]byte, 32),
		SyncAggregate: &qrysmpb.SyncAggregate{
			SyncCommitteeBits:       make([]byte, fieldparams.SyncCommitteeLength/8),
			SyncCommitteeSignatures: [][]byte{},
		},
		ExecutionPayload: &enginev1.ExecutionPayloadZond{
			ParentHash:    make([]byte, 32),
			FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:     make([]byte, 32),
			ReceiptsRoot:  make([]byte, 32),
			LogsBloom:     make([]byte, 256),
			PrevRandao:    make([]byte, 32),
			BaseFeePerGas: make([]byte, 32),
			BlockHash:     make([]byte, 32),
			Transactions:  make([][]byte, 0),
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

	ist, err := state_native.InitializeFromProtoZond(st)
	if err != nil {
		return nil, err
	}
	sc, err := altair.NextSyncCommittee(context.Background(), ist)
	if err != nil {
		return nil, err
	}
	if err := ist.SetNextSyncCommittee(sc); err != nil {
		return nil, err
	}
	if err := ist.SetCurrentSyncCommittee(sc); err != nil {
		return nil, err
	}
	return ist, nil
}

// EmptyGenesisStateZond returns an empty beacon state object.
func EmptyGenesisStateZond() (state.BeaconState, error) {
	st := &qrysmpb.BeaconStateZond{
		// Misc fields.
		Slot: 0,
		Fork: &qrysmpb.Fork{
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			Epoch:           0,
		},
		// Validator registry fields.
		Validators: []*qrysmpb.Validator{},
		Balances:   []uint64{},

		JustificationBits: []byte{0},
		HistoricalRoots:   [][]byte{},

		// Execution data.
		ExecutionData:         &qrysmpb.ExecutionData{},
		ExecutionDataVotes:    []*qrysmpb.ExecutionData{},
		ExecutionDepositIndex: 0,
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderZond{
			ParentHash:       make([]byte, 32),
			FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
			StateRoot:        make([]byte, 32),
			ReceiptsRoot:     make([]byte, 32),
			LogsBloom:        make([]byte, 256),
			PrevRandao:       make([]byte, 32),
			BaseFeePerGas:    make([]byte, 32),
			BlockHash:        make([]byte, 32),
			TransactionsRoot: make([]byte, 32),
		},
	}

	return state_native.InitializeFromProtoZond(st)
}
