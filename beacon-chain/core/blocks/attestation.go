package blocks

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/dilithium"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	"go.opencensus.io/trace"
)

// VerifyAttestationNoVerifySignatures verifies the attestation without verifying the attestation signatures. This is
// used before processing attestation with the beacon state.
func VerifyAttestationNoVerifySignatures(
	ctx context.Context,
	beaconState state.ReadOnlyBeaconState,
	att *zondpb.Attestation,
) error {
	ctx, span := trace.StartSpan(ctx, "core.VerifyAttestationNoVerifySignatures")
	defer span.End()

	if err := helpers.ValidateNilAttestation(att); err != nil {
		return err
	}
	currEpoch := time.CurrentEpoch(beaconState)
	prevEpoch := time.PrevEpoch(beaconState)
	data := att.Data
	if data.Target.Epoch != prevEpoch && data.Target.Epoch != currEpoch {
		return fmt.Errorf(
			"expected target epoch (%d) to be the previous epoch (%d) or the current epoch (%d)",
			data.Target.Epoch,
			prevEpoch,
			currEpoch,
		)
	}

	if data.Target.Epoch == currEpoch {
		if !beaconState.MatchCurrentJustifiedCheckpoint(data.Source) {
			return errors.New("source check point not equal to current justified checkpoint")
		}
	} else {
		if !beaconState.MatchPreviousJustifiedCheckpoint(data.Source) {
			return errors.New("source check point not equal to previous justified checkpoint")
		}
	}

	if err := helpers.ValidateSlotTargetEpoch(att.Data); err != nil {
		return err
	}

	s := att.Data.Slot
	minInclusionCheck := s+params.BeaconConfig().MinAttestationInclusionDelay <= beaconState.Slot()
	if !minInclusionCheck {
		return fmt.Errorf(
			"attestation slot %d + inclusion delay %d > state slot %d",
			s,
			params.BeaconConfig().MinAttestationInclusionDelay,
			beaconState.Slot(),
		)
	}

	epochInclusionCheck := beaconState.Slot() <= s+params.BeaconConfig().SlotsPerEpoch
	if !epochInclusionCheck {
		return fmt.Errorf(
			"state slot %d > attestation slot %d + SLOTS_PER_EPOCH %d",
			beaconState.Slot(),
			s,
			params.BeaconConfig().SlotsPerEpoch,
		)
	}
	activeValidatorCount, err := helpers.ActiveValidatorCount(ctx, beaconState, att.Data.Target.Epoch)
	if err != nil {
		return err
	}
	c := helpers.SlotCommitteeCount(activeValidatorCount)
	if uint64(att.Data.CommitteeIndex) >= c {
		return fmt.Errorf("committee index %d >= committee count %d", att.Data.CommitteeIndex, c)
	}

	if err := helpers.VerifyAttestationBitfieldLengths(ctx, beaconState, att); err != nil {
		return errors.Wrap(err, "could not verify attestation bitfields")
	}

	// Verify attesting indices are correct.
	committee, err := helpers.BeaconCommitteeFromState(ctx, beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	if err != nil {
		return err
	}
	indexedAtt, err := attestation.ConvertToIndexed(ctx, att, committee)
	if err != nil {
		return err
	}

	return attestation.IsValidAttestationIndices(ctx, indexedAtt)
}

// VerifyAttestationSignatures converts and attestation into an indexed attestation and verifies
// the signatures in that attestation.
func VerifyAttestationSignatures(ctx context.Context, beaconState state.ReadOnlyBeaconState, att *zondpb.Attestation) error {
	if err := helpers.ValidateNilAttestation(att); err != nil {
		return err
	}
	committee, err := helpers.BeaconCommitteeFromState(ctx, beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	if err != nil {
		return err
	}
	indexedAtt, err := attestation.ConvertToIndexed(ctx, att, committee)
	if err != nil {
		return err
	}
	return VerifyIndexedAttestation(ctx, beaconState, indexedAtt)
}

// VerifyIndexedAttestation determines the validity of an indexed attestation.
//
// Spec pseudocode definition:
//
//	def is_valid_indexed_attestation(state: BeaconState, indexed_attestation: IndexedAttestation) -> bool:
//	  """
//	  Check if ``indexed_attestation`` is not empty, has sorted and unique indices and has a valid aggregate signature.
//	  """
//	  # Verify indices are sorted and unique
//	  indices = indexed_attestation.attesting_indices
//	  if len(indices) == 0 or not indices == sorted(set(indices)):
//	      return False
//	  # Verify aggregate signature
//	  pubkeys = [state.validators[i].pubkey for i in indices]
//	  domain = get_domain(state, DOMAIN_BEACON_ATTESTER, indexed_attestation.data.target.epoch)
//	  signing_root = compute_signing_root(indexed_attestation.data, domain)
//	  return bls.FastAggregateVerify(pubkeys, signing_root, indexed_attestation.signature)
func VerifyIndexedAttestation(ctx context.Context, beaconState state.ReadOnlyBeaconState, indexedAtt *zondpb.IndexedAttestation) error {
	ctx, span := trace.StartSpan(ctx, "core.VerifyIndexedAttestation")
	defer span.End()

	if err := attestation.IsValidAttestationIndices(ctx, indexedAtt); err != nil {
		return err
	}
	domain, err := signing.Domain(
		beaconState.Fork(),
		indexedAtt.Data.Target.Epoch,
		params.BeaconConfig().DomainBeaconAttester,
		beaconState.GenesisValidatorsRoot(),
	)
	if err != nil {
		return err
	}
	indices := indexedAtt.AttestingIndices
	var pubkeys []dilithium.PublicKey
	for i := 0; i < len(indices); i++ {
		pubkeyAtIdx := beaconState.PubkeyAtIndex(primitives.ValidatorIndex(indices[i]))
		pk, err := dilithium.PublicKeyFromBytes(pubkeyAtIdx[:])
		if err != nil {
			return errors.Wrap(err, "could not deserialize validator public key")
		}
		pubkeys = append(pubkeys, pk)
	}
	return attestation.VerifyIndexedAttestationSigs(ctx, indexedAtt, pubkeys, domain)
}
