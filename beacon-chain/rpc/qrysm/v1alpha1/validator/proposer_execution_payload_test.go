package validator

import (
	"context"
	"errors"
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-zond/common"
	chainMock "github.com/theQRL/qrysm/v4/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/cache"
	dbTest "github.com/theQRL/qrysm/v4/beacon-chain/db/testing"
	powtesting "github.com/theQRL/qrysm/v4/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/v4/beacon-chain/state"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/primitives"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	pb "github.com/theQRL/qrysm/v4/proto/engine/v1"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestServer_getExecutionPayload(t *testing.T) {
	beaconDB := dbTest.SetupDB(t)
	nonTransitionSt, _ := util.DeterministicGenesisStateCapella(t, 1)
	b1pb := util.NewBeaconBlockCapella()
	b1r, err := b1pb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b1pb)
	require.NoError(t, nonTransitionSt.SetFinalizedCheckpoint(&zondpb.Checkpoint{
		Root: b1r[:],
	}))

	transitionSt, _ := util.DeterministicGenesisStateCapella(t, 1)
	wrappedHeader, err := blocks.WrappedExecutionPayloadHeaderCapella(&pb.ExecutionPayloadHeaderCapella{BlockNumber: 1}, 0)
	require.NoError(t, err)
	require.NoError(t, transitionSt.SetLatestExecutionPayloadHeader(wrappedHeader))
	b2pb := util.NewBeaconBlockCapella()
	b2r, err := b2pb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b2pb)
	require.NoError(t, transitionSt.SetFinalizedCheckpoint(&zondpb.Checkpoint{
		Root: b2r[:],
	}))

	capellaTransitionState, _ := util.DeterministicGenesisStateCapella(t, 1)
	wrappedHeaderCapella, err := blocks.WrappedExecutionPayloadHeaderCapella(&pb.ExecutionPayloadHeaderCapella{BlockNumber: 1}, 0)
	require.NoError(t, err)
	require.NoError(t, capellaTransitionState.SetLatestExecutionPayloadHeader(wrappedHeaderCapella))
	b2pbCapella := util.NewBeaconBlockCapella()
	b2rCapella, err := b2pbCapella.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b2pbCapella)
	require.NoError(t, capellaTransitionState.SetFinalizedCheckpoint(&zondpb.Checkpoint{
		Root: b2rCapella[:],
	}))

	require.NoError(t, beaconDB.SaveFeeRecipientsByValidatorIDs(context.Background(), []primitives.ValidatorIndex{0}, []common.Address{{}}))

	tests := []struct {
		name              string
		st                state.BeaconState
		errString         string
		forkchoiceErr     error
		payloadID         *pb.PayloadIDBytes
		terminalBlockHash common.Hash
		activationEpoch   primitives.Epoch
		validatorIndx     primitives.ValidatorIndex
		override          bool
		wantedOverride    bool
	}{
		{
			name:      "transition completed, nil payload id",
			st:        transitionSt,
			errString: "nil payload with block hash",
		},
		{
			name:      "transition completed, happy case (has fee recipient in Db)",
			st:        transitionSt,
			payloadID: &pb.PayloadIDBytes{0x1},
		},
		{
			name:          "transition completed, happy case (doesn't have fee recipient in Db)",
			st:            transitionSt,
			payloadID:     &pb.PayloadIDBytes{0x1},
			validatorIndx: 1,
		},
		{
			name:          "transition completed, capella, happy case (doesn't have fee recipient in Db)",
			st:            capellaTransitionState,
			payloadID:     &pb.PayloadIDBytes{0x1},
			validatorIndx: 1,
		},
		{
			name:          "transition completed, happy case, (payload ID cached)",
			st:            transitionSt,
			validatorIndx: 100,
		},
		{
			name:          "transition completed, could not prepare payload",
			st:            transitionSt,
			forkchoiceErr: errors.New("fork choice error"),
			errString:     "could not prepare payload",
		},
		{
			name:           "local client override",
			st:             transitionSt,
			validatorIndx:  100,
			override:       true,
			wantedOverride: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := params.BeaconConfig().Copy()
			params.OverrideBeaconConfig(cfg)

			vs := &Server{
				ExecutionEngineCaller:  &powtesting.EngineClient{PayloadIDBytes: tt.payloadID, ErrForkchoiceUpdated: tt.forkchoiceErr, ExecutionPayloadCapella: &pb.ExecutionPayloadCapella{}, BuilderOverride: tt.override},
				HeadFetcher:            &chainMock.ChainService{State: tt.st},
				FinalizationFetcher:    &chainMock.ChainService{},
				BeaconDB:               beaconDB,
				ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
			}
			vs.ProposerSlotIndexCache.SetProposerAndPayloadIDs(tt.st.Slot(), 100, [8]byte{100}, [32]byte{'a'})
			blk := util.NewBeaconBlockCapella()
			blk.Block.Slot = tt.st.Slot()
			blk.Block.ProposerIndex = tt.validatorIndx
			blk.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
			b, err := blocks.NewSignedBeaconBlock(blk)
			require.NoError(t, err)
			var gotOverride bool
			_, gotOverride, err = vs.getLocalPayload(context.Background(), b.Block(), tt.st)
			if tt.errString != "" {
				require.ErrorContains(t, tt.errString, err)
			} else {
				require.Equal(t, tt.wantedOverride, gotOverride)
				require.NoError(t, err)
			}
		})
	}
}

func TestServer_getExecutionPayloadContextTimeout(t *testing.T) {
	beaconDB := dbTest.SetupDB(t)
	nonTransitionSt, _ := util.DeterministicGenesisStateCapella(t, 1)
	b1pb := util.NewBeaconBlockCapella()
	b1r, err := b1pb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b1pb)
	require.NoError(t, nonTransitionSt.SetFinalizedCheckpoint(&zondpb.Checkpoint{
		Root: b1r[:],
	}))

	require.NoError(t, beaconDB.SaveFeeRecipientsByValidatorIDs(context.Background(), []primitives.ValidatorIndex{0}, []common.Address{{}}))

	vs := &Server{
		ExecutionEngineCaller:  &powtesting.EngineClient{PayloadIDBytes: &pb.PayloadIDBytes{}, ErrGetPayload: context.DeadlineExceeded, ExecutionPayloadCapella: &pb.ExecutionPayloadCapella{}},
		HeadFetcher:            &chainMock.ChainService{State: nonTransitionSt},
		BeaconDB:               beaconDB,
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
	}
	vs.ProposerSlotIndexCache.SetProposerAndPayloadIDs(nonTransitionSt.Slot(), 100, [8]byte{100}, [32]byte{'a'})

	blk := util.NewBeaconBlockCapella()
	blk.Block.Slot = nonTransitionSt.Slot()
	blk.Block.ProposerIndex = 100
	blk.Block.ParentRoot = bytesutil.PadTo([]byte{'a'}, 32)
	b, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	_, _, err = vs.getLocalPayload(context.Background(), b.Block(), nonTransitionSt)
	require.NoError(t, err)
}

func TestServer_getExecutionPayload_UnexpectedFeeRecipient(t *testing.T) {
	hook := logTest.NewGlobal()
	beaconDB := dbTest.SetupDB(t)
	nonTransitionSt, _ := util.DeterministicGenesisStateCapella(t, 1)
	b1pb := util.NewBeaconBlockCapella()
	b1r, err := b1pb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b1pb)
	require.NoError(t, nonTransitionSt.SetFinalizedCheckpoint(&zondpb.Checkpoint{
		Root: b1r[:],
	}))

	transitionSt, _ := util.DeterministicGenesisStateCapella(t, 1)
	wrappedHeader, err := blocks.WrappedExecutionPayloadHeaderCapella(&pb.ExecutionPayloadHeaderCapella{BlockNumber: 1}, 0)
	require.NoError(t, err)
	require.NoError(t, transitionSt.SetLatestExecutionPayloadHeader(wrappedHeader))
	b2pb := util.NewBeaconBlockCapella()
	b2r, err := b2pb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b2pb)
	require.NoError(t, transitionSt.SetFinalizedCheckpoint(&zondpb.Checkpoint{
		Root: b2r[:],
	}))

	feeRecipient := common.BytesToAddress([]byte("a"))
	require.NoError(t, beaconDB.SaveFeeRecipientsByValidatorIDs(context.Background(), []primitives.ValidatorIndex{0}, []common.Address{
		feeRecipient,
	}))

	payloadID := &pb.PayloadIDBytes{0x1}
	payload := emptyPayloadCapella()
	payload.FeeRecipient = feeRecipient[:]
	vs := &Server{
		ExecutionEngineCaller: &powtesting.EngineClient{
			PayloadIDBytes:          payloadID,
			ExecutionPayloadCapella: payload,
		},
		HeadFetcher:            &chainMock.ChainService{State: transitionSt},
		FinalizationFetcher:    &chainMock.ChainService{},
		BeaconDB:               beaconDB,
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
	}

	blk := util.NewBeaconBlockCapella()
	blk.Block.Slot = transitionSt.Slot()
	blk.Block.ParentRoot = bytesutil.PadTo([]byte{}, 32)
	b, err := blocks.NewSignedBeaconBlock(blk)
	require.NoError(t, err)
	gotPayload, _, err := vs.getLocalPayload(context.Background(), b.Block(), transitionSt)
	require.NoError(t, err)
	require.NotNil(t, gotPayload)

	// We should NOT be getting the warning.
	require.LogsDoNotContain(t, hook, "Fee recipient address from execution client is not what was expected")
	hook.Reset()

	evilRecipientAddress := common.BytesToAddress([]byte("evil"))
	payload.FeeRecipient = evilRecipientAddress[:]
	vs.ProposerSlotIndexCache = cache.NewProposerPayloadIDsCache()

	gotPayload, _, err = vs.getLocalPayload(context.Background(), b.Block(), transitionSt)
	require.NoError(t, err)
	require.NotNil(t, gotPayload)

	// Users should be warned.
	require.LogsContain(t, hook, "Fee recipient address from execution client is not what was expected")
}
