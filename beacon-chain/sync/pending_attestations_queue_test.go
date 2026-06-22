package sync

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/go-qrl/p2p/qnr"
	"github.com/theQRL/qrysm/async/abool"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	dbtest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	"github.com/theQRL/qrysm/beacon-chain/p2p/peers"
	p2ptest "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	lruwrpr "github.com/theQRL/qrysm/cache/lru"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/proto/qrysm/v1alpha1/attestation"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	qrysmTime "github.com/theQRL/qrysm/time"
)

func TestProcessPendingAtts_NoBlockRequestBlock(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	p2 := p2ptest.NewTestP2P(t)
	p1.Connect(p2)
	assert.Equal(t, 1, len(p1.BHost.Network().Peers()), "Expected peers to be connected")
	p1.Peers().Add(new(qnr.Record), p2.PeerID(), nil, network.DirOutbound)
	p1.Peers().SetConnectionState(p2.PeerID(), peers.PeerConnected)
	p1.Peers().SetChainState(p2.PeerID(), &qrysmpb.Status{})

	chain := &mock.ChainService{Genesis: qrysmTime.Now(), FinalizedCheckPoint: &qrysmpb.Checkpoint{}}
	r := &Service{
		cfg:                  &config{p2p: p1, beaconDB: db, chain: chain, clock: startup.NewClock(chain.Genesis, chain.ValidatorsRoot)},
		blkRootToPendingAtts: make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
		chainStarted:         abool.New(),
	}

	a := &qrysmpb.AggregateAttestationAndProof{Aggregate: &qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Target: &qrysmpb.Checkpoint{Root: make([]byte, 32)}}}}
	r.blkRootToPendingAtts[[32]byte{'A'}] = []*qrysmpb.SignedAggregateAttestationAndProof{{Message: a}}
	require.NoError(t, r.processPendingAtts(context.Background()))
	require.LogsContain(t, hook, "Requesting block for pending attestation")
}

func TestProcessPendingAtts_HasBlockSaveUnAggregatedAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	validators := uint64(256)

	beaconState, privKeys := util.DeterministicGenesisStateZond(t, validators)

	sb := util.NewBeaconBlockZond()
	util.SaveBlock(t, context.Background(), db, sb)
	root, err := sb.Block.HashTreeRoot()
	require.NoError(t, err)

	aggBits := bitfield.NewBitlist(2)
	aggBits.SetBitAt(1, true)
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
	attesterDomain, err := signing.Domain(beaconState.Fork(), 0, params.BeaconConfig().DomainBeaconAttester, beaconState.GenesisValidatorsRoot())
	require.NoError(t, err)
	hashTreeRoot, err := signing.ComputeSigningRoot(att.Data, attesterDomain)
	assert.NoError(t, err)
	for _, i := range attestingIndices {
		att.Signatures = [][]byte{privKeys[i].Sign(hashTreeRoot[:]).Marshal()}
	}

	// Arbitrary aggregator index for testing purposes.
	aggregatorIndex := committee[0]
	sszUint := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, &sszUint, params.BeaconConfig().DomainSelectionProof, privKeys[aggregatorIndex])
	require.NoError(t, err)
	aggregateAndProof := &qrysmpb.AggregateAttestationAndProof{
		SelectionProof:  sig,
		Aggregate:       att,
		AggregatorIndex: aggregatorIndex,
	}
	aggreSig, err := signing.ComputeDomainAndSign(beaconState, 0, aggregateAndProof, params.BeaconConfig().DomainAggregateAndProof, privKeys[aggregatorIndex])
	require.NoError(t, err)

	require.NoError(t, beaconState.SetGenesisTime(uint64(time.Now().Unix())))

	chain := &mock.ChainService{Genesis: time.Now(),
		State: beaconState,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
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
		blkRootToPendingAtts:             make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	go r.verifierRoutine()

	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, r.cfg.beaconDB.SaveState(context.Background(), s, root))

	r.blkRootToPendingAtts[root] = []*qrysmpb.SignedAggregateAttestationAndProof{{Message: aggregateAndProof, Signature: aggreSig}}
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

	s, _ := util.DeterministicGenesisStateZond(t, 256)
	chain := &mock.ChainService{
		State:   s,
		Genesis: qrysmTime.Now(), FinalizedCheckPoint: &qrysmpb.Checkpoint{Root: make([]byte, 32)}}
	r := &Service{
		cfg: &config{
			p2p:      p1,
			beaconDB: db,
			chain:    chain,
			clock:    startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			attPool:  attestations.NewPool(),
		},
		blkRootToPendingAtts: make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
	}

	priv, err := ml_dsa_87.RandKey()
	require.NoError(t, err)
	a := &qrysmpb.AggregateAttestationAndProof{
		Aggregate: &qrysmpb.Attestation{
			Signatures:      [][]byte{priv.Sign([]byte("foo")).Marshal()},
			AggregationBits: bitfield.Bitlist{0x02},
			Data:            util.HydrateAttestationData(&qrysmpb.AttestationData{}),
		},
		SelectionProof: make([]byte, field_params.MLDSA87SignatureLength),
	}

	b := util.NewBeaconBlockZond()
	r32, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), r.cfg.beaconDB, b)
	require.NoError(t, r.cfg.beaconDB.SaveState(context.Background(), s, r32))

	r.blkRootToPendingAtts[r32] = []*qrysmpb.SignedAggregateAttestationAndProof{{Message: a, Signature: make([]byte, field_params.MLDSA87SignatureLength)}}
	require.NoError(t, r.processPendingAtts(context.Background()))

	assert.Equal(t, false, p1.BroadcastCalled.Load(), "Broadcasted bad aggregate")
	// Clear pool.
	err = r.cfg.attPool.DeleteUnaggregatedAttestation(a.Aggregate)
	require.NoError(t, err)

	validators := uint64(256)

	_, privKeys := util.DeterministicGenesisStateZond(t, validators)
	aggBits := bitfield.NewBitlist(2)
	aggBits.SetBitAt(1, true)
	att := &qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			BeaconBlockRoot: r32[:],
			Source:          &qrysmpb.Checkpoint{Epoch: 0, Root: bytesutil.PadTo([]byte("hello-world"), 32)},
			Target:          &qrysmpb.Checkpoint{Epoch: 0, Root: r32[:]},
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
		att.Signatures = [][]byte{privKeys[i].Sign(hashTreeRoot[:]).Marshal()}
	}

	// Arbitrary aggregator index for testing purposes.
	aggregatorIndex := committee[0]
	sszSlot := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(s, 0, &sszSlot, params.BeaconConfig().DomainSelectionProof, privKeys[aggregatorIndex])
	require.NoError(t, err)
	aggregateAndProof := &qrysmpb.AggregateAttestationAndProof{
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
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
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
		blkRootToPendingAtts:             make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
		seenUnAggregatedAttestationCache: lruwrpr.New(10),
		signatureChan:                    make(chan *signatureVerifier, verifierLimit),
	}
	go r.verifierRoutine()

	r.blkRootToPendingAtts[r32] = []*qrysmpb.SignedAggregateAttestationAndProof{{Message: aggregateAndProof, Signature: aggreSig}}
	require.NoError(t, r.processPendingAtts(context.Background()))

	assert.Equal(t, true, p1.BroadcastCalled.Load(), "Could not broadcast the good aggregate")
	cancel()
}

func TestProcessPendingAtts_HasBlockSaveAggregatedAtt(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p1 := p2ptest.NewTestP2P(t)
	validators := uint64(256)

	beaconState, privKeys := util.DeterministicGenesisStateZond(t, validators)

	sb := util.NewBeaconBlockZond()
	util.SaveBlock(t, context.Background(), db, sb)
	root, err := sb.Block.HashTreeRoot()
	require.NoError(t, err)

	aggBits := bitfield.NewBitlist(validators / uint64(params.BeaconConfig().SlotsPerEpoch))
	aggBits.SetBitAt(0, true)
	aggBits.SetBitAt(1, true)
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

	// Arbitrary aggregator index for testing purposes.
	aggregatorIndex := committee[0]
	sszUint := primitives.SSZUint64(att.Data.Slot)
	sig, err := signing.ComputeDomainAndSign(beaconState, 0, &sszUint, params.BeaconConfig().DomainSelectionProof, privKeys[aggregatorIndex])
	require.NoError(t, err)
	aggregateAndProof := &qrysmpb.AggregateAttestationAndProof{
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
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
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
		blkRootToPendingAtts: make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
		signatureChan:        make(chan *signatureVerifier, verifierLimit),
	}
	r.initCaches()
	go r.verifierRoutine()
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, r.cfg.beaconDB.SaveState(context.Background(), s, root))

	r.blkRootToPendingAtts[root] = []*qrysmpb.SignedAggregateAttestationAndProof{{Message: aggregateAndProof, Signature: aggreSig}}
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
		blkRootToPendingAtts: make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
	}

	// 100 Attestations per block root.
	r1 := [32]byte{'A'}
	r2 := [32]byte{'B'}
	r3 := [32]byte{'C'}

	for i := range primitives.Slot(100) {
		s.savePendingAtt(&qrysmpb.SignedAggregateAttestationAndProof{
			Message: &qrysmpb.AggregateAttestationAndProof{
				AggregatorIndex: primitives.ValidatorIndex(i),
				Aggregate: &qrysmpb.Attestation{
					Data: &qrysmpb.AttestationData{Slot: i, BeaconBlockRoot: r1[:]}}}})
		s.savePendingAtt(&qrysmpb.SignedAggregateAttestationAndProof{
			Message: &qrysmpb.AggregateAttestationAndProof{
				AggregatorIndex: primitives.ValidatorIndex(i*2 + i),
				Aggregate: &qrysmpb.Attestation{
					Data: &qrysmpb.AttestationData{Slot: i, BeaconBlockRoot: r2[:]}}}})
		s.savePendingAtt(&qrysmpb.SignedAggregateAttestationAndProof{
			Message: &qrysmpb.AggregateAttestationAndProof{
				AggregatorIndex: primitives.ValidatorIndex(i*3 + i),
				Aggregate: &qrysmpb.Attestation{
					Data: &qrysmpb.AttestationData{Slot: i, BeaconBlockRoot: r3[:]}}}})
	}

	assert.Equal(t, 100, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 100, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")
	assert.Equal(t, 100, len(s.blkRootToPendingAtts[r3]), "Did not save pending atts")

	// Set current slot to 146, it should prune 19 attestations. (146 - 127)
	s.validatePendingAtts(context.Background(), 146)
	assert.Equal(t, 81, len(s.blkRootToPendingAtts[r1]), "Did not delete pending atts")
	assert.Equal(t, 81, len(s.blkRootToPendingAtts[r2]), "Did not delete pending atts")
	assert.Equal(t, 81, len(s.blkRootToPendingAtts[r3]), "Did not delete pending atts")

	// Set current slot to 387 + slot_duration, it should prune all the attestations.
	s.validatePendingAtts(context.Background(), 387+params.BeaconConfig().SlotsPerEpoch)
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r1]), "Did not delete pending atts")
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r2]), "Did not delete pending atts")
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r3]), "Did not delete pending atts")

	// Verify the keys are deleted.
	assert.Equal(t, 0, len(s.blkRootToPendingAtts), "Did not delete block keys")
}

func TestValidatePendingAtts_NoDuplicatingAtts(t *testing.T) {
	s := &Service{
		blkRootToPendingAtts: make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
	}

	r1 := [32]byte{'A'}
	r2 := [32]byte{'B'}
	s.savePendingAtt(&qrysmpb.SignedAggregateAttestationAndProof{
		Message: &qrysmpb.AggregateAttestationAndProof{
			AggregatorIndex: 1,
			Aggregate: &qrysmpb.Attestation{
				Data: &qrysmpb.AttestationData{Slot: 1, BeaconBlockRoot: r1[:]}}}})
	s.savePendingAtt(&qrysmpb.SignedAggregateAttestationAndProof{
		Message: &qrysmpb.AggregateAttestationAndProof{
			AggregatorIndex: 2,
			Aggregate: &qrysmpb.Attestation{
				Data: &qrysmpb.AttestationData{Slot: 2, BeaconBlockRoot: r2[:]}}}})
	s.savePendingAtt(&qrysmpb.SignedAggregateAttestationAndProof{
		Message: &qrysmpb.AggregateAttestationAndProof{
			AggregatorIndex: 2,
			Aggregate: &qrysmpb.Attestation{
				Data: &qrysmpb.AttestationData{Slot: 2, BeaconBlockRoot: r2[:]}}}})

	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")
}

func TestSavePendingAtts_BeyondLimit(t *testing.T) {
	s := &Service{
		blkRootToPendingAtts: make(map[[32]byte][]*qrysmpb.SignedAggregateAttestationAndProof),
	}

	for i := range pendingAttsLimit {
		s.savePendingAtt(&qrysmpb.SignedAggregateAttestationAndProof{
			Message: &qrysmpb.AggregateAttestationAndProof{
				AggregatorIndex: primitives.ValidatorIndex(i),
				Aggregate: &qrysmpb.Attestation{
					Data: &qrysmpb.AttestationData{Slot: 1, BeaconBlockRoot: bytesutil.Bytes32(uint64(i))}}}})
	}
	r1 := [32]byte(bytesutil.Bytes32(0))
	r2 := [32]byte(bytesutil.Bytes32(uint64(pendingAttsLimit) - 1))

	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r1]), "Did not save pending atts")
	assert.Equal(t, 1, len(s.blkRootToPendingAtts[r2]), "Did not save pending atts")

	for i := pendingAttsLimit; i < pendingAttsLimit+20; i++ {
		s.savePendingAtt(&qrysmpb.SignedAggregateAttestationAndProof{
			Message: &qrysmpb.AggregateAttestationAndProof{
				AggregatorIndex: primitives.ValidatorIndex(i),
				Aggregate: &qrysmpb.Attestation{
					Data: &qrysmpb.AttestationData{Slot: 1, BeaconBlockRoot: bytesutil.Bytes32(uint64(i))}}}})
	}

	r1 = [32]byte(bytesutil.Bytes32(uint64(pendingAttsLimit)))
	r2 = [32]byte(bytesutil.Bytes32(uint64(pendingAttsLimit) + 10))

	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r1]), "Saved pending atts")
	assert.Equal(t, 0, len(s.blkRootToPendingAtts[r2]), "Saved pending atts")

}

// Regression tests for upstream prysm#16153: ignoreAggregatorIndex must collapse
// aggregates that differ only by aggregator index, while includeAggregatorIndex
// must keep the existing save-time behaviour.
func Test_pendingAggregatesAreEqual(t *testing.T) {
	mkAgg := func(aggregatorIndex primitives.ValidatorIndex, slot primitives.Slot, committeeIndex primitives.CommitteeIndex, bits bitfield.Bitlist, withSig bool) *qrysmpb.SignedAggregateAttestationAndProof {
		agg := &qrysmpb.SignedAggregateAttestationAndProof{
			Message: &qrysmpb.AggregateAttestationAndProof{
				AggregatorIndex: aggregatorIndex,
				Aggregate: &qrysmpb.Attestation{
					Data: &qrysmpb.AttestationData{
						Slot:           slot,
						CommitteeIndex: committeeIndex,
					},
					AggregationBits: bits,
				},
			},
		}
		if withSig {
			agg.Signature = []byte{0x01}
		}
		return agg
	}

	t.Run("same aggregator with sigs is equal under includeAggregatorIndex", func(t *testing.T) {
		a := mkAgg(1, 1, 1, bitfield.Bitlist{0b1111}, true)
		b := mkAgg(1, 1, 1, bitfield.Bitlist{0b1111}, true)
		assert.Equal(t, true, pendingAggregatesAreEqual(a, b, includeAggregatorIndex))
	})
	t.Run("different aggregator with sigs is not equal under includeAggregatorIndex", func(t *testing.T) {
		a := mkAgg(1, 1, 1, bitfield.Bitlist{0b1111}, true)
		b := mkAgg(2, 1, 1, bitfield.Bitlist{0b1111}, true)
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b, includeAggregatorIndex))
	})
	t.Run("different aggregator is equal under ignoreAggregatorIndex when data matches", func(t *testing.T) {
		a := mkAgg(1, 1, 1, bitfield.Bitlist{0b1111}, true)
		b := mkAgg(2, 1, 1, bitfield.Bitlist{0b1111}, true)
		assert.Equal(t, true, pendingAggregatesAreEqual(a, b, ignoreAggregatorIndex))
	})
	t.Run("different slot is not equal under ignoreAggregatorIndex", func(t *testing.T) {
		a := mkAgg(1, 1, 1, bitfield.Bitlist{0b1111}, true)
		b := mkAgg(2, 2, 1, bitfield.Bitlist{0b1111}, true)
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b, ignoreAggregatorIndex))
	})
	t.Run("different committee index is not equal under ignoreAggregatorIndex", func(t *testing.T) {
		a := mkAgg(1, 1, 1, bitfield.Bitlist{0b1111}, true)
		b := mkAgg(2, 1, 2, bitfield.Bitlist{0b1111}, true)
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b, ignoreAggregatorIndex))
	})
	t.Run("different aggregation bits is not equal under ignoreAggregatorIndex", func(t *testing.T) {
		a := mkAgg(1, 1, 1, bitfield.Bitlist{0b1111}, true)
		b := mkAgg(2, 1, 1, bitfield.Bitlist{0b1000}, true)
		assert.Equal(t, false, pendingAggregatesAreEqual(a, b, ignoreAggregatorIndex))
	})
}
