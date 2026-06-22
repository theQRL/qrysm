package blocks

import (
	"fmt"

	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	log "github.com/sirupsen/logrus"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	consensus_types "github.com/theQRL/qrysm/consensus-types"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/runtime/version"
)

// BeaconBlockIsNil checks if any composite field of input signed beacon block is nil.
// Access to these nil fields will result in run time panic,
// it is recommended to run these checks as first line of defense.
func BeaconBlockIsNil(b interfaces.ReadOnlySignedBeaconBlock) error {
	if b == nil || b.IsNil() {
		return ErrNilSignedBeaconBlock
	}
	return nil
}

// Signature returns the respective block signature.
func (b *SignedBeaconBlock) Signature() [field_params.MLDSA87SignatureLength]byte {
	return b.signature
}

// Block returns the underlying beacon block object.
func (b *SignedBeaconBlock) Block() interfaces.ReadOnlyBeaconBlock {
	return b.block
}

// IsNil checks if the underlying beacon block is nil.
func (b *SignedBeaconBlock) IsNil() bool {
	return b == nil || b.block.IsNil()
}

// Copy performs a deep copy of the signed beacon block object.
func (b *SignedBeaconBlock) Copy() (interfaces.ReadOnlySignedBeaconBlock, error) {
	if b == nil {
		return nil, nil
	}

	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			cp := qrysmpb.CopySignedBlindedBeaconBlockZond(pb.(*qrysmpb.SignedBlindedBeaconBlockZond))
			return initBlindedSignedBlockFromProtoZond(cp)
		}
		cp := qrysmpb.CopySignedBeaconBlockZond(pb.(*qrysmpb.SignedBeaconBlockZond))
		return initSignedBlockFromProtoZond(cp)
	default:
		return nil, errIncorrectBlockVersion
	}
}

// PbGenericBlock returns a generic signed beacon block.
func (b *SignedBeaconBlock) PbGenericBlock() (*qrysmpb.GenericSignedBeaconBlock, error) {
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			return &qrysmpb.GenericSignedBeaconBlock{
				Block: &qrysmpb.GenericSignedBeaconBlock_BlindedZond{BlindedZond: pb.(*qrysmpb.SignedBlindedBeaconBlockZond)},
			}, nil
		}
		return &qrysmpb.GenericSignedBeaconBlock{
			Block: &qrysmpb.GenericSignedBeaconBlock_Zond{Zond: pb.(*qrysmpb.SignedBeaconBlockZond)},
		}, nil
	default:
		return nil, errIncorrectBlockVersion
	}
}

// PbZondBlock returns the underlying protobuf object.
func (b *SignedBeaconBlock) PbZondBlock() (*qrysmpb.SignedBeaconBlockZond, error) {
	if b.IsBlinded() {
		return nil, consensus_types.ErrNotSupported("PbZondBlock", b.version)
	}
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	return pb.(*qrysmpb.SignedBeaconBlockZond), nil
}

// PbBlindedZondBlock returns the underlying protobuf object.
func (b *SignedBeaconBlock) PbBlindedZondBlock() (*qrysmpb.SignedBlindedBeaconBlockZond, error) {
	if !b.IsBlinded() {
		return nil, consensus_types.ErrNotSupported("PbBlindedZondBlock", b.version)
	}
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	return pb.(*qrysmpb.SignedBlindedBeaconBlockZond), nil
}

// ToBlinded converts a non-blinded block to its blinded equivalent.
func (b *SignedBeaconBlock) ToBlinded() (interfaces.ReadOnlySignedBeaconBlock, error) {
	if b.IsBlinded() {
		return b, nil
	}
	if b.block.IsNil() {
		return nil, errors.New("cannot convert nil block to blinded format")
	}
	payload, err := b.block.Body().Execution()
	if err != nil {
		return nil, err
	}

	switch p := payload.Proto().(type) {
	case *enginev1.ExecutionPayloadZond:
		header, err := PayloadToHeaderZond(payload)
		if err != nil {
			return nil, err
		}
		return initBlindedSignedBlockFromProtoZond(
			&qrysmpb.SignedBlindedBeaconBlockZond{
				Block: &qrysmpb.BlindedBeaconBlockZond{
					Slot:          b.block.slot,
					ProposerIndex: b.block.proposerIndex,
					ParentRoot:    b.block.parentRoot[:],
					StateRoot:     b.block.stateRoot[:],
					Body: &qrysmpb.BlindedBeaconBlockBodyZond{
						RandaoReveal:           b.block.body.randaoReveal[:],
						ExecutionData:          b.block.body.executionData,
						Graffiti:               b.block.body.graffiti[:],
						ProposerSlashings:      b.block.body.proposerSlashings,
						AttesterSlashings:      b.block.body.attesterSlashings,
						Attestations:           b.block.body.attestations,
						Deposits:               b.block.body.deposits,
						VoluntaryExits:         b.block.body.voluntaryExits,
						SyncAggregate:          b.block.body.syncAggregate,
						ExecutionPayloadHeader: header,
					},
				},
				Signature: b.signature[:],
			})
	default:
		return nil, fmt.Errorf("%T is not an execution payload header", p)
	}
}

// Version of the underlying protobuf object.
func (b *SignedBeaconBlock) Version() int {
	return b.version
}

// IsBlinded metadata on whether a block is blinded
func (b *SignedBeaconBlock) IsBlinded() bool {
	return b.block.body.isBlinded
}

// ValueInShor metadata on the payload value returned by the builder. Value is 0 by default if local.
func (b *SignedBeaconBlock) ValueInShor() uint64 {
	exec, err := b.block.body.Execution()
	if err != nil {
		log.WithError(err).Warn("Failed to retrieve execution payload")
		return 0
	}
	val, err := exec.ValueInShor()
	if err != nil {
		log.WithError(err).Warn("Failed to retrieve value in shor")
		return 0
	}
	return val
}

// Header converts the underlying protobuf object from blinded block to header format.
func (b *SignedBeaconBlock) Header() (*qrysmpb.SignedBeaconBlockHeader, error) {
	if b.IsNil() {
		return nil, errNilBlock
	}
	root, err := b.block.body.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrapf(err, "could not hash block body")
	}

	return &qrysmpb.SignedBeaconBlockHeader{
		Header: &qrysmpb.BeaconBlockHeader{
			Slot:          b.block.slot,
			ProposerIndex: b.block.proposerIndex,
			ParentRoot:    b.block.parentRoot[:],
			StateRoot:     b.block.stateRoot[:],
			BodyRoot:      root[:],
		},
		Signature: b.signature[:],
	}, nil
}

// MarshalSSZ marshals the signed beacon block to its relevant ssz form.
func (b *SignedBeaconBlock) MarshalSSZ() ([]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return []byte{}, err
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			return pb.(*qrysmpb.SignedBlindedBeaconBlockZond).MarshalSSZ()
		}
		return pb.(*qrysmpb.SignedBeaconBlockZond).MarshalSSZ()
	default:
		return []byte{}, errIncorrectBlockVersion
	}
}

// MarshalSSZTo marshals the signed beacon block's ssz
// form to the provided byte buffer.
func (b *SignedBeaconBlock) MarshalSSZTo(dst []byte) ([]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return []byte{}, err
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			return pb.(*qrysmpb.SignedBlindedBeaconBlockZond).MarshalSSZTo(dst)
		}
		return pb.(*qrysmpb.SignedBeaconBlockZond).MarshalSSZTo(dst)
	default:
		return []byte{}, errIncorrectBlockVersion
	}
}

// SizeSSZ returns the size of the serialized signed block
//
// WARNING: This function panics. It is required to change the signature
// of fastssz's SizeSSZ() interface function to avoid panicking.
// Changing the signature causes very problematic issues with wealdtech deps.
// For the time being panicking is preferable.
func (b *SignedBeaconBlock) SizeSSZ() int {
	pb, err := b.Proto()
	if err != nil {
		panic(err) // lint:nopanic
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			return pb.(*qrysmpb.SignedBlindedBeaconBlockZond).SizeSSZ()
		}
		return pb.(*qrysmpb.SignedBeaconBlockZond).SizeSSZ()
	default:
		panic(incorrectBlockVersion) // lint:nopanic
	}
}

// UnmarshalSSZ unmarshals the signed beacon block from its relevant ssz form.
func (b *SignedBeaconBlock) UnmarshalSSZ(buf []byte) error {
	var newBlock *SignedBeaconBlock
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			pb := &qrysmpb.SignedBlindedBeaconBlockZond{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedSignedBlockFromProtoZond(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &qrysmpb.SignedBeaconBlockZond{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initSignedBlockFromProtoZond(pb)
			if err != nil {
				return err
			}
		}
	default:
		return errIncorrectBlockVersion
	}
	*b = *newBlock
	return nil
}

// Slot returns the respective slot of the block.
func (b *BeaconBlock) Slot() primitives.Slot {
	return b.slot
}

// ProposerIndex returns the proposer index of the beacon block.
func (b *BeaconBlock) ProposerIndex() primitives.ValidatorIndex {
	return b.proposerIndex
}

// ParentRoot returns the parent root of beacon block.
func (b *BeaconBlock) ParentRoot() [field_params.RootLength]byte {
	return b.parentRoot
}

// StateRoot returns the state root of the beacon block.
func (b *BeaconBlock) StateRoot() [field_params.RootLength]byte {
	return b.stateRoot
}

// Body returns the underlying block body.
func (b *BeaconBlock) Body() interfaces.ReadOnlyBeaconBlockBody {
	return b.body
}

// IsNil checks if the beacon block is nil.
func (b *BeaconBlock) IsNil() bool {
	return b == nil || b.Body().IsNil()
}

// IsBlinded checks if the beacon block is a blinded block.
func (b *BeaconBlock) IsBlinded() bool {
	return b.body.isBlinded
}

// Version of the underlying protobuf object.
func (b *BeaconBlock) Version() int {
	return b.version
}

// HashTreeRoot returns the ssz root of the block.
func (b *BeaconBlock) HashTreeRoot() ([field_params.RootLength]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return [field_params.RootLength]byte{}, err
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			return pb.(*qrysmpb.BlindedBeaconBlockZond).HashTreeRoot()
		}
		return pb.(*qrysmpb.BeaconBlockZond).HashTreeRoot()
	default:
		return [field_params.RootLength]byte{}, errIncorrectBlockVersion
	}
}

// HashTreeRootWith ssz hashes the BeaconBlock object with a hasher.
func (b *BeaconBlock) HashTreeRootWith(h *ssz.Hasher) error {
	pb, err := b.Proto()
	if err != nil {
		return err
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			return pb.(*qrysmpb.BlindedBeaconBlockZond).HashTreeRootWith(h)
		}
		return pb.(*qrysmpb.BeaconBlockZond).HashTreeRootWith(h)
	default:
		return errIncorrectBlockVersion
	}
}

// MarshalSSZ marshals the block into its respective
// ssz form.
func (b *BeaconBlock) MarshalSSZ() ([]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return []byte{}, err
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			return pb.(*qrysmpb.BlindedBeaconBlockZond).MarshalSSZ()
		}
		return pb.(*qrysmpb.BeaconBlockZond).MarshalSSZ()
	default:
		return []byte{}, errIncorrectBlockVersion
	}
}

// MarshalSSZTo marshals the beacon block's ssz
// form to the provided byte buffer.
func (b *BeaconBlock) MarshalSSZTo(dst []byte) ([]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return []byte{}, err
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			return pb.(*qrysmpb.BlindedBeaconBlockZond).MarshalSSZTo(dst)
		}
		return pb.(*qrysmpb.BeaconBlockZond).MarshalSSZTo(dst)
	default:
		return []byte{}, errIncorrectBlockVersion
	}
}

// SizeSSZ returns the size of the serialized block.
//
// WARNING: This function panics. It is required to change the signature
// of fastssz's SizeSSZ() interface function to avoid panicking.
// Changing the signature causes very problematic issues with wealdtech deps.
// For the time being panicking is preferable.
func (b *BeaconBlock) SizeSSZ() int {
	pb, err := b.Proto()
	if err != nil {
		panic(err) // lint:nopanic
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			return pb.(*qrysmpb.BlindedBeaconBlockZond).SizeSSZ()
		}
		return pb.(*qrysmpb.BeaconBlockZond).SizeSSZ()
	default:
		panic(incorrectBodyVersion) // lint:nopanic
	}
}

// UnmarshalSSZ unmarshals the beacon block from its relevant ssz form.
func (b *BeaconBlock) UnmarshalSSZ(buf []byte) error {
	var newBlock *BeaconBlock
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			pb := &qrysmpb.BlindedBeaconBlockZond{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedBlockFromProtoZond(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &qrysmpb.BeaconBlockZond{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlockFromProtoZond(pb)
			if err != nil {
				return err
			}
		}
	default:
		return errIncorrectBlockVersion
	}
	*b = *newBlock
	return nil
}

// AsSignRequestObject returns the underlying sign request object.
func (b *BeaconBlock) AsSignRequestObject() (validatorpb.SignRequestObject, error) {
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			return &validatorpb.SignRequest_BlindedBlockZond{BlindedBlockZond: pb.(*qrysmpb.BlindedBeaconBlockZond)}, nil
		}
		return &validatorpb.SignRequest_BlockZond{BlockZond: pb.(*qrysmpb.BeaconBlockZond)}, nil
	default:
		return nil, errIncorrectBlockVersion
	}
}

func (b *BeaconBlock) Copy() (interfaces.ReadOnlyBeaconBlock, error) {
	if b == nil {
		return nil, nil
	}

	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			cp := qrysmpb.CopyBlindedBeaconBlockZond(pb.(*qrysmpb.BlindedBeaconBlockZond))
			return initBlindedBlockFromProtoZond(cp)
		}
		cp := qrysmpb.CopyBeaconBlockZond(pb.(*qrysmpb.BeaconBlockZond))
		return initBlockFromProtoZond(cp)
	default:
		return nil, errIncorrectBlockVersion
	}
}

// IsNil checks if the block body is nil.
func (b *BeaconBlockBody) IsNil() bool {
	return b == nil
}

// RandaoReveal returns the randao reveal from the block body.
func (b *BeaconBlockBody) RandaoReveal() [field_params.MLDSA87SignatureLength]byte {
	return b.randaoReveal
}

// ExecutionData returns the execution data in the block.
func (b *BeaconBlockBody) ExecutionData() *qrysmpb.ExecutionData {
	return b.executionData
}

// Graffiti returns the graffiti in the block.
func (b *BeaconBlockBody) Graffiti() [field_params.RootLength]byte {
	return b.graffiti
}

// ProposerSlashings returns the proposer slashings in the block.
func (b *BeaconBlockBody) ProposerSlashings() []*qrysmpb.ProposerSlashing {
	return b.proposerSlashings
}

// AttesterSlashings returns the attester slashings in the block.
func (b *BeaconBlockBody) AttesterSlashings() []*qrysmpb.AttesterSlashing {
	return b.attesterSlashings
}

// Attestations returns the stored attestations in the block.
func (b *BeaconBlockBody) Attestations() []*qrysmpb.Attestation {
	return b.attestations
}

// Deposits returns the stored deposits in the block.
func (b *BeaconBlockBody) Deposits() []*qrysmpb.Deposit {
	return b.deposits
}

// VoluntaryExits returns the voluntary exits in the block.
func (b *BeaconBlockBody) VoluntaryExits() []*qrysmpb.SignedVoluntaryExit {
	return b.voluntaryExits
}

// SyncAggregate returns the sync aggregate in the block.
func (b *BeaconBlockBody) SyncAggregate() (*qrysmpb.SyncAggregate, error) {
	return b.syncAggregate, nil
}

// Execution returns the execution payload of the block body.
func (b *BeaconBlockBody) Execution() (interfaces.ExecutionData, error) {
	switch b.version {
	case version.Zond:
		if b.isBlinded {
			var ph *enginev1.ExecutionPayloadHeaderZond
			var ok bool
			if b.executionPayloadHeader != nil {
				ph, ok = b.executionPayloadHeader.Proto().(*enginev1.ExecutionPayloadHeaderZond)
				if !ok {
					return nil, errPayloadHeaderWrongType
				}
				return WrappedExecutionPayloadHeaderZond(ph, 0)
			}
		}
		var p *enginev1.ExecutionPayloadZond
		var ok bool
		if b.executionPayload != nil {
			p, ok = b.executionPayload.Proto().(*enginev1.ExecutionPayloadZond)
			if !ok {
				return nil, errPayloadWrongType
			}
		}
		return WrappedExecutionPayloadZond(p, 0)
	default:
		return nil, errIncorrectBlockVersion
	}
}

// HashTreeRoot returns the ssz root of the block body.
func (b *BeaconBlockBody) HashTreeRoot() ([field_params.RootLength]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return [field_params.RootLength]byte{}, err
	}
	switch b.version {
	case version.Zond:
		if b.isBlinded {
			return pb.(*qrysmpb.BlindedBeaconBlockBodyZond).HashTreeRoot()
		}
		return pb.(*qrysmpb.BeaconBlockBodyZond).HashTreeRoot()
	default:
		return [field_params.RootLength]byte{}, errIncorrectBodyVersion
	}
}
