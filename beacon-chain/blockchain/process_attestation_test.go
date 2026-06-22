package blockchain

import (
	"context"
	"testing"
	"time"

	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestStore_OnAttestation_ErrorConditions(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx, beaconDB := tr.ctx, tr.db

	_, err := blockTree1(t, beaconDB, []byte{'g'})
	require.NoError(t, err)

	blkWithoutState := util.NewBeaconBlockZond()
	blkWithoutState.Block.Slot = 0
	util.SaveBlock(t, ctx, beaconDB, blkWithoutState)

	cp := &qrysmpb.Checkpoint{}
	st, blkRoot, err := prepareForkchoiceState(ctx, 0, [32]byte{}, [32]byte{}, params.BeaconConfig().ZeroHash, cp, cp)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))

	blkWithStateBadAtt := util.NewBeaconBlockZond()
	blkWithStateBadAtt.Block.Slot = 1
	r, err := blkWithStateBadAtt.Block.HashTreeRoot()
	require.NoError(t, err)
	cp = &qrysmpb.Checkpoint{Root: r[:]}
	st, blkRoot, err = prepareForkchoiceState(ctx, blkWithStateBadAtt.Block.Slot, r, [32]byte{}, params.BeaconConfig().ZeroHash, cp, cp)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, blkRoot))
	util.SaveBlock(t, ctx, beaconDB, blkWithStateBadAtt)
	BlkWithStateBadAttRoot, err := blkWithStateBadAtt.Block.HashTreeRoot()
	require.NoError(t, err)

	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, s.SetSlot(100*params.BeaconConfig().SlotsPerEpoch))
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, s, BlkWithStateBadAttRoot))

	blkWithValidState := util.NewBeaconBlockZond()
	blkWithValidState.Block.Slot = 128
	util.SaveBlock(t, ctx, beaconDB, blkWithValidState)

	blkWithValidStateRoot, err := blkWithValidState.Block.HashTreeRoot()
	require.NoError(t, err)
	s, err = util.NewBeaconStateZond()
	require.NoError(t, err)
	err = s.SetFork(&qrysmpb.Fork{
		Epoch:           0,
		CurrentVersion:  params.BeaconConfig().GenesisForkVersion,
		PreviousVersion: params.BeaconConfig().GenesisForkVersion,
	})
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, s, blkWithValidStateRoot))

	service.head = &head{
		state: st,
	}

	tests := []struct {
		name      string
		a         *qrysmpb.Attestation
		wantedErr string
	}{
		{
			name:      "attestation's data slot not aligned with target vote",
			a:         util.HydrateAttestation(&qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Slot: params.BeaconConfig().SlotsPerEpoch, Target: &qrysmpb.Checkpoint{Root: make([]byte, 32)}}}),
			wantedErr: "slot 128 does not match target epoch 0",
		},
		{
			name: "process attestation doesn't match current epoch",
			a: util.HydrateAttestation(&qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Slot: 100 * params.BeaconConfig().SlotsPerEpoch, Target: &qrysmpb.Checkpoint{Epoch: 100,
				Root: BlkWithStateBadAttRoot[:]}}}),
			wantedErr: "target epoch 100 does not match current epoch",
		},
		{
			name:      "process nil attestation",
			a:         nil,
			wantedErr: "attestation can't be nil",
		},
		{
			name:      "process nil field (a.Data) in attestation",
			a:         &qrysmpb.Attestation{},
			wantedErr: "attestation's data can't be nil",
		},
		{
			name: "process nil field (a.Target) in attestation",
			a: &qrysmpb.Attestation{
				Data: &qrysmpb.AttestationData{
					BeaconBlockRoot: make([]byte, fieldparams.RootLength),
					Target:          nil,
					Source:          &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
				},
				AggregationBits: make([]byte, 1),
				Signatures:      [][]byte{},
			},
			wantedErr: "attestation's target can't be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.OnAttestation(ctx, tt.a, 0)
			if tt.wantedErr != "" {
				assert.ErrorContains(t, tt.wantedErr, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStore_OnAttestation_Ok_DoublyLinkedTree(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	genesisState, pks := util.DeterministicGenesisStateZond(t, 256)
	service.SetGenesisTime(time.Unix(time.Now().Unix()-int64(params.BeaconConfig().SecondsPerSlot), 0))
	require.NoError(t, service.saveGenesisData(ctx, genesisState))
	att, err := util.GenerateAttestations(genesisState, pks, 1, 0, false)
	require.NoError(t, err)
	tRoot := bytesutil.ToBytes32(att[0].Data.Target.Root)
	copied := genesisState.Copy()
	copied, err = transition.ProcessSlots(ctx, copied, 1)
	require.NoError(t, err)
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, copied, tRoot))
	ojc := &qrysmpb.Checkpoint{Epoch: 0, Root: tRoot[:]}
	ofc := &qrysmpb.Checkpoint{Epoch: 0, Root: tRoot[:]}
	state, blkRoot, err := prepareForkchoiceState(ctx, 0, tRoot, tRoot, params.BeaconConfig().ZeroHash, ojc, ofc)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, state, blkRoot))
	require.NoError(t, service.OnAttestation(ctx, att[0], 0))
}

func TestService_GetRecentPreState(t *testing.T) {
	service, _ := minimalTestService(t)
	ctx := context.Background()

	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	ckRoot := bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)
	cp0 := &qrysmpb.Checkpoint{Epoch: 0, Root: ckRoot}
	require.NoError(t, s.SetFinalizedCheckpoint(cp0))

	headSlot := params.BeaconConfig().SlotsPerEpoch - 1 // last slot of epoch 0
	st, root, err := prepareForkchoiceState(ctx, headSlot, [32]byte(ckRoot), [32]byte{}, [32]byte{'R'}, cp0, cp0)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, root))
	service.head = &head{
		root:  [32]byte(ckRoot),
		state: s,
		slot:  headSlot,
	}
	// Requesting epoch 1 (> headEpoch 0) exercises the new fast-path that uses
	// the head state + next-slot cache instead of falling through to regen.
	require.NotNil(t, service.getRecentPreState(ctx, &qrysmpb.Checkpoint{Epoch: 1, Root: ckRoot}))
}

func TestStore_SaveCheckpointState(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	err = s.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)})
	require.NoError(t, err)
	val := &qrysmpb.Validator{
		PublicKey:             bytesutil.PadTo([]byte("foo"), 2592),
		WithdrawalCredentials: bytesutil.PadTo([]byte("bar"), fieldparams.WithdrawalCredentialsLength),
	}
	err = s.SetValidators([]*qrysmpb.Validator{val})
	require.NoError(t, err)
	err = s.SetBalances([]uint64{0})
	require.NoError(t, err)
	err = s.SetInactivityScores([]uint64{0})
	require.NoError(t, err)
	r := [32]byte{'g'}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, s, r))

	cp1 := &qrysmpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, s, bytesutil.ToBytes32([]byte{'A'})))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)}))

	st, root, err := prepareForkchoiceState(ctx, 1, [32]byte(cp1.Root), [32]byte{}, [32]byte{'R'}, cp1, cp1)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, root))
	s1, err := service.getAttPreState(ctx, cp1)
	require.NoError(t, err)
	assert.Equal(t, 1*params.BeaconConfig().SlotsPerEpoch, s1.Slot(), "Unexpected state slot")

	cp2 := &qrysmpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength)}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, s, bytesutil.ToBytes32([]byte{'B'})))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength)}))

	s2, err := service.getAttPreState(ctx, cp2)
	require.ErrorContains(t, "epoch 2 root 0x4200000000000000000000000000000000000000000000000000000000000000: not a checkpoint in forkchoice", err)

	st, root, err = prepareForkchoiceState(ctx, 33, [32]byte(cp2.Root), [32]byte(cp1.Root), [32]byte{'R'}, cp2, cp2)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, root))

	s2, err = service.getAttPreState(ctx, cp2)
	require.NoError(t, err)

	assert.Equal(t, 2*params.BeaconConfig().SlotsPerEpoch, s2.Slot(), "Unexpected state slot")

	s1, err = service.getAttPreState(ctx, cp1)
	require.NoError(t, err)
	assert.Equal(t, 1*params.BeaconConfig().SlotsPerEpoch, s1.Slot(), "Unexpected state slot")

	s1, err = service.checkpointStateCache.StateByCheckpoint(cp1)
	require.NoError(t, err)
	assert.Equal(t, 1*params.BeaconConfig().SlotsPerEpoch, s1.Slot(), "Unexpected state slot")

	s2, err = service.checkpointStateCache.StateByCheckpoint(cp2)
	require.NoError(t, err)
	assert.Equal(t, 2*params.BeaconConfig().SlotsPerEpoch, s2.Slot(), "Unexpected state slot")

	require.NoError(t, s.SetSlot(params.BeaconConfig().SlotsPerEpoch+1))
	cp3 := &qrysmpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'C'}, fieldparams.RootLength)}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, s, bytesutil.ToBytes32([]byte{'C'})))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{Root: bytesutil.PadTo([]byte{'C'}, fieldparams.RootLength)}))
	st, root, err = prepareForkchoiceState(ctx, 31, [32]byte(cp3.Root), [32]byte(cp2.Root), [32]byte{'P'}, cp2, cp2)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, root))

	s3, err := service.getAttPreState(ctx, cp3)
	require.NoError(t, err)
	assert.Equal(t, s.Slot(), s3.Slot(), "Unexpected state slot")
}

func TestStore_UpdateCheckpointState(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	baseState, _ := util.DeterministicGenesisStateZond(t, 1)

	epoch := primitives.Epoch(1)
	blk := util.NewBeaconBlockZond()
	r1, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	checkpoint := &qrysmpb.Checkpoint{Epoch: epoch, Root: r1[:]}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, baseState, bytesutil.ToBytes32(checkpoint.Root)))
	st, roblock, err := prepareForkchoiceState(ctx, blk.Block.Slot, r1, [32]byte{}, params.BeaconConfig().ZeroHash, checkpoint, checkpoint)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, roblock))
	returned, err := service.getAttPreState(ctx, checkpoint)
	require.NoError(t, err)
	assert.Equal(t, params.BeaconConfig().SlotsPerEpoch.Mul(uint64(checkpoint.Epoch)), returned.Slot(), "Incorrectly returned base state")

	cached, err := service.checkpointStateCache.StateByCheckpoint(checkpoint)
	require.NoError(t, err)
	assert.Equal(t, returned.Slot(), cached.Slot(), "State should have been cached")

	epoch = 2
	blk = util.NewBeaconBlockZond()
	blk.Block.Slot = 64
	r2, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	newCheckpoint := &qrysmpb.Checkpoint{Epoch: epoch, Root: r2[:]}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, baseState, bytesutil.ToBytes32(newCheckpoint.Root)))
	st, roblock, err = prepareForkchoiceState(ctx, blk.Block.Slot, r2, r1, params.BeaconConfig().ZeroHash, newCheckpoint, newCheckpoint)
	require.NoError(t, err)
	require.NoError(t, service.cfg.ForkChoiceStore.InsertNode(ctx, st, roblock))
	returned, err = service.getAttPreState(ctx, newCheckpoint)
	require.NoError(t, err)
	s, err := slots.EpochStart(newCheckpoint.Epoch)
	require.NoError(t, err)
	baseState, err = transition.ProcessSlots(ctx, baseState, s)
	require.NoError(t, err)
	assert.Equal(t, returned.Slot(), baseState.Slot(), "Incorrectly returned base state")

	cached, err = service.checkpointStateCache.StateByCheckpoint(newCheckpoint)
	require.NoError(t, err)
	require.DeepSSZEqual(t, returned.ToProtoUnsafe(), cached.ToProtoUnsafe())
}

func TestUpdateFinalized_EvictsCheckpointStateCache(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx

	currentFinalizedBeaconBlock := util.NewBeaconBlockZond()
	currentFinalizedBeaconBlock.Block.Slot = params.BeaconConfig().SlotsPerEpoch
	currentFinalizedBlock := util.SaveBlock(t, ctx, service.cfg.BeaconDB, currentFinalizedBeaconBlock)
	currentFinalizedRoot, err := currentFinalizedBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	currentFinalized := &qrysmpb.Checkpoint{
		Epoch: 1,
		Root:  currentFinalizedRoot[:],
	}
	require.NoError(t, service.cfg.BeaconDB.SaveGenesisBlockRoot(ctx, bytesutil.ToBytes32(currentFinalized.Root)))
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{
		Root: currentFinalized.Root,
		Slot: params.BeaconConfig().SlotsPerEpoch,
	}))
	require.NoError(t, service.cfg.BeaconDB.SaveFinalizedCheckpoint(ctx, currentFinalized))

	cp1 := &qrysmpb.Checkpoint{Epoch: 1, Root: bytesutil.PadTo([]byte{'A'}, fieldparams.RootLength)}
	st1, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st1.SetSlot(params.BeaconConfig().SlotsPerEpoch))
	require.NoError(t, service.checkpointStateCache.AddCheckpointState(cp1, st1))

	cp2 := &qrysmpb.Checkpoint{Epoch: 2, Root: bytesutil.PadTo([]byte{'B'}, fieldparams.RootLength)}
	st2, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st2.SetSlot(params.BeaconConfig().SlotsPerEpoch.Mul(2)))
	require.NoError(t, service.checkpointStateCache.AddCheckpointState(cp2, st2))

	cp5 := &qrysmpb.Checkpoint{Epoch: 5, Root: bytesutil.PadTo([]byte{'C'}, fieldparams.RootLength)}
	st5, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, st5.SetSlot(params.BeaconConfig().SlotsPerEpoch.Mul(5)))
	require.NoError(t, service.checkpointStateCache.AddCheckpointState(cp5, st5))

	newFinalizedBeaconBlock := util.NewBeaconBlockZond()
	newFinalizedBeaconBlock.Block.Slot = params.BeaconConfig().SlotsPerEpoch.Mul(3)
	newFinalizedBeaconBlock.Block.ParentRoot = currentFinalized.Root
	newFinalizedBlock := util.SaveBlock(t, ctx, service.cfg.BeaconDB, newFinalizedBeaconBlock)
	newFinalizedRoot, err := newFinalizedBlock.Block().HashTreeRoot()
	require.NoError(t, err)
	newFinalized := &qrysmpb.Checkpoint{Epoch: 3, Root: newFinalizedRoot[:]}
	require.NoError(t, service.cfg.BeaconDB.SaveStateSummary(ctx, &qrysmpb.StateSummary{
		Root: newFinalized.Root,
		Slot: params.BeaconConfig().SlotsPerEpoch.Mul(3),
	}))

	require.NoError(t, service.updateFinalized(ctx, newFinalized))

	cached, err := service.checkpointStateCache.StateByCheckpoint(cp1)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), cached)

	cached, err = service.checkpointStateCache.StateByCheckpoint(cp2)
	require.NoError(t, err)
	assert.Equal(t, state.BeaconState(nil), cached)

	cached, err = service.checkpointStateCache.StateByCheckpoint(cp5)
	require.NoError(t, err)
	assert.NotNil(t, cached)
	assert.Equal(t, st5.Slot(), cached.Slot())
}

func TestAttEpoch_MatchPrevEpoch(t *testing.T) {
	ctx := context.Background()

	nowTime := uint64(params.BeaconConfig().SlotsPerEpoch) * params.BeaconConfig().SecondsPerSlot
	require.NoError(t, verifyAttTargetEpoch(ctx, 0, nowTime, &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)}))
}

func TestAttEpoch_MatchCurrentEpoch(t *testing.T) {
	ctx := context.Background()

	nowTime := uint64(params.BeaconConfig().SlotsPerEpoch) * params.BeaconConfig().SecondsPerSlot
	require.NoError(t, verifyAttTargetEpoch(ctx, 0, nowTime, &qrysmpb.Checkpoint{Epoch: 1}))
}

func TestAttEpoch_NotMatch(t *testing.T) {
	ctx := context.Background()

	nowTime := 2 * uint64(params.BeaconConfig().SlotsPerEpoch) * params.BeaconConfig().SecondsPerSlot
	err := verifyAttTargetEpoch(ctx, 0, nowTime, &qrysmpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)})
	assert.ErrorContains(t, "target epoch 0 does not match current epoch 2 or prev epoch 1", err)
}

func TestVerifyBeaconBlock_NoBlock(t *testing.T) {
	ctx := context.Background()
	opts := testServiceOptsWithDB(t)
	service, err := NewService(ctx, opts...)
	require.NoError(t, err)

	d := util.HydrateAttestationData(&qrysmpb.AttestationData{})
	require.Equal(t, errBlockNotFoundInCacheOrDB, service.verifyBeaconBlock(ctx, d))
}

func TestVerifyBeaconBlock_futureBlock(t *testing.T) {
	ctx := context.Background()

	opts := testServiceOptsWithDB(t)
	service, err := NewService(ctx, opts...)
	require.NoError(t, err)

	b := util.NewBeaconBlockZond()
	b.Block.Slot = 2
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	d := &qrysmpb.AttestationData{Slot: 1, BeaconBlockRoot: r[:]}

	assert.ErrorContains(t, "could not process attestation for future block", service.verifyBeaconBlock(ctx, d))
}

func TestVerifyBeaconBlock_OK(t *testing.T) {
	ctx := context.Background()

	opts := testServiceOptsWithDB(t)
	service, err := NewService(ctx, opts...)
	require.NoError(t, err)

	b := util.NewBeaconBlockZond()
	b.Block.Slot = 2
	util.SaveBlock(t, ctx, service.cfg.BeaconDB, b)
	r, err := b.Block.HashTreeRoot()
	require.NoError(t, err)
	d := &qrysmpb.AttestationData{Slot: 2, BeaconBlockRoot: r[:]}

	assert.NoError(t, service.verifyBeaconBlock(ctx, d), "Did not receive the wanted error")
}

func TestGetAttPreState_HeadState(t *testing.T) {
	service, tr := minimalTestService(t)
	ctx := tr.ctx
	baseState, _ := util.DeterministicGenesisStateZond(t, 1)

	epoch := primitives.Epoch(1)
	blk := util.NewBeaconBlockZond()
	r1, err := blk.Block.HashTreeRoot()
	require.NoError(t, err)
	checkpoint := &qrysmpb.Checkpoint{Epoch: epoch, Root: r1[:]}
	require.NoError(t, service.cfg.BeaconDB.SaveState(ctx, baseState, bytesutil.ToBytes32(checkpoint.Root)))
	require.NoError(t, transition.UpdateNextSlotCache(ctx, checkpoint.Root, baseState))
	_, err = service.getAttPreState(ctx, checkpoint)
	require.NoError(t, err)
	st, err := service.checkpointStateCache.StateByCheckpoint(checkpoint)
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SlotsPerEpoch, st.Slot())
}
