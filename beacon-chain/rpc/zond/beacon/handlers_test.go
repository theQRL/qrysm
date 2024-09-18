package beacon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/theQRL/go-zond/common/hexutil"
	"github.com/theQRL/qrysm/api"
	chainMock "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/core/transition"
	dbTest "github.com/theQRL/qrysm/beacon-chain/db/testing"
	doublylinkedtree "github.com/theQRL/qrysm/beacon-chain/forkchoice/doubly-linked-tree"
	"github.com/theQRL/qrysm/beacon-chain/rpc/testutil"
	"github.com/theQRL/qrysm/beacon-chain/rpc/zond/shared"
	rpctesting "github.com/theQRL/qrysm/beacon-chain/rpc/zond/shared/testing"
	"github.com/theQRL/qrysm/beacon-chain/state"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	"github.com/theQRL/qrysm/consensus-types/interfaces"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/encoding/bytesutil"
	http2 "github.com/theQRL/qrysm/network/http"
	zond "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/assert"
	mock2 "github.com/theQRL/qrysm/testing/mock"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
	"github.com/theQRL/qrysm/time/slots"
)

func TestPublishBlock(t *testing.T) {
	ctrl := gomock.NewController(t)

	t.Run("Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *zond.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*zond.GenericSignedBeaconBlock_Capella)
			converted, err := shared.BeaconBlockCapellaFromConsensus(block.Capella.Block)
			require.NoError(t, err)
			var signedblock *shared.SignedBeaconBlockCapella
			err = json.Unmarshal([]byte(rpctesting.CapellaBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.CapellaBlock)))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedCapellaBlock)))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "please add the api header"))
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "Body does not represent a valid block type"))
	})
	t.Run("invalid block with version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BadCapellaBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		body := writer.Body.String()
		assert.Equal(t, true, strings.Contains(body, "Body does not represent a valid block type"))
		assert.Equal(t, true, strings.Contains(body, fmt.Sprintf("could not decode %s request body into consensus block:", version.String(version.Capella))))
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "Beacon node is currently syncing and not serving request on that endpoint"))
	})
}

func TestPublishBlockSSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *zond.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*zond.GenericSignedBeaconBlock_Capella)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var cblock shared.SignedBeaconBlockCapella
		err := json.Unmarshal([]byte(rpctesting.CapellaBlock), &cblock)
		require.NoError(t, err)
		genericBlock, err := cblock.ToGeneric()
		require.NoError(t, err)
		sszvalue, err := genericBlock.GetCapella().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(sszvalue))
		request.Header.Set("Accept", "application/octet-stream")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedCapellaBlock)))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "Body does not represent a valid block type"))
	})
}

func TestPublishBlindedBlock(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Blinded Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *zond.GenericSignedBeaconBlock) bool {
			block, ok := req.Block.(*zond.GenericSignedBeaconBlock_BlindedCapella)
			converted, err := shared.BlindedBeaconBlockCapellaFromConsensus(block.BlindedCapella.Block)
			require.NoError(t, err)
			var signedblock *shared.SignedBlindedBeaconBlockCapella
			err = json.Unmarshal([]byte(rpctesting.BlindedCapellaBlock), &signedblock)
			require.NoError(t, err)
			require.DeepEqual(t, converted, signedblock.Message)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BlindedCapellaBlock)))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.CapellaBlock)))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "please add the api header"))
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "Body does not represent a valid block type"))
	})
	t.Run("invalid block with version header", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.BadBlindedCapellaBlock)))
		request.Header.Set(api.VersionHeader, version.String(version.Capella))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		body := writer.Body.String()
		assert.Equal(t, true, strings.Contains(body, "Body does not represent a valid block type"))
		assert.Equal(t, true, strings.Contains(body, fmt.Sprintf("could not decode %s request body into consensus block:", version.String(version.Capella))))
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &chainMock.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "Beacon node is currently syncing and not serving request on that endpoint"))
	})
}

func TestPublishBlindedBlockSSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Capella", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().ProposeBeaconBlock(gomock.Any(), mock.MatchedBy(func(req *zond.GenericSignedBeaconBlock) bool {
			_, ok := req.Block.(*zond.GenericSignedBeaconBlock_BlindedCapella)
			return ok
		}))
		server := &Server{
			V1Alpha1ValidatorServer: v1alpha1Server,
			SyncChecker:             &mockSync.Sync{IsSyncing: false},
		}

		var cblock shared.SignedBlindedBeaconBlockCapella
		err := json.Unmarshal([]byte(rpctesting.BlindedCapellaBlock), &cblock)
		require.NoError(t, err)
		genericBlock, err := cblock.ToGeneric()
		require.NoError(t, err)
		sszvalue, err := genericBlock.GetBlindedCapella().MarshalSSZ()
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader(sszvalue))
		request.Header.Set("Accept", "application/octet-stream")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
	})
	t.Run("invalid block", func(t *testing.T) {
		server := &Server{
			SyncChecker: &mockSync.Sync{IsSyncing: false},
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte(rpctesting.CapellaBlock)))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.PublishBlindedBlock(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "Body does not represent a valid block type"))
	})
}

func TestValidateConsensus(t *testing.T) {
	ctx := context.Background()

	parentState, privs := util.DeterministicGenesisStateCapella(t, params.MinimalSpecConfig().MinGenesisActiveValidatorCount)
	parentBlock, err := util.GenerateFullBlockCapella(parentState, privs, util.DefaultBlockGenConfig(), parentState.Slot()+1)
	require.NoError(t, err)
	parentSbb, err := blocks.NewSignedBeaconBlock(parentBlock)
	require.NoError(t, err)
	st, err := transition.ExecuteStateTransition(ctx, parentState, parentSbb)
	require.NoError(t, err)
	block, err := util.GenerateFullBlockCapella(st, privs, util.DefaultBlockGenConfig(), st.Slot()+1)
	require.NoError(t, err)
	sbb, err := blocks.NewSignedBeaconBlock(block)
	require.NoError(t, err)
	parentRoot, err := parentSbb.Block().HashTreeRoot()
	require.NoError(t, err)
	server := &Server{
		Blocker: &testutil.MockBlocker{RootBlockMap: map[[32]byte]interfaces.ReadOnlySignedBeaconBlock{parentRoot: parentSbb}},
		Stater:  &testutil.MockStater{StatesByRoot: map[[32]byte]state.BeaconState{bytesutil.ToBytes32(parentBlock.Block.StateRoot): parentState}},
	}

	require.NoError(t, server.validateConsensus(ctx, sbb))
}

func TestValidateEquivocation(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		st, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(10))
		fc := doublylinkedtree.New()
		require.NoError(t, fc.InsertNode(context.Background(), st, bytesutil.ToBytes32([]byte("root"))))
		server := &Server{
			ForkchoiceFetcher: &chainMock.ChainService{ForkChoiceStore: fc},
		}
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		blk.SetSlot(st.Slot() + 1)

		require.NoError(t, server.validateEquivocation(blk.Block()))
	})
	t.Run("block already exists", func(t *testing.T) {
		st, err := util.NewBeaconStateCapella()
		require.NoError(t, err)
		require.NoError(t, st.SetSlot(10))
		fc := doublylinkedtree.New()
		require.NoError(t, fc.InsertNode(context.Background(), st, bytesutil.ToBytes32([]byte("root"))))
		server := &Server{
			ForkchoiceFetcher: &chainMock.ChainService{ForkChoiceStore: fc},
		}
		blk, err := blocks.NewSignedBeaconBlock(util.NewBeaconBlockCapella())
		require.NoError(t, err)
		blk.SetSlot(st.Slot())

		assert.ErrorContains(t, "already exists", server.validateEquivocation(blk.Block()))
	})
}

func TestServer_GetBlockRoot(t *testing.T) {
	beaconDB := dbTest.SetupDB(t)
	ctx := context.Background()

	url := "http://example.com/zond/v1/beacon/blocks/{block_id}/root"
	genBlk, blkContainers := fillDBTestBlocks(ctx, t, beaconDB)
	headBlock := blkContainers[len(blkContainers)-1]
	t.Run("get root", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*zond.BeaconBlockContainer_CapellaBlock).CapellaBlock)
		require.NoError(t, err)

		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &zond.Checkpoint{Root: blkContainers[64].BlockRoot},
			FinalizedRoots:      map[[32]byte]bool{},
		}

		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			HeadFetcher:           mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}

		root, err := genBlk.Block.HashTreeRoot()
		require.NoError(t, err)

		tests := []struct {
			name     string
			blockID  map[string]string
			want     string
			wantErr  string
			wantCode int
		}{
			{
				name:     "bad formatting",
				blockID:  map[string]string{"block_id": "3bad0"},
				wantErr:  "Could not parse block ID",
				wantCode: http.StatusBadRequest,
			},
			{
				name:     "canonical slot",
				blockID:  map[string]string{"block_id": "30"},
				want:     hexutil.Encode(blkContainers[30].BlockRoot),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "head",
				blockID:  map[string]string{"block_id": "head"},
				want:     hexutil.Encode(headBlock.BlockRoot),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "finalized",
				blockID:  map[string]string{"block_id": "finalized"},
				want:     hexutil.Encode(blkContainers[64].BlockRoot),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "genesis",
				blockID:  map[string]string{"block_id": "genesis"},
				want:     hexutil.Encode(root[:]),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "genesis root",
				blockID:  map[string]string{"block_id": hexutil.Encode(root[:])},
				want:     hexutil.Encode(root[:]),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "root",
				blockID:  map[string]string{"block_id": hexutil.Encode(blkContainers[20].BlockRoot)},
				want:     hexutil.Encode(blkContainers[20].BlockRoot),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "non-existent root",
				blockID:  map[string]string{"block_id": hexutil.Encode(bytesutil.PadTo([]byte("hi there"), 32))},
				wantErr:  "Could not find block",
				wantCode: http.StatusNotFound,
			},
			{
				name:     "slot",
				blockID:  map[string]string{"block_id": "40"},
				want:     hexutil.Encode(blkContainers[40].BlockRoot),
				wantErr:  "",
				wantCode: http.StatusOK,
			},
			{
				name:     "no block",
				blockID:  map[string]string{"block_id": "105"},
				wantErr:  "Could not find any blocks with given slot",
				wantCode: http.StatusNotFound,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, url, nil)
				request = mux.SetURLVars(request, tt.blockID)
				writer := httptest.NewRecorder()

				writer.Body = &bytes.Buffer{}

				bs.GetBlockRoot(writer, request)
				assert.Equal(t, tt.wantCode, writer.Code)
				resp := &BlockRootResponse{}
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
				if tt.wantErr != "" {
					require.ErrorContains(t, tt.wantErr, errors.New(writer.Body.String()))
					return
				}
				require.NotNil(t, resp)
				require.DeepEqual(t, resp.Data.Root, tt.want)
			})
		}
	})
	t.Run("execution optimistic", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*zond.BeaconBlockContainer_CapellaBlock).CapellaBlock)
		require.NoError(t, err)

		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &zond.Checkpoint{Root: blkContainers[64].BlockRoot},
			Optimistic:          true,
			FinalizedRoots:      map[[32]byte]bool{},
			OptimisticRoots: map[[32]byte]bool{
				bytesutil.ToBytes32(headBlock.BlockRoot): true,
			},
		}

		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			HeadFetcher:           mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, url, nil)
		request = mux.SetURLVars(request, map[string]string{"block_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		bs.GetBlockRoot(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &BlockRootResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.DeepEqual(t, resp.ExecutionOptimistic, true)
	})
	t.Run("finalized", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*zond.BeaconBlockContainer_CapellaBlock).CapellaBlock)
		require.NoError(t, err)

		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &zond.Checkpoint{Root: blkContainers[64].BlockRoot},
			Optimistic:          true,
			FinalizedRoots: map[[32]byte]bool{
				bytesutil.ToBytes32(blkContainers[32].BlockRoot): true,
				bytesutil.ToBytes32(blkContainers[64].BlockRoot): false,
			},
		}

		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			HeadFetcher:           mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}
		t.Run("true", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, url, nil)
			request = mux.SetURLVars(request, map[string]string{"block_id": "32"})
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			bs.GetBlockRoot(writer, request)
			assert.Equal(t, http.StatusOK, writer.Code)
			resp := &BlockRootResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			require.DeepEqual(t, resp.Finalized, true)
		})
		t.Run("false", func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, url, nil)
			request = mux.SetURLVars(request, map[string]string{"block_id": "64"})
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			bs.GetBlockRoot(writer, request)
			assert.Equal(t, http.StatusOK, writer.Code)
			resp := &BlockRootResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			require.DeepEqual(t, resp.Finalized, false)
		})
	})
}

func TestGetStateFork(t *testing.T) {
	ctx := context.Background()
	request := httptest.NewRequest(http.MethodGet, "http://foo.example/zond/v1/beacon/states/{state_id}/fork", nil)
	request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
	request.Header.Set("Accept", "application/octet-stream")
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	fillFork := func(state *zond.BeaconStateCapella) error {
		state.Fork = &zond.Fork{
			PreviousVersion: []byte("prev"),
			CurrentVersion:  []byte("curr"),
			Epoch:           123,
		}
		return nil
	}
	fakeState, err := util.NewBeaconStateCapella(fillFork)
	require.NoError(t, err)
	db := dbTest.SetupDB(t)

	chainService := &chainMock.ChainService{}
	server := &Server{
		Stater: &testutil.MockStater{
			BeaconState: fakeState,
		},
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
		BeaconDB:              db,
	}

	server.GetStateFork(writer, request)
	require.Equal(t, http.StatusOK, writer.Code)
	var stateForkReponse *GetStateForkResponse
	err = json.Unmarshal(writer.Body.Bytes(), &stateForkReponse)
	require.NoError(t, err)
	expectedFork := fakeState.Fork()
	assert.Equal(t, fmt.Sprint(expectedFork.Epoch), stateForkReponse.Data.Epoch)
	assert.DeepEqual(t, hexutil.Encode(expectedFork.CurrentVersion), stateForkReponse.Data.CurrentVersion)
	assert.DeepEqual(t, hexutil.Encode(expectedFork.PreviousVersion), stateForkReponse.Data.PreviousVersion)
	t.Run("execution optimistic", func(t *testing.T) {
		request = httptest.NewRequest(http.MethodGet, "http://foo.example/zond/v1/beacon/states/{state_id}/fork", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		request.Header.Set("Accept", "application/octet-stream")
		writer = httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		chainService = &chainMock.ChainService{Optimistic: true}
		server = &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}
		server.GetStateFork(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		err = json.Unmarshal(writer.Body.Bytes(), &stateForkReponse)
		require.NoError(t, err)
		assert.DeepEqual(t, true, stateForkReponse.ExecutionOptimistic)
	})

	t.Run("finalized", func(t *testing.T) {
		request = httptest.NewRequest(http.MethodGet, "http://foo.example/zond/v1/beacon/states/{state_id}/fork", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		request.Header.Set("Accept", "application/octet-stream")
		writer = httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		headerRoot, err := fakeState.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService = &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		server = &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}
		server.GetStateFork(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		err = json.Unmarshal(writer.Body.Bytes(), &stateForkReponse)
		require.NoError(t, err)
		assert.DeepEqual(t, true, stateForkReponse.Finalized)
	})
}

func TestGetCommittees(t *testing.T) {
	db := dbTest.SetupDB(t)
	ctx := context.Background()
	url := "http://example.com/zond/v1/beacon/states/{state_id}/committees"

	var st state.BeaconState
	st, _ = util.DeterministicGenesisStateCapella(t, 8192)
	epoch := slots.ToEpoch(st.Slot())

	chainService := &chainMock.ChainService{}
	s := &Server{
		Stater: &testutil.MockStater{
			BeaconState: st,
		},
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
		BeaconDB:              db,
	}

	t.Run("Head all committees", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, url, nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, int(params.BeaconConfig().SlotsPerEpoch), len(resp.Data))
		for _, datum := range resp.Data {
			index, err := strconv.ParseUint(datum.Index, 10, 32)
			require.NoError(t, err)
			slot, err := strconv.ParseUint(datum.Slot, 10, 32)
			require.NoError(t, err)
			assert.Equal(t, true, index == 0 || index == 1)
			assert.Equal(t, epoch, slots.ToEpoch(primitives.Slot(slot)))
		}
	})
	t.Run("Head all committees of epoch 10", func(t *testing.T) {
		query := url + "?epoch=10"
		request := httptest.NewRequest(http.MethodGet, query, nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		for _, datum := range resp.Data {
			slot, err := strconv.ParseUint(datum.Slot, 10, 32)
			require.NoError(t, err)
			assert.Equal(t, true, slot >= 1280 && slot <= 1407)
		}
	})
	t.Run("Head all committees of slot 4", func(t *testing.T) {
		query := url + "?slot=4"
		request := httptest.NewRequest(http.MethodGet, query, nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 1, len(resp.Data))

		exSlot := uint64(4)
		exIndex := uint64(0)
		for _, datum := range resp.Data {
			slot, err := strconv.ParseUint(datum.Slot, 10, 32)
			require.NoError(t, err)
			index, err := strconv.ParseUint(datum.Index, 10, 32)
			require.NoError(t, err)
			assert.Equal(t, epoch, slots.ToEpoch(primitives.Slot(slot)))
			assert.Equal(t, exSlot, slot)
			assert.Equal(t, exIndex, index)
			exIndex++
		}
	})
	t.Run("Head all committees of index 0", func(t *testing.T) {
		query := url + "?index=0"
		request := httptest.NewRequest(http.MethodGet, query, nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, int(params.BeaconConfig().SlotsPerEpoch), len(resp.Data))

		exSlot := uint64(0)
		exIndex := uint64(0)
		for _, datum := range resp.Data {
			slot, err := strconv.ParseUint(datum.Slot, 10, 32)
			require.NoError(t, err)
			index, err := strconv.ParseUint(datum.Index, 10, 32)
			require.NoError(t, err)
			assert.Equal(t, epoch, slots.ToEpoch(primitives.Slot(slot)))
			assert.Equal(t, exSlot, slot)
			assert.Equal(t, exIndex, index)
			exSlot++
		}
	})
	t.Run("Head all committees of slot 2, index 0", func(t *testing.T) {
		query := url + "?slot=2&index=0"
		request := httptest.NewRequest(http.MethodGet, query, nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, 1, len(resp.Data))

		exIndex := uint64(0)
		exSlot := uint64(2)
		for _, datum := range resp.Data {
			index, err := strconv.ParseUint(datum.Index, 10, 32)
			require.NoError(t, err)
			slot, err := strconv.ParseUint(datum.Slot, 10, 32)
			require.NoError(t, err)
			assert.Equal(t, epoch, slots.ToEpoch(primitives.Slot(slot)))
			assert.Equal(t, exSlot, slot)
			assert.Equal(t, exIndex, index)
		}
	})
	t.Run("Execution optimistic", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		chainService = &chainMock.ChainService{Optimistic: true}
		s = &Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}

		request := httptest.NewRequest(http.MethodGet, url, nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("Finalized", func(t *testing.T) {
		parentRoot := [32]byte{'a'}
		blk := util.NewBeaconBlockCapella()
		blk.Block.ParentRoot = parentRoot[:]
		root, err := blk.Block.HashTreeRoot()
		require.NoError(t, err)
		util.SaveBlock(t, ctx, db, blk)
		require.NoError(t, db.SaveGenesisBlockRoot(ctx, root))

		headerRoot, err := st.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService = &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		s = &Server{
			Stater: &testutil.MockStater{
				BeaconState: st,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
			BeaconDB:              db,
		}

		request := httptest.NewRequest(http.MethodGet, url, nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}
		s.GetCommittees(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetCommitteesResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NoError(t, err)
		assert.Equal(t, true, resp.Finalized)
	})
}

func TestGetBlockHeaders(t *testing.T) {
	beaconDB := dbTest.SetupDB(t)
	ctx := context.Background()

	_, blkContainers := fillDBTestBlocks(ctx, t, beaconDB)
	headBlock := blkContainers[len(blkContainers)-1]

	b1 := util.NewBeaconBlockCapella()
	b1.Block.Slot = 30
	b1.Block.ParentRoot = bytesutil.PadTo([]byte{1}, 32)
	util.SaveBlock(t, ctx, beaconDB, b1)
	b2 := util.NewBeaconBlockCapella()
	b2.Block.Slot = 30
	b2.Block.ParentRoot = bytesutil.PadTo([]byte{4}, 32)
	util.SaveBlock(t, ctx, beaconDB, b2)
	b3 := util.NewBeaconBlockCapella()
	b3.Block.Slot = 31
	b3.Block.ParentRoot = bytesutil.PadTo([]byte{1}, 32)
	util.SaveBlock(t, ctx, beaconDB, b3)
	b4 := util.NewBeaconBlockCapella()
	b4.Block.Slot = 28
	b4.Block.ParentRoot = bytesutil.PadTo([]byte{1}, 32)
	util.SaveBlock(t, ctx, beaconDB, b4)

	url := "http://example.com/zond/v1/beacon/headers"

	t.Run("list headers", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*zond.BeaconBlockContainer_CapellaBlock).CapellaBlock)
		require.NoError(t, err)
		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &zond.Checkpoint{Root: blkContainers[64].BlockRoot},
			FinalizedRoots:      map[[32]byte]bool{},
		}
		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}

		tests := []struct {
			name       string
			slot       primitives.Slot
			parentRoot string
			want       []*zond.SignedBeaconBlockCapella
			wantErr    bool
		}{
			{
				name:       "slot",
				slot:       primitives.Slot(30),
				parentRoot: "",
				want: []*zond.SignedBeaconBlockCapella{
					blkContainers[30].Block.(*zond.BeaconBlockContainer_CapellaBlock).CapellaBlock,
					b1,
					b2,
				},
			},
			{
				name:       "parent root",
				parentRoot: hexutil.Encode(b1.Block.ParentRoot),
				want: []*zond.SignedBeaconBlockCapella{
					blkContainers[1].Block.(*zond.BeaconBlockContainer_CapellaBlock).CapellaBlock,
					b1,
					b3,
					b4,
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				urlWithParams := fmt.Sprintf("%s?slot=%d&parent_root=%s", url, tt.slot, tt.parentRoot)
				request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
				writer := httptest.NewRecorder()

				writer.Body = &bytes.Buffer{}

				bs.GetBlockHeaders(writer, request)
				resp := &GetBlockHeadersResponse{}
				require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))

				require.Equal(t, len(tt.want), len(resp.Data))
				for i, blk := range tt.want {
					expectedBodyRoot, err := blk.Block.Body.HashTreeRoot()
					require.NoError(t, err)
					expectedHeader := &zond.BeaconBlockHeader{
						Slot:          blk.Block.Slot,
						ProposerIndex: blk.Block.ProposerIndex,
						ParentRoot:    blk.Block.ParentRoot,
						StateRoot:     make([]byte, 32),
						BodyRoot:      expectedBodyRoot[:],
					}
					expectedHeaderRoot, err := expectedHeader.HashTreeRoot()
					require.NoError(t, err)
					assert.DeepEqual(t, hexutil.Encode(expectedHeaderRoot[:]), resp.Data[i].Root)
					assert.DeepEqual(t, shared.BeaconBlockHeaderFromConsensus(expectedHeader), resp.Data[i].Header.Message)
				}
			})
		}
	})

	t.Run("execution optimistic", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*zond.BeaconBlockContainer_CapellaBlock).CapellaBlock)
		require.NoError(t, err)
		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &zond.Checkpoint{Root: blkContainers[64].BlockRoot},
			Optimistic:          true,
			FinalizedRoots:      map[[32]byte]bool{},
			OptimisticRoots: map[[32]byte]bool{
				bytesutil.ToBytes32(blkContainers[30].BlockRoot): true,
			},
		}
		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}
		slot := primitives.Slot(30)
		urlWithParams := fmt.Sprintf("%s?slot=%d", url, slot)
		request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
		writer := httptest.NewRecorder()

		writer.Body = &bytes.Buffer{}

		bs.GetBlockHeaders(writer, request)
		resp := &GetBlockHeadersResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})

	t.Run("finalized", func(t *testing.T) {
		wsb, err := blocks.NewSignedBeaconBlock(headBlock.Block.(*zond.BeaconBlockContainer_CapellaBlock).CapellaBlock)
		require.NoError(t, err)
		child1 := util.NewBeaconBlockCapella()
		child1.Block.ParentRoot = bytesutil.PadTo([]byte("parent"), 32)
		child1.Block.Slot = 999
		util.SaveBlock(t, ctx, beaconDB, child1)
		child2 := util.NewBeaconBlockCapella()
		child2.Block.ParentRoot = bytesutil.PadTo([]byte("parent"), 32)
		child2.Block.Slot = 1000
		util.SaveBlock(t, ctx, beaconDB, child2)
		child1Root, err := child1.Block.HashTreeRoot()
		require.NoError(t, err)
		child2Root, err := child2.Block.HashTreeRoot()
		require.NoError(t, err)
		mockChainFetcher := &chainMock.ChainService{
			DB:                  beaconDB,
			Block:               wsb,
			Root:                headBlock.BlockRoot,
			FinalizedCheckPoint: &zond.Checkpoint{Root: blkContainers[64].BlockRoot},
			FinalizedRoots:      map[[32]byte]bool{child1Root: true, child2Root: false},
		}
		bs := &Server{
			BeaconDB:              beaconDB,
			ChainInfoFetcher:      mockChainFetcher,
			OptimisticModeFetcher: mockChainFetcher,
			FinalizationFetcher:   mockChainFetcher,
		}

		t.Run("true", func(t *testing.T) {
			slot := primitives.Slot(999)
			urlWithParams := fmt.Sprintf("%s?slot=%d", url, slot)
			request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
			writer := httptest.NewRecorder()

			writer.Body = &bytes.Buffer{}

			bs.GetBlockHeaders(writer, request)
			resp := &GetBlockHeadersResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, true, resp.Finalized)
		})
		t.Run("false", func(t *testing.T) {
			slot := primitives.Slot(1000)
			urlWithParams := fmt.Sprintf("%s?slot=%d", url, slot)
			request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
			writer := httptest.NewRecorder()

			writer.Body = &bytes.Buffer{}

			bs.GetBlockHeaders(writer, request)
			resp := &GetBlockHeadersResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, false, resp.Finalized)
		})
		t.Run("false when at least one not finalized", func(t *testing.T) {
			urlWithParams := fmt.Sprintf("%s?parent_root=%s", url, hexutil.Encode(child1.Block.ParentRoot))
			request := httptest.NewRequest(http.MethodGet, urlWithParams, nil)
			writer := httptest.NewRecorder()

			writer.Body = &bytes.Buffer{}

			bs.GetBlockHeaders(writer, request)
			resp := &GetBlockHeadersResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, false, resp.Finalized)
		})
	})
}

func TestServer_GetBlockHeader(t *testing.T) {
	b := util.NewBeaconBlockCapella()
	b.Block.Slot = 123
	b.Block.ProposerIndex = 123
	b.Block.StateRoot = bytesutil.PadTo([]byte("stateroot"), 32)
	b.Block.ParentRoot = bytesutil.PadTo([]byte("parentroot"), 32)
	b.Block.Body.Graffiti = bytesutil.PadTo([]byte("graffiti"), 32)
	sb, err := blocks.NewSignedBeaconBlock(b)
	sb.SetSignature(bytesutil.PadTo([]byte("sig"), 96))
	require.NoError(t, err)

	mockBlockFetcher := &testutil.MockBlocker{BlockToReturn: sb}
	mockChainService := &chainMock.ChainService{
		FinalizedRoots: map[[32]byte]bool{},
	}
	s := &Server{
		ChainInfoFetcher:      mockChainService,
		OptimisticModeFetcher: mockChainService,
		FinalizationFetcher:   mockChainService,
		Blocker:               mockBlockFetcher,
	}

	t.Run("ok", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/headers/{block_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"block_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockHeader(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &GetBlockHeaderResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.Data.Canonical)
		assert.Equal(t, "0x96e90099abc28aa88b8d9c62163cb581e3faf5e9fae8dd1e5e0a4a12f2ba9f34", resp.Data.Root)
		assert.Equal(t, "0x7369670000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", resp.Data.Header.Signature)
		assert.Equal(t, "123", resp.Data.Header.Message.Slot)
		assert.Equal(t, "0x706172656e74726f6f7400000000000000000000000000000000000000000000", resp.Data.Header.Message.ParentRoot)
		assert.Equal(t, "123", resp.Data.Header.Message.ProposerIndex)
		assert.Equal(t, "0x41401068b64f95fb332bc48c7c21cadd133d91c4bbf8acaaffdcab89fd4aadfe", resp.Data.Header.Message.BodyRoot)
		assert.Equal(t, "0x7374617465726f6f740000000000000000000000000000000000000000000000", resp.Data.Header.Message.StateRoot)
	})
	t.Run("missing block_id", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/headers/{block_id}", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockHeader(writer, request)
		require.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "block_id is required in URL params", e.Message)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)
		mockChainService := &chainMock.ChainService{
			OptimisticRoots: map[[32]byte]bool{r: true},
			FinalizedRoots:  map[[32]byte]bool{},
		}
		s := &Server{
			ChainInfoFetcher:      mockChainService,
			OptimisticModeFetcher: mockChainService,
			FinalizationFetcher:   mockChainService,
			Blocker:               mockBlockFetcher,
		}

		request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/headers/{block_id}", nil)
		request = mux.SetURLVars(request, map[string]string{"block_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetBlockHeader(writer, request)
		require.Equal(t, http.StatusOK, writer.Code)
		resp := &GetBlockHeaderResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		r, err := sb.Block().HashTreeRoot()
		require.NoError(t, err)

		t.Run("true", func(t *testing.T) {
			mockChainService := &chainMock.ChainService{FinalizedRoots: map[[32]byte]bool{r: true}}
			s := &Server{
				ChainInfoFetcher:      mockChainService,
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/headers/{block_id}", nil)
			request = mux.SetURLVars(request, map[string]string{"block_id": hexutil.Encode(r[:])})
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockHeader(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &GetBlockHeaderResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, true, resp.Finalized)
		})
		t.Run("false", func(t *testing.T) {
			mockChainService := &chainMock.ChainService{FinalizedRoots: map[[32]byte]bool{r: false}}
			s := &Server{
				ChainInfoFetcher:      mockChainService,
				OptimisticModeFetcher: mockChainService,
				FinalizationFetcher:   mockChainService,
				Blocker:               mockBlockFetcher,
			}

			request := httptest.NewRequest(http.MethodGet, "http://example.com/zond/v1/beacon/headers/{block_id}", nil)
			request = mux.SetURLVars(request, map[string]string{"block_id": hexutil.Encode(r[:])})
			writer := httptest.NewRecorder()
			writer.Body = &bytes.Buffer{}

			s.GetBlockHeader(writer, request)
			require.Equal(t, http.StatusOK, writer.Code)
			resp := &GetBlockHeaderResponse{}
			require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
			assert.Equal(t, false, resp.Finalized)
		})
	})
}

func TestGetFinalityCheckpoints(t *testing.T) {
	fillCheckpoints := func(state *zond.BeaconStateCapella) error {
		state.PreviousJustifiedCheckpoint = &zond.Checkpoint{
			Root:  bytesutil.PadTo([]byte("previous"), 32),
			Epoch: 113,
		}
		state.CurrentJustifiedCheckpoint = &zond.Checkpoint{
			Root:  bytesutil.PadTo([]byte("current"), 32),
			Epoch: 123,
		}
		state.FinalizedCheckpoint = &zond.Checkpoint{
			Root:  bytesutil.PadTo([]byte("finalized"), 32),
			Epoch: 103,
		}
		return nil
	}
	fakeState, err := util.NewBeaconStateCapella(fillCheckpoints)
	require.NoError(t, err)

	chainService := &chainMock.ChainService{}
	s := &Server{
		Stater: &testutil.MockStater{
			BeaconState: fakeState,
		},
		HeadFetcher:           chainService,
		OptimisticModeFetcher: chainService,
		FinalizationFetcher:   chainService,
	}

	t.Run("ok", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/zond/v1/beacon/states/{state_id}/finality_checkpoints", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetFinalityCheckpoints(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetFinalityCheckpointsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)
		assert.Equal(t, strconv.FormatUint(uint64(fakeState.FinalizedCheckpoint().Epoch), 10), resp.Data.Finalized.Epoch)
		assert.DeepEqual(t, hexutil.Encode(fakeState.FinalizedCheckpoint().Root), resp.Data.Finalized.Root)
		assert.Equal(t, strconv.FormatUint(uint64(fakeState.CurrentJustifiedCheckpoint().Epoch), 10), resp.Data.CurrentJustified.Epoch)
		assert.DeepEqual(t, hexutil.Encode(fakeState.CurrentJustifiedCheckpoint().Root), resp.Data.CurrentJustified.Root)
		assert.Equal(t, strconv.FormatUint(uint64(fakeState.PreviousJustifiedCheckpoint().Epoch), 10), resp.Data.PreviousJustified.Epoch)
		assert.DeepEqual(t, hexutil.Encode(fakeState.PreviousJustifiedCheckpoint().Root), resp.Data.PreviousJustified.Root)
	})
	t.Run("no state_id", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/zond/v1/beacon/states/{state_id}/finality_checkpoints", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetFinalityCheckpoints(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusBadRequest, e.Code)
		assert.StringContains(t, "state_id is required in URL params", e.Message)
	})
	t.Run("execution optimistic", func(t *testing.T) {
		chainService := &chainMock.ChainService{Optimistic: true}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "/zond/v1/beacon/states/{state_id}/finality_checkpoints", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetFinalityCheckpoints(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetFinalityCheckpointsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.ExecutionOptimistic)
	})
	t.Run("finalized", func(t *testing.T) {
		headerRoot, err := fakeState.LatestBlockHeader().HashTreeRoot()
		require.NoError(t, err)
		chainService := &chainMock.ChainService{
			FinalizedRoots: map[[32]byte]bool{
				headerRoot: true,
			},
		}
		s := &Server{
			Stater: &testutil.MockStater{
				BeaconState: fakeState,
			},
			HeadFetcher:           chainService,
			OptimisticModeFetcher: chainService,
			FinalizationFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "/zond/v1/beacon/states/{state_id}/finality_checkpoints", nil)
		request = mux.SetURLVars(request, map[string]string{"state_id": "head"})
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetFinalityCheckpoints(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetFinalityCheckpointsResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		assert.Equal(t, true, resp.Finalized)
	})
}

func TestGetGenesis(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.GenesisForkVersion = []byte("genesis")
	params.OverrideBeaconConfig(config)
	genesis := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	validatorsRoot := [32]byte{1, 2, 3, 4, 5, 6}

	t.Run("ok", func(t *testing.T) {
		chainService := &chainMock.ChainService{
			Genesis:        genesis,
			ValidatorsRoot: validatorsRoot,
		}
		s := Server{
			GenesisTimeFetcher: chainService,
			ChainInfoFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "/zond/v1/beacon/genesis", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetGenesis(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		resp := &GetGenesisResponse{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), resp))
		require.NotNil(t, resp.Data)

		assert.Equal(t, strconv.FormatInt(genesis.Unix(), 10), resp.Data.GenesisTime)
		assert.DeepEqual(t, hexutil.Encode(validatorsRoot[:]), resp.Data.GenesisValidatorsRoot)
		assert.DeepEqual(t, hexutil.Encode([]byte("genesis")), resp.Data.GenesisForkVersion)
	})
	t.Run("no genesis time", func(t *testing.T) {
		chainService := &chainMock.ChainService{
			Genesis:        time.Time{},
			ValidatorsRoot: validatorsRoot,
		}
		s := Server{
			GenesisTimeFetcher: chainService,
			ChainInfoFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "/zond/v1/beacon/genesis", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetGenesis(writer, request)
		assert.Equal(t, http.StatusNotFound, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusNotFound, e.Code)
		assert.StringContains(t, "Chain genesis info is not yet known", e.Message)
	})
	t.Run("no genesis validators root", func(t *testing.T) {
		chainService := &chainMock.ChainService{
			Genesis:        genesis,
			ValidatorsRoot: [32]byte{},
		}
		s := Server{
			GenesisTimeFetcher: chainService,
			ChainInfoFetcher:   chainService,
		}

		request := httptest.NewRequest(http.MethodGet, "/zond/v1/beacon/genesis", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		s.GetGenesis(writer, request)
		assert.Equal(t, http.StatusNotFound, writer.Code)
		e := &http2.DefaultErrorJson{}
		require.NoError(t, json.Unmarshal(writer.Body.Bytes(), e))
		assert.Equal(t, http.StatusNotFound, e.Code)
		assert.StringContains(t, "Chain genesis info is not yet known", e.Message)
	})
}

func TestGetDepositContract(t *testing.T) {
	params.SetupTestConfigCleanup(t)
	config := params.BeaconConfig().Copy()
	config.DepositChainID = uint64(10)
	config.DepositContractAddress = "0x4242424242424242424242424242424242424242"
	params.OverrideBeaconConfig(config)

	request := httptest.NewRequest(http.MethodGet, "/zond/v1/beacon/states/{state_id}/finality_checkpoints", nil)
	writer := httptest.NewRecorder()
	writer.Body = &bytes.Buffer{}

	s := &Server{}
	s.GetDepositContract(writer, request)
	assert.Equal(t, http.StatusOK, writer.Code)
	response := DepositContractResponse{}
	require.NoError(t, json.Unmarshal(writer.Body.Bytes(), &response))
	assert.Equal(t, "10", response.Data.ChainId)
	assert.Equal(t, "0x4242424242424242424242424242424242424242", response.Data.Address)
}
