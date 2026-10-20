package transition_test

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	consensusblocks "github.com/theQRL/qrysm/consensus-types/blocks"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

// noPanic runs fn and reports a panic as a test failure instead of aborting
// the whole package, so every case still reports.
func noPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PANIC: %v", r)
		}
	}()
	fn()
}

// transition.go:113 - ProcessSlot dereferences the state's latest block header
// after hashing the state (the hasher itself tolerates nil). Panicked before the guard.
func TestNoPanic_ProcessSlotNilLatestBlockHeader(t *testing.T) {
	st, err := util.NewBeaconStateZond(func(s *qrysmpb.BeaconStateZond) error {
		s.LatestBlockHeader = nil
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, true, st.LatestBlockHeader() == nil, "constructor did not keep a nil header")
	noPanic(t, func() {
		_, err := transition.ProcessSlot(context.Background(), st)
		require.ErrorContains(t, "nil latest block header", err)
	})
}

// transition.go:190 and :52 - slot ordering and a nil block are rejected up front.
func TestNoPanic_TransitionRejectsBadSlotAndNilBlock(t *testing.T) {
	ctx := context.Background()
	st, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, st.SetSlot(5))
	noPanic(t, func() {
		_, err := transition.ProcessSlots(ctx, st, 5)
		require.ErrorContains(t, "expected state.slot", err)
		_, err = transition.ProcessSlots(ctx, st, 4)
		require.ErrorContains(t, "expected state.slot", err)
		_, err = transition.ExecuteStateTransition(ctx, st, nil)
		require.NotNil(t, err)
	})
}

// transition.go:306 - the deposit-count subtraction is guarded against underflow.
func TestNoPanic_OperationLengthsDepositIndexAboveCount(t *testing.T) {
	st, _ := util.DeterministicGenesisStateZond(t, 8)
	require.NoError(t, st.SetExecutionDepositIndex(st.ExecutionData().DepositCount+1))
	wsb, err := consensusblocks.NewSignedBeaconBlock(util.NewBeaconBlockZond())
	require.NoError(t, err)
	noPanic(t, func() {
		_, err := transition.VerifyOperationLengths(context.Background(), st, wsb)
		require.ErrorContains(t, "expected state.deposit_index", err)
	})
}

// transition.go:95 - ProcessSlot is called from state replay with caller-supplied
// states; a nil or typed-nil state must error the way ProcessSlots does.
func TestNoPanic_ProcessSlotNilState(t *testing.T) {
	ctx := context.Background()
	noPanic(t, func() {
		_, err := transition.ProcessSlot(ctx, nil)
		require.ErrorContains(t, "nil state", err)
		var typedNil *state_native.BeaconState
		_, err = transition.ProcessSlot(ctx, typedNil)
		require.ErrorContains(t, "nil state", err)
	})
}
