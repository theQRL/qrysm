package sync

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	dilithium2 "github.com/theQRL/go-qrllib/dilithium"
	"github.com/theQRL/go-zond/p2p/enr"
	"github.com/theQRL/qrysm/v4/async/abool"
	mock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/signing"
	dbtest "github.com/theQRL/qrysm/v4/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/v4/beacon-chain/p2p/peers"
	p2ptest "github.com/theQRL/qrysm/v4/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/startup"
	lruwrpr "github.com/theQRL/qrysm/v4/cache/lru"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/crypto/bls"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	prysmTime "github.com/theQRL/qrysm/v4/time"
)

func TestProcessPendingAtts_NoBlockRequestBlock(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	assert.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")
	p1.Peers().Add(new(enr.Record), p2.PeerID(), nil, network.DirOutbound)
	p1.Peers().SetConnectionState(p2.PeerID(), peers.PeerConnected)
	p1.Peers().SetChainState(p2.PeerID(), &zondpb.Status{})

	chain := &mock.ChainService{Genesis: prysmTime.Now(), FinalizedCheckPoint: &zondpb.Checkpoint{}}
	r := &Service{
		cfg:                  &config{p2p: p1, beaconDB: db, chain: chain, clock: startup.NewClock(chain.Genesis, chain.ValidatorsRoot)},
		blkRootToPendingAtts: make(map[[32]byte][]*zondpb.SignedAggregateAttestationAndProof),
		chainStarted:         abool.New(),
	}

	a := &zondpb.AggregateAttestationAndProof{Aggregate: &zondpb.Attestation{Data: &zondpb.AttestationData{Target: &zondpb.Checkpoint{Root: make([]byte, 32)}}}}
	r.blkRootToPendingAtts[[32]byte{'A'}] = []*zondpb.SignedAggregateAttestationAndProof{{Message: a}}
	require.NoError(t, r.processPendingAtts(context.Background()))
	require.LogsContain(t, hook, "Requesting block for pending attestation")
}

func TestProcessPendingAtts_HasBlockSaveUnAggregatedAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	validators := uint64(256)

	beaconState, privKeys := util.DeterministicGenesisState(t, validators)

	sb := util.NewBeaconBlock()
	util.SaveBlock(t, context.Background(), db, sb)
	root, err := sb.Block.HashTreeRoot()
	require.NoError(t, err)

	aggBits := bitfield.NewBitlist(8)
	aggBits.SetBitAt(1, true)
	att := &zondpb.Attestation{
		Data: &zondpb.AttestationData{
			BeaconBlockRoot: root[:],
			Source:          &zondpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &zondpb.Checkpoint{Epoch: 0, Root: root[:]},
		},
		AggregationBits: aggBits,
	}

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	require.NoError(t, err)
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	for _, i := range attestingIndices {
		att.Signature = privKeys[i].Sign(hashTreeRoot[:]).Marshal()
	}

	// Arbitrary aggregator index for testing purposes.
	aggregatorIndex := committee[0]
	sszUint := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, &sszUint, params.BeaconConfig().DomainSelectionProof, privKeys[aggregatorIndex])
	require.NoError(t, err)
	aggregateAndProof := &zondpb.AggregateAttestationAndProof{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: aggregatorIndex,
	}
	aggreSig, err := signing.ComputeDomainAndSign(beaconState, 0, aggregateAndProof, params.BeaconConfig().DomainAggregateAndProof, privKeys[aggregatorIndex])
	require.NoError(t, err)

	require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix())))

	chain := &mock.ChainService{Genesis: time.Now(),
		State: beaconState,
		FinalizedCheckPoint: &zondpb.Checkpoint{
			Root:  aggregateAndProof.Aggregate.Data.BeaconBlockRoot,
			Epoch: 0,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:      p1,
			beaconDB: db,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:  attestations.NewPool(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]*zondpb.SignedAggregateAttestationAndProof),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	go r.verifierRoutine()

	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, r.cfg.beaconDB.SaveState(context.Background(), s, root))

	r.blkRootToPendingAtts[root] = []*zondpb.SignedAggregateAttestationAndProof{{Message: aggregateAndProof, Signature: aggreSig}}
	require.NoError(t, r.processPendingAtts(context.Background()))

	atts, err := r.cfg.attPool.UnaggregatedAttestations()
	require.NoError(t, err)
	assert.Equal(t, 1, len(atts), "Did not save unaggregated att")
	assert.DeepEqual(t, att, atts[0], "Incorrect saved att")
	assert.Equal(t, 0, len(r.cfg.attPool.AggregatedAttestations()), "Did save aggregated att")
	require.LogsContain(t, hook, "Verified and saved pending attestations to pool")
	cancel()
}

func TestProcessPendingAtts_NoBroadcastWithBadSignature(t *testing.T) {
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)

	s, _ := util.DeterministicGenesisState(t, 256)
	chain := &mock.ChainService{
		State:   s,
		Genesis: prysmTime.Now(), FinalizedCheckPoint: &zondpb.Checkpoint{Root: make([]byte, 32)}}
	r := &Service{
		cfg: &config{
			p2p:      p1,
			beaconDB: db,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:  attestations.NewPool(),
		},
		blkRootToPendingAtts: make(map[[32]byte][]*zondpb.SignedAggregateAttestationAndProof),
	}

	priv, err := bls.RandKey()
	require.NoError(t, err)
	a := &zondpb.AggregateAttestationAndProof{
		Aggregate: &zondpb.Attestation{
			Signature:       priv.Sign([]byte("foo")).Marshal(),
			AggregationBits: bitfield.Bitlist{0x02},
			Data:            util.HydrateAttestationData(&zondpb.AttestationData{}),
		},
		SelectionProof: make([]byte, dilithium2.CryptoBytes),
	}

	b := util.NewBeaconBlock()
	r32, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), r.cfg.beaconDB, b)
	require.NoError(t, r.cfg.beaconDB.SaveState(context.Background(), s, r32))

	r.blkRootToPendingAtts[r32] = []*zondpb.SignedAggregateAttestationAndProof{{Message: a, Signature: make([]byte, dilithium2.CryptoBytes)}}
	require.NoError(t, r.processPendingAtts(context.Background()))

	assert.Equal(t, false, p1.BroadcastCalled, "Broadcasted bad aggregate")
	// Clear pool.
	err = r.cfg.attPool.DeleteUnaggregatedAttestation(a.Aggregate)
	require.NoError(t, err)

	validators := uint64(256)

	_, privKeys := util.DeterministicGenesisState(t, validators)
	aggBits := bitfield.NewBitlist(8)
	aggBits.SetBitAt(1, true)
	att := &zondpb.Attestation{
		Data: &zondpb.AttestationData{
			BeaconBlockRoot: r32[:],
			Source:          &zondpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &zondpb.Checkpoint{Epoch: 0, Root: r32[:]},
		},
		AggregationBits: aggBits,
	}
	committee, err := helpers.BeaconCommitteeFromState(context.Background(), s, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	require.NoError(t, err)
	attesterDomain, err := signing.Domain(s.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, s.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	for _, i := range attestingIndices {
		att.Signature = privKeys[i].Sign(hashTreeRoot[:]).Marshal()
	}

	// Arbitrary aggregator index for testing purposes.
	aggregatorIndex := committee[0]
	sszSlot := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(s, 0, &sszSlot, params.BeaconConfig().DomainSelectionProof, privKeys[aggregatorIndex])
	require.NoError(t, err)
	aggregateAndProof := &zondpb.AggregateAttestationAndProof{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: aggregatorIndex,
	}
	aggreSig, err := signing.ComputeDomainAndSign(s, 0, aggregateAndProof, params.BeaconConfig().DomainAggregateAndProof, privKeys[aggregatorIndex])
	require.NoError(t, err)

	require.NoError(t, s.SetGenesisTime(uint64(time.Now().Unix())))
	ctx, cancel := context.WithCancel(context.Background())
	chain2 := &mock.ChainService{Genesis: time.Now(),
		State: s,
		FinalizedCheckPoint: &zondpb.Checkpoint{
			Root:  aggregateAndProof.Aggregate.Data.BeaconBlockRoot,
			Epoch: 0,
		}}
	r = &Service{
		ctx: ctx,
		cfg: &config{
			p2p:      p1,
			beaconDB: db,
			chain:    chain2,
			clock:    startup.NewClock(chain2.Genesis, chain2.ValidatorsRoot),
			attPool:  attestations.NewPool(),
		},
		blkRootToPendingAtts:             make(map[[32]byte][]*zondpb.SignedAggregateAttestationAndProof),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	go r.verifierRoutine()

	r.blkRootToPendingAtts[r32] = []*zondpb.SignedAggregateAttestationAndProof{{Message: aggregateAndProof, Signature: aggreSig}}
	require.NoError(t, r.processPendingAtts(context.Background()))

	assert.Equal(t, true, p1.BroadcastCalled, "Could not broadcast the good aggregate")
	cancel()
}

func TestProcessPendingAtts_HasBlockSaveAggregatedAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	validators := uint64(256)

	beaconState, privKeys := util.DeterministicGenesisState(t, validators)

	sb := util.NewBeaconBlock()
	util.SaveBlock(t, context.Background(), db, sb)
	root, err := sb.Block.HashTreeRoot()
	require.NoError(t, err)

	aggBits := bitfield.NewBitlist(validators / uint64(params.BeaconConfig().SlotsPerEpoch))
	aggBits.SetBitAt(0, true)
	aggBits.SetBitAt(1, true)
	att := &zondpb.Attestation{
		Data: &zondpb.AttestationData{
			BeaconBlockRoot: root[:],
			Source:          &zondpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &zondpb.Checkpoint{Epoch: 0, Root: root[:]},
		},
		AggregationBits: aggBits,
	}

	committee, err := helpers.BeaconCommitteeFromState(context.Background(), beaconState, att.Data.Slot, att.Data.CommitteeIndex)
	assert.NoError(t, err)
	attestingIndices, err := attestation.AttestingIndices(att.AggregationBits, committee)
	require.NoError(t, err)
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	sigs := make([]bls.Signature, len(attestingIndices))
	for i, indice := range attestingIndices {
		sig := privKeys[indice].Sign(hashTreeRoot[:])
		sigs[i] = sig
	}
	att.Signature = bls.AggregateSignatures(sigs).Marshal()

	// Arbitrary aggregator index for testing purposes.
	aggregatorIndex := committee[0]
	sszUint := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, &sszUint, params.BeaconConfig().DomainSelectionProof, privKeys[aggregatorIndex])
	require.NoError(t, err)
	aggregateAndProof := &zondpb.AggregateAttestationAndProof{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: aggregatorIndex,
	}
	aggreSig, err := signing.ComputeDomainAndSign(beaconState, 0, aggregateAndProof, params.BeaconConfig().DomainAggregateAndProof, privKeys[aggregatorIndex])
	require.NoError(t, err)

	require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix())))

	chain := &mock.ChainService{Genesis: time.Now(),
		DB:    db,
		State: beaconState,
		FinalizedCheckPoint: &zondpb.Checkpoint{
			Root:  aggregateAndProof.Aggregate.Data.BeaconBlockRoot,
			Epoch: 0,
		}}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Service{
		ctx: ctx,
		cfg: &config{
			p2p:      p1,
			beaconDB: db,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:  attestations.NewPool(),
		},
		blkRootToPendingAtts:           make(map[[32]byte][]*zondpb.SignedAggregateAttestationAndProof),
		seenAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                  make(chan *signatureVerifier, verifierLimit),
	}
	go r.verifierRoutine()
	s, err := util.NewBeaconState()
	require.NoError(t, err)
	require.NoError(t, r.cfg.beaconDB.SaveState(context.Background(), s, root))

	r.blkRootToPendingAtts[root] = []*zondpb.SignedAggregateAttestationAndProof{{Message: aggregateAndProof, Signature: aggreSig}}
	require.NoError(t, r.processPendingAtts(context.Background()))

	assert.Equal(t, 1, len(r.cfg.attPool.AggregatedAttestations()), "Did not save aggregated att")
	assert.DeepEqual(t, att, r.cfg.attPool.AggregatedAttestations()[0], "Incorrect saved att")
	atts, err := r.cfg.attPool.UnaggregatedAttestations()
	require.NoError(t, err)
	assert.Equal(t, 0, len(atts), "Did save aggregated att")
	require.LogsContain(t, hook, "Verified and saved pending attestations to pool")
	cancel()
}

func TestValidatePendingAtts_CanPruneOldAtts(t *testing.T) {
	s := &Service{
		blkRootToPendingAtts: make(map[[32]byte][]*zondpb.SignedAggregateAttestationAndProof),
	}

	// 100 Attestations per block root.
	r1 := [32]byte{'A'}
	r2 := [32]byte{'B'}
	r3 := [32]byte{'C'}

	for i := primitives.Slot(0); i < 100; i++ {
		s.savePendingAtt(&zondpb.SignedAggregateAttestationAndProof{
			Message: &zondpb.AggregateAttestationAndProof{
				AggregatorIndex: primitives.ValidatorIndex(i),
				Aggregate: &zondpb.Attestation{
					Data: &zondpb.AttestationData{Slot: i, BeaconBlockRoot: r1[:]}}}})
		s.savePendingAtt(&zondpb.SignedAggregateAttestationAndProof{
			Message: &zondpb.AggregateAttestationAndProof{
				AggregatorIndex: primitives.ValidatorIndex(i*2 + i),
				Aggregate: &zondpb.Attestation{
					Data: &zondpb.AttestationData{Slot: i, BeaconBlockRoot: r2[:]}}}})
		s.savePendingAtt(&zondpb.SignedAggregateAttestationAndProof{
			Message: &zondpb.AggregateAttestationAndProof{
				AggregatorIndex: primitives.ValidatorIndex(i*3 + i),
				Aggregate: &zondpb.Attestation{
					Data: &zondpb.AttestationData{Slot: i, BeaconBlockRoot: r3[:]}}}})
	}

	assert.Equal(t, 100, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 100, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")
	assert.Equal(t, 100, len(s.blkRootToPendingAtts[r3]), "Did not save pending atts")

	// Set current slot to 50, it should prune 19 attestations. (50 - 31)
	s.validatePendingAtts(context.Background(), 50)
	assert.Equal(t, 81, len(s.blkRootToPendingAtts[r1]), "Did not delete pending atts")
	assert.Equal(t, 81, len(s.blkRootToPendingAtts[r2]), "Did not delete pending atts")
	assert.Equal(t, 81, len(s.blkRootToPendingAtts[r3]), "Did not delete pending atts")

	// Set current slot to 100 + slot_duration, it should prune all the attestations.
	s.validatePendingAtts(context.Background(), 100+params.BeaconConfig().SlotsPerEpoch)
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r1]), "Did not delete pending atts")
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r2]), "Did not delete pending atts")
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r3]), "Did not delete pending atts")

	// Verify the keys are deleted.
	assert.Equal(t, 0, len(s.blkRootToPendingAtts), "Did not delete block keys")
}

func TestValidatePendingAtts_NoDuplicatingAggregatorIndex(t *testing.T) {
	s := &Service{
		blkRootToPendingAtts: make(map[[32]byte][]*zondpb.SignedAggregateAttestationAndProof),
	}

	r1 := [32]byte{'A'}
	r2 := [32]byte{'B'}
	s.savePendingAtt(&zondpb.SignedAggregateAttestationAndProof{
		Message: &zondpb.AggregateAttestationAndProof{
			AggregatorIndex: 1,
			Aggregate: &zondpb.Attestation{
				Data: &zondpb.AttestationData{Slot: 1, BeaconBlockRoot: r1[:]}}}})
	s.savePendingAtt(&zondpb.SignedAggregateAttestationAndProof{
		Message: &zondpb.AggregateAttestationAndProof{
			AggregatorIndex: 2,
			Aggregate: &zondpb.Attestation{
				Data: &zondpb.AttestationData{Slot: 2, BeaconBlockRoot: r2[:]}}}})
	s.savePendingAtt(&zondpb.SignedAggregateAttestationAndProof{
		Message: &zondpb.AggregateAttestationAndProof{
			AggregatorIndex: 2,
			Aggregate: &zondpb.Attestation{
				Data: &zondpb.AttestationData{Slot: 3, BeaconBlockRoot: r2[:]}}}})

	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")
}

func TestSavePendingAtts_BeyondLimit(t *testing.T) {
	s := &Service{
		blkRootToPendingAtts: make(map[[32]byte][]*zondpb.SignedAggregateAttestationAndProof),
	}

	for i := 0; i < pendingAttsLimit; i++ {
		s.savePendingAtt(&zondpb.SignedAggregateAttestationAndProof{
			Message: &zondpb.AggregateAttestationAndProof{
				AggregatorIndex: primitives.ValidatorIndex(i),
				Aggregate: &zondpb.Attestation{
					Data: &zondpb.AttestationData{Slot: 1, BeaconBlockRoot: bytesutil.Bytes32(uint64(i))}}}})
	}
	r1 := [32]byte(bytesutil.Bytes32(0))
	r2 := [32]byte(bytesutil.Bytes32(uint64(pendingAttsLimit) - 1))

	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")

	for i := pendingAttsLimit; i < pendingAttsLimit+20; i++ {
		s.savePendingAtt(&zondpb.SignedAggregateAttestationAndProof{
			Message: &zondpb.AggregateAttestationAndProof{
				AggregatorIndex: primitives.ValidatorIndex(i),
				Aggregate: &zondpb.Attestation{
					Data: &zondpb.AttestationData{Slot: 1, BeaconBlockRoot: bytesutil.Bytes32(uint64(i))}}}})
	}

	r1 = [32]byte(bytesutil.Bytes32(uint64(pendingAttsLimit)))
	r2 = [32]byte(bytesutil.Bytes32(uint64(pendingAttsLimit) + 10))

	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r1]), "Saved pending atts")
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r2]), "Saved pending atts")

}
