package blocks

import (
	field_params "github.com/cyyber/qrysm/v4/config/fieldparams"
	"github.com/cyyber/qrysm/v4/consensus-types/interfaces"
	"github.com/cyyber/qrysm/v4/consensus-types/primitives"
	eth "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
)

var (
	_ = interfaces.ReadOnlySignedBeaconBlock(&SignedBeaconBlock{})
	_ = interfaces.ReadOnlyBeaconBlock(&BeaconBlock{})
	_ = interfaces.ReadOnlyBeaconBlockBody(&BeaconBlockBody{})
)

var (
	errPayloadWrongType       = errors.New("execution payload has wrong type")
	errPayloadHeaderWrongType = errors.New("execution payload header has wrong type")
)

const (
	incorrectBlockVersion = "incorrect beacon block version"
	incorrectBodyVersion  = "incorrect beacon block body version"
)

var (
	// ErrUnsupportedVersion for beacon block methods.
	ErrUnsupportedVersion    = errors.New("unsupported beacon block version")
	errNilBlock              = errors.New("received nil beacon block")
	errNilBlockBody          = errors.New("received nil beacon block body")
	errIncorrectBlockVersion = errors.New(incorrectBlockVersion)
	errIncorrectBodyVersion  = errors.New(incorrectBodyVersion)
)

// BeaconBlockBody is the main beacon block body structure. It can represent any block type.
type BeaconBlockBody struct {
	version                     int
	isBlinded                   bool
	randaoReveal                [dilithium2.CryptoBytes]byte
	eth1Data                    *eth.Eth1Data
	graffiti                    [field_params.RootLength]byte
	proposerSlashings           []*eth.ProposerSlashing
	attesterSlashings           []*eth.AttesterSlashing
	attestations                []*eth.Attestation
	deposits                    []*eth.Deposit
	voluntaryExits              []*eth.SignedVoluntaryExit
	syncAggregate               *eth.SyncAggregate
	executionPayload            interfaces.ExecutionData
	executionPayloadHeader      interfaces.ExecutionData
	dilithiumToExecutionChanges []*eth.SignedDilithiumToExecutionChange
}

// BeaconBlock is the main beacon block structure. It can represent any block type.
type BeaconBlock struct {
	version       int
	slot          primitives.Slot
	proposerIndex primitives.ValidatorIndex
	parentRoot    [field_params.RootLength]byte
	stateRoot     [field_params.RootLength]byte
	body          *BeaconBlockBody
}

// SignedBeaconBlock is the main signed beacon block structure. It can represent any block type.
type SignedBeaconBlock struct {
	version   int
	block     *BeaconBlock
	signature [dilithium2.CryptoBytes]byte
}
