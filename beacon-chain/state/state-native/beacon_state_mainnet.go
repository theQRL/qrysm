//go:build !minimal

package state_native

import (
	"encoding/json"
	"sync"

	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/beacon-chain/state/fieldtrie"
	customtypes "github.com/theQRL/qrysm/beacon-chain/state/state-native/custom-types"
	"github.com/theQRL/qrysm/beacon-chain/state/state-native/types"
	"github.com/theQRL/qrysm/beacon-chain/state/stateutil"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// BeaconState defines a struct containing utilities for the QRL Beacon Chain state, defining
// getters and setters for its respective values and helpful functions such as HashTreeRoot().
type BeaconState struct {
	version                          int
	genesisTime                      uint64
	genesisValidatorsRoot            [32]byte
	slot                             primitives.Slot
	fork                             *qrysmpb.Fork
	latestBlockHeader                *qrysmpb.BeaconBlockHeader
	blockRoots                       customtypes.BlockRoots
	blockRootsMultiValue             *MultiValueBlockRoots
	stateRoots                       customtypes.StateRoots
	stateRootsMultiValue             *MultiValueStateRoots
	historicalRoots                  customtypes.HistoricalRoots
	historicalSummaries              []*qrysmpb.HistoricalSummary
	executionData                    *qrysmpb.ExecutionData
	executionDataVotes               []*qrysmpb.ExecutionData
	executionDepositIndex            uint64
	validators                       []*qrysmpb.Validator
	validatorsMultiValue             *MultiValueValidators
	balances                         []uint64
	balancesMultiValue               *MultiValueBalances
	randaoMixes                      customtypes.RandaoMixes
	randaoMixesMultiValue            *MultiValueRandaoMixes
	slashings                        []uint64
	previousEpochParticipation       []byte
	currentEpochParticipation        []byte
	justificationBits                bitfield.Bitvector4
	previousJustifiedCheckpoint      *qrysmpb.Checkpoint
	currentJustifiedCheckpoint       *qrysmpb.Checkpoint
	finalizedCheckpoint              *qrysmpb.Checkpoint
	inactivityScores                 []uint64
	inactivityScoresMultiValue       *MultiValueInactivityScores
	currentSyncCommittee             *qrysmpb.SyncCommittee
	nextSyncCommittee                *qrysmpb.SyncCommittee
	latestExecutionPayloadHeaderZond *enginev1.ExecutionPayloadHeaderZond
	nextWithdrawalIndex              uint64
	nextWithdrawalValidatorIndex     primitives.ValidatorIndex

	id                    uint64
	lock                  sync.RWMutex
	dirtyFields           map[types.FieldIndex]bool
	dirtyIndices          map[types.FieldIndex][]uint64
	stateFieldLeaves      map[types.FieldIndex]*fieldtrie.FieldTrie
	rebuildTrie           map[types.FieldIndex]bool
	valMapHandler         *stateutil.ValidatorMapHandler
	merkleLayers          [][][]byte
	sharedFieldReferences map[types.FieldIndex]*stateutil.Reference
}

type beaconStateMarshalable struct {
	Version                          int                                  `json:"version" yaml:"version"`
	GenesisTime                      uint64                               `json:"genesis_time" yaml:"genesis_time"`
	GenesisValidatorsRoot            [32]byte                             `json:"genesis_validators_root" yaml:"genesis_validators_root"`
	Slot                             primitives.Slot                      `json:"slot" yaml:"slot"`
	Fork                             *qrysmpb.Fork                        `json:"fork" yaml:"fork"`
	LatestBlockHeader                *qrysmpb.BeaconBlockHeader           `json:"latest_block_header" yaml:"latest_block_header"`
	BlockRoots                       customtypes.BlockRoots               `json:"block_roots" yaml:"block_roots"`
	StateRoots                       customtypes.StateRoots               `json:"state_roots" yaml:"state_roots"`
	HistoricalRoots                  customtypes.HistoricalRoots          `json:"historical_roots" yaml:"historical_roots"`
	HistoricalSummaries              []*qrysmpb.HistoricalSummary         `json:"historical_summaries" yaml:"historical_summaries"`
	ExecutionData                    *qrysmpb.ExecutionData               `json:"execution_data" yaml:"execution_data"`
	ExecutionDataVotes               []*qrysmpb.ExecutionData             `json:"execution_data_votes" yaml:"execution_data_votes"`
	ExecutionDepositIndex            uint64                               `json:"execution_deposit_index" yaml:"execution_deposit_index"`
	Validators                       []*qrysmpb.Validator                 `json:"validators" yaml:"validators"`
	Balances                         []uint64                             `json:"balances" yaml:"balances"`
	RandaoMixes                      customtypes.RandaoMixes              `json:"randao_mixes" yaml:"randao_mixes"`
	Slashings                        []uint64                             `json:"slashings" yaml:"slashings"`
	PreviousEpochParticipation       []byte                               `json:"previous_epoch_participation" yaml:"previous_epoch_participation"`
	CurrentEpochParticipation        []byte                               `json:"current_epoch_participation" yaml:"current_epoch_participation"`
	JustificationBits                bitfield.Bitvector4                  `json:"justification_bits" yaml:"justification_bits"`
	PreviousJustifiedCheckpoint      *qrysmpb.Checkpoint                  `json:"previous_justified_checkpoint" yaml:"previous_justified_checkpoint"`
	CurrentJustifiedCheckpoint       *qrysmpb.Checkpoint                  `json:"current_justified_checkpoint" yaml:"current_justified_checkpoint"`
	FinalizedCheckpoint              *qrysmpb.Checkpoint                  `json:"finalized_checkpoint" yaml:"finalized_checkpoint"`
	InactivityScores                 []uint64                             `json:"inactivity_scores" yaml:"inactivity_scores"`
	CurrentSyncCommittee             *qrysmpb.SyncCommittee               `json:"current_sync_committee" yaml:"current_sync_committee"`
	NextSyncCommittee                *qrysmpb.SyncCommittee               `json:"next_sync_committee" yaml:"next_sync_committee"`
	LatestExecutionPayloadHeaderZond *enginev1.ExecutionPayloadHeaderZond `json:"latest_execution_payload_header_zond" yaml:"latest_execution_payload_header_zond"`
	NextWithdrawalIndex              uint64                               `json:"next_withdrawal_index" yaml:"next_withdrawal_index"`
	NextWithdrawalValidatorIndex     primitives.ValidatorIndex            `json:"next_withdrawal_validator_index" yaml:"next_withdrawal_validator_index"`
}

func (b *BeaconState) MarshalJSON() ([]byte, error) {
	var bRoots customtypes.BlockRoots
	var sRoots customtypes.StateRoots
	var mixes customtypes.RandaoMixes
	var balances []uint64
	var inactivityScores []uint64
	var vals []*qrysmpb.Validator

	if features.Get().EnableExperimentalState {
		bRoots = b.blockRootsMultiValue.Value(b)
		sRoots = b.stateRootsMultiValue.Value(b)
		mixes = b.randaoMixesMultiValue.Value(b)
		balances = b.balancesMultiValue.Value(b)
		inactivityScores = b.inactivityScoresMultiValue.Value(b)
		vals = b.validatorsMultiValue.Value(b)
	} else {
		bRoots = b.blockRoots
		sRoots = b.stateRoots
		mixes = b.randaoMixes
		balances = b.balances
		inactivityScores = b.inactivityScores
		vals = b.validators
	}

	marshalable := &beaconStateMarshalable{
		Version:                          b.version,
		GenesisTime:                      b.genesisTime,
		GenesisValidatorsRoot:            b.genesisValidatorsRoot,
		Slot:                             b.slot,
		Fork:                             b.fork,
		LatestBlockHeader:                b.latestBlockHeader,
		BlockRoots:                       bRoots,
		StateRoots:                       sRoots,
		HistoricalRoots:                  b.historicalRoots,
		HistoricalSummaries:              b.historicalSummaries,
		ExecutionData:                    b.executionData,
		ExecutionDataVotes:               b.executionDataVotes,
		ExecutionDepositIndex:            b.executionDepositIndex,
		Validators:                       vals,
		Balances:                         balances,
		RandaoMixes:                      mixes,
		Slashings:                        b.slashings,
		PreviousEpochParticipation:       b.previousEpochParticipation,
		CurrentEpochParticipation:        b.currentEpochParticipation,
		JustificationBits:                b.justificationBits,
		PreviousJustifiedCheckpoint:      b.previousJustifiedCheckpoint,
		CurrentJustifiedCheckpoint:       b.currentJustifiedCheckpoint,
		FinalizedCheckpoint:              b.finalizedCheckpoint,
		InactivityScores:                 inactivityScores,
		CurrentSyncCommittee:             b.currentSyncCommittee,
		NextSyncCommittee:                b.nextSyncCommittee,
		LatestExecutionPayloadHeaderZond: b.latestExecutionPayloadHeaderZond,
		NextWithdrawalIndex:              b.nextWithdrawalIndex,
		NextWithdrawalValidatorIndex:     b.nextWithdrawalValidatorIndex,
	}
	return json.Marshal(marshalable)
}
