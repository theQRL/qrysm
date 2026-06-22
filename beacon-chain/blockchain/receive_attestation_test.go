package blockchain

import (
	"context"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	forkchoicetypes "github.com/theQRL/qrysm/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	qrysmTime "github.com/theQRL/qrysm/time"
	"github.com/theQRL/qrysm/time/slots"
)

var (
	_ = AttestationReceiver(&Service{})
	_ = AttestationStateFetcher(&Service{})
)

func TestAttestationCheckPtState_FarFutureSlot(t *testing.T) {
	helpers.ClearCache()
	service, _ := minimalTestService(t)

	service.genesisTime = time.Now()

	e := primitives.Epoch(slots.MaxSlotBuffer/uint64(params.BeaconConfig().SlotsPerEpoch) + 1)
	_, err := service.AttestationTargetState(context.Background(), &qrysmpb.Checkpoint{Epoch: e})
	require.ErrorContains(t, "exceeds max allowed value relative to the local clock", err)
}

func TestVerifyLMDFFGConsistent(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	f := service.cfg.ForkChoiceStore
	fc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	state, ro128, err := prepareForkchoiceState(ctx, 128, [32]byte{'a'}, params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, fc, fc)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, ro128))
	r128 := ro128.Root()

	state, ro129, err := prepareForkchoiceState(ctx, 129, [32]byte{'b'}, r128, params.BeaconConfig().ZeroHash, fc, fc)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, ro129))
	r129 := ro129.Root()

	// FFG/LMD mismatch: claimed target differs from canonical.
	wanted := "FFG and LMD votes are not consistent"
	a := util.NewAttestation()
	a.Data.Target.Epoch = 1
	a.Data.Target.Root = []byte{'c'}
	a.Data.BeaconBlockRoot = r129[:]
	require.ErrorContains(t, wanted, service.VerifyLmdFfgConsistency(context.Background(), a))

	// Consistent: claimed target matches forkchoice TargetRootForEpoch.
	a.Data.Target.Root = r128[:]
	err = service.VerifyLmdFfgConsistency(context.Background(), a)
	require.NoError(t, err, "Could not verify LMD and FFG votes to be consistent")
}

// TestVerifyLMDFFGConsistent_TargetEpochExceedsBlockSlot is a regression test
// for the FFG/LMD consistency bug that affected Prysm before PRs #13257 / #13258.
//
// Scenario: an attester is asked to produce an attestation in a new epoch before
// any block has been produced in that epoch. Per the Ethereum spec the attester
// votes with BeaconBlockRoot == Target.Root == head_root, and Target.Epoch is the
// current (new) epoch - so Target.Epoch's start slot is strictly greater than the
// head block's slot.
//
// Prysm introduced a cached per-node target field that returned the head's own
// target (pointing at an earlier epoch's checkpoint block), which spuriously
// failed the consistency check in this edge case; #13258 fixed it by making the
// lookup epoch-aware. qrysm never adopted that cache - it still walks the chain
// via Ancestor(), which returns the input block when the requested slot is at or
// after the block's own slot. This test locks that spec-correct behavior in so
// any future refactor that caches per-block target roots will be caught.
func TestVerifyLMDFFGConsistent_TargetEpochExceedsBlockSlot(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	// Head block is the first block of epoch 1. Deliberately produce no
	// block in epoch 2 so that the requested target epoch's start slot is
	// strictly greater than the head block's slot.
	f := service.cfg.ForkChoiceStore
	fc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	state, roHead, err := prepareForkchoiceState(ctx, params.BeaconConfig().SlotsPerEpoch, [32]byte{'a'}, params.BeaconConfig().ZeroHash, params.BeaconConfig().ZeroHash, fc, fc)
	require.NoError(t, err)
	require.NoError(t, f.InsertNode(ctx, state, roHead))
	headRoot := roHead.Root()

	// Attestation produced during epoch 2 before any epoch-2 block exists:
	// spec says vote with BeaconBlockRoot == Target.Root == head_root, with
	// Target.Epoch being the current (new) epoch. TargetRootForEpoch should
	// return the head root for this case (no later canonical block exists).
	a := util.NewAttestation()
	a.Data.Target.Epoch = 2
	a.Data.Target.Root = headRoot[:]
	a.Data.BeaconBlockRoot = headRoot[:]

	err = service.VerifyLmdFfgConsistency(context.Background(), a)
	require.NoError(t, err, "Could not verify LMD and FFG votes when target epoch start slot exceeds block slot")
}

func TestProcessAttestations_Ok(t *testing.T) {
	service, tr := minimalTestService(t)
	hook := logTest.NewGlobal()
	ctx := tr.ctx

	service.genesisTime = qrysmTime.Now().Add(-1 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	genesisState, pks := util.DeterministicGenesisStateZond(t, 256)
	require.NoError(t, genesisState.SetGenesisTime(uint64(qrysmTime.Now().Unix())-params.BeaconConfig().SecondsPerSlot))
	require.NoError(t, service.saveGenesisData(ctx, genesisState))
	atts, err := util.GenerateAttestations(genesisState, pks, 1, 0, false)
	require.NoError(t, err)
	tRoot := bytesutil.ToBytes32(atts[0].Data.Target.Root)
	copied := genesisState.Copy()
	copied, err = transition.ProcessSlots(ctx, copied, 1)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, copied, tRoot))
	ofc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ojc := &qrysmpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	state, blkRoot, err := prepareForkchoiceState(ctx, 0, tRoot, tRoot, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	require.NoError(t, service.cfg.AttPool.SaveForkchoiceAttestations(atts))
	service.processAttestations(ctx, 0)
	require.Equal(t, 0, len(service.cfg.AttPool.ForkchoiceAttestations()))
	require.LogsDoNotContain(t, hook, "Could not process attestation for fork choice")
}

func TestService_ProcessAttestationsAndUpdateHead(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, fcs := tr.ctx, tr.fcs

	service.genesisTime = qrysmTime.Now().Add(-2 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	genesisState, pks := util.DeterministicGenesisStateZond(t, 64)
	require.NoError(t, service.saveGenesisData(ctx, genesisState))
	ojc := &qrysmpb.Checkpoint{Epoch: 0, Root: service.originBlockRoot[:]}
	require.NoError(t, fcs.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: 0, Root: service.originBlockRoot}))
	copied := genesisState.Copy()
	// Generate a new block for attesters to attest
	blk, err := util.GenerateFullBlockZond(copied, pks, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	tRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	preState, err := service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err := service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, tRoot, wsb, postState))
	roblock, err := blocks.NewROBlockWithRoot(wsb, tRoot)
	require.NoError(t, err)
	require.NoError(t, service.postBlockProcess(ctx, roblock, postState, false))
	copied, err = service.cfg.StateGen.StateByRoot(ctx, tRoot)
	require.NoError(t, err)
	require.Equal(t, 2, fcs.NodeCount())
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb))

	// Generate attestations for this block in Slot 1
	atts, err := util.GenerateAttestations(copied, pks, 1, 1, false)
	require.NoError(t, err)
	require.NoError(t, service.cfg.AttPool.SaveForkchoiceAttestations(atts))
	// Verify the target is in forkchoice
	require.Equal(t, true, fcs.HasNode(bytesutil.ToBytes32(atts[0].Data.BeaconBlockRoot)))
	require.Equal(t, tRoot, bytesutil.ToBytes32(atts[0].Data.BeaconBlockRoot))
	require.Equal(t, true, fcs.HasNode(service.originBlockRoot))

	// Insert a new block to forkchoice
	b, err := util.GenerateFullBlockZond(genesisState, pks, util.DefaultBlockGenConfig(), 2)
	require.NoError(t, err)
	b.Block.ParentRoot = service.originBlockRoot[:]
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b)
	state, blkRoot, err := prepareForkchoiceState(ctx, 2, r, service.originBlockRoot, [32]byte{'b'}, ojc, ojc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	require.Equal(t, 3, fcs.NodeCount())
	service.head.root = r // Old head

	require.Equal(t, 1, len(service.cfg.AttPool.ForkchoiceAttestations()))
	service.UpdateHead(ctx, 0)
	require.Equal(t, tRoot, service.headRoot())
	require.Equal(t, 0, len(service.cfg.AttPool.ForkchoiceAttestations())) // Validate att pool is empty
}

func TestService_UpdateHead_NoAtts(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, fcs := tr.ctx, tr.fcs

	service.genesisTime = qrysmTime.Now().Add(-2 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	genesisState, pks := util.DeterministicGenesisStateZond(t, 64)
	require.NoError(t, service.saveGenesisData(ctx, genesisState))
	require.NoError(t, fcs.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: 0, Root: service.originBlockRoot}))
	copied := genesisState.Copy()
	// Generate a new block
	blk, err := util.GenerateFullBlockZond(copied, pks, util.DefaultBlockGenConfig(), 1)
	require.NoError(t, err)
	tRoot, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	wsb, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)

	preState, err := service.getBlockPreState(ctx, wsb.Block())
	require.NoError(t, err)
	postState, err := service.validateStateTransition(ctx, preState, wsb)
	require.NoError(t, err)
	require.NoError(t, service.savePostStateInfo(ctx, tRoot, wsb, postState))
	roblock, err := blocks.NewROBlockWithRoot(wsb, tRoot)
	require.NoError(t, err)
	require.NoError(t, service.postBlockProcess(ctx, roblock, postState, false))
	require.Equal(t, 2, fcs.NodeCount())
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb))
	require.Equal(t, tRoot, service.head.root)

	// Insert a new block to forkchoice
	ojc := &qrysmpb.Checkpoint{Epoch: 0, Root: params.BeaconConfig().ZeroHash[:]}
	b, err := util.GenerateFullBlockZond(genesisState, pks, util.DefaultBlockGenConfig(), 2)
	require.NoError(t, err)
	b.Block.ParentRoot = service.originBlockRoot[:]
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b)
	state, blkRoot, err := prepareForkchoiceState(ctx, 2, r, service.originBlockRoot, [32]byte{'b'}, ojc, ojc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	require.Equal(t, 3, fcs.NodeCount())

	require.Equal(t, 0, service.cfg.AttPool.ForkchoiceAttestationCount())
	service.UpdateHead(ctx, 0)
	require.Equal(t, r, service.headRoot())

	require.Equal(t, 0, len(service.cfg.AttPool.ForkchoiceAttestations())) // Validate att pool is empty
}
