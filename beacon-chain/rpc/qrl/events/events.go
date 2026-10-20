package events

import (
	"context"
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

// DefaultEventFeedDepth is the default buffer depth of each event subscription
// channel and of the per-client outbox (see Server.EventFeedDepth).
const DefaultEventFeedDepth = 1000

// errSlowReader is returned when a client does not read the event stream fast
// enough for the outbox to stay below its capacity.
var errSlowReader = errors.New("client failed to read fast enough to keep outgoing buffer below threshold")

// Topics served from the operation feed.
var opsFeedTopics = map[string]bool{
	AttestationTopic:               true,
	VoluntaryExitTopic:             true,
	SyncCommitteeContributionTopic: true,
}

// Topics served from the state feed.
var stateFeedTopics = map[string]bool{
	HeadTopic:                true,
	BlockTopic:               true,
	FinalizedCheckpointTopic: true,
	ChainReorgTopic:          true,
	PayloadAttributesTopic:   true,
}

type topicRequest struct {
	topics        map[string]bool
	needOpsFeed   bool
	needStateFeed bool
}

func (r *topicRequest) requested(topic string) bool {
	return r.topics[topic]
}

func newTopicRequest(rawTopics []string) (*topicRequest, error) {
	req := &topicRequest{topics: make(map[string]bool)}
	for _, rawTopic := range rawTopics {
		for topic := range strings.SplitSeq(rawTopic, ",") {
			switch {
			case opsFeedTopics[topic]:
				req.needOpsFeed = true
			case stateFeedTopics[topic]:
				req.needStateFeed = true
			default:
				return nil, status.Errorf(codes.InvalidArgument, "Topic %s not allowed for event subscriptions", topic)
			}
			req.topics[topic] = true
		}
	}
	if len(req.topics) == 0 {
		return nil, status.Error(codes.InvalidArgument, "No topics specified to subscribe to")
	}
	return req, nil
}

// StreamEvents allows requesting all events from a set of topics defined in the QRL consensus API standard.
// The topics supported include block events, attestations, chain reorgs, voluntary exits,
// chain finality, and more.
//
// Events are delivered through two decoupled stages so that a stalled client can
// never back-pressure the producers (the event feeds are written to synchronously
// from gossip validation and block import, and event.Feed.Send blocks until every
// subscriber has accepted the value):
//
//  1. The feed subscription channels are buffered (Server.EventFeedDepth) and are
//     drained by this handler, which only filters and converts events before
//     handing them to a bounded outbox with a non-blocking write.
//  2. A separate goroutine drains the outbox and performs the potentially blocking
//     stream.Send calls.
//
// If the outbox fills up because the client is not reading, the event is dropped,
// the feeds are unsubscribed and the stream is terminated (upstream #13329/#14413).
func (s *Server) StreamEvents(
	req *qrlpb.StreamEventsRequest, stream qrlpbservice.Events_StreamEventsServer,
) error {
	if req == nil || len(req.Topics) == 0 {
		return status.Error(codes.InvalidArgument, "No topics specified to subscribe to")
	}
	topics, err := newTopicRequest(req.Topics)
	if err != nil {
		return err
	}
	depth := s.EventFeedDepth
	if depth <= 0 {
		depth = DefaultEventFeedDepth
	}

	// The context is cancelled on any exit path so that the sender goroutine
	// stops as soon as this handler returns.
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// Subscribe only to the feeds that serve the requested topics, using
	// buffered channels so that feed producers are not blocked by this reader.
	// The two feeds use overlapping feed.EventType values, so they must be
	// received on separate channels to be told apart.
	opsChan := make(chan *feed.Event, depth)
	if topics.needOpsFeed {
		opsSub := s.OperationNotifier.OperationFeed().Subscribe(opsChan)
		defer opsSub.Unsubscribe()
	}
	stateChan := make(chan *feed.Event, depth)
	if topics.needStateFeed {
		stateSub := s.StateNotifier.StateFeed().Subscribe(stateChan)
		defer stateSub.Unsubscribe()
	}

	// The outbox decouples the feeds from the client: stream.Send may block on
	// transport flow control when the client stops reading, so it runs in its
	// own goroutine. The goroutine is not waited for on return: a Send blocked on
	// flow control is only released by gRPC once this handler returns and the
	// stream is closed, at which point the goroutine observes the error/ctx and
	// exits.
	outbox := make(chan *gwpb.EventSource, depth)
	sendErr := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-outbox:
				if err := stream.Send(msg); err != nil {
					sendErr <- err
					cancel()
					return
				}
			}
		}
	}()

	// enqueue hands a converted event to the sender goroutine without blocking.
	enqueue := func(msg *gwpb.EventSource) error {
		if msg == nil {
			return nil
		}
		select {
		case outbox <- msg:
			return nil
		case <-ctx.Done():
			return status.Error(codes.Canceled, "Context canceled")
		default:
			// The outbox is full: the client is not keeping up. Drop the event
			// and shut the stream down; the deferred Unsubscribe calls stop the
			// feeds from writing to our channels, and the channels' own buffers
			// give this handler time to unwind without stalling producers.
			log.WithError(errSlowReader).Warn("Client is unable to keep up with event stream, shutting down")
			return status.Error(codes.ResourceExhausted, errSlowReader.Error())
		}
	}

	for {
		select {
		case event := <-opsChan:
			msg, err := blockOperationEventMessage(topics, event)
			if err != nil {
				return status.Errorf(codes.Internal, "Could not handle block operations event: %v", err)
			}
			if err := enqueue(msg); err != nil {
				return err
			}
		case event := <-stateChan:
			msgs, err := s.stateEventMessages(topics, event)
			if err != nil {
				return status.Errorf(codes.Internal, "Could not handle state event: %v", err)
			}
			for _, msg := range msgs {
				if err := enqueue(msg); err != nil {
					return err
				}
			}
		case err := <-sendErr:
			return status.Errorf(codes.Internal, "Could not send event: %v", err)
		case <-s.Ctx.Done():
			return status.Error(codes.Canceled, "Context canceled")
		case <-ctx.Done():
			// The sender cancels ctx right after reporting a failed Send; surface
			// that error rather than a generic cancellation.
			select {
			case err := <-sendErr:
				return status.Errorf(codes.Internal, "Could not send event: %v", err)
			default:
				return status.Error(codes.Canceled, "Context canceled")
			}
		}
	}
}

// blockOperationEventMessage converts an operation feed event into the message
// to stream to the client. It returns (nil, nil) for events whose topic was not
// requested or whose payload is not of the expected type.
func blockOperationEventMessage(topics *topicRequest, event *feed.Event) (*gwpb.EventSource, error) {
	switch event.Type {
	case operation.AggregatedAttReceived:
		if !topics.requested(AttestationTopic) {
			return nil, nil
		}
		attData, ok := event.Data.(*operation.AggregatedAttReceivedData)
		if !ok {
			return nil, nil
		}
		return newEventMessage(AttestationTopic, migration.V1Alpha1AggregateAttAndProofToV1(attData.Attestation))
	case operation.UnaggregatedAttReceived:
		if !topics.requested(AttestationTopic) {
			return nil, nil
		}
		attData, ok := event.Data.(*operation.UnAggregatedAttReceivedData)
		if !ok {
			return nil, nil
		}
		return newEventMessage(AttestationTopic, migration.V1Alpha1AttestationToV1(attData.Attestation))
	case operation.ExitReceived:
		if !topics.requested(VoluntaryExitTopic) {
			return nil, nil
		}
		exitData, ok := event.Data.(*operation.ExitReceivedData)
		if !ok {
			return nil, nil
		}
		return newEventMessage(VoluntaryExitTopic, migration.V1Alpha1ExitToV1(exitData.Exit))
	case operation.SyncCommitteeContributionReceived:
		if !topics.requested(SyncCommitteeContributionTopic) {
			return nil, nil
		}
		contributionData, ok := event.Data.(*operation.SyncCommitteeContributionReceivedData)
		if !ok {
			return nil, nil
		}
		return newEventMessage(SyncCommitteeContributionTopic, migration.V1Alpha1SignedContributionAndProofToV1(contributionData.Contribution))
	default:
		return nil, nil
	}
}

// stateEventMessages converts a state feed event into the messages to stream to
// the client, in the order they must be sent. It returns (nil, nil) for events
// whose topic was not requested or whose payload is not of the expected type.
func (s *Server) stateEventMessages(topics *topicRequest, event *feed.Event) ([]*gwpb.EventSource, error) {
	switch event.Type {
	case statefeed.NewHead:
		// A new head serves two topics: the head itself and the payload
		// attributes for the next proposal. Both must be emitted when both
		// are subscribed - returning after the head message used to leave
		// relays and builders subscribed to both topics without payload
		// attributes for every slot that had a block.
		var msgs []*gwpb.EventSource
		if topics.requested(HeadTopic) {
			if head, ok := event.Data.(*qrlpb.EventHead); ok {
				msg, err := newEventMessage(HeadTopic, head)
				if err != nil {
					return nil, err
				}
				msgs = append(msgs, msg)
			}
		}
		if topics.requested(PayloadAttributesTopic) {
			msg, err := s.payloadAttributesMessage()
			if err != nil {
				return nil, err
			}
			if msg != nil {
				msgs = append(msgs, msg)
			}
		}
		return msgs, nil
	case statefeed.MissedSlot:
		if !topics.requested(PayloadAttributesTopic) {
			return nil, nil
		}
		return singleEventMessage(s.payloadAttributesMessage())
	case statefeed.FinalizedCheckpoint:
		if !topics.requested(FinalizedCheckpointTopic) {
			return nil, nil
		}
		finalizedCheckpoint, ok := event.Data.(*qrlpb.EventFinalizedCheckpoint)
		if !ok {
			return nil, nil
		}
		return singleEventMessage(newEventMessage(FinalizedCheckpointTopic, finalizedCheckpoint))
	case statefeed.Reorg:
		if !topics.requested(ChainReorgTopic) {
			return nil, nil
		}
		reorg, ok := event.Data.(*qrlpb.EventChainReorg)
		if !ok {
			return nil, nil
		}
		return singleEventMessage(newEventMessage(ChainReorgTopic, reorg))
	case statefeed.BlockProcessed:
		if !topics.requested(BlockTopic) {
			return nil, nil
		}
		blkData, ok := event.Data.(*statefeed.BlockProcessedData)
		if !ok {
			return nil, nil
		}
		v1Data, err := migration.BlockIfaceToV1BlockHeader(blkData.SignedBlock)
		if err != nil {
			return nil, err
		}
		item, err := v1Data.Message.HashTreeRoot()
		if err != nil {
			return nil, errors.Wrap(err, "could not hash tree root block")
		}
		return singleEventMessage(newEventMessage(BlockTopic, &qrlpb.EventBlock{
			Slot:                blkData.Slot,
			Block:               item[:],
			ExecutionOptimistic: blkData.Optimistic,
		}))
	default:
		return nil, nil
	}
}

// singleEventMessage wraps a single converted message (or its error) into the
// slice form used by stateEventMessages.
func singleEventMessage(msg *gwpb.EventSource, err error) ([]*gwpb.EventSource, error) {
	if err != nil || msg == nil {
		return nil, err
	}
	return []*gwpb.EventSource{msg}, nil
}

// payloadAttributesMessage builds the payload_attributes event on a new head or
// missed slot. This event stream is intended to be used by builders and relays.
// parent_ fields are based on state at N_{current_slot}, while the rest of
// fields are based on state of N_{current_slot + 1}.
//
// Failures to build the event are logged and swallowed (as before) so that a
// transient head-state problem does not tear down the stream.
func (s *Server) payloadAttributesMessage() (*gwpb.EventSource, error) {
	msg, err := s.buildPayloadAttributesMessage()
	if err != nil {
		log.WithError(err).Error("Unable to obtain stream payload attributes")
		return nil, nil
	}
	return msg, nil
}

func (s *Server) buildPayloadAttributesMessage() (*gwpb.EventSource, error) {
	// Keep the parent root, execution header, and proposal state on one head.
	// Separate reads can observe different branches during a concurrent import.
	st, headRoot, err := s.HeadFetcher.HeadStateAndRoot(s.Ctx)
	if err != nil {
		return nil, errors.Wrap(err, "could not get head snapshot")
	}
	// advance the headstate
	headState, err := transition.ProcessSlotsIfPossible(s.Ctx, st, s.ChainInfoFetcher.CurrentSlot()+1)
	if err != nil {
		return nil, err
	}

	headPayload, err := headState.LatestExecutionPayloadHeader()
	if err != nil {
		return nil, err
	}

	t, err := slots.ToTime(uint64(headState.GenesisTime()), headState.Slot())
	if err != nil {
		return nil, err
	}

	prevRando, err := helpers.RandaoMix(headState, time.CurrentEpoch(headState))
	if err != nil {
		return nil, err
	}

	proposerIndex, err := helpers.BeaconProposerIndex(s.Ctx, headState)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		return newEventMessage(PayloadAttributesTopic, &qrlpb.EventPayloadAttributeV2{
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
		return nil, errors.New("payload version is not supported")
	}
}

func newEventMessage(name string, data proto.Message) (*gwpb.EventSource, error) {
	returnData, err := anypb.New(data)
	if err != nil {
		return nil, err
	}
	return &gwpb.EventSource{
		Event: name,
		Data:  returnData,
	}, nil
}
