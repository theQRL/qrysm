package migration

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpbalpha "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	zondpbv1 "github.com/theQRL/qrysm/v4/proto/zond/v1"
	"google.golang.org/protobuf/proto"
)

// BlockIfaceToV1BlockHeader converts a signed beacon block interface into a signed beacon block header.
func BlockIfaceToV1BlockHeader(block interfaces.ReadOnlySignedBeaconBlock) (*zondpbv1.SignedBeaconBlockHeader, error) {
	bodyRoot, err := block.Block().Body().HashTreeRoot()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get body root of block")
	}
	parentRoot := block.Block().ParentRoot()
	stateRoot := block.Block().StateRoot()
	sig := block.Signature()
	return &zondpbv1.SignedBeaconBlockHeader{
		Message: &zondpbv1.BeaconBlockHeader{
			Slot:          block.Block().Slot(),
			ProposerIndex: block.Block().ProposerIndex(),
			ParentRoot:    parentRoot[:],
			StateRoot:     stateRoot[:],
			BodyRoot:      bodyRoot[:],
		},
		Signature: sig[:],
	}, nil
}

// V1Alpha1ToV1SignedBlock converts a v1alpha1 SignedBeaconBlock proto to a v1 proto.
func V1Alpha1ToV1SignedBlock(alphaBlk *zondpbalpha.SignedBeaconBlock) (*zondpbv1.SignedBeaconBlock, error) {
	marshaledBlk, err := proto.Marshal(alphaBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1Block := &zondpbv1.SignedBeaconBlock{}
	if err := proto.Unmarshal(marshaledBlk, v1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1Block, nil
}

// V1ToV1Alpha1SignedBlock converts a v1 SignedBeaconBlock proto to a v1alpha1 proto.
func V1ToV1Alpha1SignedBlock(v1Blk *zondpbv1.SignedBeaconBlock) (*zondpbalpha.SignedBeaconBlock, error) {
	marshaledBlk, err := proto.Marshal(v1Blk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1alpha1Block := &zondpbalpha.SignedBeaconBlock{}
	if err := proto.Unmarshal(marshaledBlk, v1alpha1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1alpha1Block, nil
}

// V1Alpha1ToV1Block converts a v1alpha1 ReadOnlyBeaconBlock proto to a v1 proto.
func V1Alpha1ToV1Block(alphaBlk *zondpbalpha.BeaconBlock) (*zondpbv1.BeaconBlock, error) {
	marshaledBlk, err := proto.Marshal(alphaBlk)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1Block := &zondpbv1.BeaconBlock{}
	if err := proto.Unmarshal(marshaledBlk, v1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1Block, nil
}

// V1Alpha1AggregateAttAndProofToV1 converts a v1alpha1 aggregate attestation and proof to v1.
func V1Alpha1AggregateAttAndProofToV1(v1alpha1Att *zondpbalpha.AggregateAttestationAndProof) *zondpbv1.AggregateAttestationAndProof {
	if v1alpha1Att == nil {
		return &zondpbv1.AggregateAttestationAndProof{}
	}
	return &zondpbv1.AggregateAttestationAndProof{
		AggregatorIndex: v1alpha1Att.AggregatorIndex,
		Aggregate:       V1Alpha1AttestationToV1(v1alpha1Att.Aggregate),
		SelectionProof:  v1alpha1Att.SelectionProof,
	}
}

// V1SignedAggregateAttAndProofToV1Alpha1 converts a v1 signed aggregate attestation and proof to v1alpha1.
func V1SignedAggregateAttAndProofToV1Alpha1(v1Att *zondpbv1.SignedAggregateAttestationAndProof) *zondpbalpha.SignedAggregateAttestationAndProof {
	if v1Att == nil {
		return &zondpbalpha.SignedAggregateAttestationAndProof{}
	}
	return &zondpbalpha.SignedAggregateAttestationAndProof{
		Message: &zondpbalpha.AggregateAttestationAndProof{
			AggregatorIndex: v1Att.Message.AggregatorIndex,
			Aggregate:       V1AttestationToV1Alpha1(v1Att.Message.Aggregate),
			SelectionProof:  v1Att.Message.SelectionProof,
		},
		Signature: v1Att.Signature,
	}
}

// V1Alpha1IndexedAttToV1 converts a v1alpha1 indexed attestation to v1.
func V1Alpha1IndexedAttToV1(v1alpha1Att *zondpbalpha.IndexedAttestation) *zondpbv1.IndexedAttestation {
	if v1alpha1Att == nil {
		return &zondpbv1.IndexedAttestation{}
	}
	return &zondpbv1.IndexedAttestation{
		AttestingIndices: v1alpha1Att.AttestingIndices,
		Data:             V1Alpha1AttDataToV1(v1alpha1Att.Data),
		Signature:        v1alpha1Att.Signature,
	}
}

// V1Alpha1AttestationToV1 converts a v1alpha1 attestation to v1.
func V1Alpha1AttestationToV1(v1alpha1Att *zondpbalpha.Attestation) *zondpbv1.Attestation {
	if v1alpha1Att == nil {
		return &zondpbv1.Attestation{}
	}
	return &zondpbv1.Attestation{
		AggregationBits: v1alpha1Att.AggregationBits,
		Data:            V1Alpha1AttDataToV1(v1alpha1Att.Data),
		Signature:       v1alpha1Att.Signature,
	}
}

// V1AttestationToV1Alpha1 converts a v1 attestation to v1alpha1.
func V1AttestationToV1Alpha1(v1Att *zondpbv1.Attestation) *zondpbalpha.Attestation {
	if v1Att == nil {
		return &zondpbalpha.Attestation{}
	}
	return &zondpbalpha.Attestation{
		AggregationBits: v1Att.AggregationBits,
		Data:            V1AttDataToV1Alpha1(v1Att.Data),
		Signature:       v1Att.Signature,
	}
}

// V1Alpha1AttDataToV1 converts a v1alpha1 attestation data to v1.
func V1Alpha1AttDataToV1(v1alpha1AttData *zondpbalpha.AttestationData) *zondpbv1.AttestationData {
	if v1alpha1AttData == nil || v1alpha1AttData.Source == nil || v1alpha1AttData.Target == nil {
		return &zondpbv1.AttestationData{}
	}
	return &zondpbv1.AttestationData{
		Slot:            v1alpha1AttData.Slot,
		Index:           v1alpha1AttData.CommitteeIndex,
		BeaconBlockRoot: v1alpha1AttData.BeaconBlockRoot,
		Source: &zondpbv1.Checkpoint{
			Root:  v1alpha1AttData.Source.Root,
			Epoch: v1alpha1AttData.Source.Epoch,
		},
		Target: &zondpbv1.Checkpoint{
			Root:  v1alpha1AttData.Target.Root,
			Epoch: v1alpha1AttData.Target.Epoch,
		},
	}
}

// V1Alpha1AttSlashingToV1 converts a v1alpha1 attester slashing to v1.
func V1Alpha1AttSlashingToV1(v1alpha1Slashing *zondpbalpha.AttesterSlashing) *zondpbv1.AttesterSlashing {
	if v1alpha1Slashing == nil {
		return &zondpbv1.AttesterSlashing{}
	}
	return &zondpbv1.AttesterSlashing{
		Attestation_1: V1Alpha1IndexedAttToV1(v1alpha1Slashing.Attestation_1),
		Attestation_2: V1Alpha1IndexedAttToV1(v1alpha1Slashing.Attestation_2),
	}
}

// V1Alpha1SignedHeaderToV1 converts a v1alpha1 signed beacon block header to v1.
func V1Alpha1SignedHeaderToV1(v1alpha1Hdr *zondpbalpha.SignedBeaconBlockHeader) *zondpbv1.SignedBeaconBlockHeader {
	if v1alpha1Hdr == nil || v1alpha1Hdr.Header == nil {
		return &zondpbv1.SignedBeaconBlockHeader{}
	}
	return &zondpbv1.SignedBeaconBlockHeader{
		Message: &zondpbv1.BeaconBlockHeader{
			Slot:          v1alpha1Hdr.Header.Slot,
			ProposerIndex: v1alpha1Hdr.Header.ProposerIndex,
			ParentRoot:    v1alpha1Hdr.Header.ParentRoot,
			StateRoot:     v1alpha1Hdr.Header.StateRoot,
			BodyRoot:      v1alpha1Hdr.Header.BodyRoot,
		},
		Signature: v1alpha1Hdr.Signature,
	}
}

// V1SignedHeaderToV1Alpha1 converts a v1 signed beacon block header to v1alpha1.
func V1SignedHeaderToV1Alpha1(v1Header *zondpbv1.SignedBeaconBlockHeader) *zondpbalpha.SignedBeaconBlockHeader {
	if v1Header == nil || v1Header.Message == nil {
		return &zondpbalpha.SignedBeaconBlockHeader{}
	}
	return &zondpbalpha.SignedBeaconBlockHeader{
		Header: &zondpbalpha.BeaconBlockHeader{
			Slot:          v1Header.Message.Slot,
			ProposerIndex: v1Header.Message.ProposerIndex,
			ParentRoot:    v1Header.Message.ParentRoot,
			StateRoot:     v1Header.Message.StateRoot,
			BodyRoot:      v1Header.Message.BodyRoot,
		},
		Signature: v1Header.Signature,
	}
}

// V1Alpha1ProposerSlashingToV1 converts a v1alpha1 proposer slashing to v1.
func V1Alpha1ProposerSlashingToV1(v1alpha1Slashing *zondpbalpha.ProposerSlashing) *zondpbv1.ProposerSlashing {
	if v1alpha1Slashing == nil {
		return &zondpbv1.ProposerSlashing{}
	}
	return &zondpbv1.ProposerSlashing{
		SignedHeader_1: V1Alpha1SignedHeaderToV1(v1alpha1Slashing.Header_1),
		SignedHeader_2: V1Alpha1SignedHeaderToV1(v1alpha1Slashing.Header_2),
	}
}

// V1Alpha1ExitToV1 converts a v1alpha1 SignedVoluntaryExit to v1.
func V1Alpha1ExitToV1(v1alpha1Exit *zondpbalpha.SignedVoluntaryExit) *zondpbv1.SignedVoluntaryExit {
	if v1alpha1Exit == nil || v1alpha1Exit.Exit == nil {
		return &zondpbv1.SignedVoluntaryExit{}
	}
	return &zondpbv1.SignedVoluntaryExit{
		Message: &zondpbv1.VoluntaryExit{
			Epoch:          v1alpha1Exit.Exit.Epoch,
			ValidatorIndex: v1alpha1Exit.Exit.ValidatorIndex,
		},
		Signature: v1alpha1Exit.Signature,
	}
}

// V1ExitToV1Alpha1 converts a v1 SignedVoluntaryExit to v1alpha1.
func V1ExitToV1Alpha1(v1Exit *zondpbv1.SignedVoluntaryExit) *zondpbalpha.SignedVoluntaryExit {
	if v1Exit == nil || v1Exit.Message == nil {
		return &zondpbalpha.SignedVoluntaryExit{}
	}
	return &zondpbalpha.SignedVoluntaryExit{
		Exit: &zondpbalpha.VoluntaryExit{
			Epoch:          v1Exit.Message.Epoch,
			ValidatorIndex: v1Exit.Message.ValidatorIndex,
		},
		Signature: v1Exit.Signature,
	}
}

// V1AttToV1Alpha1 converts a v1 attestation to v1alpha1.
func V1AttToV1Alpha1(v1Att *zondpbv1.Attestation) *zondpbalpha.Attestation {
	if v1Att == nil {
		return &zondpbalpha.Attestation{}
	}
	return &zondpbalpha.Attestation{
		AggregationBits: v1Att.AggregationBits,
		Data:            V1AttDataToV1Alpha1(v1Att.Data),
		Signature:       v1Att.Signature,
	}
}

// V1IndexedAttToV1Alpha1 converts a v1 indexed attestation to v1alpha1.
func V1IndexedAttToV1Alpha1(v1Att *zondpbv1.IndexedAttestation) *zondpbalpha.IndexedAttestation {
	if v1Att == nil {
		return &zondpbalpha.IndexedAttestation{}
	}
	return &zondpbalpha.IndexedAttestation{
		AttestingIndices: v1Att.AttestingIndices,
		Data:             V1AttDataToV1Alpha1(v1Att.Data),
		Signature:        v1Att.Signature,
	}
}

// V1AttDataToV1Alpha1 converts a v1 attestation data to v1alpha1.
func V1AttDataToV1Alpha1(v1AttData *zondpbv1.AttestationData) *zondpbalpha.AttestationData {
	if v1AttData == nil || v1AttData.Source == nil || v1AttData.Target == nil {
		return &zondpbalpha.AttestationData{}
	}
	return &zondpbalpha.AttestationData{
		Slot:            v1AttData.Slot,
		CommitteeIndex:  v1AttData.Index,
		BeaconBlockRoot: v1AttData.BeaconBlockRoot,
		Source: &zondpbalpha.Checkpoint{
			Root:  v1AttData.Source.Root,
			Epoch: v1AttData.Source.Epoch,
		},
		Target: &zondpbalpha.Checkpoint{
			Root:  v1AttData.Target.Root,
			Epoch: v1AttData.Target.Epoch,
		},
	}
}

// V1AttSlashingToV1Alpha1 converts a v1 attester slashing to v1alpha1.
func V1AttSlashingToV1Alpha1(v1Slashing *zondpbv1.AttesterSlashing) *zondpbalpha.AttesterSlashing {
	if v1Slashing == nil {
		return &zondpbalpha.AttesterSlashing{}
	}
	return &zondpbalpha.AttesterSlashing{
		Attestation_1: V1IndexedAttToV1Alpha1(v1Slashing.Attestation_1),
		Attestation_2: V1IndexedAttToV1Alpha1(v1Slashing.Attestation_2),
	}
}

// V1ProposerSlashingToV1Alpha1 converts a v1 proposer slashing to v1alpha1.
func V1ProposerSlashingToV1Alpha1(v1Slashing *zondpbv1.ProposerSlashing) *zondpbalpha.ProposerSlashing {
	if v1Slashing == nil {
		return &zondpbalpha.ProposerSlashing{}
	}
	return &zondpbalpha.ProposerSlashing{
		Header_1: V1SignedHeaderToV1Alpha1(v1Slashing.SignedHeader_1),
		Header_2: V1SignedHeaderToV1Alpha1(v1Slashing.SignedHeader_2),
	}
}

// V1Alpha1ValidatorToV1 converts a v1alpha1 validator to v1.
func V1Alpha1ValidatorToV1(v1Alpha1Validator *zondpbalpha.Validator) *zondpbv1.Validator {
	if v1Alpha1Validator == nil {
		return &zondpbv1.Validator{}
	}
	return &zondpbv1.Validator{
		Pubkey:                     v1Alpha1Validator.PublicKey,
		WithdrawalCredentials:      v1Alpha1Validator.WithdrawalCredentials,
		EffectiveBalance:           v1Alpha1Validator.EffectiveBalance,
		Slashed:                    v1Alpha1Validator.Slashed,
		ActivationEligibilityEpoch: v1Alpha1Validator.ActivationEligibilityEpoch,
		ActivationEpoch:            v1Alpha1Validator.ActivationEpoch,
		ExitEpoch:                  v1Alpha1Validator.ExitEpoch,
		WithdrawableEpoch:          v1Alpha1Validator.WithdrawableEpoch,
	}
}

// V1ValidatorToV1Alpha1 converts a v1 validator to v1alpha1.
func V1ValidatorToV1Alpha1(v1Validator *zondpbv1.Validator) *zondpbalpha.Validator {
	if v1Validator == nil {
		return &zondpbalpha.Validator{}
	}
	return &zondpbalpha.Validator{
		PublicKey:                  v1Validator.Pubkey,
		WithdrawalCredentials:      v1Validator.WithdrawalCredentials,
		EffectiveBalance:           v1Validator.EffectiveBalance,
		Slashed:                    v1Validator.Slashed,
		ActivationEligibilityEpoch: v1Validator.ActivationEligibilityEpoch,
		ActivationEpoch:            v1Validator.ActivationEpoch,
		ExitEpoch:                  v1Validator.ExitEpoch,
		WithdrawableEpoch:          v1Validator.WithdrawableEpoch,
	}
}

// SignedBeaconBlock converts a signed beacon block interface to a v1alpha1 block.
func SignedBeaconBlock(block interfaces.ReadOnlySignedBeaconBlock) (*zondpbv1.SignedBeaconBlock, error) {
	if block == nil || block.IsNil() {
		return nil, errors.New("could not find requested block")
	}
	blk, err := block.PbPhase0Block()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get raw block")
	}

	v1Block, err := V1Alpha1ToV1SignedBlock(blk)
	if err != nil {
		return nil, errors.New("could not convert block to v1 block")
	}

	return v1Block, nil
}

// BeaconStateToProto converts a state.BeaconState object to its protobuf equivalent.
func BeaconStateToProto(state state.BeaconState) (*zondpbv1.BeaconState, error) {
	sourceFork := state.Fork()
	sourceLatestBlockHeader := state.LatestBlockHeader()
	sourceEth1Data := state.Eth1Data()
	sourceEth1DataVotes := state.Eth1DataVotes()
	sourceValidators := state.Validators()
	sourcePrevEpochAtts, err := state.PreviousEpochAttestations()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get previous epoch attestations from state")
	}
	sourceCurrEpochAtts, err := state.CurrentEpochAttestations()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get current epoch attestations from state")
	}
	sourceJustificationBits := state.JustificationBits()
	sourcePrevJustifiedCheckpoint := state.PreviousJustifiedCheckpoint()
	sourceCurrJustifiedCheckpoint := state.CurrentJustifiedCheckpoint()
	sourceFinalizedCheckpoint := state.FinalizedCheckpoint()

	resultEth1DataVotes := make([]*zondpbv1.Eth1Data, len(sourceEth1DataVotes))
	for i, vote := range sourceEth1DataVotes {
		resultEth1DataVotes[i] = &zondpbv1.Eth1Data{
			DepositRoot:  bytesutil.SafeCopyBytes(vote.DepositRoot),
			DepositCount: vote.DepositCount,
			BlockHash:    bytesutil.SafeCopyBytes(vote.BlockHash),
		}
	}
	resultValidators := make([]*zondpbv1.Validator, len(sourceValidators))
	for i, validator := range sourceValidators {
		resultValidators[i] = &zondpbv1.Validator{
			Pubkey:                     bytesutil.SafeCopyBytes(validator.PublicKey),
			WithdrawalCredentials:      bytesutil.SafeCopyBytes(validator.WithdrawalCredentials),
			EffectiveBalance:           validator.EffectiveBalance,
			Slashed:                    validator.Slashed,
			ActivationEligibilityEpoch: validator.ActivationEligibilityEpoch,
			ActivationEpoch:            validator.ActivationEpoch,
			ExitEpoch:                  validator.ExitEpoch,
			WithdrawableEpoch:          validator.WithdrawableEpoch,
		}
	}
	resultPrevEpochAtts := make([]*zondpbv1.PendingAttestation, len(sourcePrevEpochAtts))
	for i, att := range sourcePrevEpochAtts {
		data := att.Data
		resultPrevEpochAtts[i] = &zondpbv1.PendingAttestation{
			AggregationBits: att.AggregationBits,
			Data: &zondpbv1.AttestationData{
				Slot:            data.Slot,
				Index:           data.CommitteeIndex,
				BeaconBlockRoot: data.BeaconBlockRoot,
				Source: &zondpbv1.Checkpoint{
					Epoch: data.Source.Epoch,
					Root:  data.Source.Root,
				},
				Target: &zondpbv1.Checkpoint{
					Epoch: data.Target.Epoch,
					Root:  data.Target.Root,
				},
			},
			InclusionDelay: att.InclusionDelay,
			ProposerIndex:  att.ProposerIndex,
		}
	}
	resultCurrEpochAtts := make([]*zondpbv1.PendingAttestation, len(sourceCurrEpochAtts))
	for i, att := range sourceCurrEpochAtts {
		data := att.Data
		resultCurrEpochAtts[i] = &zondpbv1.PendingAttestation{
			AggregationBits: att.AggregationBits,
			Data: &zondpbv1.AttestationData{
				Slot:            data.Slot,
				Index:           data.CommitteeIndex,
				BeaconBlockRoot: data.BeaconBlockRoot,
				Source: &zondpbv1.Checkpoint{
					Epoch: data.Source.Epoch,
					Root:  data.Source.Root,
				},
				Target: &zondpbv1.Checkpoint{
					Epoch: data.Target.Epoch,
					Root:  data.Target.Root,
				},
			},
			InclusionDelay: att.InclusionDelay,
			ProposerIndex:  att.ProposerIndex,
		}
	}

	hRoot, err := state.HistoricalRoots()
	if err != nil {
		return nil, errors.Wrapf(err, "could not get historical roots from state")
	}
	result := &zondpbv1.BeaconState{
		GenesisTime:           state.GenesisTime(),
		GenesisValidatorsRoot: bytesutil.SafeCopyBytes(state.GenesisValidatorsRoot()),
		Slot:                  state.Slot(),
		Fork: &zondpbv1.Fork{
			PreviousVersion: bytesutil.SafeCopyBytes(sourceFork.PreviousVersion),
			CurrentVersion:  bytesutil.SafeCopyBytes(sourceFork.CurrentVersion),
			Epoch:           sourceFork.Epoch,
		},
		LatestBlockHeader: &zondpbv1.BeaconBlockHeader{
			Slot:          sourceLatestBlockHeader.Slot,
			ProposerIndex: sourceLatestBlockHeader.ProposerIndex,
			ParentRoot:    bytesutil.SafeCopyBytes(sourceLatestBlockHeader.ParentRoot),
			StateRoot:     bytesutil.SafeCopyBytes(sourceLatestBlockHeader.StateRoot),
			BodyRoot:      bytesutil.SafeCopyBytes(sourceLatestBlockHeader.BodyRoot),
		},
		BlockRoots:      bytesutil.SafeCopy2dBytes(state.BlockRoots()),
		StateRoots:      bytesutil.SafeCopy2dBytes(state.StateRoots()),
		HistoricalRoots: bytesutil.SafeCopy2dBytes(hRoot),
		Eth1Data: &zondpbv1.Eth1Data{
			DepositRoot:  bytesutil.SafeCopyBytes(sourceEth1Data.DepositRoot),
			DepositCount: sourceEth1Data.DepositCount,
			BlockHash:    bytesutil.SafeCopyBytes(sourceEth1Data.BlockHash),
		},
		Eth1DataVotes:             resultEth1DataVotes,
		Eth1DepositIndex:          state.Eth1DepositIndex(),
		Validators:                resultValidators,
		Balances:                  state.Balances(),
		RandaoMixes:               bytesutil.SafeCopy2dBytes(state.RandaoMixes()),
		Slashings:                 state.Slashings(),
		PreviousEpochAttestations: resultPrevEpochAtts,
		CurrentEpochAttestations:  resultCurrEpochAtts,
		JustificationBits:         bytesutil.SafeCopyBytes(sourceJustificationBits),
		PreviousJustifiedCheckpoint: &zondpbv1.Checkpoint{
			Epoch: sourcePrevJustifiedCheckpoint.Epoch,
			Root:  bytesutil.SafeCopyBytes(sourcePrevJustifiedCheckpoint.Root),
		},
		CurrentJustifiedCheckpoint: &zondpbv1.Checkpoint{
			Epoch: sourceCurrJustifiedCheckpoint.Epoch,
			Root:  bytesutil.SafeCopyBytes(sourceCurrJustifiedCheckpoint.Root),
		},
		FinalizedCheckpoint: &zondpbv1.Checkpoint{
			Epoch: sourceFinalizedCheckpoint.Epoch,
			Root:  bytesutil.SafeCopyBytes(sourceFinalizedCheckpoint.Root),
		},
	}

	return result, nil
}
