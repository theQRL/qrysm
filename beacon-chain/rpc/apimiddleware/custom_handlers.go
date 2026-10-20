package apimiddleware

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/r3labs/sse/v2"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/api"
	"github.com/theQRL/qrysm/api/gateway/apimiddleware"
	"github.com/theQRL/qrysm/api/grpc"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/events"
	http2 "github.com/theQRL/qrysm/network/http"
	"github.com/theQRL/qrysm/runtime/version"
)

type sszConfig struct {
	fileName     string
	responseJson SszResponse
}

func handleGetBeaconStateSSZ(m *apimiddleware.ApiProxyMiddleware, endpoint apimiddleware.Endpoint, w http.ResponseWriter, req *http.Request) (handled bool) {
	config := sszConfig{
		fileName:     "beacon_state.ssz",
		responseJson: &VersionedSSZResponseJson{},
	}
	return handleGetSSZ(m, endpoint, w, req, config)
}

func handleGetBeaconBlockSSZ(m *apimiddleware.ApiProxyMiddleware, endpoint apimiddleware.Endpoint, w http.ResponseWriter, req *http.Request) (handled bool) {
	config := sszConfig{
		fileName:     "beacon_block.ssz",
		responseJson: &VersionedSSZResponseJson{},
	}
	return handleGetSSZ(m, endpoint, w, req, config)
}

func handleGetBlindedBeaconBlockSSZ(
	m *apimiddleware.ApiProxyMiddleware,
	endpoint apimiddleware.Endpoint,
	w http.ResponseWriter,
	req *http.Request,
) (handled bool) {
	config := sszConfig{
		fileName:     "beacon_block.ssz",
		responseJson: &VersionedSSZResponseJson{},
	}
	return handleGetSSZ(m, endpoint, w, req, config)
}

func handleProduceBlockSSZ(m *apimiddleware.ApiProxyMiddleware, endpoint apimiddleware.Endpoint, w http.ResponseWriter, req *http.Request) (handled bool) {
	config := sszConfig{
		fileName:     "produce_beacon_block.ssz",
		responseJson: &VersionedSSZResponseJson{},
	}
	return handleGetSSZ(m, endpoint, w, req, config)
}

func handleProduceBlindedBlockSSZ(
	m *apimiddleware.ApiProxyMiddleware,
	endpoint apimiddleware.Endpoint,
	w http.ResponseWriter,
	req *http.Request,
) (handled bool) {
	config := sszConfig{
		fileName:     "produce_blinded_beacon_block.ssz",
		responseJson: &VersionedSSZResponseJson{},
	}
	return handleGetSSZ(m, endpoint, w, req, config)
}

func handleGetSSZ(
	m *apimiddleware.ApiProxyMiddleware,
	endpoint apimiddleware.Endpoint,
	w http.ResponseWriter,
	req *http.Request,
	config sszConfig,
) (handled bool) {
	ssz := http2.RespondWithSsz(req)
	if !ssz {
		return false
	}

	if errJson := prepareSSZRequestForProxying(m, endpoint, req); errJson != nil {
		apimiddleware.WriteError(w, errJson, nil)
		return true
	}
	grpcResponse, errJson := m.ProxyRequest(req)
	if errJson != nil {
		apimiddleware.WriteError(w, errJson, nil)
		return true
	}
	grpcResponseBody, errJson := apimiddleware.ReadGrpcResponseBody(grpcResponse.Body)
	if errJson != nil {
		apimiddleware.WriteError(w, errJson, nil)
		return true
	}
	respHasError, errJson := apimiddleware.HandleGrpcResponseError(endpoint.Err, grpcResponse, grpcResponseBody, w)
	if errJson != nil {
		apimiddleware.WriteError(w, errJson, nil)
		return true
	}
	if respHasError {
		return true
	}
	if errJson := apimiddleware.DeserializeGrpcResponseBodyIntoContainer(grpcResponseBody, config.responseJson); errJson != nil {
		apimiddleware.WriteError(w, errJson, nil)
		return true
	}
	respVersion, responseSsz, errJson := serializeMiddlewareResponseIntoSSZ(config.responseJson)
	if errJson != nil {
		apimiddleware.WriteError(w, errJson, nil)
		return true
	}
	if errJson := writeSSZResponseHeaderAndBody(grpcResponse, w, responseSsz, respVersion, config.fileName); errJson != nil {
		apimiddleware.WriteError(w, errJson, nil)
		return true
	}
	if errJson := apimiddleware.Cleanup(grpcResponse.Body); errJson != nil {
		apimiddleware.WriteError(w, errJson, nil)
		return true
	}

	return true
}

func prepareSSZRequestForProxying(m *apimiddleware.ApiProxyMiddleware, endpoint apimiddleware.Endpoint, req *http.Request) apimiddleware.ErrorJson {
	req.URL.Scheme = "http"
	req.URL.Host = m.GatewayAddress
	req.RequestURI = ""
	if errJson := apimiddleware.HandleURLParameters(endpoint.Path, req, endpoint.RequestURLLiterals); errJson != nil {
		return errJson
	}
	if errJson := apimiddleware.HandleQueryParameters(req, endpoint.RequestQueryParams); errJson != nil {
		return errJson
	}
	// We have to add new segments after handling parameters because it changes URL segment indexing.
	req.URL.Path = "/internal" + req.URL.Path + "/ssz"
	return nil
}

func preparePostedSSZData(req *http.Request) apimiddleware.ErrorJson {
	buf, err := io.ReadAll(req.Body)
	if err != nil {
		return apimiddleware.InternalServerErrorWithMessage(err, "could not read body")
	}
	j := SszRequestJson{Data: base64.StdEncoding.EncodeToString(buf)}
	data, err := json.Marshal(j)
	if err != nil {
		return apimiddleware.InternalServerErrorWithMessage(err, "could not prepare POST data")
	}
	req.Body = io.NopCloser(bytes.NewBuffer(data))
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", api.JsonMediaType)
	return nil
}

func serializeMiddlewareResponseIntoSSZ(respJson SszResponse) (version string, ssz []byte, errJson apimiddleware.ErrorJson) {
	// Serialize the SSZ part of the deserialized value.
	data, err := base64.StdEncoding.DecodeString(respJson.SSZData())
	if err != nil {
		return "", nil, apimiddleware.InternalServerErrorWithMessage(err, "could not decode response body into base64")
	}
	return strings.ToLower(respJson.SSZVersion()), data, nil
}

func writeSSZResponseHeaderAndBody(grpcResp *http.Response, w http.ResponseWriter, respSsz []byte, respVersion, fileName string) apimiddleware.ErrorJson {
	var statusCodeHeader string
	for h, vs := range grpcResp.Header {
		// We don't want to expose any gRPC metadata in the HTTP response, so we skip forwarding metadata headers.
		if strings.HasPrefix(h, "Grpc-Metadata") {
			if h == "Grpc-Metadata-"+grpc.HttpCodeMetadataKey {
				statusCodeHeader = vs[0]
			}
		} else {
			for _, v := range vs {
				w.Header().Set(h, v)
			}
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(respSsz)))
	w.Header().Set("Content-Type", api.OctetStreamMediaType)
	w.Header().Set("Content-Disposition", "attachment; filename="+fileName)
	w.Header().Set(api.VersionHeader, respVersion)
	if statusCodeHeader != "" {
		code, err := strconv.Atoi(statusCodeHeader)
		if err != nil {
			return apimiddleware.InternalServerErrorWithMessage(err, "could not parse status code")
		}
		w.WriteHeader(code)
	} else {
		w.WriteHeader(grpcResp.StatusCode)
	}
	if _, err := io.Copy(w, io.NopCloser(bytes.NewReader(respSsz))); err != nil {
		return apimiddleware.InternalServerErrorWithMessage(err, "could not write response message")
	}
	return nil
}

// eventsMaxBufferSize bounds a single SSE event read from grpc-gateway. The
// sse library's default (64 KiB) is smaller than an aggregate attestation or
// sync committee contribution event carrying ML-DSA-87 signatures (up to
// 128 x 4627 bytes, hex-encoded ~1.2 MB), which made the reader fail with
// bufio.ErrTooLong, reconnect and silently drop events. The buffer only grows
// on demand, so the cap costs nothing for small events.
const eventsMaxBufferSize = 32 << 20

var log = logrus.WithField("prefix", "apimiddleware")

// eventsNoRetry is a backoff policy that never retries. It satisfies the sse
// client's ReconnectStrategy (gopkg.in/cenkalti/backoff.v1 BackOff) so that a
// non-200 answer from grpc-gateway - e.g. 400 for an unknown topic - is
// reported straight away instead of being retried with the library's default
// exponential backoff for up to 15 minutes while the client hangs.
type eventsNoRetry struct{}

func (eventsNoRetry) Reset() {}

// NextBackOff returns backoff.Stop (-1).
func (eventsNoRetry) NextBackOff() time.Duration { return -1 }

// eventsConnectError carries grpc-gateway's status and message for a failed
// events subscription so that they can be relayed to the client.
type eventsConnectError struct {
	code    int
	message string
}

func (e *eventsConnectError) Error() string {
	return fmt.Sprintf("could not connect to event stream: %d %s", e.code, e.message)
}

// eventsResponseValidator turns a non-200 grpc-gateway response into an
// eventsConnectError with the gateway's own status code and message.
func eventsResponseValidator(_ *sse.Client, resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		body = nil
	}
	message := http.StatusText(resp.StatusCode)
	gatewayErr := &struct {
		Message string `json:"message"`
	}{}
	if json.Unmarshal(body, gatewayErr) == nil && gatewayErr.Message != "" {
		message = gatewayErr.Message
	} else if len(body) > 0 {
		message = string(body)
	}
	return &eventsConnectError{code: resp.StatusCode, message: message}
}

// writeTrackingResponseWriter records whether anything has been written to the
// response, so that an upstream failure after events have already been
// streamed is not answered with a second (superfluous) error response.
type writeTrackingResponseWriter struct {
	http.ResponseWriter
	written bool
}

func (w *writeTrackingResponseWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(b)
}

func (w *writeTrackingResponseWriter) WriteHeader(statusCode int) {
	w.written = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *writeTrackingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func handleEvents(m *apimiddleware.ApiProxyMiddleware, _ apimiddleware.Endpoint, w http.ResponseWriter, req *http.Request) (handled bool) {
	sseClient := sse.NewClient(
		"http://"+m.GatewayAddress+"/internal"+req.URL.RequestURI(),
		sse.ClientMaxBufferSize(eventsMaxBufferSize),
	)
	sseClient.Headers["Grpc-Timeout"] = "0S"
	sseClient.ReconnectStrategy = eventsNoRetry{}
	sseClient.ResponseValidator = eventsResponseValidator

	// The proxied stream lives as long as the client's request, or until the
	// upstream stream ends (grpc-gateway closed it, or reading it failed):
	// in that case the client is disconnected too rather than left waiting
	// on a connection that will never carry another event.
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	// We use grpc-gateway as the server side of events, not the sse library.
	// Because of this subscribing to streams doesn't work as intended, resulting in each event being handled by all subscriptions.
	// To handle events properly, we subscribe just once using a placeholder value ('events') and handle all topics inside this subscription.
	eventChan := make(chan *sse.Event)
	streamDone := make(chan error, 1)
	go func() {
		err := sseClient.SubscribeWithContext(ctx, "events", func(msg *sse.Event) {
			select {
			case eventChan <- msg:
			case <-ctx.Done():
			}
		})
		streamDone <- err
		cancel()
	}()

	tw := &writeTrackingResponseWriter{ResponseWriter: w}
	errJson := receiveEvents(eventChan, tw, req.WithContext(ctx))
	cancel()
	streamErr := <-streamDone

	switch {
	case errJson != nil:
		if !tw.written {
			apimiddleware.WriteError(w, errJson, nil)
		} else {
			log.WithError(errors.New(errJson.Msg())).Debug("Event stream ended with an error")
		}
	case streamErr != nil && req.Context().Err() == nil:
		var connectErr *eventsConnectError
		if !tw.written {
			if errors.As(streamErr, &connectErr) {
				apimiddleware.WriteError(w, &apimiddleware.DefaultErrorJson{Message: connectErr.message, Code: connectErr.code}, nil)
			} else {
				apimiddleware.WriteError(w, apimiddleware.InternalServerError(streamErr), nil)
			}
		} else {
			log.WithError(streamErr).Debug("Event stream ended with an error")
		}
	}
	return true
}

type dataSubset struct {
	Version string `json:"version"`
}

func receiveEvents(eventChan <-chan *sse.Event, w http.ResponseWriter, req *http.Request) apimiddleware.ErrorJson {
	for {
		select {
		case msg := <-eventChan:
			var data any

			// The message's event comes to us with trailing whitespace. Remove it here for
			// ease of future processing.
			msg.Event = bytes.TrimSpace(msg.Event)

			switch string(msg.Event) {
			case events.HeadTopic:
				data = &EventHeadJson{}
			case events.BlockTopic:
				data = &ReceivedBlockDataJson{}
			case events.AttestationTopic:
				data = &AttestationJson{}

				// Data received in the aggregated att event does not fit the expected event stream output.
				// We extract the underlying attestation from event data
				// and assign the attestation back to event data for further processing.
				aggEventData := &AggregatedAttReceivedDataJson{}
				if err := json.Unmarshal(msg.Data, aggEventData); err != nil {
					return apimiddleware.InternalServerError(err)
				}
				var attData []byte
				var err error
				// If true, then we have an unaggregated attestation
				if aggEventData.Aggregate == nil {
					unaggEventData := &UnaggregatedAttReceivedDataJson{}
					if err := json.Unmarshal(msg.Data, unaggEventData); err != nil {
						return apimiddleware.InternalServerError(err)
					}
					attData, err = json.Marshal(unaggEventData)
					if err != nil {
						return apimiddleware.InternalServerError(err)
					}
				} else {
					attData, err = json.Marshal(aggEventData.Aggregate)
					if err != nil {
						return apimiddleware.InternalServerError(err)
					}
				}
				msg.Data = attData
			case events.VoluntaryExitTopic:
				data = &SignedVoluntaryExitJson{}
			case events.FinalizedCheckpointTopic:
				data = &EventFinalizedCheckpointJson{}
			case events.ChainReorgTopic:
				data = &EventChainReorgJson{}
			case events.SyncCommitteeContributionTopic:
				data = &SignedContributionAndProofJson{}
			case events.PayloadAttributesTopic:
				dataSubset := &dataSubset{}
				if err := json.Unmarshal(msg.Data, dataSubset); err != nil {
					return apimiddleware.InternalServerError(err)
				}
				switch dataSubset.Version {
				case version.String(version.Zond):
					data = &EventPayloadAttributeStreamV2Json{}
				default:
					return apimiddleware.InternalServerError(errors.New("payload version unsupported"))
				}
			case "error":
				data = &EventErrorJson{}
			default:
				return &apimiddleware.DefaultErrorJson{
					Message: fmt.Sprintf("Event type '%s' not supported", string(msg.Event)),
					Code:    http.StatusInternalServerError,
				}
			}

			if errJson := writeEvent(msg, w, data); errJson != nil {
				return errJson
			}
			if errJson := flushEvent(w); errJson != nil {
				return errJson
			}
		case <-req.Context().Done():
			return nil
		}
	}
}

func writeEvent(msg *sse.Event, w http.ResponseWriter, data any) apimiddleware.ErrorJson {
	if err := json.Unmarshal(msg.Data, data); err != nil {
		return apimiddleware.InternalServerError(err)
	}
	if errJson := apimiddleware.ProcessMiddlewareResponseFields(data); errJson != nil {
		return errJson
	}
	dataJson, errJson := apimiddleware.SerializeMiddlewareResponseIntoJson(data)
	if errJson != nil {
		return errJson
	}

	w.Header().Set("Content-Type", "text/event-stream")

	if _, err := w.Write([]byte("event: ")); err != nil {
		return apimiddleware.InternalServerError(err)
	}
	if _, err := w.Write(msg.Event); err != nil {
		return apimiddleware.InternalServerError(err)
	}
	if _, err := w.Write([]byte("\ndata: ")); err != nil {
		return apimiddleware.InternalServerError(err)
	}
	if _, err := w.Write(dataJson); err != nil {
		return apimiddleware.InternalServerError(err)
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return apimiddleware.InternalServerError(err)
	}

	return nil
}

func flushEvent(w http.ResponseWriter) apimiddleware.ErrorJson {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return &apimiddleware.DefaultErrorJson{Message: fmt.Sprintf("Flush not supported in %T", w), Code: http.StatusInternalServerError}
	}
	flusher.Flush()
	return nil
}
