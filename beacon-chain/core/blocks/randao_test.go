package blocks_test

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProcessRandao_IncorrectProposerFailsVerification(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 100)
	// We fetch the proposer's index as that is whom the RANDAO will be verified against.
	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
	require.NoError(t, err)
	epoch := primitives.Epoch(0)
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint64(buf, uint64(epoch))
	domain, err := signing.Domain(beaconState.Fork(), epoch, params.BeaconConfig().DomainRandao, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := (&zondpb.SigningData{ObjectRoot: buf, Domain: domain}).HashTreeRoot()
	require.NoError(t, err)
	// We make the previous validator's index sign the message instead of the proposer.
	epochSignature := privKeys[proposerIdx-1].Sign(root[:])
	b := util.NewBeaconBlockCapella()
	b.Block = &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			RandaoReveal: epochSignature.Marshal(),
		},
	}

	want := "block randao: signature did not verify"
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = blocks.ProcessRandao(context.Background(), beaconState, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessRandao_SignatureVerifiesAndUpdatesLatestStateMixes(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 100)

	epoch := time.CurrentEpoch(beaconState)
	epochSignature, err := util.RandaoReveal(beaconState, epoch, privKeys)
	require.NoError(t, err)

	b := util.NewBeaconBlockCapella()
	b.Block = &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			RandaoReveal: epochSignature,
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
	assert.DeepNotEqual(t, params.BeaconConfig().ZeroHash[:], mix, "Expected empty signature to be overwritten by randao reveal")
}

func TestRandaoSignatureSet_OK(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 100)

	epoch := time.CurrentEpoch(beaconState)
	epochSignature, err := util.RandaoReveal(beaconState, epoch, privKeys)
	require.NoError(t, err)

	block := &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			RandaoReveal: epochSignature,
		},
	}

	set, err := blocks.RandaoSignatureBatch(context.Background(), beaconState, block.Body.RandaoReveal)
	require.NoError(t, err)
	verified, err := set.Verify()
	require.NoError(t, err)
	assert.Equal(t, true, verified, "Unable to verify randao signature set")
}
