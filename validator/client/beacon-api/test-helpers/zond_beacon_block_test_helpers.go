package test_helpers

import (
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func GenerateProtoZondBeaconBlock() *qrysmpb.BeaconBlockZond {
	return &qrysmpb.BeaconBlockZond{
		Slot:          1,
		ProposerIndex: 2,
		ParentRoot:    FillByteSlice(32, 3),
		StateRoot:     FillByteSlice(32, 4),
		Body: &qrysmpb.BeaconBlockBodyZond{
			RandaoReveal: FillByteSlice(4627, 5),
			ExecutionData: &qrysmpb.ExecutionData{
				DepositRoot:  FillByteSlice(32, 6),
				DepositCount: 7,
				BlockHash:    FillByteSlice(32, 8),
			},
			Graffiti: FillByteSlice(32, 9),
			ProposerSlashings: []*qrysmpb.ProposerSlashing{
				{
					Header_1: &qrysmpb.SignedBeaconBlockHeader{
						Header: &qrysmpb.BeaconBlockHeader{
							Slot:          10,
							ProposerIndex: 11,
							ParentRoot:    FillByteSlice(32, 12),
							StateRoot:     FillByteSlice(32, 13),
							BodyRoot:      FillByteSlice(32, 14),
						},
						Signature: FillByteSlice(4627, 15),
					},
					Header_2: &qrysmpb.SignedBeaconBlockHeader{
						Header: &qrysmpb.BeaconBlockHeader{
							Slot:          16,
							ProposerIndex: 17,
							ParentRoot:    FillByteSlice(32, 18),
							StateRoot:     FillByteSlice(32, 19),
							BodyRoot:      FillByteSlice(32, 20),
						},
						Signature: FillByteSlice(4627, 21),
					},
				},
				{
					Header_1: &qrysmpb.SignedBeaconBlockHeader{
						Header: &qrysmpb.BeaconBlockHeader{
							Slot:          22,
							ProposerIndex: 23,
							ParentRoot:    FillByteSlice(32, 24),
							StateRoot:     FillByteSlice(32, 25),
							BodyRoot:      FillByteSlice(32, 26),
						},
						Signature: FillByteSlice(4627, 27),
					},
					Header_2: &qrysmpb.SignedBeaconBlockHeader{
						Header: &qrysmpb.BeaconBlockHeader{
							Slot:          28,
							ProposerIndex: 29,
							ParentRoot:    FillByteSlice(32, 30),
							StateRoot:     FillByteSlice(32, 31),
							BodyRoot:      FillByteSlice(32, 32),
						},
						Signature: FillByteSlice(4627, 33),
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
							BeaconBlockRoot: FillByteSlice(32, 38),
							Source: &qrysmpb.Checkpoint{
								Epoch: 39,
								Root:  FillByteSlice(32, 40),
							},
							Target: &qrysmpb.Checkpoint{
								Epoch: 41,
								Root:  FillByteSlice(32, 42),
							},
						},
						Signatures: [][]byte{FillByteSlice(4627, 43)},
					},
					Attestation_2: &qrysmpb.IndexedAttestation{
						AttestingIndices: []uint64{44, 45},
						Data: &qrysmpb.AttestationData{
							Slot:            46,
							CommitteeIndex:  47,
							BeaconBlockRoot: FillByteSlice(32, 38),
							Source: &qrysmpb.Checkpoint{
								Epoch: 49,
								Root:  FillByteSlice(32, 50),
							},
							Target: &qrysmpb.Checkpoint{
								Epoch: 51,
								Root:  FillByteSlice(32, 52),
							},
						},
						Signatures: [][]byte{FillByteSlice(4627, 53)},
					},
				},
				{
					Attestation_1: &qrysmpb.IndexedAttestation{
						AttestingIndices: []uint64{54, 55},
						Data: &qrysmpb.AttestationData{
							Slot:            56,
							CommitteeIndex:  57,
							BeaconBlockRoot: FillByteSlice(32, 38),
							Source: &qrysmpb.Checkpoint{
								Epoch: 59,
								Root:  FillByteSlice(32, 60),
							},
							Target: &qrysmpb.Checkpoint{
								Epoch: 61,
								Root:  FillByteSlice(32, 62),
							},
						},
						Signatures: [][]byte{FillByteSlice(4627, 63)},
					},
					Attestation_2: &qrysmpb.IndexedAttestation{
						AttestingIndices: []uint64{64, 65},
						Data: &qrysmpb.AttestationData{
							Slot:            66,
							CommitteeIndex:  67,
							BeaconBlockRoot: FillByteSlice(32, 38),
							Source: &qrysmpb.Checkpoint{
								Epoch: 69,
								Root:  FillByteSlice(32, 70),
							},
							Target: &qrysmpb.Checkpoint{
								Epoch: 71,
								Root:  FillByteSlice(32, 72),
							},
						},
						Signatures: [][]byte{FillByteSlice(4627, 73)},
					},
				},
			},
			Attestations: []*qrysmpb.Attestation{
				{
					AggregationBits: FillByteSlice(4, 74),
					Data: &qrysmpb.AttestationData{
						Slot:            75,
						CommitteeIndex:  76,
						BeaconBlockRoot: FillByteSlice(32, 38),
						Source: &qrysmpb.Checkpoint{
							Epoch: 78,
							Root:  FillByteSlice(32, 79),
						},
						Target: &qrysmpb.Checkpoint{
							Epoch: 80,
							Root:  FillByteSlice(32, 81),
						},
					},
					Signatures: [][]byte{FillByteSlice(4627, 82)},
				},
				{
					AggregationBits: FillByteSlice(4, 83),
					Data: &qrysmpb.AttestationData{
						Slot:            84,
						CommitteeIndex:  85,
						BeaconBlockRoot: FillByteSlice(32, 38),
						Source: &qrysmpb.Checkpoint{
							Epoch: 87,
							Root:  FillByteSlice(32, 88),
						},
						Target: &qrysmpb.Checkpoint{
							Epoch: 89,
							Root:  FillByteSlice(32, 90),
						},
					},
					Signatures: [][]byte{FillByteSlice(4627, 91)},
				},
			},
			Deposits: []*qrysmpb.Deposit{
				{
					Proof: FillByteArraySlice(33, FillByteSlice(32, 92)),
					Data: &qrysmpb.Deposit_Data{
						PublicKey:             FillByteSlice(2592, 94),
						WithdrawalCredentials: FillByteSlice(64, 95),
						Amount:                96,
						Signature:             FillByteSlice(4627, 97),
					},
				},
				{
					Proof: FillByteArraySlice(33, FillByteSlice(32, 98)),
					Data: &qrysmpb.Deposit_Data{
						PublicKey:             FillByteSlice(2592, 100),
						WithdrawalCredentials: FillByteSlice(64, 101),
						Amount:                102,
						Signature:             FillByteSlice(4627, 103),
					},
				},
			},
			VoluntaryExits: []*qrysmpb.SignedVoluntaryExit{
				{
					Exit: &qrysmpb.VoluntaryExit{
						Epoch:          104,
						ValidatorIndex: 105,
					},
					Signature: FillByteSlice(4627, 106),
				},
				{
					Exit: &qrysmpb.VoluntaryExit{
						Epoch:          107,
						ValidatorIndex: 108,
					},
					Signature: FillByteSlice(4627, 109),
				},
			},
			SyncAggregate: &qrysmpb.SyncAggregate{
				SyncCommitteeBits:       FillByteSlice(16, 110),
				SyncCommitteeSignatures: [][]byte{FillByteSlice(4627, 111)},
			},
			ExecutionPayload: &enginev1.ExecutionPayloadZond{
				ParentHash:    FillByteSlice(32, 112),
				FeeRecipient:  FillByteSlice(fieldparams.FeeRecipientLength, 113),
				StateRoot:     FillByteSlice(32, 114),
				ReceiptsRoot:  FillByteSlice(32, 115),
				LogsBloom:     FillByteSlice(256, 116),
				PrevRandao:    FillByteSlice(32, 117),
				BlockNumber:   118,
				GasLimit:      119,
				GasUsed:       120,
				Timestamp:     121,
				ExtraData:     FillByteSlice(32, 122),
				BaseFeePerGas: FillByteSlice(32, 123),
				BlockHash:     FillByteSlice(32, 124),
				Transactions: [][]byte{
					FillByteSlice(32, 125),
					FillByteSlice(32, 126),
				},
				Withdrawals: []*enginev1.Withdrawal{
					{
						Index:          127,
						ValidatorIndex: 128,
						Address:        FillByteSlice(fieldparams.FeeRecipientLength, 129),
						Amount:         130,
					},
					{
						Index:          131,
						ValidatorIndex: 132,
						Address:        FillByteSlice(fieldparams.FeeRecipientLength, 133),
						Amount:         134,
					},
				},
			},
		},
	}
}

func GenerateJsonZondBeaconBlock() *apimiddleware.BeaconBlockZondJson {
	return &apimiddleware.BeaconBlockZondJson{
		Slot:          "1",
		ProposerIndex: "2",
		ParentRoot:    FillEncodedByteSlice(32, 3),
		StateRoot:     FillEncodedByteSlice(32, 4),
		Body: &apimiddleware.BeaconBlockBodyZondJson{
			RandaoReveal: FillEncodedByteSlice(4627, 5),
			ExecutionData: &apimiddleware.ExecutionDataJson{
				DepositRoot:  FillEncodedByteSlice(32, 6),
				DepositCount: "7",
				BlockHash:    FillEncodedByteSlice(32, 8),
			},
			Graffiti: FillEncodedByteSlice(32, 9),
			ProposerSlashings: []*apimiddleware.ProposerSlashingJson{
				{
					Header_1: &apimiddleware.SignedBeaconBlockHeaderJson{
						Header: &apimiddleware.BeaconBlockHeaderJson{
							Slot:          "10",
							ProposerIndex: "11",
							ParentRoot:    FillEncodedByteSlice(32, 12),
							StateRoot:     FillEncodedByteSlice(32, 13),
							BodyRoot:      FillEncodedByteSlice(32, 14),
						},
						Signature: FillEncodedByteSlice(4627, 15),
					},
					Header_2: &apimiddleware.SignedBeaconBlockHeaderJson{
						Header: &apimiddleware.BeaconBlockHeaderJson{
							Slot:          "16",
							ProposerIndex: "17",
							ParentRoot:    FillEncodedByteSlice(32, 18),
							StateRoot:     FillEncodedByteSlice(32, 19),
							BodyRoot:      FillEncodedByteSlice(32, 20),
						},
						Signature: FillEncodedByteSlice(4627, 21),
					},
				},
				{
					Header_1: &apimiddleware.SignedBeaconBlockHeaderJson{
						Header: &apimiddleware.BeaconBlockHeaderJson{
							Slot:          "22",
							ProposerIndex: "23",
							ParentRoot:    FillEncodedByteSlice(32, 24),
							StateRoot:     FillEncodedByteSlice(32, 25),
							BodyRoot:      FillEncodedByteSlice(32, 26),
						},
						Signature: FillEncodedByteSlice(4627, 27),
					},
					Header_2: &apimiddleware.SignedBeaconBlockHeaderJson{
						Header: &apimiddleware.BeaconBlockHeaderJson{
							Slot:          "28",
							ProposerIndex: "29",
							ParentRoot:    FillEncodedByteSlice(32, 30),
							StateRoot:     FillEncodedByteSlice(32, 31),
							BodyRoot:      FillEncodedByteSlice(32, 32),
						},
						Signature: FillEncodedByteSlice(4627, 33),
					},
				},
			},
			AttesterSlashings: []*apimiddleware.AttesterSlashingJson{
				{
					Attestation_1: &apimiddleware.IndexedAttestationJson{
						AttestingIndices: []string{"34", "35"},
						Data: &apimiddleware.AttestationDataJson{
							Slot:            "36",
							CommitteeIndex:  "37",
							BeaconBlockRoot: FillEncodedByteSlice(32, 38),
							Source: &apimiddleware.CheckpointJson{
								Epoch: "39",
								Root:  FillEncodedByteSlice(32, 40),
							},
							Target: &apimiddleware.CheckpointJson{
								Epoch: "41",
								Root:  FillEncodedByteSlice(32, 42),
							},
						},
						Signatures: []string{FillEncodedByteSlice(4627, 43)},
					},
					Attestation_2: &apimiddleware.IndexedAttestationJson{
						AttestingIndices: []string{"44", "45"},
						Data: &apimiddleware.AttestationDataJson{
							Slot:            "46",
							CommitteeIndex:  "47",
							BeaconBlockRoot: FillEncodedByteSlice(32, 38),
							Source: &apimiddleware.CheckpointJson{
								Epoch: "49",
								Root:  FillEncodedByteSlice(32, 50),
							},
							Target: &apimiddleware.CheckpointJson{
								Epoch: "51",
								Root:  FillEncodedByteSlice(32, 52),
							},
						},
						Signatures: []string{FillEncodedByteSlice(4627, 53)},
					},
				},
				{
					Attestation_1: &apimiddleware.IndexedAttestationJson{
						AttestingIndices: []string{"54", "55"},
						Data: &apimiddleware.AttestationDataJson{
							Slot:            "56",
							CommitteeIndex:  "57",
							BeaconBlockRoot: FillEncodedByteSlice(32, 38),
							Source: &apimiddleware.CheckpointJson{
								Epoch: "59",
								Root:  FillEncodedByteSlice(32, 60),
							},
							Target: &apimiddleware.CheckpointJson{
								Epoch: "61",
								Root:  FillEncodedByteSlice(32, 62),
							},
						},
						Signatures: []string{FillEncodedByteSlice(4627, 63)},
					},
					Attestation_2: &apimiddleware.IndexedAttestationJson{
						AttestingIndices: []string{"64", "65"},
						Data: &apimiddleware.AttestationDataJson{
							Slot:            "66",
							CommitteeIndex:  "67",
							BeaconBlockRoot: FillEncodedByteSlice(32, 38),
							Source: &apimiddleware.CheckpointJson{
								Epoch: "69",
								Root:  FillEncodedByteSlice(32, 70),
							},
							Target: &apimiddleware.CheckpointJson{
								Epoch: "71",
								Root:  FillEncodedByteSlice(32, 72),
							},
						},
						Signatures: []string{FillEncodedByteSlice(4627, 73)},
					},
				},
			},
			Attestations: []*apimiddleware.AttestationJson{
				{
					AggregationBits: FillEncodedByteSlice(4, 74),
					Data: &apimiddleware.AttestationDataJson{
						Slot:            "75",
						CommitteeIndex:  "76",
						BeaconBlockRoot: FillEncodedByteSlice(32, 38),
						Source: &apimiddleware.CheckpointJson{
							Epoch: "78",
							Root:  FillEncodedByteSlice(32, 79),
						},
						Target: &apimiddleware.CheckpointJson{
							Epoch: "80",
							Root:  FillEncodedByteSlice(32, 81),
						},
					},
					Signatures: []string{FillEncodedByteSlice(4627, 82)},
				},
				{
					AggregationBits: FillEncodedByteSlice(4, 83),
					Data: &apimiddleware.AttestationDataJson{
						Slot:            "84",
						CommitteeIndex:  "85",
						BeaconBlockRoot: FillEncodedByteSlice(32, 38),
						Source: &apimiddleware.CheckpointJson{
							Epoch: "87",
							Root:  FillEncodedByteSlice(32, 88),
						},
						Target: &apimiddleware.CheckpointJson{
							Epoch: "89",
							Root:  FillEncodedByteSlice(32, 90),
						},
					},
					Signatures: []string{FillEncodedByteSlice(4627, 91)},
				},
			},
			Deposits: []*apimiddleware.DepositJson{
				{
					Proof: FillEncodedByteArraySlice(33, FillEncodedByteSlice(32, 92)),
					Data: &apimiddleware.Deposit_DataJson{
						PublicKey:             FillEncodedByteSlice(2592, 94),
						WithdrawalCredentials: FillEncodedByteSlice(64, 95),
						Amount:                "96",
						Signature:             FillEncodedByteSlice(4627, 97),
					},
				},
				{
					Proof: FillEncodedByteArraySlice(33, FillEncodedByteSlice(32, 98)),
					Data: &apimiddleware.Deposit_DataJson{
						PublicKey:             FillEncodedByteSlice(2592, 100),
						WithdrawalCredentials: FillEncodedByteSlice(64, 101),
						Amount:                "102",
						Signature:             FillEncodedByteSlice(4627, 103),
					},
				},
			},
			VoluntaryExits: []*apimiddleware.SignedVoluntaryExitJson{
				{
					Exit: &apimiddleware.VoluntaryExitJson{
						Epoch:          "104",
						ValidatorIndex: "105",
					},
					Signature: FillEncodedByteSlice(4627, 106),
				},
				{
					Exit: &apimiddleware.VoluntaryExitJson{
						Epoch:          "107",
						ValidatorIndex: "108",
					},
					Signature: FillEncodedByteSlice(4627, 109),
				},
			},
			SyncAggregate: &apimiddleware.SyncAggregateJson{
				SyncCommitteeBits:       FillEncodedByteSlice(16, 110),
				SyncCommitteeSignatures: []string{FillEncodedByteSlice(4627, 111)},
			},
			ExecutionPayload: &apimiddleware.ExecutionPayloadZondJson{
				ParentHash:    FillEncodedByteSlice(32, 112),
				FeeRecipient:  FillEncodedByteSlice(fieldparams.FeeRecipientLength, 113),
				StateRoot:     FillEncodedByteSlice(32, 114),
				ReceiptsRoot:  FillEncodedByteSlice(32, 115),
				LogsBloom:     FillEncodedByteSlice(256, 116),
				PrevRandao:    FillEncodedByteSlice(32, 117),
				BlockNumber:   "118",
				GasLimit:      "119",
				GasUsed:       "120",
				TimeStamp:     "121",
				ExtraData:     FillEncodedByteSlice(32, 122),
				BaseFeePerGas: bytesutil.LittleEndianBytesToBigInt(FillByteSlice(32, 123)).String(),
				BlockHash:     FillEncodedByteSlice(32, 124),
				Transactions: []string{
					FillEncodedByteSlice(32, 125),
					FillEncodedByteSlice(32, 126),
				},
				Withdrawals: []*apimiddleware.WithdrawalJson{
					{
						WithdrawalIndex:  "127",
						ValidatorIndex:   "128",
						ExecutionAddress: FillEncodedByteSlice(fieldparams.FeeRecipientLength, 129),
						Amount:           "130",
					},
					{
						WithdrawalIndex:  "131",
						ValidatorIndex:   "132",
						ExecutionAddress: FillEncodedByteSlice(fieldparams.FeeRecipientLength, 133),
						Amount:           "134",
					},
				},
			},
		},
	}
}
