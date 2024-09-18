package blocks_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	v "github.com/theQRL/qrysm/beacon-chain/core/validators"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestSlashableAttestationData_CanSlash(t *testing.T) {
	att1 := util.HydrateAttestationData(&zondpb.AttestationData{
		Target: &zondpb.Checkpoint{Epoch: 1, Root: make([]byte, 32)},
		Source: &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte{'A'}, 32)},
	})
	att2 := util.HydrateAttestationData(&zondpb.AttestationData{
		Target: &zondpb.Checkpoint{Epoch: 1, Root: make([]byte, 32)},
		Source: &zondpb.Checkpoint{Root: bytesutil.PadTo([]byte{'B'}, 32)},
	})
	assert.Equal(t, true, blocks.IsSlashableAttestationData(att1, att2), "Atts should have been slashable")
	att1.Target.Epoch = 4
	att1.Source.Epoch = 2
	att2.Source.Epoch = 3
	assert.Equal(t, true, blocks.IsSlashableAttestationData(att1, att2), "Atts should have been slashable")
}

func TestProcessAttesterSlashings_DataNotSlashable(t *testing.T) {
	slashings := []*zondpb.AttesterSlashing{{
		Attestation_1: util.HydrateIndexedAttestation(&zondpb.IndexedAttestation{}),
		Attestation_2: util.HydrateIndexedAttestation(&zondpb.IndexedAttestation{
			Data: &zondpb.AttestationData{
				Source: &zondpb.Checkpoint{Epoch: 1},
				Target: &zondpb.Checkpoint{Epoch: 1}},
		})}}

	var registry []*zondpb.Validator
	currentSlot := primitives.Slot(0)

	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Validators: registry,
		Slot:       currentSlot,
	})
	require.NoError(t, err)
	b := util.NewBeaconBlockCapella()
	b.Block = &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			AttesterSlashings: slashings,
		},
	}
	_, err = blocks.ProcessAttesterSlashings(context.Background(), beaconState, b.Block.Body.AttesterSlashings, v.SlashValidator)
	assert.ErrorContains(t, "attestations are not slashable", err)
}

func TestProcessAttesterSlashings_IndexedAttestationFailedToVerify(t *testing.T) {
	var registry []*zondpb.Validator
	currentSlot := primitives.Slot(0)

	beaconState, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Validators: registry,
		Slot:       currentSlot,
	})
	require.NoError(t, err)

	slashings := []*zondpb.AttesterSlashing{
		{
			Attestation_1: util.HydrateIndexedAttestation(&zondpb.IndexedAttestation{
				Data: &zondpb.AttestationData{
					Source: &zondpb.Checkpoint{Epoch: 1},
				},
				AttestingIndices: make([]uint64, params.BeaconConfig().MaxValidatorsPerCommittee+1),
			}),
			Attestation_2: util.HydrateIndexedAttestation(&zondpb.IndexedAttestation{
				AttestingIndices: make([]uint64, params.BeaconConfig().MaxValidatorsPerCommittee+1),
			}),
		},
	}

	b := util.NewBeaconBlockCapella()
	b.Block = &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			AttesterSlashings: slashings,
		},
	}

	_, err = blocks.ProcessAttesterSlashings(context.Background(), beaconState, b.Block.Body.AttesterSlashings, v.SlashValidator)
	assert.ErrorContains(t, "validator indices count exceeds MAX_VALIDATORS_PER_COMMITTEE", err)
}

func TestProcessAttesterSlashings_AppliesCorrectStatusCapella(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 100)
	for _, vv := range beaconState.Validators() {
		vv.WithdrawableEpoch = primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)
	}

	att1 := util.HydrateIndexedAttestation(&zondpb.IndexedAttestation{
		Data: &zondpb.AttestationData{
			Source: &zondpb.Checkpoint{Epoch: 1},
		},
		AttestingIndices: []uint64{0, 1},
	})
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(att1.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sig0 := privKeys[0].Sign(signingRoot[:]).Marshal()
	sig1 := privKeys[1].Sign(signingRoot[:]).Marshal()
	att1.Signatures = [][]byte{sig0, sig1}

	att2 := util.HydrateIndexedAttestation(&zondpb.IndexedAttestation{
		AttestingIndices: []uint64{0, 1},
	})
	signingRoot, err = signing.ComputeSigningRoot(att2.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sig0 = privKeys[0].Sign(signingRoot[:]).Marshal()
	sig1 = privKeys[1].Sign(signingRoot[:]).Marshal()
	att2.Signatures = [][]byte{sig0, sig1}

	slashings := []*zondpb.AttesterSlashing{
		{
			Attestation_1: att1,
			Attestation_2: att2,
		},
	}

	currentSlot := 2 * params.BeaconConfig().SlotsPerEpoch
	require.NoError(t, beaconState.SetSlot(currentSlot))

	b := util.NewBeaconBlockCapella()
	b.Block = &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			AttesterSlashings: slashings,
		},
	}

	newState, err := blocks.ProcessAttesterSlashings(context.Background(), beaconState, b.Block.Body.AttesterSlashings, v.SlashValidator)
	require.NoError(t, err)
	newRegistry := newState.Validators()

	// Given the intersection of slashable indices is [1], only validator
	// at index 1 should be slashed and exited. We confirm this below.
	if newRegistry[1].ExitEpoch != beaconState.Validators()[1].ExitEpoch {
		t.Errorf(
			`
			Expected validator at index 1's exit epoch to match
			%d, received %d instead
			`,
			beaconState.Validators()[1].ExitEpoch,
			newRegistry[1].ExitEpoch,
		)
	}

	require.Equal(t, uint64(38750000000000), newState.Balances()[1])
	require.Equal(t, uint64(40000000000000), newState.Balances()[2])
}
