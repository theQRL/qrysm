package blocks

import (
	"testing"

	"github.com/theQRL/go-bitfield"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type fields struct {
	root                  [32]byte
	sig                   [field_params.MLDSA87SignatureLength]byte
	deposits              []*qrysmpb.Deposit
	atts                  []*qrysmpb.Attestation
	proposerSlashings     []*qrysmpb.ProposerSlashing
	attesterSlashings     []*qrysmpb.AttesterSlashing
	voluntaryExits        []*qrysmpb.SignedVoluntaryExit
	syncAggregate         *qrysmpb.SyncAggregate
	execPayloadZond       *enginev1.ExecutionPayloadZond
	execPayloadHeaderZond *enginev1.ExecutionPayloadHeaderZond
}

func Test_SignedBeaconBlock_Proto(t *testing.T) {
	f := getFields()

	t.Run("Zond", func(t *testing.T) {
		expectedBlock := &qrysmpb.SignedBeaconBlockZond{
			Block: &qrysmpb.BeaconBlockZond{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    f.root[:],
				StateRoot:     f.root[:],
				Body:          bodyPbZond(),
			},
			Signature: f.sig[:],
		}
		block := &SignedBeaconBlock{
			version: version.Zond,
			block: &BeaconBlock{
				version:       version.Zond,
				slot:          128,
				proposerIndex: 128,
				parentRoot:    f.root,
				stateRoot:     f.root,
				body:          bodyZond(t),
			},
			signature: f.sig,
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*qrysmpb.SignedBeaconBlockZond)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("ZondBlind", func(t *testing.T) {
		expectedBlock := &qrysmpb.SignedBlindedBeaconBlockZond{
			Block: &qrysmpb.BlindedBeaconBlockZond{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    f.root[:],
				StateRoot:     f.root[:],
				Body:          bodyPbBlindedZond(),
			},
			Signature: f.sig[:],
		}
		block := &SignedBeaconBlock{
			version: version.Zond,
			block: &BeaconBlock{
				version:       version.Zond,
				slot:          128,
				proposerIndex: 128,
				parentRoot:    f.root,
				stateRoot:     f.root,
				body:          bodyBlindedZond(t),
			},
			signature: f.sig,
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*qrysmpb.SignedBlindedBeaconBlockZond)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
}

func Test_BeaconBlock_Proto(t *testing.T) {
	f := getFields()

	t.Run("Zond", func(t *testing.T) {
		expectedBlock := &qrysmpb.BeaconBlockZond{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbZond(),
		}
		block := &BeaconBlock{
			version:       version.Zond,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    f.root,
			stateRoot:     f.root,
			body:          bodyZond(t),
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*qrysmpb.BeaconBlockZond)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("ZondBlind", func(t *testing.T) {
		expectedBlock := &qrysmpb.BlindedBeaconBlockZond{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbBlindedZond(),
		}
		block := &BeaconBlock{
			version:       version.Zond,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    f.root,
			stateRoot:     f.root,
			body:          bodyBlindedZond(t),
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*qrysmpb.BlindedBeaconBlockZond)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
}

func Test_BeaconBlockBody_Proto(t *testing.T) {
	t.Run("Zond", func(t *testing.T) {
		expectedBody := bodyPbZond()
		body := bodyZond(t)
		result, err := body.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*qrysmpb.BeaconBlockBodyZond)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBody.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("ZondBlind", func(t *testing.T) {
		expectedBody := bodyPbBlindedZond()
		body := bodyBlindedZond(t)
		result, err := body.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*qrysmpb.BlindedBeaconBlockBodyZond)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBody.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("Zond - wrong payload type", func(t *testing.T) {
		body := bodyZond(t)
		body.executionPayload = &executionPayloadHeaderZond{}
		_, err := body.Proto()
		require.ErrorIs(t, err, errPayloadWrongType)
	})
	t.Run("ZondBlind - wrong payload type", func(t *testing.T) {
		body := bodyBlindedZond(t)
		body.executionPayloadHeader = &executionPayloadZond{}
		_, err := body.Proto()
		require.ErrorIs(t, err, errPayloadHeaderWrongType)
	})
}

func Test_initSignedBlockFromProtoZond(t *testing.T) {
	f := getFields()
	expectedBlock := &qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbZond(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initSignedBlockFromProtoZond(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
}

func Test_initBlindedSignedBlockFromProtoZond(t *testing.T) {
	f := getFields()
	expectedBlock := &qrysmpb.SignedBlindedBeaconBlockZond{
		Block: &qrysmpb.BlindedBeaconBlockZond{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbBlindedZond(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initBlindedSignedBlockFromProtoZond(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
}

func Test_initBlockFromProtoZond(t *testing.T) {
	f := getFields()
	expectedBlock := &qrysmpb.BeaconBlockZond{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbZond(),
	}
	resultBlock, err := initBlockFromProtoZond(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockFromProtoBlindedZond(t *testing.T) {
	f := getFields()
	expectedBlock := &qrysmpb.BlindedBeaconBlockZond{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbBlindedZond(),
	}
	resultBlock, err := initBlindedBlockFromProtoZond(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockBodyFromProtoZond(t *testing.T) {
	expectedBody := bodyPbZond()
	resultBody, err := initBlockBodyFromProtoZond(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockBodyFromProtoBlindedZond(t *testing.T) {
	expectedBody := bodyPbBlindedZond()
	resultBody, err := initBlindedBlockBodyFromProtoZond(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func bodyPbZond() *qrysmpb.BeaconBlockBodyZond {
	f := getFields()
	return &qrysmpb.BeaconBlockBodyZond{
		RandaoReveal: f.sig[:],
		ExecutionData: &qrysmpb.ExecutionData{
			DepositRoot:  f.root[:],
			DepositCount: 128,
			BlockHash:    f.root[:],
		},
		Graffiti:          f.root[:],
		ProposerSlashings: f.proposerSlashings,
		AttesterSlashings: f.attesterSlashings,
		Attestations:      f.atts,
		Deposits:          f.deposits,
		VoluntaryExits:    f.voluntaryExits,
		SyncAggregate:     f.syncAggregate,
		ExecutionPayload:  f.execPayloadZond,
	}
}

func bodyPbBlindedZond() *qrysmpb.BlindedBeaconBlockBodyZond {
	f := getFields()
	return &qrysmpb.BlindedBeaconBlockBodyZond{
		RandaoReveal: f.sig[:],
		ExecutionData: &qrysmpb.ExecutionData{
			DepositRoot:  f.root[:],
			DepositCount: 128,
			BlockHash:    f.root[:],
		},
		Graffiti:               f.root[:],
		ProposerSlashings:      f.proposerSlashings,
		AttesterSlashings:      f.attesterSlashings,
		Attestations:           f.atts,
		Deposits:               f.deposits,
		VoluntaryExits:         f.voluntaryExits,
		SyncAggregate:          f.syncAggregate,
		ExecutionPayloadHeader: f.execPayloadHeaderZond,
	}
}

func bodyZond(t *testing.T) *BeaconBlockBody {
	f := getFields()
	p, err := WrappedExecutionPayloadZond(f.execPayloadZond, 0)
	require.NoError(t, err)
	return &BeaconBlockBody{
		version:      version.Zond,
		randaoReveal: f.sig,
		executionData: &qrysmpb.ExecutionData{
			DepositRoot:  f.root[:],
			DepositCount: 128,
			BlockHash:    f.root[:],
		},
		graffiti:          f.root,
		proposerSlashings: f.proposerSlashings,
		attesterSlashings: f.attesterSlashings,
		attestations:      f.atts,
		deposits:          f.deposits,
		voluntaryExits:    f.voluntaryExits,
		syncAggregate:     f.syncAggregate,
		executionPayload:  p,
	}
}

func bodyBlindedZond(t *testing.T) *BeaconBlockBody {
	f := getFields()
	ph, err := WrappedExecutionPayloadHeaderZond(f.execPayloadHeaderZond, 0)
	require.NoError(t, err)
	return &BeaconBlockBody{
		version:      version.Zond,
		isBlinded:    true,
		randaoReveal: f.sig,
		executionData: &qrysmpb.ExecutionData{
			DepositRoot:  f.root[:],
			DepositCount: 128,
			BlockHash:    f.root[:],
		},
		graffiti:               f.root,
		proposerSlashings:      f.proposerSlashings,
		attesterSlashings:      f.attesterSlashings,
		attestations:           f.atts,
		deposits:               f.deposits,
		voluntaryExits:         f.voluntaryExits,
		syncAggregate:          f.syncAggregate,
		executionPayloadHeader: ph,
	}
}

func getFields() fields {
	b64 := make([]byte, field_params.FeeRecipientLength)
	b2592 := make([]byte, 2592)
	b256 := make([]byte, 256)
	var root [32]byte
	var creds [field_params.WithdrawalCredentialsLength]byte
	var sig [field_params.MLDSA87SignatureLength]byte
	b64[0], b64[5], b64[10] = 'q', 'u', 'x'
	b2592[0], b2592[5], b2592[10] = 'b', 'a', 'r'
	b256[0], b256[5], b256[10] = 'x', 'y', 'z'
	root[0], root[5], root[10] = 'a', 'b', 'c'
	creds[0], creds[5], creds[10] = 'a', 'b', 'c'
	sig[0], sig[5], sig[10] = 'd', 'e', 'f'
	deposits := make([]*qrysmpb.Deposit, 16)
	for i := range deposits {
		deposits[i] = &qrysmpb.Deposit{}
		deposits[i].Proof = make([][]byte, 33)
		for j := range deposits[i].Proof {
			deposits[i].Proof[j] = root[:]
		}
		deposits[i].Data = &qrysmpb.Deposit_Data{
			PublicKey:             b2592,
			WithdrawalCredentials: creds[:],
			Amount:                128,
			Signature:             sig[:],
		}
	}
	atts := make([]*qrysmpb.Attestation, 128)
	for i := range atts {
		atts[i] = &qrysmpb.Attestation{}
		atts[i].Signatures = [][]byte{sig[:]}
		atts[i].AggregationBits = bitfield.NewBitlist(1)
		atts[i].Data = &qrysmpb.AttestationData{
			Slot:            128,
			CommitteeIndex:  128,
			BeaconBlockRoot: root[:],
			Source: &qrysmpb.Checkpoint{
				Epoch: 128,
				Root:  root[:],
			},
			Target: &qrysmpb.Checkpoint{
				Epoch: 128,
				Root:  root[:],
			},
		}
	}
	proposerSlashing := &qrysmpb.ProposerSlashing{
		Header_1: &qrysmpb.SignedBeaconBlockHeader{
			Header: &qrysmpb.BeaconBlockHeader{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    root[:],
				StateRoot:     root[:],
				BodyRoot:      root[:],
			},
			Signature: sig[:],
		},
		Header_2: &qrysmpb.SignedBeaconBlockHeader{
			Header: &qrysmpb.BeaconBlockHeader{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    root[:],
				StateRoot:     root[:],
				BodyRoot:      root[:],
			},
			Signature: sig[:],
		},
	}
	attesterSlashing := &qrysmpb.AttesterSlashing{
		Attestation_1: &qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{1, 2, 8},
			Data: &qrysmpb.AttestationData{
				Slot:            128,
				CommitteeIndex:  128,
				BeaconBlockRoot: root[:],
				Source: &qrysmpb.Checkpoint{
					Epoch: 128,
					Root:  root[:],
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 128,
					Root:  root[:],
				},
			},
			Signatures: [][]byte{sig[:]},
		},
		Attestation_2: &qrysmpb.IndexedAttestation{
			AttestingIndices: []uint64{1, 2, 8},
			Data: &qrysmpb.AttestationData{
				Slot:            128,
				CommitteeIndex:  128,
				BeaconBlockRoot: root[:],
				Source: &qrysmpb.Checkpoint{
					Epoch: 128,
					Root:  root[:],
				},
				Target: &qrysmpb.Checkpoint{
					Epoch: 128,
					Root:  root[:],
				},
			},
			Signatures: [][]byte{sig[:]},
		},
	}
	voluntaryExit := &qrysmpb.SignedVoluntaryExit{
		Exit: &qrysmpb.VoluntaryExit{
			Epoch:          128,
			ValidatorIndex: 128,
		},
		Signature: sig[:],
	}
	syncCommitteeBits := bitfield.NewBitvector128()
	syncCommitteeBits.SetBitAt(1, true)
	syncCommitteeBits.SetBitAt(2, true)
	syncCommitteeBits.SetBitAt(8, true)
	syncAggregate := &qrysmpb.SyncAggregate{
		SyncCommitteeBits:       syncCommitteeBits,
		SyncCommitteeSignatures: [][]byte{sig[:]},
	}
	execPayloadZond := &enginev1.ExecutionPayloadZond{
		ParentHash:    root[:],
		FeeRecipient:  b64,
		StateRoot:     root[:],
		ReceiptsRoot:  root[:],
		LogsBloom:     b256,
		PrevRandao:    root[:],
		BlockNumber:   128,
		GasLimit:      128,
		GasUsed:       128,
		Timestamp:     128,
		ExtraData:     root[:],
		BaseFeePerGas: root[:],
		BlockHash:     root[:],
		Transactions: [][]byte{
			[]byte("transaction1"),
			[]byte("transaction2"),
			[]byte("transaction8"),
		},
		Withdrawals: []*enginev1.Withdrawal{
			{
				Index:   128,
				Address: b64,
				Amount:  128,
			},
		},
	}
	execPayloadHeaderZond := &enginev1.ExecutionPayloadHeaderZond{
		ParentHash:       root[:],
		FeeRecipient:     b64,
		StateRoot:        root[:],
		ReceiptsRoot:     root[:],
		LogsBloom:        b256,
		PrevRandao:       root[:],
		BlockNumber:      128,
		GasLimit:         128,
		GasUsed:          128,
		Timestamp:        128,
		ExtraData:        root[:],
		BaseFeePerGas:    root[:],
		BlockHash:        root[:],
		TransactionsRoot: root[:],
		WithdrawalsRoot:  root[:],
	}

	return fields{
		root:                  root,
		sig:                   sig,
		deposits:              deposits,
		atts:                  atts,
		proposerSlashings:     []*qrysmpb.ProposerSlashing{proposerSlashing},
		attesterSlashings:     []*qrysmpb.AttesterSlashing{attesterSlashing},
		voluntaryExits:        []*qrysmpb.SignedVoluntaryExit{voluntaryExit},
		syncAggregate:         syncAggregate,
		execPayloadZond:       execPayloadZond,
		execPayloadHeaderZond: execPayloadHeaderZond,
	}
}
