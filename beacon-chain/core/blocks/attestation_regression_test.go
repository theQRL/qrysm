package blocks_test

import (
	"context"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/v4/config/params"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

// Regression introduced in https://github.com/theQRL/qrysm/pull/8566.
func TestVerifyAttestationNoVerifySignature_IncorrectSourceEpoch(t *testing.T) {
	// Attestation with an empty signature
	beaconState, _ := util.DeterministicGenesisStateCapella(t, 100)

	aggBits := bitfield.NewBitlist(3)
	aggBits.SetBitAt(1, true)
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	att := &zondpb.Attestation{
		Data: &zondpb.AttestationData{
			Source: &zondpb.Checkpoint{Epoch: 99, Root: mockRoot[:]},
			Target: &zondpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
		},
		AggregationBits: aggBits,
	}

	var zeroSig [4595]byte
	att.Signatures = [][]byte{zeroSig[:]}

	err := beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)
	ckp := beaconState.CurrentJustifiedCheckpoint()
	copy(ckp.Root, "hello-world")
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(ckp))

	err = blocks.VerifyAttestationNoVerifySignatures(context.TODO(), beaconState, att)
	assert.NotEqual(t, nil, err)
}
