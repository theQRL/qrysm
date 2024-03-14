package blocks_test

import (
	"context"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation/aggregation"
	attaggregation "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestProcessAggregatedAttestation_OverlappingBits(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 256)
	data := util.HydrateAttestationData(&zondpb.AttestationData{
		Source: &zondpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
		Target: &zondpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
	})
	aggBits1 := bitfield.NewBitlist(2)
	aggBits1.SetBitAt(0, true)
	aggBits1.SetBitAt(1, true)
	att1 := &zondpb.Attestation{
		Data:            data,
		AggregationBits: aggBits1,
	}

	cfc := beaconState.CurrentJustifiedCheckpoint()
	cfc.Root = bytesutil.PadTo([]byte("hello-world"), 32)
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(cfc))

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, att1.Data.Slot, att1.Data.CommitteeIndex)
	require.NoError(t, err)
	attestingIndices1, err := attestation.AttestingIndices(att1.AggregationBits, committee)
	require.NoError(t, err)
	sigs := make([][]byte, len(attestingIndices1))
	for i, indice := range attestingIndices1 {
		sb, err := signing.ComputeDomainAndSign(beaconState, 0, att1.Data, params.BeaconConfig().DomainBeaconAttester, privKeys[indice])
		require.NoError(t, err)
		sigs[i] = sb
	}
	att1.Signatures = sigs

	aggBits2 := bitfield.NewBitlist(2)
	aggBits2.SetBitAt(1, true)
	aggBits2.SetBitAt(2, true)
	att2 := &zondpb.Attestation{
		Data:            data,
		AggregationBits: aggBits2,
	}

	committee, err = helpers.BeaconCommitteeFromState(context.Background(), beaconState, att2.Data.Slot, att2.Data.CommitteeIndex)
	require.NoError(t, err)
	attestingIndices2, err := attestation.AttestingIndices(att2.AggregationBits, committee)
	require.NoError(t, err)
	sigs = make([][]byte, len(attestingIndices2))
	for i, indice := range attestingIndices2 {
		sb, err := signing.ComputeDomainAndSign(beaconState, 0, att2.Data, params.BeaconConfig().DomainBeaconAttester, privKeys[indice])
		require.NoError(t, err)
		sigs[i] = sb
	}
	att2.Signatures = sigs

	_, err = attaggregation.AggregatePair(att1, att2)
	assert.ErrorContains(t, aggregation.ErrBitsOverlap.Error(), err)
}

func TestVerifyAttestationNoVerifySignatures_IncorrectSlotTargetEpoch(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateCapella(t, 1)

	att := util.HydrateAttestation(&zondpb.Attestation{
		Data: &zondpb.AttestationData{
			Slot:   params.BeaconConfig().SlotsPerEpoch,
			Target: &zondpb.Checkpoint{Root: make([]byte, 32)},
		},
	})
	wanted := "slot 128 does not match target epoch 0"
	err := blocks.VerifyAttestationNoVerifySignatures(context.TODO(), beaconState, att)
	assert.ErrorContains(t, wanted, err)
}

func TestProcessAttestationsNoVerify_OlderThanSlotsPerEpoch(t *testing.T) {
	aggBits := bitfield.NewBitlist(3)
	aggBits.SetBitAt(1, true)
	att := &zondpb.Attestation{
		Data: &zondpb.AttestationData{
			Source: &zondpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
			Target: &zondpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
		},
		AggregationBits: aggBits,
	}
	ctx := context.Background()

	t.Run("attestation older than slots per epoch", func(t *testing.T) {
		beaconState, _ := util.DeterministicGenesisStateCapella(t, 100)

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

	beaconState, _ := util.DeterministicGenesisStateCapella(t, 256)

	aggBits := bitfield.NewBitlist(2)
	aggBits.SetBitAt(1, true)
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	att := &zondpb.Attestation{
		Data: &zondpb.AttestationData{
			Source: &zondpb.Checkpoint{Epoch: 0, Root: mockRoot[:]},
			Target: &zondpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
		},
		AggregationBits: aggBits,
	}

	var zeroSig [field_params.DilithiumSignatureLength]byte
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
	beaconState, _ := util.DeterministicGenesisStateCapella(t, 100)
	aggBits := bitfield.NewBitlist(3)
	aggBits.SetBitAt(1, true)
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	att := &zondpb.Attestation{
		Data: &zondpb.AttestationData{
			CommitteeIndex: 100,
			Source:         &zondpb.Checkpoint{Epoch: 0, Root: mockRoot[:]},
			Target:         &zondpb.Checkpoint{Epoch: 0, Root: make([]byte, 32)},
		},
		AggregationBits: aggBits,
	}
	var zeroSig [field_params.DilithiumSignatureLength]byte
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
	validators := make([]*zondpb.Validator, 2*params.BeaconConfig().SlotsPerEpoch)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			ExitEpoch: params.BeaconConfig().FarFutureEpoch,
		}
	}

	state, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
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

	var sig [field_params.DilithiumSignatureLength]byte
	copy(sig[:], "signed")
	att := util.HydrateAttestation(&zondpb.Attestation{
		Signatures: [][]byte{},
	})
	for _, tt := range tests {
		att.AggregationBits = tt.aggregationBitfield
		signatures := make([][]byte, len(tt.aggregationBitfield.BitIndices()))
		for i := 0; i < len(tt.aggregationBitfield.BitIndices()); i++ {
			signatures[i] = make([]byte, 4595)
		}
		att.Signatures = signatures

		wanted := &zondpb.IndexedAttestation{
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
	validators := make([]*zondpb.Validator, numOfValidators)
	_, keys, err := util.DeterministicDepositsAndKeys(numOfValidators)
	require.NoError(t, err)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			PublicKey:             keys[i].PublicKey().Marshal(),
			WithdrawalCredentials: make([]byte, 32),
		}
	}

	state, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Slot:       5,
		Validators: validators,
		Fork: &zondpb.Fork{
			Epoch:           0,
			CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
			PreviousVersion: params.BeaconConfig().GenesisForkVersion,
		},
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)
	tests := []struct {
		attestation *zondpb.IndexedAttestation
	}{
		{attestation: &zondpb.IndexedAttestation{
			Data: util.HydrateAttestationData(&zondpb.AttestationData{
				Target: &zondpb.Checkpoint{
					Epoch: 2,
				},
				Source: &zondpb.Checkpoint{},
			}),
			AttestingIndices: []uint64{1},
		}},
		{attestation: &zondpb.IndexedAttestation{
			Data: util.HydrateAttestationData(&zondpb.AttestationData{
				Target: &zondpb.Checkpoint{
					Epoch: 1,
				},
			}),
			AttestingIndices: []uint64{47, 99, 101},
		}},
		{attestation: &zondpb.IndexedAttestation{
			Data: util.HydrateAttestationData(&zondpb.AttestationData{
				Target: &zondpb.Checkpoint{
					Epoch: 4,
				},
			}),
			AttestingIndices: []uint64{21, 72},
		}},
		{attestation: &zondpb.IndexedAttestation{
			Data: util.HydrateAttestationData(&zondpb.AttestationData{
				Target: &zondpb.Checkpoint{
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
	indexedAtt1 := &zondpb.IndexedAttestation{
		AttestingIndices: make([]uint64, params.BeaconConfig().MaxValidatorsPerCommittee+5),
	}

	for i := uint64(0); i < params.BeaconConfig().MaxValidatorsPerCommittee+5; i++ {
		indexedAtt1.AttestingIndices[i] = i
		indexedAtt1.Data = &zondpb.AttestationData{
			Target: &zondpb.Checkpoint{
				Epoch: primitives.Epoch(i),
			},
		}
	}

	want := "validator indices count exceeds MAX_VALIDATORS_PER_COMMITTEE"
	st, err := state_native.InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{})
	require.NoError(t, err)
	err = blocks.VerifyIndexedAttestation(context.Background(), st, indexedAtt1)
	assert.ErrorContains(t, want, err)
}

func TestValidateIndexedAttestation_BadAttestationsSignatureSet(t *testing.T) {
	beaconState, keys := util.DeterministicGenesisStateCapella(t, 256)

	sig := keys[0].Sign([]byte{'t', 'e', 's', 't'})
	list := bitfield.Bitlist{0b111}
	var atts []*zondpb.Attestation
	for i := uint64(0); i < 1000; i++ {
		atts = append(atts, &zondpb.Attestation{
			Data: &zondpb.AttestationData{
				CommitteeIndex: 1,
				Slot:           1,
			},
			Signatures:      [][]byte{sig.Marshal(), sig.Marshal()},
			AggregationBits: list,
		})
	}

	want := "nil or missing indexed attestation data"
	_, err := blocks.AttestationSignatureBatch(context.Background(), beaconState, atts)
	assert.ErrorContains(t, want, err)

	atts = []*zondpb.Attestation{}
	list = bitfield.Bitlist{0b100}
	for i := uint64(0); i < 1000; i++ {
		atts = append(atts, &zondpb.Attestation{
			Data: &zondpb.AttestationData{
				CommitteeIndex: 1,
				Slot:           1,
				Target: &zondpb.Checkpoint{
					Root: []byte{},
				},
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
	validators := make([]*zondpb.Validator, numOfValidators)
	_, keys, err := util.DeterministicDepositsAndKeys(numOfValidators)
	require.NoError(t, err)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			PublicKey:             keys[i].PublicKey().Marshal(),
			WithdrawalCredentials: make([]byte, 32),
		}
	}

	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(35))
	require.NoError(t, st.SetValidators(validators))
	require.NoError(t, st.SetFork(&zondpb.Fork{
		Epoch:           1,
		CurrentVersion:  []byte{0, 1, 2, 3},
		PreviousVersion: params.BeaconConfig().GenesisForkVersion,
	}))

	comm1, err := helpers.BeaconCommitteeFromState(context.Background(), st, 1 /*slot*/, 0 /*committeeIndex*/)
	require.NoError(t, err)
	att1 := util.HydrateAttestation(&zondpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm1))),
		Data: &zondpb.AttestationData{
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
		sigs = append(sigs, keys[u].Sign(root[:]).Marshal())
	}
	att1.Signatures = sigs

	comm2, err := helpers.BeaconCommitteeFromState(context.Background(), st, 1*params.BeaconConfig().SlotsPerEpoch+1 /*slot*/, 1 /*committeeIndex*/)
	require.NoError(t, err)
	att2 := util.HydrateAttestation(&zondpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm2))),
		Data: &zondpb.AttestationData{
			Slot:           1*params.BeaconConfig().SlotsPerEpoch + 1,
			CommitteeIndex: 1,
		},
	})
	currDomain, err := signing.Domain(st.Fork(), st.Fork().Epoch, params.BeaconConfig().DomainBeaconAttester, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	root, err = signing.ComputeSigningRoot(att2.Data, currDomain)
	require.NoError(t, err)
	sigs = nil
	for i, u := range comm2 {
		att2.AggregationBits.SetBitAt(uint64(i), true)
		sigs = append(sigs, keys[u].Sign(root[:]).Marshal())
	}
	att2.Signatures = sigs
}

func TestRetrieveAttestationSignatureSet_VerifiesMultipleAttestations(t *testing.T) {
	ctx := context.Background()
	numOfValidators := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))
	validators := make([]*zondpb.Validator, numOfValidators)
	_, keys, err := util.DeterministicDepositsAndKeys(numOfValidators)
	require.NoError(t, err)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			PublicKey:             keys[i].PublicKey().Marshal(),
			WithdrawalCredentials: make([]byte, 32),
		}
	}

	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(5))
	require.NoError(t, st.SetValidators(validators))

	comm1, err := helpers.BeaconCommitteeFromState(context.Background(), st, 1 /*slot*/, 0 /*committeeIndex*/)
	require.NoError(t, err)
	att1 := util.HydrateAttestation(&zondpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm1))),
		Data: &zondpb.AttestationData{
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
		sigs = append(sigs, keys[u].Sign(root[:]).Marshal())
	}
	att1.Signatures = sigs

	comm2, err := helpers.BeaconCommitteeFromState(context.Background(), st, 1 /*slot*/, 1 /*committeeIndex*/)
	require.NoError(t, err)
	att2 := util.HydrateAttestation(&zondpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm2))),
		Data: &zondpb.AttestationData{
			Slot:           1,
			CommitteeIndex: 1,
		},
	})
	root, err = signing.ComputeSigningRoot(att2.Data, domain)
	require.NoError(t, err)
	sigs = nil
	for i, u := range comm2 {
		att2.AggregationBits.SetBitAt(uint64(i), true)
		sigs = append(sigs, keys[u].Sign(root[:]).Marshal())
	}
	att2.Signatures = sigs

	set, err := blocks.AttestationSignatureBatch(ctx, st, []*zondpb.Attestation{att1, att2})
	require.NoError(t, err)
	verified, err := set.Verify()
	require.NoError(t, err)
	assert.Equal(t, true, verified, "Multiple signatures were unable to be verified.")
}

func TestRetrieveAttestationSignatureSet_AcrossFork(t *testing.T) {
	ctx := context.Background()
	numOfValidators := uint64(params.BeaconConfig().SlotsPerEpoch.Mul(4))
	validators := make([]*zondpb.Validator, numOfValidators)
	_, keys, err := util.DeterministicDepositsAndKeys(numOfValidators)
	require.NoError(t, err)
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
			PublicKey:             keys[i].PublicKey().Marshal(),
			WithdrawalCredentials: make([]byte, 32),
		}
	}

	st, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, st.SetSlot(5))
	require.NoError(t, st.SetValidators(validators))
	require.NoError(t, st.SetFork(&zondpb.Fork{Epoch: 1, CurrentVersion: []byte{0, 1, 2, 3}, PreviousVersion: []byte{0, 1, 1, 1}}))

	comm1, err := helpers.BeaconCommitteeFromState(ctx, st, 1 /*slot*/, 0 /*committeeIndex*/)
	require.NoError(t, err)
	att1 := util.HydrateAttestation(&zondpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm1))),
		Data: &zondpb.AttestationData{
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
		sigs = append(sigs, keys[u].Sign(root[:]).Marshal())
	}
	att1.Signatures = sigs

	comm2, err := helpers.BeaconCommitteeFromState(ctx, st, 1 /*slot*/, 1 /*committeeIndex*/)
	require.NoError(t, err)
	att2 := util.HydrateAttestation(&zondpb.Attestation{
		AggregationBits: bitfield.NewBitlist(uint64(len(comm2))),
		Data: &zondpb.AttestationData{
			Slot:           1,
			CommitteeIndex: 1,
		},
	})
	root, err = signing.ComputeSigningRoot(att2.Data, domain)
	require.NoError(t, err)
	sigs = nil
	for i, u := range comm2 {
		att2.AggregationBits.SetBitAt(uint64(i), true)
		sigs = append(sigs, keys[u].Sign(root[:]).Marshal())
	}
	att2.Signatures = sigs

	_, err = blocks.AttestationSignatureBatch(ctx, st, []*zondpb.Attestation{att1, att2})
	require.NoError(t, err)
}
