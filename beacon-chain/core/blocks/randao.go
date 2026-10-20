package blocks

import (
	"context"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/crypto/randao"
	"github.com/theQRL/qrysm/time/slots"
)

// ErrInvalidRandaoReveal is returned when a block's randao_reveal is not the
// pre-image of the proposer's current RANDAO commitment.
var ErrInvalidRandaoReveal = errors.New("randao reveal is not the pre-image of the proposer's randao commitment")

// ProcessRandao verifies the block proposer's RANDAO reveal against its
// hash-onion commitment, rotates the commitment, and mixes the reveal into
// the beacon state's randao mix for the current epoch.
//
// QRL deviates from the Ethereum spec here. ML-DSA-87 signatures are not
// unique, so hash(signature) is not a value the proposer cannot bias. Instead
// each validator commits to the top of a hash chain in its deposit and every
// block it proposes reveals the next pre-image:
//
//	def process_randao(state: BeaconState, body: BeaconBlockBody) -> None:
//	  epoch = get_current_epoch(state)
//	  proposer = state.validators[get_beacon_proposer_index(state)]
//	  # Verify RANDAO reveal against the proposer's commitment
//	  assert hash(body.randao_reveal) == proposer.randao_commitment
//	  # The reveal becomes the commitment for the proposer's next block
//	  proposer.randao_commitment = body.randao_reveal
//	  # Mix in RANDAO reveal
//	  mix = xor(get_randao_mix(state, epoch), body.randao_reveal)
//	  state.randao_mixes[epoch % EPOCHS_PER_HISTORICAL_VECTOR] = mix
//
// The reveal itself, not its hash, is mixed in: its hash is the commitment
// and is public before the block is.
func ProcessRandao(
	ctx context.Context,
	beaconState state.BeaconState,
	b interfaces.ReadOnlySignedBeaconBlock,
) (state.BeaconState, error) {
	if err := blocks.BeaconBlockIsNil(b); err != nil {
		return nil, err
	}
	reveal := b.Block().Body().RandaoReveal()
	if err := VerifyRandaoReveal(ctx, beaconState, reveal); err != nil {
		return nil, errors.Wrap(err, "could not verify block randao")
	}
	beaconState, err := ProcessRandaoNoVerify(ctx, beaconState, reveal)
	if err != nil {
		return nil, errors.Wrap(err, "could not process randao")
	}
	return beaconState, nil
}

// VerifyRandaoReveal checks that reveal is the pre-image of the current
// proposer's RANDAO commitment in beaconState. It does not modify the state.
func VerifyRandaoReveal(
	ctx context.Context,
	beaconState state.ReadOnlyBeaconState,
	reveal [fieldparams.RandaoRevealLength]byte,
) error {
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, beaconState)
	if err != nil {
		return errors.Wrap(err, "could not get beacon proposer index")
	}
	proposer, err := beaconState.ValidatorAtIndexReadOnly(proposerIdx)
	if err != nil {
		return errors.Wrap(err, "could not get proposer")
	}
	if !randao.Verify(reveal, proposer.RandaoCommitment()) {
		return ErrInvalidRandaoReveal
	}
	return nil
}

// ProcessRandaoNoVerify rotates the proposer's RANDAO commitment to the
// reveal and XORs the reveal into the state's randao mix for the current
// epoch, without checking the reveal against the commitment.
//
// WARNING: callers must verify the reveal with VerifyRandaoReveal (or use
// ProcessRandao) unless the block is known to be valid or only its post-state
// root is wanted.
func ProcessRandaoNoVerify(
	ctx context.Context,
	beaconState state.BeaconState,
	reveal [fieldparams.RandaoRevealLength]byte,
) (state.BeaconState, error) {
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, beaconState)
	if err != nil {
		return nil, errors.Wrap(err, "could not get beacon proposer index")
	}
	proposer, err := beaconState.ValidatorAtIndex(proposerIdx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get proposer")
	}
	proposer.RandaoCommitment = reveal[:]
	if err := beaconState.UpdateValidatorAtIndex(proposerIdx, proposer); err != nil {
		return nil, errors.Wrap(err, "could not update proposer randao commitment")
	}

	currentEpoch := slots.ToEpoch(beaconState.Slot())
	latestMixesLength := params.BeaconConfig().EpochsPerHistoricalVector
	latestMixSlice, err := beaconState.RandaoMixAtIndex(uint64(currentEpoch % latestMixesLength))
	if err != nil {
		return nil, err
	}
	if len(reveal) != len(latestMixSlice) {
		return nil, errors.New("randao reveal length doesn't match latestMixSlice length")
	}
	for i, x := range reveal {
		latestMixSlice[i] ^= x
	}
	if err := beaconState.UpdateRandaoMixesAtIndex(uint64(currentEpoch%latestMixesLength), [32]byte(latestMixSlice)); err != nil {
		return nil, err
	}
	return beaconState, nil
}
