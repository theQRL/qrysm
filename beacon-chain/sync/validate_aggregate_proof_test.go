package sync

import (
	"bytes"
	"context"
	"reflect"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	"github.com/theQRL/go-bitfield"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	dbtest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/beacon-chain/p2p"
	p2ptest "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestVerifyIndexInCommittee_CanVerify(t *testing.T) {
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())

	service := &Service{}
	validators := uint64(32)
	s, _ := util.DeterministicGenesisStateZond(t, validators)
	require.NoError(t, s.SetSlot(params.BeaconConfig().SlotsPerEpoch))

	bf := bitfield.NewBitlist(validators / uint64(params.BeaconConfig().SlotsPerEpoch))
	bf.SetBitAt(0, true)
	att := &qrysmpb.Attestation{Data: &qrysmpb.AttestationData{
		Target: &qrysmpb.Checkpoint{Epoch: 0}},
		AggregationBits: bf}

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), s, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	indices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	require.NoError(t, err)
	result, err := service.validateIndexInCommittee(ctx, s, att, primitives.ValidatorIndex(indices[0]))
	require.NoError(t, err)
	assert.Equal(t, pubsub.ValidationAccept, result)

	wanted := "validator index 1000 is not within the committee"
	result, err = service.validateIndexInCommittee(ctx, s, att, 1000)
	assert.ErrorContains(t, wanted, err)
	assert.Equal(t, pubsub.ValidationReject, result)
}

func TestVerifyIndexInCommittee_ExistsInBeaconCommittee(t *testing.T) {
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())

	service := &Service{}
	validators := uint64(64)
	s, _ := util.DeterministicGenesisStateZond(t, validators)
	require.NoError(t, s.SetSlot(params.BeaconConfig().SlotsPerEpoch))

	att := &qrysmpb.Attestation{Data: &qrysmpb.AttestationData{
		Target: &qrysmpb.Checkpoint{Epoch: 0}}}

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), s, att.Data.Slot, att.Data.CommitteeIndex)
	require.NoError(t, err)

	// Empty bitfield → no attesting indices → Reject.
	bl := bitfield.NewBitlist(uint64(len(committee)))
	att.AggregationBits = bl
	result, err := service.validateIndexInCommittee(ctx, s, att, committee[0])
	require.ErrorContains(t, "no attesting indices", err)
	assert.Equal(t, pubsub.ValidationReject, result)

	// Non-empty bitfield with valid validator index → Accept.
	att.AggregationBits.SetBitAt(0, true)
	result, err = service.validateIndexInCommittee(ctx, s, att, committee[0])
	require.NoError(t, err)
	assert.Equal(t, pubsub.ValidationAccept, result)

	// Validator not in committee → Reject.
	wanted := "validator index 1000 is not within the committee"
	result, err = service.validateIndexInCommittee(ctx, s, att, 1000)
	assert.ErrorContains(t, wanted, err)
	assert.Equal(t, pubsub.ValidationReject, result)

	// Bitfield length mismatch → Reject.
	att.AggregationBits = bitfield.NewBitlist(1)
	result, err = service.validateIndexInCommittee(ctx, s, att, committee[0])
	require.ErrorContains(t, "wanted participants bitfield length", err)
	assert.Equal(t, pubsub.ValidationReject, result)

	// Committee index out of range → Reject.
	att.Data.CommitteeIndex = 10000
	result, err = service.validateIndexInCommittee(ctx, s, att, committee[0])
	require.ErrorContains(t, "committee index 10000", err)
	assert.Equal(t, pubsub.ValidationReject, result)
}

func TestVerifySelection_NotAnAggregator(t *testing.T) {
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 2048)
	committee, err := helpers.BeaconCommitteeFromState(ctx, beaconState, 0, 0)
	require.NoError(t, err)
	seed, err := helpers.AggregatorSelectionSeed(beaconState, 0)
	require.NoError(t, err)
	domain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainSelectionProof, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	slot := primitives.SSZUint64(0)
	root, err := signing.ComputeSigningRoot(&slot, domain)
	require.NoError(t, err)
	data := util.HydrateAttestationData(&qrysmpb.AttestationData{})
	for _, index := range committee {
		selected, err := helpers.IsAggregator(uint64(len(committee)), seed[:], 0, 0, index)
		require.NoError(t, err)
		if selected {
			continue
		}
		// Retrying randomized ML-DSA signatures cannot turn this member into an aggregator.
		for range 32 {
			sig, err := privKeys[index].Sign(root[:])
			require.NoError(t, err)
			require.Equal(t, true, sig.Verify(privKeys[index].PublicKey(), root[:]))
			_, err = validateSelectionIndex(ctx, beaconState, data, index, sig.Marshal())
			require.ErrorContains(t, "validator is not an aggregator for slot", err)
		}
		return
	}
	t.Fatal("expected an unselected committee member")
}

func TestVerifySelection_SelectedValidatorStillNeedsValidProof(t *testing.T) {
	ctx := context.Background()
	params.SetupTestConfigCleanup(t)
	params.OverrideBeaconConfig(params.MinimalSpecConfig())
	st, keys := util.DeterministicGenesisStateZond(t, 256)
	committee, err := helpers.BeaconCommitteeFromState(ctx, st, 0, 0)
	require.NoError(t, err)
	index := committee[0] // This small committee selects every member.
	domain, err := signing.Domain(st.Fork(), 0, params.BeaconConfig().DomainSelectionProof, st.GenesisValidatorsRoot())
	require.NoError(t, err)
	slot := primitives.SSZUint64(0)
	root, err := signing.ComputeSigningRoot(&slot, domain)
	require.NoError(t, err)
	data := util.HydrateAttestationData(&qrysmpb.AttestationData{})
	var previous []byte
	for range 2 {
		proof, err := keys[index].Sign(root[:])
		require.NoError(t, err)
		require.DeepNotEqual(t, previous, proof.Marshal())
		previous = proof.Marshal()
		batch, err := validateSelectionIndex(ctx, st, data, index, proof.Marshal())
		require.NoError(t, err)
		valid, err := batch.Verify()
		require.NoError(t, err)
		require.Equal(t, true, valid)
	}
	// Eligibility is unchanged, but a proof for a different slot is invalid.
	slot = 1
	wrongRoot, err := signing.ComputeSigningRoot(&slot, domain)
	require.NoError(t, err)
	proof, err := keys[index].Sign(wrongRoot[:])
	require.NoError(t, err)
	batch, err := validateSelectionIndex(ctx, st, data, index, proof.Marshal())
	require.NoError(t, err)
	valid, err := batch.Verify()
	require.NoError(t, err)
	require.Equal(t, false, valid)
}

func TestValidateAggregateAndProof_NoBlock(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	att := util.HydrateAttestation(&qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Source: &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target: &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("hello-world"), 32)},
		},
	})

	aggregateAndProof := &qrysmpb.AggregateAttestationAndProof{
		SelectionProof:  bytesutil.PadTo([]byte{'A'}, field_params.MLDSA87SignatureLength),
		Aggregate:       att,
		AggregatorIndex: 0,
	}
	signedAggregateAndProof := &qrysmpb.SignedAggregateAttestationAndProof{Message: aggregateAndProof, Signature: make([]byte, field_params.MLDSA87SignatureLength)}

	r := &Service{
		cfg: &config{
			p2p:         p,
			beaconDB:    db,
			initialSync: &mockSync.Sync{IsSyncing: false},
			attPool:     attestations.NewPool(),
			chain:       &mock.ChainService{},
		},
		blkRootToPendingAtts: make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
	}
	r.initCaches()

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, signedAggregateAndProof)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedAggregateAttestationAndProof]()]
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	if res, err := r.validateAggregateAndProof(context.Background(), "", msg); res == pubsub.ValidationAccept {
		_ = err
		t.Error("Expected validate to fail")
	}
}

func TestValidateAggregateAndProof_NotWithinSlotRange(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	validators := uint64(256)
	beaconState, _ := util.DeterministicGenesisStateZond(t, validators)

	b := util.NewBeaconBlockZond()
	util.SaveBlock(t, context.Background(), db, b)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(context.Background(), s, root))

	aggBits := bitfield.NewBitlist(3)
	aggBits.SetBitAt(0, true)
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Slot:            1,
			BeaconBlockRoot: root[:],
			Source:          &qrysmpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &qrysmpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
		},
		AggregationBits: aggBits,
		Signatures:      [][]byte{},
	}

	aggregateAndProof := &qrysmpb.AggregateAttestationAndProof{
		Aggregate:      att,
		SelectionProof: make([]byte, field_params.MLDSA87SignatureLength),
	}
	signedAggregateAndProof := &qrysmpb.SignedAggregateAttestationAndProof{Message: aggregateAndProof, Signature: make([]byte, field_params.MLDSA87SignatureLength)}

	require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix())))

	r := &Service{
		cfg: &config{
			p2p:         p,
			beaconDB:    db,
			initialSync: &mockSync.Sync{IsSyncing: false},
			chain: &mock.ChainService{
				Genesis: time.Now(),
				State:   beaconState,
			},
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
	}
	r.initCaches()

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, signedAggregateAndProof)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedAggregateAttestationAndProof]()]
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	if res, err := r.validateAggregateAndProof(context.Background(), "", msg); res == pubsub.ValidationAccept {
		_ = err
		t.Error("Expected validate to fail")
	}

	att.Data.Slot = 1<<32 - 1

	buf = new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, signedAggregateAndProof)
	require.NoError(t, err)

	msg = &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	if res, err := r.validateAggregateAndProof(context.Background(), "", msg); res == pubsub.ValidationAccept {
		_ = err
		t.Error("Expected validate to fail")
	}
}

func TestValidateAggregateAndProof_ExistedInPool(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	validators := uint64(256)
	beaconState, _ := util.DeterministicGenesisStateZond(t, validators)

	b := util.NewBeaconBlockZond()
	util.SaveBlock(t, context.Background(), db, b)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)

	aggBits := bitfield.NewBitlist(3)
	aggBits.SetBitAt(0, true)
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Slot:            1,
			BeaconBlockRoot: root[:],
			Source:          &qrysmpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &qrysmpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
		},
		AggregationBits: aggBits,
		Signatures:      [][]byte{},
	}

	aggregateAndProof := &qrysmpb.AggregateAttestationAndProof{
		Aggregate:      att,
		SelectionProof: make([]byte, field_params.MLDSA87SignatureLength),
	}
	signedAggregateAndProof := &qrysmpb.SignedAggregateAttestationAndProof{Message: aggregateAndProof, Signature: make([]byte, field_params.MLDSA87SignatureLength)}

	require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix())))
	r := &Service{
		cfg: &config{
			attPool:     attestations.NewPool(),
			p2p:         p,
			beaconDB:    db,
			initialSync: &mockSync.Sync{IsSyncing: false},
			chain: &mock.ChainService{Genesis: time.Now(),
				State: beaconState},
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
		blkRootToPendingAtts: make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
	}
	r.initCaches()

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, signedAggregateAndProof)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedAggregateAttestationAndProof]()]
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	// An included vote counts as seen once it has been applied, not while it
	// is still pending verification against its target state.
	require.NoError(t, r.cfg.attPool.SaveBlockAttestation(att))
	require.NoError(t, r.cfg.attPool.DeleteBlockAttestation(att))
	seen, err := r.cfg.attPool.HasAggregatedAttestation(att)
	require.NoError(t, err)
	require.Equal(t, true, seen)
	if res, err := r.validateAggregateAndProof(context.Background(), "", msg); res == pubsub.ValidationAccept {
		_ = err
		t.Error("Expected validate to fail")
	}
}

func TestValidateAggregatedAtt_VerifiesAggregatorBeforeAttesters(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	cfg := params.MinimalSpecConfig().Copy()
	cfg.TargetCommitteeSize = 32
	cfg.MaxCommitteesPerSlot = 1
	params.OverrideBeaconConfig(cfg)
	helpers.ClearCache()
	t.Cleanup(helpers.ClearCache)

	beaconState, keys := util.DeterministicGenesisStateZond(t, 256)
	att := util.HydrateAttestation(&qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{Slot: 1},
	})
	committee, err := helpers.BeaconCommitteeFromState(t.Context(), beaconState, att.Data.Slot, 0)
	require.NoError(t, err)
	require.Equal(t, 32, len(committee))
	att.AggregationBits = bitfield.NewBitlist(uint64(len(committee)))
	att.Signatures = make([][]byte, len(committee))
	for i, index := range committee {
		att.AggregationBits.SetBitAt(uint64(i), true)
		att.Signatures[i], err = signing.ComputeDomainAndSign(beaconState, 0, att.Data, cfg.DomainBeaconAttester, keys[index])
		require.NoError(t, err)
	}

	seed, err := helpers.AggregatorSelectionSeed(beaconState, 0)
	require.NoError(t, err)
	var aggregatorIndex primitives.ValidatorIndex
	found := false
	for _, index := range committee {
		selected, err := helpers.IsAggregator(uint64(len(committee)), seed[:], att.Data.Slot, 0, index)
		require.NoError(t, err)
		if selected {
			aggregatorIndex, found = index, true
			break
		}
	}
	require.Equal(t, true, found)
	slot := primitives.SSZUint64(att.Data.Slot)
	selectionProof, err := signing.ComputeDomainAndSign(beaconState, 0, &slot, cfg.DomainSelectionProof, keys[aggregatorIndex])
	require.NoError(t, err)

	for _, tt := range []struct {
		name                string
		invalidSelection    bool
		invalidAggregator   bool
		invalidAttester     int
		wantResult          pubsub.ValidationResult
		wantAttesterLookups int
	}{
		{name: "valid", invalidAttester: -1, wantResult: pubsub.ValidationAccept, wantAttesterLookups: 32},
		{name: "invalid selection proof", invalidSelection: true, invalidAttester: -1, wantResult: pubsub.ValidationReject},
		{name: "invalid aggregator signature", invalidAggregator: true, invalidAttester: -1, wantResult: pubsub.ValidationReject},
		{name: "both aggregator proofs invalid", invalidSelection: true, invalidAggregator: true, invalidAttester: -1, wantResult: pubsub.ValidationReject},
		{name: "invalid first attester signature", invalidAttester: 0, wantResult: pubsub.ValidationReject, wantAttesterLookups: 32},
		{name: "invalid last attester signature", invalidAttester: 31, wantResult: pubsub.ValidationReject, wantAttesterLookups: 32},
	} {
		t.Run(tt.name, func(t *testing.T) {
			signed := &qrysmpb.SignedAggregateAttestationAndProof{
				Message: &qrysmpb.AggregateAttestationAndProof{
					AggregatorIndex: aggregatorIndex,
					Aggregate:       qrysmpb.CopyAttestation(att),
					SelectionProof:  bytes.Clone(selectionProof),
				},
			}
			if tt.invalidSelection {
				signed.Message.SelectionProof[0] ^= 1
			}
			if tt.invalidAttester >= 0 {
				signed.Message.Aggregate.Signatures[tt.invalidAttester][0] ^= 1
			}
			// Sign after changing the payload so each test isolates the invalid proof.
			signed.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, signed.Message, cfg.DomainAggregateAndProof, keys[aggregatorIndex])
			require.NoError(t, err)
			if tt.invalidAggregator {
				signed.Signature[0] ^= 1
			}

			trackedState := &pubkeyTrackingState{BeaconState: beaconState}
			s := &Service{
				ctx:           t.Context(),
				cfg:           &config{chain: &mock.ChainService{State: trackedState}},
				signatureChan: make(chan *signatureVerifier, verifierLimit),
			}
			go s.verifierRoutine()
			result, err := s.validateAggregatedAtt(t.Context(), signed)
			assert.Equal(t, tt.wantResult, result)
			if tt.wantResult == pubsub.ValidationAccept {
				assert.NoError(t, err)
			} else {
				assert.NotNil(t, err)
			}
			assert.Equal(t, tt.wantAttesterLookups, trackedState.pubkeyLookups, "invalid aggregator proofs must not trigger attester signature work")
		})
	}
}

func TestValidateAggregateAndProof_CanValidate(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	validators := uint64(256)
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, validators)

	b := util.NewBeaconBlockZond()
	util.SaveBlock(t, context.Background(), db, b)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(context.Background(), s, root))

	aggBits := bitfield.NewBitlist(validators / uint64(params.BeaconConfig().SlotsPerEpoch))
	aggBits.SetBitAt(0, true)
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			// TODO(now.youtrack.cloud/issue/TQ-12)
			// Slot:            1,
			Slot:            96,
			BeaconBlockRoot: root[:],
			Source:          &qrysmpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &qrysmpb.Checkpoint{Epoch: 0, Root: root[:]},
		},
		AggregationBits: aggBits,
	}

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	require.NoError(t, err)
	assert.NoError(t, err)
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	assert.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	sigs := make([][]byte, len(attestingIndices))
	for i, indice := range attestingIndices {
		lsig2, err := privKeys[indice].Sign(hashTreeRoot[:])
		require.NoError(t, err)
		sig := lsig2.Marshal()
		sigs[i] = sig
	}
	att.Signatures = sigs
	ai := committee[0]
	sszUint := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, &sszUint, params.BeaconConfig().DomainSelectionProof, privKeys[ai])
	require.NoError(t, err)
	aggregateAndProof := &qrysmpb.AggregateAttestationAndProof{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: ai,
	}
	signedAggregateAndProof := &qrysmpb.SignedAggregateAttestationAndProof{Message: aggregateAndProof}
	signedAggregateAndProof.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, signedAggregateAndProof.Message, params.BeaconConfig().DomainAggregateAndProof, privKeys[ai])
	require.NoError(t, err)

	require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix())))
	ctx := t.Context()
	chain := &mock.ChainService{Genesis: time.Now().Add(-oneEpoch()),
		Optimistic:       true,
		DB:               db,
		State:            beaconState,
		ValidAttestation: true,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  att.Data.BeaconBlockRoot,
		}}
	r := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:                 p,
			beaconDB:            db,
			initialSync:         &mockSync.Sync{IsSyncing: false},
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
		signatureChan: make(chan *signatureVerifier, verifierLimit),
	}
	r.initCaches()
	go r.verifierRoutine()

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, signedAggregateAndProof)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedAggregateAttestationAndProof]()]
	d, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, d)
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAggregateAndProof(context.Background(), "", msg)
	assert.NoError(t, err)
	assert.Equal(t, pubsub.ValidationAccept, res, "Validated status is false")
	assert.NotNil(t, msg.ValidatorData, "Did not set validator data")
}

func TestVerifyIndexInCommittee_SeenAggregatorEpoch(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	validators := uint64(256)
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, validators)

	b := util.NewBeaconBlockZond()
	util.SaveBlock(t, context.Background(), db, b)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(context.Background(), s, root))

	aggBits := bitfield.NewBitlist(validators / uint64(params.BeaconConfig().SlotsPerEpoch))
	aggBits.SetBitAt(0, true)
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			// Slot:            1,
			Slot:            96,
			BeaconBlockRoot: root[:],
			Source:          &qrysmpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &qrysmpb.Checkpoint{Epoch: 0, Root: root[:]},
		},
		AggregationBits: aggBits,
	}

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	require.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	require.NoError(t, err)
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	sigs := make([][]byte, len(attestingIndices))
	for i, indice := range attestingIndices {
		lsig3, err := privKeys[indice].Sign(hashTreeRoot[:])
		require.NoError(t, err)
		sig := lsig3.Marshal()
		sigs[i] = sig
	}
	att.Signatures = sigs
	ai := committee[0]
	sszUint := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, &sszUint, params.BeaconConfig().DomainSelectionProof, privKeys[ai])
	require.NoError(t, err)
	aggregateAndProof := &qrysmpb.AggregateAttestationAndProof{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: ai,
	}
	signedAggregateAndProof := &qrysmpb.SignedAggregateAttestationAndProof{Message: aggregateAndProof}
	signedAggregateAndProof.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, signedAggregateAndProof.Message, params.BeaconConfig().DomainAggregateAndProof, privKeys[ai])
	require.NoError(t, err)
	require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix())))

	ctx := t.Context()
	chain := &mock.ChainService{Genesis: time.Now().Add(-oneEpoch()),
		DB:               db,
		ValidatorsRoot:   [32]byte{'A'},
		State:            beaconState,
		ValidAttestation: true,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  signedAggregateAndProof.Message.Aggregate.Data.BeaconBlockRoot,
		}}
	r := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:                 p,
			beaconDB:            db,
			initialSync:         &mockSync.Sync{IsSyncing: false},
			chain:               chain,
			clock:               startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
		signatureChan: make(chan *signatureVerifier, verifierLimit),
	}
	r.initCaches()
	go r.verifierRoutine()

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, signedAggregateAndProof)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedAggregateAttestationAndProof]()]
	d, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, d)
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAggregateAndProof(context.Background(), "", msg)
	assert.NoError(t, err)
	require.Equal(t, pubsub.ValidationAccept, res, "Validated status is false")

	// Should fail with another attestation in the same epoch.
	signedAggregateAndProof.Message.Aggregate.Data.Slot++
	buf = new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, signedAggregateAndProof)
	require.NoError(t, err)
	msg = &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	time.Sleep(10 * time.Millisecond) // Wait for cached value to pass through buffers.
	if res, err := r.validateAggregateAndProof(context.Background(), "", msg); res == pubsub.ValidationAccept {
		_ = err
		t.Fatal("Validated status is true")
	}
}

func TestValidateAggregateAndProof_BadBlock(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	validators := uint64(256)
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, validators)

	b := util.NewBeaconBlockZond()
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(context.Background(), s, root))

	aggBits := bitfield.NewBitlist(validators / uint64(params.BeaconConfig().SlotsPerEpoch))
	aggBits.SetBitAt(0, true)
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			BeaconBlockRoot: root[:],
			Source:          &qrysmpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &qrysmpb.Checkpoint{Epoch: 0, Root: root[:]},
		},
		AggregationBits: aggBits,
	}

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	require.NoError(t, err)
	assert.NoError(t, err)
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	assert.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	sigs := make([][]byte, len(attestingIndices))
	for i, indice := range attestingIndices {
		lsig4, err := privKeys[indice].Sign(hashTreeRoot[:])
		require.NoError(t, err)
		sig := lsig4.Marshal()
		sigs[i] = sig
	}
	att.Signatures = sigs
	ai := committee[0]
	sszUint := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, &sszUint, params.BeaconConfig().DomainSelectionProof, privKeys[ai])
	require.NoError(t, err)

	aggregateAndProof := &qrysmpb.AggregateAttestationAndProof{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: ai,
	}
	signedAggregateAndProof := &qrysmpb.SignedAggregateAttestationAndProof{Message: aggregateAndProof}
	signedAggregateAndProof.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, signedAggregateAndProof.Message, params.BeaconConfig().DomainAggregateAndProof, privKeys[ai])
	require.NoError(t, err)

	require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix())))
	r := &Service{
		cfg: &config{
			p2p:         p,
			beaconDB:    db,
			initialSync: &mockSync.Sync{IsSyncing: false},
			chain: &mock.ChainService{Genesis: time.Now(),
				State:            beaconState,
				ValidAttestation: true,
				FinalizedCheckPoint: &qrysmpb.Checkpoint{
					Epoch: 0,
				}},
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
	}
	r.initCaches()
	// Set beacon block as bad.
	r.setBadBlock(context.Background(), root)
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, signedAggregateAndProof)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedAggregateAttestationAndProof]()]
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAggregateAndProof(context.Background(), "", msg)
	assert.NotNil(t, err)
	assert.Equal(t, pubsub.ValidationReject, res, "Validated status is true")
}

func TestValidateAggregateAndProof_RejectWhenAttEpochDoesntEqualTargetEpoch(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	validators := uint64(256)
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, validators)

	b := util.NewBeaconBlockZond()
	util.SaveBlock(t, context.Background(), db, b)
	root, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(context.Background(), s, root))

	aggBits := bitfield.NewBitlist(validators / uint64(params.BeaconConfig().SlotsPerEpoch))
	aggBits.SetBitAt(0, true)
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			BeaconBlockRoot: root[:],
			Source:          &qrysmpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &qrysmpb.Checkpoint{Epoch: 1, Root: root[:]},
		},
		AggregationBits: aggBits,
	}

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	require.NoError(t, err)
	assert.NoError(t, err)
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	assert.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	sigs := make([][]byte, len(attestingIndices))
	for i, indice := range attestingIndices {
		lsig5, err := privKeys[indice].Sign(hashTreeRoot[:])
		require.NoError(t, err)
		sig := lsig5.Marshal()
		sigs[i] = sig
	}
	att.Signatures = sigs
	ai := committee[0]
	sszUint := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, &sszUint, params.BeaconConfig().DomainSelectionProof, privKeys[ai])
	require.NoError(t, err)
	aggregateAndProof := &qrysmpb.AggregateAttestationAndProof{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: ai,
	}
	signedAggregateAndProof := &qrysmpb.SignedAggregateAttestationAndProof{Message: aggregateAndProof}
	signedAggregateAndProof.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, signedAggregateAndProof.Message, params.BeaconConfig().DomainAggregateAndProof, privKeys[ai])
	require.NoError(t, err)

	require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix())))
	r := &Service{
		cfg: &config{
			p2p:         p,
			beaconDB:    db,
			initialSync: &mockSync.Sync{IsSyncing: false},
			chain: &mock.ChainService{Genesis: time.Now(),
				State:            beaconState,
				ValidAttestation: true,
				FinalizedCheckPoint: &qrysmpb.Checkpoint{
					Epoch: 0,
					Root:  att.Data.BeaconBlockRoot,
				}},
			attPool:             attestations.NewPool(),
			attestationNotifier: (&mock.ChainService{}).OperationNotifier(),
		},
	}
	r.initCaches()

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, signedAggregateAndProof)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedAggregateAttestationAndProof]()]
	msg := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateAggregateAndProof(context.Background(), "", msg)
	assert.NotNil(t, err)
	assert.Equal(t, pubsub.ValidationReject, res)
}

func Test_SetAggregatorIndexEpochSeen_RetainsAcrossEpoch(t *testing.T) {
	r := &Service{}
	r.initCaches()

	const epoch = primitives.Epoch(5)
	const aggIndex = primitives.ValidatorIndex(42)

	first := r.setAggregatorIndexEpochSeen(epoch, aggIndex)
	require.Equal(t, true, first)
	second := r.setAggregatorIndexEpochSeen(epoch, aggIndex)
	require.Equal(t, false, second)

	// Regression guard: under sustained load the prior LRU-backed cache
	// could evict (epoch, aggIndex) when many other aggregators reported
	// in the same epoch, allowing duplicate aggregates to pass dedup.
	for i := 0; i < 4096; i++ {
		idx := primitives.ValidatorIndex(1000 + i)
		require.Equal(t, true, r.setAggregatorIndexEpochSeen(epoch, idx))
	}
	require.Equal(t, true, r.hasSeenAggregatorIndexEpoch(epoch, aggIndex))
}

func Test_SetAggregatorIndexEpochSeen_PrunesOldEpochs(t *testing.T) {
	r := &Service{}
	r.initCaches()

	require.Equal(t, true, r.setAggregatorIndexEpochSeen(primitives.Epoch(10), primitives.ValidatorIndex(1)))
	require.Equal(t, true, r.setAggregatorIndexEpochSeen(primitives.Epoch(11), primitives.ValidatorIndex(1)))
	require.Equal(t, true, r.setAggregatorIndexEpochSeen(primitives.Epoch(12), primitives.ValidatorIndex(1)))

	// Latest two epochs (11 and 12) must still be tracked; older ones are pruned.
	require.Equal(t, false, r.hasSeenAggregatorIndexEpoch(primitives.Epoch(10), primitives.ValidatorIndex(1)))
	require.Equal(t, true, r.hasSeenAggregatorIndexEpoch(primitives.Epoch(11), primitives.ValidatorIndex(1)))
	require.Equal(t, true, r.hasSeenAggregatorIndexEpoch(primitives.Epoch(12), primitives.ValidatorIndex(1)))

	// Late-arriving aggregates for an already-pruned epoch should be reinsertable
	// (we treat them as first-seen because we lost their history).
	require.Equal(t, true, r.setAggregatorIndexEpochSeen(primitives.Epoch(10), primitives.ValidatorIndex(1)))
}
