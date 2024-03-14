package blocks

import (
	"testing"

	"github.com/theQRL/go-bitfield"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

type fields struct {
	root                        [32]byte
	sig                         [field_params.DilithiumSignatureLength]byte
	deposits                    []*zond.Deposit
	atts                        []*zond.Attestation
	proposerSlashings           []*zond.ProposerSlashing
	attesterSlashings           []*zond.AttesterSlashing
	voluntaryExits              []*zond.SignedVoluntaryExit
	syncAggregate               *zond.SyncAggregate
	execPayloadCapella          *enginev1.ExecutionPayloadCapella
	execPayloadHeaderCapella    *enginev1.ExecutionPayloadHeaderCapella
	dilithiumToExecutionChanges []*zond.SignedDilithiumToExecutionChange
}

func Test_SignedBeaconBlock_Proto(t *testing.T) {
	f := getFields()

	t.Run("Capella", func(t *testing.T) {
		expectedBlock := &zond.SignedBeaconBlockCapella{
			Block: &zond.BeaconBlockCapella{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    f.root[:],
				StateRoot:     f.root[:],
				Body:          bodyPbCapella(),
			},
			Signature: f.sig[:],
		}
		block := &SignedBeaconBlock{
			version: version.Capella,
			block: &BeaconBlock{
				version:       version.Capella,
				slot:          128,
				proposerIndex: 128,
				parentRoot:    f.root,
				stateRoot:     f.root,
				body:          bodyCapella(t),
			},
			signature: f.sig,
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.SignedBeaconBlockCapella)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("CapellaBlind", func(t *testing.T) {
		expectedBlock := &zond.SignedBlindedBeaconBlockCapella{
			Block: &zond.BlindedBeaconBlockCapella{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    f.root[:],
				StateRoot:     f.root[:],
				Body:          bodyPbBlindedCapella(),
			},
			Signature: f.sig[:],
		}
		block := &SignedBeaconBlock{
			version: version.Capella,
			block: &BeaconBlock{
				version:       version.Capella,
				slot:          128,
				proposerIndex: 128,
				parentRoot:    f.root,
				stateRoot:     f.root,
				body:          bodyBlindedCapella(t),
			},
			signature: f.sig,
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.SignedBlindedBeaconBlockCapella)
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

	t.Run("Capella", func(t *testing.T) {
		expectedBlock := &zond.BeaconBlockCapella{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbCapella(),
		}
		block := &BeaconBlock{
			version:       version.Capella,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    f.root,
			stateRoot:     f.root,
			body:          bodyCapella(t),
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BeaconBlockCapella)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("CapellaBlind", func(t *testing.T) {
		expectedBlock := &zond.BlindedBeaconBlockCapella{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbBlindedCapella(),
		}
		block := &BeaconBlock{
			version:       version.Capella,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    f.root,
			stateRoot:     f.root,
			body:          bodyBlindedCapella(t),
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BlindedBeaconBlockCapella)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
}

func Test_BeaconBlockBody_Proto(t *testing.T) {
	t.Run("Capella", func(t *testing.T) {
		expectedBody := bodyPbCapella()
		body := bodyCapella(t)
		result, err := body.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BeaconBlockBodyCapella)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBody.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("CapellaBlind", func(t *testing.T) {
		expectedBody := bodyPbBlindedCapella()
		body := bodyBlindedCapella(t)
		result, err := body.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BlindedBeaconBlockBodyCapella)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBody.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("Capella - wrong payload type", func(t *testing.T) {
		body := bodyCapella(t)
		body.executionPayload = &executionPayloadHeaderCapella{}
		_, err := body.Proto()
		require.ErrorIs(t, err, errPayloadWrongType)
	})
	t.Run("CapellaBlind - wrong payload type", func(t *testing.T) {
		body := bodyBlindedCapella(t)
		body.executionPayloadHeader = &executionPayloadCapella{}
		_, err := body.Proto()
		require.ErrorIs(t, err, errPayloadHeaderWrongType)
	})
}

func Test_initSignedBlockFromProtoCapella(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.SignedBeaconBlockCapella{
		Block: &zond.BeaconBlockCapella{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbCapella(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initSignedBlockFromProtoCapella(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
}

func Test_initBlindedSignedBlockFromProtoCapella(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.SignedBlindedBeaconBlockCapella{
		Block: &zond.BlindedBeaconBlockCapella{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbBlindedCapella(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initBlindedSignedBlockFromProtoCapella(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
}

func Test_initBlockFromProtoCapella(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.BeaconBlockCapella{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbCapella(),
	}
	resultBlock, err := initBlockFromProtoCapella(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockFromProtoBlindedCapella(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.BlindedBeaconBlockCapella{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbBlindedCapella(),
	}
	resultBlock, err := initBlindedBlockFromProtoCapella(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockBodyFromProtoCapella(t *testing.T) {
	expectedBody := bodyPbCapella()
	resultBody, err := initBlockBodyFromProtoCapella(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockBodyFromProtoBlindedCapella(t *testing.T) {
	expectedBody := bodyPbBlindedCapella()
	resultBody, err := initBlindedBlockBodyFromProtoCapella(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func bodyPbCapella() *zond.BeaconBlockBodyCapella {
	f := getFields()
	return &zond.BeaconBlockBodyCapella{
		RandaoReveal: f.sig[:],
		Eth1Data: &zond.Eth1Data{
			DepositRoot:  f.root[:],
			DepositCount: 128,
			BlockHash:    f.root[:],
		},
		Graffiti:                    f.root[:],
		ProposerSlashings:           f.proposerSlashings,
		AttesterSlashings:           f.attesterSlashings,
		Attestations:                f.atts,
		Deposits:                    f.deposits,
		VoluntaryExits:              f.voluntaryExits,
		SyncAggregate:               f.syncAggregate,
		ExecutionPayload:            f.execPayloadCapella,
		DilithiumToExecutionChanges: f.dilithiumToExecutionChanges,
	}
}

func bodyPbBlindedCapella() *zond.BlindedBeaconBlockBodyCapella {
	f := getFields()
	return &zond.BlindedBeaconBlockBodyCapella{
		RandaoReveal: f.sig[:],
		Eth1Data: &zond.Eth1Data{
			DepositRoot:  f.root[:],
			DepositCount: 128,
			BlockHash:    f.root[:],
		},
		Graffiti:                    f.root[:],
		ProposerSlashings:           f.proposerSlashings,
		AttesterSlashings:           f.attesterSlashings,
		Attestations:                f.atts,
		Deposits:                    f.deposits,
		VoluntaryExits:              f.voluntaryExits,
		SyncAggregate:               f.syncAggregate,
		ExecutionPayloadHeader:      f.execPayloadHeaderCapella,
		DilithiumToExecutionChanges: f.dilithiumToExecutionChanges,
	}
}

func bodyCapella(t *testing.T) *BeaconBlockBody {
	f := getFields()
	p, err := WrappedExecutionPayloadCapella(f.execPayloadCapella, 0)
	require.NoError(t, err)
	return &BeaconBlockBody{
		version:      version.Capella,
		randaoReveal: f.sig,
		eth1Data: &zond.Eth1Data{
			DepositRoot:  f.root[:],
			DepositCount: 128,
			BlockHash:    f.root[:],
		},
		graffiti:                    f.root,
		proposerSlashings:           f.proposerSlashings,
		attesterSlashings:           f.attesterSlashings,
		attestations:                f.atts,
		deposits:                    f.deposits,
		voluntaryExits:              f.voluntaryExits,
		syncAggregate:               f.syncAggregate,
		executionPayload:            p,
		dilithiumToExecutionChanges: f.dilithiumToExecutionChanges,
	}
}

func bodyBlindedCapella(t *testing.T) *BeaconBlockBody {
	f := getFields()
	ph, err := WrappedExecutionPayloadHeaderCapella(f.execPayloadHeaderCapella, 0)
	require.NoError(t, err)
	return &BeaconBlockBody{
		version:      version.Capella,
		isBlinded:    true,
		randaoReveal: f.sig,
		eth1Data: &zond.Eth1Data{
			DepositRoot:  f.root[:],
			DepositCount: 128,
			BlockHash:    f.root[:],
		},
		graffiti:                    f.root,
		proposerSlashings:           f.proposerSlashings,
		attesterSlashings:           f.attesterSlashings,
		attestations:                f.atts,
		deposits:                    f.deposits,
		voluntaryExits:              f.voluntaryExits,
		syncAggregate:               f.syncAggregate,
		executionPayloadHeader:      ph,
		dilithiumToExecutionChanges: f.dilithiumToExecutionChanges,
	}
}

func getFields() fields {
	b20 := make([]byte, 20)
	b2592 := make([]byte, 2592)
	b256 := make([]byte, 256)
	var root [32]byte
	var sig [field_params.DilithiumSignatureLength]byte
	b20[0], b20[5], b20[10] = 'q', 'u', 'x'
	b2592[0], b2592[5], b2592[10] = 'b', 'a', 'r'
	b256[0], b256[5], b256[10] = 'x', 'y', 'z'
	root[0], root[5], root[10] = 'a', 'b', 'c'
	sig[0], sig[5], sig[10] = 'd', 'e', 'f'
	deposits := make([]*zond.Deposit, 16)
	for i := range deposits {
		deposits[i] = &zond.Deposit{}
		deposits[i].Proof = make([][]byte, 33)
		for j := range deposits[i].Proof {
			deposits[i].Proof[j] = root[:]
		}
		deposits[i].Data = &zond.Deposit_Data{
			PublicKey:             b2592,
			WithdrawalCredentials: root[:],
			Amount:                128,
			Signature:             sig[:],
		}
	}
	atts := make([]*zond.Attestation, 128)
	for i := range atts {
		atts[i] = &zond.Attestation{}
		atts[i].Signatures = [][]byte{sig[:]}
		atts[i].AggregationBits = bitfield.NewBitlist(1)
		atts[i].Data = &zond.AttestationData{
			Slot:            128,
			CommitteeIndex:  128,
			BeaconBlockRoot: root[:],
			Source: &zond.Checkpoint{
				Epoch: 128,
				Root:  root[:],
			},
			Target: &zond.Checkpoint{
				Epoch: 128,
				Root:  root[:],
			},
		}
	}
	proposerSlashing := &zond.ProposerSlashing{
		Header_1: &zond.SignedBeaconBlockHeader{
			Header: &zond.BeaconBlockHeader{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    root[:],
				StateRoot:     root[:],
				BodyRoot:      root[:],
			},
			Signature: sig[:],
		},
		Header_2: &zond.SignedBeaconBlockHeader{
			Header: &zond.BeaconBlockHeader{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    root[:],
				StateRoot:     root[:],
				BodyRoot:      root[:],
			},
			Signature: sig[:],
		},
	}
	attesterSlashing := &zond.AttesterSlashing{
		Attestation_1: &zond.IndexedAttestation{
			AttestingIndices: []uint64{1, 2, 8},
			Data: &zond.AttestationData{
				Slot:            128,
				CommitteeIndex:  128,
				BeaconBlockRoot: root[:],
				Source: &zond.Checkpoint{
					Epoch: 128,
					Root:  root[:],
				},
				Target: &zond.Checkpoint{
					Epoch: 128,
					Root:  root[:],
				},
			},
			Signatures: [][]byte{sig[:]},
		},
		Attestation_2: &zond.IndexedAttestation{
			AttestingIndices: []uint64{1, 2, 8},
			Data: &zond.AttestationData{
				Slot:            128,
				CommitteeIndex:  128,
				BeaconBlockRoot: root[:],
				Source: &zond.Checkpoint{
					Epoch: 128,
					Root:  root[:],
				},
				Target: &zond.Checkpoint{
					Epoch: 128,
					Root:  root[:],
				},
			},
			Signatures: [][]byte{sig[:]},
		},
	}
	voluntaryExit := &zond.SignedVoluntaryExit{
		Exit: &zond.VoluntaryExit{
			Epoch:          128,
			ValidatorIndex: 128,
		},
		Signature: sig[:],
	}
	syncCommitteeBits := bitfield.NewBitvector16()
	syncCommitteeBits.SetBitAt(1, true)
	syncCommitteeBits.SetBitAt(2, true)
	syncCommitteeBits.SetBitAt(8, true)
	syncAggregate := &zond.SyncAggregate{
		SyncCommitteeBits:       syncCommitteeBits,
		SyncCommitteeSignatures: [][]byte{sig[:]},
	}
	execPayloadCapella := &enginev1.ExecutionPayloadCapella{
		ParentHash:    root[:],
		FeeRecipient:  b20,
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
				Address: b20,
				Amount:  128,
			},
		},
	}
	execPayloadHeaderCapella := &enginev1.ExecutionPayloadHeaderCapella{
		ParentHash:       root[:],
		FeeRecipient:     b20,
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
	dilithiumToExecutionChanges := []*zond.SignedDilithiumToExecutionChange{{
		Message: &zond.DilithiumToExecutionChange{
			ValidatorIndex:      128,
			FromDilithiumPubkey: b2592,
			ToExecutionAddress:  b20,
		},
		Signature: sig[:],
	}}

	return fields{
		root:                        root,
		sig:                         sig,
		deposits:                    deposits,
		atts:                        atts,
		proposerSlashings:           []*zond.ProposerSlashing{proposerSlashing},
		attesterSlashings:           []*zond.AttesterSlashing{attesterSlashing},
		voluntaryExits:              []*zond.SignedVoluntaryExit{voluntaryExit},
		syncAggregate:               syncAggregate,
		execPayloadCapella:          execPayloadCapella,
		execPayloadHeaderCapella:    execPayloadHeaderCapella,
		dilithiumToExecutionChanges: dilithiumToExecutionChanges,
	}
}
