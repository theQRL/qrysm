package shared

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/common/hexutil"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	bytesutil2 "github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/math"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func (b *SignedBeaconBlockZond) ToGeneric() (*qrysmpb.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := DecodeHexWithLength(b.Signature, fieldparams.MLDSA87SignatureLength)
	if err != nil {
		return nil, NewDecodeError(err, "Signature")
	}
	bl, err := b.Message.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Message")
	}
	block := &qrysmpb.SignedBeaconBlockZond{
		Block:     bl,
		Signature: sig,
	}
	return &qrysmpb.GenericSignedBeaconBlock{Block: &qrysmpb.GenericSignedBeaconBlock_Zond{Zond: block}}, nil
}

func (b *BeaconBlockZond) ToGeneric() (*qrysmpb.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_Zond{Zond: block}}, nil
}

func (b *BeaconBlockZond) ToConsensus() (*qrysmpb.BeaconBlockZond, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, NewDecodeError(errNilValue, "Body")
	}
	if b.Body.ExecutionData == nil {
		return nil, NewDecodeError(errNilValue, "Body.ExecutionData")
	}
	if b.Body.SyncAggregate == nil {
		return nil, NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayload == nil {
		return nil, NewDecodeError(errNilValue, "Body.ExecutionPayload")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.MLDSA87SignatureLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := DecodeHexWithLength(b.Body.ExecutionData.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionData.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.ExecutionData.DepositCount, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionData.DepositCount")
	}
	blockHash, err := DecodeHexWithLength(b.Body.ExecutionData.BlockHash, common.HashLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionData.BlockHash")
	}
	graffiti, err := DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, NewDecodeError(err, "Body.Deposits")
	}
	exits, err := ExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}

	syncCommitteeSigs := make([][]byte, len(b.Body.SyncAggregate.SyncCommitteeSignatures))
	for i, sig := range b.Body.SyncAggregate.SyncCommitteeSignatures {
		syncCommitteeSig, err := DecodeHexWithLength(sig, fieldparams.MLDSA87SignatureLength)
		if err != nil {
			return nil, NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignatures")
		}
		syncCommitteeSigs[i] = syncCommitteeSig
	}

	payloadParentHash, err := DecodeHexWithLength(b.Body.ExecutionPayload.ParentHash, common.HashLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.ParentHash")
	}
	payloadFeeRecipient, err := DecodeAddressWithLength(b.Body.ExecutionPayload.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.FeeRecipient")
	}
	payloadStateRoot, err := DecodeHexWithLength(b.Body.ExecutionPayload.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.StateRoot")
	}
	payloadReceiptsRoot, err := DecodeHexWithLength(b.Body.ExecutionPayload.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.ReceiptsRoot")
	}
	payloadLogsBloom, err := DecodeHexWithLength(b.Body.ExecutionPayload.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.LogsBloom")
	}
	payloadPrevRandao, err := DecodeHexWithLength(b.Body.ExecutionPayload.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayload.BlockNumber, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayload.GasLimit, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayload.GasUsed, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayload.Timestamp, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.Timestamp")
	}
	payloadExtraData, err := DecodeHexWithMaxLength(b.Body.ExecutionPayload.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.ExtraData")
	}
	payloadBaseFeePerGas, err := Uint256ToSSZBytes(b.Body.ExecutionPayload.BaseFeePerGas)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.BaseFeePerGas")
	}
	payloadBlockHash, err := DecodeHexWithLength(b.Body.ExecutionPayload.BlockHash, common.HashLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.BlockHash")
	}
	err = VerifyMaxLength(b.Body.ExecutionPayload.Transactions, fieldparams.MaxTxsPerPayloadLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.Transactions")
	}
	payloadTxs := make([][]byte, len(b.Body.ExecutionPayload.Transactions))
	for i, tx := range b.Body.ExecutionPayload.Transactions {
		payloadTxs[i], err = DecodeHexWithMaxLength(tx, fieldparams.MaxBytesPerTxLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Transactions[%d]", i))
		}
	}
	err = VerifyMaxLength(b.Body.ExecutionPayload.Withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayload.Withdrawals")
	}
	withdrawals := make([]*enginev1.Withdrawal, len(b.Body.ExecutionPayload.Withdrawals))
	for i, w := range b.Body.ExecutionPayload.Withdrawals {
		withdrawalIndex, err := strconv.ParseUint(w.WithdrawalIndex, 10, 64)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].WithdrawalIndex", i))
		}
		validatorIndex, err := strconv.ParseUint(w.ValidatorIndex, 10, 64)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].ValidatorIndex", i))
		}
		address, err := DecodeAddressWithLength(w.ExecutionAddress, common.AddressLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].ExecutionAddress", i))
		}
		amount, err := strconv.ParseUint(w.Amount, 10, 64)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("Body.ExecutionPayload.Withdrawals[%d].Amount", i))
		}
		withdrawals[i] = &enginev1.Withdrawal{
			Index:          withdrawalIndex,
			ValidatorIndex: primitives.ValidatorIndex(validatorIndex),
			Address:        address,
			Amount:         amount,
		}
	}

	return &qrysmpb.BeaconBlockZond{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &qrysmpb.BeaconBlockBodyZond{
			RandaoReveal: randaoReveal,
			ExecutionData: &qrysmpb.ExecutionData{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &qrysmpb.SyncAggregate{
				SyncCommitteeBits:       syncCommitteeBits,
				SyncCommitteeSignatures: syncCommitteeSigs,
			},
			ExecutionPayload: &enginev1.ExecutionPayloadZond{
				ParentHash:    payloadParentHash,
				FeeRecipient:  payloadFeeRecipient,
				StateRoot:     payloadStateRoot,
				ReceiptsRoot:  payloadReceiptsRoot,
				LogsBloom:     payloadLogsBloom,
				PrevRandao:    payloadPrevRandao,
				BlockNumber:   payloadBlockNumber,
				GasLimit:      payloadGasLimit,
				GasUsed:       payloadGasUsed,
				Timestamp:     payloadTimestamp,
				ExtraData:     payloadExtraData,
				BaseFeePerGas: payloadBaseFeePerGas,
				BlockHash:     payloadBlockHash,
				Transactions:  payloadTxs,
				Withdrawals:   withdrawals,
			},
		},
	}, nil
}

func (b *SignedBlindedBeaconBlockZond) ToGeneric() (*qrysmpb.GenericSignedBeaconBlock, error) {
	if b == nil {
		return nil, errNilValue
	}

	sig, err := DecodeHexWithLength(b.Signature, fieldparams.MLDSA87SignatureLength)
	if err != nil {
		return nil, NewDecodeError(err, "Signature")
	}
	bl, err := b.Message.ToConsensus()
	if err != nil {
		return nil, NewDecodeError(err, "Message")
	}
	block := &qrysmpb.SignedBlindedBeaconBlockZond{
		Block:     bl,
		Signature: sig,
	}
	return &qrysmpb.GenericSignedBeaconBlock{Block: &qrysmpb.GenericSignedBeaconBlock_BlindedZond{BlindedZond: block}, IsBlinded: true, PayloadValue: 0 /* can't get payload value from blinded block */}, nil
}

func (b *BlindedBeaconBlockZond) ToGeneric() (*qrysmpb.GenericBeaconBlock, error) {
	block, err := b.ToConsensus()
	if err != nil {
		return nil, err
	}
	return &qrysmpb.GenericBeaconBlock{Block: &qrysmpb.GenericBeaconBlock_BlindedZond{BlindedZond: block}, IsBlinded: true, PayloadValue: 0 /* can't get payload value from blinded block */}, nil
}

func (b *BlindedBeaconBlockZond) ToConsensus() (*qrysmpb.BlindedBeaconBlockZond, error) {
	if b == nil {
		return nil, errNilValue
	}
	if b.Body == nil {
		return nil, NewDecodeError(errNilValue, "Body")
	}
	if b.Body.ExecutionData == nil {
		return nil, NewDecodeError(errNilValue, "Body.ExecutionData")
	}
	if b.Body.SyncAggregate == nil {
		return nil, NewDecodeError(errNilValue, "Body.SyncAggregate")
	}
	if b.Body.ExecutionPayloadHeader == nil {
		return nil, NewDecodeError(errNilValue, "Body.ExecutionPayloadHeader")
	}

	slot, err := strconv.ParseUint(b.Slot, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Slot")
	}
	proposerIndex, err := strconv.ParseUint(b.ProposerIndex, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "ProposerIndex")
	}
	parentRoot, err := DecodeHexWithLength(b.ParentRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "ParentRoot")
	}
	stateRoot, err := DecodeHexWithLength(b.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "StateRoot")
	}
	randaoReveal, err := DecodeHexWithLength(b.Body.RandaoReveal, fieldparams.MLDSA87SignatureLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.RandaoReveal")
	}
	depositRoot, err := DecodeHexWithLength(b.Body.ExecutionData.DepositRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionData.DepositRoot")
	}
	depositCount, err := strconv.ParseUint(b.Body.ExecutionData.DepositCount, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionData.DepositCount")
	}
	blockHash, err := DecodeHexWithLength(b.Body.ExecutionData.BlockHash, common.HashLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionData.BlockHash")
	}
	graffiti, err := DecodeHexWithLength(b.Body.Graffiti, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.Graffiti")
	}
	proposerSlashings, err := ProposerSlashingsToConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ProposerSlashings")
	}
	attesterSlashings, err := AttesterSlashingsToConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, NewDecodeError(err, "Body.AttesterSlashings")
	}
	atts, err := AttsToConsensus(b.Body.Attestations)
	if err != nil {
		return nil, NewDecodeError(err, "Body.Attestations")
	}
	deposits, err := DepositsToConsensus(b.Body.Deposits)
	if err != nil {
		return nil, NewDecodeError(err, "Body.Deposits")
	}
	exits, err := ExitsToConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, NewDecodeError(err, "Body.VoluntaryExits")
	}
	syncCommitteeBits, err := DecodeHexWithLength(b.Body.SyncAggregate.SyncCommitteeBits, fieldparams.SyncAggregateSyncCommitteeBytesLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeBits")
	}

	syncCommitteeSigs := make([][]byte, len(b.Body.SyncAggregate.SyncCommitteeSignatures))
	for i, sig := range b.Body.SyncAggregate.SyncCommitteeSignatures {
		syncCommitteeSig, err := DecodeHexWithLength(sig, fieldparams.MLDSA87SignatureLength)
		if err != nil {
			return nil, NewDecodeError(err, "Body.SyncAggregate.SyncCommitteeSignature")
		}
		syncCommitteeSigs[i] = syncCommitteeSig
	}

	payloadParentHash, err := DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ParentHash, common.HashLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.ParentHash")
	}
	payloadFeeRecipient, err := DecodeAddressWithLength(b.Body.ExecutionPayloadHeader.FeeRecipient, fieldparams.FeeRecipientLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.FeeRecipient")
	}
	payloadStateRoot, err := DecodeHexWithLength(b.Body.ExecutionPayloadHeader.StateRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.StateRoot")
	}
	payloadReceiptsRoot, err := DecodeHexWithLength(b.Body.ExecutionPayloadHeader.ReceiptsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.ReceiptsRoot")
	}
	payloadLogsBloom, err := DecodeHexWithLength(b.Body.ExecutionPayloadHeader.LogsBloom, fieldparams.LogsBloomLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.LogsBloom")
	}
	payloadPrevRandao, err := DecodeHexWithLength(b.Body.ExecutionPayloadHeader.PrevRandao, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.PrevRandao")
	}
	payloadBlockNumber, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.BlockNumber, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockNumber")
	}
	payloadGasLimit, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasLimit, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.GasLimit")
	}
	payloadGasUsed, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.GasUsed, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.GasUsed")
	}
	payloadTimestamp, err := strconv.ParseUint(b.Body.ExecutionPayloadHeader.Timestamp, 10, 64)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.Timestamp")
	}
	payloadExtraData, err := DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.ExtraData, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.ExtraData")
	}
	payloadBaseFeePerGas, err := Uint256ToSSZBytes(b.Body.ExecutionPayloadHeader.BaseFeePerGas)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.BaseFeePerGas")
	}
	payloadBlockHash, err := DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.BlockHash, common.HashLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.BlockHash")
	}
	payloadTxsRoot, err := DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.TransactionsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.TransactionsRoot")
	}
	payloadWithdrawalsRoot, err := DecodeHexWithMaxLength(b.Body.ExecutionPayloadHeader.WithdrawalsRoot, fieldparams.RootLength)
	if err != nil {
		return nil, NewDecodeError(err, "Body.ExecutionPayloadHeader.WithdrawalsRoot")
	}

	return &qrysmpb.BlindedBeaconBlockZond{
		Slot:          primitives.Slot(slot),
		ProposerIndex: primitives.ValidatorIndex(proposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &qrysmpb.BlindedBeaconBlockBodyZond{
			RandaoReveal: randaoReveal,
			ExecutionData: &qrysmpb.ExecutionData{
				DepositRoot:  depositRoot,
				DepositCount: depositCount,
				BlockHash:    blockHash,
			},
			Graffiti:          graffiti,
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &qrysmpb.SyncAggregate{
				SyncCommitteeBits:       syncCommitteeBits,
				SyncCommitteeSignatures: syncCommitteeSigs,
			},
			ExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderZond{
				ParentHash:       payloadParentHash,
				FeeRecipient:     payloadFeeRecipient,
				StateRoot:        payloadStateRoot,
				ReceiptsRoot:     payloadReceiptsRoot,
				LogsBloom:        payloadLogsBloom,
				PrevRandao:       payloadPrevRandao,
				BlockNumber:      payloadBlockNumber,
				GasLimit:         payloadGasLimit,
				GasUsed:          payloadGasUsed,
				Timestamp:        payloadTimestamp,
				ExtraData:        payloadExtraData,
				BaseFeePerGas:    payloadBaseFeePerGas,
				BlockHash:        payloadBlockHash,
				TransactionsRoot: payloadTxsRoot,
				WithdrawalsRoot:  payloadWithdrawalsRoot,
			},
		},
	}, nil
}

func BeaconBlockHeaderFromConsensus(h *qrysmpb.BeaconBlockHeader) *BeaconBlockHeader {
	return &BeaconBlockHeader{
		Slot:          strconv.FormatUint(uint64(h.Slot), 10),
		ProposerIndex: strconv.FormatUint(uint64(h.ProposerIndex), 10),
		ParentRoot:    hexutil.Encode(h.ParentRoot),
		StateRoot:     hexutil.Encode(h.StateRoot),
		BodyRoot:      hexutil.Encode(h.BodyRoot),
	}
}

func BlindedBeaconBlockZondFromConsensus(b *qrysmpb.BlindedBeaconBlockZond) (*BlindedBeaconBlockZond, error) {
	proposerSlashings, err := ProposerSlashingsFromConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, err
	}
	attesterSlashings, err := AttesterSlashingsFromConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, err
	}
	atts, err := AttsFromConsensus(b.Body.Attestations)
	if err != nil {
		return nil, err
	}
	deposits, err := DepositsFromConsensus(b.Body.Deposits)
	if err != nil {
		return nil, err
	}
	exits, err := ExitsFromConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, err
	}
	baseFeePerGas, err := sszBytesToUint256String(b.Body.ExecutionPayloadHeader.BaseFeePerGas)
	if err != nil {
		return nil, err
	}
	syncCommitteeSignatures := make([]string, len(b.Body.SyncAggregate.SyncCommitteeSignatures))
	for i, sig := range b.Body.SyncAggregate.SyncCommitteeSignatures {
		syncCommitteeSignatures[i] = hexutil.Encode(sig)
	}

	return &BlindedBeaconBlockZond{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BlindedBeaconBlockBodyZond{
			RandaoReveal: hexutil.Encode(b.Body.RandaoReveal),
			ExecutionData: &ExecutionData{
				DepositRoot:  hexutil.Encode(b.Body.ExecutionData.DepositRoot),
				DepositCount: fmt.Sprintf("%d", b.Body.ExecutionData.DepositCount),
				BlockHash:    hexutil.Encode(b.Body.ExecutionData.BlockHash),
			},
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:       hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignatures: syncCommitteeSignatures,
			},
			ExecutionPayloadHeader: &ExecutionPayloadHeaderZond{
				ParentHash:       hexutil.Encode(b.Body.ExecutionPayloadHeader.ParentHash),
				FeeRecipient:     hexutil.EncodeQ(b.Body.ExecutionPayloadHeader.FeeRecipient),
				StateRoot:        hexutil.Encode(b.Body.ExecutionPayloadHeader.StateRoot),
				ReceiptsRoot:     hexutil.Encode(b.Body.ExecutionPayloadHeader.ReceiptsRoot),
				LogsBloom:        hexutil.Encode(b.Body.ExecutionPayloadHeader.LogsBloom),
				PrevRandao:       hexutil.Encode(b.Body.ExecutionPayloadHeader.PrevRandao),
				BlockNumber:      fmt.Sprintf("%d", b.Body.ExecutionPayloadHeader.BlockNumber),
				GasLimit:         fmt.Sprintf("%d", b.Body.ExecutionPayloadHeader.GasLimit),
				GasUsed:          fmt.Sprintf("%d", b.Body.ExecutionPayloadHeader.GasUsed),
				Timestamp:        fmt.Sprintf("%d", b.Body.ExecutionPayloadHeader.Timestamp),
				ExtraData:        hexutil.Encode(b.Body.ExecutionPayloadHeader.ExtraData),
				BaseFeePerGas:    baseFeePerGas,
				BlockHash:        hexutil.Encode(b.Body.ExecutionPayloadHeader.BlockHash),
				TransactionsRoot: hexutil.Encode(b.Body.ExecutionPayloadHeader.TransactionsRoot),
				WithdrawalsRoot:  hexutil.Encode(b.Body.ExecutionPayloadHeader.WithdrawalsRoot), // new in zond
			},
		},
	}, nil
}

func SignedBlindedBeaconBlockZondFromConsensus(b *qrysmpb.SignedBlindedBeaconBlockZond) (*SignedBlindedBeaconBlockZond, error) {
	blindedBlock, err := BlindedBeaconBlockZondFromConsensus(b.Block)
	if err != nil {
		return nil, err
	}
	return &SignedBlindedBeaconBlockZond{
		Message:   blindedBlock,
		Signature: hexutil.Encode(b.Signature),
	}, nil
}

func BeaconBlockZondFromConsensus(b *qrysmpb.BeaconBlockZond) (*BeaconBlockZond, error) {
	proposerSlashings, err := ProposerSlashingsFromConsensus(b.Body.ProposerSlashings)
	if err != nil {
		return nil, err
	}
	attesterSlashings, err := AttesterSlashingsFromConsensus(b.Body.AttesterSlashings)
	if err != nil {
		return nil, err
	}
	atts, err := AttsFromConsensus(b.Body.Attestations)
	if err != nil {
		return nil, err
	}
	deposits, err := DepositsFromConsensus(b.Body.Deposits)
	if err != nil {
		return nil, err
	}
	exits, err := ExitsFromConsensus(b.Body.VoluntaryExits)
	if err != nil {
		return nil, err
	}
	baseFeePerGas, err := sszBytesToUint256String(b.Body.ExecutionPayload.BaseFeePerGas)
	if err != nil {
		return nil, err
	}
	transactions := make([]string, len(b.Body.ExecutionPayload.Transactions))
	for i, tx := range b.Body.ExecutionPayload.Transactions {
		transactions[i] = hexutil.Encode(tx)
	}
	withdrawals := make([]*Withdrawal, len(b.Body.ExecutionPayload.Withdrawals))
	for i, w := range b.Body.ExecutionPayload.Withdrawals {
		withdrawals[i] = &Withdrawal{
			WithdrawalIndex:  fmt.Sprintf("%d", w.Index),
			ValidatorIndex:   fmt.Sprintf("%d", w.ValidatorIndex),
			ExecutionAddress: hexutil.EncodeQ(w.Address),
			Amount:           fmt.Sprintf("%d", w.Amount),
		}
	}
	syncCommitteeSignatures := make([]string, len(b.Body.SyncAggregate.SyncCommitteeSignatures))
	for i, sig := range b.Body.SyncAggregate.SyncCommitteeSignatures {
		syncCommitteeSignatures[i] = hexutil.Encode(sig)
	}

	return &BeaconBlockZond{
		Slot:          fmt.Sprintf("%d", b.Slot),
		ProposerIndex: fmt.Sprintf("%d", b.ProposerIndex),
		ParentRoot:    hexutil.Encode(b.ParentRoot),
		StateRoot:     hexutil.Encode(b.StateRoot),
		Body: &BeaconBlockBodyZond{
			RandaoReveal: hexutil.Encode(b.Body.RandaoReveal),
			ExecutionData: &ExecutionData{
				DepositRoot:  hexutil.Encode(b.Body.ExecutionData.DepositRoot),
				DepositCount: fmt.Sprintf("%d", b.Body.ExecutionData.DepositCount),
				BlockHash:    hexutil.Encode(b.Body.ExecutionData.BlockHash),
			},
			Graffiti:          hexutil.Encode(b.Body.Graffiti),
			ProposerSlashings: proposerSlashings,
			AttesterSlashings: attesterSlashings,
			Attestations:      atts,
			Deposits:          deposits,
			VoluntaryExits:    exits,
			SyncAggregate: &SyncAggregate{
				SyncCommitteeBits:       hexutil.Encode(b.Body.SyncAggregate.SyncCommitteeBits),
				SyncCommitteeSignatures: syncCommitteeSignatures,
			},
			ExecutionPayload: &ExecutionPayloadZond{
				ParentHash:    hexutil.Encode(b.Body.ExecutionPayload.ParentHash),
				FeeRecipient:  hexutil.EncodeQ(b.Body.ExecutionPayload.FeeRecipient),
				StateRoot:     hexutil.Encode(b.Body.ExecutionPayload.StateRoot),
				ReceiptsRoot:  hexutil.Encode(b.Body.ExecutionPayload.ReceiptsRoot),
				LogsBloom:     hexutil.Encode(b.Body.ExecutionPayload.LogsBloom),
				PrevRandao:    hexutil.Encode(b.Body.ExecutionPayload.PrevRandao),
				BlockNumber:   fmt.Sprintf("%d", b.Body.ExecutionPayload.BlockNumber),
				GasLimit:      fmt.Sprintf("%d", b.Body.ExecutionPayload.GasLimit),
				GasUsed:       fmt.Sprintf("%d", b.Body.ExecutionPayload.GasUsed),
				Timestamp:     fmt.Sprintf("%d", b.Body.ExecutionPayload.Timestamp),
				ExtraData:     hexutil.Encode(b.Body.ExecutionPayload.ExtraData),
				BaseFeePerGas: baseFeePerGas,
				BlockHash:     hexutil.Encode(b.Body.ExecutionPayload.BlockHash),
				Transactions:  transactions,
				Withdrawals:   withdrawals, // new in zond
			},
		},
	}, nil
}

func ProposerSlashingsToConsensus(src []*ProposerSlashing) ([]*qrysmpb.ProposerSlashing, error) {
	if src == nil {
		return nil, errNilValue
	}
	err := VerifyMaxLength(src, 16)
	if err != nil {
		return nil, err
	}
	proposerSlashings := make([]*qrysmpb.ProposerSlashing, len(src))
	for i, s := range src {
		if s == nil {
			return nil, NewDecodeError(errNilValue, fmt.Sprintf("[%d]", i))
		}
		if s.SignedHeader1 == nil {
			return nil, NewDecodeError(errNilValue, fmt.Sprintf("[%d].SignedHeader1", i))
		}
		if s.SignedHeader1.Message == nil {
			return nil, NewDecodeError(errNilValue, fmt.Sprintf("[%d].SignedHeader1.Message", i))
		}
		if s.SignedHeader2 == nil {
			return nil, NewDecodeError(errNilValue, fmt.Sprintf("[%d].SignedHeader2", i))
		}
		if s.SignedHeader2.Message == nil {
			return nil, NewDecodeError(errNilValue, fmt.Sprintf("[%d].SignedHeader2.Message", i))
		}

		h1Sig, err := DecodeHexWithLength(s.SignedHeader1.Signature, fieldparams.MLDSA87SignatureLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader1.Signature", i))
		}
		h1Slot, err := strconv.ParseUint(s.SignedHeader1.Message.Slot, 10, 64)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader1.Message.Slot", i))
		}
		h1ProposerIndex, err := strconv.ParseUint(s.SignedHeader1.Message.ProposerIndex, 10, 64)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader1.Message.ProposerIndex", i))
		}
		h1ParentRoot, err := DecodeHexWithLength(s.SignedHeader1.Message.ParentRoot, fieldparams.RootLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader1.Message.ParentRoot", i))
		}
		h1StateRoot, err := DecodeHexWithLength(s.SignedHeader1.Message.StateRoot, fieldparams.RootLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader1.Message.StateRoot", i))
		}
		h1BodyRoot, err := DecodeHexWithLength(s.SignedHeader1.Message.BodyRoot, fieldparams.RootLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader1.Message.BodyRoot", i))
		}
		h2Sig, err := DecodeHexWithLength(s.SignedHeader2.Signature, fieldparams.MLDSA87SignatureLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader2.Signature", i))
		}
		h2Slot, err := strconv.ParseUint(s.SignedHeader2.Message.Slot, 10, 64)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader2.Message.Slot", i))
		}
		h2ProposerIndex, err := strconv.ParseUint(s.SignedHeader2.Message.ProposerIndex, 10, 64)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader2.Message.ProposerIndex", i))
		}
		h2ParentRoot, err := DecodeHexWithLength(s.SignedHeader2.Message.ParentRoot, fieldparams.RootLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader2.Message.ParentRoot", i))
		}
		h2StateRoot, err := DecodeHexWithLength(s.SignedHeader2.Message.StateRoot, fieldparams.RootLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader2.Message.StateRoot", i))
		}
		h2BodyRoot, err := DecodeHexWithLength(s.SignedHeader2.Message.BodyRoot, fieldparams.RootLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].SignedHeader2.Message.BodyRoot", i))
		}
		proposerSlashings[i] = &qrysmpb.ProposerSlashing{
			Header_1: &qrysmpb.SignedBeaconBlockHeader{
				Header: &qrysmpb.BeaconBlockHeader{
					Slot:          primitives.Slot(h1Slot),
					ProposerIndex: primitives.ValidatorIndex(h1ProposerIndex),
					ParentRoot:    h1ParentRoot,
					StateRoot:     h1StateRoot,
					BodyRoot:      h1BodyRoot,
				},
				Signature: h1Sig,
			},
			Header_2: &qrysmpb.SignedBeaconBlockHeader{
				Header: &qrysmpb.BeaconBlockHeader{
					Slot:          primitives.Slot(h2Slot),
					ProposerIndex: primitives.ValidatorIndex(h2ProposerIndex),
					ParentRoot:    h2ParentRoot,
					StateRoot:     h2StateRoot,
					BodyRoot:      h2BodyRoot,
				},
				Signature: h2Sig,
			},
		}
	}
	return proposerSlashings, nil
}

func ProposerSlashingsFromConsensus(src []*qrysmpb.ProposerSlashing) ([]*ProposerSlashing, error) {
	proposerSlashings := make([]*ProposerSlashing, len(src))
	for i, s := range src {
		proposerSlashings[i] = &ProposerSlashing{
			SignedHeader1: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          fmt.Sprintf("%d", s.Header_1.Header.Slot),
					ProposerIndex: fmt.Sprintf("%d", s.Header_1.Header.ProposerIndex),
					ParentRoot:    hexutil.Encode(s.Header_1.Header.ParentRoot),
					StateRoot:     hexutil.Encode(s.Header_1.Header.StateRoot),
					BodyRoot:      hexutil.Encode(s.Header_1.Header.BodyRoot),
				},
				Signature: hexutil.Encode(s.Header_1.Signature),
			},
			SignedHeader2: &SignedBeaconBlockHeader{
				Message: &BeaconBlockHeader{
					Slot:          fmt.Sprintf("%d", s.Header_2.Header.Slot),
					ProposerIndex: fmt.Sprintf("%d", s.Header_2.Header.ProposerIndex),
					ParentRoot:    hexutil.Encode(s.Header_2.Header.ParentRoot),
					StateRoot:     hexutil.Encode(s.Header_2.Header.StateRoot),
					BodyRoot:      hexutil.Encode(s.Header_2.Header.BodyRoot),
				},
				Signature: hexutil.Encode(s.Header_2.Signature),
			},
		}
	}
	return proposerSlashings, nil
}

func AttesterSlashingsToConsensus(src []*AttesterSlashing) ([]*qrysmpb.AttesterSlashing, error) {
	if src == nil {
		return nil, errNilValue
	}
	err := VerifyMaxLength(src, 2)
	if err != nil {
		return nil, err
	}

	attesterSlashings := make([]*qrysmpb.AttesterSlashing, len(src))
	for i, s := range src {
		if s == nil {
			return nil, NewDecodeError(errNilValue, fmt.Sprintf("[%d]", i))
		}
		if s.Attestation1 == nil {
			return nil, NewDecodeError(errNilValue, fmt.Sprintf("[%d].Attestation1", i))
		}
		if s.Attestation2 == nil {
			return nil, NewDecodeError(errNilValue, fmt.Sprintf("[%d].Attestation2", i))
		}

		a1Sigs := make([][]byte, len(s.Attestation1.Signatures))
		for j, sig := range s.Attestation1.Signatures {
			a1Sig, err := DecodeHexWithLength(sig, fieldparams.MLDSA87SignatureLength)
			if err != nil {
				return nil, NewDecodeError(err, fmt.Sprintf("[%d].Attestation1.Signatures[%d]", i, j))
			}
			a1Sigs[j] = a1Sig
		}

		err = VerifyMaxLength(s.Attestation1.AttestingIndices, 2048)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].Attestation1.AttestingIndices", i))
		}
		a1AttestingIndices := make([]uint64, len(s.Attestation1.AttestingIndices))
		for j, ix := range s.Attestation1.AttestingIndices {
			attestingIndex, err := strconv.ParseUint(ix, 10, 64)
			if err != nil {
				return nil, NewDecodeError(err, fmt.Sprintf("[%d].Attestation1.AttestingIndices[%d]", i, j))
			}
			a1AttestingIndices[j] = attestingIndex
		}
		a1Data, err := s.Attestation1.Data.ToConsensus()
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].Attestation1.Data", i))
		}

		a2Sigs := make([][]byte, len(s.Attestation2.Signatures))
		for j, sig := range s.Attestation2.Signatures {
			a2Sig, err := DecodeHexWithLength(sig, fieldparams.MLDSA87SignatureLength)
			if err != nil {
				return nil, NewDecodeError(err, fmt.Sprintf("[%d].Attestation2.Signatures[%d]", i, j))
			}
			a2Sigs[j] = a2Sig
		}

		err = VerifyMaxLength(s.Attestation2.AttestingIndices, 2048)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].Attestation2.AttestingIndices", i))
		}
		a2AttestingIndices := make([]uint64, len(s.Attestation2.AttestingIndices))
		for j, ix := range s.Attestation2.AttestingIndices {
			attestingIndex, err := strconv.ParseUint(ix, 10, 64)
			if err != nil {
				return nil, NewDecodeError(err, fmt.Sprintf("[%d].Attestation2.AttestingIndices[%d]", i, j))
			}
			a2AttestingIndices[j] = attestingIndex
		}
		a2Data, err := s.Attestation2.Data.ToConsensus()
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].Attestation2.Data", i))
		}
		attesterSlashings[i] = &qrysmpb.AttesterSlashing{
			Attestation_1: &qrysmpb.IndexedAttestation{
				AttestingIndices: a1AttestingIndices,
				Data:             a1Data,
				Signatures:       a1Sigs,
			},
			Attestation_2: &qrysmpb.IndexedAttestation{
				AttestingIndices: a2AttestingIndices,
				Data:             a2Data,
				Signatures:       a2Sigs,
			},
		}
	}
	return attesterSlashings, nil
}

func AttesterSlashingsFromConsensus(src []*qrysmpb.AttesterSlashing) ([]*AttesterSlashing, error) {
	attesterSlashings := make([]*AttesterSlashing, len(src))
	for i, s := range src {
		a1Sigs := make([]string, len(s.Attestation_1.Signatures))
		for i, sig := range s.Attestation_1.Signatures {
			a1Sigs[i] = hexutil.Encode(sig)
		}

		a2Sigs := make([]string, len(s.Attestation_2.Signatures))
		for i, sig := range s.Attestation_2.Signatures {
			a2Sigs[i] = hexutil.Encode(sig)
		}

		a1AttestingIndices := make([]string, len(s.Attestation_1.AttestingIndices))
		for j, ix := range s.Attestation_1.AttestingIndices {
			a1AttestingIndices[j] = fmt.Sprintf("%d", ix)
		}
		a2AttestingIndices := make([]string, len(s.Attestation_2.AttestingIndices))
		for j, ix := range s.Attestation_2.AttestingIndices {
			a2AttestingIndices[j] = fmt.Sprintf("%d", ix)
		}
		attesterSlashings[i] = &AttesterSlashing{
			Attestation1: &IndexedAttestation{
				AttestingIndices: a1AttestingIndices,
				Data: &AttestationData{
					Slot:            fmt.Sprintf("%d", s.Attestation_1.Data.Slot),
					CommitteeIndex:  fmt.Sprintf("%d", s.Attestation_1.Data.CommitteeIndex),
					BeaconBlockRoot: hexutil.Encode(s.Attestation_1.Data.BeaconBlockRoot),
					Source: &Checkpoint{
						Epoch: fmt.Sprintf("%d", s.Attestation_1.Data.Source.Epoch),
						Root:  hexutil.Encode(s.Attestation_1.Data.Source.Root),
					},
					Target: &Checkpoint{
						Epoch: fmt.Sprintf("%d", s.Attestation_1.Data.Target.Epoch),
						Root:  hexutil.Encode(s.Attestation_1.Data.Target.Root),
					},
				},
				Signatures: a1Sigs,
			},
			Attestation2: &IndexedAttestation{
				AttestingIndices: a2AttestingIndices,
				Data: &AttestationData{
					Slot:            fmt.Sprintf("%d", s.Attestation_2.Data.Slot),
					CommitteeIndex:  fmt.Sprintf("%d", s.Attestation_2.Data.CommitteeIndex),
					BeaconBlockRoot: hexutil.Encode(s.Attestation_2.Data.BeaconBlockRoot),
					Source: &Checkpoint{
						Epoch: fmt.Sprintf("%d", s.Attestation_2.Data.Source.Epoch),
						Root:  hexutil.Encode(s.Attestation_2.Data.Source.Root),
					},
					Target: &Checkpoint{
						Epoch: fmt.Sprintf("%d", s.Attestation_2.Data.Target.Epoch),
						Root:  hexutil.Encode(s.Attestation_2.Data.Target.Root),
					},
				},
				Signatures: a2Sigs,
			},
		}
	}
	return attesterSlashings, nil
}

func AttsToConsensus(src []*Attestation) ([]*qrysmpb.Attestation, error) {
	if src == nil {
		return nil, errNilValue
	}
	err := VerifyMaxLength(src, 128)
	if err != nil {
		return nil, err
	}

	atts := make([]*qrysmpb.Attestation, len(src))
	for i, a := range src {
		atts[i], err = a.ToConsensus()
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d]", i))
		}
	}
	return atts, nil
}

func AttsFromConsensus(src []*qrysmpb.Attestation) ([]*Attestation, error) {
	atts := make([]*Attestation, len(src))
	for i, a := range src {
		atts[i] = AttestationFromConsensus(a)
	}
	return atts, nil
}

func DepositsToConsensus(src []*Deposit) ([]*qrysmpb.Deposit, error) {
	if src == nil {
		return nil, errNilValue
	}
	err := VerifyMaxLength(src, 16)
	if err != nil {
		return nil, err
	}

	deposits := make([]*qrysmpb.Deposit, len(src))
	for i, d := range src {
		if d.Data == nil {
			return nil, NewDecodeError(errNilValue, fmt.Sprintf("[%d].Data", i))
		}

		err = VerifyMaxLength(d.Proof, 33)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].Proof", i))
		}
		proof := make([][]byte, len(d.Proof))
		for j, p := range d.Proof {
			var err error
			proof[j], err = DecodeHexWithLength(p, fieldparams.RootLength)
			if err != nil {
				return nil, NewDecodeError(err, fmt.Sprintf("[%d].Proof[%d]", i, j))
			}
		}
		pubkey, err := DecodeHexWithLength(d.Data.Pubkey, fieldparams.MLDSA87PubkeyLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].Pubkey", i))
		}
		withdrawalCreds, err := DecodeHexWithLength(d.Data.WithdrawalCredentials, fieldparams.WithdrawalCredentialsLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].WithdrawalCredentials", i))
		}
		amount, err := strconv.ParseUint(d.Data.Amount, 10, 64)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].Amount", i))
		}
		sig, err := DecodeHexWithLength(d.Data.Signature, fieldparams.MLDSA87SignatureLength)
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d].Signature", i))
		}
		deposits[i] = &qrysmpb.Deposit{
			Proof: proof,
			Data: &qrysmpb.Deposit_Data{
				PublicKey:             pubkey,
				WithdrawalCredentials: withdrawalCreds,
				Amount:                amount,
				Signature:             sig,
			},
		}
	}
	return deposits, nil
}

func DepositsFromConsensus(src []*qrysmpb.Deposit) ([]*Deposit, error) {
	deposits := make([]*Deposit, len(src))
	for i, d := range src {
		proof := make([]string, len(d.Proof))
		for j, p := range d.Proof {
			proof[j] = hexutil.Encode(p)
		}
		deposits[i] = &Deposit{
			Proof: proof,
			Data: &DepositData{
				Pubkey:                hexutil.Encode(d.Data.PublicKey),
				WithdrawalCredentials: hexutil.Encode(d.Data.WithdrawalCredentials),
				Amount:                fmt.Sprintf("%d", d.Data.Amount),
				Signature:             hexutil.Encode(d.Data.Signature),
			},
		}
	}
	return deposits, nil
}

func ExitsToConsensus(src []*SignedVoluntaryExit) ([]*qrysmpb.SignedVoluntaryExit, error) {
	if src == nil {
		return nil, errNilValue
	}
	err := VerifyMaxLength(src, 16)
	if err != nil {
		return nil, err
	}

	exits := make([]*qrysmpb.SignedVoluntaryExit, len(src))
	for i, e := range src {
		exits[i], err = e.ToConsensus()
		if err != nil {
			return nil, NewDecodeError(err, fmt.Sprintf("[%d]", i))
		}
	}
	return exits, nil
}

func ExitsFromConsensus(src []*qrysmpb.SignedVoluntaryExit) ([]*SignedVoluntaryExit, error) {
	exits := make([]*SignedVoluntaryExit, len(src))
	for i, e := range src {
		exits[i] = &SignedVoluntaryExit{
			Message: &VoluntaryExit{
				Epoch:          fmt.Sprintf("%d", e.Exit.Epoch),
				ValidatorIndex: fmt.Sprintf("%d", e.Exit.ValidatorIndex),
			},
			Signature: hexutil.Encode(e.Signature),
		}
	}
	return exits, nil
}

func Uint256ToSSZBytes(num string) ([]byte, error) {
	uint256, ok := new(big.Int).SetString(num, 10)
	if !ok {
		return nil, errors.New("could not parse Uint256")
	}
	if !math.IsValidUint256(uint256) {
		return nil, fmt.Errorf("%s is not a valid Uint256", num)
	}
	return bytesutil2.PadTo(bytesutil2.ReverseByteOrder(uint256.Bytes()), 32), nil
}

func sszBytesToUint256String(b []byte) (string, error) {
	bi := bytesutil2.LittleEndianBytesToBigInt(b)
	if !math.IsValidUint256(bi) {
		return "", fmt.Errorf("%s is not a valid Uint256", bi.String())
	}
	return string([]byte(bi.String())), nil
}
