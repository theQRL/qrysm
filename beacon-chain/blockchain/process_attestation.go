package blockchain

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/time/slots"
	"go.opencensus.io/trace"
)

// OnAttestation is called whenever an attestation is received, verifies the attestation is valid and saves
// it to the DB. As a stateless function, this does not hold nor delay attestation based on the spec descriptions.
// The delay is handled by the caller in `processAttestations`. The caller must
// hold the forkchoice write lock.
//
// Spec pseudocode definition:
//
//	def on_attestation(store: Store, attestation: Attestation) -> None:
//	 """
//	 Run ``on_attestation`` upon receiving a new ``attestation`` from either within a block or directly on the wire.
//
//	 An ``attestation`` that is asserted as invalid may be valid at a later time,
//	 consider scheduling it for later processing in such case.
//	 """
//	 validate_on_attestation(store, attestation)
//	 store_target_checkpoint_state(store, attestation.data.target)
//
//	 # Get state at the `target` to fully validate attestation
//	 target_state = store.checkpoint_states[attestation.data.target]
//	 indexed_attestation = get_indexed_attestation(target_state, attestation)
//	 assert is_valid_indexed_attestation(target_state, indexed_attestation)
//
//	 # Update latest messages for attesting indices
//	 update_latest_messages(store, indexed_attestation.attesting_indices, attestation)
func (s *Service) OnAttestation(ctx context.Context, a *qrysmpb.Attestation, disparity time.Duration) error {
	return s.onAttestation(ctx, a, disparity, false)
}

// onAttestation retains the block-inclusion exemption from the gossip age
// limit while checking the same ancestry and target-state signatures.
func (s *Service) onAttestation(ctx context.Context, a *qrysmpb.Attestation, disparity time.Duration, fromBlock bool) error {
	ctx, span := trace.StartSpan(ctx, "blockChain.onAttestation")
	defer span.End()

	if err := helpers.ValidateNilAttestation(a); err != nil {
		return err
	}
	if err := helpers.ValidateSlotTargetEpoch(a.Data); err != nil {
		return err
	}
	tgt := qrysmpb.CopyCheckpoint(a.Data.Target)
	if fromBlock && (!s.cfg.ForkChoiceStore.HasNode(bytesutil.ToBytes32(tgt.Root)) ||
		!s.cfg.ForkChoiceStore.HasNode(bytesutil.ToBytes32(a.Data.BeaconBlockRoot))) {
		return attestationDependencyError{errors.New("included attestation blocks are unknown to forkchoice")}
	}

	// Retrieve attestation's data beacon block pre state. Advance pre state to latest epoch if necessary and
	// save it to the cache.
	baseState, err := s.getAttPreState(ctx, tgt)
	if err != nil {
		if errors.Is(err, ErrNotCheckpoint) {
			return err
		}
		return attestationDependencyError{err}
	}

	genesisTime := uint64(s.genesisTime.Unix())

	// Only gossip attestations must target the current or previous epoch.
	if !fromBlock {
		if err := verifyAttTargetEpoch(ctx, genesisTime, uint64(time.Now().Add(disparity).Unix()), tgt); err != nil {
			return err
		}
	}

	// Verify attestation beacon block is known and not from the future.
	if err := s.verifyBeaconBlock(ctx, a.Data); err != nil {
		return errors.Wrap(err, "could not verify attestation beacon block")
	}

	// The pool also contains deferred block attestations, which have not passed
	// gossip's forkchoice checks. Validate them before applying their votes.
	if err := s.verifyAttestationForkchoice(a); err != nil {
		return err
	}

	// Verify attestations can only affect the fork choice of subsequent slots.
	if err := slots.VerifyTime(genesisTime, a.Data.Slot+1, disparity); err != nil {
		return err
	}

	indices, err := verifiedAttestingIndices(ctx, baseState, a)
	if err != nil {
		return err
	}
	s.cfg.ForkChoiceStore.ProcessAttestation(ctx, indices, bytesutil.ToBytes32(a.Data.BeaconBlockRoot), a.Data.Target.Epoch)
	return nil
}

// verifiedAttestingIndices authenticates the voters using the target state's
// committee. Block inclusion and pool membership do not establish this: a valid
// containing block can use a different committee on a competing branch.
func verifiedAttestingIndices(ctx context.Context, targetState state.ReadOnlyBeaconState, a *qrysmpb.Attestation) ([]uint64, error) {
	committee, err := helpers.BeaconCommitteeFromState(ctx, targetState, a.Data.Slot, a.Data.CommitteeIndex)
	if err != nil {
		return nil, err
	}
	indexedAtt, err := attestation.ConvertToIndexed(ctx, a, committee)
	if err != nil {
		return nil, err
	}
	if err := blocks.VerifyIndexedAttestation(ctx, targetState, indexedAtt); err != nil {
		return nil, err
	}
	return indexedAtt.AttestingIndices, nil
}
