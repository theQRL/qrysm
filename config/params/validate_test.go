package params_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/require"
)

func TestMainnetSlashingWindow(t *testing.T) {
	cfg := params.MainnetConfig()
	require.Equal(t, uint64(512), uint64(cfg.EpochsPerSlashingsVector))
	window := time.Duration(uint64(cfg.EpochsPerSlashingsVector)*uint64(cfg.SlotsPerEpoch)*cfg.SecondsPerSlot) * time.Second
	require.Equal(t, 45*24*time.Hour+12*time.Hour+16*time.Minute, window)
	require.Equal(t, 22*24*time.Hour+18*time.Hour+8*time.Minute, window/2)
	// Shortening the window does not change the initial or correlation penalty rates.
	require.Equal(t, uint64(32), cfg.MinSlashingPenaltyQuotient)
	require.Equal(t, uint64(3), cfg.ProportionalSlashingMultiplier)
}

func TestValidate_BuiltInConfigs(t *testing.T) {
	configs := map[string]*params.BeaconChainConfig{
		"mainnet":     params.MainnetConfig(),
		"minimal":     params.MinimalSpecConfig(),
		"e2e":         params.E2ETestConfig(),
		"e2e-mainnet": params.E2EMainnetTestConfig(),
	}
	for name, cfg := range configs {
		require.NoError(t, cfg.Validate(), "%s config fails validation", name)
	}
}

func TestValidate_NonZeroDivisors(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			for _, tc := range []struct {
				name string
				zero func(*params.BeaconChainConfig)
			}{
				{"INTERVALS_PER_SLOT", func(c *params.BeaconChainConfig) { c.IntervalsPerSlot = 0 }},
				{"SECONDS_PER_EXECUTION_BLOCK", func(c *params.BeaconChainConfig) { c.SecondsPerExecutionBlock = 0 }},
				{"TARGET_AGGREGATORS_PER_COMMITTEE", func(c *params.BeaconChainConfig) { c.TargetAggregatorsPerCommittee = 0 }},
				{"TARGET_AGGREGATORS_PER_SYNC_SUBCOMMITTEE", func(c *params.BeaconChainConfig) { c.TargetAggregatorsPerSyncSubcommittee = 0 }},
				{"EPOCHS_PER_EXECUTION_VOTING_PERIOD", func(c *params.BeaconChainConfig) { c.EpochsPerExecutionVotingPeriod = 0 }},
				{"INACTIVITY_SCORE_BIAS", func(c *params.BeaconChainConfig) { c.InactivityScoreBias = 0 }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					want := tc.name + " must be non-zero"
					cfg := base.Copy()
					tc.zero(cfg)
					require.ErrorContains(t, want, cfg.Validate())
					for _, value := range []uint64{0, 1} {
						t.Run(fmt.Sprintf("YAML_%d", value), func(t *testing.T) {
							input := fmt.Sprintf("PRESET_BASE: %s\n%s: %d\n", base.PresetBase, tc.name, value)
							loaded, err := params.UnmarshalConfig([]byte(input), nil)
							if value == 0 {
								require.ErrorContains(t, "invalid chain config", err)
								require.ErrorContains(t, want, err)
								require.Equal(t, true, loaded == nil)
								return
							}
							// A positive divisor is arithmetically valid. The exact
							// execution-voting layout is checked separately at startup.
							require.NoError(t, err)
							require.NoError(t, loaded.Validate())
						})
					}
				})
			}
		})
	}
}

func TestValidate_AttestationInclusionDelay(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			for _, delay := range []primitives.Slot{1, base.SlotsPerEpoch - 1, base.SlotsPerEpoch, base.SlotsPerEpoch + 1, math.MaxUint64} {
				t.Run(fmt.Sprintf("delay_%d", delay), func(t *testing.T) {
					cfg := base.Copy()
					cfg.MinAttestationInclusionDelay = delay
					input := fmt.Sprintf("PRESET_BASE: %s\nMIN_ATTESTATION_INCLUSION_DELAY: %d\n", base.PresetBase, delay)
					loaded, err := params.UnmarshalConfig([]byte(input), nil)
					for name, err := range map[string]error{"validation": cfg.Validate(), "YAML loading": err} {
						if delay > base.SlotsPerEpoch {
							require.ErrorContains(t, fmt.Sprintf("MIN_ATTESTATION_INCLUSION_DELAY (%d) must not exceed SLOTS_PER_EPOCH (%d)", delay, base.SlotsPerEpoch), err, name)
						} else {
							require.NoError(t, err, name)
						}
					}
					if delay > base.SlotsPerEpoch {
						require.Equal(t, true, loaded == nil)
					} else {
						require.Equal(t, delay, loaded.MinAttestationInclusionDelay)
					}
				})
			}
		})
	}
}

func TestValidate_RandomSubnetSubscriptionPeriod(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				period uint64
				want   string
			}{
				{"zero", 0, "EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION must be non-zero"},
				{"one_epoch", 1, ""},
				{"default", base.EpochsPerRandomSubnetSubscription, ""},
				{"max_int", uint64(math.MaxInt), ""},
				{"above_max_int", uint64(math.MaxInt) + 1, fmt.Sprintf("EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION (%d) must not exceed %d", uint64(math.MaxInt)+1, math.MaxInt)},
				{"max_uint64", math.MaxUint64, fmt.Sprintf("EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION (%d) must not exceed %d", uint64(math.MaxUint64), math.MaxInt)},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cfg := base.Copy()
					cfg.EpochsPerRandomSubnetSubscription = tc.period
					input := fmt.Sprintf("PRESET_BASE: %s\nEPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION: %d\n", base.PresetBase, tc.period)
					loaded, err := params.UnmarshalConfig([]byte(input), nil)
					for name, err := range map[string]error{"validation": cfg.Validate(), "YAML loading": err} {
						if tc.want != "" {
							require.ErrorContains(t, tc.want, err, name)
						} else {
							require.NoError(t, err, name)
						}
					}
					if tc.want != "" {
						require.Equal(t, true, loaded == nil)
					} else {
						require.Equal(t, tc.period, loaded.EpochsPerRandomSubnetSubscription)
					}
				})
			}
		})
	}
}

func TestValidate_ShuffleRoundCount(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			for _, rounds := range []uint64{0, 1, 10, 90, 254, 255, 256, 257, 512, math.MaxUint64} {
				t.Run(fmt.Sprintf("rounds_%d", rounds), func(t *testing.T) {
					cfg := base.Copy()
					cfg.ShuffleRoundCount = rounds
					input := fmt.Sprintf("PRESET_BASE: %s\nSHUFFLE_ROUND_COUNT: %d\n", base.PresetBase, rounds)
					loaded, err := params.UnmarshalConfig([]byte(input), nil)
					for name, err := range map[string]error{"validation": cfg.Validate(), "YAML loading": err} {
						if rounds > 255 {
							require.ErrorContains(t, fmt.Sprintf("SHUFFLE_ROUND_COUNT (%d) must not exceed 255", rounds), err, name)
						} else {
							require.NoError(t, err, name)
						}
					}
					if rounds > 255 {
						require.Equal(t, true, loaded == nil)
					} else {
						require.Equal(t, rounds, loaded.ShuffleRoundCount)
					}
				})
			}
		})
	}
}

func TestValidate_ParticipationFlagIndices(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				indices [3]uint8 // source, target, head
				want    string
			}{
				{"defaults", [3]uint8{0, 1, 2}, ""},
				{"reordered", [3]uint8{2, 0, 1}, ""},
				{"source_7", [3]uint8{7, 1, 2}, ""},
				{"target_7", [3]uint8{0, 7, 2}, ""},
				{"head_7", [3]uint8{0, 1, 7}, ""},
				{"nonconsecutive", [3]uint8{7, 5, 3}, ""},
				{"source_8", [3]uint8{8, 1, 2}, "TIMELY_SOURCE_FLAG_INDEX (8) must be between 0 and 7"},
				{"target_8", [3]uint8{0, 8, 2}, "TIMELY_TARGET_FLAG_INDEX (8) must be between 0 and 7"},
				{"head_8", [3]uint8{0, 1, 8}, "TIMELY_HEAD_FLAG_INDEX (8) must be between 0 and 7"},
				{"source_255", [3]uint8{255, 1, 2}, "TIMELY_SOURCE_FLAG_INDEX (255) must be between 0 and 7"},
				{"target_255", [3]uint8{0, 255, 2}, "TIMELY_TARGET_FLAG_INDEX (255) must be between 0 and 7"},
				{"head_255", [3]uint8{0, 1, 255}, "TIMELY_HEAD_FLAG_INDEX (255) must be between 0 and 7"},
				{"duplicate_source_target", [3]uint8{0, 0, 2}, "TIMELY_SOURCE_FLAG_INDEX and TIMELY_TARGET_FLAG_INDEX must be distinct (both are 0)"},
				{"duplicate_source_head", [3]uint8{0, 1, 0}, "TIMELY_SOURCE_FLAG_INDEX and TIMELY_HEAD_FLAG_INDEX must be distinct (both are 0)"},
				{"duplicate_target_head", [3]uint8{0, 1, 1}, "TIMELY_TARGET_FLAG_INDEX and TIMELY_HEAD_FLAG_INDEX must be distinct (both are 1)"},
				{"all_equal", [3]uint8{7, 7, 7}, "TIMELY_SOURCE_FLAG_INDEX and TIMELY_TARGET_FLAG_INDEX must be distinct (both are 7)"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cfg := base.Copy()
					cfg.TimelySourceFlagIndex = tc.indices[0]
					cfg.TimelyTargetFlagIndex = tc.indices[1]
					cfg.TimelyHeadFlagIndex = tc.indices[2]
					input := fmt.Sprintf("PRESET_BASE: %s\nTIMELY_SOURCE_FLAG_INDEX: %d\nTIMELY_TARGET_FLAG_INDEX: %d\nTIMELY_HEAD_FLAG_INDEX: %d\n",
						base.PresetBase, tc.indices[0], tc.indices[1], tc.indices[2])
					loaded, err := params.UnmarshalConfig([]byte(input), nil)
					for name, err := range map[string]error{"validation": cfg.Validate(), "YAML loading": err} {
						if tc.want != "" {
							require.ErrorContains(t, tc.want, err, name)
						} else {
							require.NoError(t, err, name)
						}
					}
					if tc.want != "" {
						require.Equal(t, true, loaded == nil)
					} else {
						require.Equal(t, tc.indices, [3]uint8{loaded.TimelySourceFlagIndex, loaded.TimelyTargetFlagIndex, loaded.TimelyHeadFlagIndex})
					}
				})
			}
		})
	}
}

func TestValidate_RewardWeightSum(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				weights [5]uint64 // source, target, head, sync, proposer
				want    string
			}{
				{"defaults", [5]uint64{14, 26, 14, 2, 8}, ""},
				{"rebalanced", [5]uint64{1, 1, 1, 1, 60}, ""},
				{"incorrect sum", [5]uint64{14, 26, 15, 2, 8}, "must equal WEIGHT_DENOMINATOR"},
				// Each of these sums is 2^64 + 64, which used to wrap to the
				// expected denominator and pass validation.
				{"source overflow", [5]uint64{math.MaxUint64, 41, 14, 2, 8}, "PROPOSER_WEIGHT overflows uint64"},
				{"target overflow", [5]uint64{29, math.MaxUint64, 26, 2, 8}, "PROPOSER_WEIGHT overflows uint64"},
				{"head overflow", [5]uint64{14, 41, math.MaxUint64, 2, 8}, "PROPOSER_WEIGHT overflows uint64"},
				{"sync overflow", [5]uint64{14, 29, 14, math.MaxUint64, 8}, "PROPOSER_WEIGHT overflows uint64"},
				{"overflow on final addition", [5]uint64{math.MaxUint64, 0, 0, 0, 8}, "PROPOSER_WEIGHT overflows uint64"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cfg := base.Copy()
					cfg.TimelySourceWeight = tc.weights[0]
					cfg.TimelyTargetWeight = tc.weights[1]
					cfg.TimelyHeadWeight = tc.weights[2]
					cfg.SyncRewardWeight = tc.weights[3]
					cfg.ProposerWeight = tc.weights[4]
					input := fmt.Sprintf("PRESET_BASE: %s\nTIMELY_SOURCE_WEIGHT: %d\nTIMELY_TARGET_WEIGHT: %d\nTIMELY_HEAD_WEIGHT: %d\nSYNC_REWARD_WEIGHT: %d\nPROPOSER_WEIGHT: %d\n",
						base.PresetBase, tc.weights[0], tc.weights[1], tc.weights[2], tc.weights[3], tc.weights[4])
					loaded, err := params.UnmarshalConfig([]byte(input), nil)
					for name, err := range map[string]error{"validation": cfg.Validate(), "YAML loading": err} {
						if tc.want != "" {
							require.ErrorContains(t, tc.want, err, name)
						} else {
							require.NoError(t, err, name)
						}
					}
					if tc.want != "" {
						require.Equal(t, true, loaded == nil)
					}
				})
			}
		})
	}
}

func TestValidate_AttestationRewardDenominator(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			capacity, err := base.MaxActiveValidators()
			require.NoError(t, err)
			maxActiveIncrements := capacity * (base.MaxEffectiveBalance / base.EffectiveBalanceIncrement)
			maxDenominator := uint64(math.MaxUint64) / maxActiveIncrements
			for _, tc := range []struct {
				name        string
				denominator uint64
				overflow    bool
			}{
				{"default denominator", 64, false},
				{"largest safe denominator", maxDenominator, false},
				// The next denominator still fits the genesis validator count;
				// validation must also account for later activations up to capacity.
				{"above safe denominator", maxDenominator + 1, true},
				{"overflow to zero", 1 << 51, true},
				{"overflow to nonzero", 1<<51 + 1, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cfg := base.Copy()
					cfg.WeightDenominator = tc.denominator
					cfg.ProposerWeight = tc.denominator - 1
					cfg.TimelySourceWeight = 1
					cfg.TimelyTargetWeight = 0
					cfg.TimelyHeadWeight = 0
					cfg.SyncRewardWeight = 0
					input := fmt.Sprintf("PRESET_BASE: %s\nWEIGHT_DENOMINATOR: %d\nPROPOSER_WEIGHT: %d\nTIMELY_SOURCE_WEIGHT: 1\nTIMELY_TARGET_WEIGHT: 0\nTIMELY_HEAD_WEIGHT: 0\nSYNC_REWARD_WEIGHT: 0\n",
						base.PresetBase, tc.denominator, tc.denominator-1)
					loaded, err := params.UnmarshalConfig([]byte(input), nil)
					for name, err := range map[string]error{"validation": cfg.Validate(), "YAML loading": err} {
						if tc.overflow {
							require.ErrorContains(t, fmt.Sprintf("maximum active balance increments (%d) * WEIGHT_DENOMINATOR (%d) overflows uint64", maxActiveIncrements, tc.denominator), err, name)
						} else {
							require.NoError(t, err, name)
						}
					}
					if tc.overflow {
						require.Equal(t, true, loaded == nil)
					} else {
						require.Equal(t, tc.denominator, loaded.WeightDenominator)
					}
				})
			}
		})
	}
}

func TestValidate_ActiveBalanceIncrementsOverflow(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			cfg := base.Copy()
			cfg.MaxEffectiveBalance = 1 << 63
			cfg.EffectiveBalanceIncrement = 1
			input := fmt.Sprintf("PRESET_BASE: %s\nMAX_EFFECTIVE_BALANCE: 9223372036854775808\nEFFECTIVE_BALANCE_INCREMENT: 1\n", base.PresetBase)
			loaded, err := params.UnmarshalConfig([]byte(input), nil)
			for name, err := range map[string]error{"validation": cfg.Validate(), "YAML loading": err} {
				require.ErrorContains(t, "active validator capacity * (MAX_EFFECTIVE_BALANCE / EFFECTIVE_BALANCE_INCREMENT) overflows uint64", err, name)
			}
			require.Equal(t, true, loaded == nil)
		})
	}
}

func TestValidate_ProposerRewardDenominator(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			for _, tc := range []struct {
				name        string
				denominator uint64
				proposer    uint64
				source      uint64
			}{
				{"zero proposer", 64, 0, 64},
				{"smallest proposer", 64, 1, 63},
				{"default proposer fraction", 64, 8, 56},
				{"just below denominator", 64, 63, 1},
				{"equal to denominator", 64, 64, 0},
				{"above denominator", 64, 65, 0},
				{"smallest valid denominator", 2, 1, 1},
				// The unchecked weight sum wraps to 1, and subtraction wraps to 2.
				// Dividing that by MaxUint64 used to produce a zero reward divisor.
				{"above denominator with wrapped sum", 1, math.MaxUint64, 2},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cfg := base.Copy()
					cfg.WeightDenominator = tc.denominator
					cfg.ProposerWeight = tc.proposer
					cfg.TimelySourceWeight = tc.source
					cfg.TimelyTargetWeight = 0
					cfg.TimelyHeadWeight = 0
					cfg.SyncRewardWeight = 0
					input := fmt.Sprintf("PRESET_BASE: %s\nWEIGHT_DENOMINATOR: %d\nPROPOSER_WEIGHT: %d\nTIMELY_SOURCE_WEIGHT: %d\nTIMELY_TARGET_WEIGHT: 0\nTIMELY_HEAD_WEIGHT: 0\nSYNC_REWARD_WEIGHT: 0\n",
						base.PresetBase, tc.denominator, tc.proposer, tc.source)
					loaded, err := params.UnmarshalConfig([]byte(input), nil)
					want := ""
					if tc.proposer == 0 {
						want = "PROPOSER_WEIGHT must be non-zero"
					} else if tc.proposer >= tc.denominator {
						want = fmt.Sprintf("PROPOSER_WEIGHT (%d) must be less than WEIGHT_DENOMINATOR (%d)", tc.proposer, tc.denominator)
					}
					for name, err := range map[string]error{"validation": cfg.Validate(), "YAML loading": err} {
						if want != "" {
							require.ErrorContains(t, want, err, name)
						} else {
							require.NoError(t, err, name)
						}
					}
					if want != "" {
						require.Equal(t, true, loaded == nil)
					} else {
						require.Equal(t, tc.proposer, loaded.ProposerWeight)
						divisor := (cfg.WeightDenominator - cfg.ProposerWeight) * cfg.WeightDenominator / cfg.ProposerWeight
						require.Equal(t, true, divisor > 0)
					}
				})
			}
		})
	}
}

func TestValidate_InactivityPenaltyDenominator(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			for _, tc := range []struct {
				name     string
				bias     uint64
				quotient uint64
				overflow bool
			}{
				{"smallest product", 1, 1, false},
				{"default product", base.InactivityScoreBias, base.InactivityPenaltyQuotient, false},
				{"largest quotient at default bias", 4, math.MaxUint64 / 4, false},
				{"quotient overflow to zero", 4, 1 << 62, true},
				{"quotient overflow to nonzero", 4, 1<<62 + 1, true},
				{"bias overflow to zero", 1 << 62, 4, true},
				{"maximum product from quotient", 1, math.MaxUint64, false},
				{"maximum product from bias", math.MaxUint64, 1, false},
				{"both factors maximum", math.MaxUint64, math.MaxUint64, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					cfg := base.Copy()
					cfg.InactivityScoreBias = tc.bias
					cfg.InactivityPenaltyQuotient = tc.quotient
					input := fmt.Sprintf("PRESET_BASE: %s\nINACTIVITY_SCORE_BIAS: %d\nINACTIVITY_PENALTY_QUOTIENT: %d\n", base.PresetBase, tc.bias, tc.quotient)
					loaded, err := params.UnmarshalConfig([]byte(input), nil)
					for name, err := range map[string]error{"validation": cfg.Validate(), "YAML loading": err} {
						if tc.overflow {
							want := fmt.Sprintf("INACTIVITY_SCORE_BIAS (%d) * INACTIVITY_PENALTY_QUOTIENT (%d) overflows uint64", tc.bias, tc.quotient)
							require.ErrorContains(t, want, err, name)
						} else {
							require.NoError(t, err, name)
						}
					}
					if tc.overflow {
						require.Equal(t, true, loaded == nil)
					} else {
						require.Equal(t, tc.bias, loaded.InactivityScoreBias)
						require.Equal(t, tc.quotient, loaded.InactivityPenaltyQuotient)
						require.Equal(t, true, cfg.InactivityScoreBias*cfg.InactivityPenaltyQuotient > 0)
					}
				})
			}
		})
	}
}

func TestValidate_ForkVersionLength(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.ConfigName, func(t *testing.T) {
			for _, version := range [][]byte{nil, {}, {1}, {1, 2}, {1, 2, 3}, {1, 2, 3, 4}, {1, 2, 3, 4, 5}, make([]byte, 8)} {
				t.Run(fmt.Sprintf("length_%d", len(version)), func(t *testing.T) {
					cfg := base.Copy()
					cfg.GenesisForkVersion = version
					err := cfg.Validate()
					if len(version) == fieldparams.VersionLength {
						require.NoError(t, err)
						return
					}
					require.ErrorContains(t, fmt.Sprintf("GENESIS_FORK_VERSION must be exactly %d bytes, got %d", fieldparams.VersionLength, len(version)), err)
				})
			}
		})
	}
}

func TestValidate_DepositTreeDepth(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			for _, depth := range []uint64{0, 1, 31, 32, 33, 63, math.MaxUint64} {
				t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
					cfg := base.Copy()
					cfg.DepositContractTreeDepth = depth
					input := fmt.Sprintf("PRESET_BASE: %s\nDEPOSIT_CONTRACT_TREE_DEPTH: %d\n", base.PresetBase, depth)
					loaded, err := params.UnmarshalConfig([]byte(input), nil)
					checks := map[string]error{"validation": cfg.Validate(), "YAML loading": err}
					if base.PresetBase == fieldparams.Preset {
						checks["compiled layout"] = cfg.ValidateStateLayout()
					}
					for name, err := range checks {
						if depth == 32 {
							require.NoError(t, err, name)
						} else {
							require.ErrorContains(t, fmt.Sprintf("DEPOSIT_CONTRACT_TREE_DEPTH (%d) must be 32 to match the SSZ deposit proof length (33)", depth), err, name)
						}
					}
					if depth == 32 {
						require.Equal(t, depth, loaded.DepositContractTreeDepth)
					} else {
						require.Equal(t, true, loaded == nil)
					}
				})
			}
		})
	}
}

func TestMaxActiveValidators(t *testing.T) {
	maxActiveValidators, err := params.MainnetConfig().MaxActiveValidators()
	require.NoError(t, err)
	require.Equal(t, uint64(4096), maxActiveValidators)
}

func TestValidate_RejectsBrokenArithmetic(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *params.BeaconChainConfig)
		want   string
	}{
		{
			name:   "zero seconds per slot",
			mutate: func(c *params.BeaconChainConfig) { c.SecondsPerSlot = 0 },
			want:   "SECONDS_PER_SLOT must be non-zero",
		},
		{
			name:   "zero effective balance increment",
			mutate: func(c *params.BeaconChainConfig) { c.EffectiveBalanceIncrement = 0 },
			want:   "EFFECTIVE_BALANCE_INCREMENT must be non-zero",
		},
		{
			name:   "zero min slashing penalty quotient",
			mutate: func(c *params.BeaconChainConfig) { c.MinSlashingPenaltyQuotient = 0 },
			want:   "MIN_SLASHING_PENALTY_QUOTIENT must be non-zero",
		},
		{
			name:   "zero proportional slashing multiplier",
			mutate: func(c *params.BeaconChainConfig) { c.ProportionalSlashingMultiplier = 0 },
			want:   "PROPORTIONAL_SLASHING_MULTIPLIER must be non-zero",
		},
		{
			name:   "zero whistleblower reward quotient",
			mutate: func(c *params.BeaconChainConfig) { c.WhistleBlowerRewardQuotient = 0 },
			want:   "WHISTLEBLOWER_REWARD_QUOTIENT must be non-zero",
		},
		{
			name:   "zero proposer reward quotient",
			mutate: func(c *params.BeaconChainConfig) { c.ProposerRewardQuotient = 0 },
			want:   "PROPOSER_REWARD_QUOTIENT must be non-zero",
		},
		{
			name:   "zero epochs per slashings vector",
			mutate: func(c *params.BeaconChainConfig) { c.EpochsPerSlashingsVector = 0 },
			want:   "EPOCHS_PER_SLASHINGS_VECTOR must be non-zero",
		},
		{
			name:   "zero hysteresis quotient",
			mutate: func(c *params.BeaconChainConfig) { c.HysteresisQuotient = 0 },
			want:   "HYSTERESIS_QUOTIENT must be non-zero",
		},
		{
			name:   "zero inactivity penalty quotient",
			mutate: func(c *params.BeaconChainConfig) { c.InactivityPenaltyQuotient = 0 },
			want:   "INACTIVITY_PENALTY_QUOTIENT must be non-zero",
		},
		{
			name:   "zero churn limit quotient",
			mutate: func(c *params.BeaconChainConfig) { c.ChurnLimitQuotient = 0 },
			want:   "CHURN_LIMIT_QUOTIENT must be non-zero",
		},
		{
			name:   "zero max per epoch activation churn limit",
			mutate: func(c *params.BeaconChainConfig) { c.MaxPerEpochActivationChurnLimit = 0 },
			want:   "MAX_PER_EPOCH_ACTIVATION_CHURN_LIMIT must be non-zero",
		},
		{
			name:   "zero weight denominator",
			mutate: func(c *params.BeaconChainConfig) { c.WeightDenominator = 0 },
			want:   "WEIGHT_DENOMINATOR must be non-zero",
		},
		{
			name:   "zero sync committee subnet count",
			mutate: func(c *params.BeaconChainConfig) { c.SyncCommitteeSubnetCount = 0 },
			want:   "SYNC_COMMITTEE_SUBNET_COUNT must be non-zero",
		},
		{
			name:   "zero max validators per committee",
			mutate: func(c *params.BeaconChainConfig) { c.MaxValidatorsPerCommittee = 0 },
			want:   "MAX_VALIDATORS_PER_COMMITTEE must be non-zero",
		},
		{
			name: "active validator capacity overflow",
			mutate: func(c *params.BeaconChainConfig) {
				c.MaxCommitteesPerSlot = math.MaxUint64
			},
			want: "MAX_COMMITTEES_PER_SLOT * SLOTS_PER_EPOCH overflows uint64",
		},
		{
			name: "genesis active validators exceed capacity",
			mutate: func(c *params.BeaconChainConfig) {
				maxActiveValidators, err := c.MaxActiveValidators()
				require.NoError(t, err)
				c.MinGenesisActiveValidatorCount = maxActiveValidators + 1
			},
			want: "MIN_GENESIS_ACTIVE_VALIDATOR_COUNT",
		},
		{
			name: "max effective balance not a multiple of the increment",
			mutate: func(c *params.BeaconChainConfig) {
				c.MaxEffectiveBalance = 3*c.EffectiveBalanceIncrement + 1
			},
			want: "must be a positive multiple of EFFECTIVE_BALANCE_INCREMENT",
		},
		{
			name: "max effective balance below the increment",
			mutate: func(c *params.BeaconChainConfig) {
				c.MaxEffectiveBalance = c.EffectiveBalanceIncrement - 1
			},
			want: "must be a positive multiple of EFFECTIVE_BALANCE_INCREMENT",
		},
		{
			name:   "hysteresis threshold overflow",
			mutate: func(c *params.BeaconChainConfig) { c.HysteresisUpwardMultiplier = math.MaxUint64 },
			want:   "HYSTERESIS_UPWARD_MULTIPLIER overflows uint64",
		},
		{
			name:   "base reward per increment overflow",
			mutate: func(c *params.BeaconChainConfig) { c.BaseRewardFactor = math.MaxUint64 },
			want:   "BASE_REWARD_FACTOR",
		},
		{
			name:   "participation weights do not sum to the denominator",
			mutate: func(c *params.BeaconChainConfig) { c.TimelyHeadWeight++ },
			want:   "must equal WEIGHT_DENOMINATOR",
		},
		{
			name: "sync committee not divisible into subnets",
			mutate: func(c *params.BeaconChainConfig) {
				c.SyncCommitteeSubnetCount = c.SyncCommitteeSize + 1
			},
			want: "SYNC_COMMITTEE_SIZE",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := params.MainnetConfig().Copy()
			tt.mutate(cfg)
			require.ErrorContains(t, tt.want, cfg.Validate())
		})
	}
}

func TestValidateStateLayout(t *testing.T) {
	// Pick the preset whose vectors match the SSZ layout this test binary was
	// compiled with (mainnet unless built with the minimal tag).
	cfg := params.MainnetConfig().Copy()
	if uint64(cfg.EpochsPerSlashingsVector) != fieldparams.SlashingsLength {
		cfg = params.MinimalSpecConfig().Copy()
	}
	require.NoError(t, cfg.ValidateStateLayout())

	broken := cfg.Copy()
	broken.EpochsPerSlashingsVector++
	require.ErrorContains(t, "EPOCHS_PER_SLASHINGS_VECTOR", broken.ValidateStateLayout())

	broken = cfg.Copy()
	broken.EpochsPerHistoricalVector++
	require.ErrorContains(t, "EPOCHS_PER_HISTORICAL_VECTOR", broken.ValidateStateLayout())

	broken = cfg.Copy()
	broken.SyncCommitteeSize *= 2
	require.ErrorContains(t, "SYNC_COMMITTEE_SIZE", broken.ValidateStateLayout())
}

func TestValidateStateLayout_ExecutionVotingLimit(t *testing.T) {
	base := params.MainnetConfig()
	if fieldparams.Preset == params.MinimalName {
		base = params.MinimalSpecConfig()
	}
	const product = "EPOCHS_PER_EXECUTION_VOTING_PERIOD * SLOTS_PER_EPOCH"
	for _, tc := range []struct {
		name   string
		mutate func(*params.BeaconChainConfig)
		want   string
	}{
		{name: "preset", mutate: func(*params.BeaconChainConfig) {}},
		{name: "larger period", mutate: func(c *params.BeaconChainConfig) { c.EpochsPerExecutionVotingPeriod++ }, want: product},
		{name: "smaller period", mutate: func(c *params.BeaconChainConfig) { c.EpochsPerExecutionVotingPeriod-- }, want: product},
		{name: "zero period", mutate: func(c *params.BeaconChainConfig) { c.EpochsPerExecutionVotingPeriod = 0 }, want: product},
		{name: "shorter epoch", mutate: func(c *params.BeaconChainConfig) { c.SlotsPerEpoch /= 2 }, want: product},
		{name: "longer epoch", mutate: func(c *params.BeaconChainConfig) { c.SlotsPerEpoch *= 2 }, want: product},
		{name: "zero slots", mutate: func(c *params.BeaconChainConfig) { c.SlotsPerEpoch = 0 }, want: product},
		{name: "overflow", mutate: func(c *params.BeaconChainConfig) { c.EpochsPerExecutionVotingPeriod = math.MaxUint64 }, want: product + " overflows uint64"},
		{
			name: "overflow with matching low bits",
			mutate: func(c *params.BeaconChainConfig) {
				c.EpochsPerExecutionVotingPeriod += math.MaxUint64/fieldparams.SlotsPerEpoch + 1
			},
			want: product + " overflows uint64",
		},
		{
			name: "same capacity with different factors",
			mutate: func(c *params.BeaconChainConfig) {
				c.EpochsPerExecutionVotingPeriod *= 2
				c.SlotsPerEpoch /= 2
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base.Copy()
			tc.mutate(cfg)
			err := cfg.ValidateStateLayout()
			if tc.want != "" {
				require.ErrorContains(t, tc.want, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, uint64(fieldparams.ExecutionDataVotesLength), cfg.ExecutionDataVotesLength())
		})
	}
}

func TestValidateStateLayout_ForkChoiceHistoryCapacity(t *testing.T) {
	base := params.MainnetConfig()
	if fieldparams.Preset == params.MinimalName {
		base = params.MinimalSpecConfig()
	}
	for _, tc := range []struct {
		name    string
		mutate  func(*params.BeaconChainConfig)
		wantErr bool
	}{
		{name: "preset epoch", mutate: func(*params.BeaconChainConfig) {}},
		{
			name: "shorter epoch with matching vote capacity",
			mutate: func(c *params.BeaconChainConfig) {
				c.SlotsPerEpoch /= 2
				c.EpochsPerExecutionVotingPeriod *= 2
			},
		},
		{
			name: "longer epoch with matching vote capacity",
			mutate: func(c *params.BeaconChainConfig) {
				c.SlotsPerEpoch *= 2
				c.EpochsPerExecutionVotingPeriod /= 2
			},
			wantErr: true,
		},
		{
			name: "voting period in one epoch",
			mutate: func(c *params.BeaconChainConfig) {
				c.SlotsPerEpoch *= 4
				c.EpochsPerExecutionVotingPeriod /= 4
			},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base.Copy()
			tc.mutate(cfg)
			// These overrides satisfy the portable arithmetic checks and the
			// execution-vote SSZ limit. Fork choice has a separate fixed bound.
			require.NoError(t, cfg.Validate())
			require.Equal(t, uint64(fieldparams.ExecutionDataVotesLength), cfg.ExecutionDataVotesLength())
			err := cfg.ValidateStateLayout()
			if tc.wantErr {
				require.ErrorContains(t, fmt.Sprintf("SLOTS_PER_EPOCH (%d) must not exceed this binary's fork-choice history capacity (%d)", cfg.SlotsPerEpoch, fieldparams.SlotsPerEpoch), err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateStateLayout_SyncCommitteeSubnetCount(t *testing.T) {
	base := params.MainnetConfig()
	if fieldparams.Preset == params.MinimalName {
		base = params.MinimalSpecConfig()
	}
	// An unchecked multiplication by the contribution byte length would wrap
	// this subnet count to exactly the expected aggregate byte length.
	wrappingCount := uint64(math.MaxUint64)/fieldparams.SyncCommitteeAggregationBytesLength + 2
	for _, preset := range []string{params.MainnetName, params.MinimalName, "custom"} {
		t.Run(preset, func(t *testing.T) {
			for _, count := range []uint64{0, 1, 2, 4, base.SyncCommitteeSize, math.MaxUint64, wrappingCount} {
				t.Run(fmt.Sprintf("subnets_%d", count), func(t *testing.T) {
					cfg := base.Copy()
					// The compiled bitfield sizes, not the preset label, determine
					// the supported subnet count.
					cfg.PresetBase = preset
					cfg.SyncCommitteeSubnetCount = count
					err := cfg.ValidateStateLayout()
					if count == 1 {
						require.NoError(t, err)
						return
					}
					require.ErrorContains(t, fmt.Sprintf("SYNC_COMMITTEE_SUBNET_COUNT is %d but this binary's SSZ state layout is compiled for 1", count), err)
				})
			}
		})
	}
}

func TestValidate_CommitteeSizeBounds(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.ConfigName, func(t *testing.T) {
			for _, size := range []uint64{0, 16, 31, 32, 33, 64, math.MaxUint64} {
				t.Run(fmt.Sprintf("committee_size_%d", size), func(t *testing.T) {
					cfg := base.Copy()
					cfg.MaxValidatorsPerCommittee = size
					capacity, err := cfg.MaxActiveValidators()
					if size == 0 || size > fieldparams.MaxValidatorsPerCommittee {
						require.ErrorContains(t, "MAX_VALIDATORS_PER_COMMITTEE", err)
						require.Equal(t, uint64(0), capacity)
						require.ErrorContains(t, "MAX_VALIDATORS_PER_COMMITTEE", cfg.Validate())
						require.ErrorContains(t, "MAX_VALIDATORS_PER_COMMITTEE", cfg.ValidateStateLayout())
						return
					}
					require.NoError(t, err)
					require.Equal(t, cfg.MaxCommitteesPerSlot*uint64(cfg.SlotsPerEpoch)*size, capacity)
					require.NoError(t, cfg.Validate())
					if cfg.PresetBase == fieldparams.Preset {
						require.NoError(t, cfg.ValidateStateLayout())
					}
				})
			}
		})
	}
}

func TestValidate_BlockOperationLimits(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig(), params.MinimalSpecConfig()} {
		t.Run(base.PresetBase, func(t *testing.T) {
			withdrawalLimit := uint64(16)
			if base.PresetBase == params.MinimalName {
				withdrawalLimit = 4
			}
			for _, operation := range []struct {
				name  string
				field func(*params.BeaconChainConfig) *uint64
				limit uint64
			}{
				{"MAX_PROPOSER_SLASHINGS", func(c *params.BeaconChainConfig) *uint64 { return &c.MaxProposerSlashings }, 16},
				{"MAX_ATTESTER_SLASHINGS", func(c *params.BeaconChainConfig) *uint64 { return &c.MaxAttesterSlashings }, 2},
				{"MAX_ATTESTATIONS", func(c *params.BeaconChainConfig) *uint64 { return &c.MaxAttestations }, 4},
				{"MAX_DEPOSITS", func(c *params.BeaconChainConfig) *uint64 { return &c.MaxDeposits }, 16},
				{"MAX_VOLUNTARY_EXITS", func(c *params.BeaconChainConfig) *uint64 { return &c.MaxVoluntaryExits }, 16},
				{"MAX_WITHDRAWALS_PER_PAYLOAD", func(c *params.BeaconChainConfig) *uint64 { return &c.MaxWithdrawalsPerPayload }, withdrawalLimit},
			} {
				for _, value := range []uint64{0, 1, operation.limit - 1, operation.limit, operation.limit + 1, math.MaxUint64} {
					t.Run(fmt.Sprintf("%s/%d", operation.name, value), func(t *testing.T) {
						cfg := base.Copy()
						*operation.field(cfg) = value
						input := fmt.Sprintf("PRESET_BASE: %s\n%s: %d\n", base.PresetBase, operation.name, value)
						loaded, err := params.UnmarshalConfig([]byte(input), nil)
						checks := map[string]error{"validation": cfg.Validate(), "YAML loading": err}
						if base.PresetBase == fieldparams.Preset {
							checks["compiled layout"] = cfg.ValidateStateLayout()
						}
						want := ""
						if value > operation.limit {
							want = fmt.Sprintf("%s (%d) must not exceed the SSZ block operation limit (%d)", operation.name, value, operation.limit)
						} else if operation.name == "MAX_WITHDRAWALS_PER_PAYLOAD" && value == 0 {
							want = "MAX_WITHDRAWALS_PER_PAYLOAD must be non-zero"
						}
						for name, err := range checks {
							if want != "" {
								require.ErrorContains(t, want, err, name)
							} else {
								require.NoError(t, err, name)
							}
						}
						if want != "" {
							require.Equal(t, true, loaded == nil)
						} else {
							require.Equal(t, value, *operation.field(loaded))
						}
					})
				}
			}
		})
	}
}

func TestValidateStateLayout_BlockOperationsUseCompiledPreset(t *testing.T) {
	base := params.MainnetConfig()
	if fieldparams.Preset == params.MinimalName {
		base = params.MinimalSpecConfig()
	}
	// Changing the preset label must not bypass the actual binary's bounds.
	for _, preset := range []string{params.MainnetName, params.MinimalName, "custom"} {
		t.Run(preset, func(t *testing.T) {
			cfg := base.Copy()
			cfg.PresetBase = preset
			cfg.MaxWithdrawalsPerPayload = fieldparams.MaxWithdrawalsPerPayload
			require.NoError(t, cfg.ValidateStateLayout())
			cfg.MaxWithdrawalsPerPayload++
			require.ErrorContains(t, "MAX_WITHDRAWALS_PER_PAYLOAD", cfg.ValidateStateLayout())
		})
	}
}

func TestUnmarshalConfig_RejectsUnsafeOverrides(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{input: "SECONDS_PER_SLOT: 0\n", want: "SECONDS_PER_SLOT must be non-zero"},
		{input: "MAX_VALIDATORS_PER_COMMITTEE: 33\n", want: "SSZ attestation limit (32)"},
		{input: "MAX_VALIDATORS_PER_COMMITTEE: 64\n", want: "SSZ attestation limit (32)"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			cfg, err := params.UnmarshalConfig([]byte("CONFIG_NAME: mainnet\n"+tc.input), nil)
			require.ErrorContains(t, "invalid chain config", err)
			require.ErrorContains(t, tc.want, err)
			require.Equal(t, true, cfg == nil)
		})
	}
}

func TestUnmarshalConfig_RejectsInvalidArithmetic(t *testing.T) {
	// A chain config file overriding a single value on top of the mainnet
	// defaults; the override breaks process_slashings.
	yaml := []byte("PROPORTIONAL_SLASHING_MULTIPLIER: 0\n")
	_, err := params.UnmarshalConfig(yaml, nil)
	require.ErrorContains(t, "PROPORTIONAL_SLASHING_MULTIPLIER must be non-zero", err)
}
