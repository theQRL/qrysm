package client

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-qrllib/common"
	dilithiumlib "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	lruwrpr "github.com/theQRL/qrysm/v4/cache/lru"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	blocktest "github.com/theQRL/qrysm/v4/consensus-types/blocks/testing"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	validatormock "github.com/theQRL/qrysm/v4/testing/validator-mock"
	testing2 "github.com/theQRL/qrysm/v4/validator/db/testing"
	"github.com/theQRL/qrysm/v4/validator/graffiti"
)

type mocks struct {
	validatorClient *validatormock.MockValidatorClient
	nodeClient      *validatormock.MockNodeClient
	signfunc        func(context.Context, *validatorpb.SignRequest) (dilithium.Signature, error)
}

type mockSignature struct{}

func (mockSignature) Verify(dilithium.PublicKey, []byte) bool {
	return true
}
func (mockSignature) AggregateVerify([]dilithium.PublicKey, [][32]byte) bool {
	return true
}
func (mockSignature) FastAggregateVerify([]dilithium.PublicKey, [32]byte) bool {
	return true
}
func (mockSignature) Eth2FastAggregateVerify([]dilithium.PublicKey, [32]byte) bool {
	return true
}
func (mockSignature) Marshal() []byte {
	return make([]byte, 32)
}
func (m mockSignature) Copy() dilithium.Signature {
	return m
}

func testKeyFromBytes(t *testing.T, b []byte) keypair {
	pri, err := dilithium.SecretKeyFromBytes(bytesutil.PadTo(b, common.SeedSize))
	require.NoError(t, err, "Failed to generate key from bytes")
	return keypair{pub: bytesutil.ToBytes2592(pri.PublicKey().Marshal()), pri: pri}
}

func setup(t *testing.T) (*validator, *mocks, dilithium.DilithiumKey, func()) {
	validatorKey, err := dilithium.RandKey()
	require.NoError(t, err)
	return setupWithKey(t, validatorKey)
}

func setupWithKey(t *testing.T, validatorKey dilithium.DilithiumKey) (*validator, *mocks, dilithium.DilithiumKey, func()) {
	var pubKey [dilithiumlib.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	valDB := testing2.SetupDB(t, [][dilithiumlib.CryptoPublicKeyBytes]byte{pubKey})
	ctrl := gomock.NewController(t)
	m := &mocks{
		validatorClient: validatormock.NewMockValidatorClient(ctrl),
		nodeClient:      validatormock.NewMockNodeClient(ctrl),
		signfunc: func(ctx context.Context, req *validatorpb.SignRequest) (dilithium.Signature, error) {
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
	var pubKey [dilithiumlib.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.ProposeBlock(context.Background(), 0, pubKey)

	require.LogsContain(t, hook, "Assigned to genesis slot, skipping proposal")
}

func TestProposeBlock_DomainDataFailed(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	var pubKey [dilithiumlib.CryptoPublicKeyBytes]byte
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
	var pubKey [dilithiumlib.CryptoPublicKeyBytes]byte
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
			name: "capella",
			slot: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t)
			defer finish()
			var pubKey [dilithiumlib.CryptoPublicKeyBytes]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
			).Return(nil, errors.New("uh oh"))

			validator.ProposeBlock(context.Background(), tt.slot, pubKey)
			require.LogsContain(t, hook, "Failed to request block from beacon node")
		})
	}
}

func TestProposeBlock_ProposeBlockFailed(t *testing.T) {
	tests := []struct {
		name  string
		block *zondpb.GenericBeaconBlock
	}{
		{
			name: "capella",
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Capella{
					Capella: util.NewBeaconBlockCapella().Block,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t)
			defer finish()
			var pubKey [dilithiumlib.CryptoPublicKeyBytes]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
			).Return(tt.block, nil /*err*/)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.GenericSignedBeaconBlock{}),
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
		blocks []*zondpb.GenericBeaconBlock
	}{
		{
			name: "capella",
			blocks: func() []*zondpb.GenericBeaconBlock {
				block0, block1 := util.NewBeaconBlockCapella(), util.NewBeaconBlockCapella()
				block1.Block.Body.Graffiti = blockGraffiti[:]

				var blocks []*zondpb.GenericBeaconBlock
				for _, block := range []*zondpb.SignedBeaconBlockCapella{block0, block1} {
					block.Block.Slot = slot
					blocks = append(blocks, &zondpb.GenericBeaconBlock{
						Block: &zondpb.GenericBeaconBlock_Capella{
							Capella: block.Block,
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
			var pubKey [dilithiumlib.CryptoPublicKeyBytes]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			var dummyRoot [32]byte
			// Save a dummy proposal history at slot 0.
			err := validator.db.SaveProposalHistoryForSlot(context.Background(), pubKey, 0, dummyRoot[:])
			require.NoError(t, err)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(1).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
			).Return(tt.blocks[0], nil /*err*/)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
			).Return(tt.blocks[1], nil /*err*/)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(3).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.GenericSignedBeaconBlock{}),
			).Return(&zondpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil /*error*/)

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
	var pubKey [dilithiumlib.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())

	var dummyRoot [32]byte
	// Save a dummy proposal history at slot 0.
	err := validator.db.SaveProposalHistoryForSlot(context.Background(), pubKey, 0, dummyRoot[:])
	require.NoError(t, err)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(1).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	testBlock := util.NewBeaconBlockCapella()
	farFuture := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().WeakSubjectivityPeriod + 9))
	testBlock.Block.Slot = farFuture
	m.validatorClient.EXPECT().GetBeaconBlock(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
	).Return(&zondpb.GenericBeaconBlock{
		Block: &zondpb.GenericBeaconBlock_Capella{
			Capella: testBlock.Block,
		},
	}, nil /*err*/)

	secondTestBlock := util.NewBeaconBlockCapella()
	secondTestBlock.Block.Slot = farFuture
	var blockGraffiti [32]byte
	copy(blockGraffiti[:], "someothergraffiti")
	secondTestBlock.Block.Body.Graffiti = blockGraffiti[:]
	m.validatorClient.EXPECT().GetBeaconBlock(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
	).Return(&zondpb.GenericBeaconBlock{
		Block: &zondpb.GenericBeaconBlock_Capella{
			Capella: secondTestBlock.Block,
		},
	}, nil /*err*/)
	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(3).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	m.validatorClient.EXPECT().ProposeBeaconBlock(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.GenericSignedBeaconBlock{}),
	).Return(&zondpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil /*error*/)

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
			var pubKey [dilithiumlib.CryptoPublicKeyBytes]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			// Save a dummy proposal history at slot 0.
			err := validator.db.SaveProposalHistoryForSlot(context.Background(), pubKey, 0, []byte{})
			require.NoError(t, err)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(2).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			blk := util.NewBeaconBlockCapella()
			blk.Block.Slot = slot
			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
			).Return(&zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Capella{
					Capella: blk.Block,
				},
			}, nil /*err*/)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(2).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().ProposeBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.GenericSignedBeaconBlock{}),
			).Times(2).Return(&zondpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil /*error*/)

			validator.ProposeBlock(context.Background(), slot, pubKey)
			require.LogsDoNotContain(t, hook, failedBlockSignLocalErr)

			blk2 := util.NewBeaconBlockCapella()
			blk2.Block.Slot = tt.pastSlot
			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
			).Return(&zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Capella{
					Capella: blk2.Block,
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
		block   *zondpb.GenericBeaconBlock
		version int
	}{
		{
			name: "capella",
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Capella{
					Capella: func() *zondpb.BeaconBlockCapella {
						blk := util.NewBeaconBlockCapella()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name: "capella blind block",
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_BlindedCapella{
					BlindedCapella: func() *zondpb.BlindedBeaconBlockCapella {
						blk := util.NewBlindedBeaconBlockCapella()
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
			var pubKey [dilithiumlib.CryptoPublicKeyBytes]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			validator.graffiti = graffiti

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
			).DoAndReturn(func(ctx context.Context, req *zondpb.BlockRequest) (*zondpb.GenericBeaconBlock, error) {
				assert.DeepEqual(t, graffiti, req.Graffiti, "Unexpected graffiti in request")

				return tt.block, nil
			})

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			var sentBlock interfaces.ReadOnlySignedBeaconBlock
			var err error

			m.validatorClient.EXPECT().ProposeBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.GenericSignedBeaconBlock{}),
			).DoAndReturn(func(ctx context.Context, block *zondpb.GenericSignedBeaconBlock) (*zondpb.ProposeResponse, error) {
				sentBlock, err = blocktest.NewSignedBeaconBlockFromGeneric(block)
				// assert.NoError(t, err, "Unexpected error unwrapping block")
				require.NoError(t, err)
				return &zondpb.ProposeResponse{BlockRoot: make([]byte, 32)}, nil
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
		Return(&zondpb.ValidatorIndexResponse{Index: 1}, nil)

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
		Return(&zondpb.ValidatorIndexResponse{Index: 1}, nil)

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
		Return(&zondpb.ValidatorIndexResponse{Index: 1}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	m.validatorClient.EXPECT().
		ProposeExit(gomock.Any(), gomock.AssignableToTypeOf(&zondpb.SignedVoluntaryExit{})).
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
		Return(&zondpb.ValidatorIndexResponse{Index: 1}, nil)

	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil)

	m.validatorClient.EXPECT().
		ProposeExit(gomock.Any(), gomock.AssignableToTypeOf(&zondpb.SignedVoluntaryExit{})).
		Return(&zondpb.ProposeExitResponse{}, nil)

	assert.NoError(t, ProposeExit(
		context.Background(),
		m.validatorClient,
		m.signfunc,
		validatorKey.PublicKey().Marshal(),
		params.BeaconConfig().GenesisEpoch,
	))
}

func TestSignCapellaBlock(t *testing.T) {
	validator, m, _, finish := setup(t)
	defer finish()

	proposerDomain := make([]byte, 32)
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&zondpb.DomainResponse{SignatureDomain: proposerDomain}, nil)

	ctx := context.Background()
	blk := util.NewBeaconBlockCapella()
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
	pubKey := [dilithiumlib.CryptoPublicKeyBytes]byte{'a'}
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
					ValidatorIndex(gomock.Any(), &zondpb.ValidatorIndexRequest{PublicKey: pubKey[:]}).
					Return(&zondpb.ValidatorIndexResponse{Index: 2}, nil)
			}
			got, err := tt.v.getGraffiti(context.Background(), pubKey)
			require.NoError(t, err)
			require.DeepEqual(t, tt.want, got)
		})
	}
}

func TestGetGraffitiOrdered_Ok(t *testing.T) {
	pubKey := [dilithiumlib.CryptoPublicKeyBytes]byte{'a'}
	valDB := testing2.SetupDB(t, [][dilithiumlib.CryptoPublicKeyBytes]byte{pubKey})
	ctrl := gomock.NewController(t)
	m := &mocks{
		validatorClient: validatormock.NewMockValidatorClient(ctrl),
	}
	m.validatorClient.EXPECT().
		ValidatorIndex(gomock.Any(), &zondpb.ValidatorIndexRequest{PublicKey: pubKey[:]}).
		Times(5).
		Return(&zondpb.ValidatorIndexResponse{Index: 2}, nil)

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
