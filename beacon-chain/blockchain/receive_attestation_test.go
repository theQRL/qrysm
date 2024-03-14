package blockchain

import (
	"context"
	"testing"
	"time"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/transition"
	forkchoicetypes "github.com/theQRL/qrysm/v4/beacon-chain/forkchoice/types"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	qrysmTime "github.com/theQRL/qrysm/v4/time"
	"github.com/theQRL/qrysm/v4/time/slots"
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
	_, err := service.AttestationTargetState(context.Background(), &zondpb.Checkpoint{Epoch: e})
	require.ErrorContains(t, "exceeds max allowed value relative to the local clock", err)
}

func TestVerifyLMDFFGConsistent_NotOK(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	b128 := util.NewBeaconBlockCapella()
	b128.Block.Slot = 128
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b128)
	r128, err := b128.Block.HashTreeRoot()
	require.NoError(t, err)
	b129 := util.NewBeaconBlockCapella()
	b129.Block.Slot = 129
	b129.Block.ParentRoot = r128[:]
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b129)
	r129, err := b129.Block.HashTreeRoot()
	require.NoError(t, err)

	wanted := "FFG and LMD votes are not consistent"
	a := util.NewAttestation()
	a.Data.Target.Epoch = 1
	a.Data.Target.Root = []byte{'a'}
	a.Data.BeaconBlockRoot = r129[:]
	require.ErrorContains(t, wanted, service.VerifyLmdFfgConsistency(context.Background(), a))
}

func TestVerifyLMDFFGConsistent_OK(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	b128 := util.NewBeaconBlockCapella()
	b128.Block.Slot = 128
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b128)
	r128, err := b128.Block.HashTreeRoot()
	require.NoError(t, err)
	b129 := util.NewBeaconBlockCapella()
	b129.Block.Slot = 129
	b129.Block.ParentRoot = r128[:]
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b129)
	r129, err := b129.Block.HashTreeRoot()
	require.NoError(t, err)

	a := util.NewAttestation()
	a.Data.Target.Epoch = 1
	a.Data.Target.Root = r128[:]
	a.Data.BeaconBlockRoot = r129[:]
	err = service.VerifyLmdFfgConsistency(context.Background(), a)
	require.NoError(t, err, "Could not verify LMD and FFG votes to be consistent")
}

func TestProcessAttestations_Ok(t *testing.T) {
	service, tr := minimalTestService(t)
	hook := logTest.NewGlobal()
	ctx := tr.ctx

	service.genesisTime = qrysmTime.Now().Add(-1 * time.Duration(params.BeaconConfig().SecondsPerSlot) * time.Second)
	genesisState, pks := util.DeterministicGenesisStateCapella(t, 256)
	require.NoError(t, genesisState.SetGenesisTime(uint64(qrysmTime.Now().Unix())-params.BeaconConfig().SecondsPerSlot))
	require.NoError(t, service.saveGenesisData(ctx, genesisState))
	atts, err := util.GenerateAttestations(genesisState, pks, 1, 0, false)
	require.NoError(t, err)
	tRoot := bytesutil.ToBytes32(atts[0].Data.Target.Root)
	copied := genesisState.Copy()
	copied, err = transition.ProcessSlots(ctx, copied, 1)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, copied, tRoot))
	ofc := &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
	ojc := &zondpb.Checkpoint{Root: params.BeaconConfig().ZeroHash[:]}
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
	genesisState, pks := util.DeterministicGenesisStateCapella(t, 64)
	require.NoError(t, service.saveGenesisData(ctx, genesisState))
	ojc := &zondpb.Checkpoint{Epoch: 0, Root: service.originBlockRoot[:]}
	require.NoError(t, fcs.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: 0, Root: service.originBlockRoot}))
	copied := genesisState.Copy()
	// Generate a new block for attesters to attest
	blk, err := util.GenerateFullBlockCapella(copied, pks, util.DefaultBlockGenConfig(), 1)
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
	require.NoError(t, service.postBlockProcess(ctx, wsb, tRoot, postState, false))
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
	b, err := util.GenerateFullBlockCapella(genesisState, pks, util.DefaultBlockGenConfig(), 2)
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
	genesisState, pks := util.DeterministicGenesisStateCapella(t, 64)
	require.NoError(t, service.saveGenesisData(ctx, genesisState))
	require.NoError(t, fcs.UpdateJustifiedCheckpoint(ctx, &forkchoicetypes.Checkpoint{Epoch: 0, Root: service.originBlockRoot}))
	copied := genesisState.Copy()
	// Generate a new block
	blk, err := util.GenerateFullBlockCapella(copied, pks, util.DefaultBlockGenConfig(), 1)
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
	require.NoError(t, service.postBlockProcess(ctx, wsb, tRoot, postState, false))
	require.Equal(t, 2, fcs.NodeCount())
	require.NoError(t, service.cfg.BeaconDB.SaveBlock(ctx, wsb))
	require.Equal(t, tRoot, service.head.root)

	// Insert a new block to forkchoice
	ojc := &zondpb.Checkpoint{Epoch: 0, Root: params.BeaconConfig().ZeroHash[:]}
	b, err := util.GenerateFullBlockCapella(genesisState, pks, util.DefaultBlockGenConfig(), 2)
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
