package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/theQRL/go-qrl/common/hexutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/apimiddleware"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/validator/client/beacon-api/mock"
	test_helpers "github.com/theQRL/qrysm/validator/client/beacon-api/test-helpers"
)

func TestProposeBeaconBlock_Zond(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	jsonRestHandler := mock.NewMockjsonRestHandler(ctrl)

	zondBlock := generateSignedZondBlock()

	genericSignedBlock := &qrysmpb.GenericSignedBeaconBlock{}
	genericSignedBlock.Block = zondBlock

	jsonZondBlock := &apimiddleware.SignedBeaconBlockZondJson{
		Signature: hexutil.Encode(zondBlock.Zond.Signature),
		Message: &apimiddleware.BeaconBlockZondJson{
			ParentRoot:    hexutil.Encode(zondBlock.Zond.Block.ParentRoot),
			ProposerIndex: uint64ToString(zondBlock.Zond.Block.ProposerIndex),
			Slot:          uint64ToString(zondBlock.Zond.Block.Slot),
			StateRoot:     hexutil.Encode(zondBlock.Zond.Block.StateRoot),
			Body: &apimiddleware.BeaconBlockBodyZondJson{
				Attestations:      jsonifyAttestations(zondBlock.Zond.Block.Body.Attestations),
				AttesterSlashings: jsonifyAttesterSlashings(zondBlock.Zond.Block.Body.AttesterSlashings),
				Deposits:          jsonifyDeposits(zondBlock.Zond.Block.Body.Deposits),
				ExecutionData:     jsonifyExecutionData(zondBlock.Zond.Block.Body.ExecutionData),
				Graffiti:          hexutil.Encode(zondBlock.Zond.Block.Body.Graffiti),
				ProposerSlashings: jsonifyProposerSlashings(zondBlock.Zond.Block.Body.ProposerSlashings),
				RandaoReveal:      hexutil.Encode(zondBlock.Zond.Block.Body.RandaoReveal),
				VoluntaryExits:    JsonifySignedVoluntaryExits(zondBlock.Zond.Block.Body.VoluntaryExits),
				SyncAggregate:     JsonifySyncAggregate(zondBlock.Zond.Block.Body.SyncAggregate),
				ExecutionPayload: &apimiddleware.ExecutionPayloadZondJson{
					BaseFeePerGas: bytesutil.LittleEndianBytesToBigInt(zondBlock.Zond.Block.Body.ExecutionPayload.BaseFeePerGas).String(),
					BlockHash:     hexutil.Encode(zondBlock.Zond.Block.Body.ExecutionPayload.BlockHash),
					BlockNumber:   uint64ToString(zondBlock.Zond.Block.Body.ExecutionPayload.BlockNumber),
					ExtraData:     hexutil.Encode(zondBlock.Zond.Block.Body.ExecutionPayload.ExtraData),
					FeeRecipient:  hexutil.Encode(zondBlock.Zond.Block.Body.ExecutionPayload.FeeRecipient),
					GasLimit:      uint64ToString(zondBlock.Zond.Block.Body.ExecutionPayload.GasLimit),
					GasUsed:       uint64ToString(zondBlock.Zond.Block.Body.ExecutionPayload.GasUsed),
					LogsBloom:     hexutil.Encode(zondBlock.Zond.Block.Body.ExecutionPayload.LogsBloom),
					ParentHash:    hexutil.Encode(zondBlock.Zond.Block.Body.ExecutionPayload.ParentHash),
					PrevRandao:    hexutil.Encode(zondBlock.Zond.Block.Body.ExecutionPayload.PrevRandao),
					ReceiptsRoot:  hexutil.Encode(zondBlock.Zond.Block.Body.ExecutionPayload.ReceiptsRoot),
					StateRoot:     hexutil.Encode(zondBlock.Zond.Block.Body.ExecutionPayload.StateRoot),
					TimeStamp:     uint64ToString(zondBlock.Zond.Block.Body.ExecutionPayload.Timestamp),
					Transactions:  jsonifyTransactions(zondBlock.Zond.Block.Body.ExecutionPayload.Transactions),
					Withdrawals:   jsonifyWithdrawals(zondBlock.Zond.Block.Body.ExecutionPayload.Withdrawals),
				},
			},
		},
	}

	marshalledBlock, err := json.Marshal(jsonZondBlock)
	require.NoError(t, err)

	// Make sure that what we send in the POST body is the marshalled version of the protobuf block
	headers := map[string]string{"Qrl-Consensus-Version": "zond"}
	jsonRestHandler.EXPECT().PostRestJson(
		context.Background(),
		"/qrl/v1/beacon/blocks",
		headers,
		bytes.NewBuffer(marshalledBlock),
		nil,
	)

	validatorClient := &beaconApiValidatorClient{jsonRestHandler: jsonRestHandler}
	proposeResponse, err := validatorClient.proposeBeaconBlock(context.Background(), genericSignedBlock)
	assert.NoError(t, err)
	require.NotNil(t, proposeResponse)

	expectedBlockRoot, err := zondBlock.Zond.Block.HashTreeRoot()
	require.NoError(t, err)

	// Make sure that the block root is set
	assert.DeepEqual(t, expectedBlockRoot[:], proposeResponse.BlockRoot)
}

func generateSignedZondBlock() *qrysmpb.GenericSignedBeaconBlock_Zond {
	return &qrysmpb.GenericSignedBeaconBlock_Zond{
		Zond: &qrysmpb.SignedBeaconBlockZond{
			Block:     test_helpers.GenerateProtoZondBeaconBlock(),
			Signature: test_helpers.FillByteSlice(4627, 127),
		},
	}
}
