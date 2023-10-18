package mock

import (
	ssz "github.com/prysmaticlabs/fastssz"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1/validator-client"
	"google.golang.org/protobuf/proto"
)

type SignedBeaconBlock struct {
	BeaconBlock interfaces.ReadOnlyBeaconBlock
}

func (SignedBeaconBlock) PbGenericBlock() (*zond.GenericSignedBeaconBlock, error) {
	panic("implement me")
}

func (m SignedBeaconBlock) Block() interfaces.ReadOnlyBeaconBlock {
	return m.BeaconBlock
}

func (SignedBeaconBlock) Signature() [dilithium2.CryptoBytes]byte {
	panic("implement me")
}

func (SignedBeaconBlock) SetSignature([]byte) {
	panic("implement me")
}

func (m SignedBeaconBlock) IsNil() bool {
	return m.BeaconBlock == nil || m.Block().IsNil()
}

func (SignedBeaconBlock) Copy() (interfaces.ReadOnlySignedBeaconBlock, error) {
	panic("implement me")
}

func (SignedBeaconBlock) Proto() (proto.Message, error) {
	panic("implement me")
}

func (SignedBeaconBlock) PbPhase0Block() (*zond.SignedBeaconBlock, error) {
	panic("implement me")
}

func (SignedBeaconBlock) PbAltairBlock() (*zond.SignedBeaconBlockAltair, error) {
	panic("implement me")
}

func (SignedBeaconBlock) PbBellatrixBlock() (*zond.SignedBeaconBlockBellatrix, error) {
	panic("implement me")
}

func (SignedBeaconBlock) PbBlindedBellatrixBlock() (*zond.SignedBlindedBeaconBlockBellatrix, error) {
	panic("implement me")
}

func (SignedBeaconBlock) PbCapellaBlock() (*zond.SignedBeaconBlockCapella, error) {
	panic("implement me")
}

func (SignedBeaconBlock) PbBlindedCapellaBlock() (*zond.SignedBlindedBeaconBlockCapella, error) {
	panic("implement me")
}

func (SignedBeaconBlock) MarshalSSZTo(_ []byte) ([]byte, error) {
	panic("implement me")
}

func (SignedBeaconBlock) MarshalSSZ() ([]byte, error) {
	panic("implement me")
}

func (SignedBeaconBlock) SizeSSZ() int {
	panic("implement me")
}

func (SignedBeaconBlock) UnmarshalSSZ(_ []byte) error {
	panic("implement me")
}

func (SignedBeaconBlock) Version() int {
	panic("implement me")
}

func (SignedBeaconBlock) IsBlinded() bool {
	return false
}

func (SignedBeaconBlock) ToBlinded() (interfaces.ReadOnlySignedBeaconBlock, error) {
	panic("implement me")
}

func (SignedBeaconBlock) Header() (*zond.SignedBeaconBlockHeader, error) {
	panic("implement me")
}

type BeaconBlock struct {
	Htr             [field_params.RootLength]byte
	HtrErr          error
	BeaconBlockBody interfaces.ReadOnlyBeaconBlockBody
	BlockSlot       primitives.Slot
}

func (BeaconBlock) AsSignRequestObject() (validatorpb.SignRequestObject, error) {
	panic("implement me")
}

func (m BeaconBlock) HashTreeRoot() ([field_params.RootLength]byte, error) {
	return m.Htr, m.HtrErr
}

func (m BeaconBlock) Slot() primitives.Slot {
	return m.BlockSlot
}

func (BeaconBlock) ProposerIndex() primitives.ValidatorIndex {
	panic("implement me")
}

func (BeaconBlock) ParentRoot() [field_params.RootLength]byte {
	panic("implement me")
}

func (BeaconBlock) StateRoot() [field_params.RootLength]byte {
	panic("implement me")
}

func (m BeaconBlock) Body() interfaces.ReadOnlyBeaconBlockBody {
	return m.BeaconBlockBody
}

func (BeaconBlock) IsNil() bool {
	return false
}

func (BeaconBlock) IsBlinded() bool {
	return false
}

func (BeaconBlock) Proto() (proto.Message, error) {
	panic("implement me")
}

func (BeaconBlock) MarshalSSZTo(_ []byte) ([]byte, error) {
	panic("implement me")
}

func (BeaconBlock) MarshalSSZ() ([]byte, error) {
	panic("implement me")
}

func (BeaconBlock) SizeSSZ() int {
	panic("implement me")
}

func (BeaconBlock) UnmarshalSSZ(_ []byte) error {
	panic("implement me")
}

func (BeaconBlock) HashTreeRootWith(_ *ssz.Hasher) error {
	panic("implement me")
}

func (BeaconBlock) Version() int {
	panic("implement me")
}

func (BeaconBlock) ToBlinded() (interfaces.ReadOnlyBeaconBlock, error) {
	panic("implement me")
}

func (BeaconBlock) SetSlot(_ primitives.Slot) {
	panic("implement me")
}

func (BeaconBlock) SetProposerIndex(_ primitives.ValidatorIndex) {
	panic("implement me")
}

func (BeaconBlock) SetParentRoot(_ []byte) {
	panic("implement me")
}

func (BeaconBlock) SetBlinded(_ bool) {
	panic("implement me")
}

func (BeaconBlock) Copy() (interfaces.ReadOnlyBeaconBlock, error) {
	panic("implement me")
}

type BeaconBlockBody struct{}

func (BeaconBlockBody) RandaoReveal() [dilithium2.CryptoBytes]byte {
	panic("implement me")
}

func (BeaconBlockBody) Eth1Data() *zond.Eth1Data {
	panic("implement me")
}

func (BeaconBlockBody) Graffiti() [field_params.RootLength]byte {
	panic("implement me")
}

func (BeaconBlockBody) ProposerSlashings() []*zond.ProposerSlashing {
	panic("implement me")
}

func (BeaconBlockBody) AttesterSlashings() []*zond.AttesterSlashing {
	panic("implement me")
}

func (BeaconBlockBody) Attestations() []*zond.Attestation {
	panic("implement me")
}

func (BeaconBlockBody) Deposits() []*zond.Deposit {
	panic("implement me")
}

func (BeaconBlockBody) VoluntaryExits() []*zond.SignedVoluntaryExit {
	panic("implement me")
}

func (BeaconBlockBody) SyncAggregate() (*zond.SyncAggregate, error) {
	panic("implement me")
}

func (BeaconBlockBody) IsNil() bool {
	return false
}

func (BeaconBlockBody) HashTreeRoot() ([field_params.RootLength]byte, error) {
	panic("implement me")
}

func (BeaconBlockBody) Proto() (proto.Message, error) {
	panic("implement me")
}

func (BeaconBlockBody) Execution() (interfaces.ExecutionData, error) {
	panic("implement me")
}

func (BeaconBlockBody) DilithiumToExecutionChanges() ([]*zond.SignedDilithiumToExecutionChange, error) {
	panic("implement me")
}

func (b *BeaconBlock) SetStateRoot(root []byte) {
	panic("implement me")
}

func (b *BeaconBlockBody) SetRandaoReveal([]byte) {
	panic("implement me")
}

func (b *BeaconBlockBody) SetEth1Data(*zond.Eth1Data) {
	panic("implement me")
}

func (b *BeaconBlockBody) SetGraffiti([]byte) {
	panic("implement me")
}

func (b *BeaconBlockBody) SetProposerSlashings([]*zond.ProposerSlashing) {
	panic("implement me")
}

func (b *BeaconBlockBody) SetAttesterSlashings([]*zond.AttesterSlashing) {
	panic("implement me")
}

func (b *BeaconBlockBody) SetAttestations([]*zond.Attestation) {
	panic("implement me")
}

func (b *BeaconBlockBody) SetDeposits([]*zond.Deposit) {
	panic("implement me")
}

func (b *BeaconBlockBody) SetVoluntaryExits([]*zond.SignedVoluntaryExit) {
	panic("implement me")
}

func (b *BeaconBlockBody) SetSyncAggregate(*zond.SyncAggregate) error {
	panic("implement me")
}

func (b *BeaconBlockBody) SetExecution(interfaces.ExecutionData) error {
	panic("implement me")
}

func (b *BeaconBlockBody) SetDilithiumToExecutionChanges([]*zond.SignedDilithiumToExecutionChange) error {
	panic("implement me")
}

var _ interfaces.ReadOnlySignedBeaconBlock = &SignedBeaconBlock{}
var _ interfaces.ReadOnlyBeaconBlock = &BeaconBlock{}
var _ interfaces.ReadOnlyBeaconBlockBody = &BeaconBlockBody{}
