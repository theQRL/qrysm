package helpers_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/state"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	zondpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestWeakSubjectivity_ComputeWeakSubjectivityPeriod(t *testing.T) {
	tests := []struct {
		valCount   uint64
		avgBalance uint64
		want       primitives.Epoch
	}{
		// Asserting that we get the same numbers as defined in the reference table:
		// https://github.com/ethereum/consensus-specs/blob/master/specs/phase0/weak-subjectivity.md#calculating-the-weak-subjectivity-period
		{valCount: 32768, avgBalance: 35, want: 115},
		{valCount: 65536, avgBalance: 35, want: 214},
		{valCount: 131072, avgBalance: 35, want: 413},
		{valCount: 262144, avgBalance: 35, want: 810},
		{valCount: 524288, avgBalance: 35, want: 1604},
		{valCount: 1048576, avgBalance: 35, want: 75},
		{valCount: 32768, avgBalance: 40, want: 179},
		{valCount: 65536, avgBalance: 40, want: 343},
		{valCount: 131072, avgBalance: 40, want: 671},
		{valCount: 262144, avgBalance: 40, want: 1326},
		{valCount: 524288, avgBalance: 40, want: 21},
		{valCount: 1048576, avgBalance: 40, want: 26},
		// Additional test vectors, to check case when T*(200+3*D) >= t*(200+12*D)
		{valCount: 32768, avgBalance: 28, want: 21},
		{valCount: 65536, avgBalance: 28, want: 27},
		{valCount: 131072, avgBalance: 28, want: 38},
		{valCount: 262144, avgBalance: 28, want: 60},
		{valCount: 524288, avgBalance: 28, want: 105},
		{valCount: 1048576, avgBalance: 28, want: 43},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("valCount: %d, avgBalance: %d", tt.valCount, tt.avgBalance), func(t *testing.T) {
			// Reset committee cache - as we need to recalculate active validator set for each test.
			helpers.ClearCache()
			got, err := helpers.ComputeWeakSubjectivityPeriod(context.Background(), genState(t, tt.valCount, tt.avgBalance), params.BeaconConfig())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got, "valCount: %v, avgBalance: %v", tt.valCount, tt.avgBalance)
		})
	}
}

type mockWsCheckpoint func() (stateRoot [32]byte, blockRoot [32]byte, e primitives.Epoch)

func TestWeakSubjectivity_IsWithinWeakSubjectivityPeriod(t *testing.T) {
	tests := []struct {
		name            string
		epoch           primitives.Epoch
		genWsState      func() state.ReadOnlyBeaconState
		genWsCheckpoint mockWsCheckpoint
		want            bool
		wantedErr       string
	}{
		{
			name: "nil weak subjectivity state",
			genWsState: func() state.ReadOnlyBeaconState {
				return nil
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, primitives.Epoch) {
				return [32]byte{}, [32]byte{}, 42
			},
			wantedErr: "invalid weak subjectivity state or checkpoint",
		},
		{
			name: "state and checkpoint roots do not match",
			genWsState: func() state.ReadOnlyBeaconState {
				beaconState := genState(t, 128, 40)
				require.NoError(t, beaconState.SetSlot(42*params.BeaconConfig().SlotsPerEpoch))
				err := beaconState.SetLatestBlockHeader(&zondpb.BeaconBlockHeader{
					Slot:      42 * params.BeaconConfig().SlotsPerEpoch,
					StateRoot: bytesutil.PadTo([]byte("stateroot1"), 32),
				})
				require.NoError(t, err)
				return beaconState
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, primitives.Epoch) {
				var sr [32]byte
				copy(sr[:], bytesutil.PadTo([]byte("stateroot2"), 32))
				return sr, [32]byte{}, 42
			},
			wantedErr: fmt.Sprintf("state (%#x) and checkpoint (%#x) roots do not match",
				bytesutil.PadTo([]byte("stateroot1"), 32), bytesutil.PadTo([]byte("stateroot2"), 32)),
		},
		{
			name: "state and checkpoint epochs do not match",
			genWsState: func() state.ReadOnlyBeaconState {
				beaconState := genState(t, 128, 40)
				require.NoError(t, beaconState.SetSlot(42*params.BeaconConfig().SlotsPerEpoch))
				err := beaconState.SetLatestBlockHeader(&zondpb.BeaconBlockHeader{
					Slot:      42 * params.BeaconConfig().SlotsPerEpoch,
					StateRoot: bytesutil.PadTo([]byte("stateroot"), 32),
				})
				require.NoError(t, err)
				return beaconState
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, primitives.Epoch) {
				var sr [32]byte
				copy(sr[:], bytesutil.PadTo([]byte("stateroot"), 32))
				return sr, [32]byte{}, 43
			},
			wantedErr: "state (42) and checkpoint (43) epochs do not match",
		},
		{
			name: "no active validators",
			genWsState: func() state.ReadOnlyBeaconState {
				beaconState := genState(t, 0, 40)
				require.NoError(t, beaconState.SetSlot(42*params.BeaconConfig().SlotsPerEpoch))
				err := beaconState.SetLatestBlockHeader(&zondpb.BeaconBlockHeader{
					Slot:      42 * params.BeaconConfig().SlotsPerEpoch,
					StateRoot: bytesutil.PadTo([]byte("stateroot"), 32),
				})
				require.NoError(t, err)
				return beaconState
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, primitives.Epoch) {
				var sr [32]byte
				copy(sr[:], bytesutil.PadTo([]byte("stateroot"), 32))
				return sr, [32]byte{}, 42
			},
			wantedErr: "cannot compute weak subjectivity period: no active validators found",
		},
		{
			name:  "outside weak subjectivity period",
			epoch: 300,
			genWsState: func() state.ReadOnlyBeaconState {
				beaconState := genState(t, 128, 40)
				require.NoError(t, beaconState.SetSlot(42*params.BeaconConfig().SlotsPerEpoch))
				err := beaconState.SetLatestBlockHeader(&zondpb.BeaconBlockHeader{
					Slot:      42 * params.BeaconConfig().SlotsPerEpoch,
					StateRoot: bytesutil.PadTo([]byte("stateroot"), 32),
				})
				require.NoError(t, err)
				return beaconState
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, primitives.Epoch) {
				var sr [32]byte
				copy(sr[:], bytesutil.PadTo([]byte("stateroot"), 32))
				return sr, [32]byte{}, 42
			},
			want: false,
		},
		{
			name:  "within weak subjectivity period",
			epoch: 57,
			genWsState: func() state.ReadOnlyBeaconState {
				beaconState := genState(t, 128, 40)
				require.NoError(t, beaconState.SetSlot(42*params.BeaconConfig().SlotsPerEpoch))
				err := beaconState.SetLatestBlockHeader(&zondpb.BeaconBlockHeader{
					Slot:      42 * params.BeaconConfig().SlotsPerEpoch,
					StateRoot: bytesutil.PadTo([]byte("stateroot"), 32),
				})
				require.NoError(t, err)
				return beaconState
			},
			genWsCheckpoint: func() ([32]byte, [32]byte, primitives.Epoch) {
				var sr [32]byte
				copy(sr[:], bytesutil.PadTo([]byte("stateroot"), 32))
				return sr, [32]byte{}, 42
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sr, _, e := tt.genWsCheckpoint()
			got, err := helpers.IsWithinWeakSubjectivityPeriod(context.Background(), tt.epoch, tt.genWsState(), sr, e, params.BeaconConfig())
			if tt.wantedErr != "" {
				assert.Equal(t, false, got)
				assert.ErrorContains(t, tt.wantedErr, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWeakSubjectivity_ParseWeakSubjectivityInputString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		checkpt   *zondpb.Checkpoint
		wantedErr string
	}{
		{
			name:      "No column in string",
			input:     "0x111111;123",
			wantedErr: "did not contain column",
		},
		{
			name:      "Too many columns in string",
			input:     "0x010203:123:456",
			wantedErr: "weak subjectivity checkpoint input should be in `block_root:epoch_number` format",
		},
		{
			name:      "Incorrect block root length",
			input:     "0x010203:987",
			wantedErr: "block root is not length of 32",
		},
		{
			name:  "Correct input",
			input: "0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:123456789",
			checkpt: &zondpb.Checkpoint{
				Root:  []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
				Epoch: primitives.Epoch(123456789),
			},
		},
		{
			name:  "Correct input without 0x",
			input: "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:123456789",
			checkpt: &zondpb.Checkpoint{
				Root:  []byte{255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
				Epoch: primitives.Epoch(123456789),
			},
		},
		{
			name:  "Correct input",
			input: "0xF0FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:123456789",
			checkpt: &zondpb.Checkpoint{
				Root:  []byte{0xf0, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
				Epoch: primitives.Epoch(123456789),
			},
		},
		{
			name:  "Correct input without 0x",
			input: "F0FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF:123456789",
			checkpt: &zondpb.Checkpoint{
				Root:  []byte{0xf0, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255},
				Epoch: primitives.Epoch(123456789),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wsCheckpt, err := helpers.ParseWeakSubjectivityInputString(tt.input)
			if tt.wantedErr != "" {
				require.ErrorContains(t, tt.wantedErr, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, wsCheckpt)
			require.DeepEqual(t, tt.checkpt.Root, wsCheckpt.Root, "Roots do not match")
			require.Equal(t, tt.checkpt.Epoch, wsCheckpt.Epoch, "Epochs do not match")
		})
	}
}

func genState(t *testing.T, valCount, avgBalance uint64) state.BeaconState {
	beaconState, err := util.NewBeaconStateCapella()
	require.NoError(t, err)

	validators := make([]*zondpb.Validator, valCount)
	balances := make([]uint64, len(validators))
	for i := uint64(0); i < valCount; i++ {
		validators[i] = &zondpb.Validator{
			PublicKey:             make([]byte, field_params.DilithiumPubkeyLength),
			WithdrawalCredentials: make([]byte, 32),
			EffectiveBalance:      avgBalance * 1e12,
			ExitEpoch:             params.BeaconConfig().FarFutureEpoch,
		}
		balances[i] = validators[i].EffectiveBalance
	}

	require.NoError(t, beaconState.SetValidators(validators))
	require.NoError(t, beaconState.SetBalances(balances))

	return beaconState
}
