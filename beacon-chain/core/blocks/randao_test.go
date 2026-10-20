package blocks_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/crypto/randao"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProcessRandao_IncorrectProposerFailsVerification(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	// We fetch the proposer's index as that is whom the RANDAO will be verified against.
	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
	require.NoError(t, err)
	proposer, err := beaconState.ValidatorAtIndexReadOnly(proposerIdx)
	require.NoError(t, err)
	// We reveal the previous validator's onion layer instead of the proposer's.
	other, err := beaconState.ValidatorAtIndexReadOnly(proposerIdx - 1)
	require.NoError(t, err)
	reveal, err := util.RandaoRevealForKey(privKeys[proposerIdx-1], other.RandaoCommitment())
	require.NoError(t, err)
	require.Equal(t, false, randao.Verify(bytesutil.ToBytes32(reveal), proposer.RandaoCommitment()))

	b := util.NewBeaconBlockZond()
	b.Block = &qrysmpb.BeaconBlockZond{
		Body: &qrysmpb.BeaconBlockBodyZond{
			RandaoReveal: reveal,
		},
	}

	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = blocks.ProcessRandao(context.Background(), beaconState, wsb)
	assert.ErrorContains(t, blocks.ErrInvalidRandaoReveal.Error(), err)
}

func TestProcessRandao_RevealVerifiesRotatesCommitmentAndUpdatesMixes(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)

	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
	require.NoError(t, err)
	before, err := beaconState.ValidatorAtIndexReadOnly(proposerIdx)
	require.NoError(t, err)
	oldCommitment := before.RandaoCommitment()

	epoch := time.CurrentEpoch(beaconState)
	oldMix, err := beaconState.RandaoMixAtIndex(uint64(epoch % params.BeaconConfig().EpochsPerHistoricalVector))
	require.NoError(t, err)
	reveal, err := util.RandaoReveal(beaconState, epoch, privKeys)
	require.NoError(t, err)
	require.Equal(t, true, randao.Verify(bytesutil.ToBytes32(reveal), oldCommitment))

	b := util.NewBeaconBlockZond()
	b.Block = &qrysmpb.BeaconBlockZond{
		Body: &qrysmpb.BeaconBlockBodyZond{
			RandaoReveal: reveal,
		},
	}
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	newState, err := blocks.ProcessRandao(
		context.Background(),
		beaconState,
		wsb,
	)
	require.NoError(t, err, "Unexpected error processing block randao")

	currentEpoch := time.CurrentEpoch(beaconState)
	mix := newState.RandaoMixes()[currentEpoch%params.BeaconConfig().EpochsPerHistoricalVector]
	assert.DeepNotEqual(t, oldMix, mix, "Expected mix to be updated by randao reveal")
	// The reveal itself, not its (public) hash, is mixed in.
	want := make([]byte, 32)
	for i := range want {
		want[i] = oldMix[i] ^ reveal[i]
	}
	assert.DeepEqual(t, want, mix)

	// The reveal is the proposer's new commitment.
	after, err := newState.ValidatorAtIndexReadOnly(proposerIdx)
	require.NoError(t, err)
	assert.Equal(t, bytesutil.ToBytes32(reveal), after.RandaoCommitment())
	assert.NotEqual(t, oldCommitment, after.RandaoCommitment())

	// Replaying the same reveal against the rotated commitment fails.
	_, err = blocks.ProcessRandao(context.Background(), newState, wsb)
	assert.ErrorContains(t, blocks.ErrInvalidRandaoReveal.Error(), err)

	// The next layer opens the rotated commitment.
	next, err := util.RandaoRevealForKey(privKeys[proposerIdx], after.RandaoCommitment())
	require.NoError(t, err)
	require.NoError(t, blocks.VerifyRandaoReveal(context.Background(), newState, bytesutil.ToBytes32(next)))
}

func TestVerifyRandaoReveal_ZeroRevealNeverVerifies(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, 16)
	err := blocks.VerifyRandaoReveal(context.Background(), beaconState, [32]byte{})
	assert.ErrorContains(t, blocks.ErrInvalidRandaoReveal.Error(), err)
}
