package v1_test

import (
	"reflect"
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	v1 "github.com/theQRL/qrysm/v4/validator/keymanager/remote-web3signer/v1"
	"github.com/theQRL/qrysm/v4/validator/keymanager/remote-web3signer/v1/mock"
)

func TestMapAggregateAndProof(t *testing.T) {
	type args struct {
		from *zondpb.AggregateAttestationAndProof
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.AggregateAndProof
		wantErr bool
	}{
		{
			name: "HappyPathTest",
			args: args{
				from: &zondpb.AggregateAttestationAndProof{
					AggregatorIndex: 0,
					Aggregate: &zondpb.Attestation{
						AggregationBits: bitfield.Bitlist{0b1101},
						Data: &zondpb.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &zondpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &zondpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, 96),
					},
					SelectionProof: make([]byte, dilithium2.CryptoBytes),
				},
			},
			want: &v1.AggregateAndProof{
				AggregatorIndex: "0",
				Aggregate:       mock.MockAttestation(),
				SelectionProof:  make([]byte, dilithium2.CryptoBytes),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1.MapAggregateAndProof(tt.args.from)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapAggregateAndProof() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got.Aggregate, tt.want.Aggregate) {
				t.Errorf("MapAggregateAndProof() got = %v, want %v", got.Aggregate, tt.want.Aggregate)
			}
		})
	}
}

func TestMapAttestation(t *testing.T) {
	type args struct {
		attestation *zondpb.Attestation
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.Attestation
		wantErr bool
	}{
		{
			name: "HappyPathTest",
			args: args{
				attestation: &zondpb.Attestation{
					AggregationBits: bitfield.Bitlist{0b1101},
					Data: &zondpb.AttestationData{
						BeaconBlockRoot: make([]byte, fieldparams.RootLength),
						Source: &zondpb.Checkpoint{
							Root: make([]byte, fieldparams.RootLength),
						},
						Target: &zondpb.Checkpoint{
							Root: make([]byte, fieldparams.RootLength),
						},
					},
					Signature: make([]byte, 96),
				},
			},
			want:    mock.MockAttestation(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1.MapAttestation(tt.args.attestation)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapAttestation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapAttestation() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapAttestationData(t *testing.T) {
	type args struct {
		data *zondpb.AttestationData
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.AttestationData
		wantErr bool
	}{
		{
			name: "HappyPathTest",
			args: args{
				data: &zondpb.AttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					Source: &zondpb.Checkpoint{
						Root: make([]byte, fieldparams.RootLength),
					},
					Target: &zondpb.Checkpoint{
						Root: make([]byte, fieldparams.RootLength),
					},
				},
			},
			want:    mock.MockAttestation().Data,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1.MapAttestationData(tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapAttestationData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapAttestationData() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapAttesterSlashing(t *testing.T) {
	type args struct {
		slashing *zondpb.AttesterSlashing
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.AttesterSlashing
		wantErr bool
	}{
		{
			name: "HappyPathTest",
			args: args{
				slashing: &zondpb.AttesterSlashing{
					Attestation_1: &zondpb.IndexedAttestation{
						AttestingIndices: []uint64{0, 1, 2},
						Data: &zondpb.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &zondpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &zondpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, dilithium2.CryptoBytes),
					},
					Attestation_2: &zondpb.IndexedAttestation{
						AttestingIndices: []uint64{0, 1, 2},
						Data: &zondpb.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &zondpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &zondpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, dilithium2.CryptoBytes),
					},
				},
			},
			want: &v1.AttesterSlashing{
				Attestation1: mock.MockIndexedAttestation(),
				Attestation2: mock.MockIndexedAttestation(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1.MapAttesterSlashing(tt.args.slashing)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapAttesterSlashing() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got.Attestation1, tt.want.Attestation1) {
				t.Errorf("MapAttesterSlashing() got = %v, want %v", got.Attestation1, tt.want.Attestation1)
			}
		})
	}
}

func TestMapBeaconBlockAltair(t *testing.T) {
	type args struct {
		block *zondpb.BeaconBlockAltair
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.BeaconBlockAltair
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				block: &zondpb.BeaconBlockAltair{
					Slot:          0,
					ProposerIndex: 0,
					ParentRoot:    make([]byte, fieldparams.RootLength),
					StateRoot:     make([]byte, fieldparams.RootLength),
					Body: &zondpb.BeaconBlockBodyAltair{
						RandaoReveal: make([]byte, 32),
						Eth1Data: &zondpb.Eth1Data{
							DepositRoot:  make([]byte, fieldparams.RootLength),
							DepositCount: 0,
							BlockHash:    make([]byte, 32),
						},
						Graffiti: make([]byte, 32),
						ProposerSlashings: []*zondpb.ProposerSlashing{
							{
								Header_1: &zondpb.SignedBeaconBlockHeader{
									Header: &zondpb.BeaconBlockHeader{
										Slot:          0,
										ProposerIndex: 0,
										ParentRoot:    make([]byte, fieldparams.RootLength),
										StateRoot:     make([]byte, fieldparams.RootLength),
										BodyRoot:      make([]byte, fieldparams.RootLength),
									},
									Signature: make([]byte, dilithium2.CryptoBytes),
								},
								Header_2: &zondpb.SignedBeaconBlockHeader{
									Header: &zondpb.BeaconBlockHeader{
										Slot:          0,
										ProposerIndex: 0,
										ParentRoot:    make([]byte, fieldparams.RootLength),
										StateRoot:     make([]byte, fieldparams.RootLength),
										BodyRoot:      make([]byte, fieldparams.RootLength),
									},
									Signature: make([]byte, dilithium2.CryptoBytes),
								},
							},
						},
						AttesterSlashings: []*zondpb.AttesterSlashing{
							{
								Attestation_1: &zondpb.IndexedAttestation{
									AttestingIndices: []uint64{0, 1, 2},
									Data: &zondpb.AttestationData{
										BeaconBlockRoot: make([]byte, fieldparams.RootLength),
										Source: &zondpb.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
										Target: &zondpb.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
									},
									Signature: make([]byte, dilithium2.CryptoBytes),
								},
								Attestation_2: &zondpb.IndexedAttestation{
									AttestingIndices: []uint64{0, 1, 2},
									Data: &zondpb.AttestationData{
										BeaconBlockRoot: make([]byte, fieldparams.RootLength),
										Source: &zondpb.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
										Target: &zondpb.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
									},
									Signature: make([]byte, dilithium2.CryptoBytes),
								},
							},
						},
						Attestations: []*zondpb.Attestation{
							{
								AggregationBits: bitfield.Bitlist{0b1101},
								Data: &zondpb.AttestationData{
									BeaconBlockRoot: make([]byte, fieldparams.RootLength),
									Source: &zondpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
									Target: &zondpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
								},
								Signature: make([]byte, 96),
							},
						},
						Deposits: []*zondpb.Deposit{
							{
								Proof: [][]byte{[]byte("A")},
								Data: &zondpb.Deposit_Data{
									PublicKey:             make([]byte, dilithium2.CryptoPublicKeyBytes),
									WithdrawalCredentials: make([]byte, 32),
									Amount:                0,
									Signature:             make([]byte, dilithium2.CryptoBytes),
								},
							},
						},
						VoluntaryExits: []*zondpb.SignedVoluntaryExit{
							{
								Exit: &zondpb.VoluntaryExit{
									Epoch:          0,
									ValidatorIndex: 0,
								},
								Signature: make([]byte, dilithium2.CryptoBytes),
							},
						},
						SyncAggregate: &zondpb.SyncAggregate{
							SyncCommitteeSignature: make([]byte, dilithium2.CryptoBytes),
							SyncCommitteeBits:      mock.MockSyncComitteeBits(),
						},
					},
				},
			},
			want:    mock.MockBeaconBlockAltair(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1.MapBeaconBlockAltair(tt.args.block)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapBeaconBlockAltair() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got.Body, tt.want.Body) {
				t.Errorf("MapBeaconBlockAltair() got = %v, want %v", got.Body.SyncAggregate, tt.want.Body.SyncAggregate)
			}
		})
	}
}

func TestMapBeaconBlockBody(t *testing.T) {
	type args struct {
		body *zondpb.BeaconBlockBody
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.BeaconBlockBody
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				body: &zondpb.BeaconBlockBody{
					RandaoReveal: make([]byte, 32),
					Eth1Data: &zondpb.Eth1Data{
						DepositRoot:  make([]byte, fieldparams.RootLength),
						DepositCount: 0,
						BlockHash:    make([]byte, 32),
					},
					Graffiti: make([]byte, 32),
					ProposerSlashings: []*zondpb.ProposerSlashing{
						{
							Header_1: &zondpb.SignedBeaconBlockHeader{
								Header: &zondpb.BeaconBlockHeader{
									Slot:          0,
									ProposerIndex: 0,
									ParentRoot:    make([]byte, fieldparams.RootLength),
									StateRoot:     make([]byte, fieldparams.RootLength),
									BodyRoot:      make([]byte, fieldparams.RootLength),
								},
								Signature: make([]byte, dilithium2.CryptoBytes),
							},
							Header_2: &zondpb.SignedBeaconBlockHeader{
								Header: &zondpb.BeaconBlockHeader{
									Slot:          0,
									ProposerIndex: 0,
									ParentRoot:    make([]byte, fieldparams.RootLength),
									StateRoot:     make([]byte, fieldparams.RootLength),
									BodyRoot:      make([]byte, fieldparams.RootLength),
								},
								Signature: make([]byte, dilithium2.CryptoBytes),
							},
						},
					},
					AttesterSlashings: []*zondpb.AttesterSlashing{
						{
							Attestation_1: &zondpb.IndexedAttestation{
								AttestingIndices: []uint64{0, 1, 2},
								Data: &zondpb.AttestationData{
									BeaconBlockRoot: make([]byte, fieldparams.RootLength),
									Source: &zondpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
									Target: &zondpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
								},
								Signature: make([]byte, dilithium2.CryptoBytes),
							},
							Attestation_2: &zondpb.IndexedAttestation{
								AttestingIndices: []uint64{0, 1, 2},
								Data: &zondpb.AttestationData{
									BeaconBlockRoot: make([]byte, fieldparams.RootLength),
									Source: &zondpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
									Target: &zondpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
								},
								Signature: make([]byte, dilithium2.CryptoBytes),
							},
						},
					},
					Attestations: []*zondpb.Attestation{
						{
							AggregationBits: bitfield.Bitlist{0b1101},
							Data: &zondpb.AttestationData{
								BeaconBlockRoot: make([]byte, fieldparams.RootLength),
								Source: &zondpb.Checkpoint{
									Root: make([]byte, fieldparams.RootLength),
								},
								Target: &zondpb.Checkpoint{
									Root: make([]byte, fieldparams.RootLength),
								},
							},
							Signature: make([]byte, 96),
						},
					},
					Deposits: []*zondpb.Deposit{
						{
							Proof: [][]byte{[]byte("A")},
							Data: &zondpb.Deposit_Data{
								PublicKey:             make([]byte, dilithium2.CryptoPublicKeyBytes),
								WithdrawalCredentials: make([]byte, 32),
								Amount:                0,
								Signature:             make([]byte, dilithium2.CryptoBytes),
							},
						},
					},
					VoluntaryExits: []*zondpb.SignedVoluntaryExit{
						{
							Exit: &zondpb.VoluntaryExit{
								Epoch:          0,
								ValidatorIndex: 0,
							},
							Signature: make([]byte, dilithium2.CryptoBytes),
						},
					},
				},
			},
			want:    mock.MockBeaconBlockBody(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1.MapBeaconBlockBody(tt.args.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapBeaconBlockBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapBeaconBlockBody() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapContributionAndProof(t *testing.T) {
	type args struct {
		contribution *zondpb.ContributionAndProof
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.ContributionAndProof
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				contribution: &zondpb.ContributionAndProof{
					AggregatorIndex: 0,
					Contribution: &zondpb.SyncCommitteeContribution{
						Slot:              0,
						BlockRoot:         make([]byte, fieldparams.RootLength),
						SubcommitteeIndex: 0,
						AggregationBits:   mock.MockAggregationBits(),
						Signature:         make([]byte, dilithium2.CryptoBytes),
					},
					SelectionProof: make([]byte, dilithium2.CryptoBytes),
				},
			},
			want: mock.MockContributionAndProof(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1.MapContributionAndProof(tt.args.contribution)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapContributionAndProof() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapContributionAndProof() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapForkInfo(t *testing.T) {
	type args struct {
		slot                  primitives.Slot
		genesisValidatorsRoot []byte
	}

	tests := []struct {
		name    string
		args    args
		want    *v1.ForkInfo
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				slot:                  0,
				genesisValidatorsRoot: make([]byte, fieldparams.RootLength),
			},
			want:    mock.MockForkInfo(),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1.MapForkInfo(tt.args.slot, tt.args.genesisValidatorsRoot)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapForkInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapForkInfo() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapSyncAggregatorSelectionData(t *testing.T) {
	type args struct {
		data *zondpb.SyncAggregatorSelectionData
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.SyncAggregatorSelectionData
		wantErr bool
	}{
		{
			name: "Happy Path Test",
			args: args{
				data: &zondpb.SyncAggregatorSelectionData{
					Slot:              0,
					SubcommitteeIndex: 0,
				},
			},
			want: &v1.SyncAggregatorSelectionData{
				Slot:              "0",
				SubcommitteeIndex: "0",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := v1.MapSyncAggregatorSelectionData(tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapSyncAggregatorSelectionData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapSyncAggregatorSelectionData() got = %v, want %v", got, tt.want)
			}
		})
	}
}
