package state_native_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	state_native "github.com/theQRL/qrysm/beacon-chain/state/state-native"
	"github.com/theQRL/qrysm/config/features"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func BenchmarkAppendBalance(b *testing.B) {
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(b, err)

	max := uint64(16777216)
	for i := uint64(0); i < max-2; i++ {
		require.NoError(b, st.AppendBalance(i))
	}

	ref := st.Copy()

	for i := 0; b.Loop(); i++ {
		require.NoError(b, ref.AppendBalance(uint64(i)))
		ref = st.Copy()
	}
}

func BenchmarkAppendInactivityScore(b *testing.B) {
	st, err := state_native.InitializeFromProtoZond(&qrysmpb.BeaconStateZond{})
	require.NoError(b, err)

	max := uint64(16777216)
	for i := uint64(0); i < max-2; i++ {
		require.NoError(b, st.AppendInactivityScore(i))
	}

	ref := st.Copy()

	for i := 0; b.Loop(); i++ {
		require.NoError(b, ref.AppendInactivityScore(uint64(i)))
		ref = st.Copy()
	}
}

// Regression test for the qrysm port of upstream PR #17303: when the callback
// errors partway through, mutations applied before the error remain in the
// state and must still be marked dirty, otherwise HashTreeRoot serves a stale
// validators trie.
func TestApplyToEveryValidator_MarksDirtyOnEarlyError(t *testing.T) {
	for _, experimental := range []bool{false, true} {
		t.Run(fmt.Sprintf("experimental=%v", experimental), func(t *testing.T) {
			resetCfg := features.InitWithReset(&features.Flags{EnableExperimentalState: experimental})
			defer resetCfg()
			ctx := context.Background()

			st, _ := util.DeterministicGenesisStateZond(t, 16)
			htrBefore, err := st.HashTreeRoot(ctx)
			require.NoError(t, err)

			cbErr := errors.New("callback failure")
			err = st.ApplyToEveryValidator(func(idx int, val *qrysmpb.Validator) (bool, *qrysmpb.Validator, error) {
				if idx == 0 {
					val.ExitEpoch = 12345
					return true, val, nil
				}
				return false, nil, cbErr
			})
			require.ErrorIs(t, err, cbErr)

			// The validator-0 mutation stayed in the state, so the hash tree
			// root must account for it instead of serving the stale trie.
			htrAfter, err := st.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.NotEqual(t, htrBefore, htrAfter, "hash tree root does not reflect the applied mutation")

			// It must equal the root of a reference state mutated through the
			// regular setter.
			ref, _ := util.DeterministicGenesisStateZond(t, 16)
			v0 := ref.Validators()[0]
			v0.ExitEpoch = 12345
			require.NoError(t, ref.UpdateValidatorAtIndex(0, v0))
			refHTR, err := ref.HashTreeRoot(ctx)
			require.NoError(t, err)
			require.Equal(t, refHTR, htrAfter)
		})
	}
}
