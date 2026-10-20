package blocks_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestVerifyAttestationNoVerifySignatures_IncorrectSlotTargetEpoch(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, 1)

	att := util.HydrateAttestation(&qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Slot:   params.BeaconConfig().SlotsPerEpoch,
			Target: &qrysmpb.Checkpoint{Root: make([]byte, 32)},
		},
	})
	wanted := "slot 128 does not match target epoch 0"
	err := blocks.VerifyAttestationNoVerifySignatures(context.TODO(), beaconState, att)
	assert.ErrorContains(t, wanted, err)
}

func TestProcessAttestationsNoVerify_OlderThanSlotsPerEpoch(t *testing.T) {
	aggBits := bitfield.NewBitlist(3)
	aggBits.SetBitAt(1, true)
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Source: &qrysmpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
			Target: &qrysmpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
		},
		AggregationBits: aggBits,
	}
	ctx := context.Background()

	t.Run("attestation older than slots per epoch", func(t *testing.T) {
		beaconState, _ := util.DeterministicGenesisStateZond(t, 100)

		err := beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().SlotsPerEpoch + 1)
		require.NoError(t, err)
		ckp := beaconState.CurrentJustifiedCheckpoint()
		copy(ckp.Root, "hello-world")
		require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(ckp))

		require.ErrorContains(t, "state slot 129 > attestation slot 0 + SLOTS_PER_EPOCH 128", blocks.VerifyAttestationNoVerifySignatures(ctx, beaconState, att))
	})
}

func TestVerifyAttestationNoVerifySignatures_OK(t *testing.T) {
	// Attestation with an empty signature

	beaconState, _ := util.DeterministicGenesisStateZond(t, 256)

	aggBits := bitfield.NewBitlist(2)
	aggBits.SetBitAt(1, true)
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Source: &qrysmpb.Checkpoint{Epoch: 0, Root: mockRoot[:]},
			Target: &qrysmpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
		},
		AggregationBits: aggBits,
	}

	var zeroSig [field_params.MLDSA87SignatureLength]byte
	att.Signatures = [][]byte{zeroSig[:]}

	err := beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)
	ckp := beaconState.CurrentJustifiedCheckpoint()
	copy(ckp.Root, "hello-world")
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(ckp))

	err = blocks.VerifyAttestationNoVerifySignatures(context.TODO(), beaconState, att)
	assert.NoError(t, err)
}

func TestVerifyAttestationNoVerifySignatures_BadAttIdx(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, 100)
	aggBits := bitfield.NewBitlist(3)
	aggBits.SetBitAt(1, true)
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			CommitteeIndex: 100,
			Source:         &qrysmpb.Checkpoint{Epoch: 0, Root: mockRoot[:]},
			Target:         &qrysmpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
		},
		AggregationBits: aggBits,
	}
	var zeroSig [field_params.MLDSA87SignatureLength]byte
	att.Signatures = [][]byte{zeroSig[:]}
	require.NoError(t, beaconState.SetSlot(beaconState.Slot()+params.BeaconConfig().MinAttestationInclusionDelay))
	ckp := beaconState.CurrentJustifiedCheckpoint()
	copy(ckp.Root, "hello-world")
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(ckp))
	err := blocks.VerifyAttestationNoVerifySignatures(context.TODO(), beaconState, att)
	require.ErrorContains(t, "committee index 100 >= committee count 1", err)
}

func TestConvertToIndexed_OK(t *testing.T) {
	helpers.ClearCache()
	validators := make([]*qrysmpb.Validator, 2*params.BeaconConfig().SlotsPerEpoch)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			ExitEpoch: params.BeaconConfig().FarFutureEpoch,
		}
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot:        5,
		Validators:  validators,
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)
	tests := []struct {
		aggregationBitfield    bitfield.Bitlist
		wantedAttestingIndices []uint64
	}{
		{
			aggregationBitfield:    bitfield.Bitlist{0x07},
			wantedAttestingIndices: []uint64{2, 167},
		},
		{
			aggregationBitfield:    bitfield.Bitlist{0x05},
			wantedAttestingIndices: []uint64{2},
		},
		{
			aggregationBitfield:    bitfield.Bitlist{0x04},
			wantedAttestingIndices: []uint64{},
		},
	}

	var sig [field_params.MLDSA87SignatureLength]byte
	copy(sig[:], "signed")
	att := util.HydrateAttestation(&qrysmpb.Attestation{
		Signatures: [][]byte{},
	})
	for _, tt := range tests {
		att.AggregationBits = tt.aggregationBitfield
		signatures := make([][]byte, len(tt.aggregationBitfield.BitIndices()))
		for i := 0; i < len(tt.aggregationBitfield.BitIndices()); i++ {
			signatures[i] = make([]byte, 4627)
		}
		att.Signatures = signatures

		wanted := &qrysmpb.IndexedAttestation{
			AttestingIndices: tt.wantedAttestingIndices,
			Data:             att.Data,
			Signatures:       att.Signatures,
		}

		committee, err := helpers.BeaconCommitteeFromState(context.Background(), state, att.Data.Slot, att.Data.CommitteeIndex)
		require.NoError(t, err)
		ia, err := attestation.ConvertToIndexed(context.Background(), att, committee)
		require.NoError(t, err)
		assert.DeepEqual(t, wanted, ia, "Convert attestation to indexed attestation didn't result as wanted")
	}
}

func TestVerifyIndexedAttestation_OK(t *testing.T) {
	numOfValidators := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))
	validators := make([]*qrysmpb.Validator, numOfValidators)
	_, keys, err := util.DeterministicDepositsAndKeys(numOfValidators)
	require.NoError(t, err)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			ExitEpoch:           params.BeaconConfig().FarFutureEpoch,
			PublicKey:           keys[i].PublicKey().Marshal(),
			WithdrawalRecipient: make([]byte, 64),
		}
	}

	state, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{
		Slot:       5,
		Validators: validators,
		Fork: &qrysmpb.Fork{
			Epoch:           0,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
		},
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)
	tests := []struct {
		attestation *qrysmpb.IndexedAttestation
	}{
		{attestation: &qrysmpb.IndexedAttestation{
			Data: util.HydrateAttestationData(&qrysmpb.AttestationData{
				Target: &qrysmpb.Checkpoint{
					Epoch: 2,
				},
				Source: &qrysmpb.Checkpoint{},
			}),
			AttestingIndices: []uint64{1},
		}},
		{attestation: &qrysmpb.IndexedAttestation{
			Data: util.HydrateAttestationData(&qrysmpb.AttestationData{
				Target: &qrysmpb.Checkpoint{
					Epoch: 1,
				},
			}),
			AttestingIndices: []uint64{47, 99, 101},
		}},
		{attestation: &qrysmpb.IndexedAttestation{
			Data: util.HydrateAttestationData(&qrysmpb.AttestationData{
				Target: &qrysmpb.Checkpoint{
					Epoch: 4,
				},
			}),
			AttestingIndices: []uint64{21, 72},
		}},
		{attestation: &qrysmpb.IndexedAttestation{
			Data: util.HydrateAttestationData(&qrysmpb.AttestationData{
				Target: &qrysmpb.Checkpoint{
					Epoch: 7,
				},
			}),
			AttestingIndices: []uint64{100, 121, 122},
		}},
	}

	for _, tt := range tests {
		var sigs [][]byte
		for _, idx := range tt.attestation.AttestingIndices {
			sb, err := signing.ComputeDomainAndSign(state, tt.attestation.Data.Target.Epoch, tt.attestation.Data, params.BeaconConfig().DomainBeaconAttester, keys[idx])
			require.NoError(t, err)
			sigs = append(sigs, sb)
		}

		tt.attestation.Signatures = sigs

		err = blocks.VerifyIndexedAttestation(context.Background(), state, tt.attestation)
		assert.NoError(t, err, "Failed to verify indexed attestation")
	}
}

func TestValidateIndexedAttestation_AboveMaxLength(t *testing.T) {
	indexedAtt1 := &qrysmpb.IndexedAttestation{
		AttestingIndices: make([]uint64, params.BeaconConfig().MaxValidatorsPerCommittee+5),
	}

	for i := uint64(0); i < params.BeaconConfig().MaxValidatorsPerCommittee+5; i++ {
		indexedAtt1.AttestingIndices[i] = i
		indexedAtt1.Data = &qrysmpb.AttestationData{
			Target: &qrysmpb.Checkpoint{
				Epoch: primitives.Epoch(i),
			},
			Source: &qrysmpb.Checkpoint{},
		}
	}

	want := "validator indices count exceeds MAX_VALIDATORS_PER_COMMITTEE"
	st, err := state_native.InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	err = blocks.VerifyIndexedAttestation(context.Background(), st, indexedAtt1)
	assert.ErrorContains(t, want, err)
}

func TestVerifyIndexedAttestation_OutOfRangeIndexRejected(t *testing.T) {
	beaconState, keys := util.DeterministicGenesisStateZond(t, 8)
	numValidators := uint64(beaconState.NumValidators())

	att := util.HydrateIndexedAttestation(&qrysmpb.IndexedAttestation{
		Data: util.HydrateAttestationData(&qrysmpb.AttestationData{
			Target: &qrysmpb.Checkpoint{Epoch: 0},
			Source: &qrysmpb.Checkpoint{},
		}),
		// Sorted and unique, but the last index is one past the registry.
		AttestingIndices: []uint64{0, numValidators},
	})
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, att.Data, params.BeaconConfig().DomainBeaconAttester, keys[0])
	require.NoError(t, err)
	// The out-of-range slot carries a well-formed but meaningless signature; an
	// attacker could forge one for the all-zero key PubkeyAtIndex used to return.
	att.Signatures = [][]byte{sig, make([]byte, field_params.MLDSA87SignatureLength)}

	for _, index := range []uint64{numValidators, 1 << 63, ^uint64(0)} {
		t.Run(fmt.Sprintf("index %d", index), func(t *testing.T) {
			att.AttestingIndices[1] = index
			err := blocks.VerifyIndexedAttestation(context.Background(), beaconState, att)
			// Require the bounds error, rather than a later key or signature error.
			require.ErrorContains(t, fmt.Sprintf("attesting index %d out of range for %d validators", index, numValidators), err)
		})
	}
}

func TestVerifyIndexedAttestation_InvalidIndices(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, 8)
	for _, tt := range []struct {
		name    string
		indices []uint64
		want    string
	}{
		{name: "empty", want: "expected non-empty attesting indices"},
		{name: "duplicate", indices: []uint64{0, 0}, want: "attesting indices is not uniquely sorted"},
		{name: "unsorted", indices: []uint64{1, 0}, want: "attesting indices is not uniquely sorted"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			att := &qrysmpb.IndexedAttestation{
				Data:             util.HydrateAttestationData(&qrysmpb.AttestationData{}),
				AttestingIndices: tt.indices,
				Signatures:       make([][]byte, len(tt.indices)),
			}
			require.ErrorContains(t, tt.want, blocks.VerifyIndexedAttestation(context.Background(), beaconState, att))
		})
	}
}

func TestValidateIndexedAttestation_BadAttestationsSignatureSet(t *testing.T) {
	beaconState, keys := util.DeterministicGenesisStateZond(t, 256)

	sig, err := keys[0].Sign([]byte{'t', 'e', 's', 't'})
	require.NoError(t, err)
	list := bitfield.Bitlist{0b111}
	var atts []*qrysmpb.Attestation
	for range 1000 {
		atts = append(atts, &qrysmpb.Attestation{
			Data: &qrysmpb.AttestationData{
				CommitteeIndex: 0,
				Slot:           1,
			},
			Signatures:      [][]byte{sig.Marshal(), sig.Marshal()},
			AggregationBits: list,
		})
	}

	want := "nil or missing indexed attestation data"
	_, err = blocks.AttestationSignatureBatch(context.Background(), beaconState, atts)
	assert.ErrorContains(t, want, err)

	atts = []*qrysmpb.Attestation{}
	list = bitfield.Bitlist{0b100}
	for range 1000 {
		atts = append(atts, &qrysmpb.Attestation{
			Data: &qrysmpb.AttestationData{
				CommitteeIndex: 0,
				Slot:           1,
				Target: &qrysmpb.Checkpoint{
					Root: []byte{},
				},
				Source: &qrysmpb.Checkpoint{},
			},
			Signatures:      [][]byte{},
			AggregationBits: list,
		})
	}

	want = "expected non-empty attesting indices"
	_, err = blocks.AttestationSignatureBatch(context.Background(), beaconState, atts)
	assert.ErrorContains(t, want, err)
}

func TestVerifyAttestations_HandlesPlannedFork(t *testing.T) {
	// In this test, att1 is from the prior fork and att2 is from the new fork.
	numOfValidators := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))
	validators := make([]*qrysmpb.Validator, numOfValidators)
	_, keys, err := util.DeterministicDepositsAndKeys(numOfValidators)
	require.NoError(t, err)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			ExitEpoch:           params.BeaconConfig().FarFutureEpoch,
			PublicKey:           keys[i].PublicKey().Marshal(),
			WithdrawalRecipient: make([]byte, 64),
		}
	}

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(35))
	require.NoError(t, st.SetValidators(validators))
	require.NoError(t, st.SetFork(&qrysmpb.Fork{
		Epoch:           1,
		CurrentVersion:  []byte{0, 1, 2, 3},
		PreviousVersion: params.BeaconConfig().GenesisForkVersion,
	}))

	comm1, err := helpers.BeaconCommitteeFromState(context.Background(), st, 1 /*slot*/, 0 /*committeeIndex*/)
	require.NoError(t, err)
	att1 := util.HydrateAttestation(&qrysmpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm1))),
		Data: &qrysmpb.AttestationData{
			Slot: 1,
		},
	})
	prevDomain, err := signing.Domain(st.Fork(), st.Fork().Epoch-1, params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(att1.Data, prevDomain)
	require.NoError(t, err)
	var sigs [][]byte
	for i, u := range comm1 {
		att1.AggregationBits.SetBitAt(uint64(i), true)
		lsig1, err := keys[u].Sign(root[:])
		require.NoError(t, err)
		sigs = append(sigs, lsig1.Marshal())
	}
	att1.Signatures = sigs

	comm2, err := helpers.BeaconCommitteeFromState(context.Background(), st, 1*params.BeaconConfig().SlotsPerEpoch+1 /*slot*/, 0 /*committeeIndex*/)
	require.NoError(t, err)
	att2 := util.HydrateAttestation(&qrysmpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm2))),
		Data: &qrysmpb.AttestationData{
			Slot:           1*params.BeaconConfig().SlotsPerEpoch + 1,
			CommitteeIndex: 0,
		},
	})
	currDomain, err := signing.Domain(st.Fork(), st.Fork().Epoch, params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err = signing.ComputeSigningRoot(att2.Data, currDomain)
	require.NoError(t, err)
	sigs = nil
	for i, u := range comm2 {
		att2.AggregationBits.SetBitAt(uint64(i), true)
		lsig2, err := keys[u].Sign(root[:])
		require.NoError(t, err)
		sigs = append(sigs, lsig2.Marshal())
	}
	att2.Signatures = sigs
}

func TestRetrieveAttestationSignatureSet_VerifiesMultipleAttestations(t *testing.T) {
	ctx := context.Background()
	numOfValidators := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))
	validators := make([]*qrysmpb.Validator, numOfValidators)
	_, keys, err := util.DeterministicDepositsAndKeys(numOfValidators)
	require.NoError(t, err)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			ExitEpoch:           params.BeaconConfig().FarFutureEpoch,
			PublicKey:           keys[i].PublicKey().Marshal(),
			WithdrawalRecipient: make([]byte, 64),
		}
	}

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(5))
	require.NoError(t, st.SetValidators(validators))

	comm1, err := helpers.BeaconCommitteeFromState(context.Background(), st, 1 /*slot*/, 0 /*committeeIndex*/)
	require.NoError(t, err)
	att1 := util.HydrateAttestation(&qrysmpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm1))),
		Data: &qrysmpb.AttestationData{
			Slot: 1,
		},
	})
	domain, err := signing.Domain(st.Fork(), st.Fork().Epoch, params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(att1.Data, domain)
	require.NoError(t, err)
	var sigs [][]byte
	for i, u := range comm1 {
		att1.AggregationBits.SetBitAt(uint64(i), true)
		lsig3, err := keys[u].Sign(root[:])
		require.NoError(t, err)
		sigs = append(sigs, lsig3.Marshal())
	}
	att1.Signatures = sigs

	comm2, err := helpers.BeaconCommitteeFromState(context.Background(), st, 2 /*slot*/, 0 /*committeeIndex*/)
	require.NoError(t, err)
	att2 := util.HydrateAttestation(&qrysmpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm2))),
		Data: &qrysmpb.AttestationData{
			Slot:           2,
			CommitteeIndex: 0,
		},
	})
	root, err = signing.ComputeSigningRoot(att2.Data, domain)
	require.NoError(t, err)
	sigs = nil
	for i, u := range comm2 {
		att2.AggregationBits.SetBitAt(uint64(i), true)
		lsig4, err := keys[u].Sign(root[:])
		require.NoError(t, err)
		sigs = append(sigs, lsig4.Marshal())
	}
	att2.Signatures = sigs

	set, err := blocks.AttestationSignatureBatch(ctx, st, []*qrysmpb.Attestation{att1, att2})
	require.NoError(t, err)
	verified, err := set.Verify()
	require.NoError(t, err)
	assert.Equal(t, true, verified, "Multiple signatures were unable to be verified.")
}

func TestRetrieveAttestationSignatureSet_AcrossFork(t *testing.T) {
	ctx := context.Background()
	numOfValidators := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))
	validators := make([]*qrysmpb.Validator, numOfValidators)
	_, keys, err := util.DeterministicDepositsAndKeys(numOfValidators)
	require.NoError(t, err)
	for i := range validators {
		validators[i] = &qrysmpb.Validator{
			ExitEpoch:           params.BeaconConfig().FarFutureEpoch,
			PublicKey:           keys[i].PublicKey().Marshal(),
			WithdrawalRecipient: make([]byte, 64),
		}
	}

	st, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(5))
	require.NoError(t, st.SetValidators(validators))
	require.NoError(t, st.SetFork(&qrysmpb.Fork{Epoch: 1, CurrentVersion: []byte{0, 1, 2, 3}, PreviousVersion: []byte{0, 1, 1, 1}}))

	comm1, err := helpers.BeaconCommitteeFromState(ctx, st, 1 /*slot*/, 0 /*committeeIndex*/)
	require.NoError(t, err)
	att1 := util.HydrateAttestation(&qrysmpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm1))),
		Data: &qrysmpb.AttestationData{
			Slot: 1,
		},
	})
	domain, err := signing.Domain(st.Fork(), st.Fork().Epoch, params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err := signing.ComputeSigningRoot(att1.Data, domain)
	require.NoError(t, err)
	var sigs [][]byte
	for i, u := range comm1 {
		att1.AggregationBits.SetBitAt(uint64(i), true)
		lsig5, err := keys[u].Sign(root[:])
		require.NoError(t, err)
		sigs = append(sigs, lsig5.Marshal())
	}
	att1.Signatures = sigs

	comm2, err := helpers.BeaconCommitteeFromState(ctx, st, 2 /*slot*/, 0 /*committeeIndex*/)
	require.NoError(t, err)
	att2 := util.HydrateAttestation(&qrysmpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm2))),
		Data: &qrysmpb.AttestationData{
			Slot:           2,
			CommitteeIndex: 0,
		},
	})
	root, err = signing.ComputeSigningRoot(att2.Data, domain)
	require.NoError(t, err)
	sigs = nil
	for i, u := range comm2 {
		att2.AggregationBits.SetBitAt(uint64(i), true)
		lsig6, err := keys[u].Sign(root[:])
		require.NoError(t, err)
		sigs = append(sigs, lsig6.Marshal())
	}
	att2.Signatures = sigs

	_, err = blocks.AttestationSignatureBatch(ctx, st, []*qrysmpb.Attestation{att1, att2})
	require.NoError(t, err)
}
