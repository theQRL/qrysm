package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

func (c beaconApiValidatorClient) proposeBeaconBlock(ctx context.Context, in *qrysmpb.GenericSignedBeaconBlock) (*qrysmpb.ProposeResponse, error) {
	var consensusVersion string
	var beaconBlockRoot [32]byte

	var err error
	var marshalledSignedBeaconBlockJson []byte
	blinded := false

	switch blockType := in.Block.(type) {
	case *qrysmpb.GenericSignedBeaconBlock_Zond:
		consensusVersion = "zond"
		beaconBlockRoot, err = blockType.Zond.Block.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for zond beacon block")
		}

		marshalledSignedBeaconBlockJson, err = marshallBeaconBlockZond(blockType.Zond)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshall zond beacon block")
		}
	case *qrysmpb.GenericSignedBeaconBlock_BlindedZond:
		blinded = true
		consensusVersion = "zond"
		beaconBlockRoot, err = blockType.BlindedZond.Block.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute block root for blinded zond beacon block")
		}

		marshalledSignedBeaconBlockJson, err = marshallBeaconBlockBlindedZond(blockType.BlindedZond)
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshall blinded zond beacon block")
		}
	default:
		return nil, errors.Errorf("unsupported block type %T", in.Block)
	}

	var endpoint string

	if blinded {
		endpoint = "/qrl/v1/beacon/blinded_blocks"
	} else {
		endpoint = "/qrl/v1/beacon/blocks"
	}

	headers := map[string]string{"Qrl-Consensus-Version": consensusVersion}
	if httpError, err := c.jsonRestHandler.PostRestJson(ctx, endpoint, headers, bytes.NewBuffer(marshalledSignedBeaconBlockJson), nil); err != nil {
		if httpError != nil && httpError.Code == http.StatusAccepted {
			// Error 202 means that the block was successfully broadcasted, but validation failed
			return nil, errors.Wrap(err, "block was successfully broadcasted but failed validation")
		}

		return nil, errors.Wrap(err, "failed to send POST data to REST endpoint")
	}

	return &qrysmpb.ProposeResponse{BlockRoot: beaconBlockRoot[:]}, nil
}

func marshallBeaconBlockZond(block *qrysmpb.SignedBeaconBlockZond) ([]byte, error) {
	signedBeaconBlockZondJson := &apimiddleware.SignedBeaconBlockZondJson{
		Signature: hexutil.Encode(block.Signature),
		Message: &apimiddleware.BeaconBlockZondJson{
			ParentRoot:    hexutil.Encode(block.Block.ParentRoot),
			ProposerIndex: uint64ToString(block.Block.ProposerIndex),
			Slot:          uint64ToString(block.Block.Slot),
			StateRoot:     hexutil.Encode(block.Block.StateRoot),
			Body: &apimiddleware.BeaconBlockBodyZondJson{
				Attestations:      jsonifyAttestations(block.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(block.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(block.Block.Body.Deposits),
				ExecutionData:     jsonifyExecutionData(block.Block.Body.ExecutionData),
				Graffiti:          hexutil.Encode(block.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(block.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(block.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(block.Block.Body.VoluntaryExits),
				SyncAggregate:     JsonifySyncAggregate(block.Block.Body.SyncAggregate),
				ExecutionPayload: &apimiddleware.ExecutionPayloadZondJson{
					BaseFeePerGas: bytesutil.LittleEndianBytesToBigInt(block.Block.Body.ExecutionPayload.BaseFeePerGas).String(),
					BlockHash:     hexutil.Encode(block.Block.Body.ExecutionPayload.BlockHash),
					BlockNumber:   uint64ToString(block.Block.Body.ExecutionPayload.BlockNumber),
					ExtraData:     hexutil.Encode(block.Block.Body.ExecutionPayload.ExtraData),
					FeeRecipient:  hexutil.Encode(block.Block.Body.ExecutionPayload.FeeRecipient),
					GasLimit:      uint64ToString(block.Block.Body.ExecutionPayload.GasLimit),
					GasUsed:       uint64ToString(block.Block.Body.ExecutionPayload.GasUsed),
					LogsBloom:     hexutil.Encode(block.Block.Body.ExecutionPayload.LogsBloom),
					ParentHash:    hexutil.Encode(block.Block.Body.ExecutionPayload.ParentHash),
					PrevRandao:    hexutil.Encode(block.Block.Body.ExecutionPayload.PrevRandao),
					ReceiptsRoot:  hexutil.Encode(block.Block.Body.ExecutionPayload.ReceiptsRoot),
					StateRoot:     hexutil.Encode(block.Block.Body.ExecutionPayload.StateRoot),
					TimeStamp:     uint64ToString(block.Block.Body.ExecutionPayload.Timestamp),
					Transactions:  jsonifyTransactions(block.Block.Body.ExecutionPayload.Transactions),
					Withdrawals:   jsonifyWithdrawals(block.Block.Body.ExecutionPayload.Withdrawals),
				},
			},
		},
	}

	return json.Marshal(signedBeaconBlockZondJson)
}

func marshallBeaconBlockBlindedZond(block *qrysmpb.SignedBlindedBeaconBlockZond) ([]byte, error) {
	signedBeaconBlockZondJson := &apimiddleware.SignedBlindedBeaconBlockZondJson{
		Signature: hexutil.Encode(block.Signature),
		Message: &apimiddleware.BlindedBeaconBlockZondJson{
			ParentRoot:    hexutil.Encode(block.Block.ParentRoot),
			ProposerIndex: uint64ToString(block.Block.ProposerIndex),
			Slot:          uint64ToString(block.Block.Slot),
			StateRoot:     hexutil.Encode(block.Block.StateRoot),
			Body: &apimiddleware.BlindedBeaconBlockBodyZondJson{
				Attestations:      jsonifyAttestations(block.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(block.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(block.Block.Body.Deposits),
				ExecutionData:     jsonifyExecutionData(block.Block.Body.ExecutionData),
				Graffiti:          hexutil.Encode(block.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(block.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(block.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(block.Block.Body.VoluntaryExits),
				SyncAggregate:     JsonifySyncAggregate(block.Block.Body.SyncAggregate),
				ExecutionPayloadHeader: &apimiddleware.ExecutionPayloadHeaderZondJson{
					BaseFeePerGas:    bytesutil.LittleEndianBytesToBigInt(block.Block.Body.ExecutionPayloadHeader.BaseFeePerGas).String(),
					BlockHash:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.BlockHash),
					BlockNumber:      uint64ToString(block.Block.Body.ExecutionPayloadHeader.BlockNumber),
					ExtraData:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.ExtraData),
					FeeRecipient:     hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.FeeRecipient),
					GasLimit:         uint64ToString(block.Block.Body.ExecutionPayloadHeader.GasLimit),
					GasUsed:          uint64ToString(block.Block.Body.ExecutionPayloadHeader.GasUsed),
					LogsBloom:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.LogsBloom),
					ParentHash:       hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.ParentHash),
					PrevRandao:       hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.PrevRandao),
					ReceiptsRoot:     hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.ReceiptsRoot),
					StateRoot:        hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.StateRoot),
					TimeStamp:        uint64ToString(block.Block.Body.ExecutionPayloadHeader.Timestamp),
					TransactionsRoot: hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.TransactionsRoot),
					WithdrawalsRoot:  hexutil.Encode(block.Block.Body.ExecutionPayloadHeader.WithdrawalsRoot),
				},
			},
		},
	}

	return json.Marshal(signedBeaconBlockZondJson)
}
