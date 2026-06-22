package validator

import (
	"context"
	"errors"
	"testing"

	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/go-qrl/common"
	chainMock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/cache"
	dbTest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	exectesting "github.com/theQRL/qrysm/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/beacon-chain/state"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	pb "github.com/theQRL/qrysm/proto/engine/v1"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestServer_getExecutionPayload(t *testing.T) {
	beaconDB := dbTest.SetupDB(t)
	nonTransitionSt, _ := util.DeterministicGenesisStateZond(t, 1)
	b1pb := util.NewBeaconBlockZond()
	b1r, err := b1pb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b1pb)
	require.NoError(t, nonTransitionSt.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{
		Root: b1r[:],
	}))

	transitionSt, _ := util.DeterministicGenesisStateZond(t, 1)
	wrappedHeader, err := blocks.WrappedExecutionPayloadHeaderZond(&pb.ExecutionPayloadHeaderZond{BlockNumber: 1}, 0)
	require.NoError(t, err)
	require.NoError(t, transitionSt.SetLatestExecutionPayloadHeader(wrappedHeader))
	b2pb := util.NewBeaconBlockZond()
	b2r, err := b2pb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b2pb)
	require.NoError(t, transitionSt.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{
		Root: b2r[:],
	}))

	zondTransitionState, _ := util.DeterministicGenesisStateZond(t, 1)
	wrappedHeaderZond, err := blocks.WrappedExecutionPayloadHeaderZond(&pb.ExecutionPayloadHeaderZond{BlockNumber: 1}, 0)
	require.NoError(t, err)
	require.NoError(t, zondTransitionState.SetLatestExecutionPayloadHeader(wrappedHeaderZond))
	b2pbZond := util.NewBeaconBlockZond()
	b2rZond, err := b2pbZond.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b2pbZond)
	require.NoError(t, zondTransitionState.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{
		Root: b2rZond[:],
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
			name:          "transition completed, zond, happy case (doesn't have fee recipient in Db)",
			st:            zondTransitionState,
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
				ExecutionEngineCaller:  &exectesting.EngineClient{PayloadIDBytes: tt.payloadID, ErrForkchoiceUpdated: tt.forkchoiceErr, ExecutionPayloadZond: &pb.ExecutionPayloadZond{}, BuilderOverride: tt.override},
				HeadFetcher:            &chainMock.ChainService{State: tt.st},
				FinalizationFetcher:    &chainMock.ChainService{},
				BeaconDB:               beaconDB,
				ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
			}
			vs.ProposerSlotIndexCache.SetProposerAndPayloadIDs(tt.st.Slot(), 100, [8]byte{100}, [32]byte{'a'})
			blk := util.NewBeaconBlockZond()
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
	nonTransitionSt, _ := util.DeterministicGenesisStateZond(t, 1)
	b1pb := util.NewBeaconBlockZond()
	b1r, err := b1pb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b1pb)
	require.NoError(t, nonTransitionSt.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{
		Root: b1r[:],
	}))

	require.NoError(t, beaconDB.SaveFeeRecipientsByValidatorIDs(context.Background(), []primitives.ValidatorIndex{0}, []common.Address{{}}))

	vs := &Server{
		ExecutionEngineCaller:  &exectesting.EngineClient{PayloadIDBytes: &pb.PayloadIDBytes{}, ErrGetPayload: context.DeadlineExceeded, ExecutionPayloadZond: &pb.ExecutionPayloadZond{}},
		HeadFetcher:            &chainMock.ChainService{State: nonTransitionSt},
		BeaconDB:               beaconDB,
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
	}
	vs.ProposerSlotIndexCache.SetProposerAndPayloadIDs(nonTransitionSt.Slot(), 100, [8]byte{100}, [32]byte{'a'})

	blk := util.NewBeaconBlockZond()
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
	nonTransitionSt, _ := util.DeterministicGenesisStateZond(t, 1)
	b1pb := util.NewBeaconBlockZond()
	b1r, err := b1pb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b1pb)
	require.NoError(t, nonTransitionSt.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{
		Root: b1r[:],
	}))

	transitionSt, _ := util.DeterministicGenesisStateZond(t, 1)
	wrappedHeader, err := blocks.WrappedExecutionPayloadHeaderZond(&pb.ExecutionPayloadHeaderZond{BlockNumber: 1}, 0)
	require.NoError(t, err)
	require.NoError(t, transitionSt.SetLatestExecutionPayloadHeader(wrappedHeader))
	b2pb := util.NewBeaconBlockZond()
	b2r, err := b2pb.Block.HashTreeRoot()
	require.NoError(t, err)
	util.SaveBlock(t, context.Background(), beaconDB, b2pb)
	require.NoError(t, transitionSt.SetFinalizedCheckpoint(&qrysmpb.Checkpoint{
		Root: b2r[:],
	}))

	feeRecipient := common.BytesToAddress([]byte("a"))
	require.NoError(t, beaconDB.SaveFeeRecipientsByValidatorIDs(context.Background(), []primitives.ValidatorIndex{0}, []common.Address{
		feeRecipient,
	}))

	payloadID := &pb.PayloadIDBytes{0x1}
	payload := emptyPayloadZond()
	payload.FeeRecipient = feeRecipient[:]
	vs := &Server{
		ExecutionEngineCaller: &exectesting.EngineClient{
			PayloadIDBytes:       payloadID,
			ExecutionPayloadZond: payload,
		},
		HeadFetcher:            &chainMock.ChainService{State: transitionSt},
		FinalizationFetcher:    &chainMock.ChainService{},
		BeaconDB:               beaconDB,
		ProposerSlotIndexCache: cache.NewProposerPayloadIDsCache(),
	}

	blk := util.NewBeaconBlockZond()
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
