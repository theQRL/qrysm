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
	validators := uint64(2048)
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, validators)

	var sig []byte
	for i := byte(0); ; i++ {
		candidate := privKeys[0].Sign([]byte{i}).Marshal()
		committee, err := helpers.BeaconCommitteeFromState(ctx, beaconState, 0, 0)
		require.NoError(t, err)
		agg, err := helpers.IsAggregator(uint64(len(committee)), candidate)
		require.NoError(t, err)
		if !agg {
			sig = candidate
			break
		}
	}
	data := util.HydrateAttestationData(&qrysmpb.AttestationData{})

	_, err := validateSelectionIndex(ctx, beaconState, data, 0, sig)
	wanted := "validator is not an aggregator for slot"
	assert.ErrorContains(t, wanted, err)
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

	require.NoError(t, r.cfg.attPool.SaveBlockAttestation(att))
	if res, err := r.validateAggregateAndProof(context.Background(), "", msg); res == pubsub.ValidationAccept {
		_ = err
		t.Error("Expected validate to fail")
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
		sig := privKeys[indice].Sign(hashTreeRoot[:]).Marshal()
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
		sig := privKeys[indice].Sign(hashTreeRoot[:]).Marshal()
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
		sig := privKeys[indice].Sign(hashTreeRoot[:]).Marshal()
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
		sig := privKeys[indice].Sign(hashTreeRoot[:]).Marshal()
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
