package client

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"google.golang.org/protobuf/types/known/emptypb"
)

func forceSyncCommitteeAggregatorSelection(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.BeaconConfig().Copy()
	cfg.TargetAggregatorsPerSyncSubcommittee = cfg.SyncCommitteeSize / cfg.SyncCommitteeSubnetCount
	params.OverrideBeaconConfig(cfg)
}

func TestSubmitSyncCommitteeMessage_ValidatorDutiesRequestFailure(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{}}
	defer finish()

	m.validatorClient.EXPECT().GetSyncMessageBlockRoot(
		gomock.Any(), // ctx
		&emptypb.Empty{},
	).Return(&qrysmpb.SyncMessageBlockRootResponse{
		Root: bytesutil.PadTo([]byte{}, 32),
	}, nil)

	var pubKey [field_params.MLDSA87PubkeyLength]byte
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
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
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
	).Return(&qrysmpb.SyncMessageBlockRootResponse{
		Root: bytesutil.PadTo(r, 32),
	}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("uh oh"))

	var pubKey [field_params.MLDSA87PubkeyLength]byte
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
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
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
	).Return(&qrysmpb.SyncMessageBlockRootResponse{
		Root: bytesutil.PadTo(r, 32),
	}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&qrysmpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	m.validatorClient.EXPECT().SubmitSyncMessage(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.SyncCommitteeMessage{}),
	).Return(&emptypb.Empty{}, errors.New("uh oh") /* error */)

	var pubKey [field_params.MLDSA87PubkeyLength]byte
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
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
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
	).Return(&qrysmpb.SyncMessageBlockRootResponse{
		Root: bytesutil.PadTo(r, 32),
	}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&qrysmpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	var generatedMsg *qrysmpb.SyncCommitteeMessage
	m.validatorClient.EXPECT().SubmitSyncMessage(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.SyncCommitteeMessage{}),
	).Do(func(_ context.Context, msg *qrysmpb.SyncCommitteeMessage) {
		generatedMsg = msg
	}).Return(&emptypb.Empty{}, nil /* error */)

	var pubKey [field_params.MLDSA87PubkeyLength]byte
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
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{}}
	defer finish()

	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Could not fetch validator assignment")
}

func TestSubmitSignedContributionAndProof_GetSyncSubcommitteeIndexFailure(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&qrysmpb.SyncSubcommitteeIndexRequest{
			Slot:      1,
			PublicKey: pubKey[:],
		},
	).Return(&qrysmpb.SyncSubcommitteeIndexResponse{}, errors.New("Bad index"))

	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Could not get sync subcommittee index")
}

func TestSubmitSignedContributionAndProof_NothingToDo(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&qrysmpb.SyncSubcommitteeIndexRequest{
			Slot:      1,
			PublicKey: pubKey[:],
		},
	).Return(&qrysmpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{}}, nil)

	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Empty subcommittee index list, do nothing")
}

func TestSubmitSignedContributionAndProof_BadDomain(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&qrysmpb.SyncSubcommitteeIndexRequest{
			Slot:      1,
			PublicKey: pubKey[:],
		},
	).Return(&qrysmpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{1}}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&qrysmpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, errors.New("bad domain response"))

	validator.SubmitSignedContributionAndProof(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Could not get selection proofs")
	require.LogsContain(t, hook, "bad domain response")
}

func TestSubmitSignedContributionAndProof_CouldNotGetContribution(t *testing.T) {
	forceSyncCommitteeAggregatorSelection(t)
	hook := logTest.NewGlobal()
	slot := primitives.Slot(10)
	// Use a fixed secret key so the validator public key is stable in mock expectations.
	rawKey, err := hex.DecodeString("659e875e1b062c03f2f2a57332974d475b97df6cfc581d322e79642d39aca8fd659e875e1b062c03f2f2a57332974d4a")
	assert.NoError(t, err)
	validatorKey, err := ml_dsa_87.SecretKeyFromSeed(rawKey)
	assert.NoError(t, err)

	validator, m, validatorKey, finish := setupWithKey(t, validatorKey)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&qrysmpb.SyncSubcommitteeIndexRequest{
			Slot:      slot,
			PublicKey: pubKey[:],
		},
	).Return(&qrysmpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{1}}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&qrysmpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	m.validatorClient.EXPECT().GetSyncCommitteeContribution(
		gomock.Any(), // ctx
		&qrysmpb.SyncCommitteeContributionRequest{
			Slot:      slot,
			PublicKey: pubKey[:],
			SubnetId:  0,
		},
	).Return(nil, errors.New("Bad contribution"))

	validator.SubmitSignedContributionAndProof(context.Background(), slot, pubKey)
	require.LogsContain(t, hook, "Could not get sync committee contribution")
}

func TestSubmitSignedContributionAndProof_CouldNotSubmitContribution(t *testing.T) {
	forceSyncCommitteeAggregatorSelection(t)
	hook := logTest.NewGlobal()
	slot := primitives.Slot(10)
	// Use a fixed secret key so the validator public key is stable in mock expectations.
	rawKey, err := hex.DecodeString("659e875e1b062c03f2f2a57332974d475b97df6cfc581d322e79642d39aca8fd659e875e1b062c03f2f2a57332974d4a")
	assert.NoError(t, err)
	validatorKey, err := ml_dsa_87.SecretKeyFromSeed(rawKey)
	assert.NoError(t, err)

	validator, m, validatorKey, finish := setupWithKey(t, validatorKey)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&qrysmpb.SyncSubcommitteeIndexRequest{
			Slot:      slot,
			PublicKey: pubKey[:],
		},
	).Return(&qrysmpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{1}}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&qrysmpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	aggBits := bitfield.NewBitvector128()
	aggBits.SetBitAt(0, true)
	m.validatorClient.EXPECT().GetSyncCommitteeContribution(
		gomock.Any(), // ctx
		&qrysmpb.SyncCommitteeContributionRequest{
			Slot:      slot,
			PublicKey: pubKey[:],
			SubnetId:  0,
		},
	).Return(&qrysmpb.SyncCommitteeContribution{
		BlockRoot:       make([]byte, field_params.RootLength),
		Signatures:      [][]byte{},
		AggregationBits: aggBits,
	}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&qrysmpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	m.validatorClient.EXPECT().SubmitSignedContributionAndProof(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.SignedContributionAndProof{
			Message: &qrysmpb.ContributionAndProof{
				AggregatorIndex: 7,
				Contribution: &qrysmpb.SyncCommitteeContribution{
					BlockRoot:         make([]byte, field_params.RootLength),
					Signatures:        [][]byte{},
					AggregationBits:   bitfield.NewBitvector128(),
					Slot:              slot,
					SubcommitteeIndex: 1,
				},
			},
		}),
	).Return(&emptypb.Empty{}, errors.New("Could not submit contribution"))

	validator.SubmitSignedContributionAndProof(context.Background(), slot, pubKey)
	require.LogsContain(t, hook, "Could not submit signed contribution and proof")
}

func TestSubmitSignedContributionAndProof_Ok(t *testing.T) {
	forceSyncCommitteeAggregatorSelection(t)
	slot := primitives.Slot(10)
	// Use a fixed secret key so the validator public key is stable in mock expectations.
	rawKey, err := hex.DecodeString("659e875e1b062c03f2f2a57332974d475b97df6cfc581d322e79642d39aca8fd659e875e1b062c03f2f2a57332974d4a")
	assert.NoError(t, err)
	validatorKey, err := ml_dsa_87.SecretKeyFromSeed(rawKey)
	assert.NoError(t, err)

	validator, m, validatorKey, finish := setupWithKey(t, validatorKey)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&qrysmpb.SyncSubcommitteeIndexRequest{
			Slot:      slot,
			PublicKey: pubKey[:],
		},
	).Return(&qrysmpb.SyncSubcommitteeIndexResponse{Indices: []primitives.CommitteeIndex{1}}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&qrysmpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	aggBits := bitfield.NewBitvector128()
	aggBits.SetBitAt(0, true)
	m.validatorClient.EXPECT().GetSyncCommitteeContribution(
		gomock.Any(), // ctx
		&qrysmpb.SyncCommitteeContributionRequest{
			Slot:      slot,
			PublicKey: pubKey[:],
			SubnetId:  0,
		},
	).Return(&qrysmpb.SyncCommitteeContribution{
		BlockRoot:       make([]byte, field_params.RootLength),
		Signatures:      [][]byte{},
		AggregationBits: aggBits,
	}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), // ctx
			gomock.Any()). // epoch
		Return(&qrysmpb.DomainResponse{
			SignatureDomain: make([]byte, 32),
		}, nil)

	m.validatorClient.EXPECT().SubmitSignedContributionAndProof(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.SignedContributionAndProof{
			Message: &qrysmpb.ContributionAndProof{
				AggregatorIndex: 7,
				Contribution: &qrysmpb.SyncCommitteeContribution{
					BlockRoot:         make([]byte, 32),
					Signatures:        [][]byte{},
					AggregationBits:   bitfield.NewBitvector128(),
					Slot:              slot,
					SubcommitteeIndex: 1,
				},
			},
		}),
	).Return(&emptypb.Empty{}, nil)

	validator.SubmitSignedContributionAndProof(context.Background(), slot, pubKey)
}

func TestSubmitSignedContributionAndProof_OncePerPubkeyAndSubcommittee(t *testing.T) {
	forceSyncCommitteeAggregatorSelection(t)
	slot := primitives.Slot(10)
	rawKey, err := hex.DecodeString("659e875e1b062c03f2f2a57332974d475b97df6cfc581d322e79642d39aca8fd659e875e1b062c03f2f2a57332974d4a")
	assert.NoError(t, err)
	validatorKey, err := ml_dsa_87.SecretKeyFromSeed(rawKey)
	assert.NoError(t, err)

	validator, m, validatorKey, finish := setupWithKey(t, validatorKey)
	validatorIndex := primitives.ValidatorIndex(7)
	committee := []primitives.ValidatorIndex{0, 3, 4, 2, validatorIndex, 6, 8, 9, 10}
	validator.duties = &qrysmpb.DutiesResponse{CurrentEpochDuties: []*qrysmpb.DutiesResponse_Duty{
		{
			PublicKey:      validatorKey.PublicKey().Marshal(),
			Committee:      committee,
			ValidatorIndex: validatorIndex,
		},
	}}
	defer finish()

	// Validator selected twice; with SyncCommitteeSubnetCount=1 both fall in subnet 0.
	aggregatorCommitteeIndices := []primitives.CommitteeIndex{1, 2}
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	m.validatorClient.EXPECT().GetSyncSubcommitteeIndex(
		gomock.Any(), // ctx
		&qrysmpb.SyncSubcommitteeIndexRequest{
			Slot:      slot,
			PublicKey: pubKey[:],
		},
	).Return(&qrysmpb.SyncSubcommitteeIndexResponse{Indices: aggregatorCommitteeIndices}, nil)

	// Two selection proofs are computed (one per index).
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Times(2).
		Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	aggBits := bitfield.NewBitvector128()
	aggBits.SetBitAt(0, true)
	// Contribution fetched only once for subnet 0, despite two selections.
	m.validatorClient.EXPECT().GetSyncCommitteeContribution(
		gomock.Any(), // ctx
		&qrysmpb.SyncCommitteeContributionRequest{
			Slot:      slot,
			PublicKey: pubKey[:],
			SubnetId:  0,
		},
	).Return(&qrysmpb.SyncCommitteeContribution{
		BlockRoot:       make([]byte, field_params.RootLength),
		Signatures:      [][]byte{},
		AggregationBits: aggBits,
	}, nil)

	// One DomainData call for signing the single ContributionAndProof.
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	// Submit must happen exactly once.
	m.validatorClient.EXPECT().SubmitSignedContributionAndProof(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.SignedContributionAndProof{
			Message: &qrysmpb.ContributionAndProof{
				AggregatorIndex: validatorIndex,
				Contribution: &qrysmpb.SyncCommitteeContribution{
					BlockRoot:         make([]byte, 32),
					Signatures:        [][]byte{},
					AggregationBits:   bitfield.NewBitvector128(),
					Slot:              slot,
					SubcommitteeIndex: 0,
				},
			},
		}),
	).Return(&emptypb.Empty{}, nil)

	validator.SubmitSignedContributionAndProof(context.Background(), slot, pubKey)
}
