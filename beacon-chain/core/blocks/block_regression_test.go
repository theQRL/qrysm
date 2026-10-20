package blocks_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	v "github.com/theQRL/qrysm/beacon-chain/core/validators"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProcessAttesterSlashings_RegressionSlashableIndices(t *testing.T) {

	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 5500)
	for _, vv := range beaconState.Validators() {
		vv.WithdrawableEpoch = primitives.Epoch(params.BeaconConfig().SlotsPerEpoch)
	}
	// This set of indices is very similar to the one from our sapphire testnet
	// when close to 100 validators were incorrectly slashed. The set is from 0 -5500,
	// instead of 55000 as it would take too long to generate a state. It is capped
	// at MAX_VALIDATORS_PER_COMMITTEE (32) sorted entries, still spanning the whole
	// range and still sharing exactly one index (2800) with setB.
	setA := []uint64{21, 236, 321, 524, 682, 858, 920, 959,
		1207, 1354, 1436, 1510, 1576, 1704, 1967, 2111,
		2307, 2417, 2532, 2740, 2762, 2800, 2824, 3110,
		3559, 3608, 3723, 3761, 3979, 4170, 4479, 5091,
	}
	// Only 2800 is the slashable index.
	setB := []uint64{1361, 1438, 2383, 2800}
	expectedSlashedVal := 2800

	root1 := [32]byte{'d', 'o', 'u', 'b', 'l', 'e', '1'}
	att1 := &qrysmpb.IndexedAttestation{
		Data:             util.HydrateAttestationData(&qrysmpb.AttestationData{Target: &qrysmpb.Checkpoint{Epoch: 0, Root: root1[:]}}),
		AttestingIndices: setA,
		Signatures:       [][]byte{make([]byte, 4627)},
	}
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	signingRoot, err := signing.ComputeSigningRoot(att1.Data, domain)
	require.NoError(t, err, "Could not get signing root of beacon block header")
	var sigs [][]byte
	for _, index := range setA {
		lsig1, err := privKeys[index].Sign(signingRoot[:])
		require.NoError(t, err)
		sig := lsig1.Marshal()
		sigs = append(sigs, sig)
	}
	att1.Signatures = sigs

	root2 := [32]byte{'d', 'o', 'u', 'b', 'l', 'e', '2'}
	att2 := &qrysmpb.IndexedAttestation{
		Data: util.HydrateAttestationData(&qrysmpb.AttestationData{
			Target: &qrysmpb.Checkpoint{Root: root2[:]},
		}),
		AttestingIndices: setB,
		Signatures:       [][]byte{make([]byte, 4627)},
	}
	signingRoot, err = signing.ComputeSigningRoot(att2.Data, domain)
	assert.NoError(t, err, "Could not get signing root of beacon block header")
	sigs = [][]byte{}
	for _, index := range setB {
		lsig2, err := privKeys[index].Sign(signingRoot[:])
		require.NoError(t, err)
		sig := lsig2.Marshal()
		sigs = append(sigs, sig)
	}
	att2.Signatures = sigs

	slashings := []*qrysmpb.AttesterSlashing{
		{
			Attestation_1: att1,
			Attestation_2: att2,
		},
	}

	currentSlot := 2 * params.BeaconConfig().SlotsPerEpoch
	require.NoError(t, beaconState.SetSlot(currentSlot))

	b := util.NewBeaconBlockZond()
	b.Block = &qrysmpb.BeaconBlockZond{
		Body: &qrysmpb.BeaconBlockBodyZond{
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
