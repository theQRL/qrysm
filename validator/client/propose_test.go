package client

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	lruwrpr "github.com/theQRL/qrysm/cache/lru"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	blocktest "github.com/theQRL/qrysm/consensus-types/blocks/testing"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	validatormock "github.com/theQRL/qrysm/testing/validator-mock"
	testing2 "github.com/theQRL/qrysm/validator/db/testing"
	"github.com/theQRL/qrysm/validator/graffiti"
)

type mocks struct {
	validatorClient *validatormock.MockValidatorClient
	nodeClient      *validatormock.MockNodeClient
	signfunc        func(context.Context, *validatorpb.SignRequest) (ml_dsa_87.Signature, error)
}

type mockSignature struct{}

func (mockSignature) Verify(ml_dsa_87.PublicKey, []byte) bool {
	return true
}
func (mockSignature) Marshal() []byte {
	return make([]byte, 32)
}
func (m mockSignature) Copy() ml_dsa_87.Signature {
	return m
}

func testKeyFromBytes(t *testing.T, b []byte) keypair {
	pri, err := ml_dsa_87.SecretKeyFromSeed(bytesutil.PadTo(b, field_params.MLDSA87SeedLength))
	require.NoError(t, err, "Failed to generate key from bytes")
	return keypair{pub: bytesutil.ToBytes2592(pri.PublicKey().Marshal()), pri: pri}
}

func setup(t *testing.T) (*validator, *mocks, ml_dsa_87.MLDSA87Key, func()) {
	validatorKey, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	return setupWithKey(t, validatorKey)
}

func setupWithKey(t *testing.T, validatorKey ml_dsa_87.MLDSA87Key) (*validator, *mocks, ml_dsa_87.MLDSA87Key, func()) {
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	valDB := testing2.SetupDB(t, [][field_params.MLDSA87PubkeyLength]byte{pubKey})
	ctrl := gomock.NewController(t)
	m := &mocks{
		validatorClient: validatormock.NewMockValidatorClient(ctrl),
		nodeClient:      validatormock.NewMockNodeClient(ctrl),
		signfunc: func(ctx context.Context, req *validatorpb.SignRequest) (ml_dsa_87.Signature, error) {
			return mockSignature{}, nil
		},
	}
	aggregatedSlotCommitteeIDCache := lruwrpr.New(int(params.BeaconConfig().MaxCommitteesPerSlot))

	validator := &validator{
		db:                             valDB,
		keyManager:                     newMockKeymanager(t, keypair{pub: pubKey, pri: validatorKey}),
		validatorClient:                m.validatorClient,
		graffiti:                       []byte{},
		attLogs:                        make(map[[32]byte]*attSubmitted),
		aggregatedSlotCommitteeIDCache: aggregatedSlotCommitteeIDCache,
	}

	return validator, m, validatorKey, ctrl.Finish
}

func TestProposeBlock_DoesNotProposeGenesisBlock(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, _, validatorKey, finish := setup(t)
	defer finish()
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.ProposeBlock(context.Background(), 0, pubKey)

	require.LogsContain(t, hook, "Assigned to genesis slot, skipping proposal")
}

func TestProposeBlock_DomainDataFailed(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Return(nil /*response*/, errors.New("uh oh"))

	validator.ProposeBlock(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, "Failed to sign randao reveal")
}

func TestProposeBlock_DomainDataIsNil(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Return(nil /*response*/, nil)

	validator.ProposeBlock(context.Background(), 1, pubKey)
	require.LogsContain(t, hook, domainDataErr)
}

func TestProposeBlock_RequestBlockFailed(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	tests := []struct {
		name string
		slot primitives.Slot
	}{
		{
			name: "zond",
			slot: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t)
			defer finish()
			var pubKey [field_params.MLDSA87PubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.BlockRequest{}),
			).Return(nil, errors.New("uh oh"))

			validator.ProposeBlock(context.Background(), tt.slot, pubKey)
			require.LogsContain(t, hook, "Failed to request block from beacon node")
		})
	}
}

func TestProposeBlock_ProposeBlockFailed(t *testing.T) {
	tests := []struct {
		name  string
		block *qrysmpb.GenericBeaconBlock
	}{
		{
			name: "zond",
			block: &qrysmpb.GenericBeaconBlock{
				Block: &qrysmpb.GenericBeaconBlock_Zond{
					Zond: util.NewBeaconBlockZond().Block,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t)
			defer finish()
			var pubKey [field_params.MLDSA87PubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.BlockRequest{}),
			).Return(tt.block, nil /*err*/)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.GenericSignedBeaconBlock{}),
			).Return(nil /*response*/, errors.New("uh oh"))

			validator.ProposeBlock(context.Background(), 1, pubKey)
			require.LogsContain(t, hook, "Failed to propose block")
		})
	}
}

func TestProposeBlock_BlocksDoubleProposal(t *testing.T) {
	slot := params.BeaconConfig().SlotsPerEpoch.Mul(5).Add(2)
	var blockGraffiti [32]byte
	copy(blockGraffiti[:], "someothergraffiti")

	tests := []struct {
		name   string
		blocks []*qrysmpb.GenericBeaconBlock
	}{
		{
			name: "zond",
			blocks: func() []*qrysmpb.GenericBeaconBlock {
				block0, block1 := util.NewBeaconBlockZond(), util.NewBeaconBlockZond()
				block1.Block.Body.Graffiti = blockGraffiti[:]

				var blocks []*qrysmpb.GenericBeaconBlock
				for _, block := range []*qrysmpb.SignedBeaconBlockZond{block0, block1} {
					block.Block.Slot = slot
					blocks = append(blocks, &qrysmpb.GenericBeaconBlock{
						Block: &qrysmpb.GenericBeaconBlock_Zond{
							Zond: block.Block,
						},
					})
				}
				return blocks
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t)
			defer finish()
			var pubKey [field_params.MLDSA87PubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			var dummyRoot [32]byte
			// Save a dummy proposal history at slot 0.
			err := validator.db.SaveProposalHistoryForSlot(context.Background(), pubKey, 0, dummyRoot[:])
			require.NoError(t, err)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(1).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.BlockRequest{}),
			).Return(tt.blocks[0], nil /*err*/)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.BlockRequest{}),
			).Return(tt.blocks[1], nil /*err*/)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(3).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.GenericSignedBeaconBlock{}),
			).Return(&qrysmpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil /*error*/)

			validator.ProposeBlock(context.Background(), slot, pubKey)
			require.LogsDoNotContain(t, hook, failedBlockSignLocalErr)

			validator.ProposeBlock(context.Background(), slot, pubKey)
			require.LogsContain(t, hook, failedBlockSignLocalErr)
		})
	}
}

func TestProposeBlock_BlocksDoubleProposal_After54KEpochs(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	var pubKey [field_params.MLDSA87PubkeyLength]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())

	var dummyRoot [32]byte
	// Save a dummy proposal history at slot 0.
	err := validator.db.SaveProposalHistoryForSlot(context.Background(), pubKey, 0, dummyRoot[:])
	require.NoError(t, err)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(1).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	testBlock := util.NewBeaconBlockZond()
	farFuture := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().WeakSubjectivityPeriod + 9))
	testBlock.Block.Slot = farFuture
	m.validatorClient.EXPECT().GetBeaconBlock(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.BlockRequest{}),
	).Return(&qrysmpb.GenericBeaconBlock{
		Block: &qrysmpb.GenericBeaconBlock_Zond{
			Zond: testBlock.Block,
		},
	}, nil /*err*/)

	secondTestBlock := util.NewBeaconBlockZond()
	secondTestBlock.Block.Slot = farFuture
	var blockGraffiti [32]byte
	copy(blockGraffiti[:], "someothergraffiti")
	secondTestBlock.Block.Body.Graffiti = blockGraffiti[:]
	m.validatorClient.EXPECT().GetBeaconBlock(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.BlockRequest{}),
	).Return(&qrysmpb.GenericBeaconBlock{
		Block: &qrysmpb.GenericBeaconBlock_Zond{
			Zond: secondTestBlock.Block,
		},
	}, nil /*err*/)
	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(3).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().ProposeBeaconBlock(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&qrysmpb.GenericSignedBeaconBlock{}),
	).Return(&qrysmpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil /*error*/)

	validator.ProposeBlock(context.Background(), farFuture, pubKey)
	require.LogsDoNotContain(t, hook, failedBlockSignLocalErr)

	validator.ProposeBlock(context.Background(), farFuture, pubKey)
	require.LogsContain(t, hook, failedBlockSignLocalErr)
}

func TestProposeBlock_AllowsPastProposals(t *testing.T) {
	slot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().WeakSubjectivityPeriod + 9))

	tests := []struct {
		name     string
		pastSlot primitives.Slot
	}{
		{
			name:     "400 slots ago",
			pastSlot: slot.Sub(400),
		},
		{
			name:     "same epoch",
			pastSlot: slot.Sub(4),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t)
			defer finish()
			var pubKey [field_params.MLDSA87PubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			// Save a dummy proposal history at slot 0.
			err := validator.db.SaveProposalHistoryForSlot(context.Background(), pubKey, 0, []byte{})
			require.NoError(t, err)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(2).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			blk := util.NewBeaconBlockZond()
			blk.Block.Slot = slot
			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.BlockRequest{}),
			).Return(&qrysmpb.GenericBeaconBlock{
				Block: &qrysmpb.GenericBeaconBlock_Zond{
					Zond: blk.Block,
				},
			}, nil /*err*/)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(2).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.GenericSignedBeaconBlock{}),
			).Times(2).Return(&qrysmpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil /*error*/)

			validator.ProposeBlock(context.Background(), slot, pubKey)
			require.LogsDoNotContain(t, hook, failedBlockSignLocalErr)

			blk2 := util.NewBeaconBlockZond()
			blk2.Block.Slot = tt.pastSlot
			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.BlockRequest{}),
			).Return(&qrysmpb.GenericBeaconBlock{
				Block: &qrysmpb.GenericBeaconBlock_Zond{
					Zond: blk2.Block,
				},
			}, nil /*err*/)
			validator.ProposeBlock(context.Background(), tt.pastSlot, pubKey)
			require.LogsDoNotContain(t, hook, failedBlockSignLocalErr)
		})
	}
}

func TestProposeBlock_BroadcastsBlock(t *testing.T) {
	testProposeBlock(t, make([]byte, 32) /*graffiti*/)
}

func TestProposeBlock_BroadcastsBlock_WithGraffiti(t *testing.T) {
	blockGraffiti := []byte("12345678901234567890123456789012")
	testProposeBlock(t, blockGraffiti)
}

func testProposeBlock(t *testing.T, graffiti []byte) {
	tests := []struct {
		name    string
		block   *qrysmpb.GenericBeaconBlock
		version int
	}{
		{
			name: "zond",
			block: &qrysmpb.GenericBeaconBlock{
				Block: &qrysmpb.GenericBeaconBlock_Zond{
					Zond: func() *qrysmpb.BeaconBlockZond {
						blk := util.NewBeaconBlockZond()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name: "zond blind block",
			block: &qrysmpb.GenericBeaconBlock{
				Block: &qrysmpb.GenericBeaconBlock_BlindedZond{
					BlindedZond: func() *qrysmpb.BlindedBeaconBlockZond {
						blk := util.NewBlindedBeaconBlockZond()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t)
			defer finish()
			var pubKey [field_params.MLDSA87PubkeyLength]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			validator.graffiti = graffiti

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.BlockRequest{}),
			).DoAndReturn(func(ctx context.Context, req *qrysmpb.BlockRequest) (*qrysmpb.GenericBeaconBlock, error) {
				assert.DeepEqual(t, graffiti, req.Graffiti, "Unexpected graffiti in request")

				return tt.block, nil
			})

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			var sentBlock interfaces.ReadOnlySignedBeaconBlock
			var err error

			m.validatorClient.EXPECT().ProposeBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&qrysmpb.GenericSignedBeaconBlock{}),
			).DoAndReturn(func(ctx context.Context, block *qrysmpb.GenericSignedBeaconBlock) (*qrysmpb.ProposeResponse, error) {
				sentBlock, err = blocktest.NewSignedBeaconBlockFromGeneric(block)
				// assert.NoError(t, err, "Unexpected error unwrapping block")
				require.NoError(t, err)
				return &qrysmpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil
			})

			validator.ProposeBlock(context.Background(), 1, pubKey)
			g := sentBlock.Block().Body().Graffiti()
			assert.Equal(t, string(validator.graffiti), string(g[:]))
			require.LogsContain(t, hook, "Submitted new block")

		})
	}
}

func TestProposeExit_ValidatorIndexFailed(t *testing.T) {
	_, m, validatorKey, finish := setup(t)
	defer finish()

	m.validatorClient.EXPECT().ValidatorIndex(
		gomock.Any(),
		gomock.Any(),
	).Return(nil, errors.New("uh oh"))

	err := ProposeExit(
		context.Background(),
		m.validatorClient,
		m.signfunc,
		validatorKey.PublicKey().Marshal(),
		params.BeaconConfig().GenesisEpoch,
	)
	assert.NotNil(t, err)
	assert.ErrorContains(t, "uh oh", err)
	assert.ErrorContains(t, "gRPC call to get validator index failed", err)
}

func TestProposeExit_DomainDataFailed(t *testing.T) {
	_, m, validatorKey, finish := setup(t)
	defer finish()

	m.validatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), gomock.Any()).
		Return(&qrysmpb.ValidatorIndexResponse{Index: 1}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("uh oh"))

	err := ProposeExit(
		context.Background(),
		m.validatorClient,
		m.signfunc,
		validatorKey.PublicKey().Marshal(),
		params.BeaconConfig().GenesisEpoch,
	)
	assert.NotNil(t, err)
	assert.ErrorContains(t, domainDataErr, err)
	assert.ErrorContains(t, "uh oh", err)
	assert.ErrorContains(t, "failed to sign voluntary exit", err)
}

func TestProposeExit_DomainDataIsNil(t *testing.T) {
	_, m, validatorKey, finish := setup(t)
	defer finish()

	m.validatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), gomock.Any()).
		Return(&qrysmpb.ValidatorIndexResponse{Index: 1}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(nil, nil)

	err := ProposeExit(
		context.Background(),
		m.validatorClient,
		m.signfunc,
		validatorKey.PublicKey().Marshal(),
		params.BeaconConfig().GenesisEpoch,
	)
	assert.NotNil(t, err)
	assert.ErrorContains(t, domainDataErr, err)
	assert.ErrorContains(t, "failed to sign voluntary exit", err)
}

func TestProposeBlock_ProposeExitFailed(t *testing.T) {
	_, m, validatorKey, finish := setup(t)
	defer finish()

	m.validatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), gomock.Any()).
		Return(&qrysmpb.ValidatorIndexResponse{Index: 1}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	m.validatorClient.EXPECT().
		ProposeExit(gomock.Any(), gomock.AssignableToTypeOf(&qrysmpb.SignedVoluntaryExit{})).
		Return(nil, errors.New("uh oh"))

	err := ProposeExit(
		context.Background(),
		m.validatorClient,
		m.signfunc,
		validatorKey.PublicKey().Marshal(),
		params.BeaconConfig().GenesisEpoch,
	)
	assert.NotNil(t, err)
	assert.ErrorContains(t, "uh oh", err)
	assert.ErrorContains(t, "failed to propose voluntary exit", err)
}

func TestProposeExit_BroadcastsBlock(t *testing.T) {
	_, m, validatorKey, finish := setup(t)
	defer finish()

	m.validatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), gomock.Any()).
		Return(&qrysmpb.ValidatorIndexResponse{Index: 1}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&qrysmpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	m.validatorClient.EXPECT().
		ProposeExit(gomock.Any(), gomock.AssignableToTypeOf(&qrysmpb.SignedVoluntaryExit{})).
		Return(&qrysmpb.ProposeExitResponse{}, nil)

	assert.NoError(t, ProposeExit(
		context.Background(),
		m.validatorClient,
		m.signfunc,
		validatorKey.PublicKey().Marshal(),
		params.BeaconConfig().GenesisEpoch,
	))
}

func TestSignZondBlock(t *testing.T) {
	validator, m, _, finish := setup(t)
	defer finish()

	proposerDomain := make([]byte, 32)
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&qrysmpb.DomainResponse{SignatureDomain: proposerDomain}, nil)

	ctx := context.Background()
	blk := util.NewBeaconBlockZond()
	blk.Block.Slot = 1
	blk.Block.ProposerIndex = 100

	kp := randKeypair(t)
	validator.keyManager = newMockKeymanager(t, kp)
	wb, err := blocks.NewBeaconBlock(blk.Block)
	require.NoError(t, err)
	sig, blockRoot, err := validator.signBlock(ctx, kp.pub, 0, 0, wb)
	require.NoError(t, err, "%x,%v", sig, err)
	// Verify the returned block root matches the expected root using the proposer signature
	// domain.
	wantedBlockRoot, err := signing.ComputeSigningRoot(wb, proposerDomain)
	if err != nil {
		require.NoError(t, err)
	}
	require.DeepEqual(t, wantedBlockRoot, blockRoot)
}

func TestGetGraffiti_Ok(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := &mocks{
		validatorClient: validatormock.NewMockValidatorClient(ctrl),
	}
	pubKey := [field_params.MLDSA87PubkeyLength]byte{'a'}
	tests := []struct {
		name string
		v    *validator
		want []byte
	}{
		{name: "use default cli graffiti",
			v: &validator{
				graffiti: []byte{'b'},
				graffitiStruct: &graffiti.Graffiti{
					Default: "c",
					Random:  []string{"d", "e"},
					Specific: map[primitives.ValidatorIndex]string{
						1: "f",
						2: "g",
					},
				},
			},
			want: []byte{'b'},
		},
		{name: "use default file graffiti",
			v: &validator{
				validatorClient: m.validatorClient,
				graffitiStruct: &graffiti.Graffiti{
					Default: "c",
				},
			},
			want: []byte{'c'},
		},
		{name: "use random file graffiti",
			v: &validator{
				validatorClient: m.validatorClient,
				graffitiStruct: &graffiti.Graffiti{
					Random:  []string{"d"},
					Default: "c",
				},
			},
			want: []byte{'d'},
		},
		{name: "use validator file graffiti, has validator",
			v: &validator{
				validatorClient: m.validatorClient,
				graffitiStruct: &graffiti.Graffiti{
					Random:  []string{"d"},
					Default: "c",
					Specific: map[primitives.ValidatorIndex]string{
						1: "f",
						2: "g",
					},
				},
			},
			want: []byte{'g'},
		},
		{name: "use validator file graffiti, none specified",
			v: &validator{
				validatorClient: m.validatorClient,
				graffitiStruct:  &graffiti.Graffiti{},
			},
			want: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.name, "use default cli graffiti") {
				m.validatorClient.EXPECT().
					ValidatorIndex(gomock.Any(), &qrysmpb.ValidatorIndexRequest{PublicKey: pubKey[:]}).
					Return(&qrysmpb.ValidatorIndexResponse{Index: 2}, nil)
			}
			got, err := tt.v.getGraffiti(context.Background(), pubKey)
			require.NoError(t, err)
			require.DeepEqual(t, tt.want, got)
		})
	}
}

func TestGetGraffitiOrdered_Ok(t *testing.T) {
	pubKey := [field_params.MLDSA87PubkeyLength]byte{'a'}
	valDB := testing2.SetupDB(t, [][field_params.MLDSA87PubkeyLength]byte{pubKey})
	ctrl := gomock.NewController(t)
	m := &mocks{
		validatorClient: validatormock.NewMockValidatorClient(ctrl),
	}
	m.validatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), &qrysmpb.ValidatorIndexRequest{PublicKey: pubKey[:]}).
		Times(5).
		Return(&qrysmpb.ValidatorIndexResponse{Index: 2}, nil)

	v := &validator{
		db:              valDB,
		validatorClient: m.validatorClient,
		graffitiStruct: &graffiti.Graffiti{
			Ordered: []string{"a", "b", "c"},
			Default: "d",
		},
	}
	for _, want := range [][]byte{{'a'}, {'b'}, {'c'}, {'d'}, {'d'}} {
		got, err := v.getGraffiti(context.Background(), pubKey)
		require.NoError(t, err)
		require.DeepEqual(t, want, got)
	}
}
