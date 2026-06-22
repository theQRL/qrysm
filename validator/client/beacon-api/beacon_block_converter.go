package beacon_api

import (
	"math/big"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

type beaconBlockConverter interface {
	ConvertRESTZondBlockToProto(block *apimiddleware.BeaconBlockZondJson) (*qrysmpb.BeaconBlockZond, error)
}

type beaconApiBeaconBlockConverter struct{}

// ConvertRESTZondBlockToProto converts a Zond JSON beacon block to its protobuf equivalent
func (c beaconApiBeaconBlockConverter) ConvertRESTZondBlockToProto(block *apimiddleware.BeaconBlockZondJson) (*qrysmpb.BeaconBlockZond, error) {
	if block.Body == nil {
		return nil, errors.New("block body is nil")
	}

	if block.Body.ExecutionPayload == nil {
		return nil, errors.New("execution payload is nil")
	}

	blockSlot, err := strconv.ParseUint(block.Slot, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse slot `%s`", block.Slot)
	}

	blockProposerIndex, err := strconv.ParseUint(block.ProposerIndex, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse proposer index `%s`", block.ProposerIndex)
	}

	parentRoot, err := hexutil.Decode(block.ParentRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode parent root `%s`", block.ParentRoot)
	}

	stateRoot, err := hexutil.Decode(block.StateRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode state root `%s`", block.StateRoot)
	}

	if block.Body == nil {
		return nil, errors.New("block body is nil")
	}

	randaoReveal, err := hexutil.Decode(block.Body.RandaoReveal)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode randao reveal `%s`", block.Body.RandaoReveal)
	}

	if block.Body.ExecutionData == nil {
		return nil, errors.New("execution data is nil")
	}

	depositRoot, err := hexutil.Decode(block.Body.ExecutionData.DepositRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode deposit root `%s`", block.Body.ExecutionData.DepositRoot)
	}

	depositCount, err := strconv.ParseUint(block.Body.ExecutionData.DepositCount, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse deposit count `%s`", block.Body.ExecutionData.DepositCount)
	}

	blockHash, err := hexutil.Decode(block.Body.ExecutionData.BlockHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode block hash `%s`", block.Body.ExecutionData.BlockHash)
	}

	graffiti, err := hexutil.Decode(block.Body.Graffiti)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode graffiti `%s`", block.Body.Graffiti)
	}

	proposerSlashings, err := convertProposerSlashingsToProto(block.Body.ProposerSlashings)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get proposer slashings")
	}

	attesterSlashings, err := convertAttesterSlashingsToProto(block.Body.AttesterSlashings)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get attester slashings")
	}

	attestations, err := convertAttestationsToProto(block.Body.Attestations)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get attestations")
	}

	deposits, err := convertDepositsToProto(block.Body.Deposits)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get deposits")
	}

	voluntaryExits, err := convertVoluntaryExitsToProto(block.Body.VoluntaryExits)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get voluntary exits")
	}

	if block.Body.SyncAggregate == nil {
		return nil, errors.New("sync aggregate is nil")
	}

	syncCommitteeBits, err := hexutil.Decode(block.Body.SyncAggregate.SyncCommitteeBits)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode sync committee bits `%s`", block.Body.SyncAggregate.SyncCommitteeBits)
	}

	syncCommitteeSignatures := make([][]byte, len(block.Body.SyncAggregate.SyncCommitteeSignatures))
	for i, sig := range block.Body.SyncAggregate.SyncCommitteeSignatures {
		syncCommitteeSignatures[i], err = hexutil.Decode(sig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode sync committee signature `%s`", sig)
		}
	}

	if block.Body.ExecutionPayload == nil {
		return nil, errors.New("execution payload is nil")
	}

	parentHash, err := hexutil.Decode(block.Body.ExecutionPayload.ParentHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode execution payload parent hash `%s`", block.Body.ExecutionPayload.ParentHash)
	}

	feeRecipient, err := hexutil.Decode(block.Body.ExecutionPayload.FeeRecipient)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode execution payload fee recipient `%s`", block.Body.ExecutionPayload.FeeRecipient)
	}

	executionStateRoot, err := hexutil.Decode(block.Body.ExecutionPayload.StateRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode execution payload state root `%s`", block.Body.ExecutionPayload.StateRoot)
	}

	receiptsRoot, err := hexutil.Decode(block.Body.ExecutionPayload.ReceiptsRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode execution payload receipts root `%s`", block.Body.ExecutionPayload.ReceiptsRoot)
	}

	logsBloom, err := hexutil.Decode(block.Body.ExecutionPayload.LogsBloom)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode execution payload logs bloom `%s`", block.Body.ExecutionPayload.LogsBloom)
	}

	prevRandao, err := hexutil.Decode(block.Body.ExecutionPayload.PrevRandao)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode execution payload prev randao `%s`", block.Body.ExecutionPayload.PrevRandao)
	}

	blockNumber, err := strconv.ParseUint(block.Body.ExecutionPayload.BlockNumber, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse execution payload block number `%s`", block.Body.ExecutionPayload.BlockNumber)
	}

	gasLimit, err := strconv.ParseUint(block.Body.ExecutionPayload.GasLimit, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse execution payload gas limit `%s`", block.Body.ExecutionPayload.GasLimit)
	}

	gasUsed, err := strconv.ParseUint(block.Body.ExecutionPayload.GasUsed, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse execution payload gas used `%s`", block.Body.ExecutionPayload.GasUsed)
	}

	timestamp, err := strconv.ParseUint(block.Body.ExecutionPayload.TimeStamp, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse execution payload timestamp `%s`", block.Body.ExecutionPayload.TimeStamp)
	}

	extraData, err := hexutil.Decode(block.Body.ExecutionPayload.ExtraData)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode execution payload extra data `%s`", block.Body.ExecutionPayload.ExtraData)
	}

	baseFeePerGas := new(big.Int)
	if _, ok := baseFeePerGas.SetString(block.Body.ExecutionPayload.BaseFeePerGas, 10); !ok {
		return nil, errors.Errorf("failed to parse execution payload base fee per gas `%s`", block.Body.ExecutionPayload.BaseFeePerGas)
	}

	executionBlockHash, err := hexutil.Decode(block.Body.ExecutionPayload.BlockHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode execution payload block hash `%s`", block.Body.ExecutionPayload.BlockHash)
	}

	transactions, err := convertTransactionsToProto(block.Body.ExecutionPayload.Transactions)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get execution payload transactions")
	}

	withdrawals, err := convertWithdrawalsToProto(block.Body.ExecutionPayload.Withdrawals)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get withdrawals")
	}

	return &qrysmpb.BeaconBlockZond{
		Slot:          primitives.Slot(blockSlot),
		ProposerIndex: primitives.ValidatorIndex(blockProposerIndex),
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
			Attestations:      attestations,
			Deposits:          deposits,
			VoluntaryExits:    voluntaryExits,
			SyncAggregate: &qrysmpb.SyncAggregate{
				SyncCommitteeBits:       syncCommitteeBits,
				SyncCommitteeSignatures: syncCommitteeSignatures,
			},
			ExecutionPayload: &enginev1.ExecutionPayloadZond{
				ParentHash:    parentHash,
				FeeRecipient:  feeRecipient,
				StateRoot:     executionStateRoot,
				ReceiptsRoot:  receiptsRoot,
				LogsBloom:     logsBloom,
				PrevRandao:    prevRandao,
				BlockNumber:   blockNumber,
				GasLimit:      gasLimit,
				GasUsed:       gasUsed,
				Timestamp:     timestamp,
				ExtraData:     extraData,
				BaseFeePerGas: bytesutil.PadTo(bytesutil.BigIntToLittleEndianBytes(baseFeePerGas), 32),
				BlockHash:     executionBlockHash,
				Transactions:  transactions,
				Withdrawals:   withdrawals,
			},
		},
	}, nil
}
