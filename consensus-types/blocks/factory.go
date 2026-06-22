package blocks

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
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
func NewSignedBeaconBlock(i any) (interfaces.SignedBeaconBlock, error) {
	switch b := i.(type) {
	case nil:
		return nil, ErrNilObject
	case *qrysmpb.GenericSignedBeaconBlock_Zond:
		return initSignedBlockFromProtoZond(b.Zond)
	case *qrysmpb.SignedBeaconBlockZond:
		return initSignedBlockFromProtoZond(b)
	case *qrysmpb.GenericSignedBeaconBlock_BlindedZond:
		return initBlindedSignedBlockFromProtoZond(b.BlindedZond)
	case *qrysmpb.SignedBlindedBeaconBlockZond:
		return initBlindedSignedBlockFromProtoZond(b)
	default:
		return nil, errors.Wrapf(ErrUnsupportedSignedBeaconBlock, "unable to create block from type %T", i)
	}
}

// NewBeaconBlock creates a beacon block from a protobuf beacon block.
func NewBeaconBlock(i any) (interfaces.ReadOnlyBeaconBlock, error) {
	switch b := i.(type) {
	case nil:
		return nil, ErrNilObject
	case *qrysmpb.GenericBeaconBlock_Zond:
		return initBlockFromProtoZond(b.Zond)
	case *qrysmpb.BeaconBlockZond:
		return initBlockFromProtoZond(b)
	case *qrysmpb.GenericBeaconBlock_BlindedZond:
		return initBlindedBlockFromProtoZond(b.BlindedZond)
	case *qrysmpb.BlindedBeaconBlockZond:
		return initBlindedBlockFromProtoZond(b)
	default:
		return nil, errors.Wrapf(errUnsupportedBeaconBlock, "unable to create block from type %T", i)
	}
}

// NewBeaconBlockBody creates a beacon block body from a protobuf beacon block body.
func NewBeaconBlockBody(i any) (interfaces.ReadOnlyBeaconBlockBody, error) {
	switch b := i.(type) {
	case nil:
		return nil, ErrNilObject
	case *qrysmpb.BeaconBlockBodyZond:
		return initBlockBodyFromProtoZond(b)
	case *qrysmpb.BlindedBeaconBlockBodyZond:
		return initBlindedBlockBodyFromProtoZond(b)
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
	case version.Zond:
		if blk.IsBlinded() {
			pb, ok := pb.(*qrysmpb.BlindedBeaconBlockZond)
			if !ok {
				return nil, errIncorrectBlockVersion
			}
			return NewSignedBeaconBlock(&qrysmpb.SignedBlindedBeaconBlockZond{Block: pb, Signature: signature})
		}
		pb, ok := pb.(*qrysmpb.BeaconBlockZond)
		if !ok {
			return nil, errIncorrectBlockVersion
		}
		return NewSignedBeaconBlock(&qrysmpb.SignedBeaconBlockZond{Block: pb, Signature: signature})
	default:
		return nil, errUnsupportedBeaconBlock
	}
}

// BuildSignedBeaconBlockFromExecutionPayload takes a signed, blinded beacon block and converts into
// a full, signed beacon block by specifying an execution payload.
func BuildSignedBeaconBlockFromExecutionPayload(
	blk interfaces.ReadOnlySignedBeaconBlock, payload any,
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
	case *enginev1.ExecutionPayloadZond:
		wrappedPayload, wrapErr = WrappedExecutionPayloadZond(p, 0)
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

	var fullBlock any
	switch p := payload.(type) {
	case *enginev1.ExecutionPayloadZond:
		fullBlock = &qrysmpb.SignedBeaconBlockZond{
			Block: &qrysmpb.BeaconBlockZond{
				Slot:          b.Slot(),
				ProposerIndex: b.ProposerIndex(),
				ParentRoot:    parentRoot[:],
				StateRoot:     stateRoot[:],
				Body: &qrysmpb.BeaconBlockBodyZond{
					RandaoReveal:      randaoReveal[:],
					ExecutionData:     b.Body().ExecutionData(),
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
	default:
		return nil, fmt.Errorf("%T is not a type of execution payload", p)
	}

	return NewSignedBeaconBlock(fullBlock)
}

// BeaconBlockContainerToSignedBeaconBlock converts BeaconBlockContainer (API response) to a SignedBeaconBlock.
// This is particularly useful for using the values from API calls.
func BeaconBlockContainerToSignedBeaconBlock(obj *qrysmpb.BeaconBlockContainer) (interfaces.ReadOnlySignedBeaconBlock, error) {
	switch obj.Block.(type) {
	case *qrysmpb.BeaconBlockContainer_BlindedZondBlock:
		return NewSignedBeaconBlock(obj.GetBlindedZondBlock())
	case *qrysmpb.BeaconBlockContainer_ZondBlock:
		return NewSignedBeaconBlock(obj.GetZondBlock())
	default:
		return nil, errors.New("container block type not recognized")
	}
}
