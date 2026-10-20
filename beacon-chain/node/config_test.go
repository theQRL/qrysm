package node

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-qrl/common"
	"github.com/theQRL/qrysm/cmd"
	"github.com/theQRL/qrysm/cmd/beacon-chain/flags"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/urfave/cli/v2"
)

func TestConfigureChainConfig_RejectsMalformedYAML(t *testing.T) {
	for _, input := range []string{
		"SECONDS_PER_SLOT: broken\n",
		"SECONDS_PER_SOLT: 12\n",
		"GENESIS_FORK_VERSION: 0x11223344zz\n",
		"GENESIS_FORK_VERSION: 0x112233445\n",
	} {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			before := params.BeaconConfig().Copy()
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(input), 0600))
			set := flag.NewFlagSet("test", flag.ContinueOnError)
			set.String(cmd.ChainConfigFileFlag.Name, "", "")
			require.NoError(t, set.Set(cmd.ChainConfigFileFlag.Name, configPath))
			cliCtx := cli.NewContext(&cli.App{}, set, nil)

			require.ErrorContains(t, "Failed to parse chain config yaml file", configureChainConfig(cliCtx))
			require.DeepEqual(t, before, params.BeaconConfig())
		})
	}
}

func TestConfigureChainConfig_RejectsUnsafeOverrides(t *testing.T) {
	for _, tc := range []struct {
		input  string
		mutate func(*params.BeaconChainConfig)
		want   string
	}{
		{
			input:  "SECONDS_PER_SLOT: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.SecondsPerSlot = 0 },
			want:   "SECONDS_PER_SLOT must be non-zero",
		},
		{
			input:  "INTERVALS_PER_SLOT: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.IntervalsPerSlot = 0 },
			want:   "INTERVALS_PER_SLOT must be non-zero",
		},
		{
			input:  "SECONDS_PER_EXECUTION_BLOCK: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.SecondsPerExecutionBlock = 0 },
			want:   "SECONDS_PER_EXECUTION_BLOCK must be non-zero",
		},
		{
			input:  "EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.EpochsPerRandomSubnetSubscription = 0 },
			want:   "EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION must be non-zero",
		},
		{
			input:  fmt.Sprintf("EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION: %d\n", uint64(math.MaxInt)+1),
			mutate: func(cfg *params.BeaconChainConfig) { cfg.EpochsPerRandomSubnetSubscription = uint64(math.MaxInt) + 1 },
			want:   fmt.Sprintf("EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION (%d) must not exceed %d", uint64(math.MaxInt)+1, math.MaxInt),
		},
		{
			input:  "SHUFFLE_ROUND_COUNT: 256\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.ShuffleRoundCount = 256 },
			want:   "SHUFFLE_ROUND_COUNT (256) must not exceed 255",
		},
		{
			input:  "SHUFFLE_ROUND_COUNT: 18446744073709551615\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.ShuffleRoundCount = math.MaxUint64 },
			want:   "SHUFFLE_ROUND_COUNT (18446744073709551615) must not exceed 255",
		},
		{
			input:  "TIMELY_TARGET_FLAG_INDEX: 8\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.TimelyTargetFlagIndex = 8 },
			want:   "TIMELY_TARGET_FLAG_INDEX (8) must be between 0 and 7",
		},
		{
			input:  "TIMELY_TARGET_FLAG_INDEX: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.TimelyTargetFlagIndex = 0 },
			want:   "TIMELY_SOURCE_FLAG_INDEX and TIMELY_TARGET_FLAG_INDEX must be distinct",
		},
		{
			input:  "MIN_ATTESTATION_INCLUSION_DELAY: 129\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.MinAttestationInclusionDelay = 129 },
			want:   "MIN_ATTESTATION_INCLUSION_DELAY (129) must not exceed SLOTS_PER_EPOCH (128)",
		},
		{
			input: "TIMELY_SOURCE_WEIGHT: 18446744073709551615\nTIMELY_TARGET_WEIGHT: 41\n",
			mutate: func(cfg *params.BeaconChainConfig) {
				cfg.TimelySourceWeight = math.MaxUint64
				cfg.TimelyTargetWeight = 41
			},
			want: "PROPOSER_WEIGHT overflows uint64",
		},
		{
			input: "WEIGHT_DENOMINATOR: 2251799813685248\nPROPOSER_WEIGHT: 2251799813685247\nTIMELY_SOURCE_WEIGHT: 1\nTIMELY_TARGET_WEIGHT: 0\nTIMELY_HEAD_WEIGHT: 0\nSYNC_REWARD_WEIGHT: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) {
				cfg.WeightDenominator = 1 << 51
				cfg.ProposerWeight = 1<<51 - 1
				cfg.TimelySourceWeight = 1
				cfg.TimelyTargetWeight = 0
				cfg.TimelyHeadWeight = 0
				cfg.SyncRewardWeight = 0
			},
			want: "maximum active balance increments",
		},
		{
			input:  "TARGET_AGGREGATORS_PER_COMMITTEE: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.TargetAggregatorsPerCommittee = 0 },
			want:   "TARGET_AGGREGATORS_PER_COMMITTEE must be non-zero",
		},
		{
			input:  "TARGET_AGGREGATORS_PER_SYNC_SUBCOMMITTEE: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.TargetAggregatorsPerSyncSubcommittee = 0 },
			want:   "TARGET_AGGREGATORS_PER_SYNC_SUBCOMMITTEE must be non-zero",
		},
		{
			input:  "SYNC_COMMITTEE_SUBNET_COUNT: 2\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.SyncCommitteeSubnetCount = 2 },
			want:   "SYNC_COMMITTEE_SUBNET_COUNT is 2 but this binary's SSZ state layout is compiled for 1",
		},
		{
			input:  "DEPOSIT_CONTRACT_TREE_DEPTH: 31\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.DepositContractTreeDepth = 31 },
			want:   "DEPOSIT_CONTRACT_TREE_DEPTH (31) must be 32 to match the SSZ deposit proof length (33)",
		},
		{
			input:  "DEPOSIT_CONTRACT_TREE_DEPTH: 33\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.DepositContractTreeDepth = 33 },
			want:   "DEPOSIT_CONTRACT_TREE_DEPTH (33) must be 32 to match the SSZ deposit proof length (33)",
		},
		{
			input:  "INACTIVITY_SCORE_BIAS: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.InactivityScoreBias = 0 },
			want:   "INACTIVITY_SCORE_BIAS must be non-zero",
		},
		{
			input: "PROPOSER_WEIGHT: 64\nTIMELY_SOURCE_WEIGHT: 0\nTIMELY_TARGET_WEIGHT: 0\nTIMELY_HEAD_WEIGHT: 0\nSYNC_REWARD_WEIGHT: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) {
				cfg.ProposerWeight = 64
				cfg.TimelySourceWeight = 0
				cfg.TimelyTargetWeight = 0
				cfg.TimelyHeadWeight = 0
				cfg.SyncRewardWeight = 0
			},
			want: "PROPOSER_WEIGHT (64) must be less than WEIGHT_DENOMINATOR (64)",
		},
		{
			input:  "INACTIVITY_PENALTY_QUOTIENT: 4611686018427387904\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.InactivityPenaltyQuotient = 1 << 62 },
			want:   "INACTIVITY_SCORE_BIAS (4) * INACTIVITY_PENALTY_QUOTIENT (4611686018427387904) overflows uint64",
		},
		{
			input:  "MAX_VALIDATORS_PER_COMMITTEE: 64\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.MaxValidatorsPerCommittee = 64 },
			want:   "SSZ attestation limit (32)",
		},
		{
			input:  "GENESIS_FORK_VERSION: [1, 2, 3]\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.GenesisForkVersion = []byte{1, 2, 3} },
			want:   "GENESIS_FORK_VERSION must be exactly 4 bytes",
		},
		{
			input:  "GENESIS_FORK_VERSION: 0x1122334455\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.GenesisForkVersion = []byte{0x11, 0x22, 0x33, 0x44, 0x55} },
			want:   "GENESIS_FORK_VERSION must be exactly 4 bytes",
		},
		{
			input:  "GENESIS_FORK_VERSION: 0x1122\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.GenesisForkVersion = []byte{0x11, 0x22} },
			want:   "GENESIS_FORK_VERSION must be exactly 4 bytes, got 2",
		},
		{
			input:  "GENESIS_FORK_VERSION: 0x112233\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.GenesisForkVersion = []byte{0x11, 0x22, 0x33} },
			want:   "GENESIS_FORK_VERSION must be exactly 4 bytes, got 3",
		},
		{
			input:  "MAX_PROPOSER_SLASHINGS: 17\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.MaxProposerSlashings = 17 },
			want:   "MAX_PROPOSER_SLASHINGS (17) must not exceed the SSZ block operation limit (16)",
		},
		{
			input:  "MAX_ATTESTER_SLASHINGS: 3\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.MaxAttesterSlashings = 3 },
			want:   "MAX_ATTESTER_SLASHINGS (3) must not exceed the SSZ block operation limit (2)",
		},
		{
			input:  "MAX_ATTESTATIONS: 5\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.MaxAttestations = 5 },
			want:   "MAX_ATTESTATIONS (5) must not exceed the SSZ block operation limit (4)",
		},
		{
			input:  "MAX_DEPOSITS: 17\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.MaxDeposits = 17 },
			want:   "MAX_DEPOSITS (17) must not exceed the SSZ block operation limit (16)",
		},
		{
			input:  "MAX_VOLUNTARY_EXITS: 17\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.MaxVoluntaryExits = 17 },
			want:   "MAX_VOLUNTARY_EXITS (17) must not exceed the SSZ block operation limit (16)",
		},
		{
			input:  "MAX_WITHDRAWALS_PER_PAYLOAD: 17\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.MaxWithdrawalsPerPayload = 17 },
			want:   "MAX_WITHDRAWALS_PER_PAYLOAD (17) must not exceed the SSZ block operation limit (16)",
		},
		{
			input:  "MAX_WITHDRAWALS_PER_PAYLOAD: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.MaxWithdrawalsPerPayload = 0 },
			want:   "MAX_WITHDRAWALS_PER_PAYLOAD must be non-zero",
		},
		{
			input:  "EPOCHS_PER_EXECUTION_VOTING_PERIOD: 5\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.EpochsPerExecutionVotingPeriod = 5 },
			want:   "EPOCHS_PER_EXECUTION_VOTING_PERIOD * SLOTS_PER_EPOCH is 640",
		},
		{
			input:  "EPOCHS_PER_EXECUTION_VOTING_PERIOD: 3\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.EpochsPerExecutionVotingPeriod = 3 },
			want:   "EPOCHS_PER_EXECUTION_VOTING_PERIOD * SLOTS_PER_EPOCH is 384",
		},
		{
			input:  "EPOCHS_PER_EXECUTION_VOTING_PERIOD: 0\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.EpochsPerExecutionVotingPeriod = 0 },
			want:   "EPOCHS_PER_EXECUTION_VOTING_PERIOD must be non-zero",
		},
		{
			input:  "SLOTS_PER_EPOCH: 64\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.SlotsPerEpoch = 64 },
			want:   "EPOCHS_PER_EXECUTION_VOTING_PERIOD * SLOTS_PER_EPOCH is 256",
		},
		{
			input: "SLOTS_PER_EPOCH: 256\nEPOCHS_PER_EXECUTION_VOTING_PERIOD: 2\n",
			mutate: func(cfg *params.BeaconChainConfig) {
				cfg.SlotsPerEpoch = 256
				cfg.EpochsPerExecutionVotingPeriod = 2
			},
			want: "SLOTS_PER_EPOCH (256) must not exceed this binary's fork-choice history capacity (128)",
		},
		{
			input:  "EPOCHS_PER_EXECUTION_VOTING_PERIOD: 18446744073709551615\n",
			mutate: func(cfg *params.BeaconChainConfig) { cfg.EpochsPerExecutionVotingPeriod = math.MaxUint64 },
			want:   "EPOCHS_PER_EXECUTION_VOTING_PERIOD * SLOTS_PER_EPOCH overflows uint64",
		},
	} {
		t.Run(strings.TrimSpace(tc.input), func(t *testing.T) {
			for _, source := range []string{"file", "active config"} {
				t.Run(source, func(t *testing.T) {
					params.SetupTestConfigCleanup(t)
					before := params.BeaconConfig().Copy()
					set := flag.NewFlagSet("test", flag.ContinueOnError)
					set.String(cmd.ChainConfigFileFlag.Name, "", "")
					if source == "file" {
						configPath := filepath.Join(t.TempDir(), "config.yaml")
						require.NoError(t, os.WriteFile(configPath, []byte("CONFIG_NAME: mainnet\n"+tc.input), 0600))
						require.NoError(t, set.Set(cmd.ChainConfigFileFlag.Name, configPath))
					} else {
						broken := before.Copy()
						tc.mutate(broken)
						params.OverrideBeaconConfig(broken)
					}
					cliCtx := cli.NewContext(&cli.App{}, set, nil)
					require.ErrorContains(t, tc.want, configureChainConfig(cliCtx))
					if source == "file" {
						require.DeepEqual(t, before, params.BeaconConfig())
					}
				})
			}
		})
	}
}

func TestConfigureChainConfig_ValidExecutionVotingLayout(t *testing.T) {
	for _, slotsPerEpoch := range []primitives.Slot{128, 64} {
		t.Run(fmt.Sprintf("slots_per_epoch_%d", slotsPerEpoch), func(t *testing.T) {
			params.SetupTestConfigCleanup(t)
			epochsPerVotingPeriod := primitives.Epoch(512 / slotsPerEpoch)
			input := fmt.Sprintf("CONFIG_NAME: mainnet\nSLOTS_PER_EPOCH: %d\nEPOCHS_PER_EXECUTION_VOTING_PERIOD: %d\n", slotsPerEpoch, epochsPerVotingPeriod)
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(configPath, []byte(input), 0600))
			set := flag.NewFlagSet("test", flag.ContinueOnError)
			set.String(cmd.ChainConfigFileFlag.Name, "", "")
			require.NoError(t, set.Set(cmd.ChainConfigFileFlag.Name, configPath))
			cliCtx := cli.NewContext(&cli.App{}, set, nil)

			require.NoError(t, configureChainConfig(cliCtx))
			require.Equal(t, slotsPerEpoch, params.BeaconConfig().SlotsPerEpoch)
			require.Equal(t, epochsPerVotingPeriod, params.BeaconConfig().EpochsPerExecutionVotingPeriod)
			require.NoError(t, params.BeaconConfig().ValidateStateLayout())
		})
	}
}

func TestConfigureHistoricalSlasher(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	hook := logTest.NewGlobal()

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.Bool(flags.HistoricalSlasherNode.Name, true, "")
	cliCtx := cli.NewContext(&app, set, nil)

	require.NoError(t, configureHistoricalSlasher(cliCtx))

	assert.Equal(t, params.BeaconConfig().SlotsPerEpoch*4, params.BeaconConfig().SlotsPerArchivedPoint)
	assert.LogsContain(t, hook,
		fmt.Sprintf(
			"Setting %d slots per archive point and %d max RPC page size for historical slasher usage",
			params.BeaconConfig().SlotsPerArchivedPoint,
			int(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().MaxAttestations))),
	)
}

func TestSlotsPerArchivedPointDefault(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	assert.Equal(t, 10112, flags.SlotsPerArchivedPoint.Value)
	assert.Equal(t, primitives.Slot(flags.SlotsPerArchivedPoint.Value), params.MainnetConfig().SlotsPerArchivedPoint)
	assert.Equal(t, params.MainnetConfig().SlotsPerArchivedPoint, params.BeaconConfig().SlotsPerArchivedPoint)
}

func TestConfigureSlotsPerArchivedPoint(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.Int(flags.SlotsPerArchivedPoint.Name, 0, "")
	require.NoError(t, set.Set(flags.SlotsPerArchivedPoint.Name, strconv.Itoa(100)))
	cliCtx := cli.NewContext(&app, set, nil)

	require.NoError(t, configureSlotsPerArchivedPoint(cliCtx))

	assert.Equal(t, primitives.Slot(100), params.BeaconConfig().SlotsPerArchivedPoint)
}

func TestConfigureProofOfWork(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.Uint64(flags.ChainID.Name, 0, "")
	set.Uint64(flags.NetworkID.Name, 0, "")
	set.String(flags.DepositContractFlag.Name, "", "")
	require.NoError(t, set.Set(flags.ChainID.Name, strconv.Itoa(100)))
	require.NoError(t, set.Set(flags.NetworkID.Name, strconv.Itoa(200)))
	require.NoError(t, set.Set(flags.DepositContractFlag.Name, "deposit-contract"))
	cliCtx := cli.NewContext(&app, set, nil)

	require.NoError(t, configureExecutionConfig(cliCtx))

	assert.Equal(t, uint64(100), params.BeaconConfig().DepositChainID)
	assert.Equal(t, uint64(200), params.BeaconConfig().DepositNetworkID)
	assert.Equal(t, "deposit-contract", params.BeaconConfig().DepositContractAddress)
}

func TestConfigureExecutionSetting(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	hook := logTest.NewGlobal()

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(flags.SuggestedFeeRecipient.Name, "", "")

	require.NoError(t, set.Set(flags.SuggestedFeeRecipient.Name, "ZB"))
	cliCtx := cli.NewContext(&app, set, nil)
	err := configureExecutionSetting(cliCtx)
	assert.LogsContain(t, hook, "ZB is not a valid fee recipient address")
	require.NoError(t, err)

	require.NoError(t, set.Set(flags.SuggestedFeeRecipient.Name, "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))
	cliCtx = cli.NewContext(&app, set, nil)
	err = configureExecutionSetting(cliCtx)
	require.NoError(t, err)
	recipient0, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	require.NoError(t, err)
	assert.Equal(t, recipient0, params.BeaconConfig().DefaultFeeRecipient)

	require.NoError(t, set.Set(flags.SuggestedFeeRecipient.Name, "Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000aAaAaAaaAaAaAaaAaAAAAAAAAaaaAaAaAaaAaaAa"))
	cliCtx = cli.NewContext(&app, set, nil)
	err = configureExecutionSetting(cliCtx)
	require.NoError(t, err)
	recipient1, err := common.NewAddressFromString("Q0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000aAaAaAaaAaAaAaaAaAAAAAAAAaaaAaAaAaaAaaAa")
	require.NoError(t, err)
	assert.Equal(t, recipient1, params.BeaconConfig().DefaultFeeRecipient)
}

func TestConfigureNetwork(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	bootstrapNodes := cli.StringSlice{}
	set.Var(&bootstrapNodes, cmd.BootstrapNode.Name, "")
	require.NoError(t, set.Set(cmd.BootstrapNode.Name, "node1"))
	require.NoError(t, set.Set(cmd.BootstrapNode.Name, "node2"))
	cliCtx := cli.NewContext(&app, set, nil)

	configureNetwork(cliCtx)

	assert.DeepEqual(t, []string{"node1", "node2"}, params.BeaconNetworkConfig().BootstrapNodes)
}

func TestConfigureNetwork_ConfigFile(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	context := cli.NewContext(&app, set, nil)

	require.NoError(t, os.WriteFile("flags_test.yaml", fmt.Appendf(nil, "%s:\n - %s\n - %s\n", cmd.BootstrapNode.Name,
		"node1",
		"node2"), 0666))

	require.NoError(t, set.Parse([]string{"test-command", "--" + cmd.ConfigFileFlag.Name, "flags_test.yaml"}))
	comFlags := cmd.WrapFlags([]cli.Flag{
		&cli.StringFlag{
			Name: cmd.ConfigFileFlag.Name,
		},
		&cli.StringSliceFlag{
			Name: cmd.BootstrapNode.Name,
		},
	})
	command := &cli.Command{
		Name:  "test-command",
		Flags: comFlags,
		Before: func(cliCtx *cli.Context) error {
			return cmd.LoadFlagsFromConfig(cliCtx, comFlags)
		},
		Action: func(cliCtx *cli.Context) error {
			require.Equal(t, true, cliCtx.IsSet(cmd.BootstrapNode.Name))

			require.Equal(t, strings.Join([]string{"node1", "node2"}, ","),
				strings.Join(cliCtx.StringSlice(cmd.BootstrapNode.Name), ","))
			return nil
		},
	}
	require.NoError(t, command.Run(context, context.Args().Slice()...))
	require.NoError(t, os.Remove("flags_test.yaml"))
}

func TestConfigureInterop(t *testing.T) {
	params.SetupTestConfigCleanup(t)

	tests := []struct {
		name       string
		flagSetter func() *cli.Context
		configName string
	}{
		{
			"nothing set",
			func() *cli.Context {
				app := cli.App{}
				set := flag.NewFlagSet("test", 0)
				return cli.NewContext(&app, set, nil)
			},
			"mainnet",
		},
		{
			"mock votes set",
			func() *cli.Context {
				app := cli.App{}
				set := flag.NewFlagSet("test", 0)
				set.Bool(flags.InteropMockExecutionDataVotesFlag.Name, false, "")
				assert.NoError(t, set.Set(flags.InteropMockExecutionDataVotesFlag.Name, "true"))
				return cli.NewContext(&app, set, nil)
			},
			"interop",
		},
		{
			"validators set",
			func() *cli.Context {
				app := cli.App{}
				set := flag.NewFlagSet("test", 0)
				set.Uint64(flags.InteropNumValidatorsFlag.Name, 0, "")
				assert.NoError(t, set.Set(flags.InteropNumValidatorsFlag.Name, "20"))
				return cli.NewContext(&app, set, nil)
			},
			"interop",
		},
		{
			"genesis time set",
			func() *cli.Context {
				app := cli.App{}
				set := flag.NewFlagSet("test", 0)
				set.Uint64(flags.InteropGenesisTimeFlag.Name, 0, "")
				assert.NoError(t, set.Set(flags.InteropGenesisTimeFlag.Name, "200"))
				return cli.NewContext(&app, set, nil)
			},
			"interop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, configureInteropConfig(tt.flagSetter()))
			assert.DeepEqual(t, tt.configName, params.BeaconConfig().ConfigName)
		})
	}
}

func TestConfigureSlotsPerArchivedPoint_RejectsNonPositive(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	for _, value := range []int{0, -1} {
		app := cli.App{}
		set := flag.NewFlagSet("test", 0)
		set.Int(flags.SlotsPerArchivedPoint.Name, 0, "")
		require.NoError(t, set.Set(flags.SlotsPerArchivedPoint.Name, strconv.Itoa(value)))
		cliCtx := cli.NewContext(&app, set, nil)

		require.ErrorContains(t, "must be a positive number of slots", configureSlotsPerArchivedPoint(cliCtx))
		assert.Equal(t, params.MainnetConfig().SlotsPerArchivedPoint, params.BeaconConfig().SlotsPerArchivedPoint, "the configured interval must be untouched")
	}
}
