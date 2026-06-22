package util

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/time/slots"
)

func TestBlockSignature(t *testing.T) {
	beaconState, privKeys := DeterministicGenesisStateZond(t, 100)
	block, err := GenerateFullBlockZond(beaconState, privKeys, nil, 0)
	require.NoError(t, err)

	require.NoError(t, beaconState.SetSlot(beaconState.Slot()+1))
	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
	assert.NoError(t, err)

	assert.NoError(t, beaconState.SetSlot(beaconState.Slot()-1))
	epoch := slots.ToEpoch(block.Block.Slot)
	domain, err := signing.Domain(beaconState.Fork(), epoch, params.BeaconConfig().DomainBeaconProposer, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)

	signature, err := BlockSignature(beaconState, block.Block, privKeys)
	assert.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(block.Block, domain)
	require.NoError(t, err)

	pubKey, err := ml_dsa_87.PublicKeyFromBytes(privKeys[proposerIdx].PublicKey().Marshal())
	require.NoError(t, err)
	ok, err := ml_dsa_87.VerifySignature(signature.Marshal(), signingRoot, pubKey)
	require.NoError(t, err)
	assert.Equal(t, true, ok)
}

func TestRandaoReveal(t *testing.T) {
	beaconState, privKeys := DeterministicGenesisStateZond(t, 100)

	epoch := time.CurrentEpoch(beaconState)
	randaoReveal, err := RandaoReveal(beaconState, epoch, privKeys)
	assert.NoError(t, err)

	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
	assert.NoError(t, err)
	buf := make([]byte, fieldparams.RootLength)
	binary.LittleEndian.PutUint64(buf, uint64(epoch))
	// We make the previous validator's index sign the message instead of the proposer.
	sszUint := primitives.SSZUint64(epoch)
	domain, err := signing.Domain(beaconState.Fork(), epoch, params.BeaconConfig().DomainRandao, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(&sszUint, domain)
	require.NoError(t, err)

	pubKey, err := ml_dsa_87.PublicKeyFromBytes(privKeys[proposerIdx].PublicKey().Marshal())
	require.NoError(t, err)
	ok, err := ml_dsa_87.VerifySignature(randaoReveal, signingRoot, pubKey)
	require.NoError(t, err)
	assert.Equal(t, true, ok)
}
