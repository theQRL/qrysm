package blocks_test

import (
	"testing"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	consensus_types "github.com/theQRL/qrysm/consensus-types"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestWrapExecutionPayloadZond(t *testing.T) {
	data := &enginev1.ExecutionPayloadZond{
		ParentHash:    []byte("parenthash"),
		FeeRecipient:  []byte("feerecipient"),
		StateRoot:     []byte("stateroot"),
		ReceiptsRoot:  []byte("receiptsroot"),
		LogsBloom:     []byte("logsbloom"),
		PrevRandao:    []byte("prevrandao"),
		BlockNumber:   11,
		GasLimit:      22,
		GasUsed:       33,
		Timestamp:     44,
		ExtraData:     []byte("extradata"),
		BaseFeePerGas: []byte("basefeepergas"),
		BlockHash:     []byte("blockhash"),
		Transactions:  [][]byte{[]byte("transaction")},
		Withdrawals: []*enginev1.Withdrawal{{
			Index:          55,
			ValidatorIndex: 66,
			Address:        []byte("executionaddress"),
			Amount:         77,
		}},
	}
	payload, err := blocks.WrappedExecutionPayloadZond(data, 10)
	require.NoError(t, err)
	v, err := payload.ValueInShor()
	require.NoError(t, err)
	assert.Equal(t, uint64(10), v)

	assert.DeepEqual(t, data, payload.Proto())
}

func TestWrapExecutionPayloadHeaderZond(t *testing.T) {
	data := &enginev1.ExecutionPayloadHeaderZond{
		ParentHash:       []byte("parenthash"),
		FeeRecipient:     []byte("feerecipient"),
		StateRoot:        []byte("stateroot"),
		ReceiptsRoot:     []byte("receiptsroot"),
		LogsBloom:        []byte("logsbloom"),
		PrevRandao:       []byte("prevrandao"),
		BlockNumber:      11,
		GasLimit:         22,
		GasUsed:          33,
		Timestamp:        44,
		ExtraData:        []byte("extradata"),
		BaseFeePerGas:    []byte("basefeepergas"),
		BlockHash:        []byte("blockhash"),
		TransactionsRoot: []byte("transactionsroot"),
		WithdrawalsRoot:  []byte("withdrawalsroot"),
	}
	payload, err := blocks.WrappedExecutionPayloadHeaderZond(data, 10)
	require.NoError(t, err)

	v, err := payload.ValueInShor()
	require.NoError(t, err)
	assert.Equal(t, uint64(10), v)

	assert.DeepEqual(t, data, payload.Proto())

	txRoot, err := payload.TransactionsRoot()
	require.NoError(t, err)
	require.DeepEqual(t, txRoot, data.TransactionsRoot)

	wrRoot, err := payload.WithdrawalsRoot()
	require.NoError(t, err)
	require.DeepEqual(t, wrRoot, data.WithdrawalsRoot)
}

func TestWrapExecutionPayloadZond_IsNil(t *testing.T) {
	_, err := blocks.WrappedExecutionPayloadZond(nil, 0)
	require.Equal(t, consensus_types.ErrNilObjectWrapped, err)

	data := &enginev1.ExecutionPayloadZond{GasUsed: 54}
	payload, err := blocks.WrappedExecutionPayloadZond(data, 0)
	require.NoError(t, err)

	assert.Equal(t, false, payload.IsNil())
}

func TestWrapExecutionPayloadHeaderZond_IsNil(t *testing.T) {
	_, err := blocks.WrappedExecutionPayloadHeaderZond(nil, 0)
	require.Equal(t, consensus_types.ErrNilObjectWrapped, err)

	data := &enginev1.ExecutionPayloadHeaderZond{GasUsed: 54}
	payload, err := blocks.WrappedExecutionPayloadHeaderZond(data, 0)
	require.NoError(t, err)

	assert.Equal(t, false, payload.IsNil())
}

func TestWrapExecutionPayloadZond_SSZ(t *testing.T) {
	payload := createWrappedPayloadZond(t)
	rt, err := payload.HashTreeRoot()
	assert.NoError(t, err)
	assert.NotEmpty(t, rt)

	var b []byte
	b, err = payload.MarshalSSZTo(b)
	assert.NoError(t, err)
	assert.NotEqual(t, 0, len(b))
	encoded, err := payload.MarshalSSZ()
	require.NoError(t, err)
	assert.NotEqual(t, 0, payload.SizeSSZ())
	assert.NoError(t, payload.UnmarshalSSZ(encoded))
}

func TestWrapExecutionPayloadHeaderZond_SSZ(t *testing.T) {
	payload := createWrappedPayloadHeaderZond(t)
	rt, err := payload.HashTreeRoot()
	assert.NoError(t, err)
	assert.NotEmpty(t, rt)

	var b []byte
	b, err = payload.MarshalSSZTo(b)
	assert.NoError(t, err)
	assert.NotEqual(t, 0, len(b))
	encoded, err := payload.MarshalSSZ()
	require.NoError(t, err)
	assert.NotEqual(t, 0, payload.SizeSSZ())
	assert.NoError(t, payload.UnmarshalSSZ(encoded))
}

func Test_executionPayloadZond_Pb(t *testing.T) {
	payload := createWrappedPayloadZond(t)
	pb, err := payload.PbZond()
	require.NoError(t, err)
	assert.DeepEqual(t, payload.Proto(), pb)
}

func Test_executionPayloadHeaderZond_Pb(t *testing.T) {
	payload := createWrappedPayloadHeaderZond(t)

	_, err := payload.PbZond()
	require.ErrorIs(t, err, consensus_types.ErrUnsupportedField)
}

func createWrappedPayloadZond(t testing.TB) interfaces.ExecutionData {
	payload, err := blocks.WrappedExecutionPayloadZond(&enginev1.ExecutionPayloadZond{
		ParentHash:    make([]byte, fieldparams.RootLength),
		FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
		StateRoot:     make([]byte, fieldparams.RootLength),
		ReceiptsRoot:  make([]byte, fieldparams.RootLength),
		LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
		PrevRandao:    make([]byte, fieldparams.RootLength),
		BlockNumber:   0,
		GasLimit:      0,
		GasUsed:       0,
		Timestamp:     0,
		ExtraData:     make([]byte, 0),
		BaseFeePerGas: make([]byte, fieldparams.RootLength),
		BlockHash:     make([]byte, fieldparams.RootLength),
		Transactions:  make([][]byte, 0),
		Withdrawals:   make([]*enginev1.Withdrawal, 0),
	}, 0)
	require.NoError(t, err)
	return payload
}

func createWrappedPayloadHeaderZond(t testing.TB) interfaces.ExecutionData {
	payload, err := blocks.WrappedExecutionPayloadHeaderZond(&enginev1.ExecutionPayloadHeaderZond{
		ParentHash:       make([]byte, fieldparams.RootLength),
		FeeRecipient:     make([]byte, fieldparams.FeeRecipientLength),
		StateRoot:        make([]byte, fieldparams.RootLength),
		ReceiptsRoot:     make([]byte, fieldparams.RootLength),
		LogsBloom:        make([]byte, fieldparams.LogsBloomLength),
		PrevRandao:       make([]byte, fieldparams.RootLength),
		BlockNumber:      0,
		GasLimit:         0,
		GasUsed:          0,
		Timestamp:        0,
		ExtraData:        make([]byte, 0),
		BaseFeePerGas:    make([]byte, fieldparams.RootLength),
		BlockHash:        make([]byte, fieldparams.RootLength),
		TransactionsRoot: make([]byte, fieldparams.RootLength),
		WithdrawalsRoot:  make([]byte, fieldparams.RootLength),
	}, 0)
	require.NoError(t, err)
	return payload
}
