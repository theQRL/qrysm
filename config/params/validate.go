package params

import (
	"fmt"
	"math"
	"math/bits"

	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
)

// Validate checks the fork-version size, participation flag indices, arithmetic
// invariants, committee bounds and preset block-operation limits that the consensus
// code assumes a configuration satisfies. Slot and epoch processing
// divide by, reduce modulo and multiply these values without checking them (process_slashings,
// slash_validator, process_rewards_and_penalties, process_registry_updates,
// process_effective_balance_updates, sync committee rewards), so a value that
// breaks an invariant either panics the node during slot or epoch processing or
// silently miscomputes balances. Built-in presets are covered by tests; a
// user-supplied chain config file is checked when it is loaded.
func (b *BeaconChainConfig) Validate() error {
	// Fork versions must fit the SSZ bytes4 fields and fork schedule keys.
	if len(b.GenesisForkVersion) != fieldparams.VersionLength {
		return fmt.Errorf("GENESIS_FORK_VERSION must be exactly %d bytes, got %d", fieldparams.VersionLength, len(b.GenesisForkVersion))
	}
	if err := b.validateDepositTreeDepth(); err != nil {
		return err
	}
	// Both scalar and bulk shuffling narrow the round count to uint8.
	// Zero is a supported no-op, but larger counts must not silently truncate.
	if b.ShuffleRoundCount > math.MaxUint8 {
		return fmt.Errorf("SHUFFLE_ROUND_COUNT (%d) must not exceed %d (uint8 shuffle round limit)",
			b.ShuffleRoundCount, math.MaxUint8)
	}

	// Participation flags share a byte. Out-of-range indices fail epoch
	// processing, while duplicate indices conflate different kinds of votes.
	participationFlags := []struct {
		name  string
		index uint8
	}{
		{"TIMELY_SOURCE_FLAG_INDEX", b.TimelySourceFlagIndex},
		{"TIMELY_TARGET_FLAG_INDEX", b.TimelyTargetFlagIndex},
		{"TIMELY_HEAD_FLAG_INDEX", b.TimelyHeadFlagIndex},
	}
	for i, flag := range participationFlags {
		if flag.index > 7 {
			return fmt.Errorf("%s (%d) must be between 0 and 7", flag.name, flag.index)
		}
		for _, previous := range participationFlags[:i] {
			if flag.index == previous.index {
				return fmt.Errorf("%s and %s must be distinct (both are %d)", previous.name, flag.name, flag.index)
			}
		}
	}

	nonZero := []struct {
		name  string
		value uint64
	}{
		// Balances and slashing penalties.
		{"EFFECTIVE_BALANCE_INCREMENT", b.EffectiveBalanceIncrement},
		{"MIN_SLASHING_PENALTY_QUOTIENT", b.MinSlashingPenaltyQuotient},
		{"PROPORTIONAL_SLASHING_MULTIPLIER", b.ProportionalSlashingMultiplier},
		{"WHISTLEBLOWER_REWARD_QUOTIENT", b.WhistleBlowerRewardQuotient},
		{"PROPOSER_REWARD_QUOTIENT", b.ProposerRewardQuotient},
		{"EPOCHS_PER_SLASHINGS_VECTOR", uint64(b.EpochsPerSlashingsVector)},
		{"HYSTERESIS_QUOTIENT", b.HysteresisQuotient},
		// Rewards, penalties and registry updates.
		{"INACTIVITY_PENALTY_QUOTIENT", b.InactivityPenaltyQuotient},
		{"INACTIVITY_SCORE_BIAS", b.InactivityScoreBias},
		{"CHURN_LIMIT_QUOTIENT", b.ChurnLimitQuotient},
		{"MAX_PER_EPOCH_ACTIVATION_CHURN_LIMIT", b.MaxPerEpochActivationChurnLimit},
		{"WEIGHT_DENOMINATOR", b.WeightDenominator},
		{"PROPOSER_WEIGHT", b.ProposerWeight},
		{"SECONDS_PER_SLOT", b.SecondsPerSlot},
		{"INTERVALS_PER_SLOT", b.IntervalsPerSlot},
		{"SECONDS_PER_EXECUTION_BLOCK", b.SecondsPerExecutionBlock},
		{"SLOTS_PER_EPOCH", uint64(b.SlotsPerEpoch)},
		{"EPOCHS_PER_EXECUTION_VOTING_PERIOD", uint64(b.EpochsPerExecutionVotingPeriod)},
		{"TARGET_COMMITTEE_SIZE", b.TargetCommitteeSize},
		{"TARGET_AGGREGATORS_PER_COMMITTEE", b.TargetAggregatorsPerCommittee},
		{"MAX_VALIDATORS_PER_COMMITTEE", b.MaxValidatorsPerCommittee},
		{"MAX_COMMITTEES_PER_SLOT", b.MaxCommitteesPerSlot},
		{"EPOCHS_PER_HISTORICAL_VECTOR", uint64(b.EpochsPerHistoricalVector)},
		{"SLOTS_PER_HISTORICAL_ROOT", uint64(b.SlotsPerHistoricalRoot)},
		{"SYNC_COMMITTEE_SIZE", b.SyncCommitteeSize},
		{"SYNC_COMMITTEE_SUBNET_COUNT", b.SyncCommitteeSubnetCount},
		{"TARGET_AGGREGATORS_PER_SYNC_SUBCOMMITTEE", b.TargetAggregatorsPerSyncSubcommittee},
		{"EPOCHS_PER_SYNC_COMMITTEE_PERIOD", uint64(b.EpochsPerSyncCommitteePeriod)},
		// Persistent subnet assignment samples a duration using Intn.
		{"EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION", b.EpochsPerRandomSubnetSubscription},
	}
	for _, c := range nonZero {
		if c.value == 0 {
			return fmt.Errorf("%s must be non-zero", c.name)
		}
	}

	// Attestations expire after one epoch. A longer minimum delay leaves no
	// slot at which an attestation can be included.
	if b.MinAttestationInclusionDelay > b.SlotsPerEpoch {
		return fmt.Errorf("MIN_ATTESTATION_INCLUSION_DELAY (%d) must not exceed SLOTS_PER_EPOCH (%d)",
			b.MinAttestationInclusionDelay, b.SlotsPerEpoch)
	}

	// Intn requires a positive int; larger uint64 values can become zero or
	// negative when converted, or silently truncate on a 32-bit platform.
	if b.EpochsPerRandomSubnetSubscription > uint64(math.MaxInt) {
		return fmt.Errorf("EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION (%d) must not exceed %d (int limit for subnet subscription sampling)",
			b.EpochsPerRandomSubnetSubscription, math.MaxInt)
	}

	// AttestationsDelta divides by this product. Nonzero factors can still
	// overflow uint64, producing a zero or otherwise incorrect denominator.
	if hi, _ := bits.Mul64(b.InactivityScoreBias, b.InactivityPenaltyQuotient); hi != 0 {
		return fmt.Errorf("INACTIVITY_SCORE_BIAS (%d) * INACTIVITY_PENALTY_QUOTIENT (%d) overflows uint64",
			b.InactivityScoreBias, b.InactivityPenaltyQuotient)
	}

	// Effective balances are rounded down to a multiple of the increment and
	// capped at MAX_EFFECTIVE_BALANCE; the slashing and reward code then works
	// in whole increments (effective_balance / EFFECTIVE_BALANCE_INCREMENT).
	if b.MaxEffectiveBalance < b.EffectiveBalanceIncrement || b.MaxEffectiveBalance%b.EffectiveBalanceIncrement != 0 {
		return fmt.Errorf("MAX_EFFECTIVE_BALANCE (%d) must be a positive multiple of EFFECTIVE_BALANCE_INCREMENT (%d)",
			b.MaxEffectiveBalance, b.EffectiveBalanceIncrement)
	}
	// process_effective_balance_updates: hysteresis thresholds are
	// hysteresis_increment * multiplier in uint64.
	hysteresisIncrement := b.EffectiveBalanceIncrement / b.HysteresisQuotient
	if hi, _ := bits.Mul64(hysteresisIncrement, b.HysteresisDownwardMultiplier); hi != 0 {
		return fmt.Errorf("EFFECTIVE_BALANCE_INCREMENT / HYSTERESIS_QUOTIENT * HYSTERESIS_DOWNWARD_MULTIPLIER overflows uint64")
	}
	if hi, _ := bits.Mul64(hysteresisIncrement, b.HysteresisUpwardMultiplier); hi != 0 {
		return fmt.Errorf("EFFECTIVE_BALANCE_INCREMENT / HYSTERESIS_QUOTIENT * HYSTERESIS_UPWARD_MULTIPLIER overflows uint64")
	}

	// get_base_reward_per_increment computes
	// EFFECTIVE_BALANCE_INCREMENT * BASE_REWARD_FACTOR in uint64.
	if hi, _ := bits.Mul64(b.EffectiveBalanceIncrement, b.BaseRewardFactor); hi != 0 {
		return fmt.Errorf("EFFECTIVE_BALANCE_INCREMENT (%d) * BASE_REWARD_FACTOR (%d) overflows uint64",
			b.EffectiveBalanceIncrement, b.BaseRewardFactor)
	}

	// The participation flag weights plus the sync and proposer weights must
	// sum to WEIGHT_DENOMINATOR (spec invariant: rewards are distributed as
	// weight / WEIGHT_DENOMINATOR shares of the base reward). The attestation
	// proposer reward divides by (WEIGHT_DENOMINATOR - PROPOSER_WEIGHT) *
	// WEIGHT_DENOMINATOR / PROPOSER_WEIGHT. Requiring 0 < proposer < denominator
	// before subtracting, and excluding multiplication overflow below, ensures
	// that the final divisor is positive.
	if b.ProposerWeight >= b.WeightDenominator {
		return fmt.Errorf("PROPOSER_WEIGHT (%d) must be less than WEIGHT_DENOMINATOR (%d)",
			b.ProposerWeight, b.WeightDenominator)
	}
	var weightSum uint64
	for _, weight := range []uint64{b.TimelySourceWeight, b.TimelyTargetWeight, b.TimelyHeadWeight, b.SyncRewardWeight, b.ProposerWeight} {
		var carry uint64
		weightSum, carry = bits.Add64(weightSum, weight, 0)
		if carry != 0 {
			return fmt.Errorf("TIMELY_SOURCE_WEIGHT + TIMELY_TARGET_WEIGHT + TIMELY_HEAD_WEIGHT + SYNC_REWARD_WEIGHT + PROPOSER_WEIGHT overflows uint64")
		}
	}
	if weightSum != b.WeightDenominator {
		return fmt.Errorf("TIMELY_SOURCE_WEIGHT + TIMELY_TARGET_WEIGHT + TIMELY_HEAD_WEIGHT + SYNC_REWARD_WEIGHT + PROPOSER_WEIGHT (%d) must equal WEIGHT_DENOMINATOR (%d)",
			weightSum, b.WeightDenominator)
	}
	if hi, _ := bits.Mul64(b.WeightDenominator-b.ProposerWeight, b.WeightDenominator); hi != 0 {
		return fmt.Errorf("(WEIGHT_DENOMINATOR - PROPOSER_WEIGHT) * WEIGHT_DENOMINATOR overflows uint64")
	}

	// Sync committee subnets partition the committee.
	if b.SyncCommitteeSize%b.SyncCommitteeSubnetCount != 0 {
		return fmt.Errorf("SYNC_COMMITTEE_SIZE (%d) must be a multiple of SYNC_COMMITTEE_SUBNET_COUNT (%d)",
			b.SyncCommitteeSize, b.SyncCommitteeSubnetCount)
	}

	maxActiveValidators, err := b.MaxActiveValidators()
	if err != nil {
		return err
	}
	if b.MinGenesisActiveValidatorCount > maxActiveValidators {
		return fmt.Errorf("MIN_GENESIS_ACTIVE_VALIDATOR_COUNT (%d) must not exceed the active validator capacity (%d)",
			b.MinGenesisActiveValidatorCount, maxActiveValidators)
	}
	// Attestation rewards divide by active increments * WEIGHT_DENOMINATOR.
	// Bound the product for the full active validator capacity, including
	// validators activated after genesis. Divide each balance into increments
	// before multiplying so the bound itself cannot silently overflow.
	hi, maxActiveIncrements := bits.Mul64(maxActiveValidators, b.MaxEffectiveBalance/b.EffectiveBalanceIncrement)
	if hi != 0 {
		return fmt.Errorf("active validator capacity * (MAX_EFFECTIVE_BALANCE / EFFECTIVE_BALANCE_INCREMENT) overflows uint64")
	}
	if hi, _ := bits.Mul64(maxActiveIncrements, b.WeightDenominator); hi != 0 {
		return fmt.Errorf("maximum active balance increments (%d) * WEIGHT_DENOMINATOR (%d) overflows uint64",
			maxActiveIncrements, b.WeightDenominator)
	}
	// Configuration tooling can read either preset regardless of build tags.
	// ValidateStateLayout separately enforces the running binary's SSZ bounds.
	withdrawalLimit := uint64(fieldparams.MainnetMaxWithdrawalsPerPayload)
	if b.PresetBase == MinimalName {
		withdrawalLimit = fieldparams.MinimalMaxWithdrawalsPerPayload
	}
	return b.validateBlockOperationLimits(withdrawalLimit)
}

func (b *BeaconChainConfig) validateDepositTreeDepth() error {
	// The proof includes one extra element for the deposit-count mix-in.
	// Compare depths directly, without overflowing an untrusted depth + 1.
	const depth = fieldparams.DepositProofLength - 1
	if b.DepositContractTreeDepth != depth {
		return fmt.Errorf("DEPOSIT_CONTRACT_TREE_DEPTH (%d) must be %d to match the SSZ deposit proof length (%d)",
			b.DepositContractTreeDepth, depth, fieldparams.DepositProofLength)
	}
	return nil
}

func (b *BeaconChainConfig) validateBlockOperationLimits(withdrawalLimit uint64) error {
	// A zero withdrawal limit makes ProcessWithdrawals index an empty list
	// when it advances the next withdrawal validator index.
	if b.MaxWithdrawalsPerPayload == 0 {
		return fmt.Errorf("MAX_WITHDRAWALS_PER_PAYLOAD must be non-zero")
	}
	checks := []struct {
		name string
		cfg  uint64
		ssz  uint64
	}{
		{"MAX_PROPOSER_SLASHINGS", b.MaxProposerSlashings, fieldparams.MaxProposerSlashings},
		{"MAX_ATTESTER_SLASHINGS", b.MaxAttesterSlashings, fieldparams.MaxAttesterSlashings},
		{"MAX_ATTESTATIONS", b.MaxAttestations, fieldparams.MaxAttestations},
		{"MAX_DEPOSITS", b.MaxDeposits, fieldparams.MaxDeposits},
		{"MAX_VOLUNTARY_EXITS", b.MaxVoluntaryExits, fieldparams.MaxVoluntaryExits},
		{"MAX_WITHDRAWALS_PER_PAYLOAD", b.MaxWithdrawalsPerPayload, withdrawalLimit},
	}
	for _, c := range checks {
		// Smaller operation caps are valid: SSZ hashing still uses the compiled
		// list limit, while processing can enforce a stricter runtime cap.
		if c.cfg > c.ssz {
			return fmt.Errorf("%s (%d) must not exceed the SSZ block operation limit (%d)", c.name, c.cfg, c.ssz)
		}
	}
	return nil
}

// ValidateStateLayout checks that the configuration's vector sizes match the
// SSZ layout compiled into this binary (config/fieldparams, selected by the
// mainnet/minimal build tag). The beacon state's slashings, randao_mixes,
// block_roots, state_roots and historical_roots vectors, the validator
// registry limit and the sync committee size are fixed at build time, while
// the epoch transition indexes them with EPOCHS_PER_SLASHINGS_VECTOR,
// EPOCHS_PER_HISTORICAL_VECTOR and SLOTS_PER_HISTORICAL_ROOT from the runtime
// config: a mismatch ends in an out-of-range panic or in slashing / RANDAO
// lookups landing on the wrong epoch, not in an error.
// Committee sizes and block-operation caps must also fit the compiled SSZ bounds.
// Deposit-tree depth must match the fixed proof vector, including its count mix-in.
// Sync subnet counts must match the compiled contribution and aggregate bitfields.
// The execution-data vote list limit must match exactly for both SSZ bounds
// and Merkleization, including when the vote list is empty.
// The epoch length must also fit the binary's fixed fork-choice history buffer.
func (b *BeaconChainConfig) ValidateStateLayout() error {
	if err := b.validateCommitteeSize(); err != nil {
		return err
	}
	if err := b.validateDepositTreeDepth(); err != nil {
		return err
	}
	if err := b.validateBlockOperationLimits(fieldparams.MaxWithdrawalsPerPayload); err != nil {
		return err
	}
	// ExecutionDataVotesLength uses a multiplication that panics on overflow.
	// Check the product safely before comparing it with the compiled limit.
	hi, executionVotesLength := bits.Mul64(uint64(b.EpochsPerExecutionVotingPeriod), uint64(b.SlotsPerEpoch))
	if hi != 0 {
		return fmt.Errorf("EPOCHS_PER_EXECUTION_VOTING_PERIOD * SLOTS_PER_EPOCH overflows uint64")
	}
	// The proposer concatenates one fixed-size contribution bitfield per subnet.
	// Derive the subnet count without multiplying an untrusted override, which
	// could overflow and falsely match the aggregate's byte length.
	const syncCommitteeSubnetCount = fieldparams.SyncAggregateSyncCommitteeBytesLength / fieldparams.SyncCommitteeAggregationBytesLength
	checks := []struct {
		name string
		cfg  uint64
		ssz  uint64
	}{
		{"EPOCHS_PER_SLASHINGS_VECTOR", uint64(b.EpochsPerSlashingsVector), fieldparams.SlashingsLength},
		{"EPOCHS_PER_HISTORICAL_VECTOR", uint64(b.EpochsPerHistoricalVector), fieldparams.RandaoMixesLength},
		{"SLOTS_PER_HISTORICAL_ROOT", uint64(b.SlotsPerHistoricalRoot), fieldparams.BlockRootsLength},
		{"SLOTS_PER_HISTORICAL_ROOT", uint64(b.SlotsPerHistoricalRoot), fieldparams.StateRootsLength},
		{"HISTORICAL_ROOTS_LIMIT", b.HistoricalRootsLimit, fieldparams.HistoricalRootsLength},
		{"VALIDATOR_REGISTRY_LIMIT", b.ValidatorRegistryLimit, fieldparams.ValidatorRegistryLimit},
		{"SYNC_COMMITTEE_SIZE", b.SyncCommitteeSize, fieldparams.SyncCommitteeLength},
		{"SYNC_COMMITTEE_SUBNET_COUNT", b.SyncCommitteeSubnetCount, syncCommitteeSubnetCount},
		{"EPOCHS_PER_EXECUTION_VOTING_PERIOD * SLOTS_PER_EPOCH", executionVotesLength, fieldparams.ExecutionDataVotesLength},
	}
	for _, c := range checks {
		if c.cfg != c.ssz {
			return fmt.Errorf("%s is %d but this binary's SSZ state layout is compiled for %d (mainnet/minimal build mismatch)",
				c.name, c.cfg, c.ssz)
		}
	}
	// Fork choice indexes receivedBlocksLastEpoch by slot % SlotsPerEpoch,
	// but the array is sized using the compiled preset, not the runtime config.
	if b.SlotsPerEpoch > fieldparams.SlotsPerEpoch {
		return fmt.Errorf("SLOTS_PER_EPOCH (%d) must not exceed this binary's fork-choice history capacity (%d)",
			b.SlotsPerEpoch, fieldparams.SlotsPerEpoch)
	}
	return nil
}
