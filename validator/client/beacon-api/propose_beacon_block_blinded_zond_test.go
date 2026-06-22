package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/validator/client/beacon-api/mock"
	test_helpers "github.com/theQRL/qrysm/validator/client/beacon-api/test-helpers"
)

func TestProposeBeaconBlock_BlindedZond(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)

	blindedZondBlock := generateSignedBlindedZondBlock()

	genericSignedBlock := &qrysmpb.GenericSignedBeaconBlock{}
	genericSignedBlock.Block = blindedZondBlock

	jsonBlindedZondBlock := &apimiddleware.SignedBlindedBeaconBlockZondJson{
		Signature: hexutil.Encode(blindedZondBlock.BlindedZond.Signature),
		Message: &apimiddleware.BlindedBeaconBlockZondJson{
			ParentRoot:    hexutil.Encode(blindedZondBlock.BlindedZond.Block.ParentRoot),
			ProposerIndex: uint64ToString(blindedZondBlock.BlindedZond.Block.ProposerIndex),
			Slot:          uint64ToString(blindedZondBlock.BlindedZond.Block.Slot),
			StateRoot:     hexutil.Encode(blindedZondBlock.BlindedZond.Block.StateRoot),
			Body: &apimiddleware.BlindedBeaconBlockBodyZondJson{
				Attestations:      jsonifyAttestations(blindedZondBlock.BlindedZond.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(blindedZondBlock.BlindedZond.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(blindedZondBlock.BlindedZond.Block.Body.Deposits),
				ExecutionData:     jsonifyExecutionData(blindedZondBlock.BlindedZond.Block.Body.ExecutionData),
				Graffiti:          hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(blindedZondBlock.BlindedZond.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(blindedZondBlock.BlindedZond.Block.Body.VoluntaryExits),
				SyncAggregate:     JsonifySyncAggregate(blindedZondBlock.BlindedZond.Block.Body.SyncAggregate),
				ExecutionPayloadHeader: &apimiddleware.ExecutionPayloadHeaderZondJson{
					BaseFeePerGas:    bytesutil.LittleEndianBytesToBigInt(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.BaseFeePerGas).String(),
					BlockHash:        hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.BlockHash),
					BlockNumber:      uint64ToString(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.BlockNumber),
					ExtraData:        hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.ExtraData),
					FeeRecipient:     hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.FeeRecipient),
					GasLimit:         uint64ToString(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.GasLimit),
					GasUsed:          uint64ToString(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.GasUsed),
					LogsBloom:        hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.LogsBloom),
					ParentHash:       hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.ParentHash),
					PrevRandao:       hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.PrevRandao),
					ReceiptsRoot:     hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.ReceiptsRoot),
					StateRoot:        hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.StateRoot),
					TimeStamp:        uint64ToString(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.Timestamp),
					TransactionsRoot: hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.TransactionsRoot),
					WithdrawalsRoot:  hexutil.Encode(blindedZondBlock.BlindedZond.Block.Body.ExecutionPayloadHeader.WithdrawalsRoot),
				},
			},
		},
	}

	marshalledBlock, err := json.Marshal(jsonBlindedZondBlock)
	require.NoError(t, err)

	ctx := context.Background()

	// Make sure that what we send in the POST body is the marshalled version of the protobuf block
	headers := map[string]string{"Qrl-Consensus-Version": "zond"}
	jsonRestHandler.EXPECT().PostRestJson(
		ctx,
		"/qrl/v1/beacon/blinded_blocks",
		headers,
		bytes.NewBuffer(marshalledBlock),
		nil,
	)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	proposeResponse, err := validatorClient.proposeBeaconBlock(ctx, genericSignedBlock)
	assert.NoError(t, err)
	require.NotNil(t, proposeResponse)

	expectedBlockRoot, err := blindedZondBlock.BlindedZond.Block.HashTreeRoot()
	require.NoError(t, err)

	// Make sure that the block root is set
	assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
}

func generateSignedBlindedZondBlock() *qrysmpb.GenericSignedBeaconBlock_BlindedZond {
	return &qrysmpb.GenericSignedBeaconBlock_BlindedZond{
		BlindedZond: &qrysmpb.SignedBlindedBeaconBlockZond{
			Block: &qrysmpb.BlindedBeaconBlockZond{
				Slot:          1,
				ProposerIndex: 2,
				ParentRoot:    test_helpers.FillByteSlice(32, 3),
				StateRoot:     test_helpers.FillByteSlice(32, 4),
				Body: &qrysmpb.BlindedBeaconBlockBodyZond{
					RandaoReveal: test_helpers.FillByteSlice(4627, 5),
					ExecutionData: &qrysmpb.ExecutionData{
						DepositRoot:  test_helpers.FillByteSlice(32, 6),
						DepositCount: 7,
						BlockHash:    test_helpers.FillByteSlice(32, 8),
					},
					Graffiti: test_helpers.FillByteSlice(32, 9),
					ProposerSlashings: []*qrysmpb.ProposerSlashing{
						{
							Header_1: &qrysmpb.SignedBeaconBlockHeader{
								Header: &qrysmpb.BeaconBlockHeader{
									Slot:          10,
									ProposerIndex: 11,
									ParentRoot:    test_helpers.FillByteSlice(32, 12),
									StateRoot:     test_helpers.FillByteSlice(32, 13),
									BodyRoot:      test_helpers.FillByteSlice(32, 14),
								},
								Signature: test_helpers.FillByteSlice(4627, 15),
							},
							Header_2: &qrysmpb.SignedBeaconBlockHeader{
								Header: &qrysmpb.BeaconBlockHeader{
									Slot:          16,
									ProposerIndex: 17,
									ParentRoot:    test_helpers.FillByteSlice(32, 18),
									StateRoot:     test_helpers.FillByteSlice(32, 19),
									BodyRoot:      test_helpers.FillByteSlice(32, 20),
								},
								Signature: test_helpers.FillByteSlice(4627, 21),
							},
						},
						{
							Header_1: &qrysmpb.SignedBeaconBlockHeader{
								Header: &qrysmpb.BeaconBlockHeader{
									Slot:          22,
									ProposerIndex: 23,
									ParentRoot:    test_helpers.FillByteSlice(32, 24),
									StateRoot:     test_helpers.FillByteSlice(32, 25),
									BodyRoot:      test_helpers.FillByteSlice(32, 26),
								},
								Signature: test_helpers.FillByteSlice(4627, 27),
							},
							Header_2: &qrysmpb.SignedBeaconBlockHeader{
								Header: &qrysmpb.BeaconBlockHeader{
									Slot:          28,
									ProposerIndex: 29,
									ParentRoot:    test_helpers.FillByteSlice(32, 30),
									StateRoot:     test_helpers.FillByteSlice(32, 31),
									BodyRoot:      test_helpers.FillByteSlice(32, 32),
								},
								Signature: test_helpers.FillByteSlice(4627, 33),
							},
						},
					},
					AttesterSlashings: []*qrysmpb.AttesterSlashing{
						{
							Attestation_1: &qrysmpb.IndexedAttestation{
								AttestingIndices: []uint64{34, 35},
								Data: &qrysmpb.AttestationData{
									Slot:            36,
									CommitteeIndex:  37,
									BeaconBlockRoot: test_helpers.FillByteSlice(32, 38),
									Source: &qrysmpb.Checkpoint{
										Epoch: 39,
										Root:  test_helpers.FillByteSlice(32, 40),
									},
									Target: &qrysmpb.Checkpoint{
										Epoch: 41,
										Root:  test_helpers.FillByteSlice(32, 42),
									},
								},
								Signatures: [][]byte{test_helpers.FillByteSlice(4627, 43)},
							},
							Attestation_2: &qrysmpb.IndexedAttestation{
								AttestingIndices: []uint64{44, 45},
								Data: &qrysmpb.AttestationData{
									Slot:            46,
									CommitteeIndex:  47,
									BeaconBlockRoot: test_helpers.FillByteSlice(32, 38),
									Source: &qrysmpb.Checkpoint{
										Epoch: 49,
										Root:  test_helpers.FillByteSlice(32, 50),
									},
									Target: &qrysmpb.Checkpoint{
										Epoch: 51,
										Root:  test_helpers.FillByteSlice(32, 52),
									},
								},
								Signatures: [][]byte{test_helpers.FillByteSlice(4627, 53)},
							},
						},
						{
							Attestation_1: &qrysmpb.IndexedAttestation{
								AttestingIndices: []uint64{54, 55},
								Data: &qrysmpb.AttestationData{
									Slot:            56,
									CommitteeIndex:  57,
									BeaconBlockRoot: test_helpers.FillByteSlice(32, 38),
									Source: &qrysmpb.Checkpoint{
										Epoch: 59,
										Root:  test_helpers.FillByteSlice(32, 60),
									},
									Target: &qrysmpb.Checkpoint{
										Epoch: 61,
										Root:  test_helpers.FillByteSlice(32, 62),
									},
								},
								Signatures: [][]byte{test_helpers.FillByteSlice(4627, 63)},
							},
							Attestation_2: &qrysmpb.IndexedAttestation{
								AttestingIndices: []uint64{64, 65},
								Data: &qrysmpb.AttestationData{
									Slot:            66,
									CommitteeIndex:  67,
									BeaconBlockRoot: test_helpers.FillByteSlice(32, 38),
									Source: &qrysmpb.Checkpoint{
										Epoch: 69,
										Root:  test_helpers.FillByteSlice(32, 70),
									},
									Target: &qrysmpb.Checkpoint{
										Epoch: 71,
										Root:  test_helpers.FillByteSlice(32, 72),
									},
								},
								Signatures: [][]byte{test_helpers.FillByteSlice(4627, 73)},
							},
						},
					},
					Attestations: []*qrysmpb.Attestation{
						{
							AggregationBits: test_helpers.FillByteSlice(4, 74),
							Data: &qrysmpb.AttestationData{
								Slot:            75,
								CommitteeIndex:  76,
								BeaconBlockRoot: test_helpers.FillByteSlice(32, 38),
								Source: &qrysmpb.Checkpoint{
									Epoch: 78,
									Root:  test_helpers.FillByteSlice(32, 79),
								},
								Target: &qrysmpb.Checkpoint{
									Epoch: 80,
									Root:  test_helpers.FillByteSlice(32, 81),
								},
							},
							Signatures: [][]byte{test_helpers.FillByteSlice(4627, 82)},
						},
						{
							AggregationBits: test_helpers.FillByteSlice(4, 83),
							Data: &qrysmpb.AttestationData{
								Slot:            84,
								CommitteeIndex:  85,
								BeaconBlockRoot: test_helpers.FillByteSlice(32, 38),
								Source: &qrysmpb.Checkpoint{
									Epoch: 87,
									Root:  test_helpers.FillByteSlice(32, 88),
								},
								Target: &qrysmpb.Checkpoint{
									Epoch: 89,
									Root:  test_helpers.FillByteSlice(32, 90),
								},
							},
							Signatures: [][]byte{test_helpers.FillByteSlice(4627, 91)},
						},
					},
					Deposits: []*qrysmpb.Deposit{
						{
							Proof: test_helpers.FillByteArraySlice(33, test_helpers.FillByteSlice(32, 92)),
							Data: &qrysmpb.Deposit_Data{
								PublicKey:             test_helpers.FillByteSlice(2592, 94),
								WithdrawalCredentials: test_helpers.FillByteSlice(64, 95),
								Amount:                96,
								Signature:             test_helpers.FillByteSlice(4627, 97),
							},
						},
						{
							Proof: test_helpers.FillByteArraySlice(33, test_helpers.FillByteSlice(32, 98)),
							Data: &qrysmpb.Deposit_Data{
								PublicKey:             test_helpers.FillByteSlice(2592, 100),
								WithdrawalCredentials: test_helpers.FillByteSlice(64, 101),
								Amount:                102,
								Signature:             test_helpers.FillByteSlice(4627, 103),
							},
						},
					},
					VoluntaryExits: []*qrysmpb.SignedVoluntaryExit{
						{
							Exit: &qrysmpb.VoluntaryExit{
								Epoch:          104,
								ValidatorIndex: 105,
							},
							Signature: test_helpers.FillByteSlice(4627, 106),
						},
						{
							Exit: &qrysmpb.VoluntaryExit{
								Epoch:          107,
								ValidatorIndex: 108,
							},
							Signature: test_helpers.FillByteSlice(4627, 109),
						},
					},
					SyncAggregate: &qrysmpb.SyncAggregate{
						SyncCommitteeBits:       test_helpers.FillByteSlice(16, 110),
						SyncCommitteeSignatures: [][]byte{test_helpers.FillByteSlice(4627, 111)},
					},
					ExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderZond{
						ParentHash:       test_helpers.FillByteSlice(32, 112),
						FeeRecipient:     test_helpers.FillByteSlice(fieldparams.FeeRecipientLength, 113),
						StateRoot:        test_helpers.FillByteSlice(32, 114),
						ReceiptsRoot:     test_helpers.FillByteSlice(32, 115),
						LogsBloom:        test_helpers.FillByteSlice(256, 116),
						PrevRandao:       test_helpers.FillByteSlice(32, 117),
						BlockNumber:      118,
						GasLimit:         119,
						GasUsed:          120,
						Timestamp:        121,
						ExtraData:        test_helpers.FillByteSlice(32, 122),
						BaseFeePerGas:    test_helpers.FillByteSlice(32, 123),
						BlockHash:        test_helpers.FillByteSlice(32, 124),
						TransactionsRoot: test_helpers.FillByteSlice(32, 125),
						WithdrawalsRoot:  test_helpers.FillByteSlice(32, 126),
					},
				},
			},
			Signature: test_helpers.FillByteSlice(4627, 135),
		},
	}
}
