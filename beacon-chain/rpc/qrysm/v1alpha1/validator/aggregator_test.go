package validator

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/theQRL/go-bitfield"
	mock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	"github.com/theQRL/qrysm/v4/beacon-chain/operations/attestations"
	mockp2p "github.com/theQRL/qrysm/v4/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/rpc/core"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	state_native "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native"
	mockSync "github.com/theQRL/qrysm/v4/beacon-chain/sync/initial-sync/testing"
	field_params "github.com/theQRL/qrysm/v4/config/fieldparams"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/crypto/dilithium"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation"
	attaggregation "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation/aggregation/attestations"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestSubmitAggregateAndProof_Syncing(t *testing.T) {
	ctx := context.Background()

	s, err := state_native.InitializeFromProtoUnsafeCapella(&zondpb.BeaconStateCapella{})
	require.NoError(t, err)

	aggregatorServer := &Server{
		HeadFetcher: &mock.ChainService{State: s},
		SyncChecker: &mockSync.Sync{IsSyncing: true},
	}

	req := &zondpb.AggregateSelectionRequest{CommitteeIndex: 1}
	wanted := "Syncing to latest head, not ready to respond"
	_, err = aggregatorServer.SubmitAggregateSelectionProof(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestSubmitAggregateAndProof_CantFindValidatorIndex(t *testing.T) {
	ctx := context.Background()

	s, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
	})
	require.NoError(t, err)

	server := &Server{
		HeadFetcher:           &mock.ChainService{State: s},
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
	}

	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte{'A'})
	req := &zondpb.AggregateSelectionRequest{CommitteeIndex: 1, SlotSignature: sig.Marshal(), PublicKey: pubKey(3)}
	wanted := "Could not locate validator index in DB"
	_, err = server.SubmitAggregateSelectionProof(ctx, req)
	assert.ErrorContains(t, wanted, err)
}

func TestSubmitAggregateAndProof_IsAggregatorAndNoAtts(t *testing.T) {
	ctx := context.Background()

	s, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),
		Validators: []*zondpb.Validator{
			{PublicKey: pubKey(0)},
			{PublicKey: pubKey(1)},
		},
	})
	require.NoError(t, err)

	server := &Server{
		HeadFetcher:           &mock.ChainService{State: s},
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		AttPool:               attestations.NewPool(),
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
	}

	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte{'A'})
	v, err := s.ValidatorAtIndex(1)
	require.NoError(t, err)
	pubKey := v.PublicKey
	req := &zondpb.AggregateSelectionRequest{CommitteeIndex: 1, SlotSignature: sig.Marshal(), PublicKey: pubKey}

	_, err = server.SubmitAggregateSelectionProof(ctx, req)
	assert.ErrorContains(t, "Could not find attestation for slot and committee in pool", err)
}

func TestSubmitAggregateAndProof_UnaggregateOk(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	c := params.MinimalSpecConfig().Copy()
	c.TargetAggregatorsPerCommittee = 16
	params.OverrideBeaconConfig(c)

	ctx := context.Background()

	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 32)
	att0, err := generateUnaggregatedAtt(beaconState, 0, privKeys)
	require.NoError(t, err)
	err = beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)

	aggregatorServer := &Server{
		HeadFetcher:           &mock.ChainService{State: beaconState},
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		AttPool:               attestations.NewPool(),
		P2P:                   &mockp2p.MockBroadcaster{},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
	}

	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte{'B'})
	v, err := beaconState.ValidatorAtIndex(1)
	require.NoError(t, err)
	pubKey := v.PublicKey
	req := &zondpb.AggregateSelectionRequest{CommitteeIndex: 1, SlotSignature: sig.Marshal(), PublicKey: pubKey}

	require.NoError(t, aggregatorServer.AttPool.SaveUnaggregatedAttestation(att0))
	_, err = aggregatorServer.SubmitAggregateSelectionProof(ctx, req)
	require.NoError(t, err)
}

func TestSubmitAggregateAndProof_AggregateOk(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	c := params.MinimalSpecConfig().Copy()
	c.TargetAggregatorsPerCommittee = 16
	params.OverrideBeaconConfig(c)

	ctx := context.Background()

	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 32)
	att0, err := generateAtt(beaconState, 0, privKeys)
	require.NoError(t, err)
	att1, err := generateAtt(beaconState, 2, privKeys)
	require.NoError(t, err)

	err = beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)

	aggregatorServer := &Server{
		HeadFetcher:           &mock.ChainService{State: beaconState},
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		AttPool:               attestations.NewPool(),
		P2P:                   &mockp2p.MockBroadcaster{},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
	}

	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte{'B'})
	v, err := beaconState.ValidatorAtIndex(1)
	require.NoError(t, err)
	pubKey := v.PublicKey
	req := &zondpb.AggregateSelectionRequest{CommitteeIndex: 1, SlotSignature: sig.Marshal(), PublicKey: pubKey}

	require.NoError(t, aggregatorServer.AttPool.SaveAggregatedAttestation(att0))
	require.NoError(t, aggregatorServer.AttPool.SaveAggregatedAttestation(att1))
	_, err = aggregatorServer.SubmitAggregateSelectionProof(ctx, req)
	require.NoError(t, err)

	aggregatedAtts := aggregatorServer.AttPool.AggregatedAttestations()
	wanted, err := attaggregation.AggregatePair(att0, att1)
	require.NoError(t, err)
	if reflect.DeepEqual(aggregatedAtts, wanted) {
		t.Error("Did not receive wanted attestation")
	}
}

func TestSubmitAggregateAndProof_AggregateNotOk(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	c := params.MinimalSpecConfig().Copy()
	c.TargetAggregatorsPerCommittee = 16
	params.OverrideBeaconConfig(c)

	ctx := context.Background()

	beaconState, _ := util.DeterministicGenesisStateCapella(t, 32)
	require.NoError(t, beaconState.SetSlot(beaconState.Slot()+params.BeaconConfig().MinAttestationInclusionDelay))

	aggregatorServer := &Server{
		HeadFetcher:           &mock.ChainService{State: beaconState},
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		AttPool:               attestations.NewPool(),
		P2P:                   &mockp2p.MockBroadcaster{},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
	}

	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte{'B'})
	v, err := beaconState.ValidatorAtIndex(1)
	require.NoError(t, err)
	pubKey := v.PublicKey
	req := &zondpb.AggregateSelectionRequest{CommitteeIndex: 1, SlotSignature: sig.Marshal(), PublicKey: pubKey}

	_, err = aggregatorServer.SubmitAggregateSelectionProof(ctx, req)
	assert.ErrorContains(t, "Could not find attestation for slot and committee in pool", err)

	aggregatedAtts := aggregatorServer.AttPool.AggregatedAttestations()
	assert.Equal(t, 0, len(aggregatedAtts), "Wanted aggregated attestation")
}

func generateAtt(state state.ReadOnlyBeaconState, index uint64, privKeys []dilithium.DilithiumKey) (*zondpb.Attestation, error) {
	aggBits := bitfield.NewBitlist(4)
	aggBits.SetBitAt(index, true)
	aggBits.SetBitAt(index+1, true)
	att := util.HydrateAttestation(&zondpb.Attestation{
		Data:            &zondpb.AttestationData{CommitteeIndex: 1},
		AggregationBits: aggBits,
	})
	committee, err := helpers.BeaconCommitteeFromState(context.Background(), state, att.Data.Slot, att.Data.CommitteeIndex)
	if err != nil {
		return nil, err
	}
	attestingIndices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	if err != nil {
		return nil, err
	}

	sigs := make([][]byte, len(attestingIndices))
	var zeroSig [4595]byte
	att.Signatures = [][]byte{zeroSig[:]}

	for i, indice := range attestingIndices {
		sb, err := signing.ComputeDomainAndSign(state, 0, att.Data, params.BeaconConfig().DomainBeaconAttester, privKeys[indice])
		if err != nil {
			return nil, err
		}
		sigs[i] = sb
	}

	att.Signatures = sigs

	return att, nil
}

func generateUnaggregatedAtt(state state.ReadOnlyBeaconState, index uint64, privKeys []dilithium.DilithiumKey) (*zondpb.Attestation, error) {
	aggBits := bitfield.NewBitlist(4)
	aggBits.SetBitAt(index, true)
	att := util.HydrateAttestation(&zondpb.Attestation{
		Data: &zondpb.AttestationData{
			CommitteeIndex: 1,
		},
		AggregationBits: aggBits,
	})
	committee, err := helpers.BeaconCommitteeFromState(context.Background(), state, att.Data.Slot, att.Data.CommitteeIndex)
	if err != nil {
		return nil, err
	}
	attestingIndices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	if err != nil {
		return nil, err
	}
	domain, err := signing.Domain(state.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, params.BeaconConfig().ZeroHash[:])
	if err != nil {
		return nil, err
	}

	sigs := make([][]byte, len(attestingIndices))
	var zeroSig [4595]byte
	att.Signatures = [][]byte{zeroSig[:]}

	for i, indice := range attestingIndices {
		hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, domain)
		if err != nil {
			return nil, err
		}
		sig := privKeys[indice].Sign(hashTreeRoot[:]).Marshal()
		sigs[i] = sig
	}

	att.Signatures = sigs

	return att, nil
}

func TestSubmitAggregateAndProof_PreferOwnAttestation(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	c := params.MinimalSpecConfig().Copy()
	c.TargetAggregatorsPerCommittee = 16
	params.OverrideBeaconConfig(c)

	ctx := context.Background()

	// This test creates 3 attestations. 0 and 2 have the same attestation data and can be
	// aggregated. 1 has the validator's signature making this request and that is the expected
	// attestation to sign, even though the aggregated 0&2 would have more aggregated bits.
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, 32)
	att0, err := generateAtt(beaconState, 0, privKeys)
	require.NoError(t, err)
	att0.Data.BeaconBlockRoot = bytesutil.PadTo([]byte("foo"), fieldparams.RootLength)
	att0.AggregationBits = bitfield.Bitlist{0b11100}
	att1, err := generateAtt(beaconState, 0, privKeys)
	require.NoError(t, err)
	att1.Data.BeaconBlockRoot = bytesutil.PadTo([]byte("bar"), fieldparams.RootLength)
	att1.AggregationBits = bitfield.Bitlist{0b11001}
	att2, err := generateAtt(beaconState, 2, privKeys)
	require.NoError(t, err)
	att2.Data.BeaconBlockRoot = bytesutil.PadTo([]byte("foo"), fieldparams.RootLength)
	att2.AggregationBits = bitfield.Bitlist{0b11110}

	err = beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)

	aggregatorServer := &Server{
		HeadFetcher:           &mock.ChainService{State: beaconState},
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		AttPool:               attestations.NewPool(),
		P2P:                   &mockp2p.MockBroadcaster{},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
	}

	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte{'B'})
	v, err := beaconState.ValidatorAtIndex(1)
	require.NoError(t, err)
	pubKey := v.PublicKey
	req := &zondpb.AggregateSelectionRequest{CommitteeIndex: 1, SlotSignature: sig.Marshal(), PublicKey: pubKey}

	err = aggregatorServer.AttPool.SaveAggregatedAttestations([]*zondpb.Attestation{
		att0,
		att1,
		att2,
	})
	require.NoError(t, err)

	res, err := aggregatorServer.SubmitAggregateSelectionProof(ctx, req)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, att1, res.AggregateAndProof.Aggregate, "Did not receive wanted attestation")
}

func TestSubmitAggregateAndProof_SelectsMostBitsWhenOwnAttestationNotPresent(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	c := params.MinimalSpecConfig().Copy()
	c.TargetAggregatorsPerCommittee = 16
	params.OverrideBeaconConfig(c)

	ctx := context.Background()

	// This test creates two distinct attestations, neither of which contain the validator's index,
	// index 0. This test should choose the most bits attestation, att1.
	beaconState, privKeys := util.DeterministicGenesisStateCapella(t, fieldparams.RootLength)
	att0, err := generateAtt(beaconState, 0, privKeys)
	require.NoError(t, err)
	att0.Data.BeaconBlockRoot = bytesutil.PadTo([]byte("foo"), fieldparams.RootLength)
	att0.AggregationBits = bitfield.Bitlist{0b11100}
	att1, err := generateAtt(beaconState, 2, privKeys)
	require.NoError(t, err)
	att1.Data.BeaconBlockRoot = bytesutil.PadTo([]byte("bar"), fieldparams.RootLength)
	att1.AggregationBits = bitfield.Bitlist{0b11110}

	err = beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)

	aggregatorServer := &Server{
		HeadFetcher:           &mock.ChainService{State: beaconState},
		SyncChecker:           &mockSync.Sync{IsSyncing: false},
		AttPool:               attestations.NewPool(),
		P2P:                   &mockp2p.MockBroadcaster{},
		TimeFetcher:           &mock.ChainService{Genesis: time.Now()},
		OptimisticModeFetcher: &mock.ChainService{Optimistic: false},
	}

	priv, err := dilithium.RandKey()
	require.NoError(t, err)
	sig := priv.Sign([]byte{'B'})
	v, err := beaconState.ValidatorAtIndex(1)
	require.NoError(t, err)
	pubKey := v.PublicKey
	req := &zondpb.AggregateSelectionRequest{CommitteeIndex: 1, SlotSignature: sig.Marshal(), PublicKey: pubKey}

	err = aggregatorServer.AttPool.SaveAggregatedAttestations([]*zondpb.Attestation{
		att0,
		att1,
	})
	require.NoError(t, err)

	res, err := aggregatorServer.SubmitAggregateSelectionProof(ctx, req)
	require.NoError(t, err)
	assert.DeepSSZEqual(t, att1, res.AggregateAndProof.Aggregate, "Did not receive wanted attestation")
}

func TestSubmitSignedAggregateSelectionProof_ZeroHashesSignatures(t *testing.T) {
	aggregatorServer := &Server{
		TimeFetcher: &mock.ChainService{Genesis: time.Now()},
	}
	req := &zondpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: &zondpb.SignedAggregateAttestationAndProof{
			Signature: make([]byte, field_params.DilithiumSignatureLength),
			Message: &zondpb.AggregateAttestationAndProof{
				Aggregate: &zondpb.Attestation{
					Data: &zondpb.AttestationData{},
				},
			},
		},
	}
	_, err := aggregatorServer.SubmitSignedAggregateSelectionProof(context.Background(), req)
	require.ErrorContains(t, "signed signatures can't be zero hashes", err)

	req = &zondpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: &zondpb.SignedAggregateAttestationAndProof{
			Signature: []byte{'a'},
			Message: &zondpb.AggregateAttestationAndProof{
				Aggregate: &zondpb.Attestation{
					Data: &zondpb.AttestationData{},
				},
				SelectionProof: make([]byte, field_params.DilithiumSignatureLength),
			},
		},
	}
	_, err = aggregatorServer.SubmitSignedAggregateSelectionProof(context.Background(), req)
	require.ErrorContains(t, "signed signatures can't be zero hashes", err)
}

func TestSubmitSignedAggregateSelectionProof_InvalidSlot(t *testing.T) {
	c := &mock.ChainService{Genesis: time.Now()}
	aggregatorServer := &Server{
		CoreService: &core.Service{
			GenesisTimeFetcher: c,
		},
	}
	req := &zondpb.SignedAggregateSubmitRequest{
		SignedAggregateAndProof: &zondpb.SignedAggregateAttestationAndProof{
			Signature: []byte{'a'},
			Message: &zondpb.AggregateAttestationAndProof{
				SelectionProof: []byte{'a'},
				Aggregate: &zondpb.Attestation{
					Data: &zondpb.AttestationData{Slot: 1000},
				},
			},
		},
	}
	_, err := aggregatorServer.SubmitSignedAggregateSelectionProof(context.Background(), req)
	require.ErrorContains(t, "attestation slot is no longer valid from current time", err)
}
