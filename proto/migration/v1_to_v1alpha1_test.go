package migration

import (
	"testing"

	"github.com/theQRL/go-bitfield"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestBlockMigrationProposerSlashings(t *testing.T) {
	newSlashing := func() *qrysmpb.ProposerSlashing {
		header1 := util.HydrateSignedBeaconHeader(&qrysmpb.SignedBeaconBlockHeader{})
		header2 := util.HydrateSignedBeaconHeader(&qrysmpb.SignedBeaconBlockHeader{})
		header1.Header.ParentRoot[0], header1.Signature[0] = 1, 3
		header2.Header.ParentRoot[0], header2.Signature[0] = 2, 4
		return &qrysmpb.ProposerSlashing{Header_1: header1, Header_2: header2}
	}
	alphaFull := util.HydrateBeaconBlockZond(&qrysmpb.BeaconBlockZond{})
	alphaFull.Body.ProposerSlashings = []*qrysmpb.ProposerSlashing{newSlashing()}
	alphaBlinded := util.HydrateBlindedBeaconBlockZond(&qrysmpb.BlindedBeaconBlockZond{})
	alphaBlinded.Body.ProposerSlashings = []*qrysmpb.ProposerSlashing{newSlashing()}
	v1Full := util.HydrateV1ZondSignedBeaconBlock(&qrlpb.SignedBeaconBlockZond{})
	v1Full.Message.Body.SyncAggregate.SyncCommitteeBits = bitfield.NewBitvector128()
	v1Full.Message.Body.ProposerSlashings = []*qrlpb.ProposerSlashing{V1Alpha1ProposerSlashingToV1(newSlashing())}
	v1Blinded := util.HydrateV1SignedBlindedBeaconBlockZond(&qrlpb.SignedBlindedBeaconBlockZond{})
	v1Blinded.Message.Body.SyncAggregate.SyncCommitteeBits = bitfield.NewBitvector128()
	v1Blinded.Message.Body.ProposerSlashings = []*qrlpb.ProposerSlashing{V1Alpha1ProposerSlashingToV1(newSlashing())}

	type sszBlock interface {
		MarshalSSZ() ([]byte, error)
		HashTreeRoot() ([32]byte, error)
	}
	tests := []struct {
		name    string
		source  sszBlock
		convert func() (sszBlock, error)
		mutate  func()
	}{
		{
			name: "full alpha to v1", source: alphaFull,
			convert: func() (sszBlock, error) { return V1Alpha1BeaconBlockZondToV1(alphaFull) },
			mutate: func() {
				alphaFull.Body.ProposerSlashings[0].Header_1.Header.ParentRoot[0]++
				alphaFull.Body.ProposerSlashings[0].Header_2.Signature[0]++
			},
		},
		{
			name: "blinded alpha to v1", source: alphaBlinded,
			convert: func() (sszBlock, error) { return V1Alpha1BeaconBlockBlindedZondToV1Blinded(alphaBlinded) },
			mutate: func() {
				alphaBlinded.Body.ProposerSlashings[0].Header_1.Header.ParentRoot[0]++
				alphaBlinded.Body.ProposerSlashings[0].Header_2.Signature[0]++
			},
		},
		{
			name: "full v1 to alpha", source: v1Full,
			convert: func() (sszBlock, error) { return ZondToV1Alpha1SignedBlock(v1Full) },
			mutate: func() {
				v1Full.Message.Body.ProposerSlashings[0].SignedHeader_1.Message.ParentRoot[0]++
				v1Full.Message.Body.ProposerSlashings[0].SignedHeader_2.Signature[0]++
			},
		},
		{
			name: "blinded v1 to alpha", source: v1Blinded,
			convert: func() (sszBlock, error) { return BlindedZondToV1Alpha1SignedBlock(v1Blinded) },
			mutate: func() {
				v1Blinded.Message.Body.ProposerSlashings[0].SignedHeader_1.Message.ParentRoot[0]++
				v1Blinded.Message.Body.ProposerSlashings[0].SignedHeader_2.Signature[0]++
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantBytes, err := tt.source.MarshalSSZ()
			require.NoError(t, err)
			wantRoot, err := tt.source.HashTreeRoot()
			require.NoError(t, err)
			converted, err := tt.convert()
			require.NoError(t, err)
			gotBytes, err := converted.MarshalSSZ()
			require.NoError(t, err)
			assert.DeepEqual(t, wantBytes, gotBytes)
			gotRoot, err := converted.HashTreeRoot()
			require.NoError(t, err)
			assert.Equal(t, wantRoot, gotRoot)

			tt.mutate()
			gotRoot, err = converted.HashTreeRoot()
			require.NoError(t, err)
			assert.Equal(t, wantRoot, gotRoot, "converted slashing must not alias its source")
		})
	}
}

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
