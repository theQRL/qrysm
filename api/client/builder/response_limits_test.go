package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"testing"

	"github.com/theQRL/go-qrl/common"
	qrltypes "github.com/theQRL/go-qrl/core/types"
	"github.com/theQRL/go-qrl/crypto/pqcrypto/wallet"
	qrlparams "github.com/theQRL/go-qrl/params"
	"github.com/theQRL/qrysm/api/client"
	fieldparams "github.com/theQRL/qrysm/config/fieldparams"
	"github.com/theQRL/qrysm/consensus-types/blocks"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestSubmitBlindedBlock_LargeExecutionPayload(t *testing.T) {
	// These signed, ordinary transfers consume only 12.6M gas, but their
	// ML-DSA-87 signatures and public keys put the JSON response above 8 MiB.
	const transactionCount = 600
	w, err := wallet.Generate(wallet.ML_DSA_87)
	require.NoError(t, err)
	signer := qrltypes.NewZondSigner(qrlparams.MainnetChainConfig.ChainID)
	to := common.Address{}
	to[len(to)-1] = 0x42
	payload := util.NewBeaconBlockZond().Block.Body.ExecutionPayload
	payload.GasLimit = qrlparams.MaxGasLimit
	payload.GasUsed = transactionCount * qrlparams.TxGas
	require.Equal(t, true, payload.GasUsed <= payload.GasLimit)
	payload.Transactions = make([][]byte, transactionCount)
	for i := uint64(0); i < transactionCount; i++ {
		tx, err := qrltypes.SignNewTx(w, signer, &qrltypes.DynamicFeeTx{
			ChainID:   signer.ChainID(),
			Nonce:     i,
			Gas:       qrlparams.TxGas,
			GasFeeCap: new(big.Int).SetUint64(2 * qrlparams.InitialBaseFee),
			GasTipCap: big.NewInt(1),
			Value:     big.NewInt(0),
			To:        &to,
		})
		require.NoError(t, err)
		_, err = qrltypes.Sender(signer, tx)
		require.NoError(t, err)
		payload.Transactions[i], err = tx.MarshalBinary()
		require.NoError(t, err)
	}
	data, err := FromProtoZond(payload)
	require.NoError(t, err)
	encoded, err := json.Marshal(ExecPayloadResponseZond{Version: "zond", Data: data})
	require.NoError(t, err)
	require.Equal(t, true, int64(len(encoded)) > client.MaxBodySize)
	require.Equal(t, true, int64(len(encoded)) <= maxExecutionPayloadResponseSize)
	expectedRoot, err := payload.HashTreeRoot()
	require.NoError(t, err)
	sb, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockZond())
	require.NoError(t, err)

	for _, contentLength := range []int64{int64(len(encoded)), -1} {
		t.Run(fmt.Sprintf("content_length_%d", contentLength), func(t *testing.T) {
			body := &trackedBuilderResponse{Reader: bytes.NewReader(encoded)}
			resp := &http.Response{StatusCode: http.StatusOK, ContentLength: contentLength, Body: body}
			c := builderClientWithResponse(t, resp)
			got, err := c.SubmitBlindedBlock(context.Background(), sb)
			require.NoError(t, err)
			actualRoot, err := got.HashTreeRoot()
			require.NoError(t, err)
			require.Equal(t, expectedRoot, actualRoot)
			txs, err := got.Transactions()
			require.NoError(t, err)
			require.DeepEqual(t, payload.Transactions, txs)
			require.Equal(t, http.MethodPost, resp.Request.Method)
			require.Equal(t, postBlindedBeaconBlockPath, resp.Request.URL.Path)
			require.Equal(t, "zond", resp.Request.Header.Get("Qrl-Consensus-Version"))
			require.Equal(t, int64(len(encoded)), body.read)
			require.Equal(t, true, body.closed)
		})
	}
}

func TestBuilderResponseBodyLimits(t *testing.T) {
	// Exercise exact boundaries with a small limit, avoiding repeated large
	// allocations. The endpoint tests below exercise the actual configured caps.
	const limit int64 = 64
	for _, tc := range []struct {
		name          string
		size          int64
		contentLength int64
		wantRead      int64
		wantErr       bool
	}{
		{name: "empty", size: 0, contentLength: 0, wantRead: 0},
		{name: "below limit", size: limit - 1, contentLength: limit - 1, wantRead: limit - 1},
		{name: "at limit", size: limit, contentLength: limit, wantRead: limit},
		{name: "at limit without content length", size: limit, contentLength: -1, wantRead: limit},
		{name: "oversized content length", size: limit + 1, contentLength: limit + 1, wantRead: 0, wantErr: true},
		{name: "oversized chunked body", size: 2 * limit, contentLength: -1, wantRead: limit + 1, wantErr: true},
		{name: "underreported content length", size: 2 * limit, contentLength: 1, wantRead: limit + 1, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBuilderResponse{Reader: io.LimitReader(zeroBuilderResponse{}, tc.size)}
			c := builderClientWithResponse(t, &http.Response{StatusCode: http.StatusOK, ContentLength: tc.contentLength, Body: body})
			got, err := c.doWithMaxBodySize(context.Background(), http.MethodGet, getStatus, nil, limit)
			if tc.wantErr {
				require.ErrorContains(t, fmt.Sprintf("builder response body exceeds size limit of %d bytes", limit), err)
				require.Equal(t, true, got == nil)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.size, int64(len(got)))
			}
			require.Equal(t, tc.wantRead, body.read)
			require.Equal(t, true, body.closed)
		})
	}
}

func TestSubmitBlindedBlock_ResponseTooLarge(t *testing.T) {
	sb, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockZond())
	require.NoError(t, err)
	for _, contentLength := range []int64{maxExecutionPayloadResponseSize + 2, -1} {
		t.Run(fmt.Sprintf("content_length_%d", contentLength), func(t *testing.T) {
			body := &trackedBuilderResponse{Reader: io.LimitReader(zeroBuilderResponse{}, maxExecutionPayloadResponseSize+2)}
			c := builderClientWithResponse(t, &http.Response{StatusCode: http.StatusOK, ContentLength: contentLength, Body: body})
			got, err := c.SubmitBlindedBlock(context.Background(), sb)
			require.ErrorContains(t, fmt.Sprintf("builder response body exceeds size limit of %d bytes", maxExecutionPayloadResponseSize), err)
			require.Equal(t, true, got == nil)
			wantRead := maxExecutionPayloadResponseSize + 1
			if contentLength > maxExecutionPayloadResponseSize {
				wantRead = 0
			}
			require.Equal(t, wantRead, body.read)
			require.Equal(t, true, body.closed)
		})
	}
}

func TestBuilderOtherEndpoints_KeepResponseLimit(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		call func(*Client) error
	}{
		{name: "status", call: func(c *Client) error { return c.Status(ctx) }},
		{name: "header", call: func(c *Client) error {
			_, err := c.GetHeader(ctx, 1, [32]byte{}, [fieldparams.MLDSA87PubkeyLength]byte{})
			return err
		}},
		{name: "registration", call: func(c *Client) error {
			return c.RegisterValidator(ctx, []*qrysmpb.SignedValidatorRegistrationV1{{
				Message: &qrysmpb.ValidatorRegistrationV1{
					FeeRecipient: make([]byte, fieldparams.FeeRecipientLength),
					Pubkey:       make([]byte, fieldparams.MLDSA87PubkeyLength),
				},
				Signature: make([]byte, fieldparams.MLDSA87SignatureLength),
			}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBuilderResponse{Reader: io.LimitReader(zeroBuilderResponse{}, client.MaxBodySize+1)}
			c := builderClientWithResponse(t, &http.Response{StatusCode: http.StatusOK, ContentLength: client.MaxBodySize + 1, Body: body})
			err := tc.call(c)
			require.ErrorContains(t, fmt.Sprintf("builder response body exceeds size limit of %d bytes", client.MaxBodySize), err)
			require.Equal(t, int64(0), body.read)
			require.Equal(t, true, body.closed)
		})
	}
}

func TestSubmitBlindedBlock_ErrorBodyLimit(t *testing.T) {
	// Even the full-payload endpoint must not read a large error response.
	// Pad a valid error with whitespace so the capped prefix remains valid JSON.
	errorPrefix := bytes.Repeat([]byte{' '}, int(client.MaxErrBodySize))
	copy(errorPrefix, `{"code":500,"message":"builder unavailable"}`)
	body := &trackedBuilderResponse{Reader: io.MultiReader(bytes.NewReader(errorPrefix), zeroBuilderResponse{})}
	c := builderClientWithResponse(t, &http.Response{StatusCode: http.StatusInternalServerError, ContentLength: maxExecutionPayloadResponseSize + 1, Body: body})
	sb, err := blocks.NewSignedBeaconBlock(util.NewBlindedBeaconBlockZond())
	require.NoError(t, err)
	_, err = c.SubmitBlindedBlock(context.Background(), sb)
	require.ErrorIs(t, err, ErrNotOK)
	require.Equal(t, client.MaxErrBodySize, body.read)
	require.Equal(t, true, body.closed)
}

func TestBuilderResponseReadError(t *testing.T) {
	body := &trackedBuilderResponse{Reader: failedBuilderResponse{}}
	c := builderClientWithResponse(t, &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: body})
	_, err := c.do(context.Background(), http.MethodGet, getStatus, nil)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, true, body.closed)
}

func builderClientWithResponse(t *testing.T, response *http.Response) *Client {
	t.Helper()
	c, err := NewClient("http://builder.invalid")
	require.NoError(t, err)
	c.hc.Transport = roundtrip(func(req *http.Request) (*http.Response, error) {
		if req.Body != nil {
			require.NoError(t, req.Body.Close())
		}
		response.Request = req
		return response, nil
	})
	return c
}

type trackedBuilderResponse struct {
	io.Reader
	read   int64
	closed bool
}

func (b *trackedBuilderResponse) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += int64(n)
	return n, err
}

func (b *trackedBuilderResponse) Close() error {
	b.closed = true
	return nil
}

type zeroBuilderResponse struct{}

func (zeroBuilderResponse) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type failedBuilderResponse struct{}

func (failedBuilderResponse) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
