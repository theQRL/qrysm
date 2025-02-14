package interfaces

import (
	ssz "github.com/prysmaticlabs/fastssz"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/validator-client"
	"google.golang.org/protobuf/proto"
)

// ReadOnlySignedBeaconBlock is an interface describing the method set of
// a signed beacon block.
type ReadOnlySignedBeaconBlock interface {
	Block() ReadOnlyBeaconBlock
	Signature() [field_params.DilithiumSignatureLength]byte
	IsNil() bool
	Copy() (ReadOnlySignedBeaconBlock, error)
	Proto() (proto.Message, error)
	PbGenericBlock() (*zondpb.GenericSignedBeaconBlock, error)
	ToBlinded() (ReadOnlySignedBeaconBlock, error)
	PbCapellaBlock() (*zondpb.SignedBeaconBlockCapella, error)
	PbBlindedCapellaBlock() (*zondpb.SignedBlindedBeaconBlockCapella, error)
	ssz.Marshaler
	ssz.Unmarshaler
	Version() int
	IsBlinded() bool
	ValueInGwei() uint64
	Header() (*zondpb.SignedBeaconBlockHeader, error)
}

// ReadOnlyBeaconBlock describes an interface which states the methods
// employed by an object that is a beacon block.
type ReadOnlyBeaconBlock interface {
	Slot() primitives.Slot
	ProposerIndex() primitives.ValidatorIndex
	ParentRoot() [field_params.RootLength]byte
	StateRoot() [field_params.RootLength]byte
	Body() ReadOnlyBeaconBlockBody
	IsNil() bool
	IsBlinded() bool
	HashTreeRoot() ([field_params.RootLength]byte, error)
	Proto() (proto.Message, error)
	ssz.Marshaler
	ssz.Unmarshaler
	ssz.HashRoot
	Version() int
	AsSignRequestObject() (validatorpb.SignRequestObject, error)
	Copy() (ReadOnlyBeaconBlock, error)
}

// ReadOnlyBeaconBlockBody describes the method set employed by an object
// that is a beacon block body.
type ReadOnlyBeaconBlockBody interface {
	RandaoReveal() [field_params.DilithiumSignatureLength]byte
	Eth1Data() *zondpb.Eth1Data
	Graffiti() [field_params.RootLength]byte
	ProposerSlashings() []*zondpb.ProposerSlashing
	AttesterSlashings() []*zondpb.AttesterSlashing
	Attestations() []*zondpb.Attestation
	Deposits() []*zondpb.Deposit
	VoluntaryExits() []*zondpb.SignedVoluntaryExit
	SyncAggregate() (*zondpb.SyncAggregate, error)
	IsNil() bool
	HashTreeRoot() ([field_params.RootLength]byte, error)
	Proto() (proto.Message, error)
	Execution() (ExecutionData, error)
	DilithiumToExecutionChanges() ([]*zondpb.SignedDilithiumToExecutionChange, error)
}

type SignedBeaconBlock interface {
	ReadOnlySignedBeaconBlock
	SetExecution(ExecutionData) error
	SetDilithiumToExecutionChanges([]*zondpb.SignedDilithiumToExecutionChange) error
	SetSyncAggregate(*zondpb.SyncAggregate) error
	SetVoluntaryExits([]*zondpb.SignedVoluntaryExit)
	SetDeposits([]*zondpb.Deposit)
	SetAttestations([]*zondpb.Attestation)
	SetAttesterSlashings([]*zondpb.AttesterSlashing)
	SetProposerSlashings([]*zondpb.ProposerSlashing)
	SetGraffiti([]byte)
	SetEth1Data(*zondpb.Eth1Data)
	SetRandaoReveal([]byte)
	SetBlinded(bool)
	SetStateRoot([]byte)
	SetParentRoot([]byte)
	SetProposerIndex(idx primitives.ValidatorIndex)
	SetSlot(slot primitives.Slot)
	SetSignature(sig []byte)
}

// ExecutionData represents execution layer information that is contained
// within beacon block bodies.
type ExecutionData interface {
	ssz.Marshaler
	ssz.Unmarshaler
	ssz.HashRoot
	IsNil() bool
	IsBlinded() bool
	Proto() proto.Message
	ParentHash() []byte
	FeeRecipient() []byte
	StateRoot() []byte
	ReceiptsRoot() []byte
	LogsBloom() []byte
	PrevRandao() []byte
	BlockNumber() uint64
	GasLimit() uint64
	GasUsed() uint64
	Timestamp() uint64
	ExtraData() []byte
	BaseFeePerGas() []byte
	BlockHash() []byte
	Transactions() ([][]byte, error)
	TransactionsRoot() ([]byte, error)
	Withdrawals() ([]*enginev1.Withdrawal, error)
	WithdrawalsRoot() ([]byte, error)
	PbCapella() (*enginev1.ExecutionPayloadCapella, error)
	ValueInGwei() (uint64, error)
}
