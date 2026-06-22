package blocks_test

import (
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestVerifyBlockHeaderSignature(t *testing.T) {
	beaconState, err := util.NewBeaconStateZond()
	require.NoError(t, err)

	privKey, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	validators := make([]*qrysmpb.Validator, 1)
	validators[0] = &qrysmpb.Validator{
		PublicKey:             privKey.PublicKey().Marshal(),
		WithdrawalCredentials: make([]byte, 64),
		EffectiveBalance:      params.BeaconConfig().MaxEffectiveBalance,
	}
	err = beaconState.SetValidators(validators)
	require.NoError(t, err)

	// Sign the block header.
	blockHeader := util.HydrateSignedBeaconHeader(&qrysmpb.SignedBeaconBlockHeader{
		Header: &qrysmpb.BeaconBlockHeader{
			Slot:          0,
			ProposerIndex: 0,
		},
	})
	domain, err := signing.Domain(
		beaconState.Fork(),
		0,
		params.BeaconConfig().DomainBeaconProposer,
		beaconState.GenesisValidatorsRoot(),
	)
	require.NoError(t, err)
	htr, err := blockHeader.Header.HashTreeRoot()
	require.NoError(t, err)
	container := &qrysmpb.SigningData{
		ObjectRoot: htr[:],
		Domain:     domain,
	}
	require.NoError(t, err)
	signingRoot, err := container.HashTreeRoot()
	require.NoError(t, err)

	// Set the signature in the block header.
	blockHeader.Signature = privKey.Sign(signingRoot[:]).Marshal()

	// Sig should verify.
	err = blocks.VerifyBlockHeaderSignature(beaconState, blockHeader)
	require.NoError(t, err)
}

func TestVerifyBlockSignatureUsingCurrentFork(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	bState, keys := util.DeterministicGenesisStateZond(t, 100)
	blk := util.NewBeaconBlockZond()
	blk.Block.ProposerIndex = 0
	blk.Block.Slot = params.BeaconConfig().SlotsPerEpoch * 100
	fData := &qrysmpb.Fork{
		Epoch:           100,
		CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		PreviousVersion: params.BeaconConfig().GenesisForkVersion,
	}
	domain, err := signing.Domain(fData, 100, params.BeaconConfig().DomainBeaconProposer, bState.GenesisValidatorsRoot())
	assert.NoError(t, err)
	blkRoot, err := blk.Block.HashTreeRoot()
	assert.NoError(t, err)
	rt, err := signing.ComputeSigningRoot(blk.Block, domain)
	assert.NoError(t, err)
	sig := keys[0].Sign(rt[:]).Marshal()
	blk.Signature = sig
	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	assert.NoError(t, blocks.VerifyBlockSignatureUsingCurrentFork(bState, wsb, blkRoot))
}

func TestVerifyBlockSignatureUsingCurrentFork_InvalidSignature(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	bState, keys := util.DeterministicGenesisStateZond(t, 100)
	blk := util.NewBeaconBlockZond()
	blk.Block.ProposerIndex = 0
	blk.Block.Slot = params.BeaconConfig().SlotsPerEpoch * 100
	fData := &qrysmpb.Fork{
		Epoch:           100,
		CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		PreviousVersion: params.BeaconConfig().GenesisForkVersion,
	}
	domain, err := signing.Domain(fData, 100, params.BeaconConfig().DomainBeaconProposer, bState.GenesisValidatorsRoot())
	assert.NoError(t, err)
	rt, err := signing.ComputeSigningRoot(blk.Block, domain)
	assert.NoError(t, err)

	// Sign with the wrong key (proposer index is 0, but using key 1).
	blk.Signature = keys[1].Sign(rt[:]).Marshal()
	wsb, err := consensusblocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	blkRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)

	err = blocks.VerifyBlockSignatureUsingCurrentFork(bState, wsb, blkRoot)
	require.ErrorIs(t, err, blocks.ErrInvalidSignature, "Expected ErrInvalidSignature for invalid signature")
}
