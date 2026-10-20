package params_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/io/file"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"gopkg.in/yaml.v2"
)

// Variables defined in the placeholderFields will not be tested in `TestLoadConfigFile`.
// These are variables that we don't use in Qrysm. (i.e. future hardfork, light client... etc)
// IMPORTANT: Use one field per line and sort these alphabetically to reduce conflicts.
var placeholderFields = []string{
	"ATTESTATION_PROPAGATION_SLOT_RANGE",
	"ATTESTATION_SUBNET_COUNT",
	"ATTESTATION_SUBNET_EXTRA_BITS",
	"ATTESTATION_SUBNET_PREFIX_BITS",
	"EIP6110_FORK_EPOCH",
	"EIP6110_FORK_VERSION",
	"EIP7002_FORK_EPOCH",
	"EIP7002_FORK_VERSION",
	"EPOCHS_PER_SUBNET_SUBSCRIPTION",
	"GOSSIP_MAX_SIZE",
	"MAXIMUM_GOSSIP_CLOCK_DISPARITY",
	"MAX_CHUNK_SIZE",
	"MAX_REQUEST_BLOCKS",
	"MESSAGE_DOMAIN_INVALID_SNAPPY",
	"MESSAGE_DOMAIN_VALID_SNAPPY",
	"MIN_EPOCHS_FOR_BLOCK_REQUESTS",
	"RESP_TIMEOUT",
	"SUBNETS_PER_NODE",
	"TTFB_TIMEOUT",
	"UPDATE_TIMEOUT",
	"WHISK_EPOCHS_PER_SHUFFLING_PHASE",
	"WHISK_FORK_EPOCH",
	"WHISK_FORK_VERSION",
	"WHISK_PROPOSER_SELECTION_GAP",
}

func TestPlaceholderFieldsDistinctSorted(t *testing.T) {
	m := make(map[string]struct{})
	for i := 0; i < len(placeholderFields)-1; i++ {
		if _, ok := m[placeholderFields[i]]; ok {
			t.Fatalf("duplicate placeholder field %s", placeholderFields[i])
		}
		m[placeholderFields[i]] = struct{}{}
	}
	if !sort.StringsAreSorted(placeholderFields) {
		t.Fatal("placeholderFields must be sorted")
	}
}

func assertEqualConfigs(t *testing.T, name string, fields []string, expected, actual *params.BeaconChainConfig) {
	//  Misc params.
	assert.Equal(t, expected.MaxCommitteesPerSlot, actual.MaxCommitteesPerSlot, "%s: MaxCommitteesPerSlot", name)
	assert.Equal(t, expected.TargetCommitteeSize, actual.TargetCommitteeSize, "%s: TargetCommitteeSize", name)
	assert.Equal(t, expected.MaxValidatorsPerCommittee, actual.MaxValidatorsPerCommittee, "%s: MaxValidatorsPerCommittee", name)
	assert.Equal(t, expected.MinPerEpochChurnLimit, actual.MinPerEpochChurnLimit, "%s: MinPerEpochChurnLimit", name)
	assert.Equal(t, expected.ChurnLimitQuotient, actual.ChurnLimitQuotient, "%s: ChurnLimitQuotient", name)
	assert.Equal(t, expected.ShuffleRoundCount, actual.ShuffleRoundCount, "%s: ShuffleRoundCount", name)
	assert.Equal(t, expected.MinGenesisActiveValidatorCount, actual.MinGenesisActiveValidatorCount, "%s: MinGenesisActiveValidatorCount", name)
	assert.Equal(t, expected.MinGenesisTime, actual.MinGenesisTime, "%s: MinGenesisTime", name)
	assert.Equal(t, expected.HysteresisQuotient, actual.HysteresisQuotient, "%s: HysteresisQuotient", name)
	assert.Equal(t, expected.HysteresisDownwardMultiplier, actual.HysteresisDownwardMultiplier, "%s: HysteresisDownwardMultiplier", name)
	assert.Equal(t, expected.HysteresisUpwardMultiplier, actual.HysteresisUpwardMultiplier, "%s: HysteresisUpwardMultiplier", name)

	// Fork Choice params.
	assert.Equal(t, expected.DeprecatedSafeSlotsToUpdateJustified, actual.DeprecatedSafeSlotsToUpdateJustified, "%s: SafeSlotsToUpdateJustified", name)

	// Validator params.
	assert.Equal(t, expected.ExecutionFollowDistance, actual.ExecutionFollowDistance, "%s: ExecutionFollowDistance", name)
	assert.Equal(t, expected.TargetAggregatorsPerCommittee, actual.TargetAggregatorsPerCommittee, "%s: TargetAggregatorsPerCommittee", name)
	assert.Equal(t, expected.RandomSubnetsPerValidator, actual.RandomSubnetsPerValidator, "%s: RandomSubnetsPerValidator", name)
	assert.Equal(t, expected.EpochsPerRandomSubnetSubscription, actual.EpochsPerRandomSubnetSubscription, "%s: EpochsPerRandomSubnetSubscription", name)
	assert.Equal(t, expected.SecondsPerExecutionBlock, actual.SecondsPerExecutionBlock, "%s: SecondsPerExecutionBlock", name)

	// Deposit contract.
	assert.Equal(t, expected.DepositChainID, actual.DepositChainID, "%s: DepositChainID", name)
	assert.Equal(t, expected.DepositNetworkID, actual.DepositNetworkID, "%s: DepositNetworkID", name)
	assert.Equal(t, expected.DepositContractAddress, actual.DepositContractAddress, "%s: DepositContractAddress", name)

	// Shor values.
	assert.Equal(t, expected.MinDepositAmount, actual.MinDepositAmount, "%s: MinDepositAmount", name)
	assert.Equal(t, expected.MaxEffectiveBalance, actual.MaxEffectiveBalance, "%s: MaxEffectiveBalance", name)
	assert.Equal(t, expected.EjectionBalance, actual.EjectionBalance, "%s: EjectionBalance", name)
	assert.Equal(t, expected.EffectiveBalanceIncrement, actual.EffectiveBalanceIncrement, "%s: EffectiveBalanceIncrement", name)

	// Initial values.
	assert.DeepEqual(t, expected.GenesisForkVersion, actual.GenesisForkVersion, "%s: GenesisForkVersion", name)

	// Time parameters.
	assert.Equal(t, expected.GenesisDelay, actual.GenesisDelay, "%s: GenesisDelay", name)
	assert.Equal(t, expected.SecondsPerSlot, actual.SecondsPerSlot, "%s: SecondsPerSlot", name)
	assert.Equal(t, expected.MinAttestationInclusionDelay, actual.MinAttestationInclusionDelay, "%s: MinAttestationInclusionDelay", name)
	assert.Equal(t, expected.SlotsPerEpoch, actual.SlotsPerEpoch, "%s: SlotsPerEpoch", name)
	assert.Equal(t, expected.MinSeedLookahead, actual.MinSeedLookahead, "%s: MinSeedLookahead", name)
	assert.Equal(t, expected.MaxSeedLookahead, actual.MaxSeedLookahead, "%s: MaxSeedLookahead", name)
	assert.Equal(t, expected.EpochsPerExecutionVotingPeriod, actual.EpochsPerExecutionVotingPeriod, "%s: EpochsPerExecutionVotingPeriod", name)
	assert.Equal(t, expected.SlotsPerHistoricalRoot, actual.SlotsPerHistoricalRoot, "%s: SlotsPerHistoricalRoot", name)
	assert.Equal(t, expected.MinValidatorWithdrawabilityDelay, actual.MinValidatorWithdrawabilityDelay, "%s: MinValidatorWithdrawabilityDelay", name)
	assert.Equal(t, expected.ShardCommitteePeriod, actual.ShardCommitteePeriod, "%s: ShardCommitteePeriod", name)
	assert.Equal(t, expected.MinEpochsToInactivityPenalty, actual.MinEpochsToInactivityPenalty, "%s: MinEpochsToInactivityPenalty", name)

	// State vector lengths.
	assert.Equal(t, expected.EpochsPerHistoricalVector, actual.EpochsPerHistoricalVector, "%s: EpochsPerHistoricalVector", name)
	assert.Equal(t, expected.EpochsPerSlashingsVector, actual.EpochsPerSlashingsVector, "%s: EpochsPerSlashingsVector", name)
	assert.Equal(t, expected.HistoricalRootsLimit, actual.HistoricalRootsLimit, "%s: HistoricalRootsLimit", name)
	assert.Equal(t, expected.ValidatorRegistryLimit, actual.ValidatorRegistryLimit, "%s: ValidatorRegistryLimit", name)

	// Reward and penalty quotients.
	assert.Equal(t, expected.BaseRewardFactor, actual.BaseRewardFactor, "%s: BaseRewardFactor", name)
	assert.Equal(t, expected.WhistleBlowerRewardQuotient, actual.WhistleBlowerRewardQuotient, "%s: WhistleBlowerRewardQuotient", name)
	assert.Equal(t, expected.ProposerRewardQuotient, actual.ProposerRewardQuotient, "%s: ProposerRewardQuotient", name)
	assert.Equal(t, expected.InactivityPenaltyQuotient, actual.InactivityPenaltyQuotient, "%s: InactivityPenaltyQuotientZond", name)
	assert.Equal(t, expected.MinSlashingPenaltyQuotient, actual.MinSlashingPenaltyQuotient, "%s: MinSlashingPenaltyQuotientZond", name)
	assert.Equal(t, expected.ProportionalSlashingMultiplier, actual.ProportionalSlashingMultiplier, "%s: ProportionalSlashingMultiplierZond", name)

	// Max operations per block.
	assert.Equal(t, expected.MaxProposerSlashings, actual.MaxProposerSlashings, "%s: MaxProposerSlashings", name)
	assert.Equal(t, expected.MaxAttesterSlashings, actual.MaxAttesterSlashings, "%s: MaxAttesterSlashings", name)
	assert.Equal(t, expected.MaxAttestations, actual.MaxAttestations, "%s: MaxAttestations", name)
	assert.Equal(t, expected.MaxDeposits, actual.MaxDeposits, "%s: MaxDeposits", name)
	assert.Equal(t, expected.MaxVoluntaryExits, actual.MaxVoluntaryExits, "%s: MaxVoluntaryExits", name)

	// Signature domains.
	assert.Equal(t, expected.DomainBeaconProposer, actual.DomainBeaconProposer, "%s: DomainBeaconProposer", name)
	assert.Equal(t, expected.DomainBeaconAttester, actual.DomainBeaconAttester, "%s: DomainBeaconAttester", name)
	assert.Equal(t, expected.DomainRandao, actual.DomainRandao, "%s: DomainRandao", name)
	assert.Equal(t, expected.DomainDeposit, actual.DomainDeposit, "%s: DomainDeposit", name)
	assert.Equal(t, expected.DomainVoluntaryExit, actual.DomainVoluntaryExit, "%s: DomainVoluntaryExit", name)
	assert.Equal(t, expected.DomainSelectionProof, actual.DomainSelectionProof, "%s: DomainSelectionProof", name)
	assert.Equal(t, expected.DomainAggregateAndProof, actual.DomainAggregateAndProof, "%s: DomainAggregateAndProof", name)
	assert.Equal(t, expected.SqrRootSlotsPerEpoch, actual.SqrRootSlotsPerEpoch, "%s: SqrRootSlotsPerEpoch", name)
	assert.DeepEqual(t, expected.GenesisForkVersion, actual.GenesisForkVersion, "%s: GenesisForkVersion", name)

	assertYamlFieldsMatch(t, name, fields, expected, actual)
}

func TestModifiedE2E(t *testing.T) {
	c := params.E2ETestConfig().Copy()
	c.DepositContractAddress = "Q42424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242"
	c.MaxPerEpochActivationChurnLimit = 3
	y := params.ConfigToYaml(c)
	cfg, err := params.UnmarshalConfig(y, nil)
	require.NoError(t, err)
	assertEqualConfigs(t, "modified-e2e", []string{}, c, cfg)
}

func TestUnmarshalConfig_RejectsMalformedYAML(t *testing.T) {
	for _, tc := range []struct {
		name      string
		input     string
		wantError string
		typeError bool
	}{
		{name: "invalid scalar", input: "SECONDS_PER_SLOT: broken\n", wantError: "cannot unmarshal", typeError: true},
		{name: "unknown field", input: "SECONDS_PER_SOLT: 12\n", wantError: "field SECONDS_PER_SOLT not found", typeError: true},
		{name: "duplicate key", input: "SECONDS_PER_SLOT: 12\nSECONDS_PER_SLOT: 24\n", wantError: "already set", typeError: true},
		{name: "negative unsigned value", input: "SECONDS_PER_SLOT: -1\n", wantError: "cannot unmarshal", typeError: true},
		{name: "sequence instead of scalar", input: "SECONDS_PER_SLOT: [12, 24]\n", wantError: "cannot unmarshal", typeError: true},
		{name: "malformed syntax", input: "SECONDS_PER_SLOT: [12\n", wantError: "did not find expected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, seed := range []string{"default preset", "supplied config"} {
				t.Run(seed, func(t *testing.T) {
					var base, before *params.BeaconChainConfig
					if seed == "supplied config" {
						base = params.MainnetConfig().Copy()
						before = base.Copy()
					}
					// Valid overrides before the error must not leak into the seed,
					// including the byte slice used to initialize the fork schedule.
					input := "CONFIG_NAME: mainnet\nGENESIS_DELAY: 123\nGENESIS_FORK_VERSION: 0x11223344\n" + tc.input
					got, err := params.UnmarshalConfig([]byte(input), base)
					require.ErrorContains(t, "Failed to parse chain config yaml file", err)
					require.ErrorContains(t, tc.wantError, err)
					require.Equal(t, true, got == nil)
					if tc.typeError {
						var yamlErr *yaml.TypeError
						require.Equal(t, true, errors.As(err, &yamlErr))
					}
					if base != nil {
						require.DeepEqual(t, before, base)
					}
				})
			}
		})
	}
}

func TestUnmarshalConfig_RejectsMalformedHex(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  error
	}{
		{name: "invalid suffix", value: "0x11223344zz", want: hex.InvalidByteError('z')},
		{name: "odd length", value: "0x112233445", want: hex.ErrLength},
		{name: "invalid first byte", value: "0xgg11223344", want: hex.InvalidByteError('g')},
		{name: "repeated prefix", value: "0x112233440x55", want: hex.InvalidByteError('x')},
		{name: "embedded whitespace", value: "0x11223344 zz", want: hex.InvalidByteError(' ')},
		{name: "unseparated comment", value: "0x11223344# comment", want: hex.InvalidByteError('#')},
		{name: "invalid suffix before comment", value: "0x11223344zz # comment", want: hex.InvalidByteError('z')},
		{name: "empty value", value: "0x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, seed := range []string{"default preset", "supplied config"} {
				t.Run(seed, func(t *testing.T) {
					var base, before *params.BeaconChainConfig
					if seed == "supplied config" {
						base = params.MainnetConfig().Copy()
						before = base.Copy()
					}
					input := "CONFIG_NAME: mainnet\nGENESIS_DELAY: 123\nGENESIS_FORK_VERSION: " + tc.value + "\n"
					got, err := params.UnmarshalConfig([]byte(input), base)
					require.ErrorContains(t, "Failed to parse chain config yaml file at line 3", err)
					require.ErrorContains(t, "failed to decode hex string", err)
					require.Equal(t, true, got == nil)
					if tc.want != nil {
						require.Equal(t, true, errors.Is(err, tc.want))
					}
					if base != nil {
						require.DeepEqual(t, before, base)
					}
				})
			}
		})
	}
}

func TestUnmarshalConfig_ForkVersionLength(t *testing.T) {
	for _, tc := range []struct {
		value  string
		length int
	}{
		{value: "[]", length: 0},
		{value: "[1]", length: 1},
		{value: "[1, 2]", length: 2},
		{value: "[1, 2, 3]", length: 3},
		{value: "[1, 2, 3, 4, 5]", length: 5},
		{value: "0x11", length: 1},
		{value: "0x1122", length: 2},
		{value: "0x112233", length: 3},
		{value: "0x1122334455", length: 5},
		{value: "[17, 34, 51, 68]", length: 4},
		{value: "0x11223344", length: 4},
	} {
		t.Run(tc.value, func(t *testing.T) {
			for _, seed := range []string{"default preset", "supplied config"} {
				t.Run(seed, func(t *testing.T) {
					var base, before *params.BeaconChainConfig
					if seed == "supplied config" {
						base = params.MainnetConfig().Copy()
						before = base.Copy()
					}
					input := "CONFIG_NAME: mainnet\nGENESIS_DELAY: 123\nGENESIS_FORK_VERSION: " + tc.value + "\n"
					got, err := params.UnmarshalConfig([]byte(input), base)
					if tc.length == 4 {
						require.NoError(t, err)
						require.DeepEqual(t, []byte{0x11, 0x22, 0x33, 0x44}, got.GenesisForkVersion)
						epoch, exists := got.ForkVersionSchedule[[4]byte{0x11, 0x22, 0x33, 0x44}]
						require.Equal(t, true, exists)
						require.Equal(t, got.GenesisEpoch, epoch)
					} else {
						if strings.HasPrefix(tc.value, "0x") {
							require.ErrorContains(t, "Failed to parse chain config yaml file", err)
						} else {
							require.ErrorContains(t, "invalid chain config", err)
						}
						require.ErrorContains(t, fmt.Sprintf("GENESIS_FORK_VERSION must be exactly 4 bytes, got %d", tc.length), err)
						require.Equal(t, true, got == nil)
					}
					if base != nil {
						require.DeepEqual(t, before, base)
					}
				})
			}
		})
	}
}

func TestUnmarshalConfig_ShortHexForkVersionSyntax(t *testing.T) {
	for _, tc := range []struct {
		input  string
		length int
	}{
		{"GENESIS_FORK_VERSION : 0x1122 # comment\n", 2},
		{"'GENESIS_FORK_VERSION': 0x112233\n", 3},
		{"\"GENESIS_FORK_VERSION\": 0x1122\n", 2},
		{"\"\\x47ENESIS_FORK_VERSION\": 0x112233\n", 3},
		{"GENESIS_FORK_VERSION:\n  0x1122\n", 2},
		{"? GENESIS_FORK_VERSION\n: 0x112233\n", 3},
		{"DOMAIN_BEACON_PROPOSER: &version 0x1122\nGENESIS_FORK_VERSION: *version\n", 2},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := params.UnmarshalConfig([]byte(tc.input), nil)
			require.ErrorContains(t, fmt.Sprintf("GENESIS_FORK_VERSION must be exactly 4 bytes, got %d", tc.length), err)
			require.Equal(t, true, got == nil)
		})
	}
}

func TestUnmarshalConfig_ValidOverridesPreserveSeed(t *testing.T) {
	for _, base := range []*params.BeaconChainConfig{params.MainnetConfig().Copy(), params.MinimalSpecConfig().Copy()} {
		t.Run(base.ConfigName, func(t *testing.T) {
			before := base.Copy()
			input := []byte("SECONDS_PER_SLOT: 12\nSLOTS_PER_EPOCH: 64\nGENESIS_FORK_VERSION: 0x11223344\n")
			got, err := params.UnmarshalConfig(input, base)
			require.NoError(t, err)
			require.Equal(t, uint64(12), got.SecondsPerSlot)
			require.Equal(t, uint64(64), uint64(got.SlotsPerEpoch))
			require.Equal(t, uint64(8), uint64(got.SqrRootSlotsPerEpoch))
			require.Equal(t, params.DevnetName, got.ConfigName)
			require.Equal(t, base.PresetBase, got.PresetBase)
			require.Equal(t, base.MaxEffectiveBalance, got.MaxEffectiveBalance)
			require.DeepEqual(t, []byte{0x11, 0x22, 0x33, 0x44}, got.GenesisForkVersion)
			_, exists := got.ForkVersionSchedule[[4]byte{0x11, 0x22, 0x33, 0x44}]
			require.Equal(t, true, exists)
			require.DeepEqual(t, before, base)
		})
	}
}

func TestUnmarshalConfig_HexWhitespaceAndComments(t *testing.T) {
	for _, suffix := range []string{"", " \t\r", " # another value: 0xzz", "\t# comment"} {
		t.Run(suffix, func(t *testing.T) {
			input := "# GENESIS_FORK_VERSION: 0xzz\n" +
				"  # Invalid example: 0x11223344zz\n" +
				"SECONDS_PER_SLOT: 12 # Hex example: 0xzz\n" +
				"GENESIS_FORK_VERSION: 0x11223344" + suffix + "\n"
			got, err := params.UnmarshalConfig([]byte(input), nil)
			require.NoError(t, err)
			require.Equal(t, uint64(12), got.SecondsPerSlot)
			require.DeepEqual(t, []byte{0x11, 0x22, 0x33, 0x44}, got.GenesisForkVersion)
		})
	}
}

func TestLoadChainConfigFile_ErrorPreservesActiveConfig(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{input: "SECONDS_PER_SLOT: broken\n", want: "Failed to parse chain config yaml file"},
		{input: "SECONDS_PER_SOLT: 12\n", want: "Failed to parse chain config yaml file"},
		{input: "INTERVALS_PER_SLOT: 0\n", want: "INTERVALS_PER_SLOT must be non-zero"},
		{input: "SECONDS_PER_EXECUTION_BLOCK: 0\n", want: "SECONDS_PER_EXECUTION_BLOCK must be non-zero"},
		{input: "EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION: 0\n", want: "EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION must be non-zero"},
		{
			input: fmt.Sprintf("EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION: %d\n", uint64(math.MaxInt)+1),
			want:  fmt.Sprintf("EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION (%d) must not exceed %d", uint64(math.MaxInt)+1, math.MaxInt),
		},
		{input: "SHUFFLE_ROUND_COUNT: 256\n", want: "SHUFFLE_ROUND_COUNT (256) must not exceed 255"},
		{input: "SHUFFLE_ROUND_COUNT: 257\n", want: "SHUFFLE_ROUND_COUNT (257) must not exceed 255"},
		{input: "TIMELY_TARGET_FLAG_INDEX: 8\n", want: "TIMELY_TARGET_FLAG_INDEX (8) must be between 0 and 7"},
		{input: "TIMELY_TARGET_FLAG_INDEX: 0\n", want: "TIMELY_SOURCE_FLAG_INDEX and TIMELY_TARGET_FLAG_INDEX must be distinct"},
		{input: "MIN_ATTESTATION_INCLUSION_DELAY: 129\n", want: "MIN_ATTESTATION_INCLUSION_DELAY (129) must not exceed SLOTS_PER_EPOCH (128)"},
		{
			input: "TIMELY_SOURCE_WEIGHT: 18446744073709551615\nTIMELY_TARGET_WEIGHT: 41\n",
			want:  "PROPOSER_WEIGHT overflows uint64",
		},
		{
			input: "WEIGHT_DENOMINATOR: 2251799813685248\nPROPOSER_WEIGHT: 2251799813685247\nTIMELY_SOURCE_WEIGHT: 1\nTIMELY_TARGET_WEIGHT: 0\nTIMELY_HEAD_WEIGHT: 0\nSYNC_REWARD_WEIGHT: 0\n",
			want:  "maximum active balance increments",
		},
		{input: "TARGET_AGGREGATORS_PER_COMMITTEE: 0\n", want: "TARGET_AGGREGATORS_PER_COMMITTEE must be non-zero"},
		{input: "TARGET_AGGREGATORS_PER_SYNC_SUBCOMMITTEE: 0\n", want: "TARGET_AGGREGATORS_PER_SYNC_SUBCOMMITTEE must be non-zero"},
		{input: "EPOCHS_PER_EXECUTION_VOTING_PERIOD: 0\n", want: "EPOCHS_PER_EXECUTION_VOTING_PERIOD must be non-zero"},
		{input: "INACTIVITY_SCORE_BIAS: 0\n", want: "INACTIVITY_SCORE_BIAS must be non-zero"},
		{
			input: "PROPOSER_WEIGHT: 64\nTIMELY_SOURCE_WEIGHT: 0\nTIMELY_TARGET_WEIGHT: 0\nTIMELY_HEAD_WEIGHT: 0\nSYNC_REWARD_WEIGHT: 0\n",
			want:  "PROPOSER_WEIGHT (64) must be less than WEIGHT_DENOMINATOR (64)",
		},
		{
			input: "INACTIVITY_PENALTY_QUOTIENT: 4611686018427387904\n",
			want:  "INACTIVITY_SCORE_BIAS (4) * INACTIVITY_PENALTY_QUOTIENT (4611686018427387904) overflows uint64",
		},
		{input: "DEPOSIT_CONTRACT_TREE_DEPTH: 31\n", want: "DEPOSIT_CONTRACT_TREE_DEPTH (31) must be 32"},
		{input: "DEPOSIT_CONTRACT_TREE_DEPTH: 33\n", want: "DEPOSIT_CONTRACT_TREE_DEPTH (33) must be 32"},
		{input: "GENESIS_FORK_VERSION: 0x11223344zz\n", want: "Failed to parse chain config yaml file"},
		{input: "GENESIS_FORK_VERSION: 0x112233445\n", want: "Failed to parse chain config yaml file"},
		{input: "GENESIS_FORK_VERSION: [1, 2, 3]\n", want: "GENESIS_FORK_VERSION must be exactly 4 bytes"},
		{input: "GENESIS_FORK_VERSION: 0x1122\n", want: "GENESIS_FORK_VERSION must be exactly 4 bytes, got 2"},
		{input: "GENESIS_FORK_VERSION: 0x112233\n", want: "GENESIS_FORK_VERSION must be exactly 4 bytes, got 3"},
		{input: "GENESIS_FORK_VERSION: 0x1122334455\n", want: "GENESIS_FORK_VERSION must be exactly 4 bytes"},
		{input: "MAX_PROPOSER_SLASHINGS: 17\n", want: "MAX_PROPOSER_SLASHINGS (17) must not exceed"},
		{input: "MAX_ATTESTER_SLASHINGS: 3\n", want: "MAX_ATTESTER_SLASHINGS (3) must not exceed"},
		{input: "MAX_ATTESTATIONS: 5\n", want: "MAX_ATTESTATIONS (5) must not exceed"},
		{input: "MAX_DEPOSITS: 17\n", want: "MAX_DEPOSITS (17) must not exceed"},
		{input: "MAX_VOLUNTARY_EXITS: 17\n", want: "MAX_VOLUNTARY_EXITS (17) must not exceed"},
		{input: "MAX_WITHDRAWALS_PER_PAYLOAD: 17\n", want: "MAX_WITHDRAWALS_PER_PAYLOAD (17) must not exceed"},
		{input: "MAX_WITHDRAWALS_PER_PAYLOAD: 0\n", want: "MAX_WITHDRAWALS_PER_PAYLOAD must be non-zero"},
	} {
		t.Run(strings.TrimSpace(tc.input), func(t *testing.T) {
			for _, useActiveSeed := range []bool{false, true} {
				name := "default preset"
				if useActiveSeed {
					name = "active config as seed"
				}
				t.Run(name, func(t *testing.T) {
					params.SetupTestConfigCleanup(t)
					active := params.BeaconConfig()
					before := active.Copy()
					configPath := filepath.Join(t.TempDir(), "config.yaml")
					require.NoError(t, os.WriteFile(configPath, []byte("GENESIS_DELAY: 123\n"+tc.input), 0600))
					var seed *params.BeaconChainConfig
					if useActiveSeed {
						seed = active
					}
					err := params.LoadChainConfigFile(configPath, seed)
					require.ErrorContains(t, tc.want, err)
					require.Equal(t, true, params.BeaconConfig() == active)
					require.DeepEqual(t, before, params.BeaconConfig())
					registered, err := params.ByName(before.ConfigName)
					require.NoError(t, err)
					require.Equal(t, true, registered == active)
				})
			}
		})
	}
}

func TestLoadConfigFile(t *testing.T) {
	// NOTE(now.youtrack.cloud/issue/TQ-4)
	/*
		t.Run("mainnet", func(t *testing.T) {
			mn := params.MainnetConfig().Copy()
			mainnetPresetsFiles := presetsFilePath(t, "mainnet")
			var err error
			for _, fp := range mainnetPresetsFiles {
				mn, err = params.UnmarshalConfigFile(fp, mn)
				require.NoError(t, err)
			}
			// configs loaded from file get the name 'devnet' unless they specify a specific name in the yaml itself.
			// since these are partial patches for presets, they do not have the config name
			mn.ConfigName = params.MainnetName
			mainnetConfigFile := configFilePath(t, "mainnet")
			mnf, err := params.UnmarshalConfigFile(mainnetConfigFile, nil)
			require.NoError(t, err)
			fields := fieldsFromYamls(t, append(mainnetPresetsFiles, mainnetConfigFile))
			assertEqualConfigs(t, "mainnet", fields, mn, mnf)
		})

		t.Run("minimal", func(t *testing.T) {
			min := params.MinimalSpecConfig().Copy()
			minimalPresetsFiles := presetsFilePath(t, "minimal")
			var err error
			for _, fp := range minimalPresetsFiles {
				min, err = params.UnmarshalConfigFile(fp, min)
				require.NoError(t, err)
			}
			// configs loaded from file get the name 'devnet' unless they specify a specific name in the yaml itself.
			// since these are partial patches for presets, they do not have the config name
			min.ConfigName = params.MinimalName
			minimalConfigFile := configFilePath(t, "minimal")
			minf, err := params.UnmarshalConfigFile(minimalConfigFile, nil)
			require.NoError(t, err)
			fields := fieldsFromYamls(t, append(minimalPresetsFiles, minimalConfigFile))
			assertEqualConfigs(t, "minimal", fields, min, minf)
		})
	*/

	t.Run("e2e", func(t *testing.T) {
		e2e, err := params.ByName(params.EndToEndName)
		require.NoError(t, err)
		configFile := "testdata/e2e_config.yaml"
		e2ef, err := params.UnmarshalConfigFile(configFile, nil)
		require.NoError(t, err)
		fields := fieldsFromYamls(t, []string{configFile})
		assertEqualConfigs(t, "e2e", fields, e2e, e2ef)
	})
}

func TestLoadConfigFile_OverwriteCorrectly(t *testing.T) {
	f, err := os.CreateTemp("", "")
	require.NoError(t, err)
	// Set current config to minimal config
	cfg := params.MinimalSpecConfig().Copy()
	params.FillTestVersions(cfg, 128)
	_, err = io.Copy(f, bytes.NewBuffer(params.ConfigToYaml(cfg)))
	require.NoError(t, err)

	// set active config to mainnet, so that we can confirm LoadChainConfigFile overrides it
	mainnet, err := params.ByName(params.MainnetName)
	require.NoError(t, err)
	undo, err := params.SetActiveWithUndo(mainnet)
	require.NoError(t, err)
	defer func() {
		err := undo()
		require.NoError(t, err)
	}()

	// load empty config file, so that it defaults to mainnet values
	require.NoError(t, params.LoadChainConfigFile(f.Name(), nil))
	if params.BeaconConfig().MinGenesisTime != cfg.MinGenesisTime {
		t.Errorf("Expected MinGenesisTime to be set to value written to config: %d found: %d",
			cfg.MinGenesisTime,
			params.BeaconConfig().MinGenesisTime)
	}
	if params.BeaconConfig().SlotsPerEpoch != cfg.SlotsPerEpoch {
		t.Errorf("Expected SlotsPerEpoch to be set to value written to config: %d found: %d",
			cfg.SlotsPerEpoch,
			params.BeaconConfig().SlotsPerEpoch)
	}
	require.Equal(t, params.MinimalName, params.BeaconConfig().ConfigName)
}

func Test_replaceHexStringWithYAMLFormat(t *testing.T) {

	testLines := []struct {
		line   string
		wanted string
	}{
		{
			line:   "ONE_BYTE: 0x41",
			wanted: "ONE_BYTE: 65\n",
		},
		{
			line:   "FOUR_BYTES: 0x41414141",
			wanted: "FOUR_BYTES: \n- 65\n- 65\n- 65\n- 65\n",
		},
		{
			line:   "THREE_BYTES: 0x414141",
			wanted: "THREE_BYTES: \n- 65\n- 65\n- 65\n- 0\n",
		},
		{
			line:   "EIGHT_BYTES: 0x4141414141414141",
			wanted: "EIGHT_BYTES: \n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n",
		},
		{
			line: "SIXTEEN_BYTES: 0x41414141414141414141414141414141",
			wanted: "SIXTEEN_BYTES: \n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n",
		},
		{
			line: "TWENTY_BYTES: 0x4141414141414141414141414141414141414141",
			wanted: "TWENTY_BYTES: \n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n",
		},
		{
			line: "THIRTY_TWO_BYTES: 0x4141414141414141414141414141414141414141414141414141414141414141",
			wanted: "THIRTY_TWO_BYTES: \n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n",
		},
		{
			line: "FORTY_EIGHT_BYTES: 0x41414141414141414141414141414141414141414141414141414141414141414141" +
				"4141414141414141414141414141",
			wanted: "FORTY_EIGHT_BYTES: \n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n",
		},
		{
			line: "NINETY_SIX_BYTES: 0x414141414141414141414141414141414141414141414141414141414141414141414141" +
				"4141414141414141414141414141414141414141414141414141414141414141414141414141414141414141414141" +
				"41414141414141414141414141",
			wanted: "NINETY_SIX_BYTES: \n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n" +
				"- 65\n- 65\n- 65\n- 65\n- 65\n- 65\n",
		},
	}
	for _, line := range testLines {
		parts, err := params.ReplaceHexStringWithYAMLFormat(line.line)
		require.NoError(t, err)
		res := strings.Join(parts, "\n")

		if res != line.wanted {
			t.Errorf("expected conversion to be: %v got: %v", line.wanted, res)
		}
	}
}

func TestReplaceHexStringWithYAMLFormat_RejectsMalformedHex(t *testing.T) {
	for _, value := range []string{"0x11223344zz", "0x112233445", "0x112233440x55", "0x"} {
		t.Run(value, func(t *testing.T) {
			parts, err := params.ReplaceHexStringWithYAMLFormat("FOUR_BYTES: " + value)
			require.ErrorContains(t, "failed to decode hex string", err)
			require.Equal(t, true, parts == nil)
		})
	}
}

func TestConfigParityYaml(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	testDir := bazel.TestTmpDir()
	yamlDir := filepath.Join(testDir, "config.yaml")

	testCfg := params.E2ETestConfig()
	yamlObj := params.ConfigToYaml(testCfg)
	assert.NoError(t, file.WriteFile(yamlDir, yamlObj))

	require.NoError(t, params.LoadChainConfigFile(yamlDir, params.E2ETestConfig().Copy()))
	assert.DeepEqual(t, params.BeaconConfig(), testCfg)
}

// configFilePath sets the proper config and returns the relevant
// config file path from eth2-spec-tests directory.
func configFilePath(t *testing.T, config string) string {
	fPath, err := bazel.Runfile("external/consensus_spec")
	require.NoError(t, err)
	configFilePath := path.Join(fPath, "configs", config+".yaml")
	return configFilePath
}

// presetsFilePath returns the relevant preset file paths from eth2-spec-tests
// directory. This method returns a preset file path for each hard fork or
// major network upgrade, in order.
func presetsFilePath(t *testing.T, config string) []string {
	fPath, err := bazel.Runfile("external/consensus_spec")
	require.NoError(t, err)
	return []string{
		path.Join(fPath, "presets", config, "phase0.yaml"),
		path.Join(fPath, "presets", config, "altair.yaml"),
	}
}

func fieldsFromYamls(t *testing.T, fps []string) []string {
	var keys []string
	for _, fp := range fps {
		yamlFile, err := os.ReadFile(fp)
		require.NoError(t, err)
		m := make(map[string]any)
		require.NoError(t, yaml.Unmarshal(yamlFile, &m))

		for k := range m {
			if k == "SHARDING_FORK_VERSION" || k == "SHARDING_FORK_EPOCH" {
				continue
			}
			keys = append(keys, k)
		}

		if len(keys) == 0 {
			t.Errorf("No fields loaded from yaml file %s", fp)
		}
	}

	return keys
}

func assertYamlFieldsMatch(t *testing.T, name string, fields []string, c1, c2 *params.BeaconChainConfig) {
	// Ensure all fields from the yaml file exist, were set, and correctly match the expected value.
	ft1 := reflect.TypeFor[params.BeaconChainConfig]()
	for _, field := range fields {
		var found bool
		for i := 0; i < ft1.NumField(); i++ {
			v, ok := ft1.Field(i).Tag.Lookup("yaml")
			if ok && v == field {
				if isPlaceholderField(v) {
					// If you see this error, remove the field from placeholderFields.
					t.Errorf("beacon config has a placeholder field defined, remove %s from the placeholder fields variable", v)
					continue
				}
				found = true
				v1 := reflect.ValueOf(*c1).Field(i).Interface()
				v2 := reflect.ValueOf(*c2).Field(i).Interface()
				if reflect.ValueOf(v1).Kind() == reflect.Slice {
					assert.DeepEqual(t, v1, v2, "%s: %s", name, field)
				} else {
					assert.Equal(t, v1, v2, "%s: %s", name, field)
				}
				break
			}
		}
		if !found && !isPlaceholderField(field) { // Ignore placeholder fields
			t.Errorf("No struct tag found `yaml:%s`", field)
		}
	}
}

func isPlaceholderField(field string) bool {
	return slices.Contains(placeholderFields, field)
}
