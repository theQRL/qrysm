package apimiddleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/r3labs/sse/v2"
	"github.com/theQRL/qrysm/api"
	"github.com/theQRL/qrysm/api/gateway/apimiddleware"
	"github.com/theQRL/qrysm/api/grpc"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/events"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

type testSSZResponseJson struct {
	Version             string `json:"version"`
	ExecutionOptimistic bool   `json:"execution_optimistic"`
	Finalized           bool   `json:"finalized"`
	Data                string `json:"data"`
}

func (t testSSZResponseJson) SSZVersion() string {
	return t.Version
}

func (t testSSZResponseJson) SSZOptimistic() bool {
	return t.ExecutionOptimistic
}

func (t testSSZResponseJson) SSZData() string {
	return t.Data
}

func (t testSSZResponseJson) SSZFinalized() bool {
	return t.Finalized
}

func TestPrepareSSZRequestForProxying(t *testing.T) {
	middleware := &apimiddleware.ApiProxyMiddleware{
		GatewayAddress: "http://apimiddleware.example",
	}
	endpoint := apimiddleware.Endpoint{
		Path: "http://foo.example",
	}
	var body bytes.Buffer
	request := httptest.NewRequest("GET", "http://foo.example", &body)

	errJson := prepareSSZRequestForProxying(middleware, endpoint, request)
	require.Equal(t, true, errJson == nil)
	assert.Equal(t, "/internal/ssz", request.URL.Path)
}

func TestPreparePostedSszData(t *testing.T) {
	var body bytes.Buffer
	body.Write([]byte("body"))
	request := httptest.NewRequest("POST", "http://foo.example", &body)

	preparePostedSSZData(request)
	assert.Equal(t, int64(19), request.ContentLength)
	assert.Equal(t, api.JsonMediaType, request.Header.Get("Content-Type"))
}

func TestSerializeMiddlewareResponseIntoSSZ(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		j := testSSZResponseJson{
			Version: "Version",
			Data:    "Zm9v",
		}
		v, ssz, errJson := serializeMiddlewareResponseIntoSSZ(j)
		require.Equal(t, true, errJson == nil)
		assert.DeepEqual(t, []byte("foo"), ssz)
		assert.Equal(t, "version", v)
	})

	t.Run("invalid_data", func(t *testing.T) {
		j := testSSZResponseJson{
			Version: "Version",
			Data:    "invalid",
		}
		_, _, errJson := serializeMiddlewareResponseIntoSSZ(j)
		require.Equal(t, false, errJson == nil)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "could not decode response body into base64"))
		assert.Equal(t, http.StatusInternalServerError, errJson.StatusCode())
	})
}

func TestWriteSSZResponseHeaderAndBody(t *testing.T) {
	responseSsz := []byte("ssz")
	version := "version"
	fileName := "test.ssz"

	t.Run("ok", func(t *testing.T) {
		response := &http.Response{
			Header: http.Header{
				"Foo": []string{"foo"},
				"Grpc-Metadata-" + grpc.HttpCodeMetadataKey: []string{"200"},
			},
		}

		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		errJson := writeSSZResponseHeaderAndBody(response, writer, responseSsz, version, fileName)
		require.Equal(t, true, errJson == nil)
		v, ok := writer.Header()["Foo"]
		require.Equal(t, true, ok, "header not found")
		require.Equal(t, 1, len(v), "wrong number of header values")
		assert.Equal(t, "foo", v[0])
		v, ok = writer.Header()["Content-Length"]
		require.Equal(t, true, ok, "header not found")
		require.Equal(t, 1, len(v), "wrong number of header values")
		assert.Equal(t, "3", v[0])
		v, ok = writer.Header()["Content-Type"]
		require.Equal(t, true, ok, "header not found")
		require.Equal(t, 1, len(v), "wrong number of header values")
		assert.Equal(t, api.OctetStreamMediaType, v[0])
		v, ok = writer.Header()["Content-Disposition"]
		require.Equal(t, true, ok, "header not found")
		require.Equal(t, 1, len(v), "wrong number of header values")
		assert.Equal(t, "attachment; filename=test.ssz", v[0])
		v, ok = writer.Header()[api.VersionHeader]
		require.Equal(t, true, ok, "header not found")
		require.Equal(t, 1, len(v), "wrong number of header values")
		assert.Equal(t, "version", v[0])
		assert.Equal(t, 200, writer.Code)
	})

	t.Run("no_grpc_status_code_header", func(t *testing.T) {
		response := &http.Response{
			Header:     http.Header{},
			StatusCode: 200,
		}
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		errJson := writeSSZResponseHeaderAndBody(response, writer, responseSsz, version, fileName)
		require.Equal(t, true, errJson == nil)
		assert.Equal(t, 200, writer.Code)
	})

	t.Run("invalid_status_code", func(t *testing.T) {
		response := &http.Response{
			Header: http.Header{
				"Foo": []string{"foo"},
				"Grpc-Metadata-" + grpc.HttpCodeMetadataKey: []string{"invalid"},
			},
		}
		responseSsz := []byte("ssz")
		writer := httptest.NewRecorder()
		writer.Body = &bytes.Buffer{}

		errJson := writeSSZResponseHeaderAndBody(response, writer, responseSsz, version, fileName)
		require.Equal(t, false, errJson == nil)
		assert.Equal(t, true, strings.Contains(errJson.Msg(), "could not parse status code"))
		assert.Equal(t, http.StatusInternalServerError, errJson.StatusCode())
	})
}

func TestReceiveEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *sse.Event)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	req := httptest.NewRequest("GET", "http://foo.example", &bytes.Buffer{})
	req = req.WithContext(ctx)

	go func() {
		base64Val := "Zm9v"
		data := &EventFinalizedCheckpointJson{
			Block: base64Val,
			State: base64Val,
			Epoch: "1",
		}
		bData, err := json.Marshal(data)
		require.NoError(t, err)
		msg := &sse.Event{
			Data:  bData,
			Event: []byte(events.FinalizedCheckpointTopic),
		}
		ch <- msg
		time.Sleep(time.Second)
		cancel()
	}()

	errJson := receiveEvents(ch, w, req)
	assert.Equal(t, true, errJson == nil)

	expectedEvent := `event: finalized_checkpoint
data: {"block":"0x666f6f","state":"0x666f6f","epoch":"1","execution_optimistic":false}

`
	assert.DeepEqual(t, expectedEvent, w.Body.String())
}

func TestReceiveEvents_AggregatedAtt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *sse.Event)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	req := httptest.NewRequest("GET", "http://foo.example", &bytes.Buffer{})
	req = req.WithContext(ctx)

	go func() {
		base64Val := "Zm9v"
		data := AggregatedAttReceivedDataJson{
			Aggregate: &AttestationJson{
				AggregationBits: base64Val,
				Data: &AttestationDataJson{
					Slot:            "1",
					CommitteeIndex:  "1",
					BeaconBlockRoot: base64Val,
					Source:          nil,
					Target:          nil,
				},
				Signatures: []string{base64Val},
			},
		}
		bData, err := json.Marshal(data)
		require.NoError(t, err)
		msg := &sse.Event{
			Data:  bData,
			Event: []byte(events.AttestationTopic),
		}
		ch <- msg
		time.Sleep(time.Second)
		cancel()
	}()

	errJson := receiveEvents(ch, w, req)
	assert.Equal(t, true, errJson == nil)

	expectedEvent := `event: attestation
data: {"aggregation_bits":"0x666f6f","data":{"slot":"1","index":"1","beacon_block_root":"0x666f6f","source":null,"target":null},"signatures":["0x666f6f"]}

`
	assert.DeepEqual(t, expectedEvent, w.Body.String())
}

func TestReceiveEvents_UnaggregatedAtt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *sse.Event)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	req := httptest.NewRequest("GET", "http://foo.example", &bytes.Buffer{})
	req = req.WithContext(ctx)

	go func() {
		base64Val := "Zm9v"
		data := UnaggregatedAttReceivedDataJson{
			AggregationBits: base64Val,
			Data: &AttestationDataJson{
				Slot:            "1",
				CommitteeIndex:  "1",
				BeaconBlockRoot: base64Val,
				Source:          nil,
				Target:          nil,
			},
			Signatures: []string{base64Val},
		}
		bData, err := json.Marshal(data)
		require.NoError(t, err)
		msg := &sse.Event{
			Data:  bData,
			Event: []byte(events.AttestationTopic),
		}
		ch <- msg
		time.Sleep(time.Second)
		cancel()
	}()

	errJson := receiveEvents(ch, w, req)
	assert.Equal(t, true, errJson == nil)

	expectedEvent := `event: attestation
data: {"aggregation_bits":"0x666f6f","data":{"slot":"1","index":"1","beacon_block_root":"0x666f6f","source":null,"target":null},"signatures":["0x666f6f"]}

`
	assert.DeepEqual(t, expectedEvent, w.Body.String())
}

func TestReceiveEvents_EventNotSupported(t *testing.T) {
	ch := make(chan *sse.Event)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	req := httptest.NewRequest("GET", "http://foo.example", &bytes.Buffer{})

	go func() {
		msg := &sse.Event{
			Data:  []byte("foo"),
			Event: []byte("not_supported"),
		}
		ch <- msg
	}()

	errJson := receiveEvents(ch, w, req)
	require.NotNil(t, errJson)
	assert.Equal(t, "Event type 'not_supported' not supported", errJson.Msg())
}

func TestReceiveEvents_TrailingSpace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan *sse.Event)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}
	req := httptest.NewRequest("GET", "http://foo.example", &bytes.Buffer{})
	req = req.WithContext(ctx)

	go func() {
		base64Val := "Zm9v"
		data := &EventFinalizedCheckpointJson{
			Block: base64Val,
			State: base64Val,
			Epoch: "1",
		}
		bData, err := json.Marshal(data)
		require.NoError(t, err)
		msg := &sse.Event{
			Data:  bData,
			Event: []byte("finalized_checkpoint "),
		}
		ch <- msg
		time.Sleep(time.Second)
		cancel()
	}()

	errJson := receiveEvents(ch, w, req)
	assert.Equal(t, true, errJson == nil)
	assert.Equal(t, `event: finalized_checkpoint
data: {"block":"0x666f6f","state":"0x666f6f","epoch":"1","execution_optimistic":false}

`, w.Body.String())
}

func TestWriteEvent(t *testing.T) {
	base64Val := "Zm9v"
	data := &EventFinalizedCheckpointJson{
		Block: base64Val,
		State: base64Val,
		Epoch: "1",
	}
	bData, err := json.Marshal(data)
	require.NoError(t, err)
	msg := &sse.Event{
		Data:  bData,
		Event: []byte("test_event"),
	}
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	errJson := writeEvent(msg, w, &EventFinalizedCheckpointJson{})
	require.Equal(t, true, errJson == nil)
	written := w.Body.String()
	assert.Equal(t, "event: test_event\ndata: {\"block\":\"0x666f6f\",\"state\":\"0x666f6f\",\"epoch\":\"1\",\"execution_optimistic\":false}\n\n", written)
}

// TestHandleEvents_UpstreamErrorIsRelayedImmediately is a regression test for
// the events proxy retrying a non-200 grpc-gateway answer (e.g. 400 for an
// unknown topic) with exponential backoff for up to 15 minutes while the
// client hung, and then answering twice. The gateway's status and message
// must be relayed at once, in a single response.
func TestHandleEvents_UpstreamErrorIsRelayedImmediately(t *testing.T) {
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/internal/qrl/v1/events", r.URL.Path)
		assert.Equal(t, "foo", r.URL.Query().Get("topics"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte(`{"code":3,"message":"Invalid topic foo"}`))
		require.NoError(t, err)
	}))
	defer gateway.Close()
	m := &apimiddleware.ApiProxyMiddleware{GatewayAddress: strings.TrimPrefix(gateway.URL, "http://")}

	req := httptest.NewRequest(http.MethodGet, "http://foo.example/qrl/v1/events?topics=foo", nil)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	start := time.Now()
	handled := handleEvents(m, apimiddleware.Endpoint{}, w, req)
	require.Equal(t, true, handled)
	require.Equal(t, true, time.Since(start) < 5*time.Second, "events proxy retried the failed upstream connection")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	e := &apimiddleware.DefaultErrorJson{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), e))
	assert.Equal(t, http.StatusBadRequest, e.Code)
	assert.Equal(t, "Invalid topic foo", e.Message)
}

// TestHandleEvents_LargeEventIsProxied checks that an event bigger than the
// sse library's 64 KiB default buffer (any aggregate attestation or sync
// contribution event with ML-DSA-87 signatures) is proxied instead of
// tripping bufio.ErrTooLong, and that the proxy returns once the upstream
// stream ends.
func TestHandleEvents_LargeEventIsProxied(t *testing.T) {
	padding := strings.Repeat("a", 256*1024)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, err := fmt.Fprintf(w, "event: head\ndata: {\"slot\":\"1\",\"block\":\"Zm9v\",\"state\":\"Zm9v\",\"padding\":\"%s\"}\n\n", padding)
		require.NoError(t, err)
		w.(http.Flusher).Flush()
		// Returning ends the upstream stream.
	}))
	defer gateway.Close()
	m := &apimiddleware.ApiProxyMiddleware{GatewayAddress: strings.TrimPrefix(gateway.URL, "http://")}

	req := httptest.NewRequest(http.MethodGet, "http://foo.example/qrl/v1/events?topics=head", nil)
	w := httptest.NewRecorder()
	w.Body = &bytes.Buffer{}

	done := make(chan bool, 1)
	go func() { done <- handleEvents(m, apimiddleware.Endpoint{}, w, req) }()
	select {
	case handled := <-done:
		require.Equal(t, true, handled)
	case <-time.After(10 * time.Second):
		t.Fatal("events proxy did not return after the upstream stream ended")
	}
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, true, strings.Contains(w.Body.String(), "event: head\n"), "large event was not proxied: %.200s", w.Body.String())
	assert.Equal(t, true, strings.Contains(w.Body.String(), `"slot":"1"`))
}
