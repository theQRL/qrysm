package precompute_test

import (
	"context"
	"testing"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/epoch/precompute"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestProcessJustificationAndFinalizationPreCompute_ConsecutiveEpochs(t *testing.T) {
	e := params.BeaconConfig().FarFutureEpoch
	a := params.BeaconConfig().MaxEffectiveBalance
	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerEpoch*2+1)
	for i := 0; i < len(blockRoots); i++ {
		blockRoots[i] = []byte{byte(i)}
	}
	base := &zondpb.BeaconStateCapella{
		Slot: params.BeaconConfig().SlotsPerEpoch*2 + 1,
		PreviousJustifiedCheckpoint: &zondpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		CurrentJustifiedCheckpoint: &zondpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		FinalizedCheckpoint: &zondpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		JustificationBits:   bitfield.Bitvector4{0x0F}, // 0b1111
		Validators:          []*zondpb.Validator{{ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}},
		Balances:            []uint64{a, a, a, a}, // validator total balance should be 128000000000
		BlockRoots:          blockRoots,
	}
	state, err := state_native.InitializeFromProtoCapella(base)
	require.NoError(t, err)
	attestedBalance := 4 * uint64(e) * 3 / 2
	b := &precompute.Balance{PrevEpochTargetAttested: attestedBalance}
	newState, err := precompute.ProcessJustificationAndFinalizationPreCompute(state, b)
	require.NoError(t, err)
	rt := [32]byte{}
	assert.DeepEqual(t, rt[:], newState.CurrentJustifiedCheckpoint().Root, "Unexpected justified root")
	assert.Equal(t, primitives.Epoch(2), newState.CurrentJustifiedCheckpoint().Epoch, "Unexpected justified epoch")
	assert.Equal(t, primitives.Epoch(0), newState.PreviousJustifiedCheckpoint().Epoch, "Unexpected previous justified epoch")
	assert.DeepEqual(t, params.BeaconConfig().ZeroHash[:], newState.FinalizedCheckpoint().Root, "Unexpected finalized root")
	assert.Equal(t, primitives.Epoch(0), newState.FinalizedCheckpointEpoch(), "Unexpected finalized epoch")
}

func TestProcessJustificationAndFinalizationPreCompute_JustifyCurrentEpoch(t *testing.T) {
	e := params.BeaconConfig().FarFutureEpoch
	a := params.BeaconConfig().MaxEffectiveBalance
	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerEpoch*2+1)
	for i := 0; i < len(blockRoots); i++ {
		blockRoots[i] = []byte{byte(i)}
	}
	base := &zondpb.BeaconStateCapella{
		Slot: params.BeaconConfig().SlotsPerEpoch*2 + 1,
		PreviousJustifiedCheckpoint: &zondpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		CurrentJustifiedCheckpoint: &zondpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		FinalizedCheckpoint: &zondpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
		JustificationBits:   bitfield.Bitvector4{0x03}, // 0b0011
		Validators:          []*zondpb.Validator{{ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}},
		Balances:            []uint64{a, a, a, a}, // validator total balance should be 128000000000
		BlockRoots:          blockRoots,
	}
	state, err := state_native.InitializeFromProtoCapella(base)
	require.NoError(t, err)
	attestedBalance := 4 * uint64(e) * 3 / 2
	b := &precompute.Balance{PrevEpochTargetAttested: attestedBalance}
	newState, err := precompute.ProcessJustificationAndFinalizationPreCompute(state, b)
	require.NoError(t, err)
	rt := [32]byte{}
	assert.DeepEqual(t, rt[:], newState.CurrentJustifiedCheckpoint().Root, "Unexpected current justified root")
	assert.Equal(t, primitives.Epoch(2), newState.CurrentJustifiedCheckpoint().Epoch, "Unexpected justified epoch")
	assert.Equal(t, primitives.Epoch(0), newState.PreviousJustifiedCheckpoint().Epoch, "Unexpected previous justified epoch")
	assert.DeepEqual(t, params.BeaconConfig().ZeroHash[:], newState.FinalizedCheckpoint().Root)
	assert.Equal(t, primitives.Epoch(0), newState.FinalizedCheckpointEpoch(), "Unexpected finalized epoch")
}

func TestProcessJustificationAndFinalizationPreCompute_JustifyPrevEpoch(t *testing.T) {
	e := params.BeaconConfig().FarFutureEpoch
	a := params.BeaconConfig().MaxEffectiveBalance
	blockRoots := make([][]byte, params.BeaconConfig().SlotsPerEpoch*2+1)
	for i := 0; i < len(blockRoots); i++ {
		blockRoots[i] = []byte{byte(i)}
	}
	base := &zondpb.BeaconStateCapella{
		Slot: params.BeaconConfig().SlotsPerEpoch*2 + 1,
		PreviousJustifiedCheckpoint: &zondpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		CurrentJustifiedCheckpoint: &zondpb.Checkpoint{
			Epoch: 0,
			Root:  params.BeaconConfig().ZeroHash[:],
		},
		JustificationBits: bitfield.Bitvector4{0x03}, // 0b0011
		Validators:        []*zondpb.Validator{{ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}, {ExitEpoch: e}},
		Balances:          []uint64{a, a, a, a}, // validator total balance should be 128000000000
		BlockRoots:        blockRoots, FinalizedCheckpoint: &zondpb.Checkpoint{Root: make([]byte, fieldparams.RootLength)},
	}
	state, err := state_native.InitializeFromProtoCapella(base)
	require.NoError(t, err)
	attestedBalance := 4 * uint64(e) * 3 / 2
	b := &precompute.Balance{PrevEpochTargetAttested: attestedBalance}
	newState, err := precompute.ProcessJustificationAndFinalizationPreCompute(state, b)
	require.NoError(t, err)
	rt := [32]byte{}
	assert.DeepEqual(t, rt[:], newState.CurrentJustifiedCheckpoint().Root, "Unexpected current justified root")
	assert.Equal(t, primitives.Epoch(0), newState.PreviousJustifiedCheckpoint().Epoch, "Unexpected previous justified epoch")
	assert.Equal(t, primitives.Epoch(2), newState.CurrentJustifiedCheckpoint().Epoch, "Unexpected justified epoch")
	assert.DeepEqual(t, params.BeaconConfig().ZeroHash[:], newState.FinalizedCheckpoint().Root)
	assert.Equal(t, primitives.Epoch(0), newState.FinalizedCheckpointEpoch(), "Unexpected finalized epoch")
}

func TestUnrealizedCheckpoints(t *testing.T) {
	validators := make([]*zondpb.Validator, params.BeaconConfig().MinGenesisActiveValidatorCount)
	balances := make([]uint64, len(validators))
	for i := 0; i < len(validators); i++ {
		validators[i] = &zondpb.Validator{
			ExitEpoch:        params.BeaconConfig().FarFutureEpoch,
			EffectiveBalance: params.BeaconConfig().MaxEffectiveBalance,
		}
		balances[i] = params.BeaconConfig().MaxEffectiveBalance
	}
	pjr := [32]byte{'p'}
	cjr := [32]byte{'c'}
	je := primitives.Epoch(3)
	fe := primitives.Epoch(2)
	pjcp := &zondpb.Checkpoint{Root: pjr[:], Epoch: fe}
	cjcp := &zondpb.Checkpoint{Root: cjr[:], Epoch: je}
	fcp := &zondpb.Checkpoint{Root: pjr[:], Epoch: fe}
	tests := []struct {
		name                                 string
		slot                                 primitives.Slot
		prevVals, currVals                   int
		expectedJustified, expectedFinalized primitives.Epoch // The expected unrealized checkpoint epochs
	}{
		{
			"Not enough votes, keep previous justification",
			513,
			len(validators) / 3,
			len(validators) / 3,
			je,
			fe,
		},
		{
			"Not enough votes, keep previous justification, N+2",
			641,
			len(validators) / 3,
			len(validators) / 3,
			je,
			fe,
		},
		{
			"Enough to justify previous epoch but not current",
			513,
			2*len(validators)/3 + 3,
			len(validators) / 3,
			je,
			fe,
		},
		{
			"Enough to justify previous epoch but not current, N+2",
			641,
			2*len(validators)/3 + 3,
			len(validators) / 3,
			je + 1,
			fe,
		},
		{
			"Enough to justify current epoch",
			513,
			len(validators) / 3,
			2*len(validators)/3 + 3,
			je + 1,
			fe,
		},
		{
			"Enough to justify current epoch, but not previous",
			641,
			len(validators) / 3,
			2*len(validators)/3 + 3,
			je + 2,
			fe,
		},
		{
			"Enough to justify current and previous",
			641,
			2*len(validators)/3 + 3,
			2*len(validators)/3 + 3,
			je + 2,
			fe,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := &zondpb.BeaconStateCapella{
				RandaoMixes: make([][]byte, params.BeaconConfig().EpochsPerHistoricalVector),

				Validators:                  validators,
				Slot:                        test.slot,
				CurrentEpochParticipation:   make([]byte, params.BeaconConfig().MinGenesisActiveValidatorCount),
				PreviousEpochParticipation:  make([]byte, params.BeaconConfig().MinGenesisActiveValidatorCount),
				Balances:                    balances,
				PreviousJustifiedCheckpoint: pjcp,
				CurrentJustifiedCheckpoint:  cjcp,
				FinalizedCheckpoint:         fcp,
				InactivityScores:            make([]uint64, len(validators)),
				JustificationBits:           make(bitfield.Bitvector4, 1),
			}
			for i := 0; i < test.prevVals; i++ {
				base.PreviousEpochParticipation[i] = 0xFF
			}
			for i := 0; i < test.currVals; i++ {
				base.CurrentEpochParticipation[i] = 0xFF
			}
			if test.slot > 130 {
				base.JustificationBits.SetBitAt(2, true)
				base.JustificationBits.SetBitAt(3, true)
			} else {
				base.JustificationBits.SetBitAt(1, true)
				base.JustificationBits.SetBitAt(2, true)
			}

			state, err := state_native.InitializeFromProtoCapella(base)
			require.NoError(t, err)

			_, _, err = altair.InitializePrecomputeValidators(context.Background(), state)
			require.NoError(t, err)

			jc, fc, err := precompute.UnrealizedCheckpoints(state)
			require.NoError(t, err)
			require.DeepEqual(t, test.expectedJustified, jc.Epoch)
			require.DeepEqual(t, test.expectedFinalized, fc.Epoch)
		})
	}
}

func Test_ComputeCheckpoints_CantUpdateToLower(t *testing.T) {
	st, err := state_native.InitializeFromProtoCapella(&zondpb.BeaconStateCapella{
		Slot: params.BeaconConfig().SlotsPerEpoch * 2,
		CurrentJustifiedCheckpoint: &zondpb.Checkpoint{
			Epoch: 2,
		},
	})
	require.NoError(t, err)
	jb := make(bitfield.Bitvector4, 1)
	jb.SetBitAt(1, true)
	cp, _, err := precompute.ComputeCheckpoints(st, jb)
	require.NoError(t, err)
	require.Equal(t, primitives.Epoch(2), cp.Epoch)
}
