package attestation_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87/common"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestConvertToIndexed(t *testing.T) {
	type args struct {
		attestation *qrysmpb.Attestation
		committee   []primitives.ValidatorIndex
	}
	tests := []struct {
		name string
		args args
		want *qrysmpb.IndexedAttestation
		err  string
	}{
		{
			name: "missing signatures",
			args: args{
				attestation: &qrysmpb.Attestation{
					AggregationBits: bitfield.Bitlist{0b1111},
					Signatures:      [][]byte{[]byte("sig0"), []byte("sig1")},
				},
				committee: []primitives.ValidatorIndex{25, 30, 17},
			},
			err: "signatures length 2 is not equal to the attesting participants indices length 3",
		},
		{
			name: "Invalid bit length",
			args: args{
				attestation: &qrysmpb.Attestation{
					AggregationBits: bitfield.Bitlist{0b11111},
					Signatures:      [][]byte{[]byte("sig0"), []byte("sig1"), []byte("sig2")},
				},
				committee: []primitives.ValidatorIndex{0, 1, 2},
			},
			err: "bitfield length 4 is not equal to committee length 3",
		},
		{
			name: "Full committee attested",
			args: args{
				attestation: &qrysmpb.Attestation{
					AggregationBits: bitfield.Bitlist{0b1111},
					Signatures:      [][]byte{[]byte("sig0"), []byte("sig1"), []byte("sig2")},
				},
				committee: []primitives.ValidatorIndex{25, 30, 17},
			},
			want: &qrysmpb.IndexedAttestation{
				AttestingIndices: []uint64{17, 25, 30},
				Signatures:       [][]byte{[]byte("sig2"), []byte("sig0"), []byte("sig1")},
			},
		},
		{
			name: "Partial committee attested",
			args: args{
				attestation: &qrysmpb.Attestation{
					AggregationBits: bitfield.Bitlist{0b1101},
					Signatures:      [][]byte{[]byte("sig0"), []byte("sig2")},
				},
				committee: []primitives.ValidatorIndex{40, 50, 60},
			},
			want: &qrysmpb.IndexedAttestation{
				AttestingIndices: []uint64{40, 60},
				Signatures:       [][]byte{[]byte("sig0"), []byte("sig2")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := attestation.ConvertToIndexed(context.Background(), tt.args.attestation, tt.args.committee)
			if tt.err == "" {
				require.NoError(t, err)
				assert.DeepEqual(t, tt.want, got)
			} else {
				require.ErrorContains(t, tt.err, err)
			}
		})
	}

}

func TestAttestingIndices(t *testing.T) {
	type args struct {
		bf        bitfield.Bitfield
		committee []primitives.ValidatorIndex
	}
	tests := []struct {
		name string
		args args
		want []uint64
		err  string
	}{
		{
			name: "Full committee attested",
			args: args{
				bf:        bitfield.Bitlist{0b1111},
				committee: []primitives.ValidatorIndex{0, 1, 2},
			},
			want: []uint64{0, 1, 2},
		},
		{
			name: "Partial committee attested",
			args: args{
				bf:        bitfield.Bitlist{0b1101},
				committee: []primitives.ValidatorIndex{0, 1, 2},
			},
			want: []uint64{0, 2},
		},
		{
			name: "Partial committee attested - validator index order",
			args: args{
				bf:        bitfield.Bitlist{0b1101},
				committee: []primitives.ValidatorIndex{0, 2, 1},
			},
			want: []uint64{0, 1},
		},
		{
			name: "Invalid bit length",
			args: args{
				bf:        bitfield.Bitlist{0b11111},
				committee: []primitives.ValidatorIndex{0, 1, 2},
			},
			err: "bitfield length 4 is not equal to committee length 3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := attestation.AttestingIndices(tt.args.bf, tt.args.committee)
			if tt.err == "" {
				require.NoError(t, err)
				assert.DeepEqual(t, tt.want, got)
			} else {
				require.ErrorContains(t, tt.err, err)
			}
		})
	}
}

func TestIsValidAttestationIndices(t *testing.T) {
	tests := []struct {
		name      string
		att       *qrysmpb.IndexedAttestation
		wantedErr string
	}{
		{
			name: "Indices should not be nil",
			att: &qrysmpb.IndexedAttestation{
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{},
					Source: &qrysmpb.Checkpoint{},
				},
				Signatures: [][]byte{},
			},
			wantedErr: "expected non-empty attesting indices",
		},
		{
			name: "Indices should be non-empty",
			att: &qrysmpb.IndexedAttestation{
				AttestingIndices: []uint64{},
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{},
					Source: &qrysmpb.Checkpoint{},
				},
				Signatures: [][]byte{},
			},
			wantedErr: "expected non-empty",
		},
		{
			name: "Greater than max validators per committee",
			att: &qrysmpb.IndexedAttestation{
				AttestingIndices: make([]uint64, params.BeaconConfig().MaxValidatorsPerCommittee+1),
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{},
					Source: &qrysmpb.Checkpoint{},
				},
				Signatures: [][]byte{},
			},
			wantedErr: "indices count exceeds",
		},
		{
			name: "Needs to be sorted",
			att: &qrysmpb.IndexedAttestation{
				AttestingIndices: []uint64{3, 2, 1},
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{},
					Source: &qrysmpb.Checkpoint{},
				},
				Signatures: [][]byte{},
			},
			wantedErr: "not uniquely sorted",
		},
		{
			name: "unique indices",
			att: &qrysmpb.IndexedAttestation{
				AttestingIndices: []uint64{1, 2, 3, 3},
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{},
					Source: &qrysmpb.Checkpoint{},
				},
				Signatures: [][]byte{},
			},
			wantedErr: "not uniquely sorted",
		},
		{
			name: "Valid indices",
			att: &qrysmpb.IndexedAttestation{
				AttestingIndices: []uint64{1, 2, 3},
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{},
					Source: &qrysmpb.Checkpoint{},
				},
				Signatures: [][]byte{},
			},
		},
		{
			name: "Valid indices with length of 2",
			att: &qrysmpb.IndexedAttestation{
				AttestingIndices: []uint64{1, 2},
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{},
					Source: &qrysmpb.Checkpoint{},
				},
				Signatures: [][]byte{},
			},
		},
		{
			name: "Valid indices with length of 1",
			att: &qrysmpb.IndexedAttestation{
				AttestingIndices: []uint64{1},
				Data: &qrysmpb.AttestationData{
					Target: &qrysmpb.Checkpoint{},
					Source: &qrysmpb.Checkpoint{},
				},
				Signatures: [][]byte{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := attestation.IsValidAttestationIndices(context.Background(), tt.att)
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func BenchmarkAttestingIndices_PartialCommittee(b *testing.B) {
	bf := bitfield.Bitlist{0b11111111, 0b11111111, 0b10000111, 0b11111111, 0b100}
	committee := []primitives.ValidatorIndex{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34}

	for b.Loop() {
		_, err := attestation.AttestingIndices(bf, committee)
		require.NoError(b, err)
	}
}

func BenchmarkIsValidAttestationIndices(b *testing.B) {
	indices := make([]uint64, params.BeaconConfig().MaxValidatorsPerCommittee)
	for i := range indices {
		indices[i] = uint64(i)
	}
	att := &qrysmpb.IndexedAttestation{
		AttestingIndices: indices,
		Data: &qrysmpb.AttestationData{
			Target: &qrysmpb.Checkpoint{},
			Source: &qrysmpb.Checkpoint{},
		},
		Signatures: [][]byte{},
	}

	for b.Loop() {
		if err := attestation.IsValidAttestationIndices(context.Background(), att); err != nil {
			require.NoError(b, err)
		}
	}
}

func TestAttDataIsEqual(t *testing.T) {
	type test struct {
		name     string
		attData1 *qrysmpb.AttestationData
		attData2 *qrysmpb.AttestationData
		equal    bool
	}
	tests := []test{
		{
			name: "same",
			attData1: &qrysmpb.AttestationData{
				Slot:            5,
				CommitteeIndex:  2,
				BeaconBlockRoot: []byte("great block"),
				Source: &qrysmpb.Checkpoint{
					Epoch: 4,
					Root:  []byte("good source"),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 10,
					Root:  []byte("good target"),
				},
			},
			attData2: &qrysmpb.AttestationData{
				Slot:            5,
				CommitteeIndex:  2,
				BeaconBlockRoot: []byte("great block"),
				Source: &qrysmpb.Checkpoint{
					Epoch: 4,
					Root:  []byte("good source"),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 10,
					Root:  []byte("good target"),
				},
			},
			equal: true,
		},
		{
			name: "diff slot",
			attData1: &qrysmpb.AttestationData{
				Slot:            5,
				CommitteeIndex:  2,
				BeaconBlockRoot: []byte("great block"),
				Source: &qrysmpb.Checkpoint{
					Epoch: 4,
					Root:  []byte("good source"),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 10,
					Root:  []byte("good target"),
				},
			},
			attData2: &qrysmpb.AttestationData{
				Slot:            4,
				CommitteeIndex:  2,
				BeaconBlockRoot: []byte("great block"),
				Source: &qrysmpb.Checkpoint{
					Epoch: 4,
					Root:  []byte("good source"),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 10,
					Root:  []byte("good target"),
				},
			},
		},
		{
			name: "diff block",
			attData1: &qrysmpb.AttestationData{
				Slot:            5,
				CommitteeIndex:  2,
				BeaconBlockRoot: []byte("good block"),
				Source: &qrysmpb.Checkpoint{
					Epoch: 4,
					Root:  []byte("good source"),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 10,
					Root:  []byte("good target"),
				},
			},
			attData2: &qrysmpb.AttestationData{
				Slot:            5,
				CommitteeIndex:  2,
				BeaconBlockRoot: []byte("great block"),
				Source: &qrysmpb.Checkpoint{
					Epoch: 4,
					Root:  []byte("good source"),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 10,
					Root:  []byte("good target"),
				},
			},
		},
		{
			name: "diff source root",
			attData1: &qrysmpb.AttestationData{
				Slot:            5,
				CommitteeIndex:  2,
				BeaconBlockRoot: []byte("great block"),
				Source: &qrysmpb.Checkpoint{
					Epoch: 4,
					Root:  []byte("good source"),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 10,
					Root:  []byte("good target"),
				},
			},
			attData2: &qrysmpb.AttestationData{
				Slot:            5,
				CommitteeIndex:  2,
				BeaconBlockRoot: []byte("great block"),
				Source: &qrysmpb.Checkpoint{
					Epoch: 4,
					Root:  []byte("bad source"),
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 10,
					Root:  []byte("good target"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.equal, attestation.AttDataIsEqual(tt.attData1, tt.attData2))
		})
	}
}

func TestCheckPointIsEqual(t *testing.T) {
	type test struct {
		name     string
		checkPt1 *qrysmpb.Checkpoint
		checkPt2 *qrysmpb.Checkpoint
		equal    bool
	}
	tests := []test{
		{
			name: "same",
			checkPt1: &qrysmpb.Checkpoint{
				Epoch: 4,
				Root:  []byte("good source"),
			},
			checkPt2: &qrysmpb.Checkpoint{
				Epoch: 4,
				Root:  []byte("good source"),
			},
			equal: true,
		},
		{
			name: "diff epoch",
			checkPt1: &qrysmpb.Checkpoint{
				Epoch: 4,
				Root:  []byte("good source"),
			},
			checkPt2: &qrysmpb.Checkpoint{
				Epoch: 5,
				Root:  []byte("good source"),
			},
			equal: false,
		},
		{
			name: "diff root",
			checkPt1: &qrysmpb.Checkpoint{
				Epoch: 4,
				Root:  []byte("good source"),
			},
			checkPt2: &qrysmpb.Checkpoint{
				Epoch: 4,
				Root:  []byte("bad source"),
			},
			equal: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.equal, attestation.CheckPointIsEqual(tt.checkPt1, tt.checkPt2))
		})
	}
}

func BenchmarkAttDataIsEqual(b *testing.B) {
	attData1 := &qrysmpb.AttestationData{
		Slot:            5,
		CommitteeIndex:  2,
		BeaconBlockRoot: []byte("great block"),
		Source: &qrysmpb.Checkpoint{
			Epoch: 4,
			Root:  []byte("good source"),
		},
		Target: &qrysmpb.Checkpoint{
			Epoch: 10,
			Root:  []byte("good target"),
		},
	}
	attData2 := &qrysmpb.AttestationData{
		Slot:            5,
		CommitteeIndex:  2,
		BeaconBlockRoot: []byte("great block"),
		Source: &qrysmpb.Checkpoint{
			Epoch: 4,
			Root:  []byte("good source"),
		},
		Target: &qrysmpb.Checkpoint{
			Epoch: 10,
			Root:  []byte("good target"),
		},
	}

	b.Run("fast", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			assert.Equal(b, true, attestation.AttDataIsEqual(attData1, attData2))
		}
	})

	b.Run("proto.Equal", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			assert.Equal(b, true, attestation.AttDataIsEqual(attData1, attData2))
		}
	})
}

func TestVerifyIndexedAttestationSigs(t *testing.T) {
	sk0, err := ml_dsa_87.SecretKeyFromSeed(bytes.Repeat([]byte{0x11}, 48))
	require.NoError(t, err)
	sk1, err := ml_dsa_87.SecretKeyFromSeed(bytes.Repeat([]byte{0x22}, 48))
	require.NoError(t, err)
	pk0 := sk0.PublicKey()
	pk1 := sk1.PublicKey()
	signingRoot, err := signing.ComputeSigningRoot(&qrysmpb.AttestationData{
		Slot:            5,
		CommitteeIndex:  2,
		BeaconBlockRoot: []byte("11111111111111111111111111111111"), // 32 bytes
		Source: &qrysmpb.Checkpoint{
			Epoch: 4,
			Root:  []byte("11111111111111111111111111111111"), // 32 bytes
		},
		Target: &qrysmpb.Checkpoint{
			Epoch: 10,
			Root:  []byte("11111111111111111111111111111111"), // 32 bytes
		},
	}, []byte("11111111111111111111111111111111")) // 32 bytes
	require.NoError(t, err)
	rawSig0 := sk0.Sign(signingRoot[:]).Marshal()
	rawSig1 := sk1.Sign(signingRoot[:]).Marshal()

	type args struct {
		idxAtt  *qrysmpb.IndexedAttestation
		pubKeys []common.PublicKey
		domain  []byte
	}
	tests := []struct {
		name string
		args args
		err  string
	}{
		{
			name: "nothing to verify",
			args: args{
				idxAtt: &qrysmpb.IndexedAttestation{
					AttestingIndices: []uint64{0, 1},
					Data: &qrysmpb.AttestationData{
						Slot:            5,
						CommitteeIndex:  2,
						BeaconBlockRoot: []byte("11111111111111111111111111111111"), // 32 bytes
						Source: &qrysmpb.Checkpoint{
							Epoch: 4,
							Root:  []byte("11111111111111111111111111111111"), // 32 bytes
						},
						Target: &qrysmpb.Checkpoint{
							Epoch: 10,
							Root:  []byte("11111111111111111111111111111111"), // 32 bytes
						},
					},
					Signatures: [][]byte{},
				},
				pubKeys: []common.PublicKey{},
				domain:  []byte("11111111111111111111111111111111"), // 32 bytes
			},
		},
		{
			name: "missing signature",
			args: args{
				idxAtt: &qrysmpb.IndexedAttestation{
					AttestingIndices: []uint64{0, 1},
					Data: &qrysmpb.AttestationData{
						Slot:            5,
						CommitteeIndex:  2,
						BeaconBlockRoot: []byte("11111111111111111111111111111111"), // 32 bytes
						Source: &qrysmpb.Checkpoint{
							Epoch: 4,
							Root:  []byte("11111111111111111111111111111111"), // 32 bytes
						},
						Target: &qrysmpb.Checkpoint{
							Epoch: 10,
							Root:  []byte("11111111111111111111111111111111"), // 32 bytes
						},
					},
					Signatures: [][]byte{rawSig0},
				},
				pubKeys: []common.PublicKey{pk0, pk1},
				domain:  []byte("11111111111111111111111111111111"), // 32 bytes
			},
			err: "signatures length 1 is not equal to pub keys length 2",
		},
		{
			name: "missing pubkey",
			args: args{
				idxAtt: &qrysmpb.IndexedAttestation{
					AttestingIndices: []uint64{0, 1},
					Data: &qrysmpb.AttestationData{
						Slot:            5,
						CommitteeIndex:  2,
						BeaconBlockRoot: []byte("11111111111111111111111111111111"), // 32 bytes
						Source: &qrysmpb.Checkpoint{
							Epoch: 4,
							Root:  []byte("11111111111111111111111111111111"), // 32 bytes
						},
						Target: &qrysmpb.Checkpoint{
							Epoch: 10,
							Root:  []byte("11111111111111111111111111111111"), // 32 bytes
						},
					},
					Signatures: [][]byte{rawSig0, rawSig1},
				},
				pubKeys: []common.PublicKey{pk0},
				domain:  []byte("11111111111111111111111111111111"), // 32 bytes
			},
			err: "signatures length 2 is not equal to pub keys length 1",
		},
		{
			name: "valid Indexed Attestation",
			args: args{
				idxAtt: &qrysmpb.IndexedAttestation{
					AttestingIndices: []uint64{0, 1},
					Data: &qrysmpb.AttestationData{
						Slot:            5,
						CommitteeIndex:  2,
						BeaconBlockRoot: []byte("11111111111111111111111111111111"), // 32 bytes
						Source: &qrysmpb.Checkpoint{
							Epoch: 4,
							Root:  []byte("11111111111111111111111111111111"), // 32 bytes
						},
						Target: &qrysmpb.Checkpoint{
							Epoch: 10,
							Root:  []byte("11111111111111111111111111111111"), // 32 bytes
						},
					},
					Signatures: [][]byte{rawSig0, rawSig1},
				},
				pubKeys: []common.PublicKey{pk0, pk1},
				domain:  []byte("11111111111111111111111111111111"), // 32 bytes
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := attestation.VerifyIndexedAttestationSigs(context.Background(), tt.args.idxAtt, tt.args.pubKeys, tt.args.domain)
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, tt.err, err)
			}
		})
	}
}

func TestSearchInsertIdxWithOffset(t *testing.T) {
	type args struct {
		slc        []int
		initialIdx int
		target     int
	}
	tests := []struct {
		name string
		args args
		err  string
		want int
	}{
		{
			args: args{
				slc:        []int{},
				initialIdx: 0,
			},
			want: 0,
		},
		{
			args: args{
				slc:        []int{0, 10, 12, 26},
				initialIdx: 4,
			},
			err: "invalid initial index 4 for slice length 4",
		},
		{
			args: args{
				slc:        []int{5, 10, 12, 26},
				initialIdx: 0,
				target:     4,
			},
			want: 0,
		},
		{
			args: args{
				slc:        []int{5, 10, 12, 26},
				initialIdx: 2,
				target:     4,
			},
			want: 2,
		},
		{
			args: args{
				slc:        []int{5, 10, 12, 26},
				initialIdx: 0,
				target:     28,
			},
			want: 4,
		},
		{
			args: args{
				slc:        []int{5, 10, 12, 26},
				initialIdx: 0,
				target:     13,
			},
			want: 3,
		},
		{
			args: args{
				slc:        []int{5, 10, 12, 26},
				initialIdx: 2,
				target:     13,
			},
			want: 3,
		},
		{
			args: args{
				slc:        []int{5, 10, 12, 26},
				initialIdx: 3,
				target:     13,
			},
			want: 3,
		},
		{
			args: args{
				slc:        []int{5, 10, 12, 26},
				initialIdx: 2,
				target:     11,
			},
			want: 2,
		},
		{
			args: args{
				slc:        []int{5, 10, 11, 12, 26},
				initialIdx: 3,
				target:     13,
			},
			want: 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := attestation.SearchInsertIdxWithOffset(tt.args.slc, tt.args.initialIdx, tt.args.target)
			if tt.err == "" {
				require.NoError(t, err)
				assert.DeepEqual(t, tt.want, got)
			} else {
				require.ErrorContains(t, tt.err, err)
			}
		})
	}
}
