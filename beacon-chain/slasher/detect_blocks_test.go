package slasher

import (
	"context"
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	dbtest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	slashingsmock "github.com/theQRL/qrysm/beacon-chain/operations/slashings/mock"
	slashertypes "github.com/theQRL/qrysm/beacon-chain/slasher/types"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func Test_processQueuedBlocks_DetectsDoubleProposals(t *testing.T) {
	hook := logTest.NewGlobal()
	slasherDB := dbtest.SetupSlasherDB(t)
	beaconDB := dbtest.SetupDB(t)
	ctx, cancel := context.WithCancel(context.Background())

	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	// Initialize validators in the state.
	numVals := params.BeaconConfig().MinGenesisActiveValidatorCount
	validators := make([]*qrysmpb.Validator, numVals)
	privKeys := make([]ml_dsa_87.MLDSA87Key, numVals)
	for i := range validators {
		privKey, err := ml_dsa_87.RandKey()
		require.NoError(t, err)
		privKeys[i] = privKey
		validators[i] = &qrysmpb.Validator{
			PublicKey:             privKey.PublicKey().Marshal(),
			WithdrawalCredentials: make([]byte, 64),
		}
	}
	err = beaconState.SetValidators(validators)
	require.NoError(t, err)
	domain, err := signing.Domain(
		beaconState.Fork(),
		0,
		params.BeaconConfig().DomainBeaconProposer,
		beaconState.GenesisValidatorsRoot(),
	)
	require.NoError(t, err)

	mockChain := &mock.ChainService{
		State: beaconState,
	}
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:             slasherDB,
			StateNotifier:        &mock.MockStateNotifier{},
			HeadStateFetcher:     mockChain,
			StateGen:             stategen.New(beaconDB, doublylinkedtree.New()),
			SlashingPoolInserter: &slashingsmock.PoolMock{},
			ClockWaiter:          startup.NewClockSynchronizer(),
		},
		params:    DefaultParams(),
		blksQueue: newBlocksQueue(),
	}

	parentRoot := bytesutil.ToBytes32([]byte("parent"))
	err = s.serviceCfg.StateGen.SaveState(ctx, parentRoot, beaconState)
	require.NoError(t, err)

	currentSlotChan := make(chan primitives.Slot)
	s.wg.Add(1)
	go func() {
		s.processQueuedBlocks(ctx, currentSlotChan)
	}()

	signedBlkHeaders := []*slashertypes.SignedBlockHeaderWrapper{
		createProposalWrapper(t, 4, 1, []byte{1}),
		createProposalWrapper(t, 4, 1, []byte{1}),
		createProposalWrapper(t, 4, 1, []byte{1}),
		createProposalWrapper(t, 4, 1, []byte{2}),
	}

	// Add valid signatures to the block headers we are testing.
	for _, proposalWrapper := range signedBlkHeaders {
		proposalWrapper.SignedBeaconBlockHeader.Header.ParentRoot = parentRoot[:]
		headerHtr, err := proposalWrapper.SignedBeaconBlockHeader.Header.HashTreeRoot()
		require.NoError(t, err)
		container := &qrysmpb.SigningData{
			ObjectRoot: headerHtr[:],
			Domain:     domain,
		}
		signingRoot, err := container.HashTreeRoot()
		require.NoError(t, err)
		privKey := privKeys[proposalWrapper.SignedBeaconBlockHeader.Header.ProposerIndex]
		proposalWrapper.SignedBeaconBlockHeader.Signature = privKey.Sign(signingRoot[:]).Marshal()
	}

	s.blksQueue.extend(signedBlkHeaders)

	currentSlot := primitives.Slot(4)
	currentSlotChan <- currentSlot
	cancel()
	s.wg.Wait()
	require.LogsContain(t, hook, "Proposer slashing detected")
}

// Test_processQueuedBlocks_DetectsDoubleProposals_AcrossBatches exercises the
// bug fixed by upstream PR 13549: when a validator proposes safely in one
// batch and then equivocates for the same slot in a later batch, the slashing
// must still be detected. Previously, the per-validator dedup in
// `filterSafeProposals` could drop the earlier proposal from the persisted
// set, so the database lookup in the second batch found nothing to compare
// against and the slashable offense escaped detection.
func Test_processQueuedBlocks_DetectsDoubleProposals_AcrossBatches(t *testing.T) {
	hook := logTest.NewGlobal()
	slasherDB := dbtest.SetupSlasherDB(t)
	beaconDB := dbtest.SetupDB(t)
	ctx, cancel := context.WithCancel(context.Background())

	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	numVals := params.BeaconConfig().MinGenesisActiveValidatorCount
	validators := make([]*qrysmpb.Validator, numVals)
	privKeys := make([]ml_dsa_87.MLDSA87Key, numVals)
	for i := range validators {
		privKey, err := ml_dsa_87.RandKey()
		require.NoError(t, err)
		privKeys[i] = privKey
		validators[i] = &qrysmpb.Validator{
			PublicKey:             privKey.PublicKey().Marshal(),
			WithdrawalCredentials: make([]byte, 32),
		}
	}
	err = beaconState.SetValidators(validators)
	require.NoError(t, err)
	domain, err := signing.Domain(
		beaconState.Fork(),
		0,
		params.BeaconConfig().DomainBeaconProposer,
		beaconState.GenesisValidatorsRoot(),
	)
	require.NoError(t, err)

	mockChain := &mock.ChainService{State: beaconState}
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:             slasherDB,
			StateNotifier:        &mock.MockStateNotifier{},
			HeadStateFetcher:     mockChain,
			StateGen:             stategen.New(beaconDB, doublylinkedtree.New()),
			SlashingPoolInserter: &slashingsmock.PoolMock{},
			ClockWaiter:          startup.NewClockSynchronizer(),
		},
		params:    DefaultParams(),
		blksQueue: newBlocksQueue(),
	}

	parentRoot := bytesutil.ToBytes32([]byte("parent"))
	require.NoError(t, s.serviceCfg.StateGen.SaveState(ctx, parentRoot, beaconState))

	currentSlotChan := make(chan primitives.Slot)
	s.wg.Add(1)
	go func() {
		s.processQueuedBlocks(ctx, currentSlotChan)
	}()

	// Batch 1: validator 1 proposes safely at slots 4 and 5.
	// Under the buggy `filterSafeProposals`, the per-validator map would
	// retain only the last proposal (slot 5), dropping slot 4 from persistence.
	batch1 := []*slashertypes.SignedBlockHeaderWrapper{
		createProposalWrapper(t, 4, 1, []byte{1}),
		createProposalWrapper(t, 5, 1, []byte{1}),
	}
	// Batch 2: validator 1 equivocates for slot 4 (different signing root).
	// With the fix, slot 4 is now in the DB and CheckDoubleBlockProposals
	// surfaces this as a slashable offense.
	batch2 := []*slashertypes.SignedBlockHeaderWrapper{
		createProposalWrapper(t, 4, 1, []byte{2}),
	}

	for _, batch := range [][]*slashertypes.SignedBlockHeaderWrapper{batch1, batch2} {
		for _, proposalWrapper := range batch {
			proposalWrapper.SignedBeaconBlockHeader.Header.ParentRoot = parentRoot[:]
			headerHtr, err := proposalWrapper.SignedBeaconBlockHeader.Header.HashTreeRoot()
			require.NoError(t, err)
			container := &qrysmpb.SigningData{
				ObjectRoot: headerHtr[:],
				Domain:     domain,
			}
			signingRoot, err := container.HashTreeRoot()
			require.NoError(t, err)
			privKey := privKeys[proposalWrapper.SignedBeaconBlockHeader.Header.ProposerIndex]
			proposalWrapper.SignedBeaconBlockHeader.Signature = privKey.Sign(signingRoot[:]).Marshal()
		}
		s.blksQueue.extend(batch)
		currentSlotChan <- primitives.Slot(4)
	}

	cancel()
	s.wg.Wait()
	require.LogsContain(t, hook, "Proposer slashing detected")
}

func Test_processQueuedBlocks_NotSlashable(t *testing.T) {
	hook := logTest.NewGlobal()
	slasherDB := dbtest.SetupSlasherDB(t)
	ctx, cancel := context.WithCancel(context.Background())

	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	currentSlot := primitives.Slot(4)
	require.NoError(t, beaconState.SetSlot(currentSlot))
	mockChain := &mock.ChainService{
		State: beaconState,
		Slot:  &currentSlot,
	}

	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:         slasherDB,
			StateNotifier:    &mock.MockStateNotifier{},
			HeadStateFetcher: mockChain,
			ClockWaiter:      startup.NewClockSynchronizer(),
		},
		params:    DefaultParams(),
		blksQueue: newBlocksQueue(),
	}
	currentSlotChan := make(chan primitives.Slot)
	s.wg.Add(1)
	go func() {
		s.processQueuedBlocks(ctx, currentSlotChan)
	}()
	s.blksQueue.extend([]*slashertypes.SignedBlockHeaderWrapper{
		createProposalWrapper(t, 4, 1, []byte{1}),
		createProposalWrapper(t, 4, 1, []byte{1}),
	})
	currentSlotChan <- currentSlot
	cancel()
	s.wg.Wait()
	require.LogsDoNotContain(t, hook, "Proposer slashing detected")
}

func createProposalWrapper(t *testing.T, slot primitives.Slot, proposerIndex primitives.ValidatorIndex, signingRoot []byte) *slashertypes.SignedBlockHeaderWrapper {
	header := &qrysmpb.BeaconBlockHeader{
		Slot:          slot,
		ProposerIndex: proposerIndex,
		ParentRoot:    params.BeaconConfig().ZeroHash[:],
		StateRoot:     bytesutil.PadTo(signingRoot, 32),
		BodyRoot:      params.BeaconConfig().ZeroHash[:],
	}
	signRoot, err := header.HashTreeRoot()
	require.NoError(t, err)
	fakeSig := make([]byte, field_params.MLDSA87SignatureLength)
	copy(fakeSig, "hello")
	return &slashertypes.SignedBlockHeaderWrapper{
		SignedBeaconBlockHeader: &qrysmpb.SignedBeaconBlockHeader{
			Header:    header,
			Signature: fakeSig,
		},
		SigningRoot: signRoot,
	}
}
