package migration

import (
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/theQRL/qrysm/v4/encoding/ssz"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
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
		Signatures:       v1alpha1Att.Signatures,
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
		Signatures:      v1alpha1Att.Signatures,
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
		Signatures:      v1Att.Signatures,
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
		Signatures:      v1Att.Signatures,
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
		Signatures:       v1Att.Signatures,
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

// V1Alpha1BeaconBlockCapellaToV1 converts a v1alpha1 Capella beacon block to a v1
// Capella block.
func V1Alpha1BeaconBlockCapellaToV1(v1alpha1Block *zondpbalpha.BeaconBlockCapella) (*zondpbv1.BeaconBlockCapella, error) {
	marshaledBlk, err := proto.Marshal(v1alpha1Block)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1Block := &zondpbv1.BeaconBlockCapella{}
	if err := proto.Unmarshal(marshaledBlk, v1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1Block, nil
}

// V1Alpha1BeaconBlockBlindedCapellaToV1Blinded converts a v1alpha1 Blinded Capella beacon block to a v1 Blinded Capella block.
func V1Alpha1BeaconBlockBlindedCapellaToV1Blinded(v1alpha1Block *zondpbalpha.BlindedBeaconBlockCapella) (*zondpbv1.BlindedBeaconBlockCapella, error) {
	marshaledBlk, err := proto.Marshal(v1alpha1Block)
	if err != nil {
		return nil, errors.Wrap(err, "could not marshal block")
	}
	v1Block := &zondpbv1.BlindedBeaconBlockCapella{}
	if err := proto.Unmarshal(marshaledBlk, v1Block); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal block")
	}
	return v1Block, nil
}

// V1Alpha1BeaconBlockCapellaToV1Blinded converts a v1alpha1 Capella beacon block to a v1
// blinded Capella block.
func V1Alpha1BeaconBlockCapellaToV1Blinded(v1alpha1Block *zondpbalpha.BeaconBlockCapella) (*zondpbv1.BlindedBeaconBlockCapella, error) {
	sourceProposerSlashings := v1alpha1Block.Body.ProposerSlashings
	resultProposerSlashings := make([]*zondpbv1.ProposerSlashing, len(sourceProposerSlashings))
	for i, s := range sourceProposerSlashings {
		resultProposerSlashings[i] = &zondpbv1.ProposerSlashing{
			SignedHeader_1: &zondpbv1.SignedBeaconBlockHeader{
				Message: &zondpbv1.BeaconBlockHeader{
					Slot:          s.Header_1.Header.Slot,
					ProposerIndex: s.Header_1.Header.ProposerIndex,
					ParentRoot:    bytesutil.SafeCopyBytes(s.Header_1.Header.ParentRoot),
					StateRoot:     bytesutil.SafeCopyBytes(s.Header_1.Header.StateRoot),
					BodyRoot:      bytesutil.SafeCopyBytes(s.Header_1.Header.BodyRoot),
				},
				Signature: bytesutil.SafeCopyBytes(s.Header_1.Signature),
			},
			SignedHeader_2: &zondpbv1.SignedBeaconBlockHeader{
				Message: &zondpbv1.BeaconBlockHeader{
					Slot:          s.Header_2.Header.Slot,
					ProposerIndex: s.Header_2.Header.ProposerIndex,
					ParentRoot:    bytesutil.SafeCopyBytes(s.Header_2.Header.ParentRoot),
					StateRoot:     bytesutil.SafeCopyBytes(s.Header_2.Header.StateRoot),
					BodyRoot:      bytesutil.SafeCopyBytes(s.Header_2.Header.BodyRoot),
				},
				Signature: bytesutil.SafeCopyBytes(s.Header_2.Signature),
			},
		}
	}

	sourceAttesterSlashings := v1alpha1Block.Body.AttesterSlashings
	resultAttesterSlashings := make([]*zondpbv1.AttesterSlashing, len(sourceAttesterSlashings))
	for i, s := range sourceAttesterSlashings {
		att1Indices := make([]uint64, len(s.Attestation_1.AttestingIndices))
		copy(att1Indices, s.Attestation_1.AttestingIndices)
		att2Indices := make([]uint64, len(s.Attestation_2.AttestingIndices))
		copy(att2Indices, s.Attestation_2.AttestingIndices)
		signatures1 := make([][]byte, len(s.Attestation_1.Signatures))
		for i, sig := range s.Attestation_1.Signatures {
			signatures1[i] = bytesutil.SafeCopyBytes(sig)
		}
		signatures2 := make([][]byte, len(s.Attestation_2.Signatures))
		for i, sig := range s.Attestation_2.Signatures {
			signatures2[i] = bytesutil.SafeCopyBytes(sig)
		}

		resultAttesterSlashings[i] = &zondpbv1.AttesterSlashing{
			Attestation_1: &zondpbv1.IndexedAttestation{
				AttestingIndices: att1Indices,
				Data: &zondpbv1.AttestationData{
					Slot:            s.Attestation_1.Data.Slot,
					Index:           s.Attestation_1.Data.CommitteeIndex,
					BeaconBlockRoot: bytesutil.SafeCopyBytes(s.Attestation_1.Data.BeaconBlockRoot),
					Source: &zondpbv1.Checkpoint{
						Epoch: s.Attestation_1.Data.Source.Epoch,
						Root:  bytesutil.SafeCopyBytes(s.Attestation_1.Data.Source.Root),
					},
					Target: &zondpbv1.Checkpoint{
						Epoch: s.Attestation_1.Data.Target.Epoch,
						Root:  bytesutil.SafeCopyBytes(s.Attestation_1.Data.Target.Root),
					},
				},
				Signatures: signatures1,
			},
			Attestation_2: &zondpbv1.IndexedAttestation{
				AttestingIndices: att2Indices,
				Data: &zondpbv1.AttestationData{
					Slot:            s.Attestation_2.Data.Slot,
					Index:           s.Attestation_2.Data.CommitteeIndex,
					BeaconBlockRoot: bytesutil.SafeCopyBytes(s.Attestation_2.Data.BeaconBlockRoot),
					Source: &zondpbv1.Checkpoint{
						Epoch: s.Attestation_2.Data.Source.Epoch,
						Root:  bytesutil.SafeCopyBytes(s.Attestation_2.Data.Source.Root),
					},
					Target: &zondpbv1.Checkpoint{
						Epoch: s.Attestation_2.Data.Target.Epoch,
						Root:  bytesutil.SafeCopyBytes(s.Attestation_2.Data.Target.Root),
					},
				},
				Signatures: signatures2,
			},
		}
	}

	sourceAttestations := v1alpha1Block.Body.Attestations
	resultAttestations := make([]*zondpbv1.Attestation, len(sourceAttestations))
	for i, a := range sourceAttestations {
		signatures := make([][]byte, len(a.Signatures))
		for i, sig := range a.Signatures {
			signatures[i] = bytesutil.SafeCopyBytes(sig)
		}

		resultAttestations[i] = &zondpbv1.Attestation{
			AggregationBits: bytesutil.SafeCopyBytes(a.AggregationBits),
			Data: &zondpbv1.AttestationData{
				Slot:            a.Data.Slot,
				Index:           a.Data.CommitteeIndex,
				BeaconBlockRoot: bytesutil.SafeCopyBytes(a.Data.BeaconBlockRoot),
				Source: &zondpbv1.Checkpoint{
					Epoch: a.Data.Source.Epoch,
					Root:  bytesutil.SafeCopyBytes(a.Data.Source.Root),
				},
				Target: &zondpbv1.Checkpoint{
					Epoch: a.Data.Target.Epoch,
					Root:  bytesutil.SafeCopyBytes(a.Data.Target.Root),
				},
			},
			Signatures: signatures,
		}
	}

	sourceDeposits := v1alpha1Block.Body.Deposits
	resultDeposits := make([]*zondpbv1.Deposit, len(sourceDeposits))
	for i, d := range sourceDeposits {
		resultDeposits[i] = &zondpbv1.Deposit{
			Proof: bytesutil.SafeCopy2dBytes(d.Proof),
			Data: &zondpbv1.Deposit_Data{
				Pubkey:                bytesutil.SafeCopyBytes(d.Data.PublicKey),
				WithdrawalCredentials: bytesutil.SafeCopyBytes(d.Data.WithdrawalCredentials),
				Amount:                d.Data.Amount,
				Signature:             bytesutil.SafeCopyBytes(d.Data.Signature),
			},
		}
	}

	sourceExits := v1alpha1Block.Body.VoluntaryExits
	resultExits := make([]*zondpbv1.SignedVoluntaryExit, len(sourceExits))
	for i, e := range sourceExits {
		resultExits[i] = &zondpbv1.SignedVoluntaryExit{
			Message: &zondpbv1.VoluntaryExit{
				Epoch:          e.Exit.Epoch,
				ValidatorIndex: e.Exit.ValidatorIndex,
			},
			Signature: bytesutil.SafeCopyBytes(e.Signature),
		}
	}

	transactionsRoot, err := ssz.TransactionsRoot(v1alpha1Block.Body.ExecutionPayload.Transactions)
	if err != nil {
		return nil, errors.Wrapf(err, "could not calculate transactions root")
	}

	withdrawalsRoot, err := ssz.WithdrawalSliceRoot(v1alpha1Block.Body.ExecutionPayload.Withdrawals, fieldparams.MaxWithdrawalsPerPayload)
	if err != nil {
		return nil, errors.Wrapf(err, "could not calculate transactions root")
	}

	changes := make([]*zondpbv1.SignedDilithiumToExecutionChange, len(v1alpha1Block.Body.DilithiumToExecutionChanges))
	for i, change := range v1alpha1Block.Body.DilithiumToExecutionChanges {
		changes[i] = &zondpbv1.SignedDilithiumToExecutionChange{
			Message: &zondpbv1.DilithiumToExecutionChange{
				ValidatorIndex:      change.Message.ValidatorIndex,
				FromDilithiumPubkey: bytesutil.SafeCopyBytes(change.Message.FromDilithiumPubkey),
				ToExecutionAddress:  bytesutil.SafeCopyBytes(change.Message.ToExecutionAddress),
			},
			Signature: bytesutil.SafeCopyBytes(change.Signature),
		}
	}

	syncSigs := make([][]byte, len(v1alpha1Block.Body.SyncAggregate.SyncCommitteeSignatures))
	for i, sig := range v1alpha1Block.Body.SyncAggregate.SyncCommitteeSignatures {
		syncSigs[i] = bytesutil.SafeCopyBytes(sig)
	}

	resultBlockBody := &zondpbv1.BlindedBeaconBlockBodyCapella{
		RandaoReveal: bytesutil.SafeCopyBytes(v1alpha1Block.Body.RandaoReveal),
		Eth1Data: &zondpbv1.Eth1Data{
			DepositRoot:  bytesutil.SafeCopyBytes(v1alpha1Block.Body.Eth1Data.DepositRoot),
			DepositCount: v1alpha1Block.Body.Eth1Data.DepositCount,
			BlockHash:    bytesutil.SafeCopyBytes(v1alpha1Block.Body.Eth1Data.BlockHash),
		},
		Graffiti:          bytesutil.SafeCopyBytes(v1alpha1Block.Body.Graffiti),
		ProposerSlashings: resultProposerSlashings,
		AttesterSlashings: resultAttesterSlashings,
		Attestations:      resultAttestations,
		Deposits:          resultDeposits,
		VoluntaryExits:    resultExits,
		SyncAggregate: &zondpbv1.SyncAggregate{
			SyncCommitteeBits:       bytesutil.SafeCopyBytes(v1alpha1Block.Body.SyncAggregate.SyncCommitteeBits),
			SyncCommitteeSignatures: syncSigs,
		},
		ExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderCapella{
			ParentHash:       bytesutil.SafeCopyBytes(v1alpha1Block.Body.ExecutionPayload.ParentHash),
			FeeRecipient:     bytesutil.SafeCopyBytes(v1alpha1Block.Body.ExecutionPayload.FeeRecipient),
			StateRoot:        bytesutil.SafeCopyBytes(v1alpha1Block.Body.ExecutionPayload.StateRoot),
			ReceiptsRoot:     bytesutil.SafeCopyBytes(v1alpha1Block.Body.ExecutionPayload.ReceiptsRoot),
			LogsBloom:        bytesutil.SafeCopyBytes(v1alpha1Block.Body.ExecutionPayload.LogsBloom),
			PrevRandao:       bytesutil.SafeCopyBytes(v1alpha1Block.Body.ExecutionPayload.PrevRandao),
			BlockNumber:      v1alpha1Block.Body.ExecutionPayload.BlockNumber,
			GasLimit:         v1alpha1Block.Body.ExecutionPayload.GasLimit,
			GasUsed:          v1alpha1Block.Body.ExecutionPayload.GasUsed,
			Timestamp:        v1alpha1Block.Body.ExecutionPayload.Timestamp,
			ExtraData:        bytesutil.SafeCopyBytes(v1alpha1Block.Body.ExecutionPayload.ExtraData),
			BaseFeePerGas:    bytesutil.SafeCopyBytes(v1alpha1Block.Body.ExecutionPayload.BaseFeePerGas),
			BlockHash:        bytesutil.SafeCopyBytes(v1alpha1Block.Body.ExecutionPayload.BlockHash),
			TransactionsRoot: transactionsRoot[:],
			WithdrawalsRoot:  withdrawalsRoot[:],
		},
		DilithiumToExecutionChanges: changes,
	}
	v1Block := &zondpbv1.BlindedBeaconBlockCapella{
		Slot:          v1alpha1Block.Slot,
		ProposerIndex: v1alpha1Block.ProposerIndex,
		ParentRoot:    bytesutil.SafeCopyBytes(v1alpha1Block.ParentRoot),
		StateRoot:     bytesutil.SafeCopyBytes(v1alpha1Block.StateRoot),
		Body:          resultBlockBody,
	}
	return v1Block, nil
}

// BeaconStateCapellaToProto converts a state.BeaconState object to its protobuf equivalent.
func BeaconStateCapellaToProto(st state.BeaconState) (*zondpbv1.BeaconStateCapella, error) {
	sourceFork := st.Fork()
	sourceLatestBlockHeader := st.LatestBlockHeader()
	sourceEth1Data := st.Eth1Data()
	sourceEth1DataVotes := st.Eth1DataVotes()
	sourceValidators := st.Validators()
	sourceJustificationBits := st.JustificationBits()
	sourcePrevJustifiedCheckpoint := st.PreviousJustifiedCheckpoint()
	sourceCurrJustifiedCheckpoint := st.CurrentJustifiedCheckpoint()
	sourceFinalizedCheckpoint := st.FinalizedCheckpoint()

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

	sourcePrevEpochParticipation, err := st.PreviousEpochParticipation()
	if err != nil {
		return nil, errors.Wrap(err, "could not get previous epoch participation")
	}
	sourceCurrEpochParticipation, err := st.CurrentEpochParticipation()
	if err != nil {
		return nil, errors.Wrap(err, "could not get current epoch participation")
	}
	sourceInactivityScores, err := st.InactivityScores()
	if err != nil {
		return nil, errors.Wrap(err, "could not get inactivity scores")
	}
	sourceCurrSyncCommittee, err := st.CurrentSyncCommittee()
	if err != nil {
		return nil, errors.Wrap(err, "could not get current sync committee")
	}
	sourceNextSyncCommittee, err := st.NextSyncCommittee()
	if err != nil {
		return nil, errors.Wrap(err, "could not get next sync committee")
	}
	executionPayloadHeaderInterface, err := st.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, errors.Wrap(err, "could not get latest execution payload header")
	}
	sourceLatestExecutionPayloadHeader, ok := executionPayloadHeaderInterface.Proto().(*enginev1.ExecutionPayloadHeaderCapella)
	if !ok {
		return nil, errors.New("execution payload header has incorrect type")
	}
	sourceNextWithdrawalIndex, err := st.NextWithdrawalIndex()
	if err != nil {
		return nil, errors.Wrap(err, "could not get next withdrawal index")
	}
	sourceNextWithdrawalValIndex, err := st.NextWithdrawalValidatorIndex()
	if err != nil {
		return nil, errors.Wrap(err, "could not get next withdrawal validator index")
	}
	summaries, err := st.HistoricalSummaries()
	if err != nil {
		return nil, errors.Wrap(err, "could not get historical summaries")
	}
	sourceHistoricalSummaries := make([]*zondpbv1.HistoricalSummary, len(summaries))
	for i, summary := range summaries {
		sourceHistoricalSummaries[i] = &zondpbv1.HistoricalSummary{
			BlockSummaryRoot: summary.BlockSummaryRoot,
			StateSummaryRoot: summary.StateSummaryRoot,
		}
	}
	hRoots, err := st.HistoricalRoots()
	if err != nil {
		return nil, errors.Wrap(err, "could not get historical roots")
	}

	result := &zondpbv1.BeaconStateCapella{
		GenesisTime:           st.GenesisTime(),
		GenesisValidatorsRoot: bytesutil.SafeCopyBytes(st.GenesisValidatorsRoot()),
		Slot:                  st.Slot(),
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
		BlockRoots: bytesutil.SafeCopy2dBytes(st.BlockRoots()),
		StateRoots: bytesutil.SafeCopy2dBytes(st.StateRoots()),
		Eth1Data: &zondpbv1.Eth1Data{
			DepositRoot:  bytesutil.SafeCopyBytes(sourceEth1Data.DepositRoot),
			DepositCount: sourceEth1Data.DepositCount,
			BlockHash:    bytesutil.SafeCopyBytes(sourceEth1Data.BlockHash),
		},
		Eth1DataVotes:              resultEth1DataVotes,
		Eth1DepositIndex:           st.Eth1DepositIndex(),
		Validators:                 resultValidators,
		Balances:                   st.Balances(),
		RandaoMixes:                bytesutil.SafeCopy2dBytes(st.RandaoMixes()),
		Slashings:                  st.Slashings(),
		PreviousEpochParticipation: bytesutil.SafeCopyBytes(sourcePrevEpochParticipation),
		CurrentEpochParticipation:  bytesutil.SafeCopyBytes(sourceCurrEpochParticipation),
		JustificationBits:          bytesutil.SafeCopyBytes(sourceJustificationBits),
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
		InactivityScores: sourceInactivityScores,
		CurrentSyncCommittee: &zondpbv1.SyncCommittee{
			Pubkeys: bytesutil.SafeCopy2dBytes(sourceCurrSyncCommittee.Pubkeys),
		},
		NextSyncCommittee: &zondpbv1.SyncCommittee{
			Pubkeys: bytesutil.SafeCopy2dBytes(sourceNextSyncCommittee.Pubkeys),
		},
		LatestExecutionPayloadHeader: &enginev1.ExecutionPayloadHeaderCapella{
			ParentHash:       bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.ParentHash),
			FeeRecipient:     bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.FeeRecipient),
			StateRoot:        bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.StateRoot),
			ReceiptsRoot:     bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.ReceiptsRoot),
			LogsBloom:        bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.LogsBloom),
			PrevRandao:       bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.PrevRandao),
			BlockNumber:      sourceLatestExecutionPayloadHeader.BlockNumber,
			GasLimit:         sourceLatestExecutionPayloadHeader.GasLimit,
			GasUsed:          sourceLatestExecutionPayloadHeader.GasUsed,
			Timestamp:        sourceLatestExecutionPayloadHeader.Timestamp,
			ExtraData:        bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.ExtraData),
			BaseFeePerGas:    bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.BaseFeePerGas),
			BlockHash:        bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.BlockHash),
			TransactionsRoot: bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.TransactionsRoot),
			WithdrawalsRoot:  bytesutil.SafeCopyBytes(sourceLatestExecutionPayloadHeader.WithdrawalsRoot),
		},
		NextWithdrawalIndex:          sourceNextWithdrawalIndex,
		NextWithdrawalValidatorIndex: sourceNextWithdrawalValIndex,
		HistoricalSummaries:          sourceHistoricalSummaries,
		HistoricalRoots:              hRoots,
	}

	return result, nil
}

// V1Alpha1SignedContributionAndProofToV1 converts a v1alpha1 SignedContributionAndProof object to its v1 equivalent.
func V1Alpha1SignedContributionAndProofToV1(alphaContribution *zondpbalpha.SignedContributionAndProof) *zondpbv1.SignedContributionAndProof {
	result := &zondpbv1.SignedContributionAndProof{
		Message: &zondpbv1.ContributionAndProof{
			AggregatorIndex: alphaContribution.Message.AggregatorIndex,
			Contribution: &zondpbv1.SyncCommitteeContribution{
				Slot:              alphaContribution.Message.Contribution.Slot,
				BeaconBlockRoot:   alphaContribution.Message.Contribution.BlockRoot,
				SubcommitteeIndex: alphaContribution.Message.Contribution.SubcommitteeIndex,
				AggregationBits:   alphaContribution.Message.Contribution.AggregationBits,
				Signatures:        alphaContribution.Message.Contribution.Signatures,
			},
			SelectionProof: alphaContribution.Message.SelectionProof,
		},
		Signature: alphaContribution.Signature,
	}
	return result
}

// V1SignedDilithiumToExecutionChangeToV1Alpha1 converts a V1 SignedDilithiumToExecutionChange to its v1alpha1 equivalent.
func V1SignedDilithiumToExecutionChangeToV1Alpha1(change *zondpbv1.SignedDilithiumToExecutionChange) *zondpbalpha.SignedDilithiumToExecutionChange {
	return &zondpbalpha.SignedDilithiumToExecutionChange{
		Message: &zondpbalpha.DilithiumToExecutionChange{
			ValidatorIndex:      change.Message.ValidatorIndex,
			FromDilithiumPubkey: bytesutil.SafeCopyBytes(change.Message.FromDilithiumPubkey),
			ToExecutionAddress:  bytesutil.SafeCopyBytes(change.Message.ToExecutionAddress),
		},
		Signature: bytesutil.SafeCopyBytes(change.Signature),
	}
}

// V1Alpha1SignedDilithiumToExecChangeToV1 converts a v1alpha1 SignedDilithiumToExecutionChange object to its v1 equivalent.
func V1Alpha1SignedDilithiumToExecChangeToV1(alphaChange *zondpbalpha.SignedDilithiumToExecutionChange) *zondpbv1.SignedDilithiumToExecutionChange {
	result := &zondpbv1.SignedDilithiumToExecutionChange{
		Message: &zondpbv1.DilithiumToExecutionChange{
			ValidatorIndex:      alphaChange.Message.ValidatorIndex,
			FromDilithiumPubkey: bytesutil.SafeCopyBytes(alphaChange.Message.FromDilithiumPubkey),
			ToExecutionAddress:  bytesutil.SafeCopyBytes(alphaChange.Message.ToExecutionAddress),
		},
		Signature: bytesutil.SafeCopyBytes(alphaChange.Signature),
	}
	return result
}
