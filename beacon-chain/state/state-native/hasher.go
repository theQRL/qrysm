package state_native

import (
	"context"
	"encoding/binary"

	"github.com/pkg/errors"
	"github.com/theQRL/qrysm/beacon-chain/state/state-native/types"
	"github.com/theQRL/qrysm/beacon-chain/state/stateutil"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	"github.com/theQRL/qrysm/encoding/ssz"
	"github.com/theQRL/qrysm/runtime/version"
	"go.opencensus.io/trace"
)

// ComputeFieldRootsWithHasher hashes the provided state and returns its respective field roots.
func ComputeFieldRootsWithHasher(ctx context.Context, state *BeaconState) ([][]byte, error) {
	_, span := trace.StartSpan(ctx, "ComputeFieldRootsWithHasher")
	defer span.End()

	if state == nil {
		return nil, errors.New("nil state")
	}
	var fieldRoots [][]byte
	switch state.version {
	case version.Zond:
		fieldRoots = make([][]byte, params.BeaconConfig().BeaconStateZondFieldCount)
	}

	// Genesis time root.
	genesisRoot := ssz.Uint64Root(state.genesisTime)
	fieldRoots[types.GenesisTime.RealPosition()] = genesisRoot[:]

	// Genesis validators root.
	var r [32]byte
	copy(r[:], state.genesisValidatorsRoot[:])
	fieldRoots[types.GenesisValidatorsRoot.RealPosition()] = r[:]

	// Slot root.
	slotRoot := ssz.Uint64Root(uint64(state.slot))
	fieldRoots[types.Slot.RealPosition()] = slotRoot[:]

	// Fork data structure root.
	forkHashTreeRoot, err := ssz.ForkRoot(state.fork)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute fork merkleization")
	}
	fieldRoots[types.Fork.RealPosition()] = forkHashTreeRoot[:]

	// BeaconBlockHeader data structure root.
	headerHashTreeRoot, err := stateutil.BlockHeaderRoot(state.latestBlockHeader)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block header merkleization")
	}
	fieldRoots[types.LatestBlockHeader.RealPosition()] = headerHashTreeRoot[:]

	// BlockRoots array root.
	blockRootsRoot, err := stateutil.ArraysRoot(state.blockRootsVal().Slice(), fieldparams.BlockRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute block roots merkleization")
	}
	fieldRoots[types.BlockRoots.RealPosition()] = blockRootsRoot[:]

	// StateRoots array root.
	stateRootsRoot, err := stateutil.ArraysRoot(state.stateRootsVal().Slice(), fieldparams.StateRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute state roots merkleization")
	}
	fieldRoots[types.StateRoots.RealPosition()] = stateRootsRoot[:]

	// HistoricalRoots slice root.
	hRoots := make([][]byte, len(state.historicalRoots))
	for i := range hRoots {
		hRoots[i] = state.historicalRoots[i][:]
	}
	historicalRootsRt, err := ssz.ByteArrayRootWithLimit(hRoots, fieldparams.HistoricalRootsLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute historical roots merkleization")
	}
	fieldRoots[types.HistoricalRoots.RealPosition()] = historicalRootsRt[:]

	// ExecutionData data structure root.
	executionHashTreeRoot, err := stateutil.ExecutionRoot(state.executionData)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute executiondata merkleization")
	}
	fieldRoots[types.ExecutionData.RealPosition()] = executionHashTreeRoot[:]

	// ExecutionDataVotes slice root.
	executionVotesRoot, err := stateutil.ExecutionDataVotesRoot(state.executionDataVotes)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute executiondata votes merkleization")
	}
	fieldRoots[types.ExecutionDataVotes.RealPosition()] = executionVotesRoot[:]

	// ExecutionDepositIndex root.
	executionDepositIndexBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(executionDepositIndexBuf, state.executionDepositIndex)
	executionDepositBuf := bytesutil.ToBytes32(executionDepositIndexBuf)
	fieldRoots[types.ExecutionDepositIndex.RealPosition()] = executionDepositBuf[:]

	// Validators slice root.
	validatorsRoot, err := stateutil.ValidatorRegistryRoot(state.validatorsVal())
	if err != nil {
		return nil, errors.Wrap(err, "could not compute validator registry merkleization")
	}
	fieldRoots[types.Validators.RealPosition()] = validatorsRoot[:]

	// Balances slice root.
	balancesRoot, err := stateutil.Uint64ListRootWithRegistryLimit(state.balancesVal())
	if err != nil {
		return nil, errors.Wrap(err, "could not compute validator balances merkleization")
	}
	fieldRoots[types.Balances.RealPosition()] = balancesRoot[:]

	// RandaoMixes array root.
	randaoRootsRoot, err := stateutil.ArraysRoot(state.randaoMixesVal().Slice(), fieldparams.RandaoMixesLength)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute randao roots merkleization")
	}
	fieldRoots[types.RandaoMixes.RealPosition()] = randaoRootsRoot[:]

	// Slashings array root.
	slashingsRootsRoot, err := ssz.SlashingsRoot(state.slashings)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute slashings merkleization")
	}
	fieldRoots[types.Slashings.RealPosition()] = slashingsRootsRoot[:]

	// PreviousEpochParticipation slice root.
	prevParticipationRoot, err := stateutil.ParticipationBitsRoot(state.previousEpochParticipation)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute previous epoch participation merkleization")
	}
	fieldRoots[types.PreviousEpochParticipationBits.RealPosition()] = prevParticipationRoot[:]

	// CurrentEpochParticipation slice root.
	currParticipationRoot, err := stateutil.ParticipationBitsRoot(state.currentEpochParticipation)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute current epoch participation merkleization")
	}
	fieldRoots[types.CurrentEpochParticipationBits.RealPosition()] = currParticipationRoot[:]

	// JustificationBits root.
	justifiedBitsRoot := bytesutil.ToBytes32(state.justificationBits)
	fieldRoots[types.JustificationBits.RealPosition()] = justifiedBitsRoot[:]

	// PreviousJustifiedCheckpoint data structure root.
	prevCheckRoot, err := ssz.CheckpointRoot(state.previousJustifiedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute previous justified checkpoint merkleization")
	}
	fieldRoots[types.PreviousJustifiedCheckpoint.RealPosition()] = prevCheckRoot[:]

	// CurrentJustifiedCheckpoint data structure root.
	currJustRoot, err := ssz.CheckpointRoot(state.currentJustifiedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute current justified checkpoint merkleization")
	}
	fieldRoots[types.CurrentJustifiedCheckpoint.RealPosition()] = currJustRoot[:]

	// FinalizedCheckpoint data structure root.
	finalRoot, err := ssz.CheckpointRoot(state.finalizedCheckpoint)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute finalized checkpoint merkleization")
	}
	fieldRoots[types.FinalizedCheckpoint.RealPosition()] = finalRoot[:]

	// Inactivity scores root.
	inactivityScoresRoot, err := stateutil.Uint64ListRootWithRegistryLimit(state.inactivityScoresVal())
	if err != nil {
		return nil, errors.Wrap(err, "could not compute inactivityScoreRoot")
	}
	fieldRoots[types.InactivityScores.RealPosition()] = inactivityScoresRoot[:]

	// Current sync committee root.
	currentSyncCommitteeRoot, err := stateutil.SyncCommitteeRoot(state.currentSyncCommittee)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute sync committee merkleization")
	}
	fieldRoots[types.CurrentSyncCommittee.RealPosition()] = currentSyncCommitteeRoot[:]

	// Next sync committee root.
	nextSyncCommitteeRoot, err := stateutil.SyncCommitteeRoot(state.nextSyncCommittee)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute sync committee merkleization")
	}
	fieldRoots[types.NextSyncCommittee.RealPosition()] = nextSyncCommitteeRoot[:]

	// Execution payload root.
	executionPayloadRoot, err := state.latestExecutionPayloadHeaderZond.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	fieldRoots[types.LatestExecutionPayloadHeaderZond.RealPosition()] = executionPayloadRoot[:]

	// Next withdrawal index root.
	nextWithdrawalIndexRoot := make([]byte, 32)
	binary.LittleEndian.PutUint64(nextWithdrawalIndexRoot, state.nextWithdrawalIndex)
	fieldRoots[types.NextWithdrawalIndex.RealPosition()] = nextWithdrawalIndexRoot

	// Next partial withdrawal validator index root.
	nextWithdrawalValidatorIndexRoot := make([]byte, 32)
	binary.LittleEndian.PutUint64(nextWithdrawalValidatorIndexRoot, uint64(state.nextWithdrawalValidatorIndex))
	fieldRoots[types.NextWithdrawalValidatorIndex.RealPosition()] = nextWithdrawalValidatorIndexRoot

	// Historical summary root.
	historicalSummaryRoot, err := stateutil.HistoricalSummariesRoot(state.historicalSummaries)
	if err != nil {
		return nil, errors.Wrap(err, "could not compute historical summary merkleization")
	}
	fieldRoots[types.HistoricalSummaries.RealPosition()] = historicalSummaryRoot[:]

	return fieldRoots, nil
}
