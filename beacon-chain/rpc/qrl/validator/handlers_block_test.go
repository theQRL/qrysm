package validator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/theQRL/qrysm/api"
	blockchainTesting "github.com/theQRL/qrysm/beacon-chain/blockchain/testing"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/shared"
	rpctesting "github.com/theQRL/qrysm/beacon-chain/rpc/qrl/shared/testing"
	mockSync "github.com/theQRL/qrysm/beacon-chain/sync/initial-sync/testing"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/assert"
	mock2 "github.com/theQRL/qrysm/testing/mock"
	"github.com/theQRL/qrysm/testing/require"
)

func TestProduceBlockV3(t *testing.T) {
	ctrl := gomock.NewController(t)

	t.Run("Zond", func(t *testing.T) {
		var block *shared.SignedBeaconBlockZond
		err := json.Unmarshal([]byte(rpctesting.ZondBlock), &block)
		require.NoError(t, err)
		jsonBytes, err := json.Marshal(block.Message)
		require.NoError(t, err)
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(
			func() (*qrysmpb.GenericBeaconBlock, error) {
				return block.Message.ToGeneric()
			}())
		mockChainService := &blockchainTesting.ChainService{}
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: mockChainService,
		}
		rr := "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc9251" +
			"&graffiti=0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
		request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v3/validator/blocks/1?randao_reveal=%s", rr), nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.ProduceBlockV3(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		want := fmt.Sprintf(`{"version":"zond","execution_payload_blinded":false,"execution_payload_value":"0","data":%s}`, string(jsonBytes))
		body := strings.ReplaceAll(writer.Body.String(), "\n", "")
		require.Equal(t, want, body)
		require.Equal(t, writer.Header().Get(api.ExecutionPayloadBlindedHeader) == "false", true)
		require.Equal(t, writer.Header().Get(api.ExecutionPayloadValueHeader) == "0", true)
	})
	t.Run("Blinded Zond", func(t *testing.T) {
		var block *shared.SignedBlindedBeaconBlockZond
		err := json.Unmarshal([]byte(rpctesting.BlindedZondBlock), &block)
		require.NoError(t, err)
		jsonBytes, err := json.Marshal(block.Message)
		require.NoError(t, err)
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(
			func() (*qrysmpb.GenericBeaconBlock, error) {
				g, err := block.Message.ToGeneric()
				require.NoError(t, err)
				g.PayloadValue = 2000 //some fake value
				return g, err
			}())
		mockChainService := &blockchainTesting.ChainService{}
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: mockChainService,
		}
		rr := "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc9251" +
			"&graffiti=0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
		request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v3/validator/blocks/1?randao_reveal=%s", rr), nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.ProduceBlockV3(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		want := fmt.Sprintf(`{"version":"zond","execution_payload_blinded":true,"execution_payload_value":"2000","data":%s}`, string(jsonBytes))
		body := strings.ReplaceAll(writer.Body.String(), "\n", "")
		require.Equal(t, want, body)
		require.Equal(t, writer.Header().Get(api.ExecutionPayloadBlindedHeader) == "true", true)
		require.Equal(t, writer.Header().Get(api.ExecutionPayloadValueHeader) == "2000", true)
	})
	t.Run("invalid query parameter slot empty", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
		}
		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v3/validator/blocks/", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.ProduceBlockV3(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "slot is required"))
	})
	t.Run("invalid query parameter slot invalid", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
		}
		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v3/validator/blocks/asdfsad", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.ProduceBlockV3(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "slot is invalid"))
	})
	t.Run("invalid query parameter randao_reveal invalid", func(t *testing.T) {
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		server := &Server{
			V1Alpha1Server: v1alpha1Server,
			SyncChecker:    &mockSync.Sync{IsSyncing: false},
		}
		request := httptest.NewRequest(http.MethodGet, "http://foo.example/eth/v3/validator/blocks/1?randao_reveal=0x213123", nil)
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.ProduceBlockV3(writer, request)
		assert.Equal(t, http.StatusBadRequest, writer.Code)
	})
	t.Run("syncing", func(t *testing.T) {
		chainService := &blockchainTesting.ChainService{}
		server := &Server{
			SyncChecker:           &mockSync.Sync{IsSyncing: true},
			HeadFetcher:           chainService,
			TimeFetcher:           chainService,
			OptimisticModeFetcher: chainService,
		}

		request := httptest.NewRequest(http.MethodPost, "http://foo.example", bytes.NewReader([]byte("foo")))
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.ProduceBlockV3(writer, request)
		assert.Equal(t, http.StatusServiceUnavailable, writer.Code)
		assert.Equal(t, true, strings.Contains(writer.Body.String(), "Beacon node is currently syncing and not serving request on that endpoint"))
	})
}

func TestProduceBlockV3SSZ(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Run("Zond", func(t *testing.T) {
		var block *shared.SignedBeaconBlockZond
		err := json.Unmarshal([]byte(rpctesting.ZondBlock), &block)
		require.NoError(t, err)
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(
			func() (*qrysmpb.GenericBeaconBlock, error) {
				return block.Message.ToGeneric()
			}())
		mockChainService := &blockchainTesting.ChainService{}
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: mockChainService,
		}
		rr := "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc9251" +
			"&graffiti=0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
		request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v3/validator/blocks/1?randao_reveal=%s", rr), nil)
		request.Header.Set("Accept", "application/octet-stream")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.ProduceBlockV3(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		g, err := block.ToGeneric()
		require.NoError(t, err)
		bl, ok := g.Block.(*qrysmpb.GenericSignedBeaconBlock_Zond)
		require.Equal(t, true, ok)
		ssz, err := bl.Zond.Block.MarshalSSZ()
		require.NoError(t, err)
		require.Equal(t, string(ssz), writer.Body.String())
		require.Equal(t, writer.Header().Get(api.ExecutionPayloadBlindedHeader) == "false", true)
		require.Equal(t, writer.Header().Get(api.ExecutionPayloadValueHeader) == "0", true)
	})
	t.Run("Blinded Zond", func(t *testing.T) {
		var block *shared.SignedBlindedBeaconBlockZond
		err := json.Unmarshal([]byte(rpctesting.BlindedZondBlock), &block)
		require.NoError(t, err)
		v1alpha1Server := mock2.NewMockBeaconNodeValidatorServer(ctrl)
		v1alpha1Server.EXPECT().GetBeaconBlock(gomock.Any(), gomock.Any()).Return(
			func() (*qrysmpb.GenericBeaconBlock, error) {
				g, err := block.Message.ToGeneric()
				require.NoError(t, err)
				g.PayloadValue = 2000 //some fake value
				return g, err
			}())
		mockChainService := &blockchainTesting.ChainService{}
		server := &Server{
			V1Alpha1Server:        v1alpha1Server,
			SyncChecker:           &mockSync.Sync{IsSyncing: false},
			OptimisticModeFetcher: mockChainService,
		}
		rr := "0x23b7f3eb8de34c192a57e259a4857a62a1f5308f949fb0de8a6ae80934bc9251" +
			"&graffiti=0xcf8e0d4e9587369b2301d0790347320302cc0943d5a1884560367e8208d920f2"
		request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("http://foo.example/eth/v3/validator/blocks/1?randao_reveal=%s", rr), nil)
		request.Header.Set("Accept", "application/octet-stream")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}
		server.ProduceBlockV3(writer, request)
		assert.Equal(t, http.StatusOK, writer.Code)
		g, err := block.ToGeneric()
		require.NoError(t, err)
		bl, ok := g.Block.(*qrysmpb.GenericSignedBeaconBlock_BlindedZond)
		require.Equal(t, true, ok)
		ssz, err := bl.BlindedZond.Block.MarshalSSZ()
		require.NoError(t, err)
		require.Equal(t, string(ssz), writer.Body.String())
		require.Equal(t, writer.Header().Get(api.ExecutionPayloadBlindedHeader) == "true", true)
		require.Equal(t, writer.Header().Get(api.ExecutionPayloadValueHeader) == "2000", true)
	})
}
