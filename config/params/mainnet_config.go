package params

import (
	"math"
	"time"

	qrlparams "github.com/theQRL/go-qrl/params"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/encoding/bytesutil"
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
	AttestationSubnetCount:          4,
	AttestationPropagationSlotRange: 128,     // One mainnet epoch, matching the attestation inclusion window.
	MaxRequestBlocks:                1 << 10, // 1024
	TtfbTimeout:                     35 * time.Second,
	RespTimeout:                     50 * time.Second,
	MaximumGossipClockDisparity:     500 * time.Millisecond,
	MessageDomainInvalidSnappy:      [4]byte{00, 00, 00, 00},
	MessageDomainValidSnappy:        [4]byte{01, 00, 00, 00},
	ConsensusKey:                    "consensus",
	AttSubnetKey:                    "attnets",
	SyncCommsSubnetKey:              "syncnets",
	MinimumPeersInSubnetSearch:      4, // Unused, see NetworkConfig.
	BootstrapNodes: []string{
		// TODO(now.youtrack.cloud/issue/TQ-13)
		"qnr:-Me4QKAyB33PkhFM0zY3r5i9GC7aSP_Zm39U8Y2rOIzr6443SngB1yhgGh7eFwNK0WNvDWjUbNksuJqME3ZEnwS9qJSGAZ1Gb_3yh2F0dG5ldHOIAAAAAAAAAACJY29uc2Vuc3VzkJCnUX4gAACJ__________-CaWSCdjSCaXCEI7LKF4lzZWNwMjU2azGhA7Z8d7NnIyueM0N06PyRt8jcnKvuRkMTmtzoDOw081ouiHN5bmNuZXRzAIN0Y3CCMsiDdWRwgi7g",
		"qnr:-Me4QE7keK-7ViWaDwpa0GtR12qbe9ZiVvKX95z7A2hkJHA6L-2A0n8G6dn8M1kubmxAeVw7Nwaa6IBPCX9765zNi56GAZ1Gb-8uh2F0dG5ldHOIAAAAAAAAAACJY29uc2Vuc3VzkJCnUX4gAACJ__________-CaWSCdjSCaXCEDSlCUolzZWNwMjU2azGhA9t_KLMx2GyZcw7tncC7iLdJpKBS_tCp9CavD6LXd5ZaiHN5bmNuZXRzAIN0Y3CCMsiDdWRwgi7g",
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
	TargetCommitteeSize:            32,
	MaxValidatorsPerCommittee:      32,
	MaxCommitteesPerSlot:           1,
	MinPerEpochChurnLimit:          2,
	ChurnLimitQuotient:             1 << 16,
	ShuffleRoundCount:              90,
	MinGenesisActiveValidatorCount: 128,
	MinGenesisTime:                 1606824000, // Dec 1, 2020, 12pm UTC.
	TargetAggregatorsPerCommittee:  8,
	HysteresisQuotient:             4,
	HysteresisDownwardMultiplier:   1,
	HysteresisUpwardMultiplier:     5,

	// Shor value constants.
	// 2000 QRL, 1/20 of a full stake. Ethereum uses 1/32 (1 of 32 ETH); the
	// contract enforces this floor, consensus does not. A never-activated record
	// costs every node ~2.7 KB of state and ~7.3 KB of deposit cache forever,
	// so a 1-QRL floor made registry bloat ~10^4x cheaper than on Ethereum.
	MinDepositAmount:          2000 * 1e9,
	MaxEffectiveBalance:       40000 * 1e9,
	EjectionBalance:           20000 * 1e9,
	EffectiveBalanceIncrement: 1 * 1e9,

	// Initial value constants.
	ZeroHash: [32]byte{},

	// Time parameter constants.
	MinAttestationInclusionDelay:     1,
	SecondsPerSlot:                   60,
	SlotsPerEpoch:                    128,
	SqrRootSlotsPerEpoch:             11,
	MinSeedLookahead:                 1,
	MaxSeedLookahead:                 4,
	EpochsPerExecutionVotingPeriod:   4,    // 4 * 128 slots per epoch = 512 slots for voting
	SlotsPerHistoricalRoot:           1024, // TODO (cyyber) : Re-evaluate the value
	MinValidatorWithdrawabilityDelay: 16,   // TODO (cyyber) : Re-evaluate the value
	ShardCommitteePeriod:             16,   // TODO (cyyber) : Re-evaluate the value
	MinEpochsToInactivityPenalty:     4,
	ExecutionFollowDistance:          512, // TODO(now.youtrack.cloud/issue/TQ-5)

	// Fork choice algorithm constants.
	ProposerScoreBoost:              40,
	ReorgHeadWeightThreshold:        20,
	ReorgParentWeightThreshold:      160,
	ReorgMaxEpochsSinceFinalization: 2,
	IntervalsPerSlot:                3,

	// QRL execution layer parameters.
	DepositChainID:         1, // Chain ID of execution network.
	DepositNetworkID:       1, // Network ID of execution network.
	DepositContractAddress: "Q42424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242",

	// Validator params.
	RandomSubnetsPerValidator:         1 << 0,
	EpochsPerRandomSubnetSubscription: 1 << 8,

	// While eth1 mainnet block times are closer to 13s, we must conform with other clients in
	// order to vote on the correct execution blocks.
	//
	// Additional context: https://github.com/ethereum/consensus-specs/issues/2132
	// Bug prompting this change: https://github.com/prysmaticlabs/prysm/issues/7856
	// Future optimization: https://github.com/prysmaticlabs/prysm/issues/7739
	SecondsPerExecutionBlock: 60,

	// State list length constants.
	EpochsPerHistoricalVector: 65536,
	EpochsPerSlashingsVector:  512, // 512 * 128 * 60 seconds = 45 days, 12 hours, 16 minutes.
	HistoricalRootsLimit:      16777216,
	ValidatorRegistryLimit:    1099511627776,

	// Reward and penalty quotients constants.
	BaseRewardFactor:            2048,
	WhistleBlowerRewardQuotient: 512,
	ProposerRewardQuotient:      8,

	// Max operations per block constants.
	MaxProposerSlashings:             16,
	MaxAttesterSlashings:             2,
	MaxAttestations:                  4,
	MaxDeposits:                      16,
	MaxVoluntaryExits:                16,
	MaxWithdrawalsPerPayload:         16,
	MaxValidatorsPerWithdrawalsSweep: 16384,

	// ML-DSA-87 domain values.
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

	// Qrysm constants.
	ShorPerQuanta:             1000000000,
	DefaultBufferSize:         10000,
	RPCSyncCheck:              1,
	EmptyMLDSA87Signature:     [fieldparams.MLDSA87SignatureLength]byte{},
	DefaultPageSize:           250,
	MaxPeersToSync:            15,
	SlotsPerArchivedPoint:     10112, // 79 epochs, 7 days at 60s slots.
	GenesisCountdownInterval:  time.Minute,
	ConfigName:                MainnetName,
	PresetBase:                "mainnet",
	BeaconStateZondFieldCount: 28,

	// Slasher related values.
	// Covers the 118-epoch dynamic WSP maximum at the 4,096-validator cap,
	// including margin for inclusive proposer-history pruning.
	//
	// The cap is enforced by the registry transition and genesis construction:
	// MaxCommitteesPerSlot * MaxValidatorsPerCommittee * SlotsPerEpoch
	// (1 * 32 * 128), the widest validator set whose committees the SSZ
	// attestation types can encode. Changing any of those, or SafetyDecay,
	// MinPerEpochChurnLimit or MinValidatorWithdrawabilityDelay, changes the
	// dynamic maximum and this value must be re-derived. At 8,192 validators
	// the dynamic WSP would be 220 epochs and 128 would be too small.
	// TestWeakSubjectivity_ProposerHistoryRetentionAtValidatorCap pins it.
	WeakSubjectivityPeriod:          128,
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
	TargetAggregatorsPerSyncSubcommittee: 8, // Note: (cyyber) Set it 16, when we will have the real aggregation
	SyncCommitteeSubnetCount:             1, // TODO: (cyyber) finalize SyncCommitteeSubnetCount, original value was 4

	// Misc values.
	SyncCommitteeSize:            128, // TODO: (cyyber) finalize SyncCommitteeSize, original value was 512
	InactivityScoreBias:          4,
	InactivityScoreRecoveryRate:  16,
	EpochsPerSyncCommitteePeriod: 8, // TODO: (cyyber) finalize EpochsPerSyncCommitteePeriod, original value was 512

	// Updated penalty values.
	MinSlashingPenaltyQuotient:     32,
	ProportionalSlashingMultiplier: 3,
	InactivityPenaltyQuotient:      1 << 16,

	// Light client
	MinSyncCommitteeParticipants: 1,

	QRLBurnAddress: "Q00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
	// Tied to go-qrl's execution-layer hard cap (20M), which is also go-qrl's own
	// miner GasCeil default. The execution layer rejects any block above
	// MaxGasLimit and the beacon node clamps the proposer's target to it, so a
	// larger default could never take effect and would only advertise an invalid
	// gas limit in validator registrations.
	DefaultBuilderGasLimit: qrlparams.MaxGasLimit,

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
