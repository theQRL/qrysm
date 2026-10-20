package params_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/require"
)

func TestMaxActiveValidators_CommitteeScaling(t *testing.T) {
	for _, tc := range []struct {
		name       string
		slots      primitives.Slot
		committees uint64
		target     uint64
		members    uint64
		want       uint64
		wantErr    string
	}{
		{name: "mainnet", slots: 128, committees: 1, target: 32, members: 32, want: 4096},
		{name: "two committees with unsafe gap", slots: 128, committees: 2, target: 32, members: 32, want: 4096},
		{name: "four committees with unsafe gap", slots: 128, committees: 4, target: 32, members: 32, want: 4096},
		{name: "two committees without gap", slots: 128, committees: 2, target: 16, members: 32, want: 8192},
		{name: "four committees without gap", slots: 128, committees: 4, target: 8, members: 32, want: 16384},
		{name: "target just too large", slots: 128, committees: 2, target: 17, members: 32, want: 4096},
		{name: "minimal", slots: 8, committees: 4, target: 4, members: 32, want: 1024},
		{name: "target above member limit", slots: 8, committees: 4, target: 64, members: 32, want: 256},
		{name: "single slot boundary", slots: 1, committees: 4, target: 2, members: 3, want: 12},
		{name: "multiple slot boundary", slots: 2, committees: 4, target: 2, members: 3, want: 6},
		{name: "maximum target", slots: 128, committees: 2, target: math.MaxUint64, members: 32, want: 4096},
		{name: "maximum capacity", slots: math.MaxUint64, committees: 1, target: 1, members: 1, want: math.MaxUint64},
		{name: "large single committee capacity", slots: math.MaxUint64 / 2, committees: 2, target: 1, members: 1, want: math.MaxUint64 / 2},
		{name: "zero target", slots: 128, committees: 2, members: 32, wantErr: "TARGET_COMMITTEE_SIZE must be non-zero"},
		{name: "committee count overflow", slots: 2, committees: math.MaxUint64, target: 1, members: 1, wantErr: "MAX_COMMITTEES_PER_SLOT * SLOTS_PER_EPOCH overflows uint64"},
		{name: "capacity overflow", slots: math.MaxUint64, committees: 1, target: 1, members: 2, wantErr: "MAX_COMMITTEES_PER_SLOT * SLOTS_PER_EPOCH * MAX_VALIDATORS_PER_COMMITTEE overflows uint64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := params.MainnetConfig().Copy()
			cfg.SlotsPerEpoch = tc.slots
			cfg.MaxCommitteesPerSlot = tc.committees
			cfg.TargetCommitteeSize = tc.target
			cfg.MaxValidatorsPerCommittee = tc.members
			got, err := cfg.MaxActiveValidators()
			if tc.wantErr != "" {
				require.ErrorContains(t, tc.wantErr, err)
				require.Equal(t, uint64(0), got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestMaxActiveValidators_ContiguousSafeRange(t *testing.T) {
	// Brute-force small configurations using the committee-selection formula,
	// independently of the capacity calculation. Stop at the first unsafe count:
	// a larger, individually encodable set is unsafe if exits can reach that gap.
	for slots := uint64(1); slots <= 4; slots++ {
		for committees := uint64(1); committees <= 4; committees++ {
			for target := uint64(1); target <= 8; target++ {
				for members := uint64(1); members <= 8; members++ {
					cfg := params.MainnetConfig().Copy()
					cfg.SlotsPerEpoch = primitives.Slot(slots)
					cfg.MaxCommitteesPerSlot = committees
					cfg.TargetCommitteeSize = target
					cfg.MaxValidatorsPerCommittee = members
					var want uint64
					for n := uint64(1); n <= slots*committees*members+1; n++ {
						actualCommittees := max(uint64(1), min(committees, n/slots/target))
						committeesPerEpoch := slots * actualCommittees
						largestCommittee := (n + committeesPerEpoch - 1) / committeesPerEpoch
						if largestCommittee > members {
							want = n - 1
							break
						}
					}
					got, err := cfg.MaxActiveValidators()
					require.NoError(t, err)
					require.Equal(t, want, got, fmt.Sprintf("slots=%d committees=%d target=%d members=%d", slots, committees, target, members))
				}
			}
		}
	}
}

func TestUnmarshalConfig_RejectsGenesisCountAboveSafeCommitteeCapacity(t *testing.T) {
	input := []byte("MAX_COMMITTEES_PER_SLOT: 2\nMIN_GENESIS_ACTIVE_VALIDATOR_COUNT: 4097\n")
	loaded, err := params.UnmarshalConfig(input, nil)
	require.ErrorContains(t, "MIN_GENESIS_ACTIVE_VALIDATOR_COUNT (4097) must not exceed the active validator capacity (4096)", err)
	require.Equal(t, true, loaded == nil)
}
