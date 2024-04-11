package params

import (
	"math"
	"time"

	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
)

// MainnetConfig returns the configuration to be used in the main network.
func MainnetConfig() *BeaconChainConfig {
	if mainnetBeaconConfig.ForkVersionSchedule == nil {
		mainnetBeaconConfig.InitializeForkSchedule()
	}
	return mainnetBeaconConfig
}

// Genesis Fork Epoch for the mainnet config.
const genesisForkEpoch = 0

var mainnetNetworkConfig = &NetworkConfig{
	GossipMaxSize:                   10 * 1 << 20, // 10 MiB
	MaxChunkSize:                    10 * 1 << 20, // 10 MiB
	AttestationSubnetCount:          64,
	AttestationPropagationSlotRange: 32,
	MaxRequestBlocks:                1 << 10, // 1024
	TtfbTimeout:                     35 * time.Second,
	RespTimeout:                     50 * time.Second,
	MaximumGossipClockDisparity:     500 * time.Millisecond,
	MessageDomainInvalidSnappy:      [4]byte{00, 00, 00, 00},
	MessageDomainValidSnappy:        [4]byte{01, 00, 00, 00},
	ETH2Key:                         "eth2",
	AttSubnetKey:                    "attnets",
	SyncCommsSubnetKey:              "syncnets",
	MinimumPeersInSubnetSearch:      20,
	ContractDeploymentBlock:         11184524, // Note: contract was deployed in block 11052984 but no transactions were sent until 11184524.
	BootstrapNodes:                  []string{
		// TODO(theQRL/qrysm/issues/61)
		// "enr:-Ku4QImhMc1z8yCiNJ1TyUxdcfNucje3BGwEHzodEZUan8PherEo4sF7pPHPSIB1NNuSg5fZy7qFsjmUKs2ea1Whi0EBh2F0dG5ldHOIAAAAAAAAAACEZXRoMpD1pf1CAAAAAP__________gmlkgnY0gmlwhBLf22SJc2VjcDI1NmsxoQOVphkDqal4QzPMksc5wnpuC3gvSC8AfbFOnZY_On34wIN1ZHCCIyg",
		// "enr:-Ku4QP2xDnEtUXIjzJ_DhlCRN9SN99RYQPJL92TMlSv7U5C1YnYLjwOQHgZIUXw6c-BvRg2Yc2QsZxxoS_pPRVe0yK8Bh2F0dG5ldHOIAAAAAAAAAACEZXRoMpD1pf1CAAAAAP__________gmlkgnY0gmlwhBLf22SJc2VjcDI1NmsxoQMeFF5GrS7UZpAH2Ly84aLK-TyvH-dRo0JM1i8yygH50YN1ZHCCJxA",
	},
}

var mainnetBeaconConfig = &BeaconChainConfig{
	// Constants (Non-configurable)
	FarFutureEpoch:           math.MaxUint64,
	FarFutureSlot:            math.MaxUint64,
	BaseRewardsPerEpoch:      4,
	DepositContractTreeDepth: 32,
	GenesisDelay:             604800, // 1 week.

	// Misc constant.
	TargetCommitteeSize:            128,
	MaxValidatorsPerCommittee:      2048,
	MaxCommitteesPerSlot:           64,
	MinPerEpochChurnLimit:          10, // TODO (cyyber): Re-evaluate the value
	ChurnLimitQuotient:             1 << 16,
	ShuffleRoundCount:              90,
	MinGenesisActiveValidatorCount: 16384,
	MinGenesisTime:                 1606824000, // Dec 1, 2020, 12pm UTC.
	TargetAggregatorsPerCommittee:  16,
	HysteresisQuotient:             4,
	HysteresisDownwardMultiplier:   1,
	HysteresisUpwardMultiplier:     5,

	// Gwei value constants.
	MinDepositAmount:          1 * 1e9,
	MaxEffectiveBalance:       40000 * 1e9,
	EjectionBalance:           20000 * 1e9,
	EffectiveBalanceIncrement: 1 * 1e9,

	// Initial value constants.
	DilithiumWithdrawalPrefixByte:   byte(0), // TODO (cyyber): Change it to 1 & check if we should add XMSSWithdrawalPrefixByte
	ZondAddressWithdrawalPrefixByte: byte(1), // TODO (cyyber): Change it to 0
	ZeroHash:                        [32]byte{},

	// Time parameter constants.
	MinAttestationInclusionDelay:     1,
	SecondsPerSlot:                   60,
	SlotsPerEpoch:                    128,
	SqrRootSlotsPerEpoch:             11,
	MinSeedLookahead:                 1,
	MaxSeedLookahead:                 4,
	EpochsPerEth1VotingPeriod:        2,    // TODO (cyyber) : Re-evaluate the value
	SlotsPerHistoricalRoot:           1024, // TODO (cyyber) : Re-evaluate the value
	MinValidatorWithdrawabilityDelay: 16,   // TODO (cyyber) : Re-evaluate the value
	ShardCommitteePeriod:             16,   // TODO (cyyber) : Re-evaluate the value
	MinEpochsToInactivityPenalty:     4,
	Eth1FollowDistance:               0, // TODO(theQRL/qrysm/issues/66)

	// Fork choice algorithm constants.
	ProposerScoreBoost:              40,
	ReorgWeightThreshold:            20,
	ReorgParentWeightThreshold:      160,
	ReorgMaxEpochsSinceFinalization: 2,
	IntervalsPerSlot:                3,

	// Zond execution layer parameters.
	DepositChainID:         1, // Chain ID of eth1 mainnet.
	DepositNetworkID:       1, // Network ID of eth1 mainnet.
	DepositContractAddress: "0x00000000219ab540356cBB839Cbe05303d7705Fa",

	// Validator params.
	RandomSubnetsPerValidator:         1 << 0,
	EpochsPerRandomSubnetSubscription: 1 << 8,

	// While eth1 mainnet block times are closer to 13s, we must conform with other clients in
	// order to vote on the correct eth1 blocks.
	//
	// Additional context: https://github.com/ethereum/consensus-specs/issues/2132
	// Bug prompting this change: https://github.com/theQRL/qrysm/issues/7856
	// Future optimization: https://github.com/theQRL/qrysm/issues/7739
	SecondsPerETH1Block: 60,

	// State list length constants.
	EpochsPerHistoricalVector: 65536,
	EpochsPerSlashingsVector:  1024,
	HistoricalRootsLimit:      16777216,
	ValidatorRegistryLimit:    1099511627776,

	// Reward and penalty quotients constants.
	BaseRewardFactor:            64,
	WhistleBlowerRewardQuotient: 512,
	ProposerRewardQuotient:      8,

	// Max operations per block constants.
	MaxProposerSlashings:             16,
	MaxAttesterSlashings:             2,
	MaxAttestations:                  128,
	MaxDeposits:                      16,
	MaxVoluntaryExits:                16,
	MaxWithdrawalsPerPayload:         16,
	MaxDilithiumToExecutionChanges:   16,
	MaxValidatorsPerWithdrawalsSweep: 16384,

	// Dilithium domain values.
	DomainBeaconProposer:              bytesutil.Uint32ToBytes4(0x00000000),
	DomainBeaconAttester:              bytesutil.Uint32ToBytes4(0x01000000),
	DomainRandao:                      bytesutil.Uint32ToBytes4(0x02000000),
	DomainDeposit:                     bytesutil.Uint32ToBytes4(0x03000000),
	DomainVoluntaryExit:               bytesutil.Uint32ToBytes4(0x04000000),
	DomainSelectionProof:              bytesutil.Uint32ToBytes4(0x05000000),
	DomainAggregateAndProof:           bytesutil.Uint32ToBytes4(0x06000000),
	DomainSyncCommittee:               bytesutil.Uint32ToBytes4(0x07000000),
	DomainSyncCommitteeSelectionProof: bytesutil.Uint32ToBytes4(0x08000000),
	DomainContributionAndProof:        bytesutil.Uint32ToBytes4(0x09000000),
	DomainApplicationMask:             bytesutil.Uint32ToBytes4(0x00000001),
	DomainApplicationBuilder:          bytesutil.Uint32ToBytes4(0x00000001),
	DomainDilithiumToExecutionChange:  bytesutil.Uint32ToBytes4(0x0A000000),

	// Qrysm constants.
	GweiPerEth:                   1000000000,
	DefaultBufferSize:            10000,
	WithdrawalPrivkeyFileName:    "/shardwithdrawalkey",
	ValidatorPrivkeyFileName:     "/validatorprivatekey",
	RPCSyncCheck:                 1,
	EmptyDilithiumSignature:      [fieldparams.DilithiumSignatureLength]byte{},
	DefaultPageSize:              250,
	MaxPeersToSync:               15,
	SlotsPerArchivedPoint:        2048,
	GenesisCountdownInterval:     time.Minute,
	ConfigName:                   MainnetName,
	PresetBase:                   "mainnet",
	BeaconStateCapellaFieldCount: 28,

	// Slasher related values.
	WeakSubjectivityPeriod:          54000,
	PruneSlasherStoragePeriod:       10,
	SlashingProtectionPruningEpochs: 512,

	// Weak subjectivity values.
	SafetyDecay: 10,

	// Fork related values.
	GenesisEpoch:       genesisForkEpoch,
	GenesisForkVersion: []byte{0, 0, 0, 0},

	// Participation flag indices.
	TimelySourceFlagIndex: 0,
	TimelyTargetFlagIndex: 1,
	TimelyHeadFlagIndex:   2,

	// Incentivization weight values.
	TimelySourceWeight: 14,
	TimelyTargetWeight: 26,
	TimelyHeadWeight:   14,
	SyncRewardWeight:   2,
	ProposerWeight:     8,
	WeightDenominator:  64,

	// Validator related values.
	TargetAggregatorsPerSyncSubcommittee: 16,
	SyncCommitteeSubnetCount:             1, // TODO: (cyyber) finalize SyncCommitteeSubnetCount, original value was 4

	// Misc values.
	SyncCommitteeSize:            16, // TODO: (cyyber) finalize SyncCommitteeSize, original value was 512
	InactivityScoreBias:          4,
	InactivityScoreRecoveryRate:  16,
	EpochsPerSyncCommitteePeriod: 8, // TODO: (cyyber) finalize EpochsPerSyncCommitteePeriod, original value was 512

	// Updated penalty values.
	MinSlashingPenaltyQuotient:     32,
	ProportionalSlashingMultiplier: 3,
	InactivityPenaltyQuotient:      1 << 24,

	// Light client
	MinSyncCommitteeParticipants: 1,

	// Bellatrix
	EthBurnAddressHex:      "0x0000000000000000000000000000000000000000",
	DefaultBuilderGasLimit: uint64(30000000),

	// Mevboost circuit breaker
	MaxBuilderConsecutiveMissedSlots: 3,
	MaxBuilderEpochMissedSlots:       5,
	// Execution engine timeout value
	ExecutionEngineTimeoutValue: 8, // 8 seconds default based on: https://github.com/ethereum/execution-apis/blob/main/src/engine/specification.md#core

	MaxPerEpochActivationChurnLimit: 8,
}

// MainnetTestConfig provides a version of the mainnet config that has a different name
// and a different fork choice schedule. This can be used in cases where we want to use config values
// that are consistent with mainnet, but won't conflict or cause the hard-coded genesis to be loaded.
func MainnetTestConfig() *BeaconChainConfig {
	mn := MainnetConfig().Copy()
	mn.ConfigName = MainnetTestName
	FillTestVersions(mn, 128)
	return mn
}

// FillTestVersions replaces the fork schedule in the given BeaconChainConfig with test values, using the given
// byte argument as the high byte (common across forks).
func FillTestVersions(c *BeaconChainConfig, b byte) {
	c.GenesisForkVersion = make([]byte, fieldparams.VersionLength)
	c.GenesisForkVersion[fieldparams.VersionLength-1] = b
	c.GenesisForkVersion[0] = 0
}
