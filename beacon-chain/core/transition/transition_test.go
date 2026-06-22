package transition_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/params"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func init() {
	transition.SkipSlotCache.Disable()
}

func TestExecuteStateTransition_IncorrectSlot(t *testing.T) {
	base := &qrysmpb.BeaconStateZond{
		Slot: 5,
	}
	beaconState, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)
	block := &qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			Slot: 4,
			Body: &qrysmpb.BeaconBlockBodyZond{},
		},
	}
	want := "expected state.slot"
	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = transition.ExecuteStateTransition(context.Background(), beaconState, wsb)
	assert.ErrorContains(t, want, err)
}

func TestExecuteStateTransition_FullProcess(t *testing.T) {
	beaconState, privKeys := util.DeterministicGenesisStateZond(t, 100)

	executionData := &qrysmpb.ExecutionData{
		DepositCount: 100,
		DepositRoot:  bytesutil.PadTo([]byte{2}, 32),
		BlockHash:    make([]byte, 32),
	}
	require.NoError(t, beaconState.SetSlot(params.BeaconConfig().SlotsPerEpoch-1))
	e := beaconState.ExecutionData()
	e.DepositCount = 100
	require.NoError(t, beaconState.SetExecutionData(e))
	bh := beaconState.LatestBlockHeader()
	bh.Slot = beaconState.Slot()
	require.NoError(t, beaconState.SetLatestBlockHeader(bh))
	require.NoError(t, beaconState.SetExecutionDataVotes([]*qrysmpb.ExecutionData{executionData}))

	oldMix, err := beaconState.RandaoMixAtIndex(1)
	require.NoError(t, err)

	require.NoError(t, beaconState.SetSlot(beaconState.Slot()+1))
	epoch := time.CurrentEpoch(beaconState)
	randaoReveal, err := util.RandaoReveal(beaconState, epoch, privKeys)
	require.NoError(t, err)
	require.NoError(t, beaconState.SetSlot(beaconState.Slot()-1))

	nextSlotState, err := transition.ProcessSlots(context.Background(), beaconState.Copy(), beaconState.Slot()+1)
	require.NoError(t, err)
	parentRoot, err := nextSlotState.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	proposerIdx, err := helpers.BeaconProposerIndex(context.Background(), nextSlotState)
	require.NoError(t, err)
	block := util.NewBeaconBlockZond()
	block.Block.ProposerIndex = proposerIdx
	block.Block.Slot = beaconState.Slot() + 1
	block.Block.ParentRoot = parentRoot[:]
	block.Block.Body.RandaoReveal = randaoReveal
	block.Block.Body.ExecutionData = executionData

	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	stateRoot, err := transition.CalculateStateRoot(context.Background(), beaconState, wsb)
	require.NoError(t, err)

	block.Block.StateRoot = stateRoot[:]

	sig, err := util.BlockSignature(beaconState, block.Block, privKeys)
	require.NoError(t, err)
	block.Signature = sig.Marshal()

	wsb, err = consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	beaconState, err = transition.ExecuteStateTransition(context.Background(), beaconState, wsb)
	require.NoError(t, err)

	assert.Equal(t, params.BeaconConfig().SlotsPerEpoch, beaconState.Slot(), "Unexpected Slot number")

	mix, err := beaconState.RandaoMixAtIndex(1)
	require.NoError(t, err)
	assert.DeepNotEqual(t, oldMix, mix, "Did not expect new and old randao mix to equal")
}

func TestProcessBlock_IncorrectProcessExits(t *testing.T) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, 100)

	proposerSlashings := []*qrysmpb.ProposerSlashing{
		{
			Header_1: util.HydrateSignedBeaconHeader(&qrysmpb.SignedBeaconBlockHeader{
				Header: &qrysmpb.BeaconBlockHeader{
					ProposerIndex: 3,
					Slot:          1,
				},
				Signature: bytesutil.PadTo([]byte("A"), 96),
			}),
			Header_2: util.HydrateSignedBeaconHeader(&qrysmpb.SignedBeaconBlockHeader{
				Header: &qrysmpb.BeaconBlockHeader{
					ProposerIndex: 3,
					Slot:          1,
				},
				Signature: bytesutil.PadTo([]byte("B"), 96),
			}),
		},
	}
	attesterSlashings := []*qrysmpb.AttesterSlashing{
		{
			Attestation_1: &qrysmpb.IndexedAttestation{
				Data:             util.HydrateAttestationData(&qrysmpb.AttestationData{}),
				AttestingIndices: []uint64{0, 1},
				Signatures:       [][]byte{make([]byte, 4627), make([]byte, 4627)},
			},
			Attestation_2: &qrysmpb.IndexedAttestation{
				Data:             util.HydrateAttestationData(&qrysmpb.AttestationData{}),
				AttestingIndices: []uint64{0, 1},
				Signatures:       [][]byte{make([]byte, 4627), make([]byte, 4627)},
			},
		},
	}
	var blockRoots [][]byte
	for i := uint64(0); i < uint64(params.BeaconConfig().SlotsPerHistoricalRoot); i++ {
		blockRoots = append(blockRoots, []byte{byte(i)})
	}
	require.NoError(t, beaconState.SetBlockRoots(blockRoots))
	blockAtt := util.HydrateAttestation(&qrysmpb.Attestation{
		Data: &qrysmpb.AttestationData{
			Target: &qrysmpb.Checkpoint{Root: bytesutil.PadTo([]byte("hello-world"), 32)},
		},
		AggregationBits: bitfield.Bitlist{0xC0, 0xC0, 0xC0, 0xC0, 0x01},
	})
	attestations := []*qrysmpb.Attestation{blockAtt}
	var exits []*qrysmpb.SignedVoluntaryExit
	for i := uint64(0); i < params.BeaconConfig().MaxVoluntaryExits+1; i++ {
		exits = append(exits, &qrysmpb.SignedVoluntaryExit{})
	}
	genesisBlock := blocks.NewGenesisBlock([]byte{})
	bodyRoot, err := genesisBlock.Block.HashTreeRoot()
	require.NoError(t, err)
	err = beaconState.SetLatestBlockHeader(util.HydrateBeaconHeader(&qrysmpb.BeaconBlockHeader{
		Slot:       genesisBlock.Block.Slot,
		ParentRoot: genesisBlock.Block.ParentRoot,
		BodyRoot:   bodyRoot[:],
	}))
	require.NoError(t, err)
	parentRoot, err := beaconState.LatestBlockHeader().HashTreeRoot()
	require.NoError(t, err)
	block := util.NewBeaconBlockZond()
	block.Block.Slot = 1
	block.Block.ParentRoot = parentRoot[:]
	block.Block.Body.ProposerSlashings = proposerSlashings
	block.Block.Body.Attestations = attestations
	block.Block.Body.AttesterSlashings = attesterSlashings
	block.Block.Body.VoluntaryExits = exits
	block.Block.Body.ExecutionData.DepositRoot = bytesutil.PadTo([]byte{2}, 32)
	block.Block.Body.ExecutionData.BlockHash = bytesutil.PadTo([]byte{3}, 32)
	err = beaconState.SetSlot(beaconState.Slot() + params.BeaconConfig().MinAttestationInclusionDelay)
	require.NoError(t, err)
	cp := beaconState.CurrentJustifiedCheckpoint()
	cp.Root = []byte("hello-world")
	require.NoError(t, beaconState.SetCurrentJustifiedCheckpoint(cp))
	wsb, err := consensusblocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), beaconState, wsb)
	wanted := "number of voluntary exits (17) in block body exceeds allowed threshold of 16"
	assert.ErrorContains(t, wanted, err)
}

func TestProcessBlock_OverMaxProposerSlashings(t *testing.T) {
	maxSlashings := params.BeaconConfig().MaxProposerSlashings
	b := &qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			Body: &qrysmpb.BeaconBlockBodyZond{
				ProposerSlashings: make([]*qrysmpb.ProposerSlashing, maxSlashings+1),
			},
		},
	}
	want := fmt.Sprintf("number of proposer slashings (%d) in block body exceeds allowed threshold of %d",
		len(b.Block.Body.ProposerSlashings), params.BeaconConfig().MaxProposerSlashings)
	s, err := state_native.InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), s, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessBlock_OverMaxAttesterSlashings(t *testing.T) {
	maxSlashings := params.BeaconConfig().MaxAttesterSlashings
	b := &qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			Body: &qrysmpb.BeaconBlockBodyZond{
				AttesterSlashings: make([]*qrysmpb.AttesterSlashing, maxSlashings+1),
			},
		},
	}
	want := fmt.Sprintf("number of attester slashings (%d) in block body exceeds allowed threshold of %d",
		len(b.Block.Body.AttesterSlashings), params.BeaconConfig().MaxAttesterSlashings)
	s, err := state_native.InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), s, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessBlock_OverMaxAttestations(t *testing.T) {
	b := &qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			Body: &qrysmpb.BeaconBlockBodyZond{
				Attestations: make([]*qrysmpb.Attestation, params.BeaconConfig().MaxAttestations+1),
			},
		},
	}
	want := fmt.Sprintf("number of attestations (%d) in block body exceeds allowed threshold of %d",
		len(b.Block.Body.Attestations), params.BeaconConfig().MaxAttestations)
	s, err := state_native.InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), s, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessBlock_OverMaxVoluntaryExits(t *testing.T) {
	maxExits := params.BeaconConfig().MaxVoluntaryExits
	b := &qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			Body: &qrysmpb.BeaconBlockBodyZond{
				VoluntaryExits: make([]*qrysmpb.SignedVoluntaryExit, maxExits+1),
			},
		},
	}
	want := fmt.Sprintf("number of voluntary exits (%d) in block body exceeds allowed threshold of %d",
		len(b.Block.Body.VoluntaryExits), maxExits)
	s, err := state_native.InitializeFromProtoUnsafeZond(&qrysmpb.BeaconStateZond{})
	require.NoError(t, err)
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), s, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessBlock_IncorrectDeposits(t *testing.T) {
	base := &qrysmpb.BeaconStateZond{
		ExecutionData:         &qrysmpb.ExecutionData{DepositCount: 100},
		ExecutionDepositIndex: 98,
	}
	s, err := state_native.InitializeFromProtoZond(base)
	require.NoError(t, err)
	b := &qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			Body: &qrysmpb.BeaconBlockBodyZond{
				Deposits: []*qrysmpb.Deposit{{}},
			},
		},
	}
	want := fmt.Sprintf("incorrect outstanding deposits in block body, wanted: %d, got: %d",
		s.ExecutionData().DepositCount-s.ExecutionDepositIndex(), len(b.Block.Body.Deposits))
	wsb, err := consensusblocks.NewSignedBeaconBlock(b)
	require.NoError(t, err)
	_, err = transition.VerifyOperationLengths(context.Background(), s, wsb)
	assert.ErrorContains(t, want, err)
}

func TestProcessSlots_SameSlotAsParentState(t *testing.T) {
	slot := primitives.Slot(2)
	parentState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Slot: slot})
	require.NoError(t, err)

	_, err = transition.ProcessSlots(context.Background(), parentState, slot)
	assert.ErrorContains(t, "expected state.slot 2 < slot 2", err)
}

func TestProcessSlots_LowerSlotAsParentState(t *testing.T) {
	slot := primitives.Slot(2)
	parentState, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{Slot: slot})
	require.NoError(t, err)

	_, err = transition.ProcessSlots(context.Background(), parentState, slot-1)
	assert.ErrorContains(t, "expected state.slot 2 < slot 1", err)
}

// NOTE(rgeraldes24): test is not valid atm: re-enable once we have more forks
/*
func TestProcessSlots_ThroughAltairEpoch(t *testing.T) {
	transition.SkipSlotCache.Disable()
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig()
	conf.AltairForkEpoch = 5
	params.OverrideBeaconConfig(conf)

	st, _ := util.DeterministicGenesisState(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	st, err := transition.ProcessSlots(context.Background(), st, params.BeaconConfig().SlotsPerEpoch*10)
	require.NoError(t, err)
	require.Equal(t, version.Altair, st.Version())

	require.Equal(t, params.BeaconConfig().SlotsPerEpoch*10, st.Slot())

	s, err := st.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(s)))

	p, err := st.PreviousEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	p, err = st.CurrentEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	sc, err := st.CurrentSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))

	sc, err = st.NextSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))
}

func TestProcessSlots_ThroughBellatrixEpoch(t *testing.T) {
	transition.SkipSlotCache.Disable()
	params.SetupTestConfigCleanup(t)
	conf := params.BeaconConfig()
	conf.BellatrixForkEpoch = 5
	params.OverrideBeaconConfig(conf)

	st, _ := util.DeterministicGenesisStateAltair(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	st, err := transition.ProcessSlots(context.Background(), st, params.BeaconConfig().SlotsPerEpoch*10)
	require.NoError(t, err)
	require.Equal(t, version.Bellatrix, st.Version())

	require.Equal(t, params.BeaconConfig().SlotsPerEpoch*10, st.Slot())
}
*/

func TestProcessSlots_OnlyZondEpoch(t *testing.T) {
	transition.SkipSlotCache.Disable()
	params.SetupTestConfigCleanup(t)

	st, _ := util.DeterministicGenesisStateZond(t, params.BeaconConfig().MaxValidatorsPerCommittee)
	require.NoError(t, st.SetSlot(params.BeaconConfig().SlotsPerEpoch*6))
	require.Equal(t, version.Zond, st.Version())
	st, err := transition.ProcessSlots(context.Background(), st, params.BeaconConfig().SlotsPerEpoch*10)
	require.NoError(t, err)
	require.Equal(t, version.Zond, st.Version())

	require.Equal(t, params.BeaconConfig().SlotsPerEpoch*10, st.Slot())

	s, err := st.InactivityScores()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(s)))

	p, err := st.PreviousEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	p, err = st.CurrentEpochParticipation()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().MaxValidatorsPerCommittee, uint64(len(p)))

	sc, err := st.CurrentSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))

	sc, err = st.NextSyncCommittee()
	require.NoError(t, err)
	require.Equal(t, params.BeaconConfig().SyncCommitteeSize, uint64(len(sc.Pubkeys)))
}

func TestProcessSlotsUsingNextSlotCache(t *testing.T) {
	s, _ := util.DeterministicGenesisStateZond(t, 1)
	r := []byte{'a'}
	s, err := transition.ProcessSlotsUsingNextSlotCache(context.Background(), s, r, 5)
	require.NoError(t, err)
	require.Equal(t, primitives.Slot(5), s.Slot())
}

func TestProcessSlotsConditionally(t *testing.T) {
	ctx := context.Background()
	s, _ := util.DeterministicGenesisStateZond(t, 1)

	t.Run("target slot below current slot", func(t *testing.T) {
		require.NoError(t, s.SetSlot(5))
		s, err := transition.ProcessSlotsIfPossible(ctx, s, 4)
		require.NoError(t, err)
		assert.Equal(t, primitives.Slot(5), s.Slot())
	})

	t.Run("target slot equal current slot", func(t *testing.T) {
		require.NoError(t, s.SetSlot(5))
		s, err := transition.ProcessSlotsIfPossible(ctx, s, 5)
		require.NoError(t, err)
		assert.Equal(t, primitives.Slot(5), s.Slot())
	})

	t.Run("target slot above current slot", func(t *testing.T) {
		require.NoError(t, s.SetSlot(5))
		s, err := transition.ProcessSlotsIfPossible(ctx, s, 6)
		require.NoError(t, err)
		assert.Equal(t, primitives.Slot(6), s.Slot())
	})
}
