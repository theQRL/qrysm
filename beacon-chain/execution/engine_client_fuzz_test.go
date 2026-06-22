package execution_test

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/theQRL/go-qrl/beacon/engine"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/qrysm/beacon-chain/execution"
	pb "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/testing/assert"
)

func FuzzForkChoiceResponse(f *testing.F) {
	valHash := common.Hash([32]byte{0xFF, 0x01})
	payloadID := engine.PayloadID([8]byte{0x01, 0xFF, 0xAA, 0x00, 0xEE, 0xFE, 0x00, 0x00})
	valErr := "asjajshjahsaj"
	seed := &engine.ForkChoiceResponse{
		PayloadStatus: engine.PayloadStatusV1{
			Status:          "INVALID_TERMINAL_BLOCK",
			LatestValidHash: &valHash,
			ValidationError: &valErr,
		},
		PayloadID: &payloadID,
	}
	output, err := json.Marshal(seed)
	assert.NoError(f, err)
	f.Add(output)
	f.Fuzz(func(t *testing.T, jsonBlob []byte) {
		gqrlResp := &engine.ForkChoiceResponse{}
		qrysmResp := &execution.ForkchoiceUpdatedResponse{}
		gqrlErr := json.Unmarshal(jsonBlob, gqrlResp)
		qrysmErr := json.Unmarshal(jsonBlob, qrysmResp)
		assert.Equal(t, gqrlErr != nil, qrysmErr != nil, fmt.Sprintf("gqrl and qrysm unmarshaller return inconsistent errors. %v and %v", gqrlErr, qrysmErr))
		// Nothing to marshal if we have an error.
		if gqrlErr != nil {
			return
		}
		gqrlBlob, gqrlErr := json.Marshal(gqrlResp)
		qrysmBlob, qrysmErr := json.Marshal(qrysmResp)
		assert.Equal(t, gqrlErr != nil, qrysmErr != nil, "gqrl and qrysm unmarshaller return inconsistent errors")
		newGqrlResp := &engine.ForkChoiceResponse{}
		newGqrlErr := json.Unmarshal(qrysmBlob, newGqrlResp)
		assert.NoError(t, newGqrlErr)
		if newGqrlResp.PayloadStatus.Status == "UNKNOWN" {
			return
		}

		newGqrlResp2 := &engine.ForkChoiceResponse{}
		newGqrlErr = json.Unmarshal(gqrlBlob, newGqrlResp2)
		assert.NoError(t, newGqrlErr)

		assert.DeepEqual(t, newGqrlResp.PayloadID, newGqrlResp2.PayloadID)
		assert.DeepEqual(t, newGqrlResp.PayloadStatus.Status, newGqrlResp2.PayloadStatus.Status)
		assert.DeepEqual(t, newGqrlResp.PayloadStatus.LatestValidHash, newGqrlResp2.PayloadStatus.LatestValidHash)
		isNilOrEmpty := newGqrlResp.PayloadStatus.ValidationError == nil || (*newGqrlResp.PayloadStatus.ValidationError == "")
		isNilOrEmpty2 := newGqrlResp2.PayloadStatus.ValidationError == nil || (*newGqrlResp2.PayloadStatus.ValidationError == "")
		assert.DeepEqual(t, isNilOrEmpty, isNilOrEmpty2)
		if !isNilOrEmpty {
			assert.DeepEqual(t, *newGqrlResp.PayloadStatus.ValidationError, *newGqrlResp2.PayloadStatus.ValidationError)
		}
	})
}

func FuzzExecutionPayload(f *testing.F) {
	logsBloom := [256]byte{'j', 'u', 'n', 'k'}
	execData := &engine.ExecutionPayloadEnvelope{
		ExecutionPayload: &engine.ExecutableData{
			ParentHash: common.Hash([32]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}),
			FeeRecipient: common.Address([64]byte{
				0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01,
				0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF,
				0x01, 0xFF, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01,
				0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01, 0xFF,
				0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0xFF, 0x01,
				0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01,
				0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01,
				0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01,
			}),
			StateRoot:     common.Hash([32]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}),
			ReceiptsRoot:  common.Hash([32]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}),
			LogsBloom:     logsBloom[:],
			Random:        common.Hash([32]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}),
			Number:        math.MaxUint64,
			GasLimit:      math.MaxUint64,
			GasUsed:       math.MaxUint64,
			Timestamp:     100,
			ExtraData:     nil,
			BaseFeePerGas: big.NewInt(math.MaxInt),
			BlockHash:     common.Hash([32]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}),
			Transactions:  [][]byte{{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}, {0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}, {0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}, {0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}},
			Withdrawals:   []*types.Withdrawal{},
		},
		BlockValue: nil,
	}
	output, err := json.Marshal(execData)
	assert.NoError(f, err)
	f.Add(output)
	f.Fuzz(func(t *testing.T, jsonBlob []byte) {
		gqrlResp := &engine.ExecutionPayloadEnvelope{}
		qrysmResp := &pb.ExecutionPayloadZondWithValue{}
		gqrlErr := json.Unmarshal(jsonBlob, gqrlResp)
		qrysmErr := json.Unmarshal(jsonBlob, qrysmResp)
		assert.Equal(t, gqrlErr != nil, qrysmErr != nil, fmt.Sprintf("gqrl and qrysm unmarshaller return inconsistent errors. %v and %v", gqrlErr, qrysmErr))
		// Nothing to marshal if we have an error.
		if gqrlErr != nil {
			return
		}
		gqrlBlob, gqrlErr := json.Marshal(gqrlResp)
		qrysmBlob, qrysmErr := json.Marshal(qrysmResp)
		assert.Equal(t, gqrlErr != nil, qrysmErr != nil, "gqrl and qrysm unmarshaller return inconsistent errors")
		newGqrlResp := &engine.ExecutionPayloadEnvelope{}
		newGqrlErr := json.Unmarshal(qrysmBlob, newGqrlResp)
		assert.NoError(t, newGqrlErr)
		newGqrlResp2 := &engine.ExecutionPayloadEnvelope{}
		newGqrlErr = json.Unmarshal(gqrlBlob, newGqrlResp2)
		assert.NoError(t, newGqrlErr)

		assert.DeepEqual(t, newGqrlResp, newGqrlResp2)
	})
}

func FuzzExecutionBlock(f *testing.F) {
	f.Skip("Is skipped until false positive rate can be resolved.")
	logsBloom := [256]byte{'j', 'u', 'n', 'k'}
	addr, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000095e7baea6a6c7c4c2dfeb977efac326af552d87")
	assert.NoError(f, err)
	innerData := &types.DynamicFeeTx{
		ChainID:   big.NewInt(math.MaxInt),
		Nonce:     math.MaxUint64,
		GasTipCap: big.NewInt(math.MaxInt),
		GasFeeCap: big.NewInt(math.MaxInt),
		Gas:       math.MaxUint64,
		To:        &addr,
		Value:     big.NewInt(math.MaxInt),
		Data:      []byte{'r', 'a', 'n', 'd', 'o', 'm'},
	}
	tx := types.NewTx(innerData)
	execBlock := &pb.ExecutionBlock{
		Header: types.Header{
			ParentHash:  common.Hash([32]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}),
			Root:        common.Hash([32]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}),
			ReceiptHash: common.Hash([32]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}),
			Bloom:       types.Bloom(logsBloom),
			Number:      big.NewInt(math.MaxInt),
			GasLimit:    math.MaxUint64,
			GasUsed:     math.MaxUint64,
			Time:        100,
			Extra:       nil,
			BaseFee:     big.NewInt(math.MaxInt),
		},
		Hash:         common.Hash([32]byte{0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01, 0xFF, 0x01}),
		Transactions: []*types.Transaction{tx, tx},
	}
	output, err := json.Marshal(execBlock)
	assert.NoError(f, err)

	f.Add(output)

	f.Fuzz(func(t *testing.T, jsonBlob []byte) {
		gqrlResp := make(map[string]any)
		qrysmResp := &pb.ExecutionBlock{}
		gqrlErr := json.Unmarshal(jsonBlob, &gqrlResp)
		qrysmErr := json.Unmarshal(jsonBlob, qrysmResp)
		// Nothing to marshal if we have an error.
		if gqrlErr != nil || qrysmErr != nil {
			return
		}
		// Exit early if fuzzer is inserting bogus hashes in.
		if isBogusTransactionHash(qrysmResp, gqrlResp) {
			return
		}
		// Exit early if fuzzer provides bogus fields.
		valid, err := jsonFieldsAreValid(qrysmResp, gqrlResp)
		assert.NoError(t, err)
		if !valid {
			return
		}
		assert.NoError(t, validateBlockConsistency(qrysmResp, gqrlResp))

		gqrlBlob, gqrlErr := json.Marshal(gqrlResp)
		qrysmBlob, qrysmErr := json.Marshal(qrysmResp)
		assert.Equal(t, gqrlErr != nil, qrysmErr != nil, "gqrl and qrysm unmarshaller return inconsistent errors")
		newGqrlResp := make(map[string]any)
		newGqrlErr := json.Unmarshal(qrysmBlob, &newGqrlResp)
		assert.NoError(t, newGqrlErr)
		newGqrlResp2 := make(map[string]any)
		newGqrlErr = json.Unmarshal(gqrlBlob, &newGqrlResp2)
		assert.NoError(t, newGqrlErr)

		assert.DeepEqual(t, newGqrlResp, newGqrlResp2)
		compareHeaders(t, jsonBlob)
	})
}

func isBogusTransactionHash(blk *pb.ExecutionBlock, jsonMap map[string]any) bool {
	if blk.Transactions == nil {
		return false
	}

	for i, tx := range blk.Transactions {
		jsonTx, ok := jsonMap["transactions"].([]any)[i].(map[string]any)
		if !ok {
			return true
		}
		// Fuzzer removed hash field.
		if _, ok := jsonTx["hash"]; !ok {
			return true
		}
		if tx.Hash().String() != jsonTx["hash"].(string) {
			return true
		}
	}
	return false
}

func compareHeaders(t *testing.T, jsonBlob []byte) {
	gqrlResp := &types.Header{}
	qrysmResp := &pb.ExecutionBlock{}
	gqrlErr := json.Unmarshal(jsonBlob, gqrlResp)
	qrysmErr := json.Unmarshal(jsonBlob, qrysmResp)
	assert.Equal(t, gqrlErr != nil, qrysmErr != nil, fmt.Sprintf("gqrl and qrysm unmarshaller return inconsistent errors. %v and %v", gqrlErr, qrysmErr))
	// Nothing to marshal if we have an error.
	if gqrlErr != nil {
		return
	}

	gqrlBlob, gqrlErr := json.Marshal(gqrlResp)
	qrysmBlob, qrysmErr := json.Marshal(qrysmResp.Header)
	assert.Equal(t, gqrlErr != nil, qrysmErr != nil, "gqrl and qrysm unmarshaller return inconsistent errors")
	newGqrlResp := &types.Header{}
	newGqrlErr := json.Unmarshal(qrysmBlob, newGqrlResp)
	assert.NoError(t, newGqrlErr)
	newGqrlResp2 := &types.Header{}
	newGqrlErr = json.Unmarshal(gqrlBlob, newGqrlResp2)
	assert.NoError(t, newGqrlErr)

	assert.DeepEqual(t, newGqrlResp, newGqrlResp2)
}

func validateBlockConsistency(execBlock *pb.ExecutionBlock, jsonMap map[string]any) error {
	blockVal := reflect.ValueOf(execBlock).Elem()
	bType := reflect.TypeFor[pb.ExecutionBlock]()

	fieldnum := bType.NumField()

	for i := range fieldnum {
		field := bType.Field(i)
		fName := field.Tag.Get("json")
		if field.Name == "Header" {
			continue
		}
		if fName == "" {
			return errors.Errorf("Field %s had no json tag", field.Name)
		}
		fVal, ok := jsonMap[fName]
		if !ok {
			return errors.Errorf("%s doesn't exist in json map for field %s", fName, field.Name)
		}
		jsonVal := fVal
		bVal := blockVal.Field(i).Interface()
		if field.Name == "Hash" {
			jsonVal = common.HexToHash(jsonVal.(string))
		}
		if field.Name == "Transactions" {
			continue
		}
		if !reflect.DeepEqual(jsonVal, bVal) {
			return errors.Errorf("fields don't match, %v and %v are not equal for field %s", jsonVal, bVal, field.Name)
		}
	}
	return nil
}

func jsonFieldsAreValid(execBlock *pb.ExecutionBlock, jsonMap map[string]any) (bool, error) {
	bType := reflect.TypeFor[pb.ExecutionBlock]()

	fieldnum := bType.NumField()

	for i := range fieldnum {
		field := bType.Field(i)
		fName := field.Tag.Get("json")
		if field.Name == "Header" {
			continue
		}
		if fName == "" {
			return false, errors.Errorf("Field %s had no json tag", field.Name)
		}
		_, ok := jsonMap[fName]
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
