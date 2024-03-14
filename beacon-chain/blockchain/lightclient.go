package blockchain

import (
	"bytes"
	"context"
	"fmt"

	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	"github.com/theQRL/qrysm/v4/proto/migration"
	zondpbv1 "github.com/theQRL/qrysm/v4/proto/zond/v1"
)

const (
	finalityBranchNumOfLeaves = 6
)

// CreateLightClientFinalityUpdate - implements https://github.com/ethereum/consensus-specs/blob/3d235740e5f1e641d3b160c8688f26e7dc5a1894/specs/altair/light-client/full-node.md#create_light_client_finality_update
// def create_light_client_finality_update(update: LightClientUpdate) -> LightClientFinalityUpdate:
//
//	return LightClientFinalityUpdate(
//	    attested_header=update.attested_header,
//	    finalized_header=update.finalized_header,
//	    finality_branch=update.finality_branch,
//	    sync_aggregate=update.sync_aggregate,
//	    signature_slot=update.signature_slot,
//	)
func CreateLightClientFinalityUpdate(update *zondpbv1.LightClientUpdate) *zondpbv1.LightClientFinalityUpdate {
	return &zondpbv1.LightClientFinalityUpdate{
		AttestedHeader:  update.AttestedHeader,
		FinalizedHeader: update.FinalizedHeader,
		FinalityBranch:  update.FinalityBranch,
		SyncAggregate:   update.SyncAggregate,
		SignatureSlot:   update.SignatureSlot,
	}
}

// CreateLightClientOptimisticUpdate - implements https://github.com/ethereum/consensus-specs/blob/3d235740e5f1e641d3b160c8688f26e7dc5a1894/specs/altair/light-client/full-node.md#create_light_client_optimistic_update
// def create_light_client_optimistic_update(update: LightClientUpdate) -> LightClientOptimisticUpdate:
//
//	return LightClientOptimisticUpdate(
//	    attested_header=update.attested_header,
//	    sync_aggregate=update.sync_aggregate,
//	    signature_slot=update.signature_slot,
//	)
func CreateLightClientOptimisticUpdate(update *zondpbv1.LightClientUpdate) *zondpbv1.LightClientOptimisticUpdate {
	return &zondpbv1.LightClientOptimisticUpdate{
		AttestedHeader: update.AttestedHeader,
		SyncAggregate:  update.SyncAggregate,
		SignatureSlot:  update.SignatureSlot,
	}
}

func NewLightClientOptimisticUpdateFromBeaconState(
	ctx context.Context,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState) (*zondpbv1.LightClientUpdate, error) {
	// assert sum(block.message.body.sync_aggregate.sync_committee_bits) >= MIN_SYNC_COMMITTEE_PARTICIPANTS
	syncAggregate, err := block.Block().Body().SyncAggregate()
	if err != nil {
		return nil, fmt.Errorf("could not get sync aggregate %v", err)
	}

	if syncAggregate.SyncCommitteeBits.Count() < params.BeaconConfig().MinSyncCommitteeParticipants {
		return nil, fmt.Errorf("invalid sync committee bits count %d", syncAggregate.SyncCommitteeBits.Count())
	}

	// assert state.slot == state.latest_block_header.slot
	if state.Slot() != state.LatestBlockHeader().Slot {
		return nil, fmt.Errorf("state slot %d not equal to latest block header slot %d", state.Slot(), state.LatestBlockHeader().Slot)
	}

	// assert hash_tree_root(header) == hash_tree_root(block.message)
	header := state.LatestBlockHeader()
	stateRoot, err := state.HashTreeRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get state root %v", err)
	}
	header.StateRoot = stateRoot[:]

	headerRoot, err := header.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("could not get header root %v", err)
	}

	blockRoot, err := block.Block().HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("could not get block root %v", err)
	}

	if headerRoot != blockRoot {
		return nil, fmt.Errorf("header root %#x not equal to block root %#x", headerRoot, blockRoot)
	}

	// assert attested_state.slot == attested_state.latest_block_header.slot
	if attestedState.Slot() != attestedState.LatestBlockHeader().Slot {
		return nil, fmt.Errorf("attested state slot %d not equal to attested latest block header slot %d", attestedState.Slot(), attestedState.LatestBlockHeader().Slot)
	}

	// attested_header = attested_state.latest_block_header.copy()
	attestedHeader := attestedState.LatestBlockHeader()

	// attested_header.state_root = hash_tree_root(attested_state)
	attestedStateRoot, err := attestedState.HashTreeRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get attested state root %v", err)
	}
	attestedHeader.StateRoot = attestedStateRoot[:]

	// assert hash_tree_root(attested_header) == block.message.parent_root
	attestedHeaderRoot, err := attestedHeader.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("could not get attested header root %v", err)
	}

	if attestedHeaderRoot != block.Block().ParentRoot() {
		return nil, fmt.Errorf("attested header root %#x not equal to block parent root %#x", attestedHeaderRoot, block.Block().ParentRoot())
	}

	// Return result
	attestedHeaderResult := &zondpbv1.BeaconBlockHeader{
		Slot:          attestedHeader.Slot,
		ProposerIndex: attestedHeader.ProposerIndex,
		ParentRoot:    attestedHeader.ParentRoot,
		StateRoot:     attestedHeader.StateRoot,
		BodyRoot:      attestedHeader.BodyRoot,
	}

	syncAggregateResult := &zondpbv1.SyncAggregate{
		SyncCommitteeBits:       syncAggregate.SyncCommitteeBits,
		SyncCommitteeSignatures: syncAggregate.SyncCommitteeSignatures,
	}

	result := &zondpbv1.LightClientUpdate{
		AttestedHeader: attestedHeaderResult,
		SyncAggregate:  syncAggregateResult,
		SignatureSlot:  block.Block().Slot(),
	}

	return result, nil
}

func NewLightClientFinalityUpdateFromBeaconState(
	ctx context.Context,
	state state.BeaconState,
	block interfaces.ReadOnlySignedBeaconBlock,
	attestedState state.BeaconState,
	finalizedBlock interfaces.ReadOnlySignedBeaconBlock) (*zondpbv1.LightClientUpdate, error) {
	result, err := NewLightClientOptimisticUpdateFromBeaconState(
		ctx,
		state,
		block,
		attestedState,
	)
	if err != nil {
		return nil, err
	}

	// Indicate finality whenever possible
	var finalizedHeader *zondpbv1.BeaconBlockHeader
	var finalityBranch [][]byte

	if finalizedBlock != nil && !finalizedBlock.IsNil() {
		if finalizedBlock.Block().Slot() != 0 {
			tempFinalizedHeader, err := finalizedBlock.Header()
			if err != nil {
				return nil, fmt.Errorf("could not get finalized header %v", err)
			}
			finalizedHeader = migration.V1Alpha1SignedHeaderToV1(tempFinalizedHeader).GetMessage()

			finalizedHeaderRoot, err := finalizedHeader.HashTreeRoot()
			if err != nil {
				return nil, fmt.Errorf("could not get finalized header root %v", err)
			}

			if finalizedHeaderRoot != bytesutil.ToBytes32(attestedState.FinalizedCheckpoint().Root) {
				return nil, fmt.Errorf("finalized header root %#x not equal to attested finalized checkpoint root %#x", finalizedHeaderRoot, bytesutil.ToBytes32(attestedState.FinalizedCheckpoint().Root))
			}
		} else {
			if !bytes.Equal(attestedState.FinalizedCheckpoint().Root, make([]byte, 32)) {
				return nil, fmt.Errorf("invalid finalized header root %v", attestedState.FinalizedCheckpoint().Root)
			}

			finalizedHeader = &zondpbv1.BeaconBlockHeader{
				Slot:          0,
				ProposerIndex: 0,
				ParentRoot:    make([]byte, 32),
				StateRoot:     make([]byte, 32),
				BodyRoot:      make([]byte, 32),
			}
		}

		var bErr error
		finalityBranch, bErr = attestedState.FinalizedRootProof(ctx)
		if bErr != nil {
			return nil, fmt.Errorf("could not get finalized root proof %v", bErr)
		}
	} else {
		finalizedHeader = &zondpbv1.BeaconBlockHeader{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    make([]byte, 32),
			StateRoot:     make([]byte, 32),
			BodyRoot:      make([]byte, 32),
		}

		finalityBranch = make([][]byte, finalityBranchNumOfLeaves)
		for i := 0; i < finalityBranchNumOfLeaves; i++ {
			finalityBranch[i] = make([]byte, 32)
		}
	}

	result.FinalizedHeader = finalizedHeader
	result.FinalityBranch = finalityBranch
	return result, nil
}

func NewLightClientUpdateFromFinalityUpdate(update *zondpbv1.LightClientFinalityUpdate) *zondpbv1.LightClientUpdate {
	return &zondpbv1.LightClientUpdate{
		AttestedHeader:  update.AttestedHeader,
		FinalizedHeader: update.FinalizedHeader,
		FinalityBranch:  update.FinalityBranch,
		SyncAggregate:   update.SyncAggregate,
		SignatureSlot:   update.SignatureSlot,
	}
}

func NewLightClientUpdateFromOptimisticUpdate(update *zondpbv1.LightClientOptimisticUpdate) *zondpbv1.LightClientUpdate {
	return &zondpbv1.LightClientUpdate{
		AttestedHeader: update.AttestedHeader,
		SyncAggregate:  update.SyncAggregate,
		SignatureSlot:  update.SignatureSlot,
	}
}
