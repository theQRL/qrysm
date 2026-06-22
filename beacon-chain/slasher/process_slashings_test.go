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
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestService_processAttesterSlashings(t *testing.T) {
	ctx := context.Background()
	slasherDB := dbtest.SetupSlasherDB(t)
	beaconDB := dbtest.SetupDB(t)

	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	privKey, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	validators := make([]*qrysmpb.Validator, 1)
	validators[0] = &qrysmpb.Validator{
		PublicKey:             privKey.PublicKey().Marshal(),
		WithdrawalCredentials: make([]byte, 64),
		EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
	}
	err = beaconState.SetValidators(validators)
	require.NoError(t, err)

	mockChain := &mock.ChainService{
		State: beaconState,
	}
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:                slasherDB,
			AttestationStateFetcher: mockChain,
			StateGen:                stategen.New(beaconDB, doublylinkedtree.New()),
			SlashingPoolInserter:    &slashingsmock.PoolMock{},
			HeadStateFetcher:        mockChain,
		},
	}

	firstAtt := util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
		AttestingIndices: []uint64{0},
	})
	secondAtt := util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
		AttestingIndices: []uint64{0},
	})

	domain, err := signing.Domain(
		beaconState.Fork(),
		0,
		params.BeaconConfig().DomainBeaconAttester,
		beaconState.GenesisValidatorsRoot(),
	)
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(firstAtt.Data, domain)
	require.NoError(t, err)

	t.Run("first_att_valid_sig_second_invalid", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use valid signature for the first att, but bad one for the second.
		signature := privKey.Sign(signingRoot[:])
		firstAtt.Signatures = [][]byte{signature.Marshal()}
		secondAtt.Signatures = [][]byte{make([]byte, 4627)}

		slashing := &qrysmpb.AttesterSlashing{
			Attestation_1: firstAtt,
			Attestation_2: secondAtt,
		}
		slashingRoot, err := slashing.HashTreeRoot()
		require.NoError(tt, err)
		slashings := map[[32]byte]*qrysmpb.AttesterSlashing{slashingRoot: slashing}

		err = s.processAttesterSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsContain(tt, hook, "Invalid signature")
	})

	t.Run("first_att_invalid_sig_second_valid", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use invalid signature for the first att, but valid for the second.
		signature := privKey.Sign(signingRoot[:])
		firstAtt.Signatures = [][]byte{make([]byte, 4627)}
		secondAtt.Signatures = [][]byte{signature.Marshal()}

		slashing := &qrysmpb.AttesterSlashing{
			Attestation_1: firstAtt,
			Attestation_2: secondAtt,
		}
		slashingRoot, err := slashing.HashTreeRoot()
		require.NoError(tt, err)
		slashings := map[[32]byte]*qrysmpb.AttesterSlashing{slashingRoot: slashing}

		err = s.processAttesterSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsContain(tt, hook, "Invalid signature")
	})

	t.Run("both_valid_att_signatures", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use valid signatures.
		signature := privKey.Sign(signingRoot[:])
		firstAtt.Signatures = [][]byte{signature.Marshal()}
		secondAtt.Signatures = [][]byte{signature.Marshal()}

		slashing := &qrysmpb.AttesterSlashing{
			Attestation_1: firstAtt,
			Attestation_2: secondAtt,
		}
		slashingRoot, err := slashing.HashTreeRoot()
		require.NoError(tt, err)
		slashings := map[[32]byte]*qrysmpb.AttesterSlashing{slashingRoot: slashing}

		err = s.processAttesterSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsDoNotContain(tt, hook, "Invalid signature")
	})
}

func TestService_processProposerSlashings(t *testing.T) {
	ctx := context.Background()
	slasherDB := dbtest.SetupSlasherDB(t)
	beaconDB := dbtest.SetupDB(t)

	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	privKey, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	validators := make([]*qrysmpb.Validator, 1)
	validators[0] = &qrysmpb.Validator{
		PublicKey:             privKey.PublicKey().Marshal(),
		WithdrawalCredentials: make([]byte, 64),
		EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
	}
	err = beaconState.SetValidators(validators)
	require.NoError(t, err)

	mockChain := &mock.ChainService{
		State: beaconState,
	}
	s := &Service{
		serviceCfg: &ServiceConfig{
			Database:                slasherDB,
			AttestationStateFetcher: mockChain,
			StateGen:                stategen.New(beaconDB, doublylinkedtree.New()),
			SlashingPoolInserter:    &slashingsmock.PoolMock{},
			HeadStateFetcher:        mockChain,
		},
	}

	parentRoot := bytesutil.ToBytes32([]byte("parent"))
	err = s.serviceCfg.StateGen.SaveState(ctx, parentRoot, beaconState)
	require.NoError(t, err)

	firstBlockHeader := util.HydrateSignedBeaconHeader(&qrysmpb.SignedBeaconBlockHeader{
		Header: &qrysmpb.BeaconBlockHeader{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    parentRoot[:],
		},
	})
	secondBlockHeader := util.HydrateSignedBeaconHeader(&qrysmpb.SignedBeaconBlockHeader{
		Header: &qrysmpb.BeaconBlockHeader{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    parentRoot[:],
		},
	})

	domain, err := signing.Domain(
		beaconState.Fork(),
		0,
		params.BeaconConfig().DomainBeaconProposer,
		beaconState.GenesisValidatorsRoot(),
	)
	require.NoError(t, err)
	htr, err := firstBlockHeader.Header.HashTreeRoot()
	require.NoError(t, err)
	container := &qrysmpb.SigningData{
		ObjectRoot: htr[:],
		Domain:     domain,
	}
	require.NoError(t, err)
	signingRoot, err := container.HashTreeRoot()
	require.NoError(t, err)

	t.Run("first_header_valid_sig_second_invalid", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use valid signature for the first header, but bad one for the second.
		signature := privKey.Sign(signingRoot[:])
		firstBlockHeader.Signature = signature.Marshal()
		secondBlockHeader.Signature = make([]byte, 96)

		slashings := []*qrysmpb.ProposerSlashing{
			{
				Header_1: firstBlockHeader,
				Header_2: secondBlockHeader,
			},
		}

		err = s.processProposerSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsContain(tt, hook, "Invalid signature")
	})

	t.Run("first_header_invalid_sig_second_valid", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use invalid signature for the first header, but valid for the second.
		signature := privKey.Sign(signingRoot[:])
		firstBlockHeader.Signature = make([]byte, 96)
		secondBlockHeader.Signature = signature.Marshal()

		slashings := []*qrysmpb.ProposerSlashing{
			{
				Header_1: firstBlockHeader,
				Header_2: secondBlockHeader,
			},
		}

		err = s.processProposerSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsContain(tt, hook, "Invalid signature")
	})

	t.Run("both_valid_header_signatures", func(tt *testing.T) {
		hook := logTest.NewGlobal()
		// Use valid signatures.
		signature := privKey.Sign(signingRoot[:])
		firstBlockHeader.Signature = signature.Marshal()
		secondBlockHeader.Signature = signature.Marshal()

		slashings := []*qrysmpb.ProposerSlashing{
			{
				Header_1: firstBlockHeader,
				Header_2: secondBlockHeader,
			},
		}

		err = s.processProposerSlashings(ctx, slashings)
		require.NoError(tt, err)
		require.LogsDoNotContain(tt, hook, "Invalid signature")
	})
}
