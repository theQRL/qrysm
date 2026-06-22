package migration

import (
	"testing"

	"github.com/theQRL/go-bitfield"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func Test_ZondToV1Alpha1SignedBlock(t *testing.T) {
	v1Block := util.HydrateV1ZondSignedBeaconBlock(&qrlpb.SignedBeaconBlockZond{})
	v1Block.Message.Slot = slot
	v1Block.Message.ProposerIndex = validatorIndex
	v1Block.Message.ParentRoot = parentRoot
	v1Block.Message.StateRoot = stateRoot
	v1Block.Message.Body.RandaoReveal = randaoReveal
	v1Block.Message.Body.ExecutionData = &qrlpb.ExecutionData{
		DepositRoot:  depositRoot,
		DepositCount: depositCount,
		BlockHash:    blockHash,
	}
	syncCommitteeBits := bitfield.NewBitvector128()
	syncCommitteeBits.SetBitAt(100, true)
	v1Block.Message.Body.SyncAggregate = &qrlpb.SyncAggregate{
		SyncCommitteeBits:       syncCommitteeBits,
		SyncCommitteeSignatures: [][]byte{signature},
	}
	v1Block.Message.Body.ExecutionPayload = &enginev1.ExecutionPayloadZond{
		ParentHash:    parentHash,
		FeeRecipient:  feeRecipient,
		StateRoot:     stateRoot,
		ReceiptsRoot:  receiptsRoot,
		LogsBloom:     logsBloom,
		PrevRandao:    prevRandao,
		BlockNumber:   blockNumber,
		GasLimit:      gasLimit,
		GasUsed:       gasUsed,
		Timestamp:     timestamp,
		ExtraData:     extraData,
		BaseFeePerGas: baseFeePerGas,
		BlockHash:     blockHash,
		Transactions:  [][]byte{[]byte("transaction1"), []byte("transaction2")},
		Withdrawals: []*enginev1.Withdrawal{{
			Index:          uint64(validatorIndex),
			ValidatorIndex: validatorIndex,
			Address:        feeRecipient,
			Amount:         10,
		}},
	}
	v1Block.Signature = signature

	alphaBlock, err := ZondToV1Alpha1SignedBlock(v1Block)
	require.NoError(t, err)
	alphaRoot, err := alphaBlock.HashTreeRoot()
	require.NoError(t, err)
	v1Root, err := v1Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, v1Root, alphaRoot)
}

func Test_BlindedZondToV1Alpha1SignedBlock(t *testing.T) {
	v1Block := util.HydrateV1SignedBlindedBeaconBlockZond(&qrlpb.SignedBlindedBeaconBlockZond{})
	v1Block.Message.Slot = slot
	v1Block.Message.ProposerIndex = validatorIndex
	v1Block.Message.ParentRoot = parentRoot
	v1Block.Message.StateRoot = stateRoot
	v1Block.Message.Body.RandaoReveal = randaoReveal
	v1Block.Message.Body.ExecutionData = &qrlpb.ExecutionData{
		DepositRoot:  depositRoot,
		DepositCount: depositCount,
		BlockHash:    blockHash,
	}
	syncCommitteeBits := bitfield.NewBitvector128()
	syncCommitteeBits.SetBitAt(100, true)
	v1Block.Message.Body.SyncAggregate = &qrlpb.SyncAggregate{
		SyncCommitteeBits:       syncCommitteeBits,
		SyncCommitteeSignatures: [][]byte{signature},
	}
	v1Block.Message.Body.ExecutionPayloadHeader = &enginev1.ExecutionPayloadHeaderZond{
		ParentHash:       parentHash,
		FeeRecipient:     feeRecipient,
		StateRoot:        stateRoot,
		ReceiptsRoot:     receiptsRoot,
		LogsBloom:        logsBloom,
		PrevRandao:       prevRandao,
		BlockNumber:      blockNumber,
		GasLimit:         gasLimit,
		GasUsed:          gasUsed,
		Timestamp:        timestamp,
		ExtraData:        extraData,
		BaseFeePerGas:    baseFeePerGas,
		BlockHash:        blockHash,
		TransactionsRoot: transactionsRoot,
		WithdrawalsRoot:  withdrawalsRoot,
	}
	v1Block.Signature = signature

	alphaBlock, err := BlindedZondToV1Alpha1SignedBlock(v1Block)
	require.NoError(t, err)
	alphaRoot, err := alphaBlock.HashTreeRoot()
	require.NoError(t, err)
	v1Root, err := v1Block.HashTreeRoot()
	require.NoError(t, err)
	assert.DeepEqual(t, v1Root, alphaRoot)
}
