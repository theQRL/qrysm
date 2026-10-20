package slasher

import (
	"context"
	"errors"
	"testing"

	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	dbtest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	slashingsmock "github.com/theQRL/qrysm/beacon-chain/operations/slashings/mock"
	slashertypes "github.com/theQRL/qrysm/beacon-chain/slasher/types"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

// Block headers reach the slasher before gossip has verified them. A header
// with a junk signature for a (slot, proposer) that already has a stored record
// used to overwrite that record, so the genuine second proposal was only ever
// compared against the junk and the pair was dropped as unverifiable.
func Test_detectProposerSlashings_JunkHeaderDoesNotMaskDoubleProposal(t *testing.T) {
	slasherDB := dbtest.SetupSlasherDB(t)
	beaconDB := dbtest.SetupDB(t)
	ctx := context.Background()

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
			PublicKey:           privKey.PublicKey().Marshal(),
			WithdrawalRecipient: make([]byte, 32),
		}
	}
	require.NoError(t, beaconState.SetValidators(validators))
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconProposer, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)

	mockChain := &mock.ChainService{State: beaconState}
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:             slasherDB,
			HeadStateFetcher:     mockChain,
			StateGen:             stategen.New(beaconDB, doublylinkedtree.New()),
			SlashingPoolInserter: &slashingsmock.PoolMock{},
		},
		params: DefaultParams(),
	}
	parentRoot := bytesutil.ToBytes32([]byte("parent"))
	require.NoError(t, s.serviceCfg.StateGen.SaveState(ctx, parentRoot, beaconState))

	sign := func(w *slashertypes.SignedBlockHeaderWrapper) {
		headerHtr, err := w.SignedBeaconBlockHeader.Header.HashTreeRoot()
		require.NoError(t, err)
		signingRoot, err := (&qrysmpb.SigningData{ObjectRoot: headerHtr[:], Domain: domain}).HashTreeRoot()
		require.NoError(t, err)
		sig, err := privKeys[w.SignedBeaconBlockHeader.Header.ProposerIndex].Sign(signingRoot[:])
		require.NoError(t, err)
		w.SignedBeaconBlockHeader.Signature = sig.Marshal()
	}
	b1 := createProposalWrapper(t, 4, 1, []byte{1})
	junk := createProposalWrapper(t, 4, 1, []byte{2})
	b2 := createProposalWrapper(t, 4, 1, []byte{3})
	for _, w := range []*slashertypes.SignedBlockHeaderWrapper{b1, junk, b2} {
		w.SignedBeaconBlockHeader.Header.ParentRoot = parentRoot[:]
	}
	sign(b1)
	sign(b2)
	// junk keeps its unverifiable signature.

	// Batch 1: the genuine first proposal.
	slashings, err := s.detectProposerSlashings(ctx, []*slashertypes.SignedBlockHeaderWrapper{b1})
	require.NoError(t, err)
	require.Equal(t, 0, len(slashings))

	// Batch 2: a junk header for the same (slot, proposer). It conflicts with
	// the stored record but must not replace it.
	slashings, err = s.detectProposerSlashings(ctx, []*slashertypes.SignedBlockHeaderWrapper{junk})
	require.NoError(t, err)
	require.Equal(t, 1, len(slashings))
	stored, err := slasherDB.BlockProposalForValidator(ctx, 1, 4)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, b1.SigningRoot, stored.SigningRoot, "the junk header replaced the verified record")

	// Batch 3: the genuine equivocation is compared against the genuine first proposal.
	slashings, err = s.detectProposerSlashings(ctx, []*slashertypes.SignedBlockHeaderWrapper{b2})
	require.NoError(t, err)
	require.Equal(t, 1, len(slashings))
	require.DeepEqual(t, b1.SignedBeaconBlockHeader.Header.StateRoot, slashings[0].Header_1.Header.StateRoot)
	require.DeepEqual(t, b2.SignedBeaconBlockHeader.Header.StateRoot, slashings[0].Header_2.Header.StateRoot)
	// Both signatures verify, so the slashing is accepted.
	require.NoError(t, s.processProposerSlashings(ctx, slashings))
	pool, ok := s.serviceCfg.SlashingPoolInserter.(*slashingsmock.PoolMock)
	require.Equal(t, true, ok)
	require.Equal(t, 1, len(pool.PendingPropSlashings))
}

// When no record exists yet, the first verifiable header of a conflicting set
// is stored; junk alone stores nothing.
func Test_detectProposerSlashings_StoresOnlyVerifiableConflictingHeaders(t *testing.T) {
	slasherDB := dbtest.SetupSlasherDB(t)
	beaconDB := dbtest.SetupDB(t)
	ctx := context.Background()

	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	privKey, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	validators := []*qrysmpb.Validator{{PublicKey: privKey.PublicKey().Marshal(), WithdrawalRecipient: make([]byte, 32)}}
	require.NoError(t, beaconState.SetValidators(validators))
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconProposer, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)

	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:             slasherDB,
			HeadStateFetcher:     &mock.ChainService{State: beaconState},
			StateGen:             stategen.New(beaconDB, doublylinkedtree.New()),
			SlashingPoolInserter: &slashingsmock.PoolMock{},
		},
		params: DefaultParams(),
	}
	parentRoot := bytesutil.ToBytes32([]byte("parent"))
	require.NoError(t, s.serviceCfg.StateGen.SaveState(ctx, parentRoot, beaconState))

	junk1 := createProposalWrapper(t, 9, 0, []byte{1})
	junk2 := createProposalWrapper(t, 9, 0, []byte{2})
	genuine := createProposalWrapper(t, 9, 0, []byte{3})
	for _, w := range []*slashertypes.SignedBlockHeaderWrapper{junk1, junk2, genuine} {
		w.SignedBeaconBlockHeader.Header.ParentRoot = parentRoot[:]
	}
	headerHtr, err := genuine.SignedBeaconBlockHeader.Header.HashTreeRoot()
	require.NoError(t, err)
	signingRoot, err := (&qrysmpb.SigningData{ObjectRoot: headerHtr[:], Domain: domain}).HashTreeRoot()
	require.NoError(t, err)
	sig, err := privKey.Sign(signingRoot[:])
	require.NoError(t, err)
	genuine.SignedBeaconBlockHeader.Signature = sig.Marshal()

	// Two junk headers: nothing verifies, nothing is stored.
	_, err = s.detectProposerSlashings(ctx, []*slashertypes.SignedBlockHeaderWrapper{junk1, junk2})
	require.NoError(t, err)
	stored, err := slasherDB.BlockProposalForValidator(ctx, 0, 9)
	require.NoError(t, err)
	require.Equal(t, true, stored == nil)

	// Junk alongside the genuine header in one batch: the genuine one is stored.
	_, err = s.detectProposerSlashings(ctx, []*slashertypes.SignedBlockHeaderWrapper{junk1, genuine})
	require.NoError(t, err)
	stored, err = slasherDB.BlockProposalForValidator(ctx, 0, 9)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, genuine.SigningRoot, stored.SigningRoot)
}

// A header whose signature cannot be checked right now (the head state is not
// available) is queued again rather than discarded, never replaces a verified
// record, and is given up on after a bounded number of retries.
func Test_resolveConflictingProposals_UnavailableHeadStateRetriesLater(t *testing.T) {
	slasherDB := dbtest.SetupSlasherDB(t)
	ctx := context.Background()

	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	privKey, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	validators := []*qrysmpb.Validator{{PublicKey: privKey.PublicKey().Marshal(), WithdrawalRecipient: make([]byte, 32)}}
	require.NoError(t, beaconState.SetValidators(validators))
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconProposer, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)

	mockChain := &mock.ChainService{State: beaconState}
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:             slasherDB,
			HeadStateFetcher:     mockChain,
			SlashingPoolInserter: &slashingsmock.PoolMock{},
		},
		params:    DefaultParams(),
		blksQueue: newBlocksQueue(),
	}
	sign := func(w *slashertypes.SignedBlockHeaderWrapper) {
		headerHtr, err := w.SignedBeaconBlockHeader.Header.HashTreeRoot()
		require.NoError(t, err)
		signingRoot, err := (&qrysmpb.SigningData{ObjectRoot: headerHtr[:], Domain: domain}).HashTreeRoot()
		require.NoError(t, err)
		sig, err := privKey.Sign(signingRoot[:])
		require.NoError(t, err)
		w.SignedBeaconBlockHeader.Signature = sig.Marshal()
	}
	a := createProposalWrapper(t, 11, 0, []byte{1})
	b := createProposalWrapper(t, 11, 0, []byte{2})
	sign(a)
	sign(b)

	// No head state: nothing can be verified, nothing is stored, both are queued again.
	mockChain.HeadStateErr = errors.New("head state not available")
	slashings, err := s.detectProposerSlashings(ctx, []*slashertypes.SignedBlockHeaderWrapper{a, b})
	require.NoError(t, err)
	require.Equal(t, 1, len(slashings), "the in-batch double proposal is still reported")
	stored, err := slasherDB.BlockProposalForValidator(ctx, 0, 11)
	require.NoError(t, err)
	require.Equal(t, true, stored == nil)
	requeued := s.blksQueue.dequeue()
	require.Equal(t, 2, len(requeued))

	// The head state is back: the retry stores the first verified header.
	mockChain.HeadStateErr = nil
	_, err = s.detectProposerSlashings(ctx, requeued)
	require.NoError(t, err)
	stored, err = slasherDB.BlockProposalForValidator(ctx, 0, 11)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, a.SigningRoot, stored.SigningRoot)
	require.Equal(t, 0, len(s.blksQueue.dequeue()))

	// With the head state gone again, a new conflicting header neither replaces
	// the verified record nor is lost; it is retried a bounded number of times.
	c := createProposalWrapper(t, 11, 0, []byte{3})
	sign(c)
	mockChain.HeadStateErr = errors.New("head state not available")
	_, err = s.detectProposerSlashings(ctx, []*slashertypes.SignedBlockHeaderWrapper{c})
	require.NoError(t, err)
	stored, err = slasherDB.BlockProposalForValidator(ctx, 0, 11)
	require.NoError(t, err)
	require.Equal(t, a.SigningRoot, stored.SigningRoot)
	rounds := 0
	for {
		items := s.blksQueue.dequeue()
		if len(items) == 0 {
			break
		}
		rounds++
		require.Equal(t, true, rounds <= maxProposalRetries+1, "unresolved header retried without bound")
		_, err = s.detectProposerSlashings(ctx, items)
		require.NoError(t, err)
	}
	require.Equal(t, maxProposalRetries, rounds)
}

// A proposal queued again for an unverifiable conflict whose counterpart is
// pruned before the retry runs comes back as a safe proposal; its retry
// bookkeeping is cleared with it.
func Test_requeuedProposal_RetryBookkeepingClearedWhenCounterpartPruned(t *testing.T) {
	slasherDB := dbtest.SetupSlasherDB(t)
	ctx := context.Background()

	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	privKey, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	validators := []*qrysmpb.Validator{{PublicKey: privKey.PublicKey().Marshal(), WithdrawalRecipient: make([]byte, 32)}}
	require.NoError(t, beaconState.SetValidators(validators))
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconProposer, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)

	mockChain := &mock.ChainService{State: beaconState}
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:             slasherDB,
			HeadStateFetcher:     mockChain,
			SlashingPoolInserter: &slashingsmock.PoolMock{},
		},
		params:    DefaultParams(),
		blksQueue: newBlocksQueue(),
	}
	sign := func(w *slashertypes.SignedBlockHeaderWrapper) {
		headerHtr, err := w.SignedBeaconBlockHeader.Header.HashTreeRoot()
		require.NoError(t, err)
		signingRoot, err := (&qrysmpb.SigningData{ObjectRoot: headerHtr[:], Domain: domain}).HashTreeRoot()
		require.NoError(t, err)
		sig, err := privKey.Sign(signingRoot[:])
		require.NoError(t, err)
		w.SignedBeaconBlockHeader.Signature = sig.Marshal()
	}
	a := createProposalWrapper(t, 5, 0, []byte{1})
	c := createProposalWrapper(t, 5, 0, []byte{2})
	sign(a)
	sign(c)
	_, err = s.detectProposerSlashings(ctx, []*slashertypes.SignedBlockHeaderWrapper{a})
	require.NoError(t, err)

	// c conflicts with a but cannot be verified: queued again.
	mockChain.HeadStateErr = errors.New("head state not available")
	_, err = s.detectProposerSlashings(ctx, []*slashertypes.SignedBlockHeaderWrapper{c})
	require.NoError(t, err)
	require.Equal(t, uint8(1), s.proposalRetries[c.SigningRoot])
	requeued := s.blksQueue.dequeue()
	require.Equal(t, 1, len(requeued))

	// The stored counterpart is pruned before the retry runs.
	_, err = slasherDB.PruneProposalsAtEpoch(ctx, 1)
	require.NoError(t, err)
	stored, err := slasherDB.BlockProposalForValidator(ctx, 0, 5)
	require.NoError(t, err)
	require.Equal(t, true, stored == nil)

	// The retry finds no conflict, is saved as safe, and leaves no bookkeeping behind.
	mockChain.HeadStateErr = nil
	_, err = s.detectProposerSlashings(ctx, requeued)
	require.NoError(t, err)
	stored, err = slasherDB.BlockProposalForValidator(ctx, 0, 5)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, c.SigningRoot, stored.SigningRoot)
	_, tracked := s.proposalRetries[c.SigningRoot]
	require.Equal(t, false, tracked, "retry bookkeeping leaked")
	require.Equal(t, 0, len(s.blksQueue.dequeue()))
}

// The latest epoch written per validator is persisted as epochs advance, not
// only on a clean shutdown.
func Test_persistLatestEpochWritten(t *testing.T) {
	slasherDB := dbtest.SetupSlasherDB(t)
	ctx := context.Background()
	s := &Service{
		serviceCfg:                     &ServiceConfig{Database: slasherDB},
		latestEpochWrittenForValidator: map[primitives.ValidatorIndex]primitives.Epoch{1: 5, 2: 4},
	}

	s.persistLatestEpochWritten(ctx, 5)
	require.Equal(t, primitives.Epoch(5), s.lastEpochWrittenPersisted)
	got, err := slasherDB.LastEpochWrittenForValidators(ctx, []primitives.ValidatorIndex{1, 2})
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(5), got[0].Epoch)
	require.Equal(t, primitives.Epoch(4), got[1].Epoch)

	// Within the same epoch nothing is rewritten; the next epoch is.
	s.latestEpochWrittenForValidator[1] = 6
	s.persistLatestEpochWritten(ctx, 5)
	got, err = slasherDB.LastEpochWrittenForValidators(ctx, []primitives.ValidatorIndex{1})
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(5), got[0].Epoch)
	s.persistLatestEpochWritten(ctx, 6)
	got, err = slasherDB.LastEpochWrittenForValidators(ctx, []primitives.ValidatorIndex{1})
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(6), got[0].Epoch)
}
