package events

import (
	"strings"

	gwpb "github.com/grpc-ecosystem/grpc-gateway/v2/proto/gateway"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	"github.com/theQRL/qrysm/beacon-chain/core/feed/operation"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	"github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	"github.com/theQRL/qrysm/config/params"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/proto/migration"
	qrlpbservice "github.com/theQRL/qrysm/proto/qrl/service"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/time/slots"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	// HeadTopic represents a new chain head event topic.
	HeadTopic = "head"
	// BlockTopic represents a new produced block event topic.
	BlockTopic = "block"
	// AttestationTopic represents a new submitted attestation event topic.
	AttestationTopic = "attestation"
	// VoluntaryExitTopic represents a new performed voluntary exit event topic.
	VoluntaryExitTopic = "voluntary_exit"
	// FinalizedCheckpointTopic represents a new finalized checkpoint event topic.
	FinalizedCheckpointTopic = "finalized_checkpoint"
	// ChainReorgTopic represents a chain reorganization event topic.
	ChainReorgTopic = "chain_reorg"
	// SyncCommitteeContributionTopic represents a new sync committee contribution event topic.
	SyncCommitteeContributionTopic = "contribution_and_proof"
	// PayloadAttributesTopic represents a new payload attributes for execution payload building event topic.
	PayloadAttributesTopic = "payload_attributes"
)

var casesHandled = map[string]bool{
	HeadTopic:                      true,
	BlockTopic:                     true,
	AttestationTopic:               true,
	VoluntaryExitTopic:             true,
	FinalizedCheckpointTopic:       true,
	ChainReorgTopic:                true,
	SyncCommitteeContributionTopic: true,
	PayloadAttributesTopic:         true,
}

// StreamEvents allows requesting all events from a set of topics defined in the QRL consensus API standard.
// The topics supported include block events, attestations, chain reorgs, voluntary exits,
// chain finality, and more.
func (s *Server) StreamEvents(
	req *qrlpb.StreamEventsRequest, stream qrlpbservice.Events_StreamEventsServer,
) error {
	if req == nil || len(req.Topics) == 0 {
		return status.Error(codes.InvalidArgument, "No topics specified to subscribe to")
	}
	// Check if the topics in the request are valid.
	requestedTopics := make(map[string]bool)
	for _, rawTopic := range req.Topics {
		splitTopic := strings.SplitSeq(rawTopic, ",")
		for topic := range splitTopic {
			if _, ok := casesHandled[topic]; !ok {
				return status.Errorf(codes.InvalidArgument, "Topic %s not allowed for event subscriptions", topic)
			}
			requestedTopics[topic] = true
		}
	}

	// Subscribe to event feeds from information received in the beacon node runtime.
	opsChan := make(chan *feed.Event, 1)
	opsSub := s.OperationNotifier.OperationFeed().Subscribe(opsChan)

	stateChan := make(chan *feed.Event, 1)
	stateSub := s.StateNotifier.StateFeed().Subscribe(stateChan)

	defer opsSub.Unsubscribe()
	defer stateSub.Unsubscribe()

	// Handle each event received and context cancelation.
	for {
		select {
		case event := <-opsChan:
			if err := handleBlockOperationEvents(stream, requestedTopics, event); err != nil {
				return status.Errorf(codes.Internal, "Could not handle block operations event: %v", err)
			}
		case event := <-stateChan:
			if err := s.handleStateEvents(stream, requestedTopics, event); err != nil {
				return status.Errorf(codes.Internal, "Could not handle state event: %v", err)
			}
		case <-s.Ctx.Done():
			return status.Errorf(codes.Canceled, "Context canceled")
		case <-stream.Context().Done():
			return status.Errorf(codes.Canceled, "Context canceled")
		}
	}
}

func handleBlockOperationEvents(
	stream qrlpbservice.Events_StreamEventsServer, requestedTopics map[string]bool, event *feed.Event,
) error {
	switch event.Type {
	case operation.AggregatedAttReceived:
		if _, ok := requestedTopics[AttestationTopic]; !ok {
			return nil
		}
		attData, ok := event.Data.(*operation.AggregatedAttReceivedData)
		if !ok {
			return nil
		}
		v1Data := migration.V1Alpha1AggregateAttAndProofToV1(attData.Attestation)
		return streamData(stream, AttestationTopic, v1Data)
	case operation.UnaggregatedAttReceived:
		if _, ok := requestedTopics[AttestationTopic]; !ok {
			return nil
		}
		attData, ok := event.Data.(*operation.UnAggregatedAttReceivedData)
		if !ok {
			return nil
		}
		v1Data := migration.V1Alpha1AttestationToV1(attData.Attestation)
		return streamData(stream, AttestationTopic, v1Data)
	case operation.ExitReceived:
		if _, ok := requestedTopics[VoluntaryExitTopic]; !ok {
			return nil
		}
		exitData, ok := event.Data.(*operation.ExitReceivedData)
		if !ok {
			return nil
		}
		v1Data := migration.V1Alpha1ExitToV1(exitData.Exit)
		return streamData(stream, VoluntaryExitTopic, v1Data)
	case operation.SyncCommitteeContributionReceived:
		if _, ok := requestedTopics[SyncCommitteeContributionTopic]; !ok {
			return nil
		}
		contributionData, ok := event.Data.(*operation.SyncCommitteeContributionReceivedData)
		if !ok {
			return nil
		}
		v2Data := migration.V1Alpha1SignedContributionAndProofToV1(contributionData.Contribution)
		return streamData(stream, SyncCommitteeContributionTopic, v2Data)
	default:
		return nil
	}
}

func (s *Server) handleStateEvents(
	stream qrlpbservice.Events_StreamEventsServer, requestedTopics map[string]bool, event *feed.Event,
) error {
	switch event.Type {
	case statefeed.NewHead:
		if _, ok := requestedTopics[HeadTopic]; ok {
			head, ok := event.Data.(*qrlpb.EventHead)
			if !ok {
				return nil
			}
			return streamData(stream, HeadTopic, head)
		}
		if _, ok := requestedTopics[PayloadAttributesTopic]; ok {
			if err := s.streamPayloadAttributes(stream); err != nil {
				log.WithError(err).Error("Unable to obtain stream payload attributes")
			}
			return nil
		}
		return nil
	case statefeed.MissedSlot:
		if _, ok := requestedTopics[PayloadAttributesTopic]; ok {
			if err := s.streamPayloadAttributes(stream); err != nil {
				log.WithError(err).Error("Unable to obtain stream payload attributes")
			}
			return nil
		}
		return nil
	case statefeed.FinalizedCheckpoint:
		if _, ok := requestedTopics[FinalizedCheckpointTopic]; !ok {
			return nil
		}
		finalizedCheckpoint, ok := event.Data.(*qrlpb.EventFinalizedCheckpoint)
		if !ok {
			return nil
		}
		return streamData(stream, FinalizedCheckpointTopic, finalizedCheckpoint)
	case statefeed.Reorg:
		if _, ok := requestedTopics[ChainReorgTopic]; !ok {
			return nil
		}
		reorg, ok := event.Data.(*qrlpb.EventChainReorg)
		if !ok {
			return nil
		}
		return streamData(stream, ChainReorgTopic, reorg)
	case statefeed.BlockProcessed:
		if _, ok := requestedTopics[BlockTopic]; !ok {
			return nil
		}
		blkData, ok := event.Data.(*statefeed.BlockProcessedData)
		if !ok {
			return nil
		}
		v1Data, err := migration.BlockIfaceToV1BlockHeader(blkData.SignedBlock)
		if err != nil {
			return err
		}
		item, err := v1Data.Message.HashTreeRoot()
		if err != nil {
			return errors.Wrap(err, "could not hash tree root block")
		}
		eventBlock := &qrlpb.EventBlock{
			Slot:                blkData.Slot,
			Block:               item[:],
			ExecutionOptimistic: blkData.Optimistic,
		}
		return streamData(stream, BlockTopic, eventBlock)
	default:
		return nil
	}
}

// streamPayloadAttributes on new head event.
// This event stream is intended to be used by builders and relays.
// parent_ fields are based on state at N_{current_slot}, while the rest of fields are based on state of N_{current_slot + 1}
func (s *Server) streamPayloadAttributes(stream qrlpbservice.Events_StreamEventsServer) error {
	headRoot, err := s.HeadFetcher.HeadRoot(s.Ctx)
	if err != nil {
		return errors.Wrap(err, "could not get head root")
	}
	st, err := s.HeadFetcher.HeadState(s.Ctx)
	if err != nil {
		return errors.Wrap(err, "could not get head state")
	}
	// advance the headstate
	headState, err := transition.ProcessSlotsIfPossible(s.Ctx, st, s.ChainInfoFetcher.CurrentSlot()+1)
	if err != nil {
		return err
	}

	headBlock, err := s.HeadFetcher.HeadBlock(s.Ctx)
	if err != nil {
		return err
	}

	headPayload, err := headBlock.Block().Body().Execution()
	if err != nil {
		return err
	}

	t, err := slots.ToTime(uint64(headState.GenesisTime()), headState.Slot())
	if err != nil {
		return err
	}

	prevRando, err := helpers.RandaoMix(headState, time.CurrentEpoch(headState))
	if err != nil {
		return err
	}

	proposerIndex, err := helpers.BeaconProposerIndex(s.Ctx, headState)
	if err != nil {
		return err
	}

	// The fee recipient advertised by the payload_attributes event must reflect the
	// proposer's own choice (their registered fee recipient), not the head block's
	// payload fee recipient (which was set by whoever proposed the previous block).
	// Fall back to the network default when the proposer hasn't registered with us.
	feeRecipient := params.BeaconConfig().DefaultFeeRecipient.Bytes()
	if s.BlockBuilder != nil && s.BlockBuilder.Configured() {
		if reg, err := s.BlockBuilder.RegistrationByValidatorID(s.Ctx, proposerIndex); err == nil && reg != nil && len(reg.FeeRecipient) > 0 {
			feeRecipient = reg.FeeRecipient
		}
	}

	switch headState.Version() {
	case version.Zond:
		withdrawals, err := headState.ExpectedWithdrawals()
		if err != nil {
			return err
		}
		return streamData(stream, PayloadAttributesTopic, &qrlpb.EventPayloadAttributeV2{
			Version: version.String(headState.Version()),
			Data: &qrlpb.EventPayloadAttributeV2_BasePayloadAttribute{
				ProposerIndex:     proposerIndex,
				ProposalSlot:      headState.Slot(),
				ParentBlockNumber: headPayload.BlockNumber(),
				ParentBlockRoot:   headRoot,
				ParentBlockHash:   headPayload.BlockHash(),
				PayloadAttributes: &enginev1.PayloadAttributesV2{
					Timestamp:             uint64(t.Unix()),
					PrevRandao:            prevRando,
					SuggestedFeeRecipient: feeRecipient,
					Withdrawals:           withdrawals,
				},
			},
		})
	default:
		return errors.New("payload version is not supported")
	}
}

func streamData(stream qrlpbservice.Events_StreamEventsServer, name string, data proto.Message) error {
	returnData, err := anypb.New(data)
	if err != nil {
		return err
	}
	return stream.Send(&gwpb.EventSource{
		Event: name,
		Data:  returnData,
	})
}
