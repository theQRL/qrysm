package v1_test

/*
import (
	"reflect"
	"testing"

	"github.com/theQRL/go-bitfield"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	v1 "github.com/theQRL/qrysm/validator/keymanager/remote-web3signer/v1"
	"github.com/theQRL/qrysm/validator/keymanager/remote-web3signer/v1/mock"
)

func TestMapAggregateAndProof(t *testing.T) {
	type args struct {
		from *qrysmpb.AggregateAttestationAndProof
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
				from: &qrysmpb.AggregateAttestationAndProof{
					AggregatorIndex: 0,
					Aggregate: &qrysmpb.Attestation{
						AggregationBits: bitfield.Bitlist{0b1101},
						Data: &qrysmpb.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &qrysmpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &qrysmpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, 4627),
					},
					SelectionProof: make([]byte, field_params.MLDSA87SignatureLength),
				},
			},
			want: &v1.AggregateAndProof{
				AggregatorIndex: "0",
				Aggregate:       mock.MockAttestation(),
				SelectionProof:  make([]byte, field_params.MLDSA87SignatureLength),
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
		attestation *qrysmpb.Attestation
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
				attestation: &qrysmpb.Attestation{
					AggregationBits: bitfield.Bitlist{0b1101},
					Data: &qrysmpb.AttestationData{
						BeaconBlockRoot: make([]byte, fieldparams.RootLength),
						Source: &qrysmpb.Checkpoint{
							Root: make([]byte, fieldparams.RootLength),
						},
						Target: &qrysmpb.Checkpoint{
							Root: make([]byte, fieldparams.RootLength),
						},
					},
					Signature: make([]byte, 4627),
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
		data *qrysmpb.AttestationData
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
				data: &qrysmpb.AttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					Source: &qrysmpb.Checkpoint{
						Root: make([]byte, fieldparams.RootLength),
					},
					Target: &qrysmpb.Checkpoint{
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
		slashing *qrysmpb.AttesterSlashing
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
				slashing: &qrysmpb.AttesterSlashing{
					Attestation_1: &qrysmpb.IndexedAttestation{
						AttestingIndices: []uint64{0, 1, 2},
						Data: &qrysmpb.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &qrysmpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &qrysmpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, field_params.MLDSA87SignatureLength),
					},
					Attestation_2: &qrysmpb.IndexedAttestation{
						AttestingIndices: []uint64{0, 1, 2},
						Data: &qrysmpb.AttestationData{
							BeaconBlockRoot: make([]byte, fieldparams.RootLength),
							Source: &qrysmpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
							Target: &qrysmpb.Checkpoint{
								Root: make([]byte, fieldparams.RootLength),
							},
						},
						Signature: make([]byte, field_params.MLDSA87SignatureLength),
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
		block *qrysmpb.BeaconBlockAltair
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
				block: &qrysmpb.BeaconBlockAltair{
					Slot:          0,
					ProposerIndex: 0,
					ParentRoot:    make([]byte, fieldparams.RootLength),
					StateRoot:     make([]byte, fieldparams.RootLength),
					Body: &qrysmpb.BeaconBlockBodyAltair{
						RandaoReveal: make([]byte, 32),
						ExecutionData: &qrysmpb.ExecutionData{
							DepositRoot:  make([]byte, fieldparams.RootLength),
							DepositCount: 0,
							BlockHash:    make([]byte, 32),
						},
						Graffiti: make([]byte, 32),
						ProposerSlashings: []*qrysmpb.ProposerSlashing{
							{
								Header_1: &qrysmpb.SignedBeaconBlockHeader{
									Header: &qrysmpb.BeaconBlockHeader{
										Slot:          0,
										ProposerIndex: 0,
										ParentRoot:    make([]byte, fieldparams.RootLength),
										StateRoot:     make([]byte, fieldparams.RootLength),
										BodyRoot:      make([]byte, fieldparams.RootLength),
									},
									Signature: make([]byte, field_params.MLDSA87SignatureLength),
								},
								Header_2: &qrysmpb.SignedBeaconBlockHeader{
									Header: &qrysmpb.BeaconBlockHeader{
										Slot:          0,
										ProposerIndex: 0,
										ParentRoot:    make([]byte, fieldparams.RootLength),
										StateRoot:     make([]byte, fieldparams.RootLength),
										BodyRoot:      make([]byte, fieldparams.RootLength),
									},
									Signature: make([]byte, field_params.MLDSA87SignatureLength),
								},
							},
						},
						AttesterSlashings: []*qrysmpb.AttesterSlashing{
							{
								Attestation_1: &qrysmpb.IndexedAttestation{
									AttestingIndices: []uint64{0, 1, 2},
									Data: &qrysmpb.AttestationData{
										BeaconBlockRoot: make([]byte, fieldparams.RootLength),
										Source: &qrysmpb.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
										Target: &qrysmpb.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
									},
									Signature: make([]byte, field_params.MLDSA87SignatureLength),
								},
								Attestation_2: &qrysmpb.IndexedAttestation{
									AttestingIndices: []uint64{0, 1, 2},
									Data: &qrysmpb.AttestationData{
										BeaconBlockRoot: make([]byte, fieldparams.RootLength),
										Source: &qrysmpb.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
										Target: &qrysmpb.Checkpoint{
											Root: make([]byte, fieldparams.RootLength),
										},
									},
									Signature: make([]byte, field_params.MLDSA87SignatureLength),
								},
							},
						},
						Attestations: []*qrysmpb.Attestation{
							{
								AggregationBits: bitfield.Bitlist{0b1101},
								Data: &qrysmpb.AttestationData{
									BeaconBlockRoot: make([]byte, fieldparams.RootLength),
									Source: &qrysmpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
									Target: &qrysmpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
								},
								Signature: make([]byte, 4627),
							},
						},
						Deposits: []*qrysmpb.Deposit{
							{
								Proof: [][]byte{[]byte("A")},
								Data: &qrysmpb.Deposit_Data{
									PublicKey:             make([]byte, field_params.MLDSA87PubkeyLength),
									WithdrawalCredentials: make([]byte, 64),
									Amount:                0,
									Signature:             make([]byte, field_params.MLDSA87SignatureLength),
								},
							},
						},
						VoluntaryExits: []*qrysmpb.SignedVoluntaryExit{
							{
								Exit: &qrysmpb.VoluntaryExit{
									Epoch:          0,
									ValidatorIndex: 0,
								},
								Signature: make([]byte, field_params.MLDSA87SignatureLength),
							},
						},
						SyncAggregate: &qrysmpb.SyncAggregate{
							SyncCommitteeSignature: make([]byte, field_params.MLDSA87SignatureLength),
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
		body *qrysmpb.BeaconBlockBody
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
				body: &qrysmpb.BeaconBlockBody{
					RandaoReveal: make([]byte, 32),
					ExecutionData: &qrysmpb.ExecutionData{
						DepositRoot:  make([]byte, fieldparams.RootLength),
						DepositCount: 0,
						BlockHash:    make([]byte, 32),
					},
					Graffiti: make([]byte, 32),
					ProposerSlashings: []*qrysmpb.ProposerSlashing{
						{
							Header_1: &qrysmpb.SignedBeaconBlockHeader{
								Header: &qrysmpb.BeaconBlockHeader{
									Slot:          0,
									ProposerIndex: 0,
									ParentRoot:    make([]byte, fieldparams.RootLength),
									StateRoot:     make([]byte, fieldparams.RootLength),
									BodyRoot:      make([]byte, fieldparams.RootLength),
								},
								Signature: make([]byte, field_params.MLDSA87SignatureLength),
							},
							Header_2: &qrysmpb.SignedBeaconBlockHeader{
								Header: &qrysmpb.BeaconBlockHeader{
									Slot:          0,
									ProposerIndex: 0,
									ParentRoot:    make([]byte, fieldparams.RootLength),
									StateRoot:     make([]byte, fieldparams.RootLength),
									BodyRoot:      make([]byte, fieldparams.RootLength),
								},
								Signature: make([]byte, field_params.MLDSA87SignatureLength),
							},
						},
					},
					AttesterSlashings: []*qrysmpb.AttesterSlashing{
						{
							Attestation_1: &qrysmpb.IndexedAttestation{
								AttestingIndices: []uint64{0, 1, 2},
								Data: &qrysmpb.AttestationData{
									BeaconBlockRoot: make([]byte, fieldparams.RootLength),
									Source: &qrysmpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
									Target: &qrysmpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
								},
								Signature: make([]byte, field_params.MLDSA87SignatureLength),
							},
							Attestation_2: &qrysmpb.IndexedAttestation{
								AttestingIndices: []uint64{0, 1, 2},
								Data: &qrysmpb.AttestationData{
									BeaconBlockRoot: make([]byte, fieldparams.RootLength),
									Source: &qrysmpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
									Target: &qrysmpb.Checkpoint{
										Root: make([]byte, fieldparams.RootLength),
									},
								},
								Signature: make([]byte, field_params.MLDSA87SignatureLength),
							},
						},
					},
					Attestations: []*qrysmpb.Attestation{
						{
							AggregationBits: bitfield.Bitlist{0b1101},
							Data: &qrysmpb.AttestationData{
								BeaconBlockRoot: make([]byte, fieldparams.RootLength),
								Source: &qrysmpb.Checkpoint{
									Root: make([]byte, fieldparams.RootLength),
								},
								Target: &qrysmpb.Checkpoint{
									Root: make([]byte, fieldparams.RootLength),
								},
							},
							Signature: make([]byte, 4627),
						},
					},
					Deposits: []*qrysmpb.Deposit{
						{
							Proof: [][]byte{[]byte("A")},
							Data: &qrysmpb.Deposit_Data{
								PublicKey:             make([]byte, field_params.MLDSA87PubkeyLength),
								WithdrawalCredentials: make([]byte, 64),
								Amount:                0,
								Signature:             make([]byte, field_params.MLDSA87SignatureLength),
							},
						},
					},
					VoluntaryExits: []*qrysmpb.SignedVoluntaryExit{
						{
							Exit: &qrysmpb.VoluntaryExit{
								Epoch:          0,
								ValidatorIndex: 0,
							},
							Signature: make([]byte, field_params.MLDSA87SignatureLength),
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
		contribution *qrysmpb.ContributionAndProof
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
				contribution: &qrysmpb.ContributionAndProof{
					AggregatorIndex: 0,
					Contribution: &qrysmpb.SyncCommitteeContribution{
						Slot:              0,
						BlockRoot:         make([]byte, fieldparams.RootLength),
						SubcommitteeIndex: 0,
						AggregationBits:   mock.MockAggregationBits(),
						Signature:         make([]byte, field_params.MLDSA87SignatureLength),
					},
					SelectionProof: make([]byte, field_params.MLDSA87SignatureLength),
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
		data *qrysmpb.SyncAggregatorSelectionData
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
				data: &qrysmpb.SyncAggregatorSelectionData{
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
*/
