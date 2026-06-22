package sync

import (
	"context"
	"fmt"
	"slices"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/blockchain"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	"github.com/theQRL/qrysm/beacon-chain/core/feed/operation"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/monitoring/tracing"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	qrysmTime "github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
)

// validateAggregateAndProof verifies the aggregated signature and the selection proof is valid before forwarding to the
// network and downstream services.
func (s *Service) validateAggregateAndProof(ctx context.Context, pid peer.ID, msg *pubsub.Message) (pubsub.ValidationResult, error) {
	receivedTime := qrysmTime.Now()
	if pid == s.cfg.p2p.PeerID() {
		return pubsub.ValidationAccept, nil
	}

	ctx, span := trace.StartSpan(ctx, "sync.validateAggregateAndProof")
	defer span.End()

	// To process the following it requires the recent blocks to be present in the database, so we'll skip
	// validating or processing aggregated attestations until fully synced.
	if s.cfg.initialSync.Syncing() {
		return pubsub.ValidationIgnore, nil
	}

	raw, err := s.decodePubsubMessage(msg)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationReject, err
	}
	m, ok := raw.(*qrysmpb.SignedAggregateAttestationAndProof)
	if !ok {
		return pubsub.ValidationReject, errors.Errorf("invalid message type: %T", raw)
	}
	if m.Message == nil {
		return pubsub.ValidationReject, errNilMessage
	}
	if err := helpers.ValidateNilAttestation(m.Message.Aggregate); err != nil {
		return pubsub.ValidationReject, err
	}
	// Do not process slot 0 aggregates.
	if m.Message.Aggregate.Data.Slot == 0 {
		return pubsub.ValidationIgnore, nil
	}

	// Broadcast the aggregated attestation on a feed to notify other services in the beacon node
	// of a received aggregated attestation.
	s.cfg.attestationNotifier.OperationFeed().Send(&feed.Event{
		Type: operation.AggregatedAttReceived,
		Data: &operation.AggregatedAttReceivedData{
			Attestation: m.Message,
		},
	})

	if err := helpers.ValidateSlotTargetEpoch(m.Message.Aggregate.Data); err != nil {
		return pubsub.ValidationReject, err
	}

	// Attestation's slot is within ATTESTATION_PROPAGATION_SLOT_RANGE and early attestation
	// processing tolerance.
	if err := helpers.ValidateAttestationTime(
		m.Message.Aggregate.Data.Slot,
		s.cfg.clock.GenesisTime(),
		earlyAttestationProcessingTolerance,
	); err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}

	// Verify this is the first aggregate received from the aggregator with index and slot.
	if s.hasSeenAggregatorIndexEpoch(m.Message.Aggregate.Data.Target.Epoch, m.Message.AggregatorIndex) {
		return pubsub.ValidationIgnore, nil
	}
	// Check that the block being voted on isn't invalid.
	if s.hasBadBlock(bytesutil.ToBytes32(m.Message.Aggregate.Data.BeaconBlockRoot)) ||
		s.hasBadBlock(bytesutil.ToBytes32(m.Message.Aggregate.Data.Target.Root)) ||
		s.hasBadBlock(bytesutil.ToBytes32(m.Message.Aggregate.Data.Source.Root)) {
		attBadBlockCount.Inc()
		return pubsub.ValidationReject, errors.New("bad block referenced in attestation data")
	}

	// Verify aggregate attestation has not already been seen via aggregate gossip, within a block, or through the creation locally.
	seen, err := s.cfg.attPool.HasAggregatedAttestation(m.Message.Aggregate)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}
	if seen {
		return pubsub.ValidationIgnore, nil
	}
	if !s.validateBlockInAttestation(ctx, m) {
		return pubsub.ValidationIgnore, nil
	}

	validationRes, err := s.validateAggregatedAtt(ctx, m)
	if validationRes != pubsub.ValidationAccept {
		return validationRes, err
	}

	if first := s.setAggregatorIndexEpochSeen(m.Message.Aggregate.Data.Target.Epoch, m.Message.AggregatorIndex); !first {
		return pubsub.ValidationIgnore, nil
	}

	msg.ValidatorData = m

	aggregateAttestationVerificationGossipSummary.Observe(float64(qrysmTime.Since(receivedTime).Milliseconds()))

	return pubsub.ValidationAccept, nil
}

func (s *Service) validateAggregatedAtt(ctx context.Context, signed *qrysmpb.SignedAggregateAttestationAndProof) (pubsub.ValidationResult, error) {
	ctx, span := trace.StartSpan(ctx, "sync.validateAggregatedAtt")
	defer span.End()

	// Verify attestation target root is consistent with the head root.
	// This verification is not in the spec, however we guard against it as it opens us up
	// to weird edge cases during verification. The attestation technically could be used to add value to a block,
	// but it's invalid in the spirit of the protocol. Here we choose safety over profit.
	if err := s.cfg.chain.VerifyLmdFfgConsistency(ctx, signed.Message.Aggregate); err != nil {
		tracing.AnnotateError(span, err)
		attBadLmdConsistencyCount.Inc()
		return pubsub.ValidationReject, err
	}

	// Verify current finalized checkpoint is an ancestor of the block defined by the attestation's beacon block root.
	if !s.cfg.chain.InForkchoice(bytesutil.ToBytes32(signed.Message.Aggregate.Data.BeaconBlockRoot)) {
		tracing.AnnotateError(span, blockchain.ErrNotDescendantOfFinalized)
		return pubsub.ValidationIgnore, blockchain.ErrNotDescendantOfFinalized
	}

	bs, err := s.cfg.chain.AttestationTargetState(ctx, signed.Message.Aggregate.Data.Target)
	if err != nil {
		tracing.AnnotateError(span, err)
		return pubsub.ValidationIgnore, err
	}

	// Verify validator index is within the beacon committee and that the
	// aggregate satisfies the spec REJECT preconditions (committee index in
	// range, bitfield length matches committee size, at least one attesting bit).
	result, err := s.validateIndexInCommittee(ctx, bs, signed.Message.Aggregate, signed.Message.AggregatorIndex)
	if result != pubsub.ValidationAccept {
		wrappedErr := errors.Wrapf(err, "Could not validate index in committee")
		tracing.AnnotateError(span, wrappedErr)
		return result, wrappedErr
	}

	// Verify selection proof reflects to the right validator.
	selectionSigSet, err := validateSelectionIndex(ctx, bs, signed.Message.Aggregate.Data, signed.Message.AggregatorIndex, signed.Message.SelectionProof)
	if err != nil {
		wrappedErr := errors.Wrapf(err, "Could not validate selection for validator %d", signed.Message.AggregatorIndex)
		tracing.AnnotateError(span, wrappedErr)
		attBadSelectionProofCount.Inc()
		return pubsub.ValidationReject, wrappedErr
	}

	// Verify selection signature, aggregator signature and attestation signatures are valid.
	// We use batch verify here to save compute.
	aggregatorSigSet, err := aggSigSet(bs, signed)
	if err != nil {
		wrappedErr := errors.Wrapf(err, "Could not get aggregator sig set %d", signed.Message.AggregatorIndex)
		tracing.AnnotateError(span, wrappedErr)
		return pubsub.ValidationIgnore, wrappedErr
	}
	attSigSet, err := blocks.AttestationSignatureBatch(ctx, bs, []*qrysmpb.Attestation{signed.Message.Aggregate})
	if err != nil {
		wrappedErr := errors.Wrapf(err, "Could not verify attestation signatures %d", signed.Message.AggregatorIndex)
		tracing.AnnotateError(span, wrappedErr)
		return pubsub.ValidationIgnore, wrappedErr
	}
	set := ml_dsa_87.NewSet()
	set.Join(selectionSigSet).Join(aggregatorSigSet).Join(attSigSet)

	return s.validateWithBatchVerifier(ctx, "aggregate", set)
}

func (s *Service) validateBlockInAttestation(ctx context.Context, satt *qrysmpb.SignedAggregateAttestationAndProof) bool {
	a := satt.Message
	// Verify the block being voted and the processed state is in beaconDB. The block should have passed validation if it's in the beaconDB.
	blockRoot := bytesutil.ToBytes32(a.Aggregate.Data.BeaconBlockRoot)
	if !s.hasBlockAndState(ctx, blockRoot) {
		// A node doesn't have the block, it'll request from peer while saving the pending attestation to a queue.
		s.savePendingAtt(satt)
		return false
	}
	return true
}

// Returns true if the node has received aggregate for the aggregator with index and target epoch.
func (s *Service) hasSeenAggregatorIndexEpoch(epoch primitives.Epoch, aggregatorIndex primitives.ValidatorIndex) bool {
	s.seenAggregatedAttestationLock.RLock()
	defer s.seenAggregatedAttestationLock.RUnlock()
	if s.seenAggregatedAttestationByEpoch == nil {
		return false
	}
	byValidator, ok := s.seenAggregatedAttestationByEpoch[epoch]
	if !ok {
		return false
	}
	_, seen := byValidator[aggregatorIndex]
	return seen
}

// Set aggregate's aggregator index target epoch as seen.
// Returns true if this is the first time seeing this aggregator index and epoch.
func (s *Service) setAggregatorIndexEpochSeen(epoch primitives.Epoch, aggregatorIndex primitives.ValidatorIndex) bool {
	s.seenAggregatedAttestationLock.Lock()
	defer s.seenAggregatedAttestationLock.Unlock()
	if s.seenAggregatedAttestationByEpoch == nil {
		s.seenAggregatedAttestationByEpoch = make(map[primitives.Epoch]map[primitives.ValidatorIndex]struct{})
		s.seenAggregatedAttestationHasMaxEpoch = false
	}
	byValidator, ok := s.seenAggregatedAttestationByEpoch[epoch]
	if !ok {
		byValidator = make(map[primitives.ValidatorIndex]struct{})
		s.seenAggregatedAttestationByEpoch[epoch] = byValidator
	}
	if _, seen := byValidator[aggregatorIndex]; seen {
		return false
	}
	byValidator[aggregatorIndex] = struct{}{}

	if !s.seenAggregatedAttestationHasMaxEpoch || epoch > s.seenAggregatedAttestationMaxEpoch {
		s.seenAggregatedAttestationMaxEpoch = epoch
		s.seenAggregatedAttestationHasMaxEpoch = true
	}
	s.pruneSeenAggregatedAttestationEpochsLocked()
	return true
}

// pruneSeenAggregatedAttestationEpochsLocked retains only the latest two epochs
// (max seen epoch and max-1) so dedup history follows protocol timing rather
// than cache pressure. Caller must hold seenAggregatedAttestationLock for write.
func (s *Service) pruneSeenAggregatedAttestationEpochsLocked() {
	if !s.seenAggregatedAttestationHasMaxEpoch {
		return
	}
	maxSeenEpoch := s.seenAggregatedAttestationMaxEpoch
	if maxSeenEpoch < 2 {
		return
	}
	minRetainedEpoch := maxSeenEpoch - 1
	for epoch := range s.seenAggregatedAttestationByEpoch {
		if epoch < minRetainedEpoch {
			delete(s.seenAggregatedAttestationByEpoch, epoch)
		}
	}
}

// validateIndexInCommittee validates the bitfield is correct and the
// aggregator's index is within the beacon committee. Implements the
// following consensus-spec REJECT conditions:
//   - committee index < get_committee_count_per_slot
//   - len(aggregation_bits) == len(beacon_committee)
//   - aggregate has at least one attesting bit
//   - aggregator's validator index is within the committee
//
// Internal lookup errors return ValidationIgnore so the peer isn't downscored
// for our own state-access failures.
func (s *Service) validateIndexInCommittee(ctx context.Context, bs state.ReadOnlyBeaconState, a *qrysmpb.Attestation, validatorIndex primitives.ValidatorIndex) (pubsub.ValidationResult, error) {
	ctx, span := trace.StartSpan(ctx, "sync.validateIndexInCommittee")
	defer span.End()

	if _, result, err := s.validateCommitteeIndex(ctx, a, bs); result != pubsub.ValidationAccept {
		return result, err
	}

	committee, result, err := s.validateBitLength(ctx, a, bs)
	if result != pubsub.ValidationAccept {
		return result, err
	}

	if a.AggregationBits.Count() == 0 {
		return pubsub.ValidationReject, errors.New("no attesting indices")
	}

	if withinCommittee := slices.Contains(committee, validatorIndex); !withinCommittee {
		return pubsub.ValidationReject, fmt.Errorf("validator index %d is not within the committee: %v",
			validatorIndex, committee)
	}
	return pubsub.ValidationAccept, nil
}

// This validates selection proof by validating it's from the correct validator index of the slot.
// It does not verify the selection proof, it returns the signature set of selection proof which can be used for batch verify.
func validateSelectionIndex(
	ctx context.Context,
	bs state.ReadOnlyBeaconState,
	data *qrysmpb.AttestationData,
	validatorIndex primitives.ValidatorIndex,
	proof []byte,
) (*ml_dsa_87.SignatureBatch, error) {
	ctx, span := trace.StartSpan(ctx, "sync.validateSelectionIndex")
	defer span.End()

	committee, err := helpers.BeaconCommitteeFromState(ctx, bs, data.Slot, data.CommitteeIndex)
	if err != nil {
		return nil, err
	}
	aggregator, err := helpers.IsAggregator(uint64(len(committee)), proof)
	if err != nil {
		return nil, err
	}
	if !aggregator {
		return nil, fmt.Errorf("validator is not an aggregator for slot %d", data.Slot)
	}

	domain := params.BeaconConfig().DomainSelectionProof
	epoch := slots.ToEpoch(data.Slot)

	v, err := bs.ValidatorAtIndex(validatorIndex)
	if err != nil {
		return nil, err
	}
	publicKey, err := ml_dsa_87.PublicKeyFromBytes(v.PublicKey)
	if err != nil {
		return nil, err
	}

	d, err := signing.Domain(bs.Fork(), epoch, domain, bs.GenesisValidatorsRoot())
	if err != nil {
		return nil, err
	}
	sszUint := primitives.SSZUint64(data.Slot)
	root, err := signing.ComputeSigningRoot(&sszUint, d)
	if err != nil {
		return nil, err
	}
	return &ml_dsa_87.SignatureBatch{
		Signatures:   [][][]byte{{proof}},
		PublicKeys:   [][]ml_dsa_87.PublicKey{{publicKey}},
		Messages:     [][32]byte{root},
		Descriptions: []string{signing.SelectionProof},
	}, nil
}

// This returns aggregator signature set which can be used to batch verify.
func aggSigSet(s state.ReadOnlyBeaconState, a *qrysmpb.SignedAggregateAttestationAndProof) (*ml_dsa_87.SignatureBatch, error) {
	v, err := s.ValidatorAtIndex(a.Message.AggregatorIndex)
	if err != nil {
		return nil, err
	}
	publicKey, err := ml_dsa_87.PublicKeyFromBytes(v.PublicKey)
	if err != nil {
		return nil, err
	}

	epoch := slots.ToEpoch(a.Message.Aggregate.Data.Slot)
	d, err := signing.Domain(s.Fork(), epoch, params.BeaconConfig().DomainAggregateAndProof, s.GenesisValidatorsRoot())
	if err != nil {
		return nil, err
	}
	root, err := signing.ComputeSigningRoot(a.Message, d)
	if err != nil {
		return nil, err
	}
	return &ml_dsa_87.SignatureBatch{
		Signatures:   [][][]byte{{a.Signature}},
		PublicKeys:   [][]ml_dsa_87.PublicKey{{publicKey}},
		Messages:     [][32]byte{root},
		Descriptions: []string{signing.AggregatorSignature},
	}, nil
}
