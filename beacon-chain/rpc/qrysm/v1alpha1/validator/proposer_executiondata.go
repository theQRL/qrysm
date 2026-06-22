package validator

import (
	"context"
	"math/big"

	"github.com/pkg/errors"
	fastssz "github.com/prysmaticlabs/fastssz"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/features"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/crypto/hash"
	"github.com/theQRL/qrysm/crypto/rand"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/time/slots"
)

// executionDataMajorityVote determines the appropriate executiondata for a block proposal using
// an algorithm called Voting with the Majority. The algorithm works as follows:
//   - Determine the timestamp for the start slot for the execution voting period.
//   - Determine the earliest and latest timestamps that a valid block can have.
//   - Determine the first block not before the earliest timestamp. This block is the lower bound.
//   - Determine the last block not after the latest timestamp. This block is the upper bound.
//   - If the last block is too early, use current executiondata from the beacon state.
//   - Filter out votes on unknown blocks and blocks which are outside of the range determined by the lower and upper bounds.
//   - If no blocks are left after filtering votes, use executiondata from the latest valid block.
//   - Otherwise:
//   - Determine the vote with the highest count. Prefer the vote with the highest execution block height in the event of a tie.
//   - This vote's block is the execution block to use for the block proposal.
func (vs *Server) executionDataMajorityVote(ctx context.Context, beaconState state.BeaconState) (*qrysmpb.ExecutionData, error) {
	ctx, cancel := context.WithTimeout(ctx, executionDataTimeout)
	defer cancel()

	slot := beaconState.Slot()
	votingPeriodStartTime := vs.slotStartTime(slot)

	if vs.MockExecutionVotes {
		return vs.mockExecutionDataVote(ctx, slot)
	}
	if !vs.ExecutionInfoFetcher.ExecutionClientConnected() {
		return vs.randomExecutionDataVote(ctx)
	}
	executionDataNotification = false

	genesisTime, _ := vs.ExecutionInfoFetcher.GenesisExecutionChainInfo()
	followDistanceSeconds := params.BeaconConfig().ExecutionFollowDistance * params.BeaconConfig().SecondsPerExecutionBlock
	latestValidTime := votingPeriodStartTime - followDistanceSeconds
	earliestValidTime := votingPeriodStartTime - 2*followDistanceSeconds

	// Special case for starting from a pre-mined genesis: the execution vote should be genesis until the chain has advanced
	// by EXECUTION_FOLLOW_DISTANCE. The head state should maintain the same ExecutionData until this condition has passed, so
	// trust the existing head for the right execution vote until we can get a meaningful value from the deposit contract.
	if latestValidTime < genesisTime+followDistanceSeconds {
		log.WithField("genesisTime", genesisTime).WithField("latestValidTime", latestValidTime).Warn("voting period before genesis + follow distance, using executiondata from head")
		return vs.HeadFetcher.HeadExecutionData(), nil
	}

	lastBlockByLatestValidTime, err := vs.ExecutionBlockFetcher.BlockByTimestamp(ctx, latestValidTime)
	if err != nil {
		log.WithError(err).Error("Could not get last block by latest valid time")
		return vs.randomExecutionDataVote(ctx)
	}
	if lastBlockByLatestValidTime.Time < earliestValidTime {
		return vs.HeadFetcher.HeadExecutionData(), nil
	}

	lastBlockDepositCount, lastBlockDepositRoot := vs.DepositFetcher.DepositsNumberAndRootAtHeight(ctx, lastBlockByLatestValidTime.Number)
	if lastBlockDepositCount == 0 {
		return vs.ChainStartFetcher.ChainStartExecutionData(), nil
	}

	if lastBlockDepositCount >= vs.HeadFetcher.HeadExecutionData().DepositCount {
		h, err := vs.ExecutionBlockFetcher.BlockHashByHeight(ctx, lastBlockByLatestValidTime.Number)
		if err != nil {
			log.WithError(err).Error("Could not get hash of last block by latest valid time")
			return vs.randomExecutionDataVote(ctx)
		}
		return &qrysmpb.ExecutionData{
			BlockHash:    h.Bytes(),
			DepositCount: lastBlockDepositCount,
			DepositRoot:  lastBlockDepositRoot[:],
		}, nil
	}
	return vs.HeadFetcher.HeadExecutionData(), nil
}

func (vs *Server) slotStartTime(slot primitives.Slot) uint64 {
	startTime, _ := vs.ExecutionInfoFetcher.GenesisExecutionChainInfo()
	return slots.VotingPeriodStartTime(startTime, slot)
}

// canonicalExecutionData determines the canonical executiondata and execution block height to use for determining deposits.
func (vs *Server) canonicalExecutionData(
	ctx context.Context,
	beaconState state.BeaconState,
	currentVote *qrysmpb.ExecutionData) (*qrysmpb.ExecutionData, *big.Int, error) {
	var executionBlockHash [32]byte

	// Add in current vote, to get accurate vote tally
	if err := beaconState.AppendExecutionDataVotes(currentVote); err != nil {
		return nil, nil, errors.Wrap(err, "could not append execution data votes to state")
	}
	hasSupport, err := blocks.ExecutionDataHasEnoughSupport(beaconState, currentVote)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not determine if current executiondata vote has enough support")
	}
	var canonicalExecutionData *qrysmpb.ExecutionData
	if hasSupport {
		canonicalExecutionData = currentVote
		executionBlockHash = bytesutil.ToBytes32(currentVote.BlockHash)
	} else {
		canonicalExecutionData = beaconState.ExecutionData()
		executionBlockHash = bytesutil.ToBytes32(beaconState.ExecutionData().BlockHash)
	}
	if features.Get().DisableStakinContractCheck && executionBlockHash == [32]byte{} {
		return canonicalExecutionData, new(big.Int).SetInt64(0), nil
	}
	_, canonicalExecutionDataHeight, err := vs.ExecutionBlockFetcher.BlockExists(ctx, executionBlockHash)
	if err != nil {
		return nil, nil, errors.Wrap(err, "could not fetch executiondata height")
	}
	return canonicalExecutionData, canonicalExecutionDataHeight, nil
}

func (vs *Server) mockExecutionDataVote(ctx context.Context, slot primitives.Slot) (*qrysmpb.ExecutionData, error) {
	if !executionDataNotification {
		log.Warn("Beacon Node is no longer connected to an execution chain, so execution data votes are now mocked.")
		executionDataNotification = true
	}
	// If a mock execution data votes is specified, we use the following for the
	// executiondata we provide to every proposer based on https://github.com/ethereum/eth2.0-pm/issues/62:
	//
	// slot_in_voting_period = current_slot % SLOTS_PER_EXECUTION_VOTING_PERIOD
	// ExecutionData(
	//   DepositRoot = hash(current_epoch + slot_in_voting_period),
	//   DepositCount = state.execution_deposit_index,
	//   BlockHash = hash(hash(current_epoch + slot_in_voting_period)),
	// )
	slotInVotingPeriod := slot.ModSlot(params.BeaconConfig().SlotsPerEpoch.Mul(uint64(params.BeaconConfig().EpochsPerExecutionVotingPeriod)))
	headState, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, err
	}
	var enc []byte
	enc = fastssz.MarshalUint64(enc, uint64(slots.ToEpoch(slot))+uint64(slotInVotingPeriod))
	depRoot := hash.Hash(enc)
	blockHash := hash.Hash(depRoot[:])
	return &qrysmpb.ExecutionData{
		DepositRoot:  depRoot[:],
		DepositCount: headState.ExecutionDepositIndex(),
		BlockHash:    blockHash[:],
	}, nil
}

func (vs *Server) randomExecutionDataVote(ctx context.Context) (*qrysmpb.ExecutionData, error) {
	if !executionDataNotification {
		log.Warn("Beacon Node is no longer connected to an execution chain, so execution data votes are now random.")
		executionDataNotification = true
	}
	headState, err := vs.HeadFetcher.HeadStateReadOnly(ctx)
	if err != nil {
		return nil, err
	}

	// set random roots and block hashes to prevent a majority from being
	// built if the execution node is offline
	randGen := rand.NewGenerator()
	depRoot := hash.Hash(bytesutil.Bytes32(randGen.Uint64()))
	blockHash := hash.Hash(bytesutil.Bytes32(randGen.Uint64()))
	return &qrysmpb.ExecutionData{
		DepositRoot:  depRoot[:],
		DepositCount: headState.ExecutionDepositIndex(),
		BlockHash:    blockHash[:],
	}, nil
}
