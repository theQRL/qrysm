package client

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	logTest "github.com/sirupsen/logrus/hooks/test"
	common2 "github.com/theQRL/go-qrllib/common"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	lruwrpr "github.com/theQRL/qrysm/v4/cache/lru"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	blocktest "github.com/theQRL/qrysm/v4/consensus-types/blocks/testing"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/v4/runtime/version"
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
	pri, err := dilithium.SecretKeyFromBytes(bytesutil.PadTo(b, common2.SeedSize))
	require.NoError(t, err, "Failed to generate key from bytes")
	return keypair{pub: bytesutil.ToBytes2592(pri.PublicKey().Marshal()), pri: pri}
}

func setup(t *testing.T) (*validator, *mocks, dilithium.DilithiumKey, func()) {
	validatorKey, err := dilithium.RandKey()
	require.NoError(t, err)
	return setupWithKey(t, validatorKey)
}

func setupWithKey(t *testing.T, validatorKey dilithium.DilithiumKey) (*validator, *mocks, dilithium.DilithiumKey, func()) {
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	valDB := testing2.SetupDB(t, [][dilithium2.CryptoPublicKeyBytes]byte{pubKey})
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
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())
	validator.ProposeBlock(context.Background(), 0, pubKey)

	require.LogsContain(t, hook, "Assigned to genesis slot, skipping proposal")
}

func TestProposeBlock_DomainDataFailed(t *testing.T) {
	hook := logTest.NewGlobal()
	validator, m, validatorKey, finish := setup(t)
	defer finish()
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
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
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
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
	cfg := params.BeaconConfig().Copy()
	cfg.AltairForkEpoch = 2
	cfg.BellatrixForkEpoch = 4
	params.OverrideBeaconConfig(cfg)

	tests := []struct {
		name string
		slot primitives.Slot
	}{
		{
			name: "phase 0",
			slot: 1,
		},
		{
			name: "altair",
			slot: params.BeaconConfig().SlotsPerEpoch.Mul(uint64(cfg.AltairForkEpoch)),
		},
		{
			name: "bellatrix",
			slot: params.BeaconConfig().SlotsPerEpoch.Mul(uint64(cfg.BellatrixForkEpoch)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t)
			defer finish()
			var pubKey [dilithium2.CryptoPublicKeyBytes]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
			).Return(nil /*response*/, errors.New("uh oh"))

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
			name: "phase0",
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Phase0{
					Phase0: util.NewBeaconBlock().Block,
				},
			},
		},
		{
			name: "altair",
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Altair{
					Altair: util.NewBeaconBlockAltair().Block,
				},
			},
		},
		{
			name: "bellatrix",
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Bellatrix{
					Bellatrix: util.NewBeaconBlockBellatrix().Block,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := logTest.NewGlobal()
			validator, m, validatorKey, finish := setup(t)
			defer finish()
			var pubKey [dilithium2.CryptoPublicKeyBytes]byte
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
			name: "phase0",
			blocks: func() []*zondpb.GenericBeaconBlock {
				block0, block1 := util.NewBeaconBlock(), util.NewBeaconBlock()
				block1.Block.Body.Graffiti = blockGraffiti[:]

				var blocks []*zondpb.GenericBeaconBlock
				for _, block := range []*zondpb.SignedBeaconBlock{block0, block1} {
					block.Block.Slot = slot
					blocks = append(blocks, &zondpb.GenericBeaconBlock{
						Block: &zondpb.GenericBeaconBlock_Phase0{
							Phase0: block.Block,
						},
					})
				}
				return blocks
			}(),
		},
		{
			name: "altair",
			blocks: func() []*zondpb.GenericBeaconBlock {
				block0, block1 := util.NewBeaconBlockAltair(), util.NewBeaconBlockAltair()
				block1.Block.Body.Graffiti = blockGraffiti[:]

				var blocks []*zondpb.GenericBeaconBlock
				for _, block := range []*zondpb.SignedBeaconBlockAltair{block0, block1} {
					block.Block.Slot = slot
					blocks = append(blocks, &zondpb.GenericBeaconBlock{
						Block: &zondpb.GenericBeaconBlock_Altair{
							Altair: block.Block,
						},
					})
				}
				return blocks
			}(),
		},
		{
			name: "bellatrix",
			blocks: func() []*zondpb.GenericBeaconBlock {
				block0, block1 := util.NewBeaconBlockBellatrix(), util.NewBeaconBlockBellatrix()
				block1.Block.Body.Graffiti = blockGraffiti[:]

				var blocks []*zondpb.GenericBeaconBlock
				for _, block := range []*zondpb.SignedBeaconBlockBellatrix{block0, block1} {
					block.Block.Slot = slot
					blocks = append(blocks, &zondpb.GenericBeaconBlock{
						Block: &zondpb.GenericBeaconBlock_Bellatrix{
							Bellatrix: block.Block,
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
			var pubKey [dilithium2.CryptoPublicKeyBytes]byte
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
	var pubKey [dilithium2.CryptoPublicKeyBytes]byte
	copy(pubKey[:], validatorKey.PublicKey().Marshal())

	var dummyRoot [32]byte
	// Save a dummy proposal history at slot 0.
	err := validator.db.SaveProposalHistoryForSlot(context.Background(), pubKey, 0, dummyRoot[:])
	require.NoError(t, err)

	m.validatorClient.EXPECT().DomainData(
		gomock.Any(), // ctx
		gomock.Any(), // epoch
	).Times(1).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

	testBlock := util.NewBeaconBlock()
	farFuture := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().WeakSubjectivityPeriod + 9))
	testBlock.Block.Slot = farFuture
	m.validatorClient.EXPECT().GetBeaconBlock(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
	).Return(&zondpb.GenericBeaconBlock{
		Block: &zondpb.GenericBeaconBlock_Phase0{
			Phase0: testBlock.Block,
		},
	}, nil /*err*/)

	secondTestBlock := util.NewBeaconBlock()
	secondTestBlock.Block.Slot = farFuture
	var blockGraffiti [32]byte
	copy(blockGraffiti[:], "someothergraffiti")
	secondTestBlock.Block.Body.Graffiti = blockGraffiti[:]
	m.validatorClient.EXPECT().GetBeaconBlock(
		gomock.Any(), // ctx
		gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
	).Return(&zondpb.GenericBeaconBlock{
		Block: &zondpb.GenericBeaconBlock_Phase0{
			Phase0: secondTestBlock.Block,
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
			var pubKey [dilithium2.CryptoPublicKeyBytes]byte
			copy(pubKey[:], validatorKey.PublicKey().Marshal())

			// Save a dummy proposal history at slot 0.
			err := validator.db.SaveProposalHistoryForSlot(context.Background(), pubKey, 0, []byte{})
			require.NoError(t, err)

			m.validatorClient.EXPECT().DomainData(
				gomock.Any(), // ctx
				gomock.Any(), // epoch
			).Times(2).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)

			blk := util.NewBeaconBlock()
			blk.Block.Slot = slot
			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
			).Return(&zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Phase0{
					Phase0: blk.Block,
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

			blk2 := util.NewBeaconBlock()
			blk2.Block.Slot = tt.pastSlot
			m.validatorClient.EXPECT().GetBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.BlockRequest{}),
			).Return(&zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Phase0{
					Phase0: blk2.Block,
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
			name: "phase0",
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Phase0{
					Phase0: func() *zondpb.BeaconBlock {
						blk := util.NewBeaconBlock()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name: "altair",
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Altair{
					Altair: func() *zondpb.BeaconBlockAltair {
						blk := util.NewBeaconBlockAltair()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name: "bellatrix",
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Bellatrix{
					Bellatrix: func() *zondpb.BeaconBlockBellatrix {
						blk := util.NewBeaconBlockBellatrix()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
		{
			name: "bellatrix blind block",
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_BlindedBellatrix{
					BlindedBellatrix: func() *zondpb.BlindedBeaconBlockBellatrix {
						blk := util.NewBlindedBeaconBlockBellatrix()
						blk.Block.Body.Graffiti = graffiti
						return blk.Block
					}(),
				},
			},
		},
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
		{
			name:    "deneb block and blobs",
			version: version.Deneb,
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_Deneb{
					Deneb: func() *zondpb.BeaconBlockAndBlobsDeneb {
						blk := util.NewBeaconBlockDeneb()
						blk.Block.Body.Graffiti = graffiti
						return &zondpb.BeaconBlockAndBlobsDeneb{
							Block: blk.Block,
							Blobs: []*zondpb.BlobSidecar{
								{
									BlockRoot:       bytesutil.PadTo([]byte("blockRoot"), 32),
									Index:           1,
									Slot:            2,
									BlockParentRoot: bytesutil.PadTo([]byte("blockParentRoot"), 32),
									ProposerIndex:   3,
									Blob:            bytesutil.PadTo([]byte("blob"), fieldparams.BlobLength),
									KzgCommitment:   bytesutil.PadTo([]byte("kzgCommitment"), 48),
									KzgProof:        bytesutil.PadTo([]byte("kzgPRoof"), 48),
								},
								{
									BlockRoot:       bytesutil.PadTo([]byte("blockRoot1"), 32),
									Index:           4,
									Slot:            5,
									BlockParentRoot: bytesutil.PadTo([]byte("blockParentRoot1"), 32),
									ProposerIndex:   6,
									Blob:            bytesutil.PadTo([]byte("blob1"), fieldparams.BlobLength),
									KzgCommitment:   bytesutil.PadTo([]byte("kzgCommitment1"), 48),
									KzgProof:        bytesutil.PadTo([]byte("kzgPRoof1"), 48),
								},
							},
						}
					}(),
				},
			},
		},
		{
			name:    "deneb blind block and blobs",
			version: version.Deneb,
			block: &zondpb.GenericBeaconBlock{
				Block: &zondpb.GenericBeaconBlock_BlindedDeneb{
					BlindedDeneb: func() *zondpb.BlindedBeaconBlockAndBlobsDeneb {
						blk := util.NewBlindedBeaconBlockDeneb()
						blk.Message.Body.Graffiti = graffiti
						return &zondpb.BlindedBeaconBlockAndBlobsDeneb{
							Block: blk.Message,
							Blobs: []*zondpb.BlindedBlobSidecar{
								{
									BlockRoot:       bytesutil.PadTo([]byte("blockRoot"), 32),
									Index:           1,
									Slot:            2,
									BlockParentRoot: bytesutil.PadTo([]byte("blockParentRoot"), 32),
									ProposerIndex:   3,
									BlobRoot:        bytesutil.PadTo([]byte("blobRoot"), 32),
									KzgCommitment:   bytesutil.PadTo([]byte("kzgCommitment"), 48),
									KzgProof:        bytesutil.PadTo([]byte("kzgPRoof"), 48),
								},
								{
									BlockRoot:       bytesutil.PadTo([]byte("blockRoot1"), 32),
									Index:           4,
									Slot:            5,
									BlockParentRoot: bytesutil.PadTo([]byte("blockParentRoot1"), 32),
									ProposerIndex:   6,
									BlobRoot:        bytesutil.PadTo([]byte("blobRoot1"), 32),
									KzgCommitment:   bytesutil.PadTo([]byte("kzgCommitment1"), 48),
									KzgProof:        bytesutil.PadTo([]byte("kzgPRoof1"), 48),
								},
							},
						}
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
			var pubKey [dilithium2.CryptoPublicKeyBytes]byte
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

			if tt.version == version.Deneb {
				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)
				m.validatorClient.EXPECT().DomainData(
					gomock.Any(), // ctx
					gomock.Any(), // epoch
				).Return(&zondpb.DomainResponse{SignatureDomain: make([]byte, 32)}, nil /*err*/)
			}

			m.validatorClient.EXPECT().ProposeBeaconBlock(
				gomock.Any(), // ctx
				gomock.AssignableToTypeOf(&zondpb.GenericSignedBeaconBlock{}),
			).DoAndReturn(func(ctx context.Context, block *zondpb.GenericSignedBeaconBlock) (*zondpb.ProposeResponse, error) {
				sentBlock, err = blocktest.NewSignedBeaconBlockFromGeneric(block)
				assert.NoError(t, err, "Unexpected error unwrapping block")
				if tt.version == version.Deneb {
					switch {
					case tt.name == "deneb block and blobs":
						require.Equal(t, 2, len(block.GetDeneb().Blobs))
					case tt.name == "deneb blind block and blobs":
						require.Equal(t, 2, len(block.GetBlindedDeneb().SignedBlindedBlobSidecars))
					}
				}
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

func TestSignBlock(t *testing.T) {
	validator, m, _, finish := setup(t)
	defer finish()

	proposerDomain := make([]byte, 32)
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&zondpb.DomainResponse{SignatureDomain: proposerDomain}, nil)
	ctx := context.Background()
	blk := util.NewBeaconBlock()
	blk.Block.Slot = 1
	blk.Block.ProposerIndex = 100

	kp := testKeyFromBytes(t, []byte{1})

	validator.keyManager = newMockKeymanager(t, kp)
	b, err := blocks.NewBeaconBlock(blk.Block)
	require.NoError(t, err)
	sig, blockRoot, err := validator.signBlock(ctx, kp.pub, 0, 0, b)
	require.NoError(t, err, "%x,%v", sig, err)
	require.Equal(t, "a049e1dc723e5a8b5bd14f292973572dffd53785ddb337"+
		"82f20bf762cbe10ee7b9b4f5ae1ad6ff2089d352403750bed402b94b58469c072536"+
		"faa9a09a88beaff697404ca028b1c7052b0de37dbcff985dfa500459783370312bdd"+
		"36d6e0f224", hex.EncodeToString(sig))

	// Verify the returned block root matches the expected root using the proposer signature
	// domain.
	wantedBlockRoot, err := signing.ComputeSigningRoot(b, proposerDomain)
	if err != nil {
		require.NoError(t, err)
	}
	require.DeepEqual(t, wantedBlockRoot, blockRoot)
}

func TestSignAltairBlock(t *testing.T) {
	validator, m, _, finish := setup(t)
	defer finish()

	kp := testKeyFromBytes(t, []byte{1})
	proposerDomain := make([]byte, 32)
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&zondpb.DomainResponse{SignatureDomain: proposerDomain}, nil)
	ctx := context.Background()
	blk := util.NewBeaconBlockAltair()
	blk.Block.Slot = 1
	blk.Block.ProposerIndex = 100
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

func TestSignBellatrixBlock(t *testing.T) {
	validator, m, _, finish := setup(t)
	defer finish()

	proposerDomain := make([]byte, 32)
	m.validatorClient.EXPECT().
		DomainData(gomock.Any(), gomock.Any()).
		Return(&zondpb.DomainResponse{SignatureDomain: proposerDomain}, nil)

	ctx := context.Background()
	blk := util.NewBeaconBlockBellatrix()
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
	pubKey := [dilithium2.CryptoPublicKeyBytes]byte{'a'}
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
	pubKey := [dilithium2.CryptoPublicKeyBytes]byte{'a'}
	valDB := testing2.SetupDB(t, [][dilithium2.CryptoPublicKeyBytes]byte{pubKey})
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
