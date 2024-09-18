package blocks

import (
	"bytes"
	"errors"
	"math/big"

	fastssz "github.com/prysmaticlabs/fastssz"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	consensus_types "github.com/theQRL/qrysm/consensus-types"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/encoding/ssz"
	"github.com/theQRL/qrysm/math"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	"google.golang.org/protobuf/proto"
)

// executionPayloadCapella is a convenience wrapper around a beacon block body's execution payload data structure
// This wrapper allows us to conform to a common interface so that beacon
// blocks for future forks can also be applied across Qrysm without issues.
type executionPayloadCapella struct {
	p     *enginev1.ExecutionPayloadCapella
	value uint64
}

// WrappedExecutionPayloadCapella is a constructor which wraps a protobuf execution payload into an interface.
func WrappedExecutionPayloadCapella(p *enginev1.ExecutionPayloadCapella, value math.Gwei) (interfaces.ExecutionData, error) {
	w := executionPayloadCapella{p: p, value: uint64(value)}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// IsNil checks if the underlying data is nil.
func (e executionPayloadCapella) IsNil() bool {
	return e.p == nil
}

// IsBlinded returns true if the underlying data is blinded.
func (executionPayloadCapella) IsBlinded() bool {
	return false
}

// MarshalSSZ --
func (e executionPayloadCapella) MarshalSSZ() ([]byte, error) {
	return e.p.MarshalSSZ()
}

// MarshalSSZTo --
func (e executionPayloadCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	return e.p.MarshalSSZTo(dst)
}

// SizeSSZ --
func (e executionPayloadCapella) SizeSSZ() int {
	return e.p.SizeSSZ()
}

// UnmarshalSSZ --
func (e executionPayloadCapella) UnmarshalSSZ(buf []byte) error {
	return e.p.UnmarshalSSZ(buf)
}

// HashTreeRoot --
func (e executionPayloadCapella) HashTreeRoot() ([32]byte, error) {
	return e.p.HashTreeRoot()
}

// HashTreeRootWith --
func (e executionPayloadCapella) HashTreeRootWith(hh *fastssz.Hasher) error {
	return e.p.HashTreeRootWith(hh)
}

// Proto --
func (e executionPayloadCapella) Proto() proto.Message {
	return e.p
}

// ParentHash --
func (e executionPayloadCapella) ParentHash() []byte {
	return e.p.ParentHash
}

// FeeRecipient --
func (e executionPayloadCapella) FeeRecipient() []byte {
	return e.p.FeeRecipient
}

// StateRoot --
func (e executionPayloadCapella) StateRoot() []byte {
	return e.p.StateRoot
}

// ReceiptsRoot --
func (e executionPayloadCapella) ReceiptsRoot() []byte {
	return e.p.ReceiptsRoot
}

// LogsBloom --
func (e executionPayloadCapella) LogsBloom() []byte {
	return e.p.LogsBloom
}

// PrevRandao --
func (e executionPayloadCapella) PrevRandao() []byte {
	return e.p.PrevRandao
}

// BlockNumber --
func (e executionPayloadCapella) BlockNumber() uint64 {
	return e.p.BlockNumber
}

// GasLimit --
func (e executionPayloadCapella) GasLimit() uint64 {
	return e.p.GasLimit
}

// GasUsed --
func (e executionPayloadCapella) GasUsed() uint64 {
	return e.p.GasUsed
}

// Timestamp --
func (e executionPayloadCapella) Timestamp() uint64 {
	return e.p.Timestamp
}

// ExtraData --
func (e executionPayloadCapella) ExtraData() []byte {
	return e.p.ExtraData
}

// BaseFeePerGas --
func (e executionPayloadCapella) BaseFeePerGas() []byte {
	return e.p.BaseFeePerGas
}

// BlockHash --
func (e executionPayloadCapella) BlockHash() []byte {
	return e.p.BlockHash
}

// Transactions --
func (e executionPayloadCapella) Transactions() ([][]byte, error) {
	return e.p.Transactions, nil
}

// TransactionsRoot --
func (executionPayloadCapella) TransactionsRoot() ([]byte, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// Withdrawals --
func (e executionPayloadCapella) Withdrawals() ([]*enginev1.Withdrawal, error) {
	return e.p.Withdrawals, nil
}

// WithdrawalsRoot --
func (executionPayloadCapella) WithdrawalsRoot() ([]byte, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// PbV2 --
func (e executionPayloadCapella) PbCapella() (*enginev1.ExecutionPayloadCapella, error) {
	return e.p, nil
}

// ValueInGwei --
func (e executionPayloadCapella) ValueInGwei() (uint64, error) {
	return e.value, nil
}

// executionPayloadHeaderCapella is a convenience wrapper around a blinded beacon block body's execution header data structure
// This wrapper allows us to conform to a common interface so that beacon
// blocks for future forks can also be applied across Qrysm without issues.
type executionPayloadHeaderCapella struct {
	p     *enginev1.ExecutionPayloadHeaderCapella
	value uint64
}

// WrappedExecutionPayloadHeaderCapella is a constructor which wraps a protobuf execution header into an interface.
func WrappedExecutionPayloadHeaderCapella(p *enginev1.ExecutionPayloadHeaderCapella, value math.Gwei) (interfaces.ExecutionData, error) {
	w := executionPayloadHeaderCapella{p: p, value: uint64(value)}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// IsNil checks if the underlying data is nil.
func (e executionPayloadHeaderCapella) IsNil() bool {
	return e.p == nil
}

// IsBlinded returns true if the underlying data is blinded.
func (executionPayloadHeaderCapella) IsBlinded() bool {
	return true
}

// MarshalSSZ --
func (e executionPayloadHeaderCapella) MarshalSSZ() ([]byte, error) {
	return e.p.MarshalSSZ()
}

// MarshalSSZTo --
func (e executionPayloadHeaderCapella) MarshalSSZTo(dst []byte) ([]byte, error) {
	return e.p.MarshalSSZTo(dst)
}

// SizeSSZ --
func (e executionPayloadHeaderCapella) SizeSSZ() int {
	return e.p.SizeSSZ()
}

// UnmarshalSSZ --
func (e executionPayloadHeaderCapella) UnmarshalSSZ(buf []byte) error {
	return e.p.UnmarshalSSZ(buf)
}

// HashTreeRoot --
func (e executionPayloadHeaderCapella) HashTreeRoot() ([32]byte, error) {
	return e.p.HashTreeRoot()
}

// HashTreeRootWith --
func (e executionPayloadHeaderCapella) HashTreeRootWith(hh *fastssz.Hasher) error {
	return e.p.HashTreeRootWith(hh)
}

// Proto --
func (e executionPayloadHeaderCapella) Proto() proto.Message {
	return e.p
}

// ParentHash --
func (e executionPayloadHeaderCapella) ParentHash() []byte {
	return e.p.ParentHash
}

// FeeRecipient --
func (e executionPayloadHeaderCapella) FeeRecipient() []byte {
	return e.p.FeeRecipient
}

// StateRoot --
func (e executionPayloadHeaderCapella) StateRoot() []byte {
	return e.p.StateRoot
}

// ReceiptsRoot --
func (e executionPayloadHeaderCapella) ReceiptsRoot() []byte {
	return e.p.ReceiptsRoot
}

// LogsBloom --
func (e executionPayloadHeaderCapella) LogsBloom() []byte {
	return e.p.LogsBloom
}

// PrevRandao --
func (e executionPayloadHeaderCapella) PrevRandao() []byte {
	return e.p.PrevRandao
}

// BlockNumber --
func (e executionPayloadHeaderCapella) BlockNumber() uint64 {
	return e.p.BlockNumber
}

// GasLimit --
func (e executionPayloadHeaderCapella) GasLimit() uint64 {
	return e.p.GasLimit
}

// GasUsed --
func (e executionPayloadHeaderCapella) GasUsed() uint64 {
	return e.p.GasUsed
}

// Timestamp --
func (e executionPayloadHeaderCapella) Timestamp() uint64 {
	return e.p.Timestamp
}

// ExtraData --
func (e executionPayloadHeaderCapella) ExtraData() []byte {
	return e.p.ExtraData
}

// BaseFeePerGas --
func (e executionPayloadHeaderCapella) BaseFeePerGas() []byte {
	return e.p.BaseFeePerGas
}

// BlockHash --
func (e executionPayloadHeaderCapella) BlockHash() []byte {
	return e.p.BlockHash
}

// Transactions --
func (executionPayloadHeaderCapella) Transactions() ([][]byte, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// TransactionsRoot --
func (e executionPayloadHeaderCapella) TransactionsRoot() ([]byte, error) {
	return e.p.TransactionsRoot, nil
}

// Withdrawals --
func (executionPayloadHeaderCapella) Withdrawals() ([]*enginev1.Withdrawal, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// WithdrawalsRoot --
func (e executionPayloadHeaderCapella) WithdrawalsRoot() ([]byte, error) {
	return e.p.WithdrawalsRoot, nil
}

// PbV2 --
func (executionPayloadHeaderCapella) PbCapella() (*enginev1.ExecutionPayloadCapella, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// ValueInGwei --
func (e executionPayloadHeaderCapella) ValueInGwei() (uint64, error) {
	return e.value, nil
}

// PayloadToHeaderCapella converts `payload` into execution payload header format.
func PayloadToHeaderCapella(payload interfaces.ExecutionData) (*enginev1.ExecutionPayloadHeaderCapella, error) {
	txs, err := payload.Transactions()
	if err != nil {
		return nil, err
	}
	txRoot, err := ssz.TransactionsRoot(txs)
	if err != nil {
		return nil, err
	}
	withdrawals, err := payload.Withdrawals()
	if err != nil {
		return nil, err
	}
	withdrawalsRoot, err := ssz.WithdrawalSliceRoot(withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, err
	}

	return &enginev1.ExecutionPayloadHeaderCapella{
		ParentHash:       bytesutil.SafeCopyBytes(payload.ParentHash()),
		FeeRecipient:     bytesutil.SafeCopyBytes(payload.FeeRecipient()),
		StateRoot:        bytesutil.SafeCopyBytes(payload.StateRoot()),
		ReceiptsRoot:     bytesutil.SafeCopyBytes(payload.ReceiptsRoot()),
		LogsBloom:        bytesutil.SafeCopyBytes(payload.LogsBloom()),
		PrevRandao:       bytesutil.SafeCopyBytes(payload.PrevRandao()),
		BlockNumber:      payload.BlockNumber(),
		GasLimit:         payload.GasLimit(),
		GasUsed:          payload.GasUsed(),
		Timestamp:        payload.Timestamp(),
		ExtraData:        bytesutil.SafeCopyBytes(payload.ExtraData()),
		BaseFeePerGas:    bytesutil.SafeCopyBytes(payload.BaseFeePerGas()),
		BlockHash:        bytesutil.SafeCopyBytes(payload.BlockHash()),
		TransactionsRoot: txRoot[:],
		WithdrawalsRoot:  withdrawalsRoot[:],
	}, nil
}

// IsEmptyExecutionData checks if an execution data is empty underneath. If a single field has
// a non-zero value, this function will return false.
func IsEmptyExecutionData(data interfaces.ExecutionData) (bool, error) {
	if !bytes.Equal(data.ParentHash(), make([]byte, fieldparams.RootLength)) {
		return false, nil
	}
	if !bytes.Equal(data.FeeRecipient(), make([]byte, fieldparams.FeeRecipientLength)) {
		return false, nil
	}
	if !bytes.Equal(data.StateRoot(), make([]byte, fieldparams.RootLength)) {
		return false, nil
	}
	if !bytes.Equal(data.ReceiptsRoot(), make([]byte, fieldparams.RootLength)) {
		return false, nil
	}
	if !bytes.Equal(data.LogsBloom(), make([]byte, fieldparams.LogsBloomLength)) {
		return false, nil
	}
	if !bytes.Equal(data.PrevRandao(), make([]byte, fieldparams.RootLength)) {
		return false, nil
	}
	if !bytes.Equal(data.BaseFeePerGas(), make([]byte, fieldparams.RootLength)) {
		return false, nil
	}
	if !bytes.Equal(data.BlockHash(), make([]byte, fieldparams.RootLength)) {
		return false, nil
	}

	txs, err := data.Transactions()
	switch {
	case errors.Is(err, consensus_types.ErrUnsupportedField):
	case err != nil:
		return false, err
	default:
		if len(txs) != 0 {
			return false, nil
		}
	}

	if len(data.ExtraData()) != 0 {
		return false, nil
	}
	if data.BlockNumber() != 0 {
		return false, nil
	}
	if data.GasLimit() != 0 {
		return false, nil
	}
	if data.GasUsed() != 0 {
		return false, nil
	}
	if data.Timestamp() != 0 {
		return false, nil
	}
	return true, nil
}

// PayloadValueToGwei returns a Gwei value given the payload's value
func PayloadValueToGwei(value []byte) math.Gwei {
	// We have to convert big endian to little endian because the value is coming from the execution layer.
	v := big.NewInt(0).SetBytes(bytesutil.ReverseByteOrder(value))
	return math.WeiToGwei(v)
}
