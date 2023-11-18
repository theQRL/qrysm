package blocks

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
)

var (
	// ErrUnsupportedSignedBeaconBlock is returned when the struct type is not a supported signed
	// beacon block type.
	ErrUnsupportedSignedBeaconBlock = errors.New("unsupported signed beacon block")
	// errUnsupportedBeaconBlock is returned when the struct type is not a supported beacon block
	// type.
	errUnsupportedBeaconBlock = errors.New("unsupported beacon block")
	// errUnsupportedBeaconBlockBody is returned when the struct type is not a supported beacon block body
	// type.
	errUnsupportedBeaconBlockBody = errors.New("unsupported beacon block body")
	// ErrNilObject is returned in a constructor when the underlying object is nil.
	ErrNilObject = errors.New("received nil object")
	// ErrNilSignedBeaconBlock is returned when a nil signed beacon block is received.
	ErrNilSignedBeaconBlock        = errors.New("signed beacon block can't be nil")
	errNonBlindedSignedBeaconBlock = errors.New("can only build signed beacon block from blinded format")
)

// NewSignedBeaconBlock creates a signed beacon block from a protobuf signed beacon block.
func NewSignedBeaconBlock(i interface{}) (interfaces.SignedBeaconBlock, error) {
	switch b := i.(type) {
	case nil:
		return nil, ErrNilObject
	case *zond.GenericSignedBeaconBlock_Phase0:
		return initSignedBlockFromProtoPhase0(b.Phase0)
	case *zond.SignedBeaconBlock:
		return initSignedBlockFromProtoPhase0(b)
	case *zond.GenericSignedBeaconBlock_Altair:
		return initSignedBlockFromProtoAltair(b.Altair)
	case *zond.SignedBeaconBlockAltair:
		return initSignedBlockFromProtoAltair(b)
	case *zond.GenericSignedBeaconBlock_Bellatrix:
		return initSignedBlockFromProtoBellatrix(b.Bellatrix)
	case *zond.SignedBeaconBlockBellatrix:
		return initSignedBlockFromProtoBellatrix(b)
	case *zond.GenericSignedBeaconBlock_BlindedBellatrix:
		return initBlindedSignedBlockFromProtoBellatrix(b.BlindedBellatrix)
	case *zond.SignedBlindedBeaconBlockBellatrix:
		return initBlindedSignedBlockFromProtoBellatrix(b)
	case *zond.GenericSignedBeaconBlock_Capella:
		return initSignedBlockFromProtoCapella(b.Capella)
	case *zond.SignedBeaconBlockCapella:
		return initSignedBlockFromProtoCapella(b)
	case *zond.GenericSignedBeaconBlock_BlindedCapella:
		return initBlindedSignedBlockFromProtoCapella(b.BlindedCapella)
	case *zond.SignedBlindedBeaconBlockCapella:
		return initBlindedSignedBlockFromProtoCapella(b)
	case *zond.GenericSignedBeaconBlock_Deneb:
		return initSignedBlockFromProtoDeneb(b.Deneb.Block)
	case *zond.SignedBeaconBlockDeneb:
		return initSignedBlockFromProtoDeneb(b)
	case *zond.SignedBlindedBeaconBlockDeneb:
		return initBlindedSignedBlockFromProtoDeneb(b)
	case *zond.GenericSignedBeaconBlock_BlindedDeneb:
		return initBlindedSignedBlockFromProtoDeneb(b.BlindedDeneb.SignedBlindedBlock)
	default:
		return nil, errors.Wrapf(ErrUnsupportedSignedBeaconBlock, "unable to create block from type %T", i)
	}
}

// NewBeaconBlock creates a beacon block from a protobuf beacon block.
func NewBeaconBlock(i interface{}) (interfaces.ReadOnlyBeaconBlock, error) {
	switch b := i.(type) {
	case nil:
		return nil, ErrNilObject
	case *zond.GenericBeaconBlock_Phase0:
		return initBlockFromProtoPhase0(b.Phase0)
	case *zond.BeaconBlock:
		return initBlockFromProtoPhase0(b)
	case *zond.GenericBeaconBlock_Altair:
		return initBlockFromProtoAltair(b.Altair)
	case *zond.BeaconBlockAltair:
		return initBlockFromProtoAltair(b)
	case *zond.GenericBeaconBlock_Bellatrix:
		return initBlockFromProtoBellatrix(b.Bellatrix)
	case *zond.BeaconBlockBellatrix:
		return initBlockFromProtoBellatrix(b)
	case *zond.GenericBeaconBlock_BlindedBellatrix:
		return initBlindedBlockFromProtoBellatrix(b.BlindedBellatrix)
	case *zond.BlindedBeaconBlockBellatrix:
		return initBlindedBlockFromProtoBellatrix(b)
	case *zond.GenericBeaconBlock_Capella:
		return initBlockFromProtoCapella(b.Capella)
	case *zond.BeaconBlockCapella:
		return initBlockFromProtoCapella(b)
	case *zond.GenericBeaconBlock_BlindedCapella:
		return initBlindedBlockFromProtoCapella(b.BlindedCapella)
	case *zond.BlindedBeaconBlockCapella:
		return initBlindedBlockFromProtoCapella(b)
	case *zond.GenericBeaconBlock_Deneb:
		return initBlockFromProtoDeneb(b.Deneb.Block)
	case *zond.BeaconBlockDeneb:
		return initBlockFromProtoDeneb(b)
	case *zond.BlindedBeaconBlockDeneb:
		return initBlindedBlockFromProtoDeneb(b)
	case *zond.GenericBeaconBlock_BlindedDeneb:
		return initBlindedBlockFromProtoDeneb(b.BlindedDeneb.Block)
	default:
		return nil, errors.Wrapf(errUnsupportedBeaconBlock, "unable to create block from type %T", i)
	}
}

// NewBeaconBlockBody creates a beacon block body from a protobuf beacon block body.
func NewBeaconBlockBody(i interface{}) (interfaces.ReadOnlyBeaconBlockBody, error) {
	switch b := i.(type) {
	case nil:
		return nil, ErrNilObject
	case *zond.BeaconBlockBody:
		return initBlockBodyFromProtoPhase0(b)
	case *zond.BeaconBlockBodyAltair:
		return initBlockBodyFromProtoAltair(b)
	case *zond.BeaconBlockBodyBellatrix:
		return initBlockBodyFromProtoBellatrix(b)
	case *zond.BlindedBeaconBlockBodyBellatrix:
		return initBlindedBlockBodyFromProtoBellatrix(b)
	case *zond.BeaconBlockBodyCapella:
		return initBlockBodyFromProtoCapella(b)
	case *zond.BlindedBeaconBlockBodyCapella:
		return initBlindedBlockBodyFromProtoCapella(b)
	case *zond.BeaconBlockBodyDeneb:
		return initBlockBodyFromProtoDeneb(b)
	case *zond.BlindedBeaconBlockBodyDeneb:
		return initBlindedBlockBodyFromProtoDeneb(b)
	default:
		return nil, errors.Wrapf(errUnsupportedBeaconBlockBody, "unable to create block body from type %T", i)
	}
}

// BuildSignedBeaconBlock assembles a block.ReadOnlySignedBeaconBlock interface compatible struct from a
// given beacon block and the appropriate signature. This method may be used to easily create a
// signed beacon block.
func BuildSignedBeaconBlock(blk interfaces.ReadOnlyBeaconBlock, signature []byte) (interfaces.SignedBeaconBlock, error) {
	pb, err := blk.Proto()
	if err != nil {
		return nil, err
	}

	switch blk.Version() {
	case version.Phase0:
		pb, ok := pb.(*zond.BeaconBlock)
		if !ok {
			return nil, errIncorrectBlockVersion
		}
		return NewSignedBeaconBlock(&zond.SignedBeaconBlock{Block: pb, Signature: signature})
	case version.Altair:
		pb, ok := pb.(*zond.BeaconBlockAltair)
		if !ok {
			return nil, errIncorrectBlockVersion
		}
		return NewSignedBeaconBlock(&zond.SignedBeaconBlockAltair{Block: pb, Signature: signature})
	case version.Bellatrix:
		if blk.IsBlinded() {
			pb, ok := pb.(*zond.BlindedBeaconBlockBellatrix)
			if !ok {
				return nil, errIncorrectBlockVersion
			}
			return NewSignedBeaconBlock(&zond.SignedBlindedBeaconBlockBellatrix{Block: pb, Signature: signature})
		}
		pb, ok := pb.(*zond.BeaconBlockBellatrix)
		if !ok {
			return nil, errIncorrectBlockVersion
		}
		return NewSignedBeaconBlock(&zond.SignedBeaconBlockBellatrix{Block: pb, Signature: signature})
	case version.Capella:
		if blk.IsBlinded() {
			pb, ok := pb.(*zond.BlindedBeaconBlockCapella)
			if !ok {
				return nil, errIncorrectBlockVersion
			}
			return NewSignedBeaconBlock(&zond.SignedBlindedBeaconBlockCapella{Block: pb, Signature: signature})
		}
		pb, ok := pb.(*zond.BeaconBlockCapella)
		if !ok {
			return nil, errIncorrectBlockVersion
		}
		return NewSignedBeaconBlock(&zond.SignedBeaconBlockCapella{Block: pb, Signature: signature})
	case version.Deneb:
		if blk.IsBlinded() {
			pb, ok := pb.(*zond.BlindedBeaconBlockDeneb)
			if !ok {
				return nil, errIncorrectBlockVersion
			}
			return NewSignedBeaconBlock(&zond.SignedBlindedBeaconBlockDeneb{Message: pb, Signature: signature})
		}
		pb, ok := pb.(*zond.BeaconBlockDeneb)
		if !ok {
			return nil, errIncorrectBlockVersion
		}
		return NewSignedBeaconBlock(&zond.SignedBeaconBlockDeneb{Block: pb, Signature: signature})
	default:
		return nil, errUnsupportedBeaconBlock
	}
}

// BuildSignedBeaconBlockFromExecutionPayload takes a signed, blinded beacon block and converts into
// a full, signed beacon block by specifying an execution payload.
func BuildSignedBeaconBlockFromExecutionPayload(
	blk interfaces.ReadOnlySignedBeaconBlock, payload interface{},
) (interfaces.SignedBeaconBlock, error) {
	if err := BeaconBlockIsNil(blk); err != nil {
		return nil, err
	}
	if !blk.IsBlinded() {
		return nil, errNonBlindedSignedBeaconBlock
	}
	b := blk.Block()
	payloadHeader, err := b.Body().Execution()
	if err != nil {
		return nil, errors.Wrap(err, "could not get execution payload header")
	}

	var wrappedPayload interfaces.ExecutionData
	var wrapErr error
	switch p := payload.(type) {
	case *enginev1.ExecutionPayload:
		wrappedPayload, wrapErr = WrappedExecutionPayload(p)
	case *enginev1.ExecutionPayloadCapella:
		wrappedPayload, wrapErr = WrappedExecutionPayloadCapella(p, 0)
	case *enginev1.ExecutionPayloadDeneb:
		wrappedPayload, wrapErr = WrappedExecutionPayloadDeneb(p, 0)
	default:
		return nil, fmt.Errorf("%T is not a type of execution payload", p)
	}
	if wrapErr != nil {
		return nil, wrapErr
	}
	empty, err := IsEmptyExecutionData(wrappedPayload)
	if err != nil {
		return nil, err
	}
	if !empty {
		payloadRoot, err := wrappedPayload.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "could not hash tree root execution payload")
		}
		payloadHeaderRoot, err := payloadHeader.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "could not hash tree root payload header")
		}
		if payloadRoot != payloadHeaderRoot {
			return nil, fmt.Errorf(
				"payload %#x and header %#x roots do not match",
				payloadRoot,
				payloadHeaderRoot,
			)
		}
	}
	syncAgg, err := b.Body().SyncAggregate()
	if err != nil {
		return nil, errors.Wrap(err, "could not get sync aggregate from block body")
	}
	parentRoot := b.ParentRoot()
	stateRoot := b.StateRoot()
	randaoReveal := b.Body().RandaoReveal()
	graffiti := b.Body().Graffiti()
	sig := blk.Signature()

	var fullBlock interface{}
	switch p := payload.(type) {
	case *enginev1.ExecutionPayload:
		fullBlock = &zond.SignedBeaconBlockBellatrix{
			Block: &zond.BeaconBlockBellatrix{
				Slot:          b.Slot(),
				ProposerIndex: b.ProposerIndex(),
				ParentRoot:    parentRoot[:],
				StateRoot:     stateRoot[:],
				Body: &zond.BeaconBlockBodyBellatrix{
					RandaoReveal:      randaoReveal[:],
					Eth1Data:          b.Body().Eth1Data(),
					Graffiti:          graffiti[:],
					ProposerSlashings: b.Body().ProposerSlashings(),
					AttesterSlashings: b.Body().AttesterSlashings(),
					Attestations:      b.Body().Attestations(),
					Deposits:          b.Body().Deposits(),
					VoluntaryExits:    b.Body().VoluntaryExits(),
					SyncAggregate:     syncAgg,
					ExecutionPayload:  p,
				},
			},
			Signature: sig[:],
		}
	case *enginev1.ExecutionPayloadCapella:
		dilithiumToExecutionChanges, err := b.Body().DilithiumToExecutionChanges()
		if err != nil {
			return nil, err
		}
		fullBlock = &zond.SignedBeaconBlockCapella{
			Block: &zond.BeaconBlockCapella{
				Slot:          b.Slot(),
				ProposerIndex: b.ProposerIndex(),
				ParentRoot:    parentRoot[:],
				StateRoot:     stateRoot[:],
				Body: &zond.BeaconBlockBodyCapella{
					RandaoReveal:                randaoReveal[:],
					Eth1Data:                    b.Body().Eth1Data(),
					Graffiti:                    graffiti[:],
					ProposerSlashings:           b.Body().ProposerSlashings(),
					AttesterSlashings:           b.Body().AttesterSlashings(),
					Attestations:                b.Body().Attestations(),
					Deposits:                    b.Body().Deposits(),
					VoluntaryExits:              b.Body().VoluntaryExits(),
					SyncAggregate:               syncAgg,
					ExecutionPayload:            p,
					DilithiumToExecutionChanges: dilithiumToExecutionChanges,
				},
			},
			Signature: sig[:],
		}
	case *enginev1.ExecutionPayloadDeneb:
		dilithiumToExecutionChanges, err := b.Body().DilithiumToExecutionChanges()
		if err != nil {
			return nil, err
		}
		commitments, err := b.Body().BlobKzgCommitments()
		if err != nil {
			return nil, err
		}
		fullBlock = &zond.SignedBeaconBlockDeneb{
			Block: &zond.BeaconBlockDeneb{
				Slot:          b.Slot(),
				ProposerIndex: b.ProposerIndex(),
				ParentRoot:    parentRoot[:],
				StateRoot:     stateRoot[:],
				Body: &zond.BeaconBlockBodyDeneb{
					RandaoReveal:                randaoReveal[:],
					Eth1Data:                    b.Body().Eth1Data(),
					Graffiti:                    graffiti[:],
					ProposerSlashings:           b.Body().ProposerSlashings(),
					AttesterSlashings:           b.Body().AttesterSlashings(),
					Attestations:                b.Body().Attestations(),
					Deposits:                    b.Body().Deposits(),
					VoluntaryExits:              b.Body().VoluntaryExits(),
					SyncAggregate:               syncAgg,
					ExecutionPayload:            p,
					DilithiumToExecutionChanges: dilithiumToExecutionChanges,
					BlobKzgCommitments:          commitments,
				},
			},
			Signature: sig[:],
		}
	default:
		return nil, fmt.Errorf("%T is not a type of execution payload", p)
	}

	return NewSignedBeaconBlock(fullBlock)
}

// BeaconBlockContainerToSignedBeaconBlock converts BeaconBlockContainer (API response) to a SignedBeaconBlock.
// This is particularly useful for using the values from API calls.
func BeaconBlockContainerToSignedBeaconBlock(obj *zond.BeaconBlockContainer) (interfaces.ReadOnlySignedBeaconBlock, error) {
	switch obj.Block.(type) {
	case *zond.BeaconBlockContainer_BlindedDenebBlock:
		return NewSignedBeaconBlock(obj.GetBlindedDenebBlock())
	case *zond.BeaconBlockContainer_DenebBlock:
		return NewSignedBeaconBlock(obj.GetDenebBlock())
	case *zond.BeaconBlockContainer_BlindedCapellaBlock:
		return NewSignedBeaconBlock(obj.GetBlindedCapellaBlock())
	case *zond.BeaconBlockContainer_CapellaBlock:
		return NewSignedBeaconBlock(obj.GetCapellaBlock())
	case *zond.BeaconBlockContainer_BlindedBellatrixBlock:
		return NewSignedBeaconBlock(obj.GetBlindedBellatrixBlock())
	case *zond.BeaconBlockContainer_BellatrixBlock:
		return NewSignedBeaconBlock(obj.GetBellatrixBlock())
	case *zond.BeaconBlockContainer_AltairBlock:
		return NewSignedBeaconBlock(obj.GetAltairBlock())
	case *zond.BeaconBlockContainer_Phase0Block:
		return NewSignedBeaconBlock(obj.GetPhase0Block())
	default:
		return nil, errors.New("container block type not recognized")
	}
}
