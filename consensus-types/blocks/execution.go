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

// executionPayloadZond is a convenience wrapper around a beacon block body's execution payload data structure
// This wrapper allows us to conform to a common interface so that beacon
// blocks for future forks can also be applied across Qrysm without issues.
type executionPayloadZond struct {
	p     *enginev1.ExecutionPayloadZond
	value uint64
}

// WrappedExecutionPayloadZond is a constructor which wraps a protobuf execution payload into an interface.
func WrappedExecutionPayloadZond(p *enginev1.ExecutionPayloadZond, value math.Shor) (interfaces.ExecutionData, error) {
	w := executionPayloadZond{p: p, value: uint64(value)}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// IsNil checks if the underlying data is nil.
func (e executionPayloadZond) IsNil() bool {
	return e.p == nil
}

// IsBlinded returns true if the underlying data is blinded.
func (executionPayloadZond) IsBlinded() bool {
	return false
}

// MarshalSSZ --
func (e executionPayloadZond) MarshalSSZ() ([]byte, error) {
	return e.p.MarshalSSZ()
}

// MarshalSSZTo --
func (e executionPayloadZond) MarshalSSZTo(dst []byte) ([]byte, error) {
	return e.p.MarshalSSZTo(dst)
}

// SizeSSZ --
func (e executionPayloadZond) SizeSSZ() int {
	return e.p.SizeSSZ()
}

// UnmarshalSSZ --
func (e executionPayloadZond) UnmarshalSSZ(buf []byte) error {
	return e.p.UnmarshalSSZ(buf)
}

// HashTreeRoot --
func (e executionPayloadZond) HashTreeRoot() ([32]byte, error) {
	return e.p.HashTreeRoot()
}

// HashTreeRootWith --
func (e executionPayloadZond) HashTreeRootWith(hh *fastssz.Hasher) error {
	return e.p.HashTreeRootWith(hh)
}

// Proto --
func (e executionPayloadZond) Proto() proto.Message {
	return e.p
}

// ParentHash --
func (e executionPayloadZond) ParentHash() []byte {
	return e.p.ParentHash
}

// FeeRecipient --
func (e executionPayloadZond) FeeRecipient() []byte {
	return e.p.FeeRecipient
}

// StateRoot --
func (e executionPayloadZond) StateRoot() []byte {
	return e.p.StateRoot
}

// ReceiptsRoot --
func (e executionPayloadZond) ReceiptsRoot() []byte {
	return e.p.ReceiptsRoot
}

// LogsBloom --
func (e executionPayloadZond) LogsBloom() []byte {
	return e.p.LogsBloom
}

// PrevRandao --
func (e executionPayloadZond) PrevRandao() []byte {
	return e.p.PrevRandao
}

// BlockNumber --
func (e executionPayloadZond) BlockNumber() uint64 {
	return e.p.BlockNumber
}

// GasLimit --
func (e executionPayloadZond) GasLimit() uint64 {
	return e.p.GasLimit
}

// GasUsed --
func (e executionPayloadZond) GasUsed() uint64 {
	return e.p.GasUsed
}

// Timestamp --
func (e executionPayloadZond) Timestamp() uint64 {
	return e.p.Timestamp
}

// ExtraData --
func (e executionPayloadZond) ExtraData() []byte {
	return e.p.ExtraData
}

// BaseFeePerGas --
func (e executionPayloadZond) BaseFeePerGas() []byte {
	return e.p.BaseFeePerGas
}

// BlockHash --
func (e executionPayloadZond) BlockHash() []byte {
	return e.p.BlockHash
}

// Transactions --
func (e executionPayloadZond) Transactions() ([][]byte, error) {
	return e.p.Transactions, nil
}

// TransactionsRoot --
func (executionPayloadZond) TransactionsRoot() ([]byte, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// Withdrawals --
func (e executionPayloadZond) Withdrawals() ([]*enginev1.Withdrawal, error) {
	return e.p.Withdrawals, nil
}

// WithdrawalsRoot --
func (executionPayloadZond) WithdrawalsRoot() ([]byte, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// PbV2 --
func (e executionPayloadZond) PbZond() (*enginev1.ExecutionPayloadZond, error) {
	return e.p, nil
}

// ValueInShor --
func (e executionPayloadZond) ValueInShor() (uint64, error) {
	return e.value, nil
}

// executionPayloadHeaderZond is a convenience wrapper around a blinded beacon block body's execution header data structure
// This wrapper allows us to conform to a common interface so that beacon
// blocks for future forks can also be applied across Qrysm without issues.
type executionPayloadHeaderZond struct {
	p     *enginev1.ExecutionPayloadHeaderZond
	value uint64
}

// WrappedExecutionPayloadHeaderZond is a constructor which wraps a protobuf execution header into an interface.
func WrappedExecutionPayloadHeaderZond(p *enginev1.ExecutionPayloadHeaderZond, value math.Shor) (interfaces.ExecutionData, error) {
	w := executionPayloadHeaderZond{p: p, value: uint64(value)}
	if w.IsNil() {
		return nil, consensus_types.ErrNilObjectWrapped
	}
	return w, nil
}

// IsNil checks if the underlying data is nil.
func (e executionPayloadHeaderZond) IsNil() bool {
	return e.p == nil
}

// IsBlinded returns true if the underlying data is blinded.
func (executionPayloadHeaderZond) IsBlinded() bool {
	return true
}

// MarshalSSZ --
func (e executionPayloadHeaderZond) MarshalSSZ() ([]byte, error) {
	return e.p.MarshalSSZ()
}

// MarshalSSZTo --
func (e executionPayloadHeaderZond) MarshalSSZTo(dst []byte) ([]byte, error) {
	return e.p.MarshalSSZTo(dst)
}

// SizeSSZ --
func (e executionPayloadHeaderZond) SizeSSZ() int {
	return e.p.SizeSSZ()
}

// UnmarshalSSZ --
func (e executionPayloadHeaderZond) UnmarshalSSZ(buf []byte) error {
	return e.p.UnmarshalSSZ(buf)
}

// HashTreeRoot --
func (e executionPayloadHeaderZond) HashTreeRoot() ([32]byte, error) {
	return e.p.HashTreeRoot()
}

// HashTreeRootWith --
func (e executionPayloadHeaderZond) HashTreeRootWith(hh *fastssz.Hasher) error {
	return e.p.HashTreeRootWith(hh)
}

// Proto --
func (e executionPayloadHeaderZond) Proto() proto.Message {
	return e.p
}

// ParentHash --
func (e executionPayloadHeaderZond) ParentHash() []byte {
	return e.p.ParentHash
}

// FeeRecipient --
func (e executionPayloadHeaderZond) FeeRecipient() []byte {
	return e.p.FeeRecipient
}

// StateRoot --
func (e executionPayloadHeaderZond) StateRoot() []byte {
	return e.p.StateRoot
}

// ReceiptsRoot --
func (e executionPayloadHeaderZond) ReceiptsRoot() []byte {
	return e.p.ReceiptsRoot
}

// LogsBloom --
func (e executionPayloadHeaderZond) LogsBloom() []byte {
	return e.p.LogsBloom
}

// PrevRandao --
func (e executionPayloadHeaderZond) PrevRandao() []byte {
	return e.p.PrevRandao
}

// BlockNumber --
func (e executionPayloadHeaderZond) BlockNumber() uint64 {
	return e.p.BlockNumber
}

// GasLimit --
func (e executionPayloadHeaderZond) GasLimit() uint64 {
	return e.p.GasLimit
}

// GasUsed --
func (e executionPayloadHeaderZond) GasUsed() uint64 {
	return e.p.GasUsed
}

// Timestamp --
func (e executionPayloadHeaderZond) Timestamp() uint64 {
	return e.p.Timestamp
}

// ExtraData --
func (e executionPayloadHeaderZond) ExtraData() []byte {
	return e.p.ExtraData
}

// BaseFeePerGas --
func (e executionPayloadHeaderZond) BaseFeePerGas() []byte {
	return e.p.BaseFeePerGas
}

// BlockHash --
func (e executionPayloadHeaderZond) BlockHash() []byte {
	return e.p.BlockHash
}

// Transactions --
func (executionPayloadHeaderZond) Transactions() ([][]byte, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// TransactionsRoot --
func (e executionPayloadHeaderZond) TransactionsRoot() ([]byte, error) {
	return e.p.TransactionsRoot, nil
}

// Withdrawals --
func (executionPayloadHeaderZond) Withdrawals() ([]*enginev1.Withdrawal, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// WithdrawalsRoot --
func (e executionPayloadHeaderZond) WithdrawalsRoot() ([]byte, error) {
	return e.p.WithdrawalsRoot, nil
}

// PbV2 --
func (executionPayloadHeaderZond) PbZond() (*enginev1.ExecutionPayloadZond, error) {
	return nil, consensus_types.ErrUnsupportedField
}

// ValueInShor --
func (e executionPayloadHeaderZond) ValueInShor() (uint64, error) {
	return e.value, nil
}

// PayloadToHeaderZond converts `payload` into execution payload header format.
func PayloadToHeaderZond(payload interfaces.ExecutionData) (*enginev1.ExecutionPayloadHeaderZond, error) {
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

	return &enginev1.ExecutionPayloadHeaderZond{
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

// PayloadValueToShor returns a Shor value given the payload's value
func PayloadValueToShor(value []byte) math.Shor {
	// We have to convert big endian to little endian because the value is coming from the execution layer.
	v := big.NewInt(0).SetBytes(bytesutil.ReverseByteOrder(value))
	return math.PlanckToShor(v)
}
