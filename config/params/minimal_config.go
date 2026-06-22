package params

import (
	"math"

	"github.com/theQRL/qrysm/encoding/bytesutil"
)

// MinimalSpecConfig retrieves the minimal config used in spec tests.
func MinimalSpecConfig() *BeaconChainConfig {
	minimalConfig := mainnetBeaconConfig.Copy()
	// Misc
	minimalConfig.MaxCommitteesPerSlot = 4
	minimalConfig.TargetCommitteeSize = 4
	minimalConfig.MaxValidatorsPerCommittee = 2048
	minimalConfig.MinPerEpochChurnLimit = 2           // Changed in EIP7514
	minimalConfig.MaxPerEpochActivationChurnLimit = 4 // New in EIP7514
	minimalConfig.ChurnLimitQuotient = 32
	minimalConfig.ShuffleRoundCount = 10
	minimalConfig.MinGenesisActiveValidatorCount = 64
	minimalConfig.MinGenesisTime = 1578009600
	minimalConfig.GenesisDelay = 300 // 5 minutes
	minimalConfig.TargetAggregatorsPerCommittee = 16

	// Shor values
	minimalConfig.MinDepositAmount = 1e9
	minimalConfig.MaxEffectiveBalance = 40000e9
	minimalConfig.EjectionBalance = 20000e9
	minimalConfig.EffectiveBalanceIncrement = 1e9

	// Time parameters
	minimalConfig.SecondsPerSlot = 6
	minimalConfig.MinAttestationInclusionDelay = 1
	minimalConfig.SlotsPerEpoch = 8
	minimalConfig.SqrRootSlotsPerEpoch = 2
	minimalConfig.MinSeedLookahead = 1
	minimalConfig.MaxSeedLookahead = 4
	minimalConfig.EpochsPerExecutionVotingPeriod = 4
	minimalConfig.SlotsPerHistoricalRoot = 64
	minimalConfig.MinValidatorWithdrawabilityDelay = 256
	minimalConfig.ShardCommitteePeriod = 64
	minimalConfig.MinEpochsToInactivityPenalty = 4
	minimalConfig.ExecutionFollowDistance = 16
	minimalConfig.SecondsPerExecutionBlock = 60

	// State vector lengths
	minimalConfig.EpochsPerHistoricalVector = 64
	minimalConfig.EpochsPerSlashingsVector = 64
	minimalConfig.HistoricalRootsLimit = 16777216
	minimalConfig.ValidatorRegistryLimit = 1099511627776

	// Reward and penalty quotients
	minimalConfig.BaseRewardFactor = 64
	minimalConfig.WhistleBlowerRewardQuotient = 512
	minimalConfig.ProposerRewardQuotient = 8

	// Max operations per block
	minimalConfig.MaxProposerSlashings = 16
	minimalConfig.MaxAttesterSlashings = 2
	minimalConfig.MaxAttestations = 128
	minimalConfig.MaxDeposits = 16
	minimalConfig.MaxVoluntaryExits = 16
	minimalConfig.MaxWithdrawalsPerPayload = 4
	minimalConfig.MaxValidatorsPerWithdrawalsSweep = 16

	// Signature domains
	minimalConfig.DomainBeaconProposer = bytesutil.ToBytes4(bytesutil.Bytes4(0))
	minimalConfig.DomainBeaconAttester = bytesutil.ToBytes4(bytesutil.Bytes4(1))
	minimalConfig.DomainRandao = bytesutil.ToBytes4(bytesutil.Bytes4(2))
	minimalConfig.DomainDeposit = bytesutil.ToBytes4(bytesutil.Bytes4(3))
	minimalConfig.DomainVoluntaryExit = bytesutil.ToBytes4(bytesutil.Bytes4(4))
	minimalConfig.GenesisForkVersion = []byte{0, 0, 0, 1}

	minimalConfig.DepositContractTreeDepth = 32
	minimalConfig.FarFutureEpoch = math.MaxUint64
	minimalConfig.FarFutureSlot = math.MaxUint64

	minimalConfig.SyncCommitteeSize = 16
	minimalConfig.InactivityScoreBias = 4
	minimalConfig.EpochsPerSyncCommitteePeriod = 8

	// QRL execution layer parameters.
	minimalConfig.DepositChainID = 5
	minimalConfig.DepositNetworkID = 5
	minimalConfig.DepositContractAddress = "Q12345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678"

	minimalConfig.ConfigName = MinimalName
	minimalConfig.PresetBase = "minimal"

	minimalConfig.InitializeForkSchedule()
	return minimalConfig
}
