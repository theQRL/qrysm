package blocks

import (
	"fmt"

	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	consensus_types "github.com/theQRL/qrysm/v4/consensus-types"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/v4/runtime/version"
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
func (b *SignedBeaconBlock) Signature() [dilithium2.CryptoBytes]byte {
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
	case version.Phase0:
		cp := zond.CopySignedBeaconBlock(pb.(*zond.SignedBeaconBlock))
		return initSignedBlockFromProtoPhase0(cp)
	case version.Altair:
		cp := zond.CopySignedBeaconBlockAltair(pb.(*zond.SignedBeaconBlockAltair))
		return initSignedBlockFromProtoAltair(cp)
	case version.Bellatrix:
		if b.IsBlinded() {
			cp := zond.CopySignedBlindedBeaconBlockBellatrix(pb.(*zond.SignedBlindedBeaconBlockBellatrix))
			return initBlindedSignedBlockFromProtoBellatrix(cp)
		}
		cp := zond.CopySignedBeaconBlockBellatrix(pb.(*zond.SignedBeaconBlockBellatrix))
		return initSignedBlockFromProtoBellatrix(cp)
	case version.Capella:
		if b.IsBlinded() {
			cp := zond.CopySignedBlindedBeaconBlockCapella(pb.(*zond.SignedBlindedBeaconBlockCapella))
			return initBlindedSignedBlockFromProtoCapella(cp)
		}
		cp := zond.CopySignedBeaconBlockCapella(pb.(*zond.SignedBeaconBlockCapella))
		return initSignedBlockFromProtoCapella(cp)
	default:
		return nil, errIncorrectBlockVersion
	}
}

// PbGenericBlock returns a generic signed beacon block.
func (b *SignedBeaconBlock) PbGenericBlock() (*zond.GenericSignedBeaconBlock, error) {
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	switch b.version {
	case version.Phase0:
		return &zond.GenericSignedBeaconBlock{
			Block: &zond.GenericSignedBeaconBlock_Phase0{Phase0: pb.(*zond.SignedBeaconBlock)},
		}, nil
	case version.Altair:
		return &zond.GenericSignedBeaconBlock{
			Block: &zond.GenericSignedBeaconBlock_Altair{Altair: pb.(*zond.SignedBeaconBlockAltair)},
		}, nil
	case version.Bellatrix:
		if b.IsBlinded() {
			return &zond.GenericSignedBeaconBlock{
				Block: &zond.GenericSignedBeaconBlock_BlindedBellatrix{BlindedBellatrix: pb.(*zond.SignedBlindedBeaconBlockBellatrix)},
			}, nil
		}
		return &zond.GenericSignedBeaconBlock{
			Block: &zond.GenericSignedBeaconBlock_Bellatrix{Bellatrix: pb.(*zond.SignedBeaconBlockBellatrix)},
		}, nil
	case version.Capella:
		if b.IsBlinded() {
			return &zond.GenericSignedBeaconBlock{
				Block: &zond.GenericSignedBeaconBlock_BlindedCapella{BlindedCapella: pb.(*zond.SignedBlindedBeaconBlockCapella)},
			}, nil
		}
		return &zond.GenericSignedBeaconBlock{
			Block: &zond.GenericSignedBeaconBlock_Capella{Capella: pb.(*zond.SignedBeaconBlockCapella)},
		}, nil
	default:
		return nil, errIncorrectBlockVersion
	}
}

// PbPhase0Block returns the underlying protobuf object.
func (b *SignedBeaconBlock) PbPhase0Block() (*zond.SignedBeaconBlock, error) {
	if b.version != version.Phase0 {
		return nil, consensus_types.ErrNotSupported("PbPhase0Block", b.version)
	}
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	return pb.(*zond.SignedBeaconBlock), nil
}

// PbAltairBlock returns the underlying protobuf object.
func (b *SignedBeaconBlock) PbAltairBlock() (*zond.SignedBeaconBlockAltair, error) {
	if b.version != version.Altair {
		return nil, consensus_types.ErrNotSupported("PbAltairBlock", b.version)
	}
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	return pb.(*zond.SignedBeaconBlockAltair), nil
}

// PbBellatrixBlock returns the underlying protobuf object.
func (b *SignedBeaconBlock) PbBellatrixBlock() (*zond.SignedBeaconBlockBellatrix, error) {
	if b.version != version.Bellatrix || b.IsBlinded() {
		return nil, consensus_types.ErrNotSupported("PbBellatrixBlock", b.version)
	}
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	return pb.(*zond.SignedBeaconBlockBellatrix), nil
}

// PbBlindedBellatrixBlock returns the underlying protobuf object.
func (b *SignedBeaconBlock) PbBlindedBellatrixBlock() (*zond.SignedBlindedBeaconBlockBellatrix, error) {
	if b.version != version.Bellatrix || !b.IsBlinded() {
		return nil, consensus_types.ErrNotSupported("PbBlindedBellatrixBlock", b.version)
	}
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	return pb.(*zond.SignedBlindedBeaconBlockBellatrix), nil
}

// PbCapellaBlock returns the underlying protobuf object.
func (b *SignedBeaconBlock) PbCapellaBlock() (*zond.SignedBeaconBlockCapella, error) {
	if b.version != version.Capella || b.IsBlinded() {
		return nil, consensus_types.ErrNotSupported("PbCapellaBlock", b.version)
	}
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	return pb.(*zond.SignedBeaconBlockCapella), nil
}

// PbBlindedCapellaBlock returns the underlying protobuf object.
func (b *SignedBeaconBlock) PbBlindedCapellaBlock() (*zond.SignedBlindedBeaconBlockCapella, error) {
	if b.version != version.Capella || !b.IsBlinded() {
		return nil, consensus_types.ErrNotSupported("PbBlindedCapellaBlock", b.version)
	}
	pb, err := b.Proto()
	if err != nil {
		return nil, err
	}
	return pb.(*zond.SignedBlindedBeaconBlockCapella), nil
}

// ToBlinded converts a non-blinded block to its blinded equivalent.
func (b *SignedBeaconBlock) ToBlinded() (interfaces.ReadOnlySignedBeaconBlock, error) {
	if b.version < version.Bellatrix {
		return nil, ErrUnsupportedVersion
	}
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
	case *enginev1.ExecutionPayload:
		header, err := PayloadToHeader(payload)
		if err != nil {
			return nil, err
		}
		return initBlindedSignedBlockFromProtoBellatrix(
			&zond.SignedBlindedBeaconBlockBellatrix{
				Block: &zond.BlindedBeaconBlockBellatrix{
					Slot:          b.block.slot,
					ProposerIndex: b.block.proposerIndex,
					ParentRoot:    b.block.parentRoot[:],
					StateRoot:     b.block.stateRoot[:],
					Body: &zond.BlindedBeaconBlockBodyBellatrix{
						RandaoReveal:           b.block.body.randaoReveal[:],
						Eth1Data:               b.block.body.eth1Data,
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
	case *enginev1.ExecutionPayloadCapella:
		header, err := PayloadToHeaderCapella(payload)
		if err != nil {
			return nil, err
		}
		return initBlindedSignedBlockFromProtoCapella(
			&zond.SignedBlindedBeaconBlockCapella{
				Block: &zond.BlindedBeaconBlockCapella{
					Slot:          b.block.slot,
					ProposerIndex: b.block.proposerIndex,
					ParentRoot:    b.block.parentRoot[:],
					StateRoot:     b.block.stateRoot[:],
					Body: &zond.BlindedBeaconBlockBodyCapella{
						RandaoReveal:                b.block.body.randaoReveal[:],
						Eth1Data:                    b.block.body.eth1Data,
						Graffiti:                    b.block.body.graffiti[:],
						ProposerSlashings:           b.block.body.proposerSlashings,
						AttesterSlashings:           b.block.body.attesterSlashings,
						Attestations:                b.block.body.attestations,
						Deposits:                    b.block.body.deposits,
						VoluntaryExits:              b.block.body.voluntaryExits,
						SyncAggregate:               b.block.body.syncAggregate,
						ExecutionPayloadHeader:      header,
						DilithiumToExecutionChanges: b.block.body.dilithiumToExecutionChanges,
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

func (b *SignedBeaconBlock) IsBlinded() bool {
	return b.block.body.isBlinded
}

// Header converts the underlying protobuf object from blinded block to header format.
func (b *SignedBeaconBlock) Header() (*zond.SignedBeaconBlockHeader, error) {
	if b.IsNil() {
		return nil, errNilBlock
	}
	root, err := b.block.body.HashTreeRoot()
	if err != nil {
		return nil, errors.Wrapf(err, "could not hash block body")
	}

	return &zond.SignedBeaconBlockHeader{
		Header: &zond.BeaconBlockHeader{
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
	case version.Phase0:
		return pb.(*zond.SignedBeaconBlock).MarshalSSZ()
	case version.Altair:
		return pb.(*zond.SignedBeaconBlockAltair).MarshalSSZ()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*zond.SignedBlindedBeaconBlockBellatrix).MarshalSSZ()
		}
		return pb.(*zond.SignedBeaconBlockBellatrix).MarshalSSZ()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*zond.SignedBlindedBeaconBlockCapella).MarshalSSZ()
		}
		return pb.(*zond.SignedBeaconBlockCapella).MarshalSSZ()
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
	case version.Phase0:
		return pb.(*zond.SignedBeaconBlock).MarshalSSZTo(dst)
	case version.Altair:
		return pb.(*zond.SignedBeaconBlockAltair).MarshalSSZTo(dst)
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*zond.SignedBlindedBeaconBlockBellatrix).MarshalSSZTo(dst)
		}
		return pb.(*zond.SignedBeaconBlockBellatrix).MarshalSSZTo(dst)
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*zond.SignedBlindedBeaconBlockCapella).MarshalSSZTo(dst)
		}
		return pb.(*zond.SignedBeaconBlockCapella).MarshalSSZTo(dst)
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
		panic(err)
	}
	switch b.version {
	case version.Phase0:
		return pb.(*zond.SignedBeaconBlock).SizeSSZ()
	case version.Altair:
		return pb.(*zond.SignedBeaconBlockAltair).SizeSSZ()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*zond.SignedBlindedBeaconBlockBellatrix).SizeSSZ()
		}
		return pb.(*zond.SignedBeaconBlockBellatrix).SizeSSZ()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*zond.SignedBlindedBeaconBlockCapella).SizeSSZ()
		}
		return pb.(*zond.SignedBeaconBlockCapella).SizeSSZ()
	default:
		panic(incorrectBlockVersion)
	}
}

// UnmarshalSSZ unmarshals the signed beacon block from its relevant ssz form.
func (b *SignedBeaconBlock) UnmarshalSSZ(buf []byte) error {
	var newBlock *SignedBeaconBlock
	switch b.version {
	case version.Phase0:
		pb := &zond.SignedBeaconBlock{}
		if err := pb.UnmarshalSSZ(buf); err != nil {
			return err
		}
		var err error
		newBlock, err = initSignedBlockFromProtoPhase0(pb)
		if err != nil {
			return err
		}
	case version.Altair:
		pb := &zond.SignedBeaconBlockAltair{}
		if err := pb.UnmarshalSSZ(buf); err != nil {
			return err
		}
		var err error
		newBlock, err = initSignedBlockFromProtoAltair(pb)
		if err != nil {
			return err
		}
	case version.Bellatrix:
		if b.IsBlinded() {
			pb := &zond.SignedBlindedBeaconBlockBellatrix{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedSignedBlockFromProtoBellatrix(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &zond.SignedBeaconBlockBellatrix{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initSignedBlockFromProtoBellatrix(pb)
			if err != nil {
				return err
			}
		}
	case version.Capella:
		if b.IsBlinded() {
			pb := &zond.SignedBlindedBeaconBlockCapella{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedSignedBlockFromProtoCapella(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &zond.SignedBeaconBlockCapella{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initSignedBlockFromProtoCapella(pb)
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
	case version.Phase0:
		return pb.(*zond.BeaconBlock).HashTreeRoot()
	case version.Altair:
		return pb.(*zond.BeaconBlockAltair).HashTreeRoot()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*zond.BlindedBeaconBlockBellatrix).HashTreeRoot()
		}
		return pb.(*zond.BeaconBlockBellatrix).HashTreeRoot()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*zond.BlindedBeaconBlockCapella).HashTreeRoot()
		}
		return pb.(*zond.BeaconBlockCapella).HashTreeRoot()
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
	case version.Phase0:
		return pb.(*zond.BeaconBlock).HashTreeRootWith(h)
	case version.Altair:
		return pb.(*zond.BeaconBlockAltair).HashTreeRootWith(h)
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*zond.BlindedBeaconBlockBellatrix).HashTreeRootWith(h)
		}
		return pb.(*zond.BeaconBlockBellatrix).HashTreeRootWith(h)
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*zond.BlindedBeaconBlockCapella).HashTreeRootWith(h)
		}
		return pb.(*zond.BeaconBlockCapella).HashTreeRootWith(h)
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
	case version.Phase0:
		return pb.(*zond.BeaconBlock).MarshalSSZ()
	case version.Altair:
		return pb.(*zond.BeaconBlockAltair).MarshalSSZ()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*zond.BlindedBeaconBlockBellatrix).MarshalSSZ()
		}
		return pb.(*zond.BeaconBlockBellatrix).MarshalSSZ()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*zond.BlindedBeaconBlockCapella).MarshalSSZ()
		}
		return pb.(*zond.BeaconBlockCapella).MarshalSSZ()
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
	case version.Phase0:
		return pb.(*zond.BeaconBlock).MarshalSSZTo(dst)
	case version.Altair:
		return pb.(*zond.BeaconBlockAltair).MarshalSSZTo(dst)
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*zond.BlindedBeaconBlockBellatrix).MarshalSSZTo(dst)
		}
		return pb.(*zond.BeaconBlockBellatrix).MarshalSSZTo(dst)
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*zond.BlindedBeaconBlockCapella).MarshalSSZTo(dst)
		}
		return pb.(*zond.BeaconBlockCapella).MarshalSSZTo(dst)
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
		panic(err)
	}
	switch b.version {
	case version.Phase0:
		return pb.(*zond.BeaconBlock).SizeSSZ()
	case version.Altair:
		return pb.(*zond.BeaconBlockAltair).SizeSSZ()
	case version.Bellatrix:
		if b.IsBlinded() {
			return pb.(*zond.BlindedBeaconBlockBellatrix).SizeSSZ()
		}
		return pb.(*zond.BeaconBlockBellatrix).SizeSSZ()
	case version.Capella:
		if b.IsBlinded() {
			return pb.(*zond.BlindedBeaconBlockCapella).SizeSSZ()
		}
		return pb.(*zond.BeaconBlockCapella).SizeSSZ()
	default:
		panic(incorrectBodyVersion)
	}
}

// UnmarshalSSZ unmarshals the beacon block from its relevant ssz form.
func (b *BeaconBlock) UnmarshalSSZ(buf []byte) error {
	var newBlock *BeaconBlock
	switch b.version {
	case version.Phase0:
		pb := &zond.BeaconBlock{}
		if err := pb.UnmarshalSSZ(buf); err != nil {
			return err
		}
		var err error
		newBlock, err = initBlockFromProtoPhase0(pb)
		if err != nil {
			return err
		}
	case version.Altair:
		pb := &zond.BeaconBlockAltair{}
		if err := pb.UnmarshalSSZ(buf); err != nil {
			return err
		}
		var err error
		newBlock, err = initBlockFromProtoAltair(pb)
		if err != nil {
			return err
		}
	case version.Bellatrix:
		if b.IsBlinded() {
			pb := &zond.BlindedBeaconBlockBellatrix{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedBlockFromProtoBellatrix(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &zond.BeaconBlockBellatrix{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlockFromProtoBellatrix(pb)
			if err != nil {
				return err
			}
		}
	case version.Capella:
		if b.IsBlinded() {
			pb := &zond.BlindedBeaconBlockCapella{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlindedBlockFromProtoCapella(pb)
			if err != nil {
				return err
			}
		} else {
			pb := &zond.BeaconBlockCapella{}
			if err := pb.UnmarshalSSZ(buf); err != nil {
				return err
			}
			var err error
			newBlock, err = initBlockFromProtoCapella(pb)
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
	case version.Phase0:
		return &validatorpb.SignRequest_Block{Block: pb.(*zond.BeaconBlock)}, nil
	case version.Altair:
		return &validatorpb.SignRequest_BlockAltair{BlockAltair: pb.(*zond.BeaconBlockAltair)}, nil
	case version.Bellatrix:
		if b.IsBlinded() {
			return &validatorpb.SignRequest_BlindedBlockBellatrix{BlindedBlockBellatrix: pb.(*zond.BlindedBeaconBlockBellatrix)}, nil
		}
		return &validatorpb.SignRequest_BlockBellatrix{BlockBellatrix: pb.(*zond.BeaconBlockBellatrix)}, nil
	case version.Capella:
		if b.IsBlinded() {
			return &validatorpb.SignRequest_BlindedBlockCapella{BlindedBlockCapella: pb.(*zond.BlindedBeaconBlockCapella)}, nil
		}
		return &validatorpb.SignRequest_BlockCapella{BlockCapella: pb.(*zond.BeaconBlockCapella)}, nil
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
	case version.Phase0:
		cp := zond.CopyBeaconBlock(pb.(*zond.BeaconBlock))
		return initBlockFromProtoPhase0(cp)
	case version.Altair:
		cp := zond.CopyBeaconBlockAltair(pb.(*zond.BeaconBlockAltair))
		return initBlockFromProtoAltair(cp)
	case version.Bellatrix:
		if b.IsBlinded() {
			cp := zond.CopyBlindedBeaconBlockBellatrix(pb.(*zond.BlindedBeaconBlockBellatrix))
			return initBlindedBlockFromProtoBellatrix(cp)
		}
		cp := zond.CopyBeaconBlockBellatrix(pb.(*zond.BeaconBlockBellatrix))
		return initBlockFromProtoBellatrix(cp)
	case version.Capella:
		if b.IsBlinded() {
			cp := zond.CopyBlindedBeaconBlockCapella(pb.(*zond.BlindedBeaconBlockCapella))
			return initBlindedBlockFromProtoCapella(cp)
		}
		cp := zond.CopyBeaconBlockCapella(pb.(*zond.BeaconBlockCapella))
		return initBlockFromProtoCapella(cp)
	default:
		return nil, errIncorrectBlockVersion
	}
}

// IsNil checks if the block body is nil.
func (b *BeaconBlockBody) IsNil() bool {
	return b == nil
}

// RandaoReveal returns the randao reveal from the block body.
func (b *BeaconBlockBody) RandaoReveal() [dilithium2.CryptoBytes]byte {
	return b.randaoReveal
}

// Eth1Data returns the eth1 data in the block.
func (b *BeaconBlockBody) Eth1Data() *zond.Eth1Data {
	return b.eth1Data
}

// Graffiti returns the graffiti in the block.
func (b *BeaconBlockBody) Graffiti() [field_params.RootLength]byte {
	return b.graffiti
}

// ProposerSlashings returns the proposer slashings in the block.
func (b *BeaconBlockBody) ProposerSlashings() []*zond.ProposerSlashing {
	return b.proposerSlashings
}

// AttesterSlashings returns the attester slashings in the block.
func (b *BeaconBlockBody) AttesterSlashings() []*zond.AttesterSlashing {
	return b.attesterSlashings
}

// Attestations returns the stored attestations in the block.
func (b *BeaconBlockBody) Attestations() []*zond.Attestation {
	return b.attestations
}

// Deposits returns the stored deposits in the block.
func (b *BeaconBlockBody) Deposits() []*zond.Deposit {
	return b.deposits
}

// VoluntaryExits returns the voluntary exits in the block.
func (b *BeaconBlockBody) VoluntaryExits() []*zond.SignedVoluntaryExit {
	return b.voluntaryExits
}

// SyncAggregate returns the sync aggregate in the block.
func (b *BeaconBlockBody) SyncAggregate() (*zond.SyncAggregate, error) {
	if b.version == version.Phase0 {
		return nil, consensus_types.ErrNotSupported("SyncAggregate", b.version)
	}
	return b.syncAggregate, nil
}

// Execution returns the execution payload of the block body.
func (b *BeaconBlockBody) Execution() (interfaces.ExecutionData, error) {
	switch b.version {
	case version.Phase0, version.Altair:
		return nil, consensus_types.ErrNotSupported("Execution", b.version)
	case version.Bellatrix:
		if b.isBlinded {
			var ph *enginev1.ExecutionPayloadHeader
			var ok bool
			if b.executionPayloadHeader != nil {
				ph, ok = b.executionPayloadHeader.Proto().(*enginev1.ExecutionPayloadHeader)
				if !ok {
					return nil, errPayloadHeaderWrongType
				}
			}
			return WrappedExecutionPayloadHeader(ph)
		}
		var p *enginev1.ExecutionPayload
		var ok bool
		if b.executionPayload != nil {
			p, ok = b.executionPayload.Proto().(*enginev1.ExecutionPayload)
			if !ok {
				return nil, errPayloadWrongType
			}
		}
		return WrappedExecutionPayload(p)
	case version.Capella:
		if b.isBlinded {
			var ph *enginev1.ExecutionPayloadHeaderCapella
			var ok bool
			if b.executionPayloadHeader != nil {
				ph, ok = b.executionPayloadHeader.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				if !ok {
					return nil, errPayloadHeaderWrongType
				}
				return WrappedExecutionPayloadHeaderCapella(ph, 0)
			}
		}
		var p *enginev1.ExecutionPayloadCapella
		var ok bool
		if b.executionPayload != nil {
			p, ok = b.executionPayload.Proto().(*enginev1.ExecutionPayloadCapella)
			if !ok {
				return nil, errPayloadWrongType
			}
		}
		return WrappedExecutionPayloadCapella(p, 0)
	default:
		return nil, errIncorrectBlockVersion
	}
}

func (b *BeaconBlockBody) DilithiumToExecutionChanges() ([]*zond.SignedDilithiumToExecutionChange, error) {
	if b.version < version.Capella {
		return nil, consensus_types.ErrNotSupported("DilithiumToExecutionChanges", b.version)
	}
	return b.dilithiumToExecutionChanges, nil
}

// HashTreeRoot returns the ssz root of the block body.
func (b *BeaconBlockBody) HashTreeRoot() ([field_params.RootLength]byte, error) {
	pb, err := b.Proto()
	if err != nil {
		return [field_params.RootLength]byte{}, err
	}
	switch b.version {
	case version.Phase0:
		return pb.(*zond.BeaconBlockBody).HashTreeRoot()
	case version.Altair:
		return pb.(*zond.BeaconBlockBodyAltair).HashTreeRoot()
	case version.Bellatrix:
		if b.isBlinded {
			return pb.(*zond.BlindedBeaconBlockBodyBellatrix).HashTreeRoot()
		}
		return pb.(*zond.BeaconBlockBodyBellatrix).HashTreeRoot()
	case version.Capella:
		if b.isBlinded {
			return pb.(*zond.BlindedBeaconBlockBodyCapella).HashTreeRoot()
		}
		return pb.(*zond.BeaconBlockBodyCapella).HashTreeRoot()
	default:
		return [field_params.RootLength]byte{}, errIncorrectBodyVersion
	}
}
