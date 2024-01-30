package events

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/grpc-ecosystem/grpc-gateway/v2/proto/gateway"
	"github.com/theQRL/go-bitfield"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/v4/async/event"
	"github.com/theQRL/qrysm/v4/beacon-chain/blockchain"
	mockChain "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	b "github.com/theQRL/qrysm/v4/beacon-chain/core/blocks"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/feed"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/feed/operation"
	statefeed "github.com/theQRL/qrysm/v4/beacon-chain/core/feed/state"
	"github.com/theQRL/qrysm/v4/beacon-chain/core/helpers"
	prysmtime "github.com/theQRL/qrysm/v4/beacon-chain/core/time"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	"github.com/theQRL/qrysm/v4/proto/migration"
	zond "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	zondpb "github.com/theQRL/qrysm/v4/proto/zond/v1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/mock"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestStreamEvents_Preconditions(t *testing.T) {
	t.Run("no_topics_specified", func(t *testing.T) {
		srv := &Server{}
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockStream := mock.NewMockEvents_StreamEventsServer(ctrl)
		err := srv.StreamEvents(&zondpb.StreamEventsRequest{Topics: nil}, mockStream)
		require.ErrorContains(t, "No topics specified", err)
	})
	t.Run("topic_not_allowed", func(t *testing.T) {
		srv := &Server{}
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockStream := mock.NewMockEvents_StreamEventsServer(ctrl)
		err := srv.StreamEvents(&zondpb.StreamEventsRequest{Topics: []string{"foobar"}}, mockStream)
		require.ErrorContains(t, "Topic foobar not allowed", err)
	})
}

func TestStreamEvents_OperationsEvents(t *testing.T) {
	t.Run("attestation_unaggregated", func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		wantedAttV1alpha1 := util.HydrateAttestation(&zond.Attestation{
			Data: &zond.AttestationData{
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

		wantedAttV1alpha1 := &zond.AggregateAttestationAndProof{
			Aggregate: util.HydrateAttestation(&zond.Attestation{}),
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

		wantedExitV1alpha1 := &zond.SignedVoluntaryExit{
			Exit: &zond.VoluntaryExit{
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

		wantedContributionV1alpha1 := &zond.SignedContributionAndProof{
			Message: &zond.ContributionAndProof{
				AggregatorIndex: 1,
				Contribution: &zond.SyncCommitteeContribution{
					Slot:              1,
					BlockRoot:         []byte("root"),
					SubcommitteeIndex: 1,
					AggregationBits:   bitfield.NewBitvector128(),
					Signature:         []byte("sig"),
				},
				SelectionProof: []byte("proof"),
			},
			Signature: []byte("sig"),
		}
		wantedContribution := migration.V1Alpha1SignedContributionAndProofToV2(wantedContributionV1alpha1)
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
	t.Run(DilithiumToExecutionChangeTopic, func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		wantedChangeV1alpha1 := &zond.SignedDilithiumToExecutionChange{
			Message: &zond.DilithiumToExecutionChange{
				ValidatorIndex:      1,
				FromDilithiumPubkey: []byte("from"),
				ToExecutionAddress:  []byte("to"),
			},
			Signature: make([]byte, 96),
		}
		wantedChange := migration.V1Alpha1SignedDilithiumToExecChangeToV2(wantedChangeV1alpha1)
		genericResponse, err := anypb.New(wantedChange)
		require.NoError(t, err)

		wantedMessage := &gateway.EventSource{
			Event: DilithiumToExecutionChangeTopic,
			Data:  genericResponse,
		}

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{DilithiumToExecutionChangeTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: operation.DilithiumToExecutionChangeReceived,
				Data: &operation.DilithiumToExecutionChangeReceivedData{
					Change: wantedChangeV1alpha1,
				},
			},
			feed: srv.OperationNotifier.OperationFeed(),
		})
	})
	t.Run(BlobSidecarTopic, func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()
		commitment, err := hexutil.Decode("0x1b66ac1fb663c9bc59509846d6ec05345bd908eda73e670af888da41af171505cc411d61252fb6cb3fa0017b679f8000")
		require.NoError(t, err)
		wantedBlobV1alpha1 := &zond.SignedBlobSidecar{
			Message: &zond.BlobSidecar{
				BlockRoot:     make([]byte, fieldparams.RootLength),
				Index:         1,
				Slot:          3,
				KzgCommitment: commitment,
			},
			Signature: make([]byte, 96),
		}
		versionedHash := blockchain.ConvertKzgCommitmentToVersionedHash(commitment)
		blobEvent := &zondpb.EventBlobSidecar{
			BlockRoot:     bytesutil.SafeCopyBytes(wantedBlobV1alpha1.Message.BlockRoot),
			Index:         wantedBlobV1alpha1.Message.Index,
			Slot:          wantedBlobV1alpha1.Message.Slot,
			VersionedHash: bytesutil.SafeCopyBytes(versionedHash.Bytes()),
			KzgCommitment: bytesutil.SafeCopyBytes(wantedBlobV1alpha1.Message.KzgCommitment),
		}
		genericResponse, err := anypb.New(blobEvent)
		require.NoError(t, err)

		wantedMessage := &gateway.EventSource{
			Event: BlobSidecarTopic,
			Data:  genericResponse,
		}

		assertFeedSendAndReceive(ctx, &assertFeedArgs{
			t:             t,
			srv:           srv,
			topics:        []string{BlobSidecarTopic},
			stream:        mockStream,
			shouldReceive: wantedMessage,
			itemToSend: &feed.Event{
				Type: operation.BlobSidecarReceived,
				Data: &operation.BlobSidecarReceivedData{
					Blob: wantedBlobV1alpha1,
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

		wantedHead := &zondpb.EventHead{
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
	t.Run(PayloadAttributesTopic+"_bellatrix", func(t *testing.T) {
		ctx := context.Background()

		beaconState, _ := util.DeterministicGenesisStateBellatrix(t, 1)
		err := beaconState.SetSlot(2)
		require.NoError(t, err, "Count not set slot")
		stateRoot, err := beaconState.HashTreeRoot(ctx)
		require.NoError(t, err, "Could not hash genesis state")

		genesis := b.NewGenesisBlock(stateRoot[:])

		parentRoot, err := genesis.Block.HashTreeRoot()
		require.NoError(t, err, "Could not get signing root")

		var scBits [fieldparams.SyncAggregateSyncCommitteeBytesLength]byte
		blk := &zond.SignedBeaconBlockBellatrix{
			Block: &zond.BeaconBlockBellatrix{
				ProposerIndex: 0,
				Slot:          1,
				ParentRoot:    parentRoot[:],
				StateRoot:     genesis.Block.StateRoot,
				Body: &zond.BeaconBlockBodyBellatrix{
					RandaoReveal:  genesis.Block.Body.RandaoReveal,
					Graffiti:      genesis.Block.Body.Graffiti,
					Eth1Data:      genesis.Block.Body.Eth1Data,
					SyncAggregate: &zond.SyncAggregate{SyncCommitteeBits: scBits[:], SyncCommitteeSignature: make([]byte, 96)},
					ExecutionPayload: &enginev1.ExecutionPayload{
						BlockNumber:   1,
						ParentHash:    make([]byte, fieldparams.RootLength),
						FeeRecipient:  make([]byte, fieldparams.FeeRecipientLength),
						StateRoot:     make([]byte, fieldparams.RootLength),
						ReceiptsRoot:  make([]byte, fieldparams.RootLength),
						LogsBloom:     make([]byte, fieldparams.LogsBloomLength),
						PrevRandao:    make([]byte, fieldparams.RootLength),
						BaseFeePerGas: make([]byte, fieldparams.RootLength),
						BlockHash:     make([]byte, fieldparams.RootLength),
					},
				},
			},
			Signature: genesis.Signature,
		}
		signedBlk, err := blocks.NewSignedBeaconBlock(blk)
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

		prevRando, err := helpers.RandaoMix(beaconState, prysmtime.CurrentEpoch(beaconState))
		require.NoError(t, err)

		wantedPayload := &zondpb.EventPayloadAttributeV1{
			Version: version.String(version.Bellatrix),
			Data: &zondpb.EventPayloadAttributeV1_BasePayloadAttribute{
				ProposerIndex:     0,
				ProposalSlot:      2,
				ParentBlockNumber: 1,
				ParentBlockRoot:   make([]byte, 32),
				ParentBlockHash:   make([]byte, 32),
				PayloadAttributes: &enginev1.PayloadAttributes{
					Timestamp:             24,
					PrevRandao:            prevRando,
					SuggestedFeeRecipient: make([]byte, 20),
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
	t.Run(PayloadAttributesTopic+"_capella", func(t *testing.T) {
		ctx := context.Background()
		beaconState, _ := util.DeterministicGenesisStateCapella(t, 1)
		validator, err := beaconState.ValidatorAtIndex(0)
		require.NoError(t, err, "Could not get validator")
		by, err := hexutil.Decode("0x010000000000000000000000a94f5374fce5edbc8e2a8697c15331677e6ebf0b")
		require.NoError(t, err)
		validator.WithdrawalCredentials = by
		err = beaconState.UpdateValidatorAtIndex(0, validator)
		require.NoError(t, err)
		err = beaconState.SetSlot(2)
		require.NoError(t, err, "Count not set slot")
		err = beaconState.SetNextWithdrawalValidatorIndex(0)
		require.NoError(t, err, "Could not set withdrawal index")
		err = beaconState.SetBalances([]uint64{33000000000})
		require.NoError(t, err, "Could not set validator balance")
		stateRoot, err := beaconState.HashTreeRoot(ctx)
		require.NoError(t, err, "Could not hash genesis state")

		genesis := b.NewGenesisBlock(stateRoot[:])

		parentRoot, err := genesis.Block.HashTreeRoot()
		require.NoError(t, err, "Could not get signing root")

		withdrawals, err := beaconState.ExpectedWithdrawals()
		require.NoError(t, err, "Could get expected withdrawals")
		require.NotEqual(t, len(withdrawals), 0)
		var scBits [fieldparams.SyncAggregateSyncCommitteeBytesLength]byte
		blk := &zond.SignedBeaconBlockCapella{
			Block: &zond.BeaconBlockCapella{
				ProposerIndex: 0,
				Slot:          1,
				ParentRoot:    parentRoot[:],
				StateRoot:     genesis.Block.StateRoot,
				Body: &zond.BeaconBlockBodyCapella{
					RandaoReveal:  genesis.Block.Body.RandaoReveal,
					Graffiti:      genesis.Block.Body.Graffiti,
					Eth1Data:      genesis.Block.Body.Eth1Data,
					SyncAggregate: &zond.SyncAggregate{SyncCommitteeBits: scBits[:], SyncCommitteeSignature: make([]byte, 96)},
					ExecutionPayload: &enginev1.ExecutionPayloadCapella{
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
		signedBlk, err := blocks.NewSignedBeaconBlock(blk)
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

		prevRando, err := helpers.RandaoMix(beaconState, prysmtime.CurrentEpoch(beaconState))
		require.NoError(t, err)

		wantedPayload := &zondpb.EventPayloadAttributeV2{
			Version: version.String(version.Capella),
			Data: &zondpb.EventPayloadAttributeV2_BasePayloadAttribute{
				ProposerIndex:     0,
				ProposalSlot:      2,
				ParentBlockNumber: 1,
				ParentBlockRoot:   make([]byte, 32),
				ParentBlockHash:   make([]byte, 32),
				PayloadAttributes: &enginev1.PayloadAttributesV2{
					Timestamp:             24,
					PrevRandao:            prevRando,
					SuggestedFeeRecipient: make([]byte, 20),
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
	t.Run(FinalizedCheckpointTopic, func(t *testing.T) {
		ctx := context.Background()
		srv, ctrl, mockStream := setupServer(ctx, t)
		defer ctrl.Finish()

		wantedCheckpoint := &zondpb.EventFinalizedCheckpoint{
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

		wantedReorg := &zondpb.EventChainReorg{
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

		blk := util.HydrateSignedBeaconBlock(&zond.SignedBeaconBlock{
			Block: &zond.BeaconBlock{
				Slot: 8,
			},
		})
		bodyRoot, err := blk.Block.Body.HashTreeRoot()
		require.NoError(t, err)
		wantedHeader := util.HydrateBeaconHeader(&zond.BeaconBlockHeader{
			Slot:     8,
			BodyRoot: bodyRoot[:],
		})
		wantedBlockRoot, err := wantedHeader.HashTreeRoot()
		require.NoError(t, err)
		genericResponse, err := anypb.New(&zondpb.EventBlock{
			Slot:                8,
			Block:               wantedBlockRoot[:],
			ExecutionOptimistic: true,
		})
		require.NoError(t, err)
		wantedMessage := &gateway.EventSource{
			Event: BlockTopic,
			Data:  genericResponse,
		}
		wsb, err := blocks.NewSignedBeaconBlock(blk)
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

	wantedHead := &zondpb.EventHead{
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

	wantedCheckpoint := &zondpb.EventFinalizedCheckpoint{
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
	shouldReceive interface{}
	itemToSend    *feed.Event
	feed          *event.Feed
}

func assertFeedSendAndReceive(ctx context.Context, args *assertFeedArgs) {
	exitRoutine := make(chan bool)
	defer close(exitRoutine)
	args.stream.EXPECT().Send(args.shouldReceive).Do(func(arg0 interface{}) {
		exitRoutine <- true
	})
	args.stream.EXPECT().Context().Return(ctx).AnyTimes()

	req := &zondpb.StreamEventsRequest{Topics: args.topics}
	go func(tt *testing.T) {
		assert.NoError(tt, args.srv.StreamEvents(req, args.stream), "Could not call RPC method")
	}(args.t)
	// Send in a loop to ensure it is delivered (busy wait for the service to subscribe to the state feed).
	for sent := 0; sent == 0; {
		sent = args.feed.Send(args.itemToSend)
	}
	<-exitRoutine
}
