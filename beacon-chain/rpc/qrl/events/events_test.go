package events

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/grpc-ecosystem/grpc-gateway/v2/proto/gateway"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/qrysm/async/event"
	mockChain "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	mockBuilder "github.com/theQRL/qrysm/beacon-chain/builder/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	"github.com/theQRL/qrysm/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/beacon-chain/core/feed"
	"github.com/theQRL/qrysm/beacon-chain/core/feed/operation"
	statefeed "github.com/theQRL/qrysm/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/beacon-chain/core/helpers"
	qrysmtime "github.com/theQRL/qrysm/beacon-chain/core/time"
	"github.com/theQRL/qrysm/beacon-chain/state"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	consensusBlocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/proto/migration"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/mock"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestStreamEvents_Preconditions(t *testing.T) {
	t.Run("no_topics_specified", func(t *testing.T) {
		srv := &Server{}
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockStream := mock.NewMockEvents_StreamEventsServer(ctrl)
		err := srv.StreamEvents(&qrlpb.StreamEventsRequest{Topics: nil}, mockStream)
		require.ErrorContains(t, "No topics specified", err)
	})
	t.Run("topic_not_allowed", func(t *testing.T) {
		srv := &Server{}
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockStream := mock.NewMockEvents_StreamEventsServer(ctrl)
		err := srv.StreamEvents(&qrlpb.StreamEventsRequest{Topics: []string{"foobar"}}, mockStream)
		require.ErrorContains(t, "Topic foobar not allowed", err)
	})
}

func TestStreamEvents_OperationsEvents(t *testing.T) {
	t.Run("attestation_unaggregated", func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		wantedAttV1alpha1 := util.HydrateAttestation(&qrysmpb.Attestation{
			Data: &qrysmpb.AttestationData{
				Slot: 8,
			},
		})
		wantedAtt := migration.V1Alpha1AttestationToV1(wantedAttV1alpha1)
		genericResponse, err := anypb.New(wantedAtt)
		require.NoError(t, err)

		wantedMessage := &gateway.EventSource{
			Event: AttestationTopic,
			Data:  genericResponse,
		}

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{AttestationTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: operation.UnaggregatedAttReceived,
				Data: &operation.UnAggregatedAttReceivedData{
					Attestation: wantedAttV1alpha1,
				},
			},
			feed: srv.OperationNotifier.OperationFeed(),
		})
	})
	t.Run("attestation_aggregated", func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		wantedAttV1alpha1 := &qrysmpb.AggregateAttestationAndProof{
			Aggregate: util.HydrateAttestation(&qrysmpb.Attestation{}),
		}
		wantedAtt := migration.V1Alpha1AggregateAttAndProofToV1(wantedAttV1alpha1)
		genericResponse, err := anypb.New(wantedAtt)
		require.NoError(t, err)

		wantedMessage := &gateway.EventSource{
			Event: AttestationTopic,
			Data:  genericResponse,
		}

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{AttestationTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: operation.AggregatedAttReceived,
				Data: &operation.AggregatedAttReceivedData{
					Attestation: wantedAttV1alpha1,
				},
			},
			feed: srv.OperationNotifier.OperationFeed(),
		})
	})
	t.Run(VoluntaryExitTopic, func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		wantedExitV1alpha1 := &qrysmpb.SignedVoluntaryExit{
			Exit: &qrysmpb.VoluntaryExit{
				Epoch:          1,
				ValidatorIndex: 1,
			},
			Signature: make([]byte, 96),
		}
		wantedExit := migration.V1Alpha1ExitToV1(wantedExitV1alpha1)
		genericResponse, err := anypb.New(wantedExit)
		require.NoError(t, err)

		wantedMessage := &gateway.EventSource{
			Event: VoluntaryExitTopic,
			Data:  genericResponse,
		}

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{VoluntaryExitTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: operation.ExitReceived,
				Data: &operation.ExitReceivedData{
					Exit: wantedExitV1alpha1,
				},
			},
			feed: srv.OperationNotifier.OperationFeed(),
		})
	})
	t.Run(SyncCommitteeContributionTopic, func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		wantedContributionV1alpha1 := &qrysmpb.SignedContributionAndProof{
			Message: &qrysmpb.ContributionAndProof{
				AggregatorIndex: 1,
				Contribution: &qrysmpb.SyncCommitteeContribution{
					Slot:              1,
					BlockRoot:         []byte("root"),
					SubcommitteeIndex: 1,
					AggregationBits:   bitfield.NewBitvector128(),
					Signatures:        [][]byte{[]byte("sig")},
				},
				SelectionProof: []byte("proof"),
			},
			Signature: []byte("sig"),
		}
		wantedContribution := migration.V1Alpha1SignedContributionAndProofToV1(wantedContributionV1alpha1)
		genericResponse, err := anypb.New(wantedContribution)
		require.NoError(t, err)

		wantedMessage := &gateway.EventSource{
			Event: SyncCommitteeContributionTopic,
			Data:  genericResponse,
		}

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{SyncCommitteeContributionTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: operation.SyncCommitteeContributionReceived,
				Data: &operation.SyncCommitteeContributionReceivedData{
					Contribution: wantedContributionV1alpha1,
				},
			},
			feed: srv.OperationNotifier.OperationFeed(),
		})
	})
}

func TestStreamEvents_StateEvents(t *testing.T) {
	t.Run(HeadTopic, func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		wantedHead := &qrlpb.EventHead{
			Slot:                      8,
			Block:                     make([]byte, 32),
			State:                     make([]byte, 32),
			EpochTransition:           true,
			PreviousDutyDependentRoot: make([]byte, 32),
			CurrentDutyDependentRoot:  make([]byte, 32),
			ExecutionOptimistic:       true,
		}
		genericResponse, err := anypb.New(wantedHead)
		require.NoError(t, err)
		wantedMessage := &gateway.EventSource{
			Event: HeadTopic,
			Data:  genericResponse,
		}

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{HeadTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: statefeed.NewHead,
				Data: wantedHead,
			},
			feed: srv.StateNotifier.StateFeed(),
		})
	})

	t.Run(PayloadAttributesTopic+"_zond", func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream, wantedPayload := payloadAttributesFixture(ctx, t)
		defer ctrl.Finish()
		genericResponse, err := anypb.New(wantedPayload)
		require.NoError(t, err)
		wantedMessage := &gateway.EventSource{
			Event: PayloadAttributesTopic,
			Data:  genericResponse,
		}

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{PayloadAttributesTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: statefeed.NewHead,
				Data: wantedPayload,
			},
			feed: srv.StateNotifier.StateFeed(),
		})
	})

	t.Run(PayloadAttributesTopic+"_uses_registered_fee_recipient", func(t *testing.T) {
		ctx := context.Background()
		beaconState, _ := util.DeterministicGenesisStateZond(t, 1)
		require.NoError(t, beaconState.SetSlot(2))
		require.NoError(t, beaconState.SetNextWithdrawalValidatorIndex(0))
		require.NoError(t, beaconState.SetBalances([]uint64{41000000000000}))
		stateRoot, err := beaconState.HashTreeRoot(ctx)
		require.NoError(t, err)

		genesis := blocks.NewGenesisBlock(stateRoot[:])
		parentRoot, err := genesis.Block.HashTreeRoot()
		require.NoError(t, err)

		withdrawals, err := beaconState.ExpectedWithdrawals()
		require.NoError(t, err)

		// The block's payload fee recipient is intentionally a non-zero "previous proposer" value.
		// streamPayloadAttributes must NOT use this — it should look up the *upcoming* proposer's
		// registration and use that fee recipient instead.
		previousProposerFeeRecipient := bytes.Repeat([]byte{0xee}, fieldparams.FeeRecipientLength)
		registeredFeeRecipient := bytes.Repeat([]byte{0xab}, fieldparams.FeeRecipientLength)

		var scBits [fieldparams.SyncAggregateSyncCommitteeBytesLength]byte
		blk := &qrysmpb.SignedBeaconBlockZond{
			Block: &qrysmpb.BeaconBlockZond{
				ProposerIndex: 0,
				Slot:          1,
				ParentRoot:    parentRoot[:],
				StateRoot:     genesis.Block.StateRoot,
				Body: &qrysmpb.BeaconBlockBodyZond{
					RandaoReveal:  genesis.Block.Body.RandaoReveal,
					Graffiti:      genesis.Block.Body.Graffiti,
					ExecutionData: genesis.Block.Body.ExecutionData,
					SyncAggregate: &qrysmpb.SyncAggregate{SyncCommitteeBits: scBits[:], SyncCommitteeSignatures: [][]byte{}},
					ExecutionPayload: &enginev1.ExecutionPayloadZond{
						BlockNumber:   1,
						ParentHash:    make([]byte, fieldparams.RootLength),
						FeeRecipient:  previousProposerFeeRecipient,
						StateRoot:     make([]byte, fieldparams.RootLength),
						ReceiptsRoot:  make([]byte, fieldparams.RootLength),
						LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
						PrevRandao:    make([]byte, fieldparams.RootLength),
						BaseFeePerGas: make([]byte, fieldparams.RootLength),
						BlockHash:     make([]byte, fieldparams.RootLength),
						Withdrawals:   withdrawals,
					},
				},
			},
			Signature: genesis.Signature,
		}
		signedBlk, err := consensusBlocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		payload, err := signedBlk.Block().Body().Execution()
		require.NoError(t, err)
		require.NoError(t, beaconState.SetLatestExecutionPayloadHeader(payload))

		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		fetcher := &mockChain.ChainService{
			Genesis:        time.Now(),
			State:          beaconState,
			Block:          signedBlk,
			Root:           make([]byte, 32),
			ValidatorsRoot: [32]byte{},
		}
		srv.HeadFetcher = fetcher
		srv.ChainInfoFetcher = fetcher

		// Wire a configured BlockBuilder with proposer 0 registered to a non-zero fee recipient.
		regCache := cache.NewRegistrationCache()
		regCache.UpdateIndexToRegisteredMap(ctx, map[primitives.ValidatorIndex]*qrysmpb.ValidatorRegistrationV1{
			0: {FeeRecipient: registeredFeeRecipient},
		})
		srv.BlockBuilder = &mockBuilder.MockBuilderService{
			HasConfigured:     true,
			RegistrationCache: regCache,
		}

		prevRando, err := helpers.RandaoMix(beaconState, qrysmtime.CurrentEpoch(beaconState))
		require.NoError(t, err)

		wantedPayload := &qrlpb.EventPayloadAttributeV2{
			Version: version.String(version.Zond),
			Data: &qrlpb.EventPayloadAttributeV2_BasePayloadAttribute{
				ProposerIndex:     0,
				ProposalSlot:      2,
				ParentBlockNumber: 1,
				ParentBlockRoot:   make([]byte, 32),
				ParentBlockHash:   make([]byte, 32),
				PayloadAttributes: &enginev1.PayloadAttributesV2{
					Timestamp:             120,
					PrevRandao:            prevRando,
					SuggestedFeeRecipient: registeredFeeRecipient,
					Withdrawals:           withdrawals,
				},
			},
		}
		genericResponse, err := anypb.New(wantedPayload)
		require.NoError(t, err)

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{PayloadAttributesTopic},
			stream:        mockStream,
			shouldReceive: &gateway.EventSource{Event: PayloadAttributesTopic, Data: genericResponse},
			itemToSend: &feed.Event{
				Type: statefeed.NewHead,
				Data: wantedPayload,
			},
			feed: srv.StateNotifier.StateFeed(),
		})
	})

	t.Run(FinalizedCheckpointTopic, func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		wantedCheckpoint := &qrlpb.EventFinalizedCheckpoint{
			Block:               make([]byte, 32),
			State:               make([]byte, 32),
			Epoch:               8,
			ExecutionOptimistic: true,
		}
		genericResponse, err := anypb.New(wantedCheckpoint)
		require.NoError(t, err)
		wantedMessage := &gateway.EventSource{
			Event: FinalizedCheckpointTopic,
			Data:  genericResponse,
		}

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{FinalizedCheckpointTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: statefeed.FinalizedCheckpoint,
				Data: wantedCheckpoint,
			},
			feed: srv.StateNotifier.StateFeed(),
		})
	})
	t.Run(ChainReorgTopic, func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		wantedReorg := &qrlpb.EventChainReorg{
			Slot:                8,
			Depth:               1,
			OldHeadBlock:        make([]byte, 32),
			NewHeadBlock:        make([]byte, 32),
			OldHeadState:        make([]byte, 32),
			NewHeadState:        make([]byte, 32),
			Epoch:               0,
			ExecutionOptimistic: true,
		}
		genericResponse, err := anypb.New(wantedReorg)
		require.NoError(t, err)
		wantedMessage := &gateway.EventSource{
			Event: ChainReorgTopic,
			Data:  genericResponse,
		}

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{ChainReorgTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: statefeed.Reorg,
				Data: wantedReorg,
			},
			feed: srv.StateNotifier.StateFeed(),
		})
	})
	t.Run(BlockTopic, func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		blk := util.HydrateSignedBeaconBlockZond(&qrysmpb.SignedBeaconBlockZond{
			Block: &qrysmpb.BeaconBlockZond{
				Slot: 8,
			},
		})
		bodyRoot, err := blk.Block.Body.HashTreeRoot()
		require.NoError(t, err)
		wantedHeader := util.HydrateBeaconHeader(&qrysmpb.BeaconBlockHeader{
			Slot:     8,
			BodyRoot: bodyRoot[:],
		})
		wantedBlockRoot, err := wantedHeader.HashTreeRoot()
		require.NoError(t, err)
		genericResponse, err := anypb.New(&qrlpb.EventBlock{
			Slot:                8,
			Block:               wantedBlockRoot[:],
			ExecutionOptimistic: true,
		})
		require.NoError(t, err)
		wantedMessage := &gateway.EventSource{
			Event: BlockTopic,
			Data:  genericResponse,
		}
		wsb, err := consensusBlocks.NewSignedBeaconBlock(blk)
		require.NoError(t, err)
		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{BlockTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: statefeed.BlockProcessed,
				Data: &statefeed.BlockProcessedData{
					Slot:        8,
					SignedBlock: wsb,
					Optimistic:  true,
				},
			},
			feed: srv.StateNotifier.StateFeed(),
		})
	})
}

func TestStreamEvents_CommaSeparatedTopics(t *testing.T) {
	ctx := context.Background()
	srv, ctrl, mockStream := setupServer(ctx, t)
	defer ctrl.Finish()

	wantedHead := &qrlpb.EventHead{
		Slot:                      8,
		Block:                     make([]byte, 32),
		State:                     make([]byte, 32),
		EpochTransition:           true,
		PreviousDutyDependentRoot: make([]byte, 32),
		CurrentDutyDependentRoot:  make([]byte, 32),
	}
	headGenericResponse, err := anypb.New(wantedHead)
	require.NoError(t, err)
	wantedHeadMessage := &gateway.EventSource{
		Event: HeadTopic,
		Data:  headGenericResponse,
	}
	wantedCheckpoint := &qrlpb.EventFinalizedCheckpoint{
		Block: make([]byte, 32),
		State: make([]byte, 32),
		Epoch: 8,
	}
	checkpointGenericResponse, err := anypb.New(wantedCheckpoint)
	require.NoError(t, err)
	wantedCheckpointMessage := &gateway.EventSource{
		Event: FinalizedCheckpointTopic,
		Data:  checkpointGenericResponse,
	}

	received := make(chan *gateway.EventSource, 2)
	mockStream.EXPECT().Send(wantedHeadMessage).Do(func(arg0 any) {
		received <- arg0.(*gateway.EventSource)
	})
	mockStream.EXPECT().Send(wantedCheckpointMessage).Do(func(arg0 any) {
		received <- arg0.(*gateway.EventSource)
	})

	run := startStream(ctx, t, srv, []string{HeadTopic + "," + FinalizedCheckpointTopic}, mockStream)
	f := srv.StateNotifier.StateFeed()
	sendUntilSubscribed(f, &feed.Event{Type: statefeed.NewHead, Data: wantedHead})
	require.Equal(t, HeadTopic, waitForEvent(t, received).Event)
	require.Equal(t, 1, f.Send(&feed.Event{Type: statefeed.FinalizedCheckpoint, Data: wantedCheckpoint}))
	require.Equal(t, FinalizedCheckpointTopic, waitForEvent(t, received).Event)

	require.Equal(t, codes.Canceled, status.Code(run.stop(t)))
}

// Regression test for the qrysm port of upstream #13329/#14413: the event feeds
// are written to synchronously from gossip validation and block import, and
// event.Feed.Send blocks until every subscriber has accepted the value. A
// client that stops reading the event stream must therefore be dropped by the
// stream, never allowed to back-pressure the feed producers.
func TestStreamEvents_SlowReaderIsShedWithoutBlockingProducers(t *testing.T) {
	ctx := context.Background()
	srv, ctrl, mockStream := setupServer(ctx, t)
	defer ctrl.Finish()
	const depth = 4
	srv.EventFeedDepth = depth

	// The client never reads: every Send blocks until the test releases it.
	release := make(chan struct{})
	mockStream.EXPECT().Send(gomock.Any()).DoAndReturn(func(any) error {
		<-release
		return nil
	}).AnyTimes()

	// Fetch the feed before starting the stream: the mock notifier's lazy
	// initialization is not synchronized.
	f := srv.StateNotifier.StateFeed()
	run := startStream(ctx, t, srv, []string{FinalizedCheckpointTopic}, mockStream)
	ev := &feed.Event{
		Type: statefeed.FinalizedCheckpoint,
		Data: &qrlpb.EventFinalizedCheckpoint{Block: make([]byte, 32), State: make([]byte, 32), Epoch: 1},
	}
	sendUntilSubscribed(f, ev)

	// Far more events than the subscription buffer plus the outbox can hold.
	// Before the fix the producer would block on the second event (buffer of
	// one, reader stuck in Send). Now every Send must return promptly and the
	// stream must shed the client once its outbox is full.
	const numEvents = 4*depth + 8
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		for range numEvents {
			f.Send(ev)
		}
	}()
	select {
	case <-producerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("feed producer blocked behind a stalled event stream client")
	}

	err := run.wait(t)
	require.Equal(t, codes.ResourceExhausted, status.Code(err), "unexpected error: %v", err)
	require.ErrorContains(t, "failed to read fast enough", err)
	// The stream unsubscribed from the feed on the way out.
	require.Equal(t, 0, f.Send(ev), "feed still has a subscriber after the slow client was shed")
	close(release)
}

func TestStreamEvents_SubscribesOnlyToRequestedFeeds(t *testing.T) {
	ctx := context.Background()
	srv, ctrl, mockStream := setupServer(ctx, t)
	defer ctrl.Finish()

	wantedAttV1alpha1 := util.HydrateAttestation(&qrysmpb.Attestation{Data: &qrysmpb.AttestationData{Slot: 8}})
	genericResponse, err := anypb.New(migration.V1Alpha1AttestationToV1(wantedAttV1alpha1))
	require.NoError(t, err)
	received := make(chan *gateway.EventSource, 1)
	mockStream.EXPECT().Send(&gateway.EventSource{Event: AttestationTopic, Data: genericResponse}).Do(func(arg0 any) {
		received <- arg0.(*gateway.EventSource)
	})

	opsFeed := srv.OperationNotifier.OperationFeed()
	stateFeed := srv.StateNotifier.StateFeed()
	run := startStream(ctx, t, srv, []string{AttestationTopic}, mockStream)
	sendUntilSubscribed(opsFeed, &feed.Event{
		Type: operation.UnaggregatedAttReceived,
		Data: &operation.UnAggregatedAttReceivedData{Attestation: wantedAttV1alpha1},
	})
	waitForEvent(t, received)

	// No state-feed topic was requested, so the stream must not be a subscriber
	// of the state feed at all.
	require.Equal(t, 0, stateFeed.Send(&feed.Event{Type: statefeed.NewHead, Data: &qrlpb.EventHead{}}))

	require.Equal(t, codes.Canceled, status.Code(run.stop(t)))
}

func TestStreamEvents_SendErrorTerminatesStream(t *testing.T) {
	ctx := context.Background()
	srv, ctrl, mockStream := setupServer(ctx, t)
	defer ctrl.Finish()

	mockStream.EXPECT().Send(gomock.Any()).Return(errors.New("client went away"))

	f := srv.StateNotifier.StateFeed()
	run := startStream(ctx, t, srv, []string{FinalizedCheckpointTopic}, mockStream)
	sendUntilSubscribed(f, &feed.Event{
		Type: statefeed.FinalizedCheckpoint,
		Data: &qrlpb.EventFinalizedCheckpoint{Block: make([]byte, 32), State: make([]byte, 32), Epoch: 1},
	})

	// The handler returns on its own once the sender reports the failure.
	err := run.wait(t)
	require.Equal(t, codes.Internal, status.Code(err), "unexpected error: %v", err)
	require.ErrorContains(t, "client went away", err)
}

func setupServer(ctx context.Context, t testing.TB) (*Server, *gomock.Controller, *mock.MockEvents_StreamEventsServer) {
	srv := &Server{
		StateNotifier:     &mockChain.MockStateNotifier{},
		OperationNotifier: &mockChain.MockOperationNotifier{},
		Ctx:               ctx,
	}
	ctrl := gomock.NewController(t)
	mockStream := mock.NewMockEvents_StreamEventsServer(ctrl)
	return srv, ctrl, mockStream
}

// streamRun is a StreamEvents call running in the background.
type streamRun struct {
	cancel context.CancelFunc
	errc   chan error
}

// startStream runs StreamEvents for the given topics against a cancellable
// stream context.
func startStream(ctx context.Context, t *testing.T, srv *Server, topics []string, stream *mock.MockEvents_StreamEventsServer) *streamRun {
	ctx, cancel := context.WithCancel(ctx)
	stream.EXPECT().Context().Return(ctx).AnyTimes()
	run := &streamRun{cancel: cancel, errc: make(chan error, 1)}
	go func() {
		run.errc <- srv.StreamEvents(&qrlpb.StreamEventsRequest{Topics: topics}, stream)
	}()
	t.Cleanup(cancel)
	return run
}

// wait blocks until StreamEvents returns and yields its error.
func (r *streamRun) wait(t *testing.T) error {
	select {
	case err := <-r.errc:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("StreamEvents did not return")
		return nil
	}
}

// stop cancels the stream context and waits for StreamEvents to return.
func (r *streamRun) stop(t *testing.T) error {
	r.cancel()
	return r.wait(t)
}

// sendUntilSubscribed sends the event in a loop until the stream has subscribed
// to the feed and accepted it.
func sendUntilSubscribed(f *event.Feed, ev *feed.Event) {
	for sent := 0; sent == 0; {
		sent = f.Send(ev)
	}
}

func waitForEvent(t *testing.T, received <-chan *gateway.EventSource) *gateway.EventSource {
	select {
	case ev := <-received:
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal("event was not streamed to the client")
		return nil
	}
}

type assertFeedArgs struct {
	t             *testing.T
	topics        []string
	srv           *Server
	stream        *mock.MockEvents_StreamEventsServer
	shouldReceive *gateway.EventSource
	itemToSend    *feed.Event
	feed          *event.Feed
}

func assertFeedSendAndReceive(ctx context.Context, args *assertFeedArgs) {
	t := args.t
	received := make(chan *gateway.EventSource, 1)
	args.stream.EXPECT().Send(args.shouldReceive).Do(func(arg0 any) {
		received <- arg0.(*gateway.EventSource)
	})

	run := startStream(ctx, t, args.srv, args.topics, args.stream)
	sendUntilSubscribed(args.feed, args.itemToSend)
	waitForEvent(t, received)
	require.Equal(t, codes.Canceled, status.Code(run.stop(t)))
}

// TestStreamEvents_HeadAndPayloadAttributesBothSent is a regression test for
// the state event handler returning after the head message, so a client
// subscribed to both "head" and "payload_attributes" (the usual relay /
// builder subscription) never received payload attributes for slots with a
// block.
func TestStreamEvents_HeadAndPayloadAttributesBothSent(t *testing.T) {
	ctx := context.Background()
	srv, ctrl, mockStream, wantedPayload := payloadAttributesFixture(ctx, t)
	defer ctrl.Finish()

	head := &qrlpb.EventHead{
		Slot:                      1,
		Block:                     make([]byte, 32),
		State:                     make([]byte, 32),
		EpochTransition:           false,
		PreviousDutyDependentRoot: make([]byte, 32),
		CurrentDutyDependentRoot:  make([]byte, 32),
	}
	wantedHead, err := anypb.New(head)
	require.NoError(t, err)
	wantedAttributes, err := anypb.New(wantedPayload)
	require.NoError(t, err)

	received := make(chan *gateway.EventSource, 2)
	record := func(arg0 any) { received <- arg0.(*gateway.EventSource) }
	gomock.InOrder(
		mockStream.EXPECT().Send(&gateway.EventSource{Event: HeadTopic, Data: wantedHead}).Do(record),
		mockStream.EXPECT().Send(&gateway.EventSource{Event: PayloadAttributesTopic, Data: wantedAttributes}).Do(record),
	)

	run := startStream(ctx, t, srv, []string{HeadTopic, PayloadAttributesTopic}, mockStream)
	sendUntilSubscribed(srv.StateNotifier.StateFeed(), &feed.Event{Type: statefeed.NewHead, Data: head})
	require.Equal(t, HeadTopic, waitForEvent(t, received).Event)
	require.Equal(t, PayloadAttributesTopic, waitForEvent(t, received).Event)
	require.Equal(t, codes.Canceled, status.Code(run.stop(t)))
}

// payloadAttributesFixture returns a server whose head has an execution
// payload and expected withdrawals, together with the payload_attributes
// event it must emit for the next proposal.
func payloadAttributesFixture(ctx context.Context, t *testing.T) (*Server, *gomock.Controller, *mock.MockEvents_StreamEventsServer, *qrlpb.EventPayloadAttributeV2) {
	beaconState, _ := util.DeterministicGenesisStateZond(t, 1)
	err := beaconState.SetSlot(2)
	require.NoError(t, err, "Count not set slot")
	err = beaconState.SetNextWithdrawalValidatorIndex(0)
	require.NoError(t, err, "Could not set withdrawal index")
	err = beaconState.SetBalances([]uint64{41000000000000})
	require.NoError(t, err, "Could not set validator balance")
	stateRoot, err := beaconState.HashTreeRoot(ctx)
	require.NoError(t, err, "Could not hash genesis state")

	genesis := blocks.NewGenesisBlock(stateRoot[:])

	parentRoot, err := genesis.Block.HashTreeRoot()
	require.NoError(t, err, "Could not get signing root")

	withdrawals, err := beaconState.ExpectedWithdrawals()
	require.NoError(t, err, "Could get expected withdrawals")
	require.NotEqual(t, len(withdrawals), 0)
	var scBits [fieldparams.SyncAggregateSyncCommitteeBytesLength]byte
	blk := &qrysmpb.SignedBeaconBlockZond{
		Block: &qrysmpb.BeaconBlockZond{
			ProposerIndex: 0,
			Slot:          1,
			ParentRoot:    parentRoot[:],
			StateRoot:     genesis.Block.StateRoot,
			Body: &qrysmpb.BeaconBlockBodyZond{
				RandaoReveal:  genesis.Block.Body.RandaoReveal,
				Graffiti:      genesis.Block.Body.Graffiti,
				ExecutionData: genesis.Block.Body.ExecutionData,
				SyncAggregate: &qrysmpb.SyncAggregate{SyncCommitteeBits: scBits[:], SyncCommitteeSignatures: [][]byte{}},
				ExecutionPayload: &enginev1.ExecutionPayloadZond{
					BlockNumber:   1,
					ParentHash:    make([]byte, fieldparams.RootLength),
					FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
					StateRoot:     make([]byte, fieldparams.RootLength),
					ReceiptsRoot:  make([]byte, fieldparams.RootLength),
					LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
					PrevRandao:    make([]byte, fieldparams.RootLength),
					BaseFeePerGas: make([]byte, fieldparams.RootLength),
					BlockHash:     make([]byte, fieldparams.RootLength),
					Withdrawals:   withdrawals,
				},
			},
		},
		Signature: genesis.Signature,
	}
	signedBlk, err := consensusBlocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	payload, err := signedBlk.Block().Body().Execution()
	require.NoError(t, err)
	require.NoError(t, beaconState.SetLatestExecutionPayloadHeader(payload))
	srv, ctrl, mockStream := setupServer(ctx, t)
	defer ctrl.Finish()
	fetcher := &mockChain.ChainService{
		Genesis:        time.Now(),
		State:          beaconState,
		Block:          signedBlk,
		Root:           make([]byte, 32),
		ValidatorsRoot: [32]byte{},
	}

	srv.HeadFetcher = fetcher
	srv.ChainInfoFetcher = fetcher

	prevRando, err := helpers.RandaoMix(beaconState, qrysmtime.CurrentEpoch(beaconState))
	require.NoError(t, err)

	wantedPayload := &qrlpb.EventPayloadAttributeV2{
		Version: version.String(version.Zond),
		Data: &qrlpb.EventPayloadAttributeV2_BasePayloadAttribute{
			ProposerIndex:     0,
			ProposalSlot:      2,
			ParentBlockNumber: 1,
			ParentBlockRoot:   make([]byte, 32),
			ParentBlockHash:   make([]byte, 32),
			PayloadAttributes: &enginev1.PayloadAttributesV2{
				Timestamp:             120,
				PrevRandao:            prevRando,
				SuggestedFeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
				Withdrawals:           withdrawals,
			},
		},
	}
	return srv, ctrl, mockStream, wantedPayload
}

// Advance the published head after a read returns its snapshot, just as a
// concurrent block import can do between the event builder's separate reads.
type changingPayloadHead struct {
	*mockChain.ChainService
	afterRead func()
}

func (h *changingPayloadHead) advance() {
	if h.afterRead != nil {
		afterRead := h.afterRead
		h.afterRead = nil
		afterRead()
	}
}

func (h *changingPayloadHead) HeadRoot(ctx context.Context) ([]byte, error) {
	root, err := h.ChainService.HeadRoot(ctx)
	h.advance()
	return root, err
}

func (h *changingPayloadHead) HeadState(ctx context.Context) (state.BeaconState, error) {
	st, err := h.ChainService.HeadState(ctx)
	if st != nil {
		st = st.Copy()
	}
	h.advance()
	return st, err
}

func (h *changingPayloadHead) HeadStateAndRoot(ctx context.Context) (state.BeaconState, []byte, error) {
	st, root, err := h.ChainService.HeadStateAndRoot(ctx)
	h.advance()
	return st, root, err
}

func TestPayloadAttributes_ConcurrentHeadChange(t *testing.T) {
	for _, changeHead := range []bool{false, true} {
		name := "stable_head"
		if changeHead {
			name = "head_changes_after_snapshot"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			srv, ctrl, _, want := payloadAttributesFixture(ctx, t)
			defer ctrl.Finish()
			current := srv.HeadFetcher.(*mockChain.ChainService)
			current.Root = bytes.Repeat([]byte{0x11}, fieldparams.RootLength)
			want.Data.ParentBlockRoot = current.Root
			fetcher := &changingPayloadHead{ChainService: current}
			if changeHead {
				next := *current
				next.Root = bytes.Repeat([]byte{0x22}, fieldparams.RootLength)
				next.State = current.State.Copy()
				require.NoError(t, next.State.SetBalances([]uint64{42000000000000}))
				block, err := current.Block.Copy()
				require.NoError(t, err)
				pb, err := block.Proto()
				require.NoError(t, err)
				payload := pb.(*qrysmpb.SignedBeaconBlockZond).Block.Body.ExecutionPayload
				payload.BlockNumber = 2
				payload.BlockHash = bytes.Repeat([]byte{0x33}, fieldparams.RootLength)
				next.Block, err = consensusBlocks.NewSignedBeaconBlock(pb)
				require.NoError(t, err)
				execution, err := next.Block.Block().Body().Execution()
				require.NoError(t, err)
				require.NoError(t, next.State.SetLatestExecutionPayloadHeader(execution))
				fetcher.afterRead = func() { fetcher.ChainService = &next }
			}
			srv.HeadFetcher, srv.ChainInfoFetcher = fetcher, fetcher
			msg, err := srv.buildPayloadAttributesMessage()
			require.NoError(t, err)
			require.NotNil(t, msg)
			got := &qrlpb.EventPayloadAttributeV2{}
			require.NoError(t, msg.Data.UnmarshalTo(got))
			require.DeepEqual(t, want, got, "parent root, execution parent, and withdrawals must describe one head")
		})
	}
}
