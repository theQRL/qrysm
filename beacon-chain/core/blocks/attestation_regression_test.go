package blocks_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestAttestationValidation_Bounds(t *testing.T) {
	ctx := context.Background()
	beaconState, _ := util.DeterministicGenesisStateZond(t, uint64(params.BeaconConfig().SlotsPerEpoch)*2)
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().MinAttestationInclusionDelay))
	committee, err := helpers.BeaconCommitteeFromState(ctx, beaconState, 0, 0)
	require.NoError(t, err)
	require.Equal(t, 2, len(committee))
	require.Equal(t, uint64(1), helpers.SlotCommitteeCount(uint64(beaconState.NumValidators())))

	newAttestation := func() *qrysmpb.Attestation {
		att := util.HydrateAttestation(&qrysmpb.Attestation{
			AggregationBits: bitfield.Bitlist{0x07}, // Two participants in a two-member committee.
		})
		att.Signatures = append(att.Signatures, att.Signatures[0])
		return att
	}

	for _, verifier := range []struct {
		name   string
		verify func(*qrysmpb.Attestation) error
	}{
		{
			name: "VerifyAttestationNoVerifySignatures",
			verify: func(att *qrysmpb.Attestation) error {
				return blocks.VerifyAttestationNoVerifySignatures(ctx, beaconState, att)
			},
		},
		{
			name: "AttestationSignatureBatch",
			verify: func(att *qrysmpb.Attestation) error {
				_, err := blocks.AttestationSignatureBatch(ctx, beaconState, []*qrysmpb.Attestation{att})
				return err
			},
		},
	} {
		t.Run(verifier.name, func(t *testing.T) {
			// A valid control ensures each probe reaches the intended validation.
			require.NoError(t, verifier.verify(newAttestation()))
			for _, index := range []primitives.CommitteeIndex{1, 1 << 63, ^primitives.CommitteeIndex(0)} {
				t.Run(fmt.Sprintf("committee index %d", index), func(t *testing.T) {
					att := newAttestation()
					att.Data.CommitteeIndex = index
					require.ErrorContains(t, fmt.Sprintf("committee index %d >=", index), verifier.verify(att))
				})
			}
			for _, tt := range []struct {
				name       string
				bits       bitfield.Bitlist
				signatures int
				want       string
			}{
				{name: "short bitfield", bits: bitfield.Bitlist{0x03}, signatures: 1, want: "bitfield length"},
				{name: "long bitfield", bits: bitfield.Bitlist{0x0b}, signatures: 2, want: "bitfield length"},
				{name: "empty participants", bits: bitfield.Bitlist{0x04}, signatures: 0, want: "expected non-empty attesting indices"},
				{name: "missing signatures", bits: bitfield.Bitlist{0x07}, signatures: 0, want: "signatures length 0 is not equal to the attesting participants indices length 2"},
				{name: "too few signatures", bits: bitfield.Bitlist{0x07}, signatures: 1, want: "signatures length 1 is not equal to the attesting participants indices length 2"},
				{name: "too many signatures", bits: bitfield.Bitlist{0x07}, signatures: 3, want: "signatures length 3 is not equal to the attesting participants indices length 2"},
			} {
				t.Run(tt.name, func(t *testing.T) {
					att := newAttestation()
					att.AggregationBits = tt.bits
					sig := att.Signatures[0]
					att.Signatures = make([][]byte, tt.signatures)
					for i := range att.Signatures {
						att.Signatures[i] = sig
					}
					require.ErrorContains(t, tt.want, verifier.verify(att))
				})
			}
		})
	}
}

// Regression introduced in https://github.com/theQRL/qrysm/pull/8566.
func TestVerifyAttestationNoVerifySignature_IncorrectSourceEpoch(t *testing.T) {
	// Attestation with an empty signature
	beaconState, _ := util.DeterministicGenesisStateZond(t, 100)

	aggBits := bitfield.NewBitlist(3)
	aggBits.SetBitAt(1, true)
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Source: &qrysmpb.Checkpoint{Epoch: 99, Root: mockRoot[:]},
			Target: &qrysmpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
		},
		AggregationBits: aggBits,
	}

	var zeroSig [4627]byte
	att.Signatures = [][]byte{zeroSig[:]}

	err := beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)
	ckp := beaconState.CurrentJustifiedCheckpoint()
	copy(ckp.Root, "hello-world")
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(ckp))

	err = blocks.VerifyAttestationNoVerifySignatures(context.TODO(), beaconState, att)
	assert.NotEqual(t, nil, err)
}
