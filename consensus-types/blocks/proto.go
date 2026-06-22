package blocks

import (
	"github.com/pkg/errors"
	consensus_types "github.com/theQRL/qrysm/consensus-types"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"google.golang.org/protobuf/proto"
)

// Proto converts the signed beacon block to a protobuf object.
func (b *SignedBeaconBlock) Proto() (proto.Message, error) {
	if b == nil {
		return nil, errNilBlock
	}

	blockMessage, err := b.block.Proto()
	if err != nil {
		return nil, err
	}

	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			var block *qrysmpb.BlindedBeaconBlockZond
			if blockMessage != nil {
				var ok bool
				block, ok = blockMessage.(*qrysmpb.BlindedBeaconBlockZond)
				if !ok {
					return nil, errIncorrectBlockVersion
				}
			}
			return &qrysmpb.SignedBlindedBeaconBlockZond{
				Block:     block,
				Signature: b.signature[:],
			}, nil
		}
		var block *qrysmpb.BeaconBlockZond
		if blockMessage != nil {
			var ok bool
			block, ok = blockMessage.(*qrysmpb.BeaconBlockZond)
			if !ok {
				return nil, errIncorrectBlockVersion
			}
		}
		return &qrysmpb.SignedBeaconBlockZond{
			Block:     block,
			Signature: b.signature[:],
		}, nil
	default:
		return nil, errors.New("unsupported signed beacon block version")
	}
}

// Proto converts the beacon block to a protobuf object.
func (b *BeaconBlock) Proto() (proto.Message, error) {
	if b == nil {
		return nil, nil
	}

	bodyMessage, err := b.body.Proto()
	if err != nil {
		return nil, err
	}

	switch b.version {
	case version.Zond:
		if b.IsBlinded() {
			var body *qrysmpb.BlindedBeaconBlockBodyZond
			if bodyMessage != nil {
				var ok bool
				body, ok = bodyMessage.(*qrysmpb.BlindedBeaconBlockBodyZond)
				if !ok {
					return nil, errIncorrectBodyVersion
				}
			}
			return &qrysmpb.BlindedBeaconBlockZond{
				Slot:          b.slot,
				ProposerIndex: b.proposerIndex,
				ParentRoot:    b.parentRoot[:],
				StateRoot:     b.stateRoot[:],
				Body:          body,
			}, nil
		}
		var body *qrysmpb.BeaconBlockBodyZond
		if bodyMessage != nil {
			var ok bool
			body, ok = bodyMessage.(*qrysmpb.BeaconBlockBodyZond)
			if !ok {
				return nil, errIncorrectBodyVersion
			}
		}
		return &qrysmpb.BeaconBlockZond{
			Slot:          b.slot,
			ProposerIndex: b.proposerIndex,
			ParentRoot:    b.parentRoot[:],
			StateRoot:     b.stateRoot[:],
			Body:          body,
		}, nil
	default:
		return nil, errors.New("unsupported beacon block version")
	}
}

// Proto converts the beacon block body to a protobuf object.
func (b *BeaconBlockBody) Proto() (proto.Message, error) {
	if b == nil {
		return nil, nil
	}

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
			}
			return &qrysmpb.BlindedBeaconBlockBodyZond{
				RandaoReveal:           b.randaoReveal[:],
				ExecutionData:          b.executionData,
				Graffiti:               b.graffiti[:],
				ProposerSlashings:      b.proposerSlashings,
				AttesterSlashings:      b.attesterSlashings,
				Attestations:           b.attestations,
				Deposits:               b.deposits,
				VoluntaryExits:         b.voluntaryExits,
				SyncAggregate:          b.syncAggregate,
				ExecutionPayloadHeader: ph,
			}, nil
		}
		var p *enginev1.ExecutionPayloadZond
		var ok bool
		if b.executionPayload != nil {
			p, ok = b.executionPayload.Proto().(*enginev1.ExecutionPayloadZond)
			if !ok {
				return nil, errPayloadWrongType
			}
		}
		return &qrysmpb.BeaconBlockBodyZond{
			RandaoReveal:      b.randaoReveal[:],
			ExecutionData:     b.executionData,
			Graffiti:          b.graffiti[:],
			ProposerSlashings: b.proposerSlashings,
			AttesterSlashings: b.attesterSlashings,
			Attestations:      b.attestations,
			Deposits:          b.deposits,
			VoluntaryExits:    b.voluntaryExits,
			SyncAggregate:     b.syncAggregate,
			ExecutionPayload:  p,
		}, nil
	default:
		return nil, errors.New("unsupported beacon block body version")
	}
}

func initSignedBlockFromProtoZond(pb *qrysmpb.SignedBeaconBlockZond) (*SignedBeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	block, err := initBlockFromProtoZond(pb.Block)
	if err != nil {
		return nil, err
	}
	b := &SignedBeaconBlock{
		version:   version.Zond,
		block:     block,
		signature: bytesutil.ToBytes4627(pb.Signature),
	}
	return b, nil
}

func initBlindedSignedBlockFromProtoZond(pb *qrysmpb.SignedBlindedBeaconBlockZond) (*SignedBeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	block, err := initBlindedBlockFromProtoZond(pb.Block)
	if err != nil {
		return nil, err
	}
	b := &SignedBeaconBlock{
		version:   version.Zond,
		block:     block,
		signature: bytesutil.ToBytes4627(pb.Signature),
	}
	return b, nil
}

func initBlockFromProtoZond(pb *qrysmpb.BeaconBlockZond) (*BeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	body, err := initBlockBodyFromProtoZond(pb.Body)
	if err != nil {
		return nil, err
	}
	b := &BeaconBlock{
		version:       version.Zond,
		slot:          pb.Slot,
		proposerIndex: pb.ProposerIndex,
		parentRoot:    bytesutil.ToBytes32(pb.ParentRoot),
		stateRoot:     bytesutil.ToBytes32(pb.StateRoot),
		body:          body,
	}
	return b, nil
}

func initBlindedBlockFromProtoZond(pb *qrysmpb.BlindedBeaconBlockZond) (*BeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	body, err := initBlindedBlockBodyFromProtoZond(pb.Body)
	if err != nil {
		return nil, err
	}
	b := &BeaconBlock{
		version:       version.Zond,
		slot:          pb.Slot,
		proposerIndex: pb.ProposerIndex,
		parentRoot:    bytesutil.ToBytes32(pb.ParentRoot),
		stateRoot:     bytesutil.ToBytes32(pb.StateRoot),
		body:          body,
	}
	return b, nil
}

func initBlockBodyFromProtoZond(pb *qrysmpb.BeaconBlockBodyZond) (*BeaconBlockBody, error) {
	if pb == nil {
		return nil, errNilBlockBody
	}

	p, err := WrappedExecutionPayloadZond(pb.ExecutionPayload, 0)
	// We allow the payload to be nil
	if err != nil && err != consensus_types.ErrNilObjectWrapped {
		return nil, err
	}
	b := &BeaconBlockBody{
		version:           version.Zond,
		isBlinded:         false,
		randaoReveal:      bytesutil.ToBytes4627(pb.RandaoReveal),
		executionData:     pb.ExecutionData,
		graffiti:          bytesutil.ToBytes32(pb.Graffiti),
		proposerSlashings: pb.ProposerSlashings,
		attesterSlashings: pb.AttesterSlashings,
		attestations:      pb.Attestations,
		deposits:          pb.Deposits,
		voluntaryExits:    pb.VoluntaryExits,
		syncAggregate:     pb.SyncAggregate,
		executionPayload:  p,
	}
	return b, nil
}

func initBlindedBlockBodyFromProtoZond(pb *qrysmpb.BlindedBeaconBlockBodyZond) (*BeaconBlockBody, error) {
	if pb == nil {
		return nil, errNilBlockBody
	}

	ph, err := WrappedExecutionPayloadHeaderZond(pb.ExecutionPayloadHeader, 0)
	// We allow the payload to be nil
	if err != nil && err != consensus_types.ErrNilObjectWrapped {
		return nil, err
	}
	b := &BeaconBlockBody{
		version:                version.Zond,
		isBlinded:              true,
		randaoReveal:           bytesutil.ToBytes4627(pb.RandaoReveal),
		executionData:          pb.ExecutionData,
		graffiti:               bytesutil.ToBytes32(pb.Graffiti),
		proposerSlashings:      pb.ProposerSlashings,
		attesterSlashings:      pb.AttesterSlashings,
		attestations:           pb.Attestations,
		deposits:               pb.Deposits,
		voluntaryExits:         pb.VoluntaryExits,
		syncAggregate:          pb.SyncAggregate,
		executionPayloadHeader: ph,
	}
	return b, nil
}
