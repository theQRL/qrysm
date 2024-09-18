package altair_test

import (
	"math"
	"testing"

	"github.com/theQRL/qrysm/beacon-chain/core/altair"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func Test_BaseReward(t *testing.T) {
	helpers.ClearCache()
	genState := func(valCount uint64) state.ReadOnlyBeaconState {
		s, _ := util.DeterministicGenesisStateCapella(t, valCount)
		return s
	}
	tests := []struct {
		name      string
		valIdx    primitives.ValidatorIndex
		st        state.ReadOnlyBeaconState
		want      uint64
		errString string
	}{
		{
			name:      "unknown validator",
			valIdx:    2,
			st:        genState(1),
			want:      0,
			errString: "validator index 2 does not exist",
		},
		{
			name:      "active balance is 40000eth",
			valIdx:    0,
			st:        genState(1),
			want:      404760000,
			errString: "",
		},
		{
			name:      "active balance is 40000eth * target committee size",
			valIdx:    0,
			st:        genState(params.BeaconConfig().TargetCommitteeSize),
			want:      35760000,
			errString: "",
		},
		{
			name:      "active balance is 40000eth * max validator per  committee size",
			valIdx:    0,
			st:        genState(params.BeaconConfig().MaxValidatorsPerCommittee),
			want:      8920000,
			errString: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := altair.BaseReward(tt.st, tt.valIdx)
			if (err != nil) && (tt.errString != "") {
				require.ErrorContains(t, tt.errString, err)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_BaseRewardWithTotalBalance(t *testing.T) {
	helpers.ClearCache()
	s, _ := util.DeterministicGenesisStateCapella(t, 1)
	tests := []struct {
		name          string
		valIdx        primitives.ValidatorIndex
		activeBalance uint64
		want          uint64
		errString     string
	}{
		{
			name:          "active balance is 0",
			valIdx:        0,
			activeBalance: 0,
			want:          0,
			errString:     "active balance can't be 0",
		},
		{
			name:          "unknown validator",
			valIdx:        2,
			activeBalance: 1,
			want:          0,
			errString:     "validator index 2 does not exist",
		},
		{
			name:          "active balance is 1",
			valIdx:        0,
			activeBalance: 1,
			want:          2560000000000000,
			errString:     "",
		},
		{
			name:          "active balance is 1eth",
			valIdx:        0,
			activeBalance: params.BeaconConfig().EffectiveBalanceIncrement,
			want:          80956280000,
			errString:     "",
		},
		{
			name:          "active balance is 40000eth",
			valIdx:        0,
			activeBalance: params.BeaconConfig().MaxEffectiveBalance,
			want:          404760000,
			errString:     "",
		},
		{
			name:          "active balance is 40000eth * 1m validators",
			valIdx:        0,
			activeBalance: params.BeaconConfig().MaxEffectiveBalance * 1e9,
			want:          920000,
			errString:     "",
		},
		{
			name:          "active balance is max uint64",
			valIdx:        0,
			activeBalance: math.MaxUint64,
			want:          560000,
			errString:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := altair.BaseRewardWithTotalBalance(s, tt.valIdx, tt.activeBalance)
			if (err != nil) && (tt.errString != "") {
				require.ErrorContains(t, tt.errString, err)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_BaseRewardPerIncrement(t *testing.T) {
	helpers.ClearCache()
	tests := []struct {
		name          string
		activeBalance uint64
		want          uint64
		errString     string
	}{
		{
			name:          "active balance is 0",
			activeBalance: 0,
			want:          0,
			errString:     "active balance can't be 0",
		},
		{
			name:          "active balance is 1",
			activeBalance: 1,
			want:          64000000000,
			errString:     "",
		},
		{
			name:          "active balance is 1eth",
			activeBalance: params.BeaconConfig().EffectiveBalanceIncrement,
			want:          2023907,
			errString:     "",
		},
		{
			name:          "active balance is 40000eth",
			activeBalance: params.BeaconConfig().MaxEffectiveBalance,
			want:          10119,
			errString:     "",
		},
		{
			name:          "active balance is 40000eth * 1m validators",
			activeBalance: params.BeaconConfig().MaxEffectiveBalance * 1e9,
			want:          23,
			errString:     "",
		},
		{
			name:          "active balance is max uint64",
			activeBalance: math.MaxUint64,
			want:          14,
			errString:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := altair.BaseRewardPerIncrement(tt.activeBalance)
			if (err != nil) && (tt.errString != "") {
				require.ErrorContains(t, tt.errString, err)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}
