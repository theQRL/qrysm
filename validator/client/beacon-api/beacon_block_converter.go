package beacon_api

import (
	"math/big"
	"strconv"

	"github.com/pkg/errors"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

type beaconBlockConverter interface {
	ConvertRESTCapellaBlockToProto(block *apimiddleware.BeaconBlockCapellaJson) (*zondpb.BeaconBlockCapella, error)
}

type beaconApiBeaconBlockConverter struct{}

// ConvertRESTCapellaBlockToProto converts a Capella JSON beacon block to its protobuf equivalent
func (c beaconApiBeaconBlockConverter) ConvertRESTCapellaBlockToProto(block *apimiddleware.BeaconBlockCapellaJson) (*zondpb.BeaconBlockCapella, error) {
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

	if block.Body.Eth1Data == nil {
		return nil, errors.New("eth1 data is nil")
	}

	depositRoot, err := hexutil.Decode(block.Body.Eth1Data.DepositRoot)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode deposit root `%s`", block.Body.Eth1Data.DepositRoot)
	}

	depositCount, err := strconv.ParseUint(block.Body.Eth1Data.DepositCount, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse deposit count `%s`", block.Body.Eth1Data.DepositCount)
	}

	blockHash, err := hexutil.Decode(block.Body.Eth1Data.BlockHash)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode block hash `%s`", block.Body.Eth1Data.BlockHash)
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

	syncCommitteeSignature, err := hexutil.Decode(block.Body.SyncAggregate.SyncCommitteeSignature)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode sync committee signature `%s`", block.Body.SyncAggregate.SyncCommitteeSignature)
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

	dilithiumToExecutionChanges, err := convertDilithiumToExecutionChangesToProto(block.Body.DilithiumToExecutionChanges)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get dilithium to execution changes")
	}

	return &zondpb.BeaconBlockCapella{
		Slot:          primitives.Slot(blockSlot),
		ProposerIndex: primitives.ValidatorIndex(blockProposerIndex),
		ParentRoot:    parentRoot,
		StateRoot:     stateRoot,
		Body: &zondpb.BeaconBlockBodyCapella{
			RandaoReveal: randaoReveal,
			Eth1Data: &zondpb.Eth1Data{
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
			SyncAggregate: &zondpb.SyncAggregate{
				SyncCommitteeBits:      syncCommitteeBits,
				SyncCommitteeSignature: syncCommitteeSignature,
			},
			ExecutionPayload: &enginev1.ExecutionPayloadCapella{
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
			DilithiumToExecutionChanges: dilithiumToExecutionChanges,
		},
	}, nil
}
