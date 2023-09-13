package interfaces

import (
	field_params "github.com/cyyber/qrysm/v4/config/fieldparams"
	"github.com/cyyber/qrysm/v4/consensus-types/primitives"
	enginev1 "github.com/cyyber/qrysm/v4/proto/engine/v1"
	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	validatorpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1/validator-client"
	ssz "github.com/prysmaticlabs/fastssz"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"google.golang.org/protobuf/proto"
)

// ReadOnlySignedBeaconBlock is an interface describing the method set of
// a signed beacon block.
type ReadOnlySignedBeaconBlock interface {
	Block() ReadOnlyBeaconBlock
	Signature() [dilithium2.CryptoBytes]byte
	IsNil() bool
	Copy() (ReadOnlySignedBeaconBlock, error)
	Proto() (proto.Message, error)
	PbGenericBlock() (*ethpb.GenericSignedBeaconBlock, error)
	PbPhase0Block() (*ethpb.SignedBeaconBlock, error)
	PbAltairBlock() (*ethpb.SignedBeaconBlockAltair, error)
	ToBlinded() (ReadOnlySignedBeaconBlock, error)
	PbBellatrixBlock() (*ethpb.SignedBeaconBlockBellatrix, error)
	PbBlindedBellatrixBlock() (*ethpb.SignedBlindedBeaconBlockBellatrix, error)
	PbCapellaBlock() (*ethpb.SignedBeaconBlockCapella, error)
	PbBlindedCapellaBlock() (*ethpb.SignedBlindedBeaconBlockCapella, error)
	ssz.Marshaler
	ssz.Unmarshaler
	Version() int
	IsBlinded() bool
	Header() (*ethpb.SignedBeaconBlockHeader, error)
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
	RandaoReveal() [dilithium2.CryptoBytes]byte
	Eth1Data() *ethpb.Eth1Data
	Graffiti() [field_params.RootLength]byte
	ProposerSlashings() []*ethpb.ProposerSlashing
	AttesterSlashings() []*ethpb.AttesterSlashing
	Attestations() []*ethpb.Attestation
	Deposits() []*ethpb.Deposit
	VoluntaryExits() []*ethpb.SignedVoluntaryExit
	SyncAggregate() (*ethpb.SyncAggregate, error)
	IsNil() bool
	HashTreeRoot() ([field_params.RootLength]byte, error)
	Proto() (proto.Message, error)
	Execution() (ExecutionData, error)
	DilithiumToExecutionChanges() ([]*ethpb.SignedDilithiumToExecutionChange, error)
}

type SignedBeaconBlock interface {
	ReadOnlySignedBeaconBlock
	SetExecution(ExecutionData) error
	SetDilithiumToExecutionChanges([]*ethpb.SignedDilithiumToExecutionChange) error
	SetSyncAggregate(*ethpb.SyncAggregate) error
	SetVoluntaryExits([]*ethpb.SignedVoluntaryExit)
	SetDeposits([]*ethpb.Deposit)
	SetAttestations([]*ethpb.Attestation)
	SetAttesterSlashings([]*ethpb.AttesterSlashing)
	SetProposerSlashings([]*ethpb.ProposerSlashing)
	SetGraffiti([]byte)
	SetEth1Data(*ethpb.Eth1Data)
	SetRandaoReveal([]byte)
	SetBlinded(bool)
	SetStateRoot([]byte)
	SetParentRoot([]byte)
	SetProposerIndex(idx primitives.ValidatorIndex)
	SetSlot(slot primitives.Slot)
	SetSignature(sig []byte)
}

// ExecutionData represents execution layer information that is contained
// within post-Bellatrix beacon block bodies.
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
	PbBellatrix() (*enginev1.ExecutionPayload, error)
	ValueInGwei() (uint64, error)
}
