package events

import (
	"bytes"
	"context"
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
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	consensusBlocks "github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	enginev1 "github.com/theQRL/qrysm/proto/engine/v1"
	"github.com/theQRL/qrysm/proto/migration"
	qrlpb "github.com/theQRL/qrysm/proto/qrl/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/mock"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
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

	assertFeedSendAndReceive(ctx, &assertFeedArgs{
		t:             t,
		srv:           srv,
		topics:        []string{HeadTopic + "," + FinalizedCheckpointTopic},
		stream:        mockStream,
		shouldReceive: wantedHeadMessage,
		itemToSend: &feed.Event{
			Type: statefeed.NewHead,
			Data: wantedHead,
		},
		feed: srv.StateNotifier.StateFeed(),
	})

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

	assertFeedSendAndReceive(ctx, &assertFeedArgs{
		t:             t,
		srv:           srv,
		topics:        []string{HeadTopic + "," + FinalizedCheckpointTopic},
		stream:        mockStream,
		shouldReceive: wantedCheckpointMessage,
		itemToSend: &feed.Event{
			Type: statefeed.FinalizedCheckpoint,
			Data: wantedCheckpoint,
		},
		feed: srv.StateNotifier.StateFeed(),
	})
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

type assertFeedArgs struct {
	t             *testing.T
	topics        []string
	srv           *Server
	stream        *mock.MockEvents_StreamEventsServer
	shouldReceive any
	itemToSend    *feed.Event
	feed          *event.Feed
}

func assertFeedSendAndReceive(ctx context.Context, args *assertFeedArgs) {
	exitRoutine := make(chan bool)
	defer close(exitRoutine)
	args.stream.EXPECT().Send(args.shouldReceive).Do(func(arg0 any) {
		exitRoutine <- true
	})
	args.stream.EXPECT().Context().Return(ctx).AnyTimes()

	req := &qrlpb.StreamEventsRequest{Topics: args.topics}
	go func(tt *testing.T) {
		assert.NoError(tt, args.srv.StreamEvents(req, args.stream), "Could not call RPC method")
	}(args.t)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = args.feed.Send(args.itemToSend)
	}

	<-exitRoutine
}
