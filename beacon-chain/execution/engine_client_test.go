package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pkg/errors"
	zond "github.com/theQRL/go-zond"
	"github.com/theQRL/go-zond/common"
	"github.com/theQRL/go-zond/common/hexutil"
	zondtypes "github.com/theQRL/go-zond/core/types"
	"github.com/theQRL/go-zond/rpc"
	zondRPC "github.com/theQRL/go-zond/rpc"
	mocks "github.com/theQRL/qrysm/v4/beacon-chain/execution/testing"
	"github.com/theQRL/qrysm/v4/config/features"
	fieldparams "github.com/theQRL/qrysm/v4/config/fieldparams"
	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/consensus-types/blocks"
	"github.com/theQRL/qrysm/v4/consensus-types/interfaces"
	payloadattribute "github.com/theQRL/qrysm/v4/consensus-types/payload-attribute"
	"github.com/theQRL/qrysm/v4/encoding/bytesutil"
	enginev1 "github.com/theQRL/qrysm/v4/proto/engine/v1"
	pb "github.com/theQRL/qrysm/v4/proto/engine/v1"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/assert"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

var (
	_ = ExecutionPayloadReconstructor(&Service{})
	_ = EngineCaller(&Service{})
	_ = ExecutionPayloadReconstructor(&Service{})
	_ = EngineCaller(&mocks.EngineClient{})
)

type RPCClientBad struct {
}

func (RPCClientBad) Close() {}
func (RPCClientBad) BatchCall([]zondRPC.BatchElem) error {
	return errors.New("rpc client is not initialized")
}

func (RPCClientBad) CallContext(context.Context, interface{}, string, ...interface{}) error {
	return zond.NotFound
}

func TestClient_IPC(t *testing.T) {
	t.Skip("Skipping IPC test to support Capella devnet-3")
	server := newTestIPCServer(t)
	defer server.Stop()
	rpcClient := rpc.DialInProc(server)
	defer rpcClient.Close()
	srv := &Service{}
	srv.rpcClient = rpcClient
	ctx := context.Background()
	fix := fixtures()

	params.SetupTestConfigCleanup(t)

	t.Run(GetPayloadMethodV2, func(t *testing.T) {
		want, ok := fix["ExecutionPayloadCapellaWithValue"].(*pb.ExecutionPayloadCapellaWithValue)
		require.Equal(t, true, ok)
		payloadId := [8]byte{1}
		resp, override, err := srv.GetPayload(ctx, payloadId, params.BeaconConfig().SlotsPerEpoch)
		require.NoError(t, err)
		require.Equal(t, false, override)
		resPb, err := resp.PbCapella()
		require.NoError(t, err)
		require.DeepEqual(t, want, resPb)
	})
	t.Run(ForkchoiceUpdatedMethodV2, func(t *testing.T) {
		want, ok := fix["ForkchoiceUpdatedResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		p, err := payloadattribute.New(&pb.PayloadAttributesV2{})
		require.NoError(t, err)
		payloadID, validHash, err := srv.ForkchoiceUpdated(ctx, &pb.ForkchoiceState{}, p)
		require.NoError(t, err)
		require.DeepEqual(t, want.Status.LatestValidHash, validHash)
		require.DeepEqual(t, want.PayloadId, payloadID)
	})
	t.Run(NewPayloadMethodV2, func(t *testing.T) {
		want, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		req, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(req, 0)
		require.NoError(t, err)
		latestValidHash, err := srv.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{})
		require.NoError(t, err)
		require.DeepEqual(t, bytesutil.ToBytes32(want.LatestValidHash), bytesutil.ToBytes32(latestValidHash))
	})
	t.Run(ExecutionBlockByNumberMethod, func(t *testing.T) {
		want, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
		require.Equal(t, true, ok)
		resp, err := srv.LatestExecutionBlock(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, want, resp)
	})
	t.Run(ExecutionBlockByHashMethod, func(t *testing.T) {
		want, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
		require.Equal(t, true, ok)
		arg := common.BytesToHash([]byte("foo"))
		resp, err := srv.ExecutionBlockByHash(ctx, arg, true /* with txs */)
		require.NoError(t, err)
		require.DeepEqual(t, want, resp)
	})
}

func TestClient_HTTP(t *testing.T) {
	ctx := context.Background()
	fix := fixtures()

	params.SetupTestConfigCleanup(t)

	t.Run(GetPayloadMethodV2, func(t *testing.T) {
		payloadId := [8]byte{1}
		want, ok := fix["ExecutionPayloadCapellaWithValue"].(*pb.GetPayloadV2ResponseJson)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			enc, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			jsonRequestString := string(enc)

			reqArg, err := json.Marshal(pb.PayloadIDBytes(payloadId))
			require.NoError(t, err)

			// We expect the JSON string RPC request contains the right arguments.
			require.Equal(t, true, strings.Contains(
				jsonRequestString, string(reqArg),
			))
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		client := &Service{}
		client.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, override, err := client.GetPayload(ctx, payloadId, params.BeaconConfig().SlotsPerEpoch)
		require.NoError(t, err)
		require.Equal(t, false, override)
		pb, err := resp.PbCapella()
		require.NoError(t, err)
		require.DeepEqual(t, want.ExecutionPayload.BlockHash.Bytes(), pb.BlockHash)
		require.DeepEqual(t, want.ExecutionPayload.StateRoot.Bytes(), pb.StateRoot)
		require.DeepEqual(t, want.ExecutionPayload.ParentHash.Bytes(), pb.ParentHash)
		require.DeepEqual(t, want.ExecutionPayload.FeeRecipient.Bytes(), pb.FeeRecipient)
		require.DeepEqual(t, want.ExecutionPayload.PrevRandao.Bytes(), pb.PrevRandao)
		require.DeepEqual(t, want.ExecutionPayload.ParentHash.Bytes(), pb.ParentHash)

		v, err := resp.ValueInGwei()
		require.NoError(t, err)
		require.Equal(t, uint64(1236), v)
	})
	t.Run(ForkchoiceUpdatedMethodV2+" VALID status", func(t *testing.T) {
		forkChoiceState := &pb.ForkchoiceState{
			HeadBlockHash:      []byte("head"),
			SafeBlockHash:      []byte("safe"),
			FinalizedBlockHash: []byte("finalized"),
		}
		payloadAttributes := &pb.PayloadAttributesV2{
			Timestamp:             1,
			PrevRandao:            []byte("random"),
			SuggestedFeeRecipient: []byte("suggestedFeeRecipient"),
			Withdrawals:           []*pb.Withdrawal{{ValidatorIndex: 1, Amount: 1}},
		}
		p, err := payloadattribute.New(payloadAttributes)
		require.NoError(t, err)
		want, ok := fix["ForkchoiceUpdatedResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		srv := forkchoiceUpdateSetupV2(t, forkChoiceState, payloadAttributes, want)

		// We call the RPC method via HTTP and expect a proper result.
		payloadID, validHash, err := srv.ForkchoiceUpdated(ctx, forkChoiceState, p)
		require.NoError(t, err)
		require.DeepEqual(t, want.Status.LatestValidHash, validHash)
		require.DeepEqual(t, want.PayloadId, payloadID)
	})
	t.Run(ForkchoiceUpdatedMethodV2+" SYNCING status", func(t *testing.T) {
		forkChoiceState := &pb.ForkchoiceState{
			HeadBlockHash:      []byte("head"),
			SafeBlockHash:      []byte("safe"),
			FinalizedBlockHash: []byte("finalized"),
		}
		payloadAttributes := &pb.PayloadAttributesV2{
			Timestamp:             1,
			PrevRandao:            []byte("random"),
			SuggestedFeeRecipient: []byte("suggestedFeeRecipient"),
			Withdrawals:           []*pb.Withdrawal{{ValidatorIndex: 1, Amount: 1}},
		}
		p, err := payloadattribute.New(payloadAttributes)
		require.NoError(t, err)
		want, ok := fix["ForkchoiceUpdatedSyncingResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		srv := forkchoiceUpdateSetupV2(t, forkChoiceState, payloadAttributes, want)

		// We call the RPC method via HTTP and expect a proper result.
		payloadID, validHash, err := srv.ForkchoiceUpdated(ctx, forkChoiceState, p)
		require.ErrorIs(t, err, ErrAcceptedSyncingPayloadStatus)
		require.DeepEqual(t, (*pb.PayloadIDBytes)(nil), payloadID)
		require.DeepEqual(t, []byte(nil), validHash)
	})
	t.Run(ForkchoiceUpdatedMethodV2+" INVALID status", func(t *testing.T) {
		forkChoiceState := &pb.ForkchoiceState{
			HeadBlockHash:      []byte("head"),
			SafeBlockHash:      []byte("safe"),
			FinalizedBlockHash: []byte("finalized"),
		}
		payloadAttributes := &pb.PayloadAttributesV2{
			Timestamp:             1,
			PrevRandao:            []byte("random"),
			SuggestedFeeRecipient: []byte("suggestedFeeRecipient"),
		}
		p, err := payloadattribute.New(payloadAttributes)
		require.NoError(t, err)
		want, ok := fix["ForkchoiceUpdatedInvalidResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		client := forkchoiceUpdateSetupV2(t, forkChoiceState, payloadAttributes, want)

		// We call the RPC method via HTTP and expect a proper result.
		payloadID, validHash, err := client.ForkchoiceUpdated(ctx, forkChoiceState, p)
		require.ErrorIs(t, err, ErrInvalidPayloadStatus)
		require.DeepEqual(t, (*pb.PayloadIDBytes)(nil), payloadID)
		require.DeepEqual(t, want.Status.LatestValidHash, validHash)
	})
	t.Run(ForkchoiceUpdatedMethodV2+" UNKNOWN status", func(t *testing.T) {
		forkChoiceState := &pb.ForkchoiceState{
			HeadBlockHash:      []byte("head"),
			SafeBlockHash:      []byte("safe"),
			FinalizedBlockHash: []byte("finalized"),
		}
		payloadAttributes := &pb.PayloadAttributesV2{
			Timestamp:             1,
			PrevRandao:            []byte("random"),
			SuggestedFeeRecipient: []byte("suggestedFeeRecipient"),
		}
		p, err := payloadattribute.New(payloadAttributes)
		require.NoError(t, err)
		want, ok := fix["ForkchoiceUpdatedAcceptedResponse"].(*ForkchoiceUpdatedResponse)
		require.Equal(t, true, ok)
		client := forkchoiceUpdateSetupV2(t, forkChoiceState, payloadAttributes, want)

		// We call the RPC method via HTTP and expect a proper result.
		payloadID, validHash, err := client.ForkchoiceUpdated(ctx, forkChoiceState, p)
		require.ErrorIs(t, err, ErrUnknownPayloadStatus)
		require.DeepEqual(t, (*pb.PayloadIDBytes)(nil), payloadID)
		require.DeepEqual(t, []byte(nil), validHash)
	})
	t.Run(NewPayloadMethodV2+" VALID status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		want, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV2Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(execPayload, 0)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{})
		require.NoError(t, err)
		require.DeepEqual(t, want.LatestValidHash, resp)
	})
	t.Run(NewPayloadMethodV2+" SYNCING status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		want, ok := fix["SyncingStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV2Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(execPayload, 0)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{})
		require.ErrorIs(t, ErrAcceptedSyncingPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(NewPayloadMethodV2+" INVALID_BLOCK_HASH status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		want, ok := fix["InvalidBlockHashStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV2Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(execPayload, 0)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{})
		require.ErrorIs(t, ErrInvalidBlockHashPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(NewPayloadMethodV2+" INVALID status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		want, ok := fix["InvalidStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV2Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(execPayload, 0)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{})
		require.ErrorIs(t, ErrInvalidPayloadStatus, err)
		require.DeepEqual(t, want.LatestValidHash, resp)
	})
	t.Run(NewPayloadMethodV2+" UNKNOWN status", func(t *testing.T) {
		execPayload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)
		want, ok := fix["UnknownStatus"].(*pb.PayloadStatus)
		require.Equal(t, true, ok)
		client := newPayloadV2Setup(t, want, execPayload)

		// We call the RPC method via HTTP and expect a proper result.
		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(execPayload, 0)
		require.NoError(t, err)
		resp, err := client.NewPayload(ctx, wrappedPayload, []common.Hash{}, &common.Hash{})
		require.ErrorIs(t, ErrUnknownPayloadStatus, err)
		require.DeepEqual(t, []uint8(nil), resp)
	})
	t.Run(ExecutionBlockByNumberMethod, func(t *testing.T) {
		want, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, err := service.LatestExecutionBlock(ctx)
		require.NoError(t, err)
		require.DeepEqual(t, want, resp)
	})
	t.Run(ExecutionBlockByHashMethod, func(t *testing.T) {
		arg := common.BytesToHash([]byte("foo"))
		want, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
		require.Equal(t, true, ok)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			enc, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			jsonRequestString := string(enc)
			// We expect the JSON string RPC request contains the right arguments.
			require.Equal(t, true, strings.Contains(
				jsonRequestString, fmt.Sprintf("%#x", arg),
			))
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  want,
			}
			err = json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		defer srv.Close()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient

		// We call the RPC method via HTTP and expect a proper result.
		resp, err := service.ExecutionBlockByHash(ctx, arg, true /* with txs */)
		require.NoError(t, err)
		require.DeepEqual(t, want, resp)
	})
}

func TestReconstructFullBlock(t *testing.T) {
	ctx := context.Background()
	t.Run("nil block", func(t *testing.T) {
		service := &Service{}

		_, err := service.ReconstructFullBlock(ctx, nil)
		require.ErrorContains(t, "nil data", err)
	})
	t.Run("only blinded block", func(t *testing.T) {
		want := "can only reconstruct block from blinded block format"
		service := &Service{}
		bellatrixBlock := util.NewBeaconBlockCapella()
		wrapped, err := blocks.NewSignedBeaconBlock(bellatrixBlock)
		require.NoError(t, err)
		_, err = service.ReconstructFullBlock(ctx, wrapped)
		require.ErrorContains(t, want, err)
	})
	t.Run("properly reconstructs block with correct payload", func(t *testing.T) {
		fix := fixtures()
		payload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)

		jsonPayload := make(map[string]interface{})

		to := common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87")
		tx := zondtypes.NewTx(&zondtypes.DynamicFeeTx{
			Nonce: 0,
			To:    &to,
			Value: big.NewInt(0),
			Data:  nil,
		})
		txs := []*zondtypes.Transaction{tx}
		encodedBinaryTxs := make([][]byte, 1)
		var err error
		encodedBinaryTxs[0], err = txs[0].MarshalBinary()
		require.NoError(t, err)
		payload.Transactions = encodedBinaryTxs
		payload.Withdrawals = make([]*enginev1.Withdrawal, 0)
		jsonPayload["transactions"] = txs
		num := big.NewInt(1)
		encodedNum := hexutil.EncodeBig(num)
		jsonPayload["hash"] = hexutil.Encode(payload.BlockHash)
		jsonPayload["parentHash"] = common.BytesToHash([]byte("parent"))
		jsonPayload["sha3Uncles"] = common.BytesToHash([]byte("uncles"))
		jsonPayload["miner"] = common.BytesToAddress([]byte("miner"))
		jsonPayload["stateRoot"] = common.BytesToHash([]byte("state"))
		jsonPayload["transactionsRoot"] = common.BytesToHash([]byte("txs"))
		jsonPayload["receiptsRoot"] = common.BytesToHash([]byte("receipts"))
		jsonPayload["logsBloom"] = zondtypes.BytesToBloom([]byte("bloom"))
		jsonPayload["gasLimit"] = hexutil.EncodeUint64(1)
		jsonPayload["gasUsed"] = hexutil.EncodeUint64(2)
		jsonPayload["timestamp"] = hexutil.EncodeUint64(3)
		jsonPayload["number"] = encodedNum
		jsonPayload["extraData"] = common.BytesToHash([]byte("extra"))
		jsonPayload["totalDifficulty"] = "0x123456"
		jsonPayload["difficulty"] = encodedNum
		jsonPayload["size"] = encodedNum
		jsonPayload["baseFeePerGas"] = encodedNum
		jsonPayload["withdrawals"] = []*zondtypes.Withdrawal{}

		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(payload, 0)
		require.NoError(t, err)
		header, err := blocks.PayloadToHeaderCapella(wrappedPayload)
		require.NoError(t, err)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			respJSON := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  jsonPayload,
			}
			require.NoError(t, json.NewEncoder(w).Encode(respJSON))
		}))
		defer srv.Close()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient
		blindedBlock := util.NewBlindedBeaconBlockCapella()

		blindedBlock.Block.Body.ExecutionPayloadHeader = header
		wrapped, err := blocks.NewSignedBeaconBlock(blindedBlock)
		require.NoError(t, err)
		reconstructed, err := service.ReconstructFullBlock(ctx, wrapped)
		require.NoError(t, err)

		got, err := reconstructed.Block().Body().Execution()
		require.NoError(t, err)
		require.DeepEqual(t, payload, got.Proto())
	})
}

func TestReconstructFullBlockBatch(t *testing.T) {
	ctx := context.Background()
	t.Run("nil block", func(t *testing.T) {
		service := &Service{}

		_, err := service.ReconstructFullBlockBatch(ctx, []interfaces.ReadOnlySignedBeaconBlock{nil})
		require.ErrorContains(t, "nil data", err)
	})
	t.Run("only blinded block", func(t *testing.T) {
		want := "can only reconstruct block from blinded block format"
		service := &Service{}
		bellatrixBlock := util.NewBeaconBlockCapella()
		wrapped, err := blocks.NewSignedBeaconBlock(bellatrixBlock)
		require.NoError(t, err)
		_, err = service.ReconstructFullBlockBatch(ctx, []interfaces.ReadOnlySignedBeaconBlock{wrapped})
		require.ErrorContains(t, want, err)
	})
	t.Run("properly reconstructs block batch with correct payload", func(t *testing.T) {
		fix := fixtures()
		payload, ok := fix["ExecutionPayloadCapella"].(*pb.ExecutionPayloadCapella)
		require.Equal(t, true, ok)

		jsonPayload := make(map[string]interface{})

		to := common.HexToAddress("095e7baea6a6c7c4c2dfeb977efac326af552d87")
		tx := zondtypes.NewTx(&zondtypes.DynamicFeeTx{
			Nonce: 0,
			To:    &to,
			Value: big.NewInt(0),
			Data:  nil,
		})
		txs := []*zondtypes.Transaction{tx}
		encodedBinaryTxs := make([][]byte, 1)
		var err error
		encodedBinaryTxs[0], err = txs[0].MarshalBinary()
		require.NoError(t, err)
		payload.Transactions = encodedBinaryTxs
		jsonPayload["transactions"] = txs
		num := big.NewInt(1)
		encodedNum := hexutil.EncodeBig(num)
		jsonPayload["hash"] = hexutil.Encode(payload.BlockHash)
		jsonPayload["parentHash"] = common.BytesToHash([]byte("parent"))
		jsonPayload["sha3Uncles"] = common.BytesToHash([]byte("uncles"))
		jsonPayload["miner"] = common.BytesToAddress([]byte("miner"))
		jsonPayload["stateRoot"] = common.BytesToHash([]byte("state"))
		jsonPayload["transactionsRoot"] = common.BytesToHash([]byte("txs"))
		jsonPayload["receiptsRoot"] = common.BytesToHash([]byte("receipts"))
		jsonPayload["logsBloom"] = zondtypes.BytesToBloom([]byte("bloom"))
		jsonPayload["gasLimit"] = hexutil.EncodeUint64(1)
		jsonPayload["gasUsed"] = hexutil.EncodeUint64(2)
		jsonPayload["timestamp"] = hexutil.EncodeUint64(3)
		jsonPayload["number"] = encodedNum
		jsonPayload["extraData"] = common.BytesToHash([]byte("extra"))
		jsonPayload["totalDifficulty"] = "0x123456"
		jsonPayload["difficulty"] = encodedNum
		jsonPayload["size"] = encodedNum
		jsonPayload["baseFeePerGas"] = encodedNum
		jsonPayload["withdrawals"] = []*zondtypes.Withdrawal{}

		wrappedPayload, err := blocks.WrappedExecutionPayloadCapella(payload, 0)
		require.NoError(t, err)
		header, err := blocks.PayloadToHeaderCapella(wrappedPayload)
		require.NoError(t, err)

		capellaBlock := util.NewBlindedBeaconBlockCapella()
		wanted := util.NewBeaconBlockCapella()
		wanted.Block.Slot = 1
		// Make sure block hash is the zero hash.
		capellaBlock.Block.Body.ExecutionPayloadHeader.BlockHash = make([]byte, 32)
		capellaBlock.Block.Slot = 1

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()

			respJSON := []map[string]interface{}{
				{
					"jsonrpc": "2.0",
					"id":      1,
					"result":  jsonPayload,
				},
				{
					"jsonrpc": "2.0",
					"id":      2,
					"result":  jsonPayload,
				},
			}
			require.NoError(t, json.NewEncoder(w).Encode(respJSON))
			require.NoError(t, json.NewEncoder(w).Encode(respJSON))

		}))
		defer srv.Close()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)
		defer rpcClient.Close()

		service := &Service{}
		service.rpcClient = rpcClient
		blindedBlock := util.NewBlindedBeaconBlockCapella()

		blindedBlock.Block.Body.ExecutionPayloadHeader = header
		wrapped, err := blocks.NewSignedBeaconBlock(blindedBlock)
		require.NoError(t, err)
		copiedWrapped, err := wrapped.Copy()
		require.NoError(t, err)

		reconstructed, err := service.ReconstructFullBlockBatch(ctx, []interfaces.ReadOnlySignedBeaconBlock{wrapped, copiedWrapped})
		require.NoError(t, err)

		// Handle normal execution blocks correctly
		got, err := reconstructed[0].Block().Body().Execution()
		require.NoError(t, err)
		require.DeepEqual(t, payload, got.Proto())

		got, err = reconstructed[1].Block().Body().Execution()
		require.NoError(t, err)
		require.DeepEqual(t, payload, got.Proto())
	})
}

type customError struct {
	code    int
	timeout bool
}

func (c *customError) ErrorCode() int {
	return c.code
}

func (*customError) Error() string {
	return "something went wrong"
}

func (c *customError) Timeout() bool {
	return c.timeout
}

type dataError struct {
	code int
	data interface{}
}

func (c *dataError) ErrorCode() int {
	return c.code
}

func (*dataError) Error() string {
	return "something went wrong"
}

func (c *dataError) ErrorData() interface{} {
	return c.data
}

func Test_handleRPCError(t *testing.T) {
	got := handleRPCError(nil)
	require.Equal(t, true, got == nil)

	var tests = []struct {
		name             string
		expected         error
		expectedContains string
		given            error
	}{
		{
			name:             "not an rpc error",
			expectedContains: "got an unexpected error",
			given:            errors.New("foo"),
		},
		{
			name:             "HTTP times out",
			expectedContains: ErrHTTPTimeout.Error(),
			given:            &customError{timeout: true},
		},
		{
			name:             "ErrParse",
			expectedContains: ErrParse.Error(),
			given:            &customError{code: -32700},
		},
		{
			name:             "ErrInvalidRequest",
			expectedContains: ErrInvalidRequest.Error(),
			given:            &customError{code: -32600},
		},
		{
			name:             "ErrMethodNotFound",
			expectedContains: ErrMethodNotFound.Error(),
			given:            &customError{code: -32601},
		},
		{
			name:             "ErrInvalidParams",
			expectedContains: ErrInvalidParams.Error(),
			given:            &customError{code: -32602},
		},
		{
			name:             "ErrInternal",
			expectedContains: ErrInternal.Error(),
			given:            &customError{code: -32603},
		},
		{
			name:             "ErrUnknownPayload",
			expectedContains: ErrUnknownPayload.Error(),
			given:            &customError{code: -38001},
		},
		{
			name:             "ErrInvalidForkchoiceState",
			expectedContains: ErrInvalidForkchoiceState.Error(),
			given:            &customError{code: -38002},
		},
		{
			name:             "ErrInvalidPayloadAttributes",
			expectedContains: ErrInvalidPayloadAttributes.Error(),
			given:            &customError{code: -38003},
		},
		{
			name:             "ErrServer unexpected no data",
			expectedContains: "got an unexpected error",
			given:            &customError{code: -32000},
		},
		{
			name:             "ErrServer with data",
			expectedContains: ErrServer.Error(),
			given:            &dataError{code: -32000, data: 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleRPCError(tt.given)
			require.ErrorContains(t, tt.expectedContains, got)
		})
	}
}

func newTestIPCServer(t *testing.T) *rpc.Server {
	server := rpc.NewServer()
	err := server.RegisterName("engine", new(testEngineService))
	require.NoError(t, err)
	err = server.RegisterName("eth", new(testEngineService))
	require.NoError(t, err)
	return server
}

func fixtures() map[string]interface{} {
	foo := bytesutil.ToBytes32([]byte("foo"))
	bar := bytesutil.PadTo([]byte("bar"), 20)
	baz := bytesutil.PadTo([]byte("baz"), 256)
	baseFeePerGas := big.NewInt(12345)
	executionPayloadFixtureCapella := &pb.ExecutionPayloadCapella{
		ParentHash:    foo[:],
		FeeRecipient:  bar,
		StateRoot:     foo[:],
		ReceiptsRoot:  foo[:],
		LogsBloom:     baz,
		PrevRandao:    foo[:],
		BlockNumber:   1,
		GasLimit:      1,
		GasUsed:       1,
		Timestamp:     1,
		ExtraData:     foo[:],
		BaseFeePerGas: bytesutil.PadTo(baseFeePerGas.Bytes(), fieldparams.RootLength),
		BlockHash:     foo[:],
		Transactions:  [][]byte{foo[:]},
		Withdrawals:   []*pb.Withdrawal{},
	}
	hexUint := hexutil.Uint64(1)
	executionPayloadWithValueFixtureCapella := &pb.GetPayloadV2ResponseJson{
		ExecutionPayload: &pb.ExecutionPayloadCapellaJSON{
			ParentHash:    &common.Hash{'a'},
			FeeRecipient:  &common.Address{'b'},
			StateRoot:     &common.Hash{'c'},
			ReceiptsRoot:  &common.Hash{'d'},
			LogsBloom:     &hexutil.Bytes{'e'},
			PrevRandao:    &common.Hash{'f'},
			BaseFeePerGas: "0x123",
			BlockHash:     &common.Hash{'g'},
			Transactions:  []hexutil.Bytes{{'h'}},
			Withdrawals:   []*pb.Withdrawal{},
			BlockNumber:   &hexUint,
			GasLimit:      &hexUint,
			GasUsed:       &hexUint,
			Timestamp:     &hexUint,
		},
		BlockValue: "0x11fffffffff",
	}
	parent := bytesutil.PadTo([]byte("parentHash"), fieldparams.RootLength)
	miner := bytesutil.PadTo([]byte("miner"), fieldparams.FeeRecipientLength)
	stateRoot := bytesutil.PadTo([]byte("stateRoot"), fieldparams.RootLength)
	transactionsRoot := bytesutil.PadTo([]byte("transactionsRoot"), fieldparams.RootLength)
	receiptsRoot := bytesutil.PadTo([]byte("receiptsRoot"), fieldparams.RootLength)
	logsBloom := bytesutil.PadTo([]byte("logs"), fieldparams.LogsBloomLength)
	executionBlock := &pb.ExecutionBlock{
		Version: version.Capella,
		Header: zondtypes.Header{
			ParentHash:  common.BytesToHash(parent),
			Coinbase:    common.BytesToAddress(miner),
			Root:        common.BytesToHash(stateRoot),
			TxHash:      common.BytesToHash(transactionsRoot),
			ReceiptHash: common.BytesToHash(receiptsRoot),
			Bloom:       zondtypes.BytesToBloom(logsBloom),
			Number:      big.NewInt(2),
			GasLimit:    3,
			GasUsed:     4,
			Time:        5,
			Extra:       []byte("extra"),
			Random:      common.BytesToHash([]byte("random")),
			BaseFee:     big.NewInt(7),
		},
		Withdrawals: []*pb.Withdrawal{},
	}
	status := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_VALID,
		LatestValidHash: foo[:],
		ValidationError: "",
	}
	id := pb.PayloadIDBytes([8]byte{1, 0, 0, 0, 0, 0, 0, 0})
	forkChoiceResp := &ForkchoiceUpdatedResponse{
		Status:    status,
		PayloadId: &id,
	}
	forkChoiceSyncingResp := &ForkchoiceUpdatedResponse{
		Status: &pb.PayloadStatus{
			Status:          pb.PayloadStatus_SYNCING,
			LatestValidHash: nil,
		},
		PayloadId: &id,
	}
	forkChoiceAcceptedResp := &ForkchoiceUpdatedResponse{
		Status: &pb.PayloadStatus{
			Status:          pb.PayloadStatus_ACCEPTED,
			LatestValidHash: nil,
		},
		PayloadId: &id,
	}
	forkChoiceInvalidResp := &ForkchoiceUpdatedResponse{
		Status: &pb.PayloadStatus{
			Status:          pb.PayloadStatus_INVALID,
			LatestValidHash: bytesutil.PadTo([]byte("latestValidHash"), 32),
		},
		PayloadId: &id,
	}
	validStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_VALID,
		LatestValidHash: foo[:],
		ValidationError: "",
	}
	inValidBlockHashStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_INVALID_BLOCK_HASH,
		LatestValidHash: nil,
	}
	acceptedStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_ACCEPTED,
		LatestValidHash: nil,
	}
	syncingStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_SYNCING,
		LatestValidHash: nil,
	}
	invalidStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_INVALID,
		LatestValidHash: foo[:],
	}
	unknownStatus := &pb.PayloadStatus{
		Status:          pb.PayloadStatus_UNKNOWN,
		LatestValidHash: foo[:],
	}
	return map[string]interface{}{
		"ExecutionBlock":                    executionBlock,
		"ExecutionPayloadCapella":           executionPayloadFixtureCapella,
		"ExecutionPayloadCapellaWithValue":  executionPayloadWithValueFixtureCapella,
		"ValidPayloadStatus":                validStatus,
		"InvalidBlockHashStatus":            inValidBlockHashStatus,
		"AcceptedStatus":                    acceptedStatus,
		"SyncingStatus":                     syncingStatus,
		"InvalidStatus":                     invalidStatus,
		"UnknownStatus":                     unknownStatus,
		"ForkchoiceUpdatedResponse":         forkChoiceResp,
		"ForkchoiceUpdatedSyncingResponse":  forkChoiceSyncingResp,
		"ForkchoiceUpdatedAcceptedResponse": forkChoiceAcceptedResp,
		"ForkchoiceUpdatedInvalidResponse":  forkChoiceInvalidResp,
	}
}

func Test_fullPayloadFromExecutionBlockCapella(t *testing.T) {
	type args struct {
		header  *pb.ExecutionPayloadHeaderCapella
		block   *pb.ExecutionBlock
		version int
	}
	wantedHash := common.BytesToHash([]byte("foo"))
	tests := []struct {
		name string
		args args
		want func() interfaces.ExecutionData
		err  string
	}{
		{
			name: "block hash field in header and block hash mismatch",
			args: args{
				header: &pb.ExecutionPayloadHeaderCapella{
					BlockHash: []byte("foo"),
				},
				block: &pb.ExecutionBlock{
					Hash: common.BytesToHash([]byte("bar")),
				},
				version: version.Capella,
			},
			err: "does not match execution block hash",
		},
		{
			name: "ok",
			args: args{
				header: &pb.ExecutionPayloadHeaderCapella{
					BlockHash: wantedHash[:],
				},
				block: &pb.ExecutionBlock{
					Hash: wantedHash,
				},
				version: version.Capella,
			},
			want: func() interfaces.ExecutionData {
				p, err := blocks.WrappedExecutionPayloadCapella(&pb.ExecutionPayloadCapella{
					BlockHash:    wantedHash[:],
					Transactions: [][]byte{},
				}, 0)
				require.NoError(t, err)
				return p
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped, err := blocks.WrappedExecutionPayloadHeaderCapella(tt.args.header, 0)
			require.NoError(t, err)
			got, err := fullPayloadFromExecutionBlock(tt.args.version, wrapped, tt.args.block)
			if err != nil {
				assert.ErrorContains(t, tt.err, err)
			} else {
				assert.DeepEqual(t, tt.want(), got)
			}
		})
	}
}

func TestHeaderByHash_NotFound(t *testing.T) {
	srv := &Service{}
	srv.rpcClient = RPCClientBad{}

	_, err := srv.HeaderByHash(context.Background(), [32]byte{})
	assert.Equal(t, zond.NotFound, err)
}

func TestHeaderByNumber_NotFound(t *testing.T) {
	srv := &Service{}
	srv.rpcClient = RPCClientBad{}

	_, err := srv.HeaderByNumber(context.Background(), big.NewInt(100))
	assert.Equal(t, zond.NotFound, err)
}

func TestToBlockNumArg(t *testing.T) {
	tests := []struct {
		name   string
		number *big.Int
		want   string
	}{
		{
			name:   "genesis",
			number: big.NewInt(0),
			want:   "0x0",
		},
		{
			name:   "near genesis block",
			number: big.NewInt(300),
			want:   "0x12c",
		},
		{
			name:   "current block",
			number: big.NewInt(15838075),
			want:   "0xf1ab7b",
		},
		{
			name:   "far off block",
			number: big.NewInt(12032894823020),
			want:   "0xaf1a06bea6c",
		},
		{
			name:   "latest block",
			number: nil,
			want:   "latest",
		},
		{
			name:   "pending block",
			number: big.NewInt(-1),
			want:   "pending",
		},
		{
			name:   "finalized block",
			number: big.NewInt(-3),
			want:   "finalized",
		},
		{
			name:   "safe block",
			number: big.NewInt(-4),
			want:   "safe",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toBlockNumArg(tt.number); got != tt.want {
				t.Errorf("toBlockNumArg() = %v, want %v", got, tt.want)
			}
		})
	}
}

type testEngineService struct{}

func (*testEngineService) NoArgsRets() {}

func (*testEngineService) GetBlockByHash(
	_ context.Context, _ common.Hash, _ bool,
) *pb.ExecutionBlock {
	fix := fixtures()
	item, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testEngineService) GetBlockByNumber(
	_ context.Context, _ string, _ bool,
) *pb.ExecutionBlock {
	fix := fixtures()
	item, ok := fix["ExecutionBlock"].(*pb.ExecutionBlock)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testEngineService) GetPayloadV2(
	_ context.Context, _ pb.PayloadIDBytes,
) *pb.ExecutionPayloadCapellaWithValue {
	fix := fixtures()
	item, ok := fix["ExecutionPayloadCapellaWithValue"].(*pb.ExecutionPayloadCapellaWithValue)
	if !ok {
		panic("not found")
	}
	return item
}

func (*testEngineService) ForkchoiceUpdatedV2(
	_ context.Context, _ *pb.ForkchoiceState, _ *pb.PayloadAttributesV2,
) *ForkchoiceUpdatedResponse {
	fix := fixtures()
	item, ok := fix["ForkchoiceUpdatedResponse"].(*ForkchoiceUpdatedResponse)
	if !ok {
		panic("not found")
	}
	item.Status.Status = pb.PayloadStatus_VALID
	return item
}

func (*testEngineService) NewPayloadV2(
	_ context.Context, _ *pb.ExecutionPayloadCapella,
) *pb.PayloadStatus {
	fix := fixtures()
	item, ok := fix["ValidPayloadStatus"].(*pb.PayloadStatus)
	if !ok {
		panic("not found")
	}
	return item
}

func forkchoiceUpdateSetupV2(t *testing.T, fcs *pb.ForkchoiceState, att *pb.PayloadAttributesV2, res *ForkchoiceUpdatedResponse) *Service {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		defer func() {
			require.NoError(t, r.Body.Close())
		}()
		enc, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		jsonRequestString := string(enc)

		forkChoiceStateReq, err := json.Marshal(fcs)
		require.NoError(t, err)
		payloadAttrsReq, err := json.Marshal(att)
		require.NoError(t, err)

		// We expect the JSON string RPC request contains the right arguments.
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(forkChoiceStateReq),
		))
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(payloadAttrsReq),
		))
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  res,
		}
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))

	rpcClient, err := rpc.Dial(srv.URL)
	require.NoError(t, err)

	service := &Service{}
	service.rpcClient = rpcClient
	return service
}

func newPayloadV2Setup(t *testing.T, status *pb.PayloadStatus, payload *pb.ExecutionPayloadCapella) *Service {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		defer func() {
			require.NoError(t, r.Body.Close())
		}()
		enc, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		jsonRequestString := string(enc)

		reqArg, err := json.Marshal(payload)
		require.NoError(t, err)

		// We expect the JSON string RPC request contains the right arguments.
		require.Equal(t, true, strings.Contains(
			jsonRequestString, string(reqArg),
		))
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  status,
		}
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))

	rpcClient, err := rpc.Dial(srv.URL)
	require.NoError(t, err)

	service := &Service{}
	service.rpcClient = rpcClient
	return service
}

func TestCapella_PayloadBodiesByHash(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableOptionalEngineMethods: true,
	})
	defer resetFn()
	t.Run("empty response works", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 0)
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByHash(ctx, []common.Hash{})
		require.NoError(t, err)
		require.Equal(t, 0, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
	t.Run("single element response null works", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 1)
			executionPayloadBodies[0] = nil

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByHash(ctx, []common.Hash{})
		require.NoError(t, err)
		require.Equal(t, 1, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
	t.Run("empty, null, full works", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 3)
			executionPayloadBodies[0] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{},
				Withdrawals:  []*pb.Withdrawal{},
			}
			executionPayloadBodies[1] = nil
			executionPayloadBodies[2] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{hexutil.MustDecode("0x02f878831469668303f51d843b9ac9f9843b9aca0082520894c93269b73096998db66be0441e836d873535cb9c8894a19041886f000080c001a031cc29234036afbf9a1fb9476b463367cb1f957ac0b919b69bbc798436e604aaa018c4e9c3914eb27aadd0b91e10b18655739fcf8c1fc398763a9f1beecb8ddc86")},
				Withdrawals: []*pb.Withdrawal{{
					Index:          1,
					ValidatorIndex: 1,
					Address:        hexutil.MustDecode("0xcf8e0d4e9587369b2301d0790347320302cc0943"),
					Amount:         1,
				}},
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByHash(ctx, []common.Hash{})
		require.NoError(t, err)
		require.Equal(t, 3, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
	t.Run("full works, single item", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 1)
			executionPayloadBodies[0] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{hexutil.MustDecode("0x02f878831469668303f51d843b9ac9f9843b9aca0082520894c93269b73096998db66be0441e836d873535cb9c8894a19041886f000080c001a031cc29234036afbf9a1fb9476b463367cb1f957ac0b919b69bbc798436e604aaa018c4e9c3914eb27aadd0b91e10b18655739fcf8c1fc398763a9f1beecb8ddc86")},
				Withdrawals: []*pb.Withdrawal{{
					Index:          1,
					ValidatorIndex: 1,
					Address:        hexutil.MustDecode("0xcf8e0d4e9587369b2301d0790347320302cc0943"),
					Amount:         1,
				}},
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByHash(ctx, []common.Hash{})
		require.NoError(t, err)
		require.Equal(t, 1, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
	t.Run("full works, multiple items", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 2)
			executionPayloadBodies[0] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{hexutil.MustDecode("0x02f878831469668303f51d843b9ac9f9843b9aca0082520894c93269b73096998db66be0441e836d873535cb9c8894a19041886f000080c001a031cc29234036afbf9a1fb9476b463367cb1f957ac0b919b69bbc798436e604aaa018c4e9c3914eb27aadd0b91e10b18655739fcf8c1fc398763a9f1beecb8ddc86")},
				Withdrawals: []*pb.Withdrawal{{
					Index:          1,
					ValidatorIndex: 1,
					Address:        hexutil.MustDecode("0xcf8e0d4e9587369b2301d0790347320302cc0943"),
					Amount:         1,
				}},
			}
			executionPayloadBodies[1] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{hexutil.MustDecode("0x02f878831469668303f51d843b9ac9f9843b9aca0082520894c93269b73096998db66be0441e836d873535cb9c8894a19041886f000080c001a031cc29234036afbf9a1fb9476b463367cb1f957ac0b919b69bbc798436e604aaa018c4e9c3914eb27aadd0b91e10b18655739fcf8c1fc398763a9f1beecb8ddc86")},
				Withdrawals: []*pb.Withdrawal{{
					Index:          2,
					ValidatorIndex: 1,
					Address:        hexutil.MustDecode("0xcf8e0d4e9587369b2301d0790347320302cc0943"),
					Amount:         1,
				}},
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByHash(ctx, []common.Hash{})
		require.NoError(t, err)
		require.Equal(t, 2, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
	t.Run("returning empty, null, empty should work properly", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			// [A, B, C] but no B in the server means
			// we get [Abody, null, Cbody].
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 3)
			executionPayloadBodies[0] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{},
				Withdrawals:  []*pb.Withdrawal{},
			}
			executionPayloadBodies[1] = nil
			executionPayloadBodies[2] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{},
				Withdrawals:  []*pb.Withdrawal{},
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByHash(ctx, []common.Hash{})
		require.NoError(t, err)
		require.Equal(t, 3, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
}

func TestCapella_PayloadBodiesByRange(t *testing.T) {
	resetFn := features.InitWithReset(&features.Flags{
		EnableOptionalEngineMethods: true,
	})
	defer resetFn()
	t.Run("empty response works", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 0)
			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByRange(ctx, uint64(1), uint64(2))
		require.NoError(t, err)
		require.Equal(t, 0, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
	t.Run("single element response null works", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 1)
			executionPayloadBodies[0] = nil

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByRange(ctx, uint64(1), uint64(2))
		require.NoError(t, err)
		require.Equal(t, 1, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
	t.Run("empty, null, full works", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 3)
			executionPayloadBodies[0] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{},
				Withdrawals:  []*pb.Withdrawal{},
			}
			executionPayloadBodies[1] = nil
			executionPayloadBodies[2] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{hexutil.MustDecode("0x02f878831469668303f51d843b9ac9f9843b9aca0082520894c93269b73096998db66be0441e836d873535cb9c8894a19041886f000080c001a031cc29234036afbf9a1fb9476b463367cb1f957ac0b919b69bbc798436e604aaa018c4e9c3914eb27aadd0b91e10b18655739fcf8c1fc398763a9f1beecb8ddc86")},
				Withdrawals: []*pb.Withdrawal{{
					Index:          1,
					ValidatorIndex: 1,
					Address:        hexutil.MustDecode("0xcf8e0d4e9587369b2301d0790347320302cc0943"),
					Amount:         1,
				}},
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByRange(ctx, uint64(1), uint64(2))
		require.NoError(t, err)
		require.Equal(t, 3, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
	t.Run("full works, single item", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 1)
			executionPayloadBodies[0] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{hexutil.MustDecode("0x02f878831469668303f51d843b9ac9f9843b9aca0082520894c93269b73096998db66be0441e836d873535cb9c8894a19041886f000080c001a031cc29234036afbf9a1fb9476b463367cb1f957ac0b919b69bbc798436e604aaa018c4e9c3914eb27aadd0b91e10b18655739fcf8c1fc398763a9f1beecb8ddc86")},
				Withdrawals: []*pb.Withdrawal{{
					Index:          1,
					ValidatorIndex: 1,
					Address:        hexutil.MustDecode("0xcf8e0d4e9587369b2301d0790347320302cc0943"),
					Amount:         1,
				}},
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByRange(ctx, uint64(1), uint64(2))
		require.NoError(t, err)
		require.Equal(t, 1, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
	t.Run("full works, multiple items", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 2)
			executionPayloadBodies[0] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{hexutil.MustDecode("0x02f878831469668303f51d843b9ac9f9843b9aca0082520894c93269b73096998db66be0441e836d873535cb9c8894a19041886f000080c001a031cc29234036afbf9a1fb9476b463367cb1f957ac0b919b69bbc798436e604aaa018c4e9c3914eb27aadd0b91e10b18655739fcf8c1fc398763a9f1beecb8ddc86")},
				Withdrawals: []*pb.Withdrawal{{
					Index:          1,
					ValidatorIndex: 1,
					Address:        hexutil.MustDecode("0xcf8e0d4e9587369b2301d0790347320302cc0943"),
					Amount:         1,
				}},
			}
			executionPayloadBodies[1] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{hexutil.MustDecode("0x02f878831469668303f51d843b9ac9f9843b9aca0082520894c93269b73096998db66be0441e836d873535cb9c8894a19041886f000080c001a031cc29234036afbf9a1fb9476b463367cb1f957ac0b919b69bbc798436e604aaa018c4e9c3914eb27aadd0b91e10b18655739fcf8c1fc398763a9f1beecb8ddc86")},
				Withdrawals: []*pb.Withdrawal{{
					Index:          2,
					ValidatorIndex: 1,
					Address:        hexutil.MustDecode("0xcf8e0d4e9587369b2301d0790347320302cc0943"),
					Amount:         1,
				}},
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByRange(ctx, uint64(1), uint64(2))
		require.NoError(t, err)
		require.Equal(t, 2, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
	t.Run("returning empty, null, empty should work properly", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			defer func() {
				require.NoError(t, r.Body.Close())
			}()
			// [A, B, C] but no B in the server means
			// we get [Abody, null, Cbody].
			executionPayloadBodies := make([]*pb.ExecutionPayloadBodyV1, 3)
			executionPayloadBodies[0] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{},
				Withdrawals:  []*pb.Withdrawal{},
			}
			executionPayloadBodies[1] = nil
			executionPayloadBodies[2] = &pb.ExecutionPayloadBodyV1{
				Transactions: [][]byte{},
				Withdrawals:  []*pb.Withdrawal{},
			}

			resp := map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  executionPayloadBodies,
			}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
		}))
		ctx := context.Background()

		rpcClient, err := rpc.Dial(srv.URL)
		require.NoError(t, err)

		service := &Service{}
		service.rpcClient = rpcClient

		results, err := service.GetPayloadBodiesByRange(ctx, uint64(1), uint64(2))
		require.NoError(t, err)
		require.Equal(t, 3, len(results))

		for _, item := range results {
			require.NotNil(t, item)
		}
	})
}
