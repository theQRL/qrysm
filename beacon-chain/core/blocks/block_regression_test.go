package blocks_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/v4/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	v "github.com/theQRL/qrysm/v4/beacon-chain/core/validators"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestProcessAttesterSlashings_RegressionSlashableIndices(t *testing.T) {

	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 5500)
	for _, vv := range beaconState.Validators() {
		vv.WithdrawableEpoch = primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)
	}
	// This set of indices is very similar to the one from our sapphire testnet
	// when close to 100 validators were incorrectly slashed. The set is from 0 -5500,
	// instead of 55000 as it would take too long to generate a state.
	setA := []uint64{21, 92, 236, 244, 281, 321, 510, 524,
		538, 682, 828, 858, 913, 920, 922, 959, 1176, 1207,
		1222, 1229, 1354, 1394, 1436, 1454, 1510, 1550,
		1552, 1576, 1645, 1704, 1842, 1967, 2076, 2111, 2134, 2307,
		2343, 2354, 2417, 2524, 2532, 2555, 2740, 2749, 2759, 2762,
		2800, 2809, 2824, 2987, 3110, 3125, 3559, 3583, 3599, 3608,
		3657, 3685, 3723, 3756, 3759, 3761, 3820, 3826, 3979, 4030,
		4141, 4170, 4205, 4247, 4257, 4479, 4492, 4569, 5091,
	}
	// Only 2800 is the slashable index.
	setB := []uint64{1361, 1438, 2383, 2800}
	expectedSlashedVal := 2800

	root1 := [32]byte{'d', 'o', 'u', 'b', 'l', 'e', '1'}
	att1 := &zondpb.IndexedAttestation{
		Data:             util.HydrateAttestationData(&zondpb.AttestationData{Target: &zondpb.Checkpoint{Epoch: 0, Root: root1[:]}}),
		AttestingIndices: setA,
		Signatures:       [][]byte{make([]byte, 4595)},
	}
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(att1.Data, domain)
	require.NoError(t, err, "Could not get signing root of beacon block header")
	var sigs [][]byte
	for _, index := range setA {
		sig := privKeys[index].Sign(signingRoot[:]).Marshal()
		sigs = append(sigs, sig)
	}
	att1.Signatures = sigs

	root2 := [32]byte{'d', 'o', 'u', 'b', 'l', 'e', '2'}
	att2 := &zondpb.IndexedAttestation{
		Data: util.HydrateAttestationData(&zondpb.AttestationData{
			Target: &zondpb.Checkpoint{Root: root2[:]},
		}),
		AttestingIndices: setB,
		Signatures:       [][]byte{make([]byte, 4595)},
	}
	signingRoot, err = signing.ComputeSigningRoot(att2.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sigs = [][]byte{}
	for _, index := range setB {
		sig := privKeys[index].Sign(signingRoot[:]).Marshal()
		sigs = append(sigs, sig)
	}
	att2.Signatures = sigs

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
	if !newRegistry[expectedSlashedVal].Slashed {
		t.Errorf("Validator with index %d was not slashed despite performing a double vote", expectedSlashedVal)
	}

	for idx, val := range newRegistry {
		if val.Slashed && idx != expectedSlashedVal {
			t.Errorf("validator with index: %d was unintentionally slashed", idx)
		}
	}
}
