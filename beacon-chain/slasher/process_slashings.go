package slasher

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/network/forks"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
)

// errHeaderVerificationUnavailable is returned when a block header signature
// cannot be checked right now (no head state, or a proposer index the head
// registry does not hold yet). It says nothing about the signature itself.
var errHeaderVerificationUnavailable = errors.New("block header signature cannot be verified at the moment")

// Verifies attester slashings, logs them, and submits them to the slashing operations pool
// in the beacon node if they pass validation.
func (s *Service) processAttesterSlashings(
	ctx context.Context, slashings map[[fieldparams.RootLength]byte]*qrysmpb.AttesterSlashing,
) error {
	var beaconState state.BeaconState
	var err error
	if len(slashings) > 0 {
		beaconState, err = s.serviceCfg.HeadStateFetcher.HeadState(ctx)
		if err != nil {
			return err
		}
	}
	for _, sl := range slashings {
		if err := s.verifyAttSignature(ctx, sl.Attestation_1); err != nil {
			log.WithError(err).WithField("a", sl.Attestation_1).Warn(
				"Invalid signature for attestation in detected slashing offense",
			)
			continue
		}
		if err := s.verifyAttSignature(ctx, sl.Attestation_2); err != nil {
			log.WithError(err).WithField("b", sl.Attestation_2).Warn(
				"Invalid signature for attestation in detected slashing offense",
			)
			continue
		}

		// Log the slashing event and insert into the beacon node's operations pool.
		logAttesterSlashing(sl)
		if err := s.serviceCfg.SlashingPoolInserter.InsertAttesterSlashing(
			ctx, beaconState, sl,
		); err != nil {
			log.WithError(err).Error("Could not insert attester slashing into operations pool")
		}
	}
	return nil
}

// Verifies proposer slashings, logs them, and submits them to the slashing operations pool
// in the beacon node if they pass validation.
func (s *Service) processProposerSlashings(ctx context.Context, slashings []*qrysmpb.ProposerSlashing) error {
	var beaconState state.BeaconState
	var err error
	if len(slashings) > 0 {
		beaconState, err = s.serviceCfg.HeadStateFetcher.HeadState(ctx)
		if err != nil {
			return err
		}
	}
	for _, sl := range slashings {
		if err := s.verifyBlockSignature(ctx, sl.Header_1); err != nil {
			log.WithError(err).WithField("a", sl.Header_1).Warn(
				"Invalid signature for block header in detected slashing offense",
			)
			continue
		}
		if err := s.verifyBlockSignature(ctx, sl.Header_2); err != nil {
			log.WithError(err).WithField("b", sl.Header_2).Warn(
				"Invalid signature for block header in detected slashing offense",
			)
			continue
		}
		// Log the slashing event and insert into the beacon node's operations pool.
		logProposerSlashing(sl)
		if err := s.serviceCfg.SlashingPoolInserter.InsertProposerSlashing(ctx, beaconState, sl); err != nil {
			log.WithError(err).Error("Could not insert proposer slashing into operations pool")
		}
	}
	return nil
}

// verifyBlockSignature checks a block header's proposer signature against the
// head state. A validator's public key is immutable once registered and the
// signing domain follows from the header's epoch, so the parent state is not
// needed: that gives a definite verdict whenever the head state is available
// and avoids regenerating arbitrary parent states for junk headers.
func (s *Service) verifyBlockSignature(ctx context.Context, header *qrysmpb.SignedBeaconBlockHeader) error {
	if header == nil || header.Header == nil {
		return errors.New("nil block header cannot be verified")
	}
	if s.serviceCfg.HeadStateFetcher == nil {
		return errors.Wrap(errHeaderVerificationUnavailable, "no head state fetcher configured")
	}
	headState, err := s.serviceCfg.HeadStateFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return errors.Wrap(errHeaderVerificationUnavailable, err.Error())
	}
	if headState == nil || headState.IsNil() {
		return errors.Wrap(errHeaderVerificationUnavailable, "nil head state")
	}
	idx := header.Header.ProposerIndex
	if uint64(idx) >= uint64(headState.NumValidators()) {
		return errors.Wrapf(errHeaderVerificationUnavailable, "proposer index %d is not in the head registry of %d validators", idx, headState.NumValidators())
	}
	epoch := slots.ToEpoch(header.Header.Slot)
	fork, err := forks.Fork(epoch)
	if err != nil {
		return err
	}
	domain, err := signing.Domain(fork, epoch, params.BeaconConfig().DomainBeaconProposer, headState.GenesisValidatorsRoot())
	if err != nil {
		return err
	}
	pub := headState.PubkeyAtIndex(idx)
	return signing.VerifyBlockHeaderSigningRoot(header.Header, pub[:], header.Signature, domain)
}

func (s *Service) verifyAttSignature(ctx context.Context, att *qrysmpb.IndexedAttestation) error {
	preState, err := s.serviceCfg.AttestationStateFetcher.AttestationTargetState(ctx, att.Data.Target)
	if err != nil {
		return err
	}
	return blocks.VerifyIndexedAttestation(ctx, preState, att)
}
