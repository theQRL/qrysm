package client

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestSubmitSyncCommitteeMessage_ValidatorDutiesRequestFailure(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{}}
	defer finish()

	m.validatorClient.EXPECT().GetSyncMessageBlockRoot(
		gomock.Any(), // ctx
		&emptypb.Empty{},
	).Return(&zondpb.SyncMessageBlockRootResponse{
		Root: bytesutil.PadTo([]byte{}, 32),
	}, nil)

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitSyncCommitteeMessage(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Could not fetch validator assignment")
}

func TestSubmitSyncCommitteeMessage_BadDomainData(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	hook := logTest.NewGlobal()
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}

	r := []byte{'a'}
	m.validatorClient.EXPECT().GetSyncMessageBlockRoot(
		gomock.Any(), // ctx
		&emptypb.Empty{},
	).Return(&zondpb.SyncMessageBlockRootResponse{
		Root: bytesutil.PadTo(r, 32),
	}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("uh oh"))

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitSyncCommitteeMessage(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Could not get sync committee domain data")
}

func TestSubmitSyncCommitteeMessage_CouldNotSubmit(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	hook := logTest.NewGlobal()
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}

	r := []byte{'a'}
	m.validatorClient.EXPECT().GetSyncMessageBlockRoot(
		gomock.Any(), // ctx
		&emptypb.Empty{},
	).Return(&zondpb.SyncMessageBlockRootResponse{
		Root: bytesutil.PadTo(r, 32),
	}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&zondpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	m.validatorClient.EXPECT().SubmitSyncMessage(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.SyncCommitteeMessage{}),
	).Return(&emptypb.Empty{}, errors.New("uh oh") /* error */)

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitSyncCommitteeMessage(context.Background(), 1, pubKey)

	require.LogsContain(t, hook, "Could not submit sync committee message")
}

func TestSubmitSyncCommitteeMessage_OK(t *testing.T) {
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	hook := logTest.NewGlobal()
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}

	r := []byte{'a'}
	m.validatorClient.EXPECT().GetSyncMessageBlockRoot(
		gomock.Any(), // ctx
		&emptypb.Empty{},
	).Return(&zondpb.SyncMessageBlockRootResponse{
		Root: bytesutil.PadTo(r, 32),
	}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&zondpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	var generatedMsg *zondpb.SyncCommitteeMessage
	m.validatorClient.EXPECT().SubmitSyncMessage(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.SyncCommitteeMessage{}),
	).Do(func(_ context.Context, msg *zondpb.SyncCommitteeMessage) {
		generatedMsg = msg
	}).Return(&emptypb.Empty{}, nil /* error */)

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitSyncCommitteeMessage(context.Background(), 1, pubKey)

	require.LogsDoNotContain(t, hook, "Could not")
	require.Equal(t, primitives.Slot(1), generatedMsg.Slot)
	require.Equal(t, validatorIndex, generatedMsg.ValidatorIndex)
	require.DeepEqual(t, bytesutil.PadTo(r, 32), generatedMsg.BlockRoot)
}

func TestSubmitSignedContributionAndProof_ValidatorDutiesRequestFailure(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, _, validatorKey, finish := setup(t)
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{}}
	defer finish()

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Could not fetch validator assignment")
}

func TestSubmitSignedContributionAndProof_GetSyncSubcommitteeIndexFailure(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&zondpb.SyncSubcommitteeIndexRequest{
			Slot:      1,
			PublicKey: pubKey[:],
		},
	).Return(&zondpb.SyncSubcommitteeIndexResponse{}, errors.New("Bad index"))

	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Could not get sync subcommittee index")
}

func TestSubmitSignedContributionAndProof_NothingToDo(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&zondpb.SyncSubcommitteeIndexRequest{
			Slot:      1,
			PublicKey: pubKey[:],
		},
	).Return(&zondpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{}}, nil)

	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Empty subcommittee index list, do nothing")
}

func TestSubmitSignedContributionAndProof_BadDomain(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&zondpb.SyncSubcommitteeIndexRequest{
			Slot:      1,
			PublicKey: pubKey[:],
		},
	).Return(&zondpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{1}}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&zondpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, errors.New("bad domain response"))

	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Could not get selection proofs")
	require.LogsContain(t, hook, "bad domain response")
}

func TestSubmitSignedContributionAndProof_CouldNotGetContribution(t *testing.T) {
	hook := logTest.NewGlobal()
	// Hardcode secret key in order to have a valid aggregator signature.
	rawKey, err := hex.DecodeString("659e875e1b062c03f2f2a57332974d475b97df6cfc581d322e79642d39aca8fd659e875e1b062c03f2f2a57332974d4a")
	assert.NoError(t, err)
	validatorKey, err := dilithium.SecretKeyFromSeed(rawKey)
	assert.NoError(t, err)

	validator, m, validatorKey, finish := setupWithKey(t, validatorKey)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&zondpb.SyncSubcommitteeIndexRequest{
			Slot:      1,
			PublicKey: pubKey[:],
		},
	).Return(&zondpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{1}}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&zondpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	m.validatorClient.EXPECT().GetSyncCommitteeContribution(
		gomock.Any(), // ctx
		&zondpb.SyncCommitteeContributionRequest{
			Slot:      1,
			PublicKey: pubKey[:],
			SubnetId:  0,
		},
	).Return(nil, errors.New("Bad contribution"))

	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Could not get sync committee contribution")
}

func TestSubmitSignedContributionAndProof_CouldNotSubmitContribution(t *testing.T) {
	hook := logTest.NewGlobal()
	// Hardcode secret key in order to have a valid aggregator signature.
	rawKey, err := hex.DecodeString("659e875e1b062c03f2f2a57332974d475b97df6cfc581d322e79642d39aca8fd659e875e1b062c03f2f2a57332974d4a")
	assert.NoError(t, err)
	validatorKey, err := dilithium.SecretKeyFromSeed(rawKey)
	assert.NoError(t, err)

	validator, m, validatorKey, finish := setupWithKey(t, validatorKey)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&zondpb.SyncSubcommitteeIndexRequest{
			Slot:      1,
			PublicKey: pubKey[:],
		},
	).Return(&zondpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{1}}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&zondpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	aggBits := bitfield.NewBitvector16()
	aggBits.SetBitAt(0, true)
	m.validatorClient.EXPECT().GetSyncCommitteeContribution(
		gomock.Any(), // ctx
		&zondpb.SyncCommitteeContributionRequest{
			Slot:      1,
			PublicKey: pubKey[:],
			SubnetId:  0,
		},
	).Return(&zondpb.SyncCommitteeContribution{
		BlockRoot:       make([]byte, field_params.RootLength),
		Signatures:      [][]byte{},
		AggregationBits: aggBits,
	}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&zondpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	m.validatorClient.EXPECT().SubmitSignedContributionAndProof(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.SignedContributionAndProof{
			Message: &zondpb.ContributionAndProof{
				AggregatorIndex: 7,
				Contribution: &zondpb.SyncCommitteeContribution{
					BlockRoot:         make([]byte, field_params.RootLength),
					Signatures:        [][]byte{},
					AggregationBits:   bitfield.NewBitvector16(),
					Slot:              1,
					SubcommitteeIndex: 1,
				},
			},
		}),
	).Return(&emptypb.Empty{}, errors.New("Could not submit contribution"))

	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Could not submit signed contribution and proof")
}

func TestSubmitSignedContributionAndProof_Ok(t *testing.T) {
	// Hardcode secret key in order to have a valid aggregator signature.
	rawKey, err := hex.DecodeString("659e875e1b062c03f2f2a57332974d475b97df6cfc581d322e79642d39aca8fd659e875e1b062c03f2f2a57332974d4a")
	assert.NoError(t, err)
	validatorKey, err := dilithium.SecretKeyFromSeed(rawKey)
	assert.NoError(t, err)

	validator, m, validatorKey, finish := setupWithKey(t, validatorKey)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &zondpb.DutiesResponse{CurrentEpochDuties: []*zondpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.DilithiumPubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&zondpb.SyncSubcommitteeIndexRequest{
			Slot:      1,
			PublicKey: pubKey[:],
		},
	).Return(&zondpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{1}}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&zondpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	aggBits := bitfield.NewBitvector16()
	aggBits.SetBitAt(0, true)
	m.validatorClient.EXPECT().GetSyncCommitteeContribution(
		gomock.Any(), // ctx
		&zondpb.SyncCommitteeContributionRequest{
			Slot:      1,
			PublicKey: pubKey[:],
			SubnetId:  0,
		},
	).Return(&zondpb.SyncCommitteeContribution{
		BlockRoot:       make([]byte, field_params.RootLength),
		Signatures:      [][]byte{},
		AggregationBits: aggBits,
	}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&zondpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	m.validatorClient.EXPECT().SubmitSignedContributionAndProof(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.SignedContributionAndProof{
			Message: &zondpb.ContributionAndProof{
				AggregatorIndex: 7,
				Contribution: &zondpb.SyncCommitteeContribution{
					BlockRoot:         make([]byte, 32),
					Signatures:        [][]byte{},
					AggregationBits:   bitfield.NewBitvector16(),
					Slot:              1,
					SubcommitteeIndex: 1,
				},
			},
		}),
	).Return(&emptypb.Empty{}, nil)

	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
}
