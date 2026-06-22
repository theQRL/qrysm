package interfaces

import (
	ssz "github.com/prysmaticlabs/fastssz"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/validator-client"
	"google.golang.org/protobuf/proto"
)

// ReadOnlySignedBeaconBlock is an interface describing the method set of
// a signed beacon block.
type ReadOnlySignedBeaconBlock interface {
	Block() ReadOnlyBeaconBlock
	Signature() [field_params.MLDSA87SignatureLength]byte
	IsNil() bool
	Copy() (ReadOnlySignedBeaconBlock, error)
	Proto() (proto.Message, error)
	PbGenericBlock() (*qrysmpb.GenericSignedBeaconBlock, error)
	ToBlinded() (ReadOnlySignedBeaconBlock, error)
	PbZondBlock() (*qrysmpb.SignedBeaconBlockZond, error)
	PbBlindedZondBlock() (*qrysmpb.SignedBlindedBeaconBlockZond, error)
	ssz.Marshaler
	ssz.Unmarshaler
	Version() int
	IsBlinded() bool
	ValueInShor() uint64
	Header() (*qrysmpb.SignedBeaconBlockHeader, error)
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
	RandaoReveal() [field_params.MLDSA87SignatureLength]byte
	ExecutionData() *qrysmpb.ExecutionData
	Graffiti() [field_params.RootLength]byte
	ProposerSlashings() []*qrysmpb.ProposerSlashing
	AttesterSlashings() []*qrysmpb.AttesterSlashing
	Attestations() []*qrysmpb.Attestation
	Deposits() []*qrysmpb.Deposit
	VoluntaryExits() []*qrysmpb.SignedVoluntaryExit
	SyncAggregate() (*qrysmpb.SyncAggregate, error)
	IsNil() bool
	HashTreeRoot() ([field_params.RootLength]byte, error)
	Proto() (proto.Message, error)
	Execution() (ExecutionData, error)
}

type SignedBeaconBlock interface {
	ReadOnlySignedBeaconBlock
	SetExecution(ExecutionData) error
	SetSyncAggregate(*qrysmpb.SyncAggregate) error
	SetVoluntaryExits([]*qrysmpb.SignedVoluntaryExit)
	SetDeposits([]*qrysmpb.Deposit)
	SetAttestations([]*qrysmpb.Attestation)
	SetAttesterSlashings([]*qrysmpb.AttesterSlashing)
	SetProposerSlashings([]*qrysmpb.ProposerSlashing)
	SetGraffiti([]byte)
	SetExecutionData(*qrysmpb.ExecutionData)
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
	PbZond() (*enginev1.ExecutionPayloadZond, error)
	ValueInShor() (uint64, error)
}
