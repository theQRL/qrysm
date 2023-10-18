//go:build !minimal

package state_native

import (
	"sync"

	"github.com/prysmaticlabs/go-bitfield"
	"github.com/theQRL/qrysm/v4/beacon-chain/state/fieldtrie"
	customtypes "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native/custom-types"
	"github.com/theQRL/qrysm/v4/beacon-chain/state/state-native/types"
	"github.com/theQRL/qrysm/v4/beacon-chain/state/stateutil"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

// BeaconState defines a struct containing utilities for the Ethereum Beacon Chain state, defining
// getters and setters for its respective values and helpful functions such as HashTreeRoot().
type BeaconState struct {
	version                             int
	genesisTime                         uint64
	genesisValidatorsRoot               [32]byte
	slot                                primitives.Slot
	fork                                *zondpb.Fork
	latestBlockHeader                   *zondpb.BeaconBlockHeader
	blockRoots                          *customtypes.BlockRoots
	stateRoots                          *customtypes.StateRoots
	historicalRoots                     customtypes.HistoricalRoots
	historicalSummaries                 []*zondpb.HistoricalSummary
	eth1Data                            *zondpb.Eth1Data
	eth1DataVotes                       []*zondpb.Eth1Data
	eth1DepositIndex                    uint64
	validators                          []*zondpb.Validator
	balances                            []uint64
	randaoMixes                         *customtypes.RandaoMixes
	slashings                           []uint64
	previousEpochAttestations           []*zondpb.PendingAttestation
	currentEpochAttestations            []*zondpb.PendingAttestation
	previousEpochParticipation          []byte
	currentEpochParticipation           []byte
	justificationBits                   bitfield.Bitvector4
	previousJustifiedCheckpoint         *zondpb.Checkpoint
	currentJustifiedCheckpoint          *zondpb.Checkpoint
	finalizedCheckpoint                 *zondpb.Checkpoint
	inactivityScores                    []uint64
	currentSyncCommittee                *zondpb.SyncCommittee
	nextSyncCommittee                   *zondpb.SyncCommittee
	latestExecutionPayloadHeader        *enginev1.ExecutionPayloadHeader
	latestExecutionPayloadHeaderCapella *enginev1.ExecutionPayloadHeaderCapella
	nextWithdrawalIndex                 uint64
	nextWithdrawalValidatorIndex        primitives.ValidatorIndex

	lock                  sync.RWMutex
	dirtyFields           map[types.FieldIndex]bool
	dirtyIndices          map[types.FieldIndex][]uint64
	stateFieldLeaves      map[types.FieldIndex]*fieldtrie.FieldTrie
	rebuildTrie           map[types.FieldIndex]bool
	valMapHandler         *stateutil.ValidatorMapHandler
	merkleLayers          [][][]byte
	sharedFieldReferences map[types.FieldIndex]*stateutil.Reference
}
