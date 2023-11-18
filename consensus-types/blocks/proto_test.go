package blocks

import (
	"testing"

	"github.com/prysmaticlabs/go-bitfield"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
)

type fields struct {
	root                        [32]byte
	sig                         [dilithium2.CryptoBytes]byte
	deposits                    []*zond.Deposit
	atts                        []*zond.Attestation
	proposerSlashings           []*zond.ProposerSlashing
	attesterSlashings           []*zond.AttesterSlashing
	voluntaryExits              []*zond.SignedVoluntaryExit
	syncAggregate               *zond.SyncAggregate
	execPayload                 *enginev1.ExecutionPayload
	execPayloadHeader           *enginev1.ExecutionPayloadHeader
	execPayloadCapella          *enginev1.ExecutionPayloadCapella
	execPayloadHeaderCapella    *enginev1.ExecutionPayloadHeaderCapella
	execPayloadDeneb            *enginev1.ExecutionPayloadDeneb
	execPayloadHeaderDeneb      *enginev1.ExecutionPayloadHeaderDeneb
	dilithiumToExecutionChanges []*zond.SignedDilithiumToExecutionChange
	kzgCommitments              [][]byte
}

func Test_SignedBeaconBlock_Proto(t *testing.T) {
	f := getFields()

	t.Run("Phase0", func(t *testing.T) {
		expectedBlock := &zond.SignedBeaconBlock{
			Block: &zond.BeaconBlock{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    f.root[:],
				StateRoot:     f.root[:],
				Body:          bodyPbPhase0(),
			},
			Signature: f.sig[:],
		}
		block := &SignedBeaconBlock{
			version: version.Phase0,
			block: &BeaconBlock{
				version:       version.Phase0,
				slot:          128,
				proposerIndex: 128,
				parentRoot:    f.root,
				stateRoot:     f.root,
				body:          bodyPhase0(),
			},
			signature: f.sig,
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.SignedBeaconBlock)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("Altair", func(t *testing.T) {
		expectedBlock := &zond.SignedBeaconBlockAltair{
			Block: &zond.BeaconBlockAltair{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    f.root[:],
				StateRoot:     f.root[:],
				Body:          bodyPbAltair(),
			},
			Signature: f.sig[:],
		}
		block := &SignedBeaconBlock{
			version: version.Altair,
			block: &BeaconBlock{
				version:       version.Altair,
				slot:          128,
				proposerIndex: 128,
				parentRoot:    f.root,
				stateRoot:     f.root,
				body:          bodyAltair(),
			},
			signature: f.sig,
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.SignedBeaconBlockAltair)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		expectedBlock := &zond.SignedBeaconBlockBellatrix{
			Block: &zond.BeaconBlockBellatrix{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    f.root[:],
				StateRoot:     f.root[:],
				Body:          bodyPbBellatrix(),
			},
			Signature: f.sig[:],
		}
		block := &SignedBeaconBlock{
			version: version.Bellatrix,
			block: &BeaconBlock{
				version:       version.Bellatrix,
				slot:          128,
				proposerIndex: 128,
				parentRoot:    f.root,
				stateRoot:     f.root,
				body:          bodyBellatrix(t),
			},
			signature: f.sig,
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.SignedBeaconBlockBellatrix)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("BellatrixBlind", func(t *testing.T) {
		expectedBlock := &zond.SignedBlindedBeaconBlockBellatrix{
			Block: &zond.BlindedBeaconBlockBellatrix{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    f.root[:],
				StateRoot:     f.root[:],
				Body:          bodyPbBlindedBellatrix(),
			},
			Signature: f.sig[:],
		}
		block := &SignedBeaconBlock{
			version: version.Bellatrix,
			block: &BeaconBlock{
				version:       version.Bellatrix,
				slot:          128,
				proposerIndex: 128,
				parentRoot:    f.root,
				stateRoot:     f.root,
				body:          bodyBlindedBellatrix(t),
			},
			signature: f.sig,
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.SignedBlindedBeaconBlockBellatrix)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
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
	t.Run("Deneb", func(t *testing.T) {
		expectedBlock := &zond.SignedBeaconBlockDeneb{
			Block: &zond.BeaconBlockDeneb{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    f.root[:],
				StateRoot:     f.root[:],
				Body:          bodyPbDeneb(),
			},
			Signature: f.sig[:],
		}
		block := &SignedBeaconBlock{
			version: version.Deneb,
			block: &BeaconBlock{
				version:       version.Deneb,
				slot:          128,
				proposerIndex: 128,
				parentRoot:    f.root,
				stateRoot:     f.root,
				body:          bodyDeneb(t),
			},
			signature: f.sig,
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.SignedBeaconBlockDeneb)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("DenebBlind", func(t *testing.T) {
		expectedBlock := &zond.SignedBlindedBeaconBlockDeneb{
			Message: &zond.BlindedBeaconBlockDeneb{
				Slot:          128,
				ProposerIndex: 128,
				ParentRoot:    f.root[:],
				StateRoot:     f.root[:],
				Body:          bodyPbBlindedDeneb(),
			},
			Signature: f.sig[:],
		}
		block := &SignedBeaconBlock{
			version: version.Deneb,
			block: &BeaconBlock{
				version:       version.Deneb,
				slot:          128,
				proposerIndex: 128,
				parentRoot:    f.root,
				stateRoot:     f.root,
				body:          bodyBlindedDeneb(t),
			},
			signature: f.sig,
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.SignedBlindedBeaconBlockDeneb)
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

	t.Run("Phase0", func(t *testing.T) {
		expectedBlock := &zond.BeaconBlock{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbPhase0(),
		}
		block := &BeaconBlock{
			version:       version.Phase0,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    f.root,
			stateRoot:     f.root,
			body:          bodyPhase0(),
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BeaconBlock)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("Altair", func(t *testing.T) {
		expectedBlock := &zond.BeaconBlockAltair{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbAltair(),
		}
		block := &BeaconBlock{
			version:       version.Altair,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    f.root,
			stateRoot:     f.root,
			body:          bodyAltair(),
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BeaconBlockAltair)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		expectedBlock := &zond.BeaconBlockBellatrix{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbBellatrix(),
		}
		block := &BeaconBlock{
			version:       version.Bellatrix,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    f.root,
			stateRoot:     f.root,
			body:          bodyBellatrix(t),
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BeaconBlockBellatrix)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("BellatrixBlind", func(t *testing.T) {
		expectedBlock := &zond.BlindedBeaconBlockBellatrix{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbBlindedBellatrix(),
		}
		block := &BeaconBlock{
			version:       version.Bellatrix,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    f.root,
			stateRoot:     f.root,
			body:          bodyBlindedBellatrix(t),
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BlindedBeaconBlockBellatrix)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
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
	t.Run("Deneb", func(t *testing.T) {
		expectedBlock := &zond.BeaconBlockDeneb{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbDeneb(),
		}
		block := &BeaconBlock{
			version:       version.Deneb,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    f.root,
			stateRoot:     f.root,
			body:          bodyDeneb(t),
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BeaconBlockDeneb)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("DenebBlind", func(t *testing.T) {
		expectedBlock := &zond.BlindedBeaconBlockDeneb{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbBlindedDeneb(),
		}
		block := &BeaconBlock{
			version:       version.Deneb,
			slot:          128,
			proposerIndex: 128,
			parentRoot:    f.root,
			stateRoot:     f.root,
			body:          bodyBlindedDeneb(t),
		}

		result, err := block.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BlindedBeaconBlockDeneb)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBlock.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
}

func Test_BeaconBlockBody_Proto(t *testing.T) {
	t.Run("Phase0", func(t *testing.T) {
		expectedBody := bodyPbPhase0()
		body := bodyPhase0()

		result, err := body.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BeaconBlockBody)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBody.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("Altair", func(t *testing.T) {
		expectedBody := bodyPbAltair()
		body := bodyAltair()
		result, err := body.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BeaconBlockBodyAltair)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBody.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("Bellatrix", func(t *testing.T) {
		expectedBody := bodyPbBellatrix()
		body := bodyBellatrix(t)
		result, err := body.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BeaconBlockBodyBellatrix)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBody.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("BellatrixBlind", func(t *testing.T) {
		expectedBody := bodyPbBlindedBellatrix()
		body := bodyBlindedBellatrix(t)
		result, err := body.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BlindedBeaconBlockBodyBellatrix)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBody.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
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
	t.Run("Deneb", func(t *testing.T) {
		expectedBody := bodyPbDeneb()
		body := bodyDeneb(t)
		result, err := body.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BeaconBlockBodyDeneb)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBody.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("DenebBlind", func(t *testing.T) {
		expectedBody := bodyPbBlindedDeneb()
		body := bodyBlindedDeneb(t)
		result, err := body.Proto()
		require.NoError(t, err)
		resultBlock, ok := result.(*zond.BlindedBeaconBlockBodyDeneb)
		require.Equal(t, true, ok)
		resultHTR, err := resultBlock.HashTreeRoot()
		require.NoError(t, err)
		expectedHTR, err := expectedBody.HashTreeRoot()
		require.NoError(t, err)
		assert.DeepEqual(t, expectedHTR, resultHTR)
	})
	t.Run("Bellatrix - wrong payload type", func(t *testing.T) {
		body := bodyBellatrix(t)
		body.executionPayload = &executionPayloadHeader{}
		_, err := body.Proto()
		require.ErrorIs(t, err, errPayloadWrongType)
	})
	t.Run("BellatrixBlind - wrong payload type", func(t *testing.T) {
		body := bodyBlindedBellatrix(t)
		body.executionPayloadHeader = &executionPayload{}
		_, err := body.Proto()
		require.ErrorIs(t, err, errPayloadHeaderWrongType)
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
	t.Run("Deneb - wrong payload type", func(t *testing.T) {
		body := bodyDeneb(t)
		body.executionPayload = &executionPayloadHeaderDeneb{}
		_, err := body.Proto()
		require.ErrorIs(t, err, errPayloadWrongType)
	})
	t.Run("DenebBlind - wrong payload type", func(t *testing.T) {
		body := bodyBlindedDeneb(t)
		body.executionPayloadHeader = &executionPayloadDeneb{}
		_, err := body.Proto()
		require.ErrorIs(t, err, errPayloadHeaderWrongType)
	})
}

func Test_initSignedBlockFromProtoPhase0(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.SignedBeaconBlock{
		Block: &zond.BeaconBlock{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbPhase0(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initSignedBlockFromProtoPhase0(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
}

func Test_initSignedBlockFromProtoAltair(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.SignedBeaconBlockAltair{
		Block: &zond.BeaconBlockAltair{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbAltair(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initSignedBlockFromProtoAltair(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
}

func Test_initSignedBlockFromProtoBellatrix(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.SignedBeaconBlockBellatrix{
		Block: &zond.BeaconBlockBellatrix{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbBellatrix(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initSignedBlockFromProtoBellatrix(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
}

func Test_initBlindedSignedBlockFromProtoBellatrix(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.SignedBlindedBeaconBlockBellatrix{
		Block: &zond.BlindedBeaconBlockBellatrix{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbBlindedBellatrix(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initBlindedSignedBlockFromProtoBellatrix(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
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

func Test_initSignedBlockFromProtoDeneb(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.SignedBeaconBlockDeneb{
		Block: &zond.BeaconBlockDeneb{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbDeneb(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initSignedBlockFromProtoDeneb(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
}

func Test_initBlindedSignedBlockFromProtoDeneb(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.SignedBlindedBeaconBlockDeneb{
		Message: &zond.BlindedBeaconBlockDeneb{
			Slot:          128,
			ProposerIndex: 128,
			ParentRoot:    f.root[:],
			StateRoot:     f.root[:],
			Body:          bodyPbBlindedDeneb(),
		},
		Signature: f.sig[:],
	}
	resultBlock, err := initBlindedSignedBlockFromProtoDeneb(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.block.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.Message.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
	assert.DeepEqual(t, expectedBlock.Signature, resultBlock.signature[:])
}

func Test_initBlockFromProtoPhase0(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.BeaconBlock{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbPhase0(),
	}
	resultBlock, err := initBlockFromProtoPhase0(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockFromProtoAltair(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.BeaconBlockAltair{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbAltair(),
	}
	resultBlock, err := initBlockFromProtoAltair(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockFromProtoBellatrix(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.BeaconBlockBellatrix{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbBellatrix(),
	}
	resultBlock, err := initBlockFromProtoBellatrix(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockFromProtoBlindedBellatrix(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.BlindedBeaconBlockBellatrix{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbBlindedBellatrix(),
	}
	resultBlock, err := initBlindedBlockFromProtoBellatrix(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
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

func Test_initBlockFromProtoDeneb(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.BeaconBlockDeneb{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbDeneb(),
	}
	resultBlock, err := initBlockFromProtoDeneb(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockFromProtoBlindedDeneb(t *testing.T) {
	f := getFields()
	expectedBlock := &zond.BlindedBeaconBlockDeneb{
		Slot:          128,
		ProposerIndex: 128,
		ParentRoot:    f.root[:],
		StateRoot:     f.root[:],
		Body:          bodyPbBlindedDeneb(),
	}
	resultBlock, err := initBlindedBlockFromProtoDeneb(expectedBlock)
	require.NoError(t, err)
	resultHTR, err := resultBlock.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBlock.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockBodyFromProtoPhase0(t *testing.T) {
	expectedBody := bodyPbPhase0()
	resultBody, err := initBlockBodyFromProtoPhase0(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockBodyFromProtoAltair(t *testing.T) {
	expectedBody := bodyPbAltair()
	resultBody, err := initBlockBodyFromProtoAltair(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockBodyFromProtoBellatrix(t *testing.T) {
	expectedBody := bodyPbBellatrix()
	resultBody, err := initBlockBodyFromProtoBellatrix(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockBodyFromProtoBlindedBellatrix(t *testing.T) {
	expectedBody := bodyPbBlindedBellatrix()
	resultBody, err := initBlindedBlockBodyFromProtoBellatrix(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
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

func Test_initBlockBodyFromProtoDeneb(t *testing.T) {
	expectedBody := bodyPbDeneb()
	resultBody, err := initBlockBodyFromProtoDeneb(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func Test_initBlockBodyFromProtoBlindedDeneb(t *testing.T) {
	expectedBody := bodyPbBlindedDeneb()
	resultBody, err := initBlindedBlockBodyFromProtoDeneb(expectedBody)
	require.NoError(t, err)
	resultHTR, err := resultBody.HashTreeRoot()
	require.NoError(t, err)
	expectedHTR, err := expectedBody.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, expectedHTR, resultHTR)
}

func bodyPbPhase0() *zond.BeaconBlockBody {
	f := getFields()
	return &zond.BeaconBlockBody{
		RandaoReveal: f.sig[:],
		Eth1Data: &zond.Eth1Data{
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
	}
}

func bodyPbAltair() *zond.BeaconBlockBodyAltair {
	f := getFields()
	return &zond.BeaconBlockBodyAltair{
		RandaoReveal: f.sig[:],
		Eth1Data: &zond.Eth1Data{
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
	}
}

func bodyPbBellatrix() *zond.BeaconBlockBodyBellatrix {
	f := getFields()
	return &zond.BeaconBlockBodyBellatrix{
		RandaoReveal: f.sig[:],
		Eth1Data: &zond.Eth1Data{
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
		ExecutionPayload:  f.execPayload,
	}
}

func bodyPbBlindedBellatrix() *zond.BlindedBeaconBlockBodyBellatrix {
	f := getFields()
	return &zond.BlindedBeaconBlockBodyBellatrix{
		RandaoReveal: f.sig[:],
		Eth1Data: &zond.Eth1Data{
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
		ExecutionPayloadHeader: f.execPayloadHeader,
	}
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

func bodyPbDeneb() *zond.BeaconBlockBodyDeneb {
	f := getFields()
	return &zond.BeaconBlockBodyDeneb{
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
		ExecutionPayload:            f.execPayloadDeneb,
		DilithiumToExecutionChanges: f.dilithiumToExecutionChanges,
		BlobKzgCommitments:          f.kzgCommitments,
	}
}

func bodyPbBlindedDeneb() *zond.BlindedBeaconBlockBodyDeneb {
	f := getFields()
	return &zond.BlindedBeaconBlockBodyDeneb{
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
		ExecutionPayloadHeader:      f.execPayloadHeaderDeneb,
		DilithiumToExecutionChanges: f.dilithiumToExecutionChanges,
		BlobKzgCommitments:          f.kzgCommitments,
	}
}

func bodyPhase0() *BeaconBlockBody {
	f := getFields()
	return &BeaconBlockBody{
		version:      version.Phase0,
		randaoReveal: f.sig,
		eth1Data: &zond.Eth1Data{
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
	}
}

func bodyAltair() *BeaconBlockBody {
	f := getFields()
	return &BeaconBlockBody{
		version:      version.Altair,
		randaoReveal: f.sig,
		eth1Data: &zond.Eth1Data{
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
	}
}

func bodyBellatrix(t *testing.T) *BeaconBlockBody {
	f := getFields()
	p, err := WrappedExecutionPayload(f.execPayload)
	require.NoError(t, err)
	return &BeaconBlockBody{
		version:      version.Bellatrix,
		randaoReveal: f.sig,
		eth1Data: &zond.Eth1Data{
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

func bodyBlindedBellatrix(t *testing.T) *BeaconBlockBody {
	f := getFields()
	ph, err := WrappedExecutionPayloadHeader(f.execPayloadHeader)
	require.NoError(t, err)
	return &BeaconBlockBody{
		version:      version.Bellatrix,
		isBlinded:    true,
		randaoReveal: f.sig,
		eth1Data: &zond.Eth1Data{
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

func bodyDeneb(t *testing.T) *BeaconBlockBody {
	f := getFields()
	p, err := WrappedExecutionPayloadDeneb(f.execPayloadDeneb, 0)
	require.NoError(t, err)
	return &BeaconBlockBody{
		version:      version.Deneb,
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
		blobKzgCommitments:          f.kzgCommitments,
	}
}

func bodyBlindedDeneb(t *testing.T) *BeaconBlockBody {
	f := getFields()
	ph, err := WrappedExecutionPayloadHeaderDeneb(f.execPayloadHeaderDeneb, 0)
	require.NoError(t, err)
	return &BeaconBlockBody{
		version:      version.Deneb,
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
		blobKzgCommitments:          f.kzgCommitments,
	}
}

func getFields() fields {
	b20 := make([]byte, 20)
	b48 := make([]byte, 48)
	b256 := make([]byte, 256)
	var root [32]byte
	var sig [dilithium2.CryptoBytes]byte
	b20[0], b20[5], b20[10] = 'q', 'u', 'x'
	b48[0], b48[5], b48[10] = 'b', 'a', 'r'
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
			PublicKey:             b48,
			WithdrawalCredentials: root[:],
			Amount:                128,
			Signature:             sig[:],
		}
	}
	atts := make([]*zond.Attestation, 128)
	for i := range atts {
		atts[i] = &zond.Attestation{}
		atts[i].Signature = sig[:]
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
			Signature: sig[:],
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
			Signature: sig[:],
		},
	}
	voluntaryExit := &zond.SignedVoluntaryExit{
		Exit: &zond.VoluntaryExit{
			Epoch:          128,
			ValidatorIndex: 128,
		},
		Signature: sig[:],
	}
	syncCommitteeBits := bitfield.NewBitvector512()
	syncCommitteeBits.SetBitAt(1, true)
	syncCommitteeBits.SetBitAt(2, true)
	syncCommitteeBits.SetBitAt(8, true)
	syncAggregate := &zond.SyncAggregate{
		SyncCommitteeBits:      syncCommitteeBits,
		SyncCommitteeSignature: sig[:],
	}
	execPayload := &enginev1.ExecutionPayload{
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
	}
	execPayloadHeader := &enginev1.ExecutionPayloadHeader{
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
			FromDilithiumPubkey: b48,
			ToExecutionAddress:  b20,
		},
		Signature: sig[:],
	}}

	execPayloadDeneb := &enginev1.ExecutionPayloadDeneb{
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
		BlobGasUsed:   128,
		ExcessBlobGas: 128,
	}
	execPayloadHeaderDeneb := &enginev1.ExecutionPayloadHeaderDeneb{
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
		BlobGasUsed:      128,
		ExcessBlobGas:    128,
	}

	kzgCommitments := [][]byte{
		bytesutil.PadTo([]byte{123}, 48),
		bytesutil.PadTo([]byte{223}, 48),
		bytesutil.PadTo([]byte{183}, 48),
		bytesutil.PadTo([]byte{143}, 48),
	}

	return fields{
		root:                        root,
		sig:                         sig,
		deposits:                    deposits,
		atts:                        atts,
		proposerSlashings:           []*zond.ProposerSlashing{proposerSlashing},
		attesterSlashings:           []*zond.AttesterSlashing{attesterSlashing},
		voluntaryExits:              []*zond.SignedVoluntaryExit{voluntaryExit},
		syncAggregate:               syncAggregate,
		execPayload:                 execPayload,
		execPayloadHeader:           execPayloadHeader,
		execPayloadCapella:          execPayloadCapella,
		execPayloadHeaderCapella:    execPayloadHeaderCapella,
		execPayloadDeneb:            execPayloadDeneb,
		execPayloadHeaderDeneb:      execPayloadHeaderDeneb,
		dilithiumToExecutionChanges: dilithiumToExecutionChanges,
		kzgCommitments:              kzgCommitments,
	}
}
