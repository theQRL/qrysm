package blocks

import (
	"github.com/pkg/errors"
	consensus_types "github.com/theQRL/qrysm/v4/consensus-types"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zond "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
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
	case version.Phase0:
		var block *zond.BeaconBlock
		if blockMessage != nil {
			var ok bool
			block, ok = blockMessage.(*zond.BeaconBlock)
			if !ok {
				return nil, errIncorrectBlockVersion
			}
		}
		return &zond.SignedBeaconBlock{
			Block:     block,
			Signature: b.signature[:],
		}, nil
	case version.Altair:
		var block *zond.BeaconBlockAltair
		if blockMessage != nil {
			var ok bool
			block, ok = blockMessage.(*zond.BeaconBlockAltair)
			if !ok {
				return nil, errIncorrectBlockVersion
			}
		}
		return &zond.SignedBeaconBlockAltair{
			Block:     block,
			Signature: b.signature[:],
		}, nil
	case version.Bellatrix:
		if b.IsBlinded() {
			var block *zond.BlindedBeaconBlockBellatrix
			if blockMessage != nil {
				var ok bool
				block, ok = blockMessage.(*zond.BlindedBeaconBlockBellatrix)
				if !ok {
					return nil, errIncorrectBlockVersion
				}
			}
			return &zond.SignedBlindedBeaconBlockBellatrix{
				Block:     block,
				Signature: b.signature[:],
			}, nil
		}
		var block *zond.BeaconBlockBellatrix
		if blockMessage != nil {
			var ok bool
			block, ok = blockMessage.(*zond.BeaconBlockBellatrix)
			if !ok {
				return nil, errIncorrectBlockVersion
			}
		}
		return &zond.SignedBeaconBlockBellatrix{
			Block:     block,
			Signature: b.signature[:],
		}, nil
	case version.Capella:
		if b.IsBlinded() {
			var block *zond.BlindedBeaconBlockCapella
			if blockMessage != nil {
				var ok bool
				block, ok = blockMessage.(*zond.BlindedBeaconBlockCapella)
				if !ok {
					return nil, errIncorrectBlockVersion
				}
			}
			return &zond.SignedBlindedBeaconBlockCapella{
				Block:     block,
				Signature: b.signature[:],
			}, nil
		}
		var block *zond.BeaconBlockCapella
		if blockMessage != nil {
			var ok bool
			block, ok = blockMessage.(*zond.BeaconBlockCapella)
			if !ok {
				return nil, errIncorrectBlockVersion
			}
		}
		return &zond.SignedBeaconBlockCapella{
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
	case version.Phase0:
		var body *zond.BeaconBlockBody
		if bodyMessage != nil {
			var ok bool
			body, ok = bodyMessage.(*zond.BeaconBlockBody)
			if !ok {
				return nil, errIncorrectBodyVersion
			}
		}
		return &zond.BeaconBlock{
			Slot:          b.slot,
			ProposerIndex: b.proposerIndex,
			ParentRoot:    b.parentRoot[:],
			StateRoot:     b.stateRoot[:],
			Body:          body,
		}, nil
	case version.Altair:
		var body *zond.BeaconBlockBodyAltair
		if bodyMessage != nil {
			var ok bool
			body, ok = bodyMessage.(*zond.BeaconBlockBodyAltair)
			if !ok {
				return nil, errIncorrectBodyVersion
			}
		}
		return &zond.BeaconBlockAltair{
			Slot:          b.slot,
			ProposerIndex: b.proposerIndex,
			ParentRoot:    b.parentRoot[:],
			StateRoot:     b.stateRoot[:],
			Body:          body,
		}, nil
	case version.Bellatrix:
		if b.IsBlinded() {
			var body *zond.BlindedBeaconBlockBodyBellatrix
			if bodyMessage != nil {
				var ok bool
				body, ok = bodyMessage.(*zond.BlindedBeaconBlockBodyBellatrix)
				if !ok {
					return nil, errIncorrectBodyVersion
				}
			}
			return &zond.BlindedBeaconBlockBellatrix{
				Slot:          b.slot,
				ProposerIndex: b.proposerIndex,
				ParentRoot:    b.parentRoot[:],
				StateRoot:     b.stateRoot[:],
				Body:          body,
			}, nil
		}
		var body *zond.BeaconBlockBodyBellatrix
		if bodyMessage != nil {
			var ok bool
			body, ok = bodyMessage.(*zond.BeaconBlockBodyBellatrix)
			if !ok {
				return nil, errIncorrectBodyVersion
			}
		}
		return &zond.BeaconBlockBellatrix{
			Slot:          b.slot,
			ProposerIndex: b.proposerIndex,
			ParentRoot:    b.parentRoot[:],
			StateRoot:     b.stateRoot[:],
			Body:          body,
		}, nil
	case version.Capella:
		if b.IsBlinded() {
			var body *zond.BlindedBeaconBlockBodyCapella
			if bodyMessage != nil {
				var ok bool
				body, ok = bodyMessage.(*zond.BlindedBeaconBlockBodyCapella)
				if !ok {
					return nil, errIncorrectBodyVersion
				}
			}
			return &zond.BlindedBeaconBlockCapella{
				Slot:          b.slot,
				ProposerIndex: b.proposerIndex,
				ParentRoot:    b.parentRoot[:],
				StateRoot:     b.stateRoot[:],
				Body:          body,
			}, nil
		}
		var body *zond.BeaconBlockBodyCapella
		if bodyMessage != nil {
			var ok bool
			body, ok = bodyMessage.(*zond.BeaconBlockBodyCapella)
			if !ok {
				return nil, errIncorrectBodyVersion
			}
		}
		return &zond.BeaconBlockCapella{
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
	case version.Phase0:
		return &zond.BeaconBlockBody{
			RandaoReveal:      b.randaoReveal[:],
			Eth1Data:          b.eth1Data,
			Graffiti:          b.graffiti[:],
			ProposerSlashings: b.proposerSlashings,
			AttesterSlashings: b.attesterSlashings,
			Attestations:      b.attestations,
			Deposits:          b.deposits,
			VoluntaryExits:    b.voluntaryExits,
		}, nil
	case version.Altair:
		return &zond.BeaconBlockBodyAltair{
			RandaoReveal:      b.randaoReveal[:],
			Eth1Data:          b.eth1Data,
			Graffiti:          b.graffiti[:],
			ProposerSlashings: b.proposerSlashings,
			AttesterSlashings: b.attesterSlashings,
			Attestations:      b.attestations,
			Deposits:          b.deposits,
			VoluntaryExits:    b.voluntaryExits,
			SyncAggregate:     b.syncAggregate,
		}, nil
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
			return &zond.BlindedBeaconBlockBodyBellatrix{
				RandaoReveal:           b.randaoReveal[:],
				Eth1Data:               b.eth1Data,
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
		var p *enginev1.ExecutionPayload
		var ok bool
		if b.executionPayload != nil {
			p, ok = b.executionPayload.Proto().(*enginev1.ExecutionPayload)
			if !ok {
				return nil, errPayloadWrongType
			}
		}
		return &zond.BeaconBlockBodyBellatrix{
			RandaoReveal:      b.randaoReveal[:],
			Eth1Data:          b.eth1Data,
			Graffiti:          b.graffiti[:],
			ProposerSlashings: b.proposerSlashings,
			AttesterSlashings: b.attesterSlashings,
			Attestations:      b.attestations,
			Deposits:          b.deposits,
			VoluntaryExits:    b.voluntaryExits,
			SyncAggregate:     b.syncAggregate,
			ExecutionPayload:  p,
		}, nil
	case version.Capella:
		if b.isBlinded {
			var ph *enginev1.ExecutionPayloadHeaderCapella
			var ok bool
			if b.executionPayloadHeader != nil {
				ph, ok = b.executionPayloadHeader.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
				if !ok {
					return nil, errPayloadHeaderWrongType
				}
			}
			return &zond.BlindedBeaconBlockBodyCapella{
				RandaoReveal:                b.randaoReveal[:],
				Eth1Data:                    b.eth1Data,
				Graffiti:                    b.graffiti[:],
				ProposerSlashings:           b.proposerSlashings,
				AttesterSlashings:           b.attesterSlashings,
				Attestations:                b.attestations,
				Deposits:                    b.deposits,
				VoluntaryExits:              b.voluntaryExits,
				SyncAggregate:               b.syncAggregate,
				ExecutionPayloadHeader:      ph,
				DilithiumToExecutionChanges: b.dilithiumToExecutionChanges,
			}, nil
		}
		var p *enginev1.ExecutionPayloadCapella
		var ok bool
		if b.executionPayload != nil {
			p, ok = b.executionPayload.Proto().(*enginev1.ExecutionPayloadCapella)
			if !ok {
				return nil, errPayloadWrongType
			}
		}
		return &zond.BeaconBlockBodyCapella{
			RandaoReveal:                b.randaoReveal[:],
			Eth1Data:                    b.eth1Data,
			Graffiti:                    b.graffiti[:],
			ProposerSlashings:           b.proposerSlashings,
			AttesterSlashings:           b.attesterSlashings,
			Attestations:                b.attestations,
			Deposits:                    b.deposits,
			VoluntaryExits:              b.voluntaryExits,
			SyncAggregate:               b.syncAggregate,
			ExecutionPayload:            p,
			DilithiumToExecutionChanges: b.dilithiumToExecutionChanges,
		}, nil
	default:
		return nil, errors.New("unsupported beacon block body version")
	}
}

func initSignedBlockFromProtoPhase0(pb *zond.SignedBeaconBlock) (*SignedBeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	block, err := initBlockFromProtoPhase0(pb.Block)
	if err != nil {
		return nil, err
	}
	b := &SignedBeaconBlock{
		version:   version.Phase0,
		block:     block,
		signature: bytesutil.ToBytes4595(pb.Signature),
	}
	return b, nil
}

func initSignedBlockFromProtoAltair(pb *zond.SignedBeaconBlockAltair) (*SignedBeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	block, err := initBlockFromProtoAltair(pb.Block)
	if err != nil {
		return nil, err
	}
	b := &SignedBeaconBlock{
		version:   version.Altair,
		block:     block,
		signature: bytesutil.ToBytes4595(pb.Signature),
	}
	return b, nil
}

func initSignedBlockFromProtoBellatrix(pb *zond.SignedBeaconBlockBellatrix) (*SignedBeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	block, err := initBlockFromProtoBellatrix(pb.Block)
	if err != nil {
		return nil, err
	}
	b := &SignedBeaconBlock{
		version:   version.Bellatrix,
		block:     block,
		signature: bytesutil.ToBytes4595(pb.Signature),
	}
	return b, nil
}

func initSignedBlockFromProtoCapella(pb *zond.SignedBeaconBlockCapella) (*SignedBeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	block, err := initBlockFromProtoCapella(pb.Block)
	if err != nil {
		return nil, err
	}
	b := &SignedBeaconBlock{
		version:   version.Capella,
		block:     block,
		signature: bytesutil.ToBytes4595(pb.Signature),
	}
	return b, nil
}

func initBlindedSignedBlockFromProtoBellatrix(pb *zond.SignedBlindedBeaconBlockBellatrix) (*SignedBeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	block, err := initBlindedBlockFromProtoBellatrix(pb.Block)
	if err != nil {
		return nil, err
	}
	b := &SignedBeaconBlock{
		version:   version.Bellatrix,
		block:     block,
		signature: bytesutil.ToBytes4595(pb.Signature),
	}
	return b, nil
}

func initBlindedSignedBlockFromProtoCapella(pb *zond.SignedBlindedBeaconBlockCapella) (*SignedBeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	block, err := initBlindedBlockFromProtoCapella(pb.Block)
	if err != nil {
		return nil, err
	}
	b := &SignedBeaconBlock{
		version:   version.Capella,
		block:     block,
		signature: bytesutil.ToBytes4595(pb.Signature),
	}
	return b, nil
}

func initBlockFromProtoPhase0(pb *zond.BeaconBlock) (*BeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	body, err := initBlockBodyFromProtoPhase0(pb.Body)
	if err != nil {
		return nil, err
	}
	b := &BeaconBlock{
		version:       version.Phase0,
		slot:          pb.Slot,
		proposerIndex: pb.ProposerIndex,
		parentRoot:    bytesutil.ToBytes32(pb.ParentRoot),
		stateRoot:     bytesutil.ToBytes32(pb.StateRoot),
		body:          body,
	}
	return b, nil
}

func initBlockFromProtoAltair(pb *zond.BeaconBlockAltair) (*BeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	body, err := initBlockBodyFromProtoAltair(pb.Body)
	if err != nil {
		return nil, err
	}
	b := &BeaconBlock{
		version:       version.Altair,
		slot:          pb.Slot,
		proposerIndex: pb.ProposerIndex,
		parentRoot:    bytesutil.ToBytes32(pb.ParentRoot),
		stateRoot:     bytesutil.ToBytes32(pb.StateRoot),
		body:          body,
	}
	return b, nil
}

func initBlockFromProtoBellatrix(pb *zond.BeaconBlockBellatrix) (*BeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	body, err := initBlockBodyFromProtoBellatrix(pb.Body)
	if err != nil {
		return nil, err
	}
	b := &BeaconBlock{
		version:       version.Bellatrix,
		slot:          pb.Slot,
		proposerIndex: pb.ProposerIndex,
		parentRoot:    bytesutil.ToBytes32(pb.ParentRoot),
		stateRoot:     bytesutil.ToBytes32(pb.StateRoot),
		body:          body,
	}
	return b, nil
}

func initBlindedBlockFromProtoBellatrix(pb *zond.BlindedBeaconBlockBellatrix) (*BeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	body, err := initBlindedBlockBodyFromProtoBellatrix(pb.Body)
	if err != nil {
		return nil, err
	}
	b := &BeaconBlock{
		version:       version.Bellatrix,
		slot:          pb.Slot,
		proposerIndex: pb.ProposerIndex,
		parentRoot:    bytesutil.ToBytes32(pb.ParentRoot),
		stateRoot:     bytesutil.ToBytes32(pb.StateRoot),
		body:          body,
	}
	return b, nil
}

func initBlockFromProtoCapella(pb *zond.BeaconBlockCapella) (*BeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	body, err := initBlockBodyFromProtoCapella(pb.Body)
	if err != nil {
		return nil, err
	}
	b := &BeaconBlock{
		version:       version.Capella,
		slot:          pb.Slot,
		proposerIndex: pb.ProposerIndex,
		parentRoot:    bytesutil.ToBytes32(pb.ParentRoot),
		stateRoot:     bytesutil.ToBytes32(pb.StateRoot),
		body:          body,
	}
	return b, nil
}

func initBlindedBlockFromProtoCapella(pb *zond.BlindedBeaconBlockCapella) (*BeaconBlock, error) {
	if pb == nil {
		return nil, errNilBlock
	}

	body, err := initBlindedBlockBodyFromProtoCapella(pb.Body)
	if err != nil {
		return nil, err
	}
	b := &BeaconBlock{
		version:       version.Capella,
		slot:          pb.Slot,
		proposerIndex: pb.ProposerIndex,
		parentRoot:    bytesutil.ToBytes32(pb.ParentRoot),
		stateRoot:     bytesutil.ToBytes32(pb.StateRoot),
		body:          body,
	}
	return b, nil
}

func initBlockBodyFromProtoPhase0(pb *zond.BeaconBlockBody) (*BeaconBlockBody, error) {
	if pb == nil {
		return nil, errNilBlockBody
	}

	b := &BeaconBlockBody{
		version:           version.Phase0,
		isBlinded:         false,
		randaoReveal:      bytesutil.ToBytes4595(pb.RandaoReveal),
		eth1Data:          pb.Eth1Data,
		graffiti:          bytesutil.ToBytes32(pb.Graffiti),
		proposerSlashings: pb.ProposerSlashings,
		attesterSlashings: pb.AttesterSlashings,
		attestations:      pb.Attestations,
		deposits:          pb.Deposits,
		voluntaryExits:    pb.VoluntaryExits,
	}
	return b, nil
}

func initBlockBodyFromProtoAltair(pb *zond.BeaconBlockBodyAltair) (*BeaconBlockBody, error) {
	if pb == nil {
		return nil, errNilBlockBody
	}

	b := &BeaconBlockBody{
		version:           version.Altair,
		isBlinded:         false,
		randaoReveal:      bytesutil.ToBytes4595(pb.RandaoReveal),
		eth1Data:          pb.Eth1Data,
		graffiti:          bytesutil.ToBytes32(pb.Graffiti),
		proposerSlashings: pb.ProposerSlashings,
		attesterSlashings: pb.AttesterSlashings,
		attestations:      pb.Attestations,
		deposits:          pb.Deposits,
		voluntaryExits:    pb.VoluntaryExits,
		syncAggregate:     pb.SyncAggregate,
	}
	return b, nil
}

func initBlockBodyFromProtoBellatrix(pb *zond.BeaconBlockBodyBellatrix) (*BeaconBlockBody, error) {
	if pb == nil {
		return nil, errNilBlockBody
	}

	p, err := WrappedExecutionPayload(pb.ExecutionPayload)
	// We allow the payload to be nil
	if err != nil && err != consensus_types.ErrNilObjectWrapped {
		return nil, err
	}
	b := &BeaconBlockBody{
		version:           version.Bellatrix,
		isBlinded:         false,
		randaoReveal:      bytesutil.ToBytes4595(pb.RandaoReveal),
		eth1Data:          pb.Eth1Data,
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

func initBlindedBlockBodyFromProtoBellatrix(pb *zond.BlindedBeaconBlockBodyBellatrix) (*BeaconBlockBody, error) {
	if pb == nil {
		return nil, errNilBlockBody
	}

	ph, err := WrappedExecutionPayloadHeader(pb.ExecutionPayloadHeader)
	// We allow the payload to be nil
	if err != nil && err != consensus_types.ErrNilObjectWrapped {
		return nil, err
	}
	b := &BeaconBlockBody{
		version:                version.Bellatrix,
		isBlinded:              true,
		randaoReveal:           bytesutil.ToBytes4595(pb.RandaoReveal),
		eth1Data:               pb.Eth1Data,
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

func initBlockBodyFromProtoCapella(pb *zond.BeaconBlockBodyCapella) (*BeaconBlockBody, error) {
	if pb == nil {
		return nil, errNilBlockBody
	}

	p, err := WrappedExecutionPayloadCapella(pb.ExecutionPayload, 0)
	// We allow the payload to be nil
	if err != nil && err != consensus_types.ErrNilObjectWrapped {
		return nil, err
	}
	b := &BeaconBlockBody{
		version:                     version.Capella,
		isBlinded:                   false,
		randaoReveal:                bytesutil.ToBytes4595(pb.RandaoReveal),
		eth1Data:                    pb.Eth1Data,
		graffiti:                    bytesutil.ToBytes32(pb.Graffiti),
		proposerSlashings:           pb.ProposerSlashings,
		attesterSlashings:           pb.AttesterSlashings,
		attestations:                pb.Attestations,
		deposits:                    pb.Deposits,
		voluntaryExits:              pb.VoluntaryExits,
		syncAggregate:               pb.SyncAggregate,
		executionPayload:            p,
		dilithiumToExecutionChanges: pb.DilithiumToExecutionChanges,
	}
	return b, nil
}

func initBlindedBlockBodyFromProtoCapella(pb *zond.BlindedBeaconBlockBodyCapella) (*BeaconBlockBody, error) {
	if pb == nil {
		return nil, errNilBlockBody
	}

	ph, err := WrappedExecutionPayloadHeaderCapella(pb.ExecutionPayloadHeader, 0)
	// We allow the payload to be nil
	if err != nil && err != consensus_types.ErrNilObjectWrapped {
		return nil, err
	}
	b := &BeaconBlockBody{
		version:                     version.Capella,
		isBlinded:                   true,
		randaoReveal:                bytesutil.ToBytes4595(pb.RandaoReveal),
		eth1Data:                    pb.Eth1Data,
		graffiti:                    bytesutil.ToBytes32(pb.Graffiti),
		proposerSlashings:           pb.ProposerSlashings,
		attesterSlashings:           pb.AttesterSlashings,
		attestations:                pb.Attestations,
		deposits:                    pb.Deposits,
		voluntaryExits:              pb.VoluntaryExits,
		syncAggregate:               pb.SyncAggregate,
		executionPayloadHeader:      ph,
		dilithiumToExecutionChanges: pb.DilithiumToExecutionChanges,
	}
	return b, nil
}
