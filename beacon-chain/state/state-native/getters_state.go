package state_native

import (
	"github.com/pkg/errors"
	customtypes "github.com/theQRL/qrysm/v4/beacon-chain/state/state-native/custom-types"
	"github.com/theQRL/qrysm/v4/config/features"
	consensus_types "github.com/theQRL/qrysm/v4/consensus-types"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/runtime/version"
)

// ToProtoUnsafe returns the pointer value of the underlying
// beacon state proto object, bypassing immutability. Use with care.
func (b *BeaconState) ToProtoUnsafe() interface{} {
	if b == nil {
		return nil
	}

	gvrCopy := b.genesisValidatorsRoot
	br := b.blockRootsVal().Slice()
	sr := b.stateRootsVal().Slice()
	rm := b.randaoMixesVal().Slice()
	var vals []*zondpb.Validator
	var bals []uint64
	if features.Get().EnableExperimentalState {
		vals = b.validatorsVal()
		bals = b.balancesVal()
	} else {
		vals = b.validators
		bals = b.balances
	}

	switch b.version {
	case version.Phase0:
		return &zondpb.BeaconState{
			GenesisTime:                 b.genesisTime,
			GenesisValidatorsRoot:       gvrCopy[:],
			Slot:                        b.slot,
			Fork:                        b.fork,
			LatestBlockHeader:           b.latestBlockHeader,
			BlockRoots:                  br,
			StateRoots:                  sr,
			HistoricalRoots:             b.historicalRoots.Slice(),
			Eth1Data:                    b.eth1Data,
			Eth1DataVotes:               b.eth1DataVotes,
			Eth1DepositIndex:            b.eth1DepositIndex,
			Validators:                  vals,
			Balances:                    bals,
			RandaoMixes:                 rm,
			Slashings:                   b.slashings,
			PreviousEpochAttestations:   b.previousEpochAttestations,
			CurrentEpochAttestations:    b.currentEpochAttestations,
			JustificationBits:           b.justificationBits,
			PreviousJustifiedCheckpoint: b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:  b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:         b.finalizedCheckpoint,
		}
	case version.Altair:
		return &zondpb.BeaconStateAltair{
			GenesisTime:                 b.genesisTime,
			GenesisValidatorsRoot:       gvrCopy[:],
			Slot:                        b.slot,
			Fork:                        b.fork,
			LatestBlockHeader:           b.latestBlockHeader,
			BlockRoots:                  br,
			StateRoots:                  sr,
			HistoricalRoots:             b.historicalRoots.Slice(),
			Eth1Data:                    b.eth1Data,
			Eth1DataVotes:               b.eth1DataVotes,
			Eth1DepositIndex:            b.eth1DepositIndex,
			Validators:                  vals,
			Balances:                    bals,
			RandaoMixes:                 rm,
			Slashings:                   b.slashings,
			PreviousEpochParticipation:  b.previousEpochParticipation,
			CurrentEpochParticipation:   b.currentEpochParticipation,
			JustificationBits:           b.justificationBits,
			PreviousJustifiedCheckpoint: b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:  b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:         b.finalizedCheckpoint,
			InactivityScores:            b.inactivityScoresVal(),
			CurrentSyncCommittee:        b.currentSyncCommittee,
			NextSyncCommittee:           b.nextSyncCommittee,
		}
	case version.Bellatrix:
		return &zondpb.BeaconStateBellatrix{
			GenesisTime:                  b.genesisTime,
			GenesisValidatorsRoot:        gvrCopy[:],
			Slot:                         b.slot,
			Fork:                         b.fork,
			LatestBlockHeader:            b.latestBlockHeader,
			BlockRoots:                   br,
			StateRoots:                   sr,
			HistoricalRoots:              b.historicalRoots.Slice(),
			Eth1Data:                     b.eth1Data,
			Eth1DataVotes:                b.eth1DataVotes,
			Eth1DepositIndex:             b.eth1DepositIndex,
			Validators:                   vals,
			Balances:                     bals,
			RandaoMixes:                  rm,
			Slashings:                    b.slashings,
			PreviousEpochParticipation:   b.previousEpochParticipation,
			CurrentEpochParticipation:    b.currentEpochParticipation,
			JustificationBits:            b.justificationBits,
			PreviousJustifiedCheckpoint:  b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:   b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:          b.finalizedCheckpoint,
			InactivityScores:             b.inactivityScoresVal(),
			CurrentSyncCommittee:         b.currentSyncCommittee,
			NextSyncCommittee:            b.nextSyncCommittee,
			LatestExecutionPayloadHeader: b.latestExecutionPayloadHeader,
		}
	case version.Capella:
		return &zondpb.BeaconStateCapella{
			GenesisTime:                  b.genesisTime,
			GenesisValidatorsRoot:        gvrCopy[:],
			Slot:                         b.slot,
			Fork:                         b.fork,
			LatestBlockHeader:            b.latestBlockHeader,
			BlockRoots:                   br,
			StateRoots:                   sr,
			HistoricalRoots:              b.historicalRoots.Slice(),
			Eth1Data:                     b.eth1Data,
			Eth1DataVotes:                b.eth1DataVotes,
			Eth1DepositIndex:             b.eth1DepositIndex,
			Validators:                   vals,
			Balances:                     bals,
			RandaoMixes:                  rm,
			Slashings:                    b.slashings,
			PreviousEpochParticipation:   b.previousEpochParticipation,
			CurrentEpochParticipation:    b.currentEpochParticipation,
			JustificationBits:            b.justificationBits,
			PreviousJustifiedCheckpoint:  b.previousJustifiedCheckpoint,
			CurrentJustifiedCheckpoint:   b.currentJustifiedCheckpoint,
			FinalizedCheckpoint:          b.finalizedCheckpoint,
			InactivityScores:             b.inactivityScoresVal(),
			CurrentSyncCommittee:         b.currentSyncCommittee,
			NextSyncCommittee:            b.nextSyncCommittee,
			LatestExecutionPayloadHeader: b.latestExecutionPayloadHeaderCapella,
			NextWithdrawalIndex:          b.nextWithdrawalIndex,
			NextWithdrawalValidatorIndex: b.nextWithdrawalValidatorIndex,
			HistoricalSummaries:          b.historicalSummaries,
		}
	default:
		return nil
	}
}

// ToProto the beacon state into a protobuf for usage.
func (b *BeaconState) ToProto() interface{} {
	if b == nil {
		return nil
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	gvrCopy := b.genesisValidatorsRoot
	br := b.blockRootsVal().Slice()
	sr := b.stateRootsVal().Slice()
	rm := b.randaoMixesVal().Slice()

	var inactivityScores []uint64
	if b.version > version.Phase0 {
		inactivityScores = b.inactivityScoresVal()
	}

	switch b.version {
	case version.Phase0:
		return &zondpb.BeaconState{
			GenesisTime:                 b.genesisTime,
			GenesisValidatorsRoot:       gvrCopy[:],
			Slot:                        b.slot,
			Fork:                        b.forkVal(),
			LatestBlockHeader:           b.latestBlockHeaderVal(),
			BlockRoots:                  br,
			StateRoots:                  sr,
			HistoricalRoots:             b.historicalRoots.Slice(),
			Eth1Data:                    b.eth1DataVal(),
			Eth1DataVotes:               b.eth1DataVotesVal(),
			Eth1DepositIndex:            b.eth1DepositIndex,
			Validators:                  b.validatorsVal(),
			Balances:                    b.balancesVal(),
			RandaoMixes:                 rm,
			Slashings:                   b.slashingsVal(),
			PreviousEpochAttestations:   b.previousEpochAttestationsVal(),
			CurrentEpochAttestations:    b.currentEpochAttestationsVal(),
			JustificationBits:           b.justificationBitsVal(),
			PreviousJustifiedCheckpoint: b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:  b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:         b.finalizedCheckpointVal(),
		}
	case version.Altair:
		return &zondpb.BeaconStateAltair{
			GenesisTime:                 b.genesisTime,
			GenesisValidatorsRoot:       gvrCopy[:],
			Slot:                        b.slot,
			Fork:                        b.forkVal(),
			LatestBlockHeader:           b.latestBlockHeaderVal(),
			BlockRoots:                  br,
			StateRoots:                  sr,
			HistoricalRoots:             b.historicalRoots.Slice(),
			Eth1Data:                    b.eth1DataVal(),
			Eth1DataVotes:               b.eth1DataVotesVal(),
			Eth1DepositIndex:            b.eth1DepositIndex,
			Validators:                  b.validatorsVal(),
			Balances:                    b.balancesVal(),
			RandaoMixes:                 rm,
			Slashings:                   b.slashingsVal(),
			PreviousEpochParticipation:  b.previousEpochParticipationVal(),
			CurrentEpochParticipation:   b.currentEpochParticipationVal(),
			JustificationBits:           b.justificationBitsVal(),
			PreviousJustifiedCheckpoint: b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:  b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:         b.finalizedCheckpointVal(),
			InactivityScores:            inactivityScores,
			CurrentSyncCommittee:        b.currentSyncCommitteeVal(),
			NextSyncCommittee:           b.nextSyncCommitteeVal(),
		}
	case version.Bellatrix:
		return &zondpb.BeaconStateBellatrix{
			GenesisTime:                  b.genesisTime,
			GenesisValidatorsRoot:        gvrCopy[:],
			Slot:                         b.slot,
			Fork:                         b.forkVal(),
			LatestBlockHeader:            b.latestBlockHeaderVal(),
			BlockRoots:                   br,
			StateRoots:                   sr,
			HistoricalRoots:              b.historicalRoots.Slice(),
			Eth1Data:                     b.eth1DataVal(),
			Eth1DataVotes:                b.eth1DataVotesVal(),
			Eth1DepositIndex:             b.eth1DepositIndex,
			Validators:                   b.validatorsVal(),
			Balances:                     b.balancesVal(),
			RandaoMixes:                  rm,
			Slashings:                    b.slashingsVal(),
			PreviousEpochParticipation:   b.previousEpochParticipationVal(),
			CurrentEpochParticipation:    b.currentEpochParticipationVal(),
			JustificationBits:            b.justificationBitsVal(),
			PreviousJustifiedCheckpoint:  b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:   b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:          b.finalizedCheckpointVal(),
			InactivityScores:             inactivityScores,
			CurrentSyncCommittee:         b.currentSyncCommitteeVal(),
			NextSyncCommittee:            b.nextSyncCommitteeVal(),
			LatestExecutionPayloadHeader: b.latestExecutionPayloadHeaderVal(),
		}
	case version.Capella:
		return &zondpb.BeaconStateCapella{
			GenesisTime:                  b.genesisTime,
			GenesisValidatorsRoot:        gvrCopy[:],
			Slot:                         b.slot,
			Fork:                         b.forkVal(),
			LatestBlockHeader:            b.latestBlockHeaderVal(),
			BlockRoots:                   br,
			StateRoots:                   sr,
			HistoricalRoots:              b.historicalRoots.Slice(),
			Eth1Data:                     b.eth1DataVal(),
			Eth1DataVotes:                b.eth1DataVotesVal(),
			Eth1DepositIndex:             b.eth1DepositIndex,
			Validators:                   b.validatorsVal(),
			Balances:                     b.balancesVal(),
			RandaoMixes:                  rm,
			Slashings:                    b.slashingsVal(),
			PreviousEpochParticipation:   b.previousEpochParticipationVal(),
			CurrentEpochParticipation:    b.currentEpochParticipationVal(),
			JustificationBits:            b.justificationBitsVal(),
			PreviousJustifiedCheckpoint:  b.previousJustifiedCheckpointVal(),
			CurrentJustifiedCheckpoint:   b.currentJustifiedCheckpointVal(),
			FinalizedCheckpoint:          b.finalizedCheckpointVal(),
			InactivityScores:             inactivityScores,
			CurrentSyncCommittee:         b.currentSyncCommitteeVal(),
			NextSyncCommittee:            b.nextSyncCommitteeVal(),
			LatestExecutionPayloadHeader: b.latestExecutionPayloadHeaderCapellaVal(),
			NextWithdrawalIndex:          b.nextWithdrawalIndex,
			NextWithdrawalValidatorIndex: b.nextWithdrawalValidatorIndex,
			HistoricalSummaries:          b.historicalSummariesVal(),
		}
	default:
		return nil
	}
}

// StateRoots kept track of in the beacon state.
func (b *BeaconState) StateRoots() [][]byte {
	b.lock.RLock()
	defer b.lock.RUnlock()

	roots := b.stateRootsVal()
	if roots == nil {
		return nil
	}
	return roots.Slice()
}

func (b *BeaconState) stateRootsVal() customtypes.StateRoots {
	if features.Get().EnableExperimentalState {
		if b.stateRootsMultiValue == nil {
			return nil
		}
		return b.stateRootsMultiValue.Value(b)
	}
	return b.stateRoots
}

// StateRootAtIndex retrieves a specific state root based on an
// input index value.
func (b *BeaconState) StateRootAtIndex(idx uint64) ([]byte, error) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	if features.Get().EnableExperimentalState {
		if b.stateRootsMultiValue == nil {
			return nil, nil
		}
		r, err := b.stateRootsMultiValue.At(b, idx)
		if err != nil {
			return nil, err
		}
		return r[:], nil
	}

	if b.stateRoots == nil {
		return nil, nil
	}
	r, err := b.stateRootAtIndex(idx)
	if err != nil {
		return nil, err
	}
	return r[:], nil
}

// stateRootAtIndex retrieves a specific state root based on an
// input index value.
// This assumes that a lock is already held on BeaconState.
//
// WARNING: This function does not work with the multi-value slice feature.
func (b *BeaconState) stateRootAtIndex(idx uint64) ([32]byte, error) {
	if uint64(len(b.stateRoots)) <= idx {
		return [32]byte{}, errors.Wrapf(consensus_types.ErrOutOfBounds, "state root index %d does not exist", idx)
	}
	return b.stateRoots[idx], nil
}

// ProtobufBeaconStatePhase0 transforms an input into beacon state in the form of protobuf.
// Error is returned if the input is not type protobuf beacon state.
func ProtobufBeaconStatePhase0(s interface{}) (*zondpb.BeaconState, error) {
	pbState, ok := s.(*zondpb.BeaconState)
	if !ok {
		return nil, errors.New("input is not type zondpb.BeaconState")
	}
	return pbState, nil
}

// ProtobufBeaconStateAltair transforms an input into beacon state Altair in the form of protobuf.
// Error is returned if the input is not type protobuf beacon state.
func ProtobufBeaconStateAltair(s interface{}) (*zondpb.BeaconStateAltair, error) {
	pbState, ok := s.(*zondpb.BeaconStateAltair)
	if !ok {
		return nil, errors.New("input is not type pb.BeaconStateAltair")
	}
	return pbState, nil
}

// ProtobufBeaconStateBellatrix transforms an input into beacon state Bellatrix in the form of protobuf.
// Error is returned if the input is not type protobuf beacon state.
func ProtobufBeaconStateBellatrix(s interface{}) (*zondpb.BeaconStateBellatrix, error) {
	pbState, ok := s.(*zondpb.BeaconStateBellatrix)
	if !ok {
		return nil, errors.New("input is not type pb.BeaconStateBellatrix")
	}
	return pbState, nil
}

// ProtobufBeaconStateCapella transforms an input into beacon state Capella in the form of protobuf.
// Error is returned if the input is not type protobuf beacon state.
func ProtobufBeaconStateCapella(s interface{}) (*zondpb.BeaconStateCapella, error) {
	pbState, ok := s.(*zondpb.BeaconStateCapella)
	if !ok {
		return nil, errors.New("input is not type pb.BeaconStateCapella")
	}
	return pbState, nil
}
