package blocks_test

import (
	"context"
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	p2ptypes "github.com/theQRL/qrysm/beacon-chain/p2p/types"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/crypto/dilithium"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/protobuf/proto"
)

func init() {
	logrus.SetOutput(io.Discard) // Ignore "validator activated" logs
}

func TestProcessBlockHeader_ImproperBlockSlot(t *testing.T) {
	validators := make([]*zondpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			PublicKey:             make([]byte, 32),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			Slashed:               true,
		}
	}

	state, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, state.SetSlot(10))
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetLatestBlockHeader(util.HydrateBeaconHeader(&zondpb.BeaconBlockHeader{
		Slot: 10, // Must be less than block.Slot
	})))

	latestBlockSignedRoot, err := state.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)

	currentEpoch := time.CurrentEpoch(state)
	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	pID, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	block := util.NewBeaconBlockCapella()
	block.Block.ProposerIndex = pID
	block.Block.Slot = 10
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, field_params.DilithiumSignatureLength)
	block.Block.ParentRoot = latestBlockSignedRoot[:]
	block.Signature, err = signing.ComputeDomainAndSign(state, currentEpoch, block.Block, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)

	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	validators[proposerIdx].Slashed = false
	validators[proposerIdx].PublicKey = priv.PublicKey().Marshal()
	err = state.UpdateValidatorAtIndex(proposerIdx, validators[proposerIdx])
	require.NoError(t, err)

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = blocks.ProcessBlockHeader(context.Background(), state, wsb)
	assert.ErrorContains(t, "block.Slot 10 must be greater than state.LatestBlockHeader.Slot 10", err)
}

func TestProcessBlockHeader_WrongProposerSig(t *testing.T) {

	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 100)
	require.NoError(t, beaconState.SetLatestBlockHeader(util.HydrateBeaconHeader(&zondpb.BeaconBlockHeader{
		Slot: 9,
	})))
	require.NoError(t, beaconState.SetSlot(10))

	lbhdr, err := beaconState.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)

	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
	require.NoError(t, err)

	block := util.NewBeaconBlockCapella()
	block.Block.ProposerIndex = proposerIdx
	block.Block.Slot = 10
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, field_params.DilithiumSignatureLength)
	block.Block.ParentRoot = lbhdr[:]
	block.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, block.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx+1])
	require.NoError(t, err)

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = blocks.ProcessBlockHeader(context.Background(), beaconState, wsb)
	want := "signature did not verify"
	assert.ErrorContains(t, want, err)
}

func TestProcessBlockHeader_DifferentSlots(t *testing.T) {
	validators := make([]*zondpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			PublicKey:             make([]byte, 32),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			Slashed:               true,
		}
	}

	state, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(10))
	require.NoError(t, state.SetLatestBlockHeader(util.HydrateBeaconHeader(&zondpb.BeaconBlockHeader{
		Slot: 9,
	})))

	lbhsr, err := state.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	currentEpoch := time.CurrentEpoch(state)

	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sszBytes := p2ptypes.SSZBytes("hello")
	blockSig, err := signing.ComputeDomainAndSign(state, currentEpoch, &sszBytes, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)
	validators[5896].PublicKey = priv.PublicKey().Marshal()
	block := util.HydrateSignedBeaconBlockCapella(&zondpb.SignedBeaconBlockCapella{
		Block: &zondpb.BeaconBlockCapella{
			Slot:       1,
			ParentRoot: lbhsr[:],
		},
		Signature: blockSig,
	})

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = blocks.ProcessBlockHeader(context.Background(), state, wsb)
	want := "is different than block slot"
	assert.ErrorContains(t, want, err)
}

func TestProcessBlockHeader_PreviousBlockRootNotSignedRoot(t *testing.T) {
	validators := make([]*zondpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			PublicKey:             make([]byte, 48),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			Slashed:               true,
		}
	}

	state, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(10))
	bh := state.LatestBlockHeader()
	bh.Slot = 9
	require.NoError(t, state.SetLatestBlockHeader(bh))
	currentEpoch := time.CurrentEpoch(state)
	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sszBytes := p2ptypes.SSZBytes("hello")
	blockSig, err := signing.ComputeDomainAndSign(state, currentEpoch, &sszBytes, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)
	validators[5896].PublicKey = priv.PublicKey().Marshal()
	pID, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	block := util.NewBeaconBlockCapella()
	block.Block.Slot = 10
	block.Block.ProposerIndex = pID
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, 96)
	block.Block.ParentRoot = bytesutil.PadTo([]byte{'A'}, 32)
	block.Signature = blockSig

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = blocks.ProcessBlockHeader(context.Background(), state, wsb)
	want := "does not match"
	assert.ErrorContains(t, want, err)
}

func TestProcessBlockHeader_SlashedProposer(t *testing.T) {
	validators := make([]*zondpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			PublicKey:             make([]byte, field_params.DilithiumPubkeyLength),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			Slashed:               true,
		}
	}

	state, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(10))
	bh := state.LatestBlockHeader()
	bh.Slot = 9
	require.NoError(t, state.SetLatestBlockHeader(bh))
	parentRoot, err := state.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	currentEpoch := time.CurrentEpoch(state)
	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sszBytes := p2ptypes.SSZBytes("hello")
	blockSig, err := signing.ComputeDomainAndSign(state, currentEpoch, &sszBytes, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)

	validators[12683].PublicKey = priv.PublicKey().Marshal()
	pID, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	block := util.NewBeaconBlockCapella()
	block.Block.Slot = 10
	block.Block.ProposerIndex = pID
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, 96)
	block.Block.ParentRoot = parentRoot[:]
	block.Signature = blockSig

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = blocks.ProcessBlockHeader(context.Background(), state, wsb)
	want := "was previously slashed"
	assert.ErrorContains(t, want, err)
}

func TestProcessBlockHeader_OK(t *testing.T) {
	validators := make([]*zondpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			PublicKey:             make([]byte, 32),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			Slashed:               true,
		}
	}

	state, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(10))
	require.NoError(t, state.SetLatestBlockHeader(util.HydrateBeaconHeader(&zondpb.BeaconBlockHeader{
		Slot: 9,
	})))

	latestBlockSignedRoot, err := state.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)

	currentEpoch := time.CurrentEpoch(state)
	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	pID, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	block := util.NewBeaconBlockCapella()
	block.Block.ProposerIndex = pID
	block.Block.Slot = 10
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, field_params.DilithiumSignatureLength)
	block.Block.ParentRoot = latestBlockSignedRoot[:]
	block.Signature, err = signing.ComputeDomainAndSign(state, currentEpoch, block.Block, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)
	bodyRoot, err := block.Block.Body.HashTreeRoot()
	require.NoError(t, err, "Failed to hash block bytes got")

	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	validators[proposerIdx].Slashed = false
	validators[proposerIdx].PublicKey = priv.PublicKey().Marshal()
	err = state.UpdateValidatorAtIndex(proposerIdx, validators[proposerIdx])
	require.NoError(t, err)

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	newState, err := blocks.ProcessBlockHeader(context.Background(), state, wsb)
	require.NoError(t, err, "Failed to process block header got")
	var zeroHash [32]byte
	nsh := newState.LatestBlockHeader()
	expected := &zondpb.BeaconBlockHeader{
		ProposerIndex: pID,
		Slot:          block.Block.Slot,
		ParentRoot:    latestBlockSignedRoot[:],
		BodyRoot:      bodyRoot[:],
		StateRoot:     zeroHash[:],
	}
	assert.Equal(t, true, proto.Equal(nsh, expected), "Expected %v, received %v", expected, nsh)
}

func TestBlockSignatureSet_OK(t *testing.T) {
	validators := make([]*zondpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			PublicKey:             make([]byte, 32),
			WithdrawalCredentials: make([]byte, 32),
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			Slashed:               true,
		}
	}

	state, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, state.SetValidators(validators))
	require.NoError(t, state.SetSlot(10))
	require.NoError(t, state.SetLatestBlockHeader(util.HydrateBeaconHeader(&zondpb.BeaconBlockHeader{
		Slot:          9,
		ProposerIndex: 0,
	})))

	latestBlockSignedRoot, err := state.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)

	currentEpoch := time.CurrentEpoch(state)
	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	pID, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	block := util.NewBeaconBlockCapella()
	block.Block.Slot = 10
	block.Block.ProposerIndex = pID
	block.Block.Body.RandaoReveal = bytesutil.PadTo([]byte{'A', 'B', 'C'}, field_params.DilithiumSignatureLength)
	block.Block.ParentRoot = latestBlockSignedRoot[:]
	block.Signature, err = signing.ComputeDomainAndSign(state, currentEpoch, block.Block, params.BeaconConfig().DomainBeaconProposer, priv)
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), state)
	require.NoError(t, err)
	validators[proposerIdx].Slashed = false
	validators[proposerIdx].PublicKey = priv.PublicKey().Marshal()
	err = state.UpdateValidatorAtIndex(proposerIdx, validators[proposerIdx])
	require.NoError(t, err)
	set, err := blocks.BlockSignatureBatch(state, block.Block.ProposerIndex, block.Signature, block.Block.HashTreeRoot)
	require.NoError(t, err)

	verified, err := set.Verify()
	require.NoError(t, err)
	assert.Equal(t, true, verified, "Block signature set returned a set which was unable to be verified")
}
