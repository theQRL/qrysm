package sync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsubpb "github.com/libp2p/go-libp2p-pubsub/pb"
	gcache "github.com/patrickmn/go-cache"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/async/abool"
	mock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	coreblocks "github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/signing"
	coreTime "github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	dbtest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/operations/attestations"
	slashingsmock "github.com/theQRL/qrysm/beacon-chain/operations/slashings/mock"
	"github.com/theQRL/qrysm/beacon-chain/p2p"
	p2ptest "github.com/theQRL/qrysm/beacon-chain/p2p/testing"
	"github.com/theQRL/qrysm/beacon-chain/startup"
	"github.com/theQRL/qrysm/beacon-chain/state/stategen"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	lruwrpr "github.com/theQRL/qrysm/cache/lru"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/ml_dsa_87"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

// General note for writing validation tests: Use a random value for any field
// on the beacon block to avoid hitting shared global cache conditions across
// tests in this package.

func TestValidateBeaconBlockPubSub_InvalidSignature(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	badPrivKeyIdx := proposerIdx + 1 // We generate a valid signature from a wrong private key which fails to verify
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[badPrivKeyIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		DB: db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorIs(t, err, signing.ErrSigFailedToVerify)
	result := res == pubsub.ValidationReject
	assert.Equal(t, true, result)
}

func TestValidateBeaconBlockPubSub_InvalidSignature_MarksBlockAsBad(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	badPrivKeyIdx := proposerIdx + 1 // We generate a valid signature from a wrong private key which fails to verify
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[badPrivKeyIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		DB: db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	blockRoot, err := msg.Block.HashTreeRoot()
	require.NoError(t, err)

	// Block must not be in the bad-block cache before validation runs.
	assert.Equal(t, false, r.hasBadBlock(blockRoot), "block should not be marked as bad initially")

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorIs(t, err, coreblocks.ErrInvalidSignature)
	assert.Equal(t, pubsub.ValidationReject, res)

	// After a real signature failure the block must be cached as bad.
	assert.Equal(t, true, r.hasBadBlock(blockRoot), "block should be marked as bad after invalid signature")
}

func TestValidateBeaconBlockPubSub_TransientVerifyError_DoesNotMarkBlockAsBad(t *testing.T) {
	// Regression test for the qrysm-side fix: a non-signature error returned by
	// VerifyBlockSignatureUsingCurrentFork (here triggered by a proposer index
	// that doesn't exist in the parent state) must not poison the bad-block cache.
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))

	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	// ProposerIndex deliberately past the validator set (state has 100 validators, indices 0..99).
	// VerifyBlockSignatureUsingCurrentFork will fail at ValidatorAtIndex with a non-signature error.
	msg.Block.ProposerIndex = primitives.ValidatorIndex(999)
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		DB: db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	blockRoot, err := msg.Block.HashTreeRoot()
	require.NoError(t, err)

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.NotNil(t, err)
	require.Equal(t, false, errors.Is(err, coreblocks.ErrInvalidSignature), "transient error must not surface as ErrInvalidSignature")
	assert.NotEqual(t, pubsub.ValidationAccept, res)

	// The block must NOT be cached as bad — its signature was never proven invalid.
	assert.Equal(t, false, r.hasBadBlock(blockRoot), "block must not be marked as bad on a transient error")
}

func TestValidateBeaconBlockPubSub_BlockAlreadyPresentInDB(t *testing.T) {
	db := dbtest.SetupDB(t)
	ctx := context.Background()

	p := p2ptest.NewTestP2P(t)
	msg := util.NewBeaconBlockZond()
	msg.Block.Slot = 100
	msg.Block.ParentRoot = util.Random32Bytes(t)
	util.SaveBlock(t, context.Background(), db, msg)

	chainService := &mock.ChainService{Genesis: time.Now()}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err := p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)

	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "block present in DB should be ignored")
}

func TestValidateBeaconBlockPubSub_CanRecoverStateSummary(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		DB: db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
	assert.NotNil(t, m.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateBeaconBlockPubSub_IsInCache(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		InitSyncBlockRoots: map[[32]byte]bool{bRoot: true},
		DB:                 db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
	assert.NotNil(t, m.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateBeaconBlockPubSub_ValidProposerSignature(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		DB: db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
	assert.NotNil(t, m.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateBeaconBlockPubSub_WithLookahead(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	// The next block is only 1 epoch ahead so as to not induce a new seed.
	blkSlot := params.BeaconConfig().SlotsPerEpoch.Mul(uint64(coreTime.NextEpoch(copied)))
	copied, err = transition.ProcessSlots(context.Background(), copied, blkSlot)
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Slot = blkSlot
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	offset := int64(blkSlot.Mul(params.BeaconConfig().SecondsPerSlot))
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-offset, 0),
		DB:    db,
		State: beaconState,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
		subHandler:          newSubTopicHandler(),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
	assert.NotNil(t, m.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateBeaconBlockPubSub_AdvanceEpochsForState(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	// The next block is at least 2 epochs ahead to induce shuffling and a new seed.
	blkSlot := params.BeaconConfig().SlotsPerEpoch * 2
	copied, err = transition.ProcessSlots(context.Background(), copied, blkSlot)
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Slot = blkSlot
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	offset := int64(blkSlot.Mul(params.BeaconConfig().SecondsPerSlot))
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-offset, 0),
		DB:    db,
		State: beaconState,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
	assert.NotNil(t, m.ValidatorData, "Decoded message was not set on the message validator data")
}

func TestValidateBeaconBlockPubSub_Syncing(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	b := []byte("sk")
	b48 := bytesutil.ToBytes48(b)
	sk, err := ml_dsa_87.SecretKeyFromSeed(b48[:])
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = util.Random32Bytes(t)
	msg.Signature = sk.Sign([]byte("data")).Marshal()
	chainService := &mock.ChainService{
		Genesis: time.Now(),
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: true},
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
		},
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "block is ignored until fully synced")
}

func TestValidateBeaconBlockPubSub_IgnoreAndQueueBlocksFromNearFuture(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()

	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg := util.NewBeaconBlockZond()
	msg.Block.Slot = 2 // two slots in future
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.ProposerIndex = proposerIdx
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Now(),
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		State: beaconState}
	r := &Service{
		cfg: &config{
			p2p:           p,
			beaconDB:      db,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		chainStarted:        abool.New(),
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "early block, with current slot", err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "early block should be ignored and queued")

	// check if the block is inserted in the Queue
	assert.Equal(t, true, len(r.pendingBlocksInCache(msg.Block.Slot)) == 1)
}

func TestValidateBeaconBlockPubSub_RejectBlocksFromFuture(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	b := []byte("sk")
	b48 := bytesutil.ToBytes48(b)
	sk, err := ml_dsa_87.SecretKeyFromSeed(b48[:])
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.Slot = 10
	msg.Block.ParentRoot = util.Random32Bytes(t)
	msg.Signature = sk.Sign([]byte("data")).Marshal()

	chainService := &mock.ChainService{Genesis: time.Now()}
	r := &Service{
		cfg: &config{
			p2p:           p,
			beaconDB:      db,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
		},
		chainStarted:        abool.New(),
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "block from the future should be ignored")
}

func TestValidateBeaconBlockPubSub_RejectBlocksFromThePast(t *testing.T) {
	db := dbtest.SetupDB(t)
	b := []byte("sk")
	b48 := bytesutil.ToBytes48(b)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	sk, err := ml_dsa_87.SecretKeyFromSeed(b48[:])
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = util.Random32Bytes(t)
	msg.Block.Slot = 10
	msg.Signature = sk.Sign([]byte("data")).Marshal()

	genesisTime := time.Now()
	chainService := &mock.ChainService{
		Genesis: time.Unix(genesisTime.Unix()-1000, 0),
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 1,
		},
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "greater or equal to block slot", err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "block from the past should be ignored")
}

func TestValidateBeaconBlockPubSub_SeenProposerSlot(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, beaconState)
	require.NoError(t, err)

	msg := util.NewBeaconBlockZond()
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	// Clone the same block (same signature, not an equivocation).
	msgClone := util.NewBeaconBlockZond()
	msgClone.Block.Slot = 1
	msgClone.Block.ProposerIndex = proposerIdx
	msgClone.Block.ParentRoot = bRoot[:]
	msgClone.Signature = msg.Signature

	signedBlock, err := blocks.NewSignedBeaconBlock(msg)
	require.NoError(t, err)

	slashingPool := &slashingsmock.PoolMock{}
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		Block: signedBlock,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			slashingPool:  slashingPool,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}

	r.setSeenBlockIndexSlot(msg.Block.Slot, msg.Block.ProposerIndex)
	time.Sleep(10 * time.Millisecond) // Wait for cached value to pass through buffers.

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msgClone)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.NoError(t, err)
	assert.Equal(t, pubsub.ValidationIgnore, res, "block with same signature should be ignored")
	assert.Equal(t, 0, len(slashingPool.PendingPropSlashings), "Expected no slashings for same signature")
}

func TestValidateBeaconBlockPubSub_FilterByFinalizedEpoch(t *testing.T) {
	hook := logTest.NewGlobal()
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)

	parent := util.NewBeaconBlockZond()
	util.SaveBlock(t, context.Background(), db, parent)
	parentRoot, err := parent.Block.HashTreeRoot()
	require.NoError(t, err)
	chain := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 1,
		},
		ValidatorsRoot: [32]byte{},
	}

	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			chain:         chain,
			clock:         startup.NewClock(chain.Genesis, chain.ValidatorsRoot),
			blockNotifier: chain.BlockNotifier(),
			attPool:       attestations.NewPool(),
			initialSync:   &mockSync.Sync{IsSyncing: false},
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	b := util.NewBeaconBlockZond()
	b.Block.Slot = 1
	b.Block.ParentRoot = parentRoot[:]
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, b)
	require.NoError(t, err)
	digest, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, make([]byte, 32))
	assert.NoError(t, err)
	topic := fmt.Sprintf(p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()], digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	res, err := r.validateBeaconBlockPubSub(context.Background(), "", m)
	_ = err
	assert.Equal(t, pubsub.ValidationIgnore, res)

	hook.Reset()
	b.Block.Slot = params.BeaconConfig().SlotsPerEpoch
	buf = new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, b)
	require.NoError(t, err)
	m = &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	res, err = r.validateBeaconBlockPubSub(context.Background(), "", m)
	assert.NoError(t, err)
	assert.Equal(t, pubsub.ValidationIgnore, res)
}

func TestValidateBeaconBlockPubSub_ParentNotFinalizedDescendant(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{
		Genesis:      time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		NotFinalized: true,
		State:        beaconState,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		},
		VerifyBlkDescendantErr: errors.New("not part of finalized chain"),
		DB:                     db,
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.Equal(t, pubsub.ValidationReject, res, "Wrong validation result returned")
	require.ErrorContains(t, "not descendant of finalized checkpoint", err)
}

func TestValidateBeaconBlockPubSub_InvalidParentBlock(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Slot = 1
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	// Mutate Signature
	copy(msg.Signature[:4], []byte{1, 2, 3, 4})
	currBlockRoot, err := msg.Block.HashTreeRoot()
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "signature did not verify", err)
	assert.Equal(t, res, pubsub.ValidationReject, "block with invalid signature should be rejected")

	require.NoError(t, copied.SetSlot(2))
	proposerIdx, err = helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg = util.NewBeaconBlockZond()
	msg.Block.Slot = 2
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.ParentRoot = currBlockRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	buf = new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	m = &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	chainService = &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(2*params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
		}}
	r.cfg.chain = chainService
	r.cfg.clock = startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot)

	res, err = r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "has an invalid parent", err)
	// Expect block with bad parent to fail too
	assert.Equal(t, res, pubsub.ValidationReject, "block with invalid parent should be rejected")
}

func TestValidateBeaconBlockPubSub_InsertValidPendingBlock(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)
	msg := util.NewBeaconBlockZond()
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Slot = 1
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0),
		State: beaconState,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.ErrorContains(t, "unknown parent for block", err)
	assert.Equal(t, res, pubsub.ValidationIgnore, "block with unknown parent should be ignored")
	bRoot, err = msg.Block.HashTreeRoot()
	assert.NoError(t, err)
	assert.Equal(t, true, r.seenPendingBlocks[bRoot])
}

func TestValidateBeaconBlockPubSub_RejectBlocksFromBadParent(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()

	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	parentBlock.Block.ParentRoot = bytesutil.PadTo([]byte("foo"), 32)
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))

	copied := beaconState.Copy()
	// The next block is at least 2 epochs ahead to induce shuffling and a new seed.
	blkSlot := params.BeaconConfig().SlotsPerEpoch * 2
	copied, err = transition.ProcessSlots(context.Background(), copied, blkSlot)
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg := util.NewBeaconBlockZond()
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Slot = blkSlot

	perSlot := params.BeaconConfig().SecondsPerSlot
	// current slot time
	slotsSinceGenesis := primitives.Slot(1000)
	// max uint, divided by slot time. But avoid losing precision too much.
	overflowBase := (1 << 63) / (perSlot >> 1)
	msg.Block.Slot = slotsSinceGenesis.Add(overflowBase)

	// valid block
	msg.Block.ParentRoot = bRoot[:]
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	genesisTime := time.Now()

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{
		Genesis: time.Unix(genesisTime.Unix()-int64(slotsSinceGenesis.Mul(perSlot)), 0),
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
		},
	}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache:      lruwrpr.New(10),
		badBlockCache:       lruwrpr.New(10),
		slotToPendingBlocks: gcache.New(time.Second, 2*time.Second),
		seenPendingBlocks:   make(map[[32]byte]bool),
	}
	r.setBadBlock(ctx, bytesutil.ToBytes32(msg.Block.ParentRoot))

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	digest, err := r.currentForkDigest()
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, digest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	assert.ErrorContains(t, "invalid parent", err)
	assert.Equal(t, res, pubsub.ValidationReject)
}

func TestService_setBadBlock_DoesntSetWithContextErr(t *testing.T) {
	s := Service{}
	s.initCaches()

	root := [32]byte{'b', 'a', 'd'}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.setBadBlock(ctx, root)
	if s.hasBadBlock(root) {
		t.Error("Set bad root with cancelled context")
	}
}

func TestService_isBlockQueueable(t *testing.T) {
	currentTime := time.Now().Round(time.Second)
	genesisTime := uint64(currentTime.Unix() - int64(params.BeaconConfig().SecondsPerSlot))
	blockSlot := primitives.Slot(1)

	// slot time within MAXIMUM_GOSSIP_CLOCK_DISPARITY, so don't queue the block.
	receivedTime := currentTime.Add(-400 * time.Millisecond)
	result := isBlockQueueable(genesisTime, blockSlot, receivedTime)
	assert.Equal(t, false, result)

	// slot time just above MAXIMUM_GOSSIP_CLOCK_DISPARITY, so queue the block.
	receivedTime = currentTime.Add(-600 * time.Millisecond)
	result = isBlockQueueable(genesisTime, blockSlot, receivedTime)
	assert.Equal(t, true, result)
}

func TestValidateBeaconBlockPubSub_ValidExecutionPayload(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	presentTime := time.Now().Unix()
	require.NoError(t, beaconState.SetGenesisTime(uint64(presentTime)))
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Body.ExecutionPayload.Timestamp = uint64(presentTime) + params.BeaconConfig().SecondsPerSlot
	msg.Block.Body.ExecutionPayload.GasUsed = 10
	msg.Block.Body.ExecutionPayload.GasLimit = 11
	msg.Block.Body.ExecutionPayload.BlockHash = bytesutil.PadTo([]byte("blockHash"), 32)
	msg.Block.Body.ExecutionPayload.ParentHash = bytesutil.PadTo([]byte("parentHash"), 32)
	msg.Block.Body.ExecutionPayload.Transactions = append(msg.Block.Body.ExecutionPayload.Transactions, []byte("transaction 1"), []byte("transaction 2"))
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(presentTime-int64(params.BeaconConfig().SecondsPerSlot), 0),
		DB: db,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	genesisValidatorsRoot := r.cfg.clock.GenesisValidatorsRoot()
	zondDigest, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, genesisValidatorsRoot[:])
	require.NoError(t, err)
	topic = r.addDigestToTopic(topic, zondDigest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.NoError(t, err)
	result := res == pubsub.ValidationAccept
	require.Equal(t, true, result)
}

func TestValidateBeaconBlockPubSub_InvalidPayloadTimestamp(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	presentTime := time.Now().Unix()
	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Body.ExecutionPayload.Timestamp = uint64(presentTime - 600) // add an invalid timestamp
	msg.Block.Body.ExecutionPayload.GasUsed = 10
	msg.Block.Body.ExecutionPayload.GasLimit = 11
	msg.Block.Body.ExecutionPayload.BlockHash = bytesutil.PadTo([]byte("blockHash"), 32)
	msg.Block.Body.ExecutionPayload.ParentHash = bytesutil.PadTo([]byte("parentHash"), 32)
	msg.Block.Body.ExecutionPayload.Transactions = append(msg.Block.Body.ExecutionPayload.Transactions, []byte("transaction 1"), []byte("transaction 2"))
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	stateGen := stategen.New(db, doublylinkedtree.New())
	chainService := &mock.ChainService{Genesis: time.Unix(presentTime-int64(params.BeaconConfig().SecondsPerSlot), 0),
		DB: db,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	genesisValidatorsRoot := r.cfg.clock.GenesisValidatorsRoot()
	zondDigest, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, genesisValidatorsRoot[:])
	assert.NoError(t, err)
	topic = r.addDigestToTopic(topic, zondDigest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}
	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.NotNil(t, err)
	result := res == pubsub.ValidationReject
	assert.Equal(t, true, result)
}

// NOTE(rgeraldes24): test is not valid at the moment: re-enable once we have more block versions
/*
func Test_validateBellatrixBeaconBlock(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	stateGen := stategen.New(db, doublylinkedtree.New())
	presentTime := time.Now().Unix()
	chainService := &mock.ChainService{Genesis: time.Unix(presentTime-int64(params.BeaconConfig().SecondsPerSlot), 0),
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	st, _ := util.DeterministicGenesisStateZond(t, 1)
	b := util.NewBeaconBlockZond()
	blk, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	require.ErrorContains(t, "block and state are not the same version", r.validateBellatrixBeaconBlock(ctx, st, blk.Block()))
}
*/

func Test_validateBellatrixBeaconBlockParentValidation(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	stateGen := stategen.New(db, doublylinkedtree.New())

	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Body.ExecutionPayload.Timestamp = beaconState.GenesisTime() + params.BeaconConfig().SecondsPerSlot
	msg.Block.Body.ExecutionPayload.GasUsed = 10
	msg.Block.Body.ExecutionPayload.GasLimit = 11
	msg.Block.Body.ExecutionPayload.BlockHash = bytesutil.PadTo([]byte("blockHash"), 32)
	msg.Block.Body.ExecutionPayload.ParentHash = bytesutil.PadTo([]byte("parentHash"), 32)
	msg.Block.Body.ExecutionPayload.Transactions = append(msg.Block.Body.ExecutionPayload.Transactions, []byte("transaction 1"), []byte("transaction 2"))
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	blk, err := blocks.NewSignedBeaconBlock(msg)
	require.NoError(t, err)

	chainService := &mock.ChainService{Genesis: time.Unix(int64(beaconState.GenesisTime()), 0),
		OptimisticRoots: make(map[[32]byte]bool),
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		}}

	chainService.OptimisticRoots[blk.Block().ParentRoot()] = true
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}
	require.ErrorContains(t, "parent of the block is optimistic", r.validateBellatrixBeaconBlock(ctx, beaconState, blk.Block()))
}

func Test_validateBeaconBlockProcessingWhenParentIsOptimistic(t *testing.T) {
	db := dbtest.SetupDB(t)
	p := p2ptest.NewTestP2P(t)
	ctx := context.Background()
	stateGen := stategen.New(db, doublylinkedtree.New())

	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)
	parentBlock := util.NewBeaconBlockZond()
	util.SaveBlock(t, ctx, db, parentBlock)
	bRoot, err := parentBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	require.NoError(t, db.SaveState(ctx, beaconState, bRoot))
	require.NoError(t, db.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bRoot[:]}))
	copied := beaconState.Copy()
	require.NoError(t, copied.SetSlot(1))
	proposerIdx, err := helpers.BeaconProposerIndex(ctx, copied)
	require.NoError(t, err)

	msg := util.NewBeaconBlockZond()
	msg.Block.ParentRoot = bRoot[:]
	msg.Block.Slot = 1
	msg.Block.ProposerIndex = proposerIdx
	msg.Block.Body.ExecutionPayload.Timestamp = beaconState.GenesisTime() + params.BeaconConfig().SecondsPerSlot
	msg.Block.Body.ExecutionPayload.GasUsed = 10
	msg.Block.Body.ExecutionPayload.GasLimit = 11
	msg.Block.Body.ExecutionPayload.BlockHash = bytesutil.PadTo([]byte("blockHash"), 32)
	msg.Block.Body.ExecutionPayload.ParentHash = bytesutil.PadTo([]byte("parentHash"), 32)
	msg.Block.Body.ExecutionPayload.Transactions = append(msg.Block.Body.ExecutionPayload.Transactions, []byte("transaction 1"), []byte("transaction 2"))
	msg.Signature, err = signing.ComputeDomainAndSign(beaconState, 0, msg.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[proposerIdx])
	require.NoError(t, err)

	chainService := &mock.ChainService{Genesis: time.Unix(int64(beaconState.GenesisTime()), 0),
		DB:         db,
		Optimistic: true,
		FinalizedCheckPoint: &qrysmpb.Checkpoint{
			Epoch: 0,
			Root:  make([]byte, 32),
		}}
	r := &Service{
		cfg: &config{
			beaconDB:      db,
			p2p:           p,
			initialSync:   &mockSync.Sync{IsSyncing: false},
			chain:         chainService,
			blockNotifier: chainService.BlockNotifier(),
			stateGen:      stateGen,
			clock:         startup.NewClock(chainService.Genesis, chainService.ValidatorsRoot),
		},
		seenBlockCache: lruwrpr.New(10),
		badBlockCache:  lruwrpr.New(10),
	}

	buf := new(bytes.Buffer)
	_, err = p.Encoding().EncodeGossip(buf, msg)
	require.NoError(t, err)
	topic := p2p.GossipTypeMapping[reflect.TypeFor[*qrysmpb.SignedBeaconBlockZond]()]
	genesisValidatorsRoot := r.cfg.clock.GenesisValidatorsRoot()
	zondDigest, err := signing.ComputeForkDigest(params.BeaconConfig().GenesisForkVersion, genesisValidatorsRoot[:])
	require.NoError(t, err)
	topic = r.addDigestToTopic(topic, zondDigest)
	m := &pubsub.Message{
		Message: &pubsubpb.Message{
			Data:  buf.Bytes(),
			Topic: &topic,
		},
	}

	res, err := r.validateBeaconBlockPubSub(ctx, "", m)
	require.NoError(t, err)
	result := res == pubsub.ValidationAccept
	assert.Equal(t, true, result)
}

func Test_getBlockFields(t *testing.T) {
	hook := logTest.NewGlobal()

	// Nil
	log.WithFields(getBlockFields(nil)).Info("nil block")
	// Good block
	b := util.NewBeaconBlockZond()
	wb, err := blocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	log.WithFields(getBlockFields(wb)).Info("bad block")

	require.LogsContain(t, hook, "nil block")
	require.LogsContain(t, hook, "bad block")
}

func TestDetectAndBroadcastEquivocation(t *testing.T) {
	ctx := context.Background()
	p := p2ptest.NewTestP2P(t)
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)

	t.Run("no equivocation - different slot/proposer", func(t *testing.T) {
		block := util.NewBeaconBlockZond()
		block.Block.Slot = 1
		block.Block.ProposerIndex = 0
		sig, err := signing.ComputeDomainAndSign(beaconState, 0, block.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		block.Signature = sig

		headBlock := util.NewBeaconBlockZond()
		headBlock.Block.Slot = 2
		headBlock.Block.ProposerIndex = 1
		signedHeadBlock, err := blocks.NewSignedBeaconBlock(headBlock)
		require.NoError(t, err)

		slashingPool := &slashingsmock.PoolMock{}
		chainService := &mock.ChainService{
			State:   beaconState,
			Genesis: time.Now(),
			Block:   signedHeadBlock,
		}

		r := &Service{
			cfg: &config{
				p2p:          p,
				chain:        chainService,
				slashingPool: slashingPool,
			},
			seenBlockCache: lruwrpr.New(10),
		}

		signedBlock, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)

		err = r.detectAndBroadcastEquivocation(ctx, signedBlock)
		require.NoError(t, err)
		assert.Equal(t, 0, len(slashingPool.PendingPropSlashings), "Expected no slashings")
	})

	t.Run("equivocation detected", func(t *testing.T) {
		headBlock := util.NewBeaconBlockZond()
		headBlock.Block.Slot = 1
		headBlock.Block.ProposerIndex = 0
		headBlock.Block.ParentRoot = bytesutil.PadTo([]byte("parent1"), 32)
		sig1, err := signing.ComputeDomainAndSign(beaconState, 0, headBlock.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		headBlock.Signature = sig1

		newBlock := util.NewBeaconBlockZond()
		newBlock.Block.Slot = 1
		newBlock.Block.ProposerIndex = 0
		newBlock.Block.ParentRoot = bytesutil.PadTo([]byte("parent2"), 32)
		sig2, err := signing.ComputeDomainAndSign(beaconState, 0, newBlock.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		newBlock.Signature = sig2

		signedHeadBlock, err := blocks.NewSignedBeaconBlock(headBlock)
		require.NoError(t, err)

		slashingPool := &slashingsmock.PoolMock{}
		chainService := &mock.ChainService{
			State:   beaconState,
			Genesis: time.Now(),
			Block:   signedHeadBlock,
		}

		r := &Service{
			cfg: &config{
				p2p:          p,
				chain:        chainService,
				slashingPool: slashingPool,
			},
			seenBlockCache: lruwrpr.New(10),
		}

		signedNewBlock, err := blocks.NewSignedBeaconBlock(newBlock)
		require.NoError(t, err)

		err = r.detectAndBroadcastEquivocation(ctx, signedNewBlock)
		require.NoError(t, err)

		require.Equal(t, 1, len(slashingPool.PendingPropSlashings), "Expected a slashing to be inserted")
		slashing := slashingPool.PendingPropSlashings[0]
		assert.Equal(t, primitives.ValidatorIndex(0), slashing.Header_1.Header.ProposerIndex, "Wrong proposer index")
		assert.Equal(t, primitives.Slot(1), slashing.Header_1.Header.Slot, "Wrong slot")
	})

	t.Run("same signature", func(t *testing.T) {
		block := util.NewBeaconBlockZond()
		block.Block.Slot = 1
		block.Block.ProposerIndex = 0
		sig, err := signing.ComputeDomainAndSign(beaconState, 0, block.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		block.Signature = sig

		signedBlock, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)

		slashingPool := &slashingsmock.PoolMock{}
		chainService := &mock.ChainService{
			State:   beaconState,
			Genesis: time.Now(),
			Block:   signedBlock,
		}

		r := &Service{
			cfg: &config{
				p2p:          p,
				chain:        chainService,
				slashingPool: slashingPool,
			},
			seenBlockCache: lruwrpr.New(10),
		}

		err = r.detectAndBroadcastEquivocation(ctx, signedBlock)
		require.NoError(t, err)
		assert.Equal(t, 0, len(slashingPool.PendingPropSlashings), "Expected no slashings for same signature")
	})

	t.Run("head state error", func(t *testing.T) {
		block := util.NewBeaconBlockZond()
		block.Block.Slot = 1
		block.Block.ProposerIndex = 0
		block.Block.ParentRoot = bytesutil.PadTo([]byte("parent1"), 32)
		sig1, err := signing.ComputeDomainAndSign(beaconState, 0, block.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		block.Signature = sig1

		headBlock := util.NewBeaconBlockZond()
		headBlock.Block.Slot = 1
		headBlock.Block.ProposerIndex = 0
		headBlock.Block.ParentRoot = bytesutil.PadTo([]byte("parent2"), 32)
		sig2, err := signing.ComputeDomainAndSign(beaconState, 0, headBlock.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		headBlock.Signature = sig2

		signedBlock, err := blocks.NewSignedBeaconBlock(block)
		require.NoError(t, err)
		signedHeadBlock, err := blocks.NewSignedBeaconBlock(headBlock)
		require.NoError(t, err)

		chainService := &mock.ChainService{
			State:        nil,
			Block:        signedHeadBlock,
			HeadStateErr: errors.New("could not get head state"),
		}

		r := &Service{
			cfg: &config{
				p2p:          p,
				chain:        chainService,
				slashingPool: &slashingsmock.PoolMock{},
			},
			seenBlockCache: lruwrpr.New(10),
		}

		err = r.detectAndBroadcastEquivocation(ctx, signedBlock)
		require.ErrorContains(t, "could not get head state", err)
	})

	t.Run("signature verification failure", func(t *testing.T) {
		headBlock := util.NewBeaconBlockZond()
		headBlock.Block.Slot = 1
		headBlock.Block.ProposerIndex = 0
		sig1, err := signing.ComputeDomainAndSign(beaconState, 0, headBlock.Block, params.BeaconConfig().DomainBeaconProposer, privKeys[0])
		require.NoError(t, err)
		headBlock.Signature = sig1

		newBlock := util.NewBeaconBlockZond()
		newBlock.Block.Slot = 1
		newBlock.Block.ProposerIndex = 0
		newBlock.Block.ParentRoot = bytesutil.PadTo([]byte("different"), 32)
		invalidSig := make([]byte, fieldparams.MLDSA87SignatureLength)
		copy(invalidSig, []byte("invalid signature"))
		newBlock.Signature = invalidSig

		signedHeadBlock, err := blocks.NewSignedBeaconBlock(headBlock)
		require.NoError(t, err)
		signedNewBlock, err := blocks.NewSignedBeaconBlock(newBlock)
		require.NoError(t, err)

		slashingPool := &slashingsmock.PoolMock{}
		chainService := &mock.ChainService{
			State:   beaconState,
			Genesis: time.Now(),
			Block:   signedHeadBlock,
		}

		r := &Service{
			cfg: &config{
				p2p:          p,
				chain:        chainService,
				slashingPool: slashingPool,
			},
			seenBlockCache: lruwrpr.New(10),
		}

		err = r.detectAndBroadcastEquivocation(ctx, signedNewBlock)
		require.ErrorIs(t, err, ErrSlashingSignatureFailure)
	})
}
