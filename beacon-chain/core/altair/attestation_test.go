package altair_test

import (
	"context"
	"fmt"
	"testing"

	fuzz "github.com/google/gofuzz"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/math"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestProcessAttestations_InclusionDelayFailure(t *testing.T) {
	attestations := []*zondpb.Attestation{
		util.HydrateAttestation(&zondpb.Attestation{
			Data: &zondpb.AttestationData{
				Target: &zondpb.Checkpoint{Epoch: 0, Root: make([]byte, fieldparams.RootLength)},
				Slot:   5,
			},
		}),
	}
	b := util.NewBeaconBlockCapella()
	b.Block = &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			Attestations: attestations,
		},
	}
	beaconState, _ := util.DeterministicGenesisStateCapella(t, 100)

	want := fmt.Sprintf(
		"attestation slot %d + inclusion delay %d > state slot %d",
		attestations[0].Data.Slot,
		params.BeaconConfig().MinAttestationInclusionDelay,
		beaconState.Slot(),
	)
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = altair.ProcessAttestationsNoVerifySignature(context.Background(), beaconState, wsb)
	require.ErrorContains(t, want, err)
}

func TestProcessAttestations_NeitherCurrentNorPrevEpoch(t *testing.T) {
	att := util.HydrateAttestation(&zondpb.Attestation{
		Data: &zondpb.AttestationData{
			Source: &zondpb.Checkpoint{Epoch: 0, Root: []byte("hello-world")},
			Target: &zondpb.Checkpoint{Epoch: 0}}})

	b := util.NewBeaconBlockCapella()
	b.Block = &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			Attestations: []*zondpb.Attestation{att},
		},
	}
	beaconState, _ := util.DeterministicGenesisStateCapella(t, 100)
	err := beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().SlotsPerEpoch*4 + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)
	pfc := beaconState.PreviousJustifiedCheckpoint()
	pfc.Root = []byte("hello-world")
	require.NoError(t, beaconState.SetPreviousJustifiedCheckpoint(pfc))

	want := fmt.Sprintf(
		"expected target epoch (%d) to be the previous epoch (%d) or the current epoch (%d)",
		att.Data.Target.Epoch,
		time.PrevEpoch(beaconState),
		time.CurrentEpoch(beaconState),
	)
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = altair.ProcessAttestationsNoVerifySignature(context.Background(), beaconState, wsb)
	require.ErrorContains(t, want, err)
}

func TestProcessAttestations_CurrentEpochFFGDataMismatches(t *testing.T) {
	attestations := []*zondpb.Attestation{
		{
			Data: &zondpb.AttestationData{
				Target: &zondpb.Checkpoint{Epoch: 0, Root: make([]byte, fieldparams.RootLength)},
				Source: &zondpb.Checkpoint{Epoch: 1, Root: make([]byte, fieldparams.RootLength)},
			},
			AggregationBits: bitfield.Bitlist{0x09},
		},
	}
	b := util.NewBeaconBlockCapella()
	b.Block = &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			Attestations: attestations,
		},
	}
	beaconState, _ := util.DeterministicGenesisStateCapella(t, 100)
	require.NoError(t, beaconState.SetSlot(beaconState.Slot()+params.BeaconConfig().MinAttestationInclusionDelay))
	cfc := beaconState.CurrentJustifiedCheckpoint()
	cfc.Root = []byte("hello-world")
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(cfc))

	want := "source check point not equal to current justified checkpoint"
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = altair.ProcessAttestationsNoVerifySignature(context.Background(), beaconState, wsb)
	require.ErrorContains(t, want, err)
	b.Block.Body.Attestations[0].Data.Source.Epoch = time.CurrentEpoch(beaconState)
	b.Block.Body.Attestations[0].Data.Source.Root = []byte{}
	wsb, err = blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = altair.ProcessAttestationsNoVerifySignature(context.Background(), beaconState, wsb)
	require.ErrorContains(t, want, err)
}

func TestProcessAttestations_PrevEpochFFGDataMismatches(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateCapella(t, 100)

	aggBits := bitfield.NewBitlist(3)
	aggBits.SetBitAt(0, true)
	attestations := []*zondpb.Attestation{
		{
			Data: &zondpb.AttestationData{
				Source: &zondpb.Checkpoint{Epoch: 1, Root: make([]byte, fieldparams.RootLength)},
				Target: &zondpb.Checkpoint{Epoch: 1, Root: make([]byte, fieldparams.RootLength)},
				Slot:   params.BeaconConfig().SlotsPerEpoch,
			},
			AggregationBits: aggBits,
		},
	}
	b := util.NewBeaconBlockCapella()
	b.Block = &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			Attestations: attestations,
		},
	}

	err := beaconState.SetSlot(beaconState.Slot() + 2*params.BeaconConfig().SlotsPerEpoch)
	require.NoError(t, err)
	pfc := beaconState.PreviousJustifiedCheckpoint()
	pfc.Root = []byte("hello-world")
	require.NoError(t, beaconState.SetPreviousJustifiedCheckpoint(pfc))

	want := "source check point not equal to previous justified checkpoint"
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = altair.ProcessAttestationsNoVerifySignature(context.Background(), beaconState, wsb)
	require.ErrorContains(t, want, err)
	b.Block.Body.Attestations[0].Data.Source.Epoch = time.PrevEpoch(beaconState)
	b.Block.Body.Attestations[0].Data.Target.Epoch = time.PrevEpoch(beaconState)
	b.Block.Body.Attestations[0].Data.Source.Root = []byte{}
	wsb, err = blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = altair.ProcessAttestationsNoVerifySignature(context.Background(), beaconState, wsb)
	require.ErrorContains(t, want, err)
}

func TestProcessAttestations_InvalidAggregationBitsLength(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateCapella(t, 256)

	aggBits := bitfield.NewBitlist(4)
	att := &zondpb.Attestation{
		Data: &zondpb.AttestationData{
			Source: &zondpb.Checkpoint{Epoch: 0, Root: []byte("hello-world")},
			Target: &zondpb.Checkpoint{Epoch: 0}},
		AggregationBits: aggBits,
	}

	b := util.NewBeaconBlockCapella()
	b.Block = &zondpb.BeaconBlockCapella{
		Body: &zondpb.BeaconBlockBodyCapella{
			Attestations: []*zondpb.Attestation{att},
		},
	}

	err := beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)

	cfc := beaconState.CurrentJustifiedCheckpoint()
	cfc.Root = []byte("hello-world")
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(cfc))

	expected := "failed to verify aggregation bitfield: wanted participants bitfield length 2, got: 4"
	wsb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = altair.ProcessAttestationsNoVerifySignature(context.Background(), beaconState, wsb)
	require.ErrorContains(t, expected, err)
}

func TestProcessAttestations_OK(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 256)

	aggBits := bitfield.NewBitlist(2)
	aggBits.SetBitAt(0, true)
	var mockRoot [32]byte
	copy(mockRoot[:], "hello-world")
	att := util.HydrateAttestation(&zondpb.Attestation{
		Data: &zondpb.AttestationData{
			Source: &zondpb.Checkpoint{Root: mockRoot[:]},
			Target: &zondpb.Checkpoint{Root: mockRoot[:]},
		},
		AggregationBits: aggBits,
	})

	cfc := beaconState.CurrentJustifiedCheckpoint()
	cfc.Root = mockRoot[:]
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(cfc))

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	require.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	require.NoError(t, err)
	sigs := make([][]byte, len(attestingIndices))
	for i, indice := range attestingIndices {
		sb, err := signing.ComputeDomainAndSign(beaconState, 0, att.Data, params.BeaconConfig().DomainBeaconAttester, privKeys[indice])
		require.NoError(t, err)
		sigs[i] = sb
	}
	att.Signatures = sigs

	block := util.NewBeaconBlockCapella()
	block.Block.Body.Attestations = []*zondpb.Attestation{att}

	err = beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = altair.ProcessAttestationsNoVerifySignature(context.Background(), beaconState, wsb)
	require.NoError(t, err)
}

func TestProcessAttestationNoVerify_SourceTargetHead(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateCapella(t, 256)
	err := beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)

	aggBits := bitfield.NewBitlist(2)
	aggBits.SetBitAt(0, true)
	aggBits.SetBitAt(1, true)
	r, err := helpers.BlockRootAtSlot(beaconState, 0)
	require.NoError(t, err)
	att := &zondpb.Attestation{
		Data: &zondpb.AttestationData{
			BeaconBlockRoot: r,
			Source:          &zondpb.Checkpoint{Epoch: 0, Root: make([]byte, fieldparams.RootLength)},
			Target:          &zondpb.Checkpoint{Epoch: 0, Root: make([]byte, fieldparams.RootLength)},
		},
		AggregationBits: aggBits,
	}
	var zeroSig [4595]byte
	att.Signatures = [][]byte{zeroSig[:], zeroSig[:]}

	ckp := beaconState.CurrentJustifiedCheckpoint()
	copy(ckp.Root, make([]byte, fieldparams.RootLength))
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(ckp))

	b, err := helpers.TotalActiveBalance(beaconState)
	require.NoError(t, err)
	beaconState, err = altair.ProcessAttestationNoVerifySignature(context.Background(), beaconState, att, b)
	require.NoError(t, err)

	p, err := beaconState.CurrentEpochParticipation()
	require.NoError(t, err)

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	require.NoError(t, err)
	indices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	require.NoError(t, err)
	for _, index := range indices {
		has, err := altair.HasValidatorFlag(p[index], params.BeaconConfig().TimelyHeadFlagIndex)
		require.NoError(t, err)
		require.Equal(t, true, has)
		has, err = altair.HasValidatorFlag(p[index], params.BeaconConfig().TimelySourceFlagIndex)
		require.NoError(t, err)
		require.Equal(t, true, has)
		has, err = altair.HasValidatorFlag(p[index], params.BeaconConfig().TimelyTargetFlagIndex)
		require.NoError(t, err)
		require.Equal(t, true, has)
	}
}

func TestValidatorFlag_Has(t *testing.T) {
	tests := []struct {
		name     string
		set      uint8
		expected []uint8
	}{
		{name: "none",
			set:      0,
			expected: []uint8{},
		},
		{
			name:     "source",
			set:      1,
			expected: []uint8{params.BeaconConfig().TimelySourceFlagIndex},
		},
		{
			name:     "target",
			set:      2,
			expected: []uint8{params.BeaconConfig().TimelyTargetFlagIndex},
		},
		{
			name:     "head",
			set:      4,
			expected: []uint8{params.BeaconConfig().TimelyHeadFlagIndex},
		},
		{
			name:     "source, target",
			set:      3,
			expected: []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex},
		},
		{
			name:     "source, head",
			set:      5,
			expected: []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
		},
		{
			name:     "target, head",
			set:      6,
			expected: []uint8{params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex},
		},
		{
			name:     "source, target, head",
			set:      7,
			expected: []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, f := range tt.expected {
				has, err := altair.HasValidatorFlag(tt.set, f)
				require.NoError(t, err)
				require.Equal(t, true, has)
			}
		})
	}
}

func TestValidatorFlag_Has_ExceedsLength(t *testing.T) {
	_, err := altair.HasValidatorFlag(0, 8)
	require.ErrorContains(t, "flag position exceeds length", err)
}

func TestValidatorFlag_Add(t *testing.T) {
	tests := []struct {
		name          string
		set           []uint8
		expectedTrue  []uint8
		expectedFalse []uint8
	}{
		{name: "none",
			set:           []uint8{},
			expectedTrue:  []uint8{},
			expectedFalse: []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
		},
		{
			name:          "source",
			set:           []uint8{params.BeaconConfig().TimelySourceFlagIndex},
			expectedTrue:  []uint8{params.BeaconConfig().TimelySourceFlagIndex},
			expectedFalse: []uint8{params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
		},
		{
			name:          "source, target",
			set:           []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex},
			expectedTrue:  []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex},
			expectedFalse: []uint8{params.BeaconConfig().TimelyHeadFlagIndex},
		},
		{
			name:          "source, target, head",
			set:           []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
			expectedTrue:  []uint8{params.BeaconConfig().TimelySourceFlagIndex, params.BeaconConfig().TimelyTargetFlagIndex, params.BeaconConfig().TimelyHeadFlagIndex},
			expectedFalse: []uint8{},
		},
	}
	var err error
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := uint8(0)
			for _, f := range tt.set {
				b, err = altair.AddValidatorFlag(b, f)
				require.NoError(t, err)
			}
			for _, f := range tt.expectedFalse {
				has, err := altair.HasValidatorFlag(b, f)
				require.NoError(t, err)
				require.Equal(t, false, has)
			}
			for _, f := range tt.expectedTrue {
				has, err := altair.HasValidatorFlag(b, f)
				require.NoError(t, err)
				require.Equal(t, true, has)
			}
		})
	}
}

func TestValidatorFlag_Add_ExceedsLength(t *testing.T) {
	_, err := altair.AddValidatorFlag(0, 8)
	require.ErrorContains(t, "flag position exceeds length", err)
}

func TestFuzzProcessAttestationsNoVerify_10000(t *testing.T) {
	fuzzer := fuzz.NewWithSeed(0)
	st := &zondpb.BeaconStateCapella{}
	b := &zondpb.SignedBeaconBlockCapella{Block: &zondpb.BeaconBlockCapella{}}
	for i := 0; i < 10000; i++ {
		fuzzer.Fuzz(st)
		fuzzer.Fuzz(b)
		if b.Block == nil {
			b.Block = &zondpb.BeaconBlockCapella{}
		}
		s, err := state_native.InitializeFromProtoUnsafeCapella(st)
		require.NoError(t, err)
		if b.Block == nil || b.Block.Body == nil {
			continue
		}
		wsb, err := blocks.NewSignedBeaconBlock(b)
		require.NoError(t, err)
		r, err := altair.ProcessAttestationsNoVerifySignature(context.Background(), s, wsb)
		if err != nil && r != nil {
			t.Fatalf("return value should be nil on err. found: %v on error: %v for state: %v and block: %v", r, err, s, b)
		}
	}
}

func TestSetParticipationAndRewardProposer(t *testing.T) {
	cfg := params.BeaconConfig()
	sourceFlagIndex := cfg.TimelySourceFlagIndex
	targetFlagIndex := cfg.TimelyTargetFlagIndex
	headFlagIndex := cfg.TimelyHeadFlagIndex
	tests := []struct {
		name                string
		indices             []uint64
		epochParticipation  []byte
		participatedFlags   map[uint8]bool
		epoch               primitives.Epoch
		wantedBalance       uint64
		wantedParticipation []byte
	}{
		{name: "none participated",
			indices: []uint64{}, epochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0}, participatedFlags: map[uint8]bool{
				sourceFlagIndex: false,
				targetFlagIndex: false,
				headFlagIndex:   false,
			},
			wantedParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0},
			wantedBalance:       40000000000000,
		},
		{name: "some participated without flags",
			indices: []uint64{0, 1, 2, 3}, epochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0}, participatedFlags: map[uint8]bool{
				sourceFlagIndex: false,
				targetFlagIndex: false,
				headFlagIndex:   false,
			},
			wantedParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0},
			wantedBalance:       40000000000000,
		},
		{name: "some participated with some flags",
			indices: []uint64{0, 1, 2, 3}, epochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0}, participatedFlags: map[uint8]bool{
				sourceFlagIndex: true,
				targetFlagIndex: true,
				headFlagIndex:   false,
			},
			wantedParticipation: []byte{3, 3, 3, 3, 0, 0, 0, 0},
			wantedBalance:       40000003185714,
		},
		{name: "all participated with some flags",
			indices: []uint64{0, 1, 2, 3, 4, 5, 6, 7}, epochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0}, participatedFlags: map[uint8]bool{
				sourceFlagIndex: true,
				targetFlagIndex: false,
				headFlagIndex:   false,
			},
			wantedParticipation: []byte{1, 1, 1, 1, 1, 1, 1, 1},
			wantedBalance:       40000002230000,
		},
		{name: "all participated with all flags",
			indices: []uint64{0, 1, 2, 3, 4, 5, 6, 7}, epochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0}, participatedFlags: map[uint8]bool{
				sourceFlagIndex: true,
				targetFlagIndex: true,
				headFlagIndex:   true,
			},
			wantedParticipation: []byte{7, 7, 7, 7, 7, 7, 7, 7},
			wantedBalance:       40000008601428,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beaconState, _ := util.DeterministicGenesisStateCapella(t, params.BeaconConfig().MaxValidatorsPerCommittee)
			require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch))

			currentEpoch := time.CurrentEpoch(beaconState)
			if test.epoch == currentEpoch {
				require.NoError(t, beaconState.SetCurrentParticipationBits(test.epochParticipation))
			} else {
				require.NoError(t, beaconState.SetPreviousParticipationBits(test.epochParticipation))
			}

			b, err := helpers.TotalActiveBalance(beaconState)
			require.NoError(t, err)
			st, err := altair.SetParticipationAndRewardProposer(context.Background(), beaconState, test.epoch, test.indices, test.participatedFlags, b)
			require.NoError(t, err)

			i, err := helpers.BeaconProposerIndex(context.Background(), st)
			require.NoError(t, err)
			b, err = beaconState.BalanceAtIndex(i)
			require.NoError(t, err)
			require.Equal(t, test.wantedBalance, b)

			if test.epoch == currentEpoch {
				p, err := beaconState.CurrentEpochParticipation()
				require.NoError(t, err)
				require.DeepSSZEqual(t, test.wantedParticipation, p)
			} else {
				p, err := beaconState.PreviousEpochParticipation()
				require.NoError(t, err)
				require.DeepSSZEqual(t, test.wantedParticipation, p)
			}
		})
	}
}

func TestEpochParticipation(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateCapella(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	cfg := params.BeaconConfig()
	sourceFlagIndex := cfg.TimelySourceFlagIndex
	targetFlagIndex := cfg.TimelyTargetFlagIndex
	headFlagIndex := cfg.TimelyHeadFlagIndex
	tests := []struct {
		name                     string
		indices                  []uint64
		epochParticipation       []byte
		participatedFlags        map[uint8]bool
		wantedNumerator          uint64
		wantedEpochParticipation []byte
	}{
		{name: "none participated",
			indices: []uint64{}, epochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0}, participatedFlags: map[uint8]bool{
				sourceFlagIndex: false,
				targetFlagIndex: false,
				headFlagIndex:   false,
			},
			wantedNumerator:          0,
			wantedEpochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0},
		},
		{name: "some participated without flags",
			indices: []uint64{0, 1, 2, 3}, epochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0}, participatedFlags: map[uint8]bool{
				sourceFlagIndex: false,
				targetFlagIndex: false,
				headFlagIndex:   false,
			},
			wantedNumerator:          0,
			wantedEpochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0},
		},
		{name: "some participated with some flags",
			indices: []uint64{0, 1, 2, 3}, epochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0}, participatedFlags: map[uint8]bool{
				sourceFlagIndex: true,
				targetFlagIndex: true,
				headFlagIndex:   false,
			},
			wantedNumerator:          1427200000,
			wantedEpochParticipation: []byte{3, 3, 3, 3, 0, 0, 0, 0},
		},
		{name: "all participated with some flags",
			indices: []uint64{0, 1, 2, 3, 4, 5, 6, 7}, epochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0}, participatedFlags: map[uint8]bool{
				sourceFlagIndex: true,
				targetFlagIndex: false,
				headFlagIndex:   false,
			},
			wantedNumerator:          999040000,
			wantedEpochParticipation: []byte{1, 1, 1, 1, 1, 1, 1, 1},
		},
		{name: "all participated with all flags",
			indices: []uint64{0, 1, 2, 3, 4, 5, 6, 7}, epochParticipation: []byte{0, 0, 0, 0, 0, 0, 0, 0}, participatedFlags: map[uint8]bool{
				sourceFlagIndex: true,
				targetFlagIndex: true,
				headFlagIndex:   true,
			},
			wantedNumerator:          3853440000,
			wantedEpochParticipation: []byte{7, 7, 7, 7, 7, 7, 7, 7},
		},
	}
	for _, test := range tests {
		b, err := helpers.TotalActiveBalance(beaconState)
		require.NoError(t, err)
		n, p, err := altair.EpochParticipation(beaconState, test.indices, test.epochParticipation, test.participatedFlags, b)
		require.NoError(t, err)
		require.Equal(t, test.wantedNumerator, n)
		require.DeepSSZEqual(t, test.wantedEpochParticipation, p)
	}
}

func TestRewardProposer(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateCapella(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, beaconState.SetSlot(1))
	tests := []struct {
		rewardNumerator uint64
		want            uint64
	}{
		{rewardNumerator: 1, want: 40000000000000},
		{rewardNumerator: 10000, want: 40000000000022},
		{rewardNumerator: 1000000, want: 40000000002254},
		{rewardNumerator: 1000000000, want: 40000002234396},
		{rewardNumerator: 1000000000000, want: 40002234377253},
	}
	for _, test := range tests {
		require.NoError(t, altair.RewardProposer(context.Background(), beaconState, test.rewardNumerator))
		i, err := helpers.BeaconProposerIndex(context.Background(), beaconState)
		require.NoError(t, err)
		b, err := beaconState.BalanceAtIndex(i)
		require.NoError(t, err)
		require.Equal(t, test.want, b)
	}
}

func TestAttestationParticipationFlagIndices(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateCapella(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, beaconState.SetSlot(1))
	cfg := params.BeaconConfig()
	sourceFlagIndex := cfg.TimelySourceFlagIndex
	targetFlagIndex := cfg.TimelyTargetFlagIndex
	headFlagIndex := cfg.TimelyHeadFlagIndex

	tests := []struct {
		name                 string
		inputState           state.BeaconState
		inputData            *zondpb.AttestationData
		inputDelay           primitives.Slot
		participationIndices map[uint8]bool
	}{
		{
			name: "none",
			inputState: func() state.BeaconState {
				return beaconState
			}(),
			inputData: &zondpb.AttestationData{
				Source: &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
				Target: &zondpb.Checkpoint{},
			},
			inputDelay:           params.BeaconConfig().SlotsPerEpoch,
			participationIndices: map[uint8]bool{},
		},
		{
			name: "participated source",
			inputState: func() state.BeaconState {
				return beaconState
			}(),
			inputData: &zondpb.AttestationData{
				Source: &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
				Target: &zondpb.Checkpoint{},
			},
			inputDelay: primitives.Slot(math.IntegerSquareRoot(uint64(cfg.SlotsPerEpoch)) - 1),
			participationIndices: map[uint8]bool{
				sourceFlagIndex: true,
			},
		},
		{
			name: "participated source and target",
			inputState: func() state.BeaconState {
				return beaconState
			}(),
			inputData: &zondpb.AttestationData{
				Source: &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
				Target: &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
			},
			inputDelay: primitives.Slot(math.IntegerSquareRoot(uint64(cfg.SlotsPerEpoch)) - 1),
			participationIndices: map[uint8]bool{
				sourceFlagIndex: true,
				targetFlagIndex: true,
			},
		},
		{
			name: "participated source and target with delay",
			inputState: func() state.BeaconState {
				return beaconState
			}(),
			inputData: &zondpb.AttestationData{
				Source: &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
				Target: &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
			},
			inputDelay: params.BeaconConfig().SlotsPerEpoch + 1,
			participationIndices: map[uint8]bool{
				targetFlagIndex: true,
			},
		},
		{
			name: "participated source and target and head",
			inputState: func() state.BeaconState {
				return beaconState
			}(),
			inputData: &zondpb.AttestationData{
				BeaconBlockRoot: params.BeaconConfig().ZeroHash[:],
				Source:          &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
				Target:          &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
			},
			inputDelay: 1,
			participationIndices: map[uint8]bool{
				sourceFlagIndex: true,
				targetFlagIndex: true,
				headFlagIndex:   true,
			},
		},
	}
	for _, test := range tests {
		flagIndices, err := altair.AttestationParticipationFlagIndices(test.inputState, test.inputData, test.inputDelay)
		require.NoError(t, err)
		require.DeepEqual(t, test.participationIndices, flagIndices)
	}
}

func TestMatchingStatus(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateCapella(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, beaconState.SetSlot(1))
	tests := []struct {
		name          string
		inputState    state.BeaconState
		inputData     *zondpb.AttestationData
		inputCheckpt  *zondpb.Checkpoint
		matchedSource bool
		matchedTarget bool
		matchedHead   bool
	}{
		{
			name:       "non matched",
			inputState: beaconState,
			inputData: &zondpb.AttestationData{
				Source: &zondpb.Checkpoint{Epoch: 1},
				Target: &zondpb.Checkpoint{},
			},
			inputCheckpt: &zondpb.Checkpoint{},
		},
		{
			name:       "source matched",
			inputState: beaconState,
			inputData: &zondpb.AttestationData{
				Source: &zondpb.Checkpoint{},
				Target: &zondpb.Checkpoint{},
			},
			inputCheckpt:  &zondpb.Checkpoint{},
			matchedSource: true,
		},
		{
			name:       "target matched",
			inputState: beaconState,
			inputData: &zondpb.AttestationData{
				Source: &zondpb.Checkpoint{Epoch: 1},
				Target: &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
			},
			inputCheckpt:  &zondpb.Checkpoint{},
			matchedTarget: true,
		},
		{
			name:       "head matched",
			inputState: beaconState,
			inputData: &zondpb.AttestationData{
				Source:          &zondpb.Checkpoint{Epoch: 1},
				Target:          &zondpb.Checkpoint{},
				BeaconBlockRoot: params.BeaconConfig().ZeroHash[:],
			},
			inputCheckpt: &zondpb.Checkpoint{},
			matchedHead:  true,
		},
		{
			name:       "everything matched",
			inputState: beaconState,
			inputData: &zondpb.AttestationData{
				Source:          &zondpb.Checkpoint{},
				Target:          &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]},
				BeaconBlockRoot: params.BeaconConfig().ZeroHash[:],
			},
			inputCheckpt:  &zondpb.Checkpoint{},
			matchedSource: true,
			matchedTarget: true,
			matchedHead:   true,
		},
	}

	for _, test := range tests {
		src, tgt, head, err := altair.MatchingStatus(test.inputState, test.inputData, test.inputCheckpt)
		require.NoError(t, err)
		require.Equal(t, test.matchedSource, src)
		require.Equal(t, test.matchedTarget, tgt)
		require.Equal(t, test.matchedHead, head)
	}
}
