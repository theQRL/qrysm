package util

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/bls"
	"github.com/theQRL/qrysm/v4/crypto/rand"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

// RandaoReveal returns a signature of the requested epoch using the beacon proposer private key.
func RandaoReveal(beaconState state.ReadOnlyBeaconState, epoch primitives.Epoch, privKeys []bls.SecretKey) ([]byte, error) {
	// We fetch the proposer's index as that is whom the RANDAO will be verified against.
	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
	if err != nil {
		return []byte{}, errors.Wrap(err, "could not get beacon proposer index")
	}
	buf := make([]byte, 32)
	binary.LittleEndian.PutUint64(buf, uint64(epoch))

	// We make the previous validator's index sign the message instead of the proposer.
	sszEpoch := primitives.SSZUint64(epoch)
	return signing.ComputeDomainAndSign(beaconState, epoch, &sszEpoch, params.BeaconConfig().DomainRandao, privKeys[proposerIdx])
}

// BlockSignature calculates the post-state root of the block and returns the signature.
func BlockSignature(
	bState state.BeaconState,
	block interface{},
	privKeys []bls.SecretKey,
) (bls.Signature, error) {
	var wsb interfaces.ReadOnlySignedBeaconBlock
	var err error
	// copy the state since we need to process slots
	bState = bState.Copy()
	switch b := block.(type) {
	case *zondpb.BeaconBlock:
		wsb, err = blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlock{Block: b})
	case *zondpb.BeaconBlockAltair:
		wsb, err = blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockAltair{Block: b})
	case *zondpb.BeaconBlockBellatrix:
		wsb, err = blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockBellatrix{Block: b})
	case *zondpb.BeaconBlockCapella:
		wsb, err = blocks.NewSignedBeaconBlock(&zondpb.SignedBeaconBlockCapella{Block: b})
	default:
		return nil, errors.New("unsupported block type")
	}
	if err != nil {
		return nil, errors.Wrap(err, "could not wrap block")
	}
	s, err := transition.CalculateStateRoot(context.Background(), bState, wsb)
	if err != nil {
		return nil, errors.Wrap(err, "could not calculate state root")
	}

	switch b := block.(type) {
	case *zondpb.BeaconBlock:
		b.StateRoot = s[:]
	case *zondpb.BeaconBlockAltair:
		b.StateRoot = s[:]
	case *zondpb.BeaconBlockBellatrix:
		b.StateRoot = s[:]
	case *zondpb.BeaconBlockCapella:
		b.StateRoot = s[:]
	}

	// Temporarily increasing the beacon state slot here since BeaconProposerIndex is a
	// function deterministic on beacon state slot.
	var blockSlot primitives.Slot
	switch b := block.(type) {
	case *zondpb.BeaconBlock:
		blockSlot = b.Slot
	case *zondpb.BeaconBlockAltair:
		blockSlot = b.Slot
	case *zondpb.BeaconBlockBellatrix:
		blockSlot = b.Slot
	case *zondpb.BeaconBlockCapella:
		blockSlot = b.Slot
	}

	// process slots to get the right fork
	bState, err = transition.ProcessSlots(context.Background(), bState, blockSlot)
	if err != nil {
		return nil, err
	}

	domain, err := signing.Domain(bState.Fork(), time.CurrentEpoch(bState), params.BeaconConfig().DomainBeaconProposer, bState.GenesisValidatorsRoot())
	if err != nil {
		return nil, err
	}

	var blockRoot [32]byte
	switch b := block.(type) {
	case *zondpb.BeaconBlock:
		blockRoot, err = signing.ComputeSigningRoot(b, domain)
	case *zondpb.BeaconBlockAltair:
		blockRoot, err = signing.ComputeSigningRoot(b, domain)
	case *zondpb.BeaconBlockBellatrix:
		blockRoot, err = signing.ComputeSigningRoot(b, domain)
	case *zondpb.BeaconBlockCapella:
		blockRoot, err = signing.ComputeSigningRoot(b, domain)
	}
	if err != nil {
		return nil, err
	}

	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), bState)
	if err != nil {
		return nil, err
	}
	return privKeys[proposerIdx].Sign(blockRoot[:]), nil
}

// Random32Bytes generates a random 32 byte slice.
func Random32Bytes(t *testing.T) []byte {
	b := make([]byte, 32)
	_, err := rand.NewDeterministicGenerator().Read(b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
