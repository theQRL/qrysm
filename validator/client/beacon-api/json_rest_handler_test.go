package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/theQRL/qrysm/api/gateway/apimiddleware"
	"github.com/theQRL/qrysm/beacon-chain/rpc/qrl/beacon"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

func TestGetRestJsonResponse_Valid(t *testing.T) {
	const endpoint = "/example/rest/api/endpoint"

	genesisJson := &beacon.GetGenesisResponse{
		Data: &beacon.Genesis{
			GenesisTime:           "123",
			GenesisValidatorsRoot: "0x456",
			GenesisForkVersion:    "0x789",
		},
	}

	ctx := context.Background()

	mux := http.NewServeMux()
	mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
		// Make sure the url parameters match
		assert.Equal(t, "abc", r.URL.Query().Get("arg1"))
		assert.Equal(t, "def", r.URL.Query().Get("arg2"))

		marshalledJson, err := json.Marshal(genesisJson)
		require.NoError(t, err)

		_, err = w.Write(marshalledJson)
		require.NoError(t, err)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	jsonRestHandler := beaconApiJsonRestHandler{
		timeout: time.Second * 5,
		host:    server.URL,
	}

	responseJson := &beacon.GetGenesisResponse{}
	_, err := jsonRestHandler.GetRestJsonResponse(ctx, endpoint+"?arg1=abc&arg2=def", responseJson)
	assert.NoError(t, err)
	assert.DeepEqual(t, genesisJson, responseJson)
}

func TestGetRestJsonResponse_Error(t *testing.T) {
	const endpoint = "/example/rest/api/endpoint"

	testCases := []struct {
		name                 string
		funcHandler          func(w http.ResponseWriter, r *http.Request)
		expectedErrorJson    *apimiddleware.DefaultErrorJson
		expectedErrorMessage string
		timeout              time.Duration
		responseJson         any
	}{
		{
			name:                 "nil response json",
			funcHandler:          invalidJsonResponseHandler,
			expectedErrorMessage: "responseJson is nil",
			timeout:              time.Second * 5,
			responseJson:         nil,
		},
		{
			name:                 "400 error",
			funcHandler:          httpErrorJsonHandler(http.StatusBadRequest, "Bad request"),
			expectedErrorMessage: "error 400: Bad request",
			expectedErrorJson: &apimiddleware.DefaultErrorJson{
				Code:    http.StatusBadRequest,
				Message: "Bad request",
			},
			timeout:      time.Second * 5,
			responseJson: &beacon.GetGenesisResponse{},
		},
		{
			name:                 "404 error",
			funcHandler:          httpErrorJsonHandler(http.StatusNotFound, "Not found"),
			expectedErrorMessage: "error 404: Not found",
			expectedErrorJson: &apimiddleware.DefaultErrorJson{
				Code:    http.StatusNotFound,
				Message: "Not found",
			},
			timeout:      time.Second * 5,
			responseJson: &beacon.GetGenesisResponse{},
		},
		{
			name:                 "500 error",
			funcHandler:          httpErrorJsonHandler(http.StatusInternalServerError, "Internal server error"),
			expectedErrorMessage: "error 500: Internal server error",
			expectedErrorJson: &apimiddleware.DefaultErrorJson{
				Code:    http.StatusInternalServerError,
				Message: "Internal server error",
			},
			timeout:      time.Second * 5,
			responseJson: &beacon.GetGenesisResponse{},
		},
		{
			name:                 "999 error",
			funcHandler:          httpErrorJsonHandler(999, "Invalid error"),
			expectedErrorMessage: "error 999: Invalid error",
			expectedErrorJson: &apimiddleware.DefaultErrorJson{
				Code:    999,
				Message: "Invalid error",
			},
			timeout:      time.Second * 5,
			responseJson: &beacon.GetGenesisResponse{},
		},
		{
			// Regression: when the error body is not JSON (e.g. an HTML 502
			// from a reverse proxy), the wrapped error must surface the raw
			// body and status code instead of "failed to decode error json".
			name:                 "non-JSON error body",
			funcHandler:          invalidJsonErrHandler,
			expectedErrorMessage: "unsuccessful (404: foo)",
			timeout:              time.Second * 5,
			responseJson:         &beacon.GetGenesisResponse{},
		},
		{
			name:                 "bad response json formatting",
			funcHandler:          invalidJsonResponseHandler,
			expectedErrorMessage: "failed to decode response json",
			timeout:              time.Second * 5,
			responseJson:         &beacon.GetGenesisResponse{},
		},
		{
			name:                 "timeout",
			funcHandler:          httpErrorJsonHandler(http.StatusNotFound, "Not found"),
			expectedErrorMessage: "failed to query REST API",
			timeout:              1,
			responseJson:         &beacon.GetGenesisResponse{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc(endpoint, testCase.funcHandler)
			server := httptest.NewServer(mux)
			defer server.Close()

			ctx := context.Background()

			jsonRestHandler := beaconApiJsonRestHandler{
				timeout: testCase.timeout,
				host:    server.URL,
			}
			errorJson, err := jsonRestHandler.GetRestJsonResponse(ctx, endpoint, testCase.responseJson)
			assert.ErrorContains(t, testCase.expectedErrorMessage, err)
			assert.DeepEqual(t, testCase.expectedErrorJson, errorJson)
		})
	}
}

func TestPostRestJson_Valid(t *testing.T) {
	const endpoint = "/example/rest/api/endpoint"
	dataBytes := []byte{1, 2, 3, 4, 5}

	genesisJson := &beacon.GetGenesisResponse{
		Data: &beacon.Genesis{
			GenesisTime:           "123",
			GenesisValidatorsRoot: "0x456",
			GenesisForkVersion:    "0x789",
		},
	}

	testCases := []struct {
		name         string
		headers      map[string]string
		data         *bytes.Buffer
		responseJson any
	}{
		{
			name:         "nil headers",
			headers:      nil,
			data:         bytes.NewBuffer(dataBytes),
			responseJson: &beacon.GetGenesisResponse{},
		},
		{
			name:         "empty headers",
			headers:      map[string]string{},
			data:         bytes.NewBuffer(dataBytes),
			responseJson: &beacon.GetGenesisResponse{},
		},
		{
			name:         "nil response json",
			headers:      map[string]string{"DummyHeaderKey": "DummyHeaderValue"},
			data:         bytes.NewBuffer(dataBytes),
			responseJson: nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
				// Make sure the request headers have been set
				for headerKey, headerValue := range testCase.headers {
					assert.Equal(t, headerValue, r.Header.Get(headerKey))
				}

				// Make sure the data matches
				receivedBytes, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.DeepEqual(t, dataBytes, receivedBytes)

				marshalledJson, err := json.Marshal(genesisJson)
				require.NoError(t, err)

				_, err = w.Write(marshalledJson)
				require.NoError(t, err)
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			ctx := context.Background()

			jsonRestHandler := beaconApiJsonRestHandler{
				timeout: time.Second * 5,
				host:    server.URL,
			}

			_, err := jsonRestHandler.PostRestJson(
				ctx,
				endpoint,
				testCase.headers,
				testCase.data,
				testCase.responseJson,
			)

			assert.NoError(t, err)

			if testCase.responseJson != nil {
				assert.DeepEqual(t, genesisJson, testCase.responseJson)
			}
		})
	}
}

func TestPostRestJson_Error(t *testing.T) {
	const endpoint = "/example/rest/api/endpoint"

	testCases := []struct {
		name                 string
		funcHandler          func(w http.ResponseWriter, r *http.Request)
		expectedErrorJson    *apimiddleware.DefaultErrorJson
		expectedErrorMessage string
		timeout              time.Duration
		responseJson         *beacon.GetGenesisResponse
		data                 *bytes.Buffer
	}{
		{
			name:                 "nil POST data",
			funcHandler:          httpErrorJsonHandler(http.StatusNotFound, "Not found"),
			expectedErrorMessage: "POST data is nil",
			timeout:              time.Second * 5,
			data:                 nil,
		},
		{
			name:                 "400 error",
			funcHandler:          httpErrorJsonHandler(http.StatusBadRequest, "Bad request"),
			expectedErrorMessage: "error 400: Bad request",
			expectedErrorJson: &apimiddleware.DefaultErrorJson{
				Code:    http.StatusBadRequest,
				Message: "Bad request",
			},
			timeout:      time.Second * 5,
			responseJson: &beacon.GetGenesisResponse{},
			data:         &bytes.Buffer{},
		},
		{
			name:                 "404 error",
			funcHandler:          httpErrorJsonHandler(http.StatusNotFound, "Not found"),
			expectedErrorMessage: "error 404: Not found",
			expectedErrorJson: &apimiddleware.DefaultErrorJson{
				Code:    http.StatusNotFound,
				Message: "Not found",
			},
			timeout: time.Second * 5,
			data:    &bytes.Buffer{},
		},
		{
			name:                 "500 error",
			funcHandler:          httpErrorJsonHandler(http.StatusInternalServerError, "Internal server error"),
			expectedErrorMessage: "error 500: Internal server error",
			expectedErrorJson: &apimiddleware.DefaultErrorJson{
				Code:    http.StatusInternalServerError,
				Message: "Internal server error",
			},
			timeout: time.Second * 5,
			data:    &bytes.Buffer{},
		},
		{
			name:                 "999 error",
			funcHandler:          httpErrorJsonHandler(999, "Invalid error"),
			expectedErrorMessage: "error 999: Invalid error",
			expectedErrorJson: &apimiddleware.DefaultErrorJson{
				Code:    999,
				Message: "Invalid error",
			},
			timeout: time.Second * 5,
			data:    &bytes.Buffer{},
		},
		{
			// Regression: when the error body is not JSON (e.g. an HTML 502
			// from a reverse proxy), the wrapped error must surface the raw
			// body and status code instead of "failed to decode error json".
			name:                 "non-JSON error body",
			funcHandler:          invalidJsonErrHandler,
			expectedErrorMessage: "unsuccessful (404: foo)",
			timeout:              time.Second * 5,
			data:                 &bytes.Buffer{},
		},
		{
			name:                 "bad response json formatting",
			funcHandler:          invalidJsonResponseHandler,
			expectedErrorMessage: "failed to decode response json",
			timeout:              time.Second * 5,
			responseJson:         &beacon.GetGenesisResponse{},
			data:                 &bytes.Buffer{},
		},
		{
			name:                 "timeout",
			funcHandler:          httpErrorJsonHandler(http.StatusNotFound, "Not found"),
			expectedErrorMessage: "failed to send POST data to REST endpoint",
			timeout:              1,
			data:                 &bytes.Buffer{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc(endpoint, testCase.funcHandler)
			server := httptest.NewServer(mux)
			defer server.Close()

			ctx := context.Background()

			jsonRestHandler := beaconApiJsonRestHandler{
				timeout: testCase.timeout,
				host:    server.URL,
			}

			errorJson, err := jsonRestHandler.PostRestJson(
				ctx,
				endpoint,
				map[string]string{},
				testCase.data,
				testCase.responseJson,
			)

			assert.ErrorContains(t, testCase.expectedErrorMessage, err)
			assert.DeepEqual(t, testCase.expectedErrorJson, errorJson)
		})
	}
}

func TestGetRestJsonResponse_FailsOverToHealthyHost(t *testing.T) {
	const endpoint = "/example/rest/api/endpoint"

	genesisJSON := &beacon.GetGenesisResponse{
		Data: &beacon.Genesis{
			GenesisTime:           "123",
			GenesisValidatorsRoot: "0x456",
			GenesisForkVersion:    "0x789",
		},
	}

	var primaryCount atomic.Int32
	var secondaryCount atomic.Int32

	restoreDefaultClient := setDefaultClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Host {
			case "primary.example":
				primaryCount.Add(1)
				return jsonHTTPErrorResponse(req, http.StatusServiceUnavailable, "primary unavailable"), nil
			case "secondary.example":
				secondaryCount.Add(1)
				return jsonHTTPResponse(req, genesisJSON), nil
			default:
				t.Fatalf("unexpected host %q", req.URL.Host)
				return nil, nil
			}
		}),
	})
	defer restoreDefaultClient()

	jsonRestHandler := newBeaconAPIJSONRestHandler("http://primary.example,http://secondary.example", 5*time.Second)

	firstResponse := &beacon.GetGenesisResponse{}
	_, err := jsonRestHandler.GetRestJsonResponse(context.Background(), endpoint, firstResponse)
	require.NoError(t, err)
	assert.DeepEqual(t, genesisJSON, firstResponse)
	assert.Equal(t, int32(1), primaryCount.Load())
	assert.Equal(t, int32(1), secondaryCount.Load())

	secondResponse := &beacon.GetGenesisResponse{}
	_, err = jsonRestHandler.GetRestJsonResponse(context.Background(), endpoint, secondResponse)
	require.NoError(t, err)
	assert.DeepEqual(t, genesisJSON, secondResponse)
	assert.Equal(t, int32(1), primaryCount.Load())
	assert.Equal(t, int32(2), secondaryCount.Load())
}

func TestPostRestJson_FailsOverToHealthyHost(t *testing.T) {
	const endpoint = "/example/rest/api/endpoint"

	genesisJSON := &beacon.GetGenesisResponse{
		Data: &beacon.Genesis{
			GenesisTime:           "123",
			GenesisValidatorsRoot: "0x456",
			GenesisForkVersion:    "0x789",
		},
	}
	payload := []byte{1, 2, 3, 4, 5}

	var primaryCount atomic.Int32
	var secondaryCount atomic.Int32

	restoreDefaultClient := setDefaultClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Host {
			case "primary.example":
				primaryCount.Add(1)
				return jsonHTTPErrorResponse(req, http.StatusServiceUnavailable, "primary unavailable"), nil
			case "secondary.example":
				secondaryCount.Add(1)
				receivedPayload, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				assert.DeepEqual(t, payload, receivedPayload)
				return jsonHTTPResponse(req, genesisJSON), nil
			default:
				t.Fatalf("unexpected host %q", req.URL.Host)
				return nil, nil
			}
		}),
	})
	defer restoreDefaultClient()

	jsonRestHandler := newBeaconAPIJSONRestHandler("http://primary.example,http://secondary.example", 5*time.Second)

	firstResponse := &beacon.GetGenesisResponse{}
	_, err := jsonRestHandler.PostRestJson(context.Background(), endpoint, map[string]string{}, bytes.NewBuffer(payload), firstResponse)
	require.NoError(t, err)
	assert.DeepEqual(t, genesisJSON, firstResponse)
	assert.Equal(t, int32(1), primaryCount.Load())
	assert.Equal(t, int32(1), secondaryCount.Load())

	secondResponse := &beacon.GetGenesisResponse{}
	_, err = jsonRestHandler.PostRestJson(context.Background(), endpoint, map[string]string{}, bytes.NewBuffer(payload), secondResponse)
	require.NoError(t, err)
	assert.DeepEqual(t, genesisJSON, secondResponse)
	assert.Equal(t, int32(1), primaryCount.Load())
	assert.Equal(t, int32(2), secondaryCount.Load())
}

func TestJsonHandler_ContextError(t *testing.T) {
	const endpoint = "/example/rest/api/endpoint"
	mux := http.NewServeMux()
	mux.HandleFunc(endpoint, func(writer http.ResponseWriter, request *http.Request) {})
	server := httptest.NewServer(mux)
	defer server.Close()

	// Instantiate a cancellable context.
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel the context which results in "context canceled" error.
	cancel()

	jsonRestHandler := beaconApiJsonRestHandler{
		timeout: time.Second * 30,
		host:    server.URL,
	}

	_, err := jsonRestHandler.PostRestJson(
		ctx,
		endpoint,
		map[string]string{},
		&bytes.Buffer{},
		nil,
	)

	assert.ErrorContains(t, context.Canceled.Error(), err)

	_, err = jsonRestHandler.GetRestJsonResponse(
		ctx,
		endpoint,
		&beacon.GetGenesisResponse{},
	)

	assert.ErrorContains(t, context.Canceled.Error(), err)
}

func TestGetRestJsonResponse_ContextDeadlineOverridesDefaultTimeout(t *testing.T) {
	genesisJSON := &beacon.GetGenesisResponse{
		Data: &beacon.Genesis{
			GenesisTime:           "123",
			GenesisValidatorsRoot: "0x456",
			GenesisForkVersion:    "0x789",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	jsonRestHandler := beaconApiJsonRestHandler{
		timeout: 5 * time.Millisecond,
		host:    "http://example.com",
	}

	restoreDefaultClient := setDefaultClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			time.Sleep(20 * time.Millisecond)
			return jsonHTTPResponse(req, genesisJSON), nil
		}),
	})
	defer restoreDefaultClient()

	responseJSON := &beacon.GetGenesisResponse{}
	_, err := jsonRestHandler.GetRestJsonResponse(ctx, "/example/rest/api/endpoint", responseJSON)
	require.NoError(t, err)
	assert.DeepEqual(t, genesisJSON, responseJSON)
}

func TestPostRestJson_ContextDeadlineOverridesDefaultTimeout(t *testing.T) {
	dataBytes := []byte{1, 2, 3, 4, 5}

	genesisJSON := &beacon.GetGenesisResponse{
		Data: &beacon.Genesis{
			GenesisTime:           "123",
			GenesisValidatorsRoot: "0x456",
			GenesisForkVersion:    "0x789",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	jsonRestHandler := beaconApiJsonRestHandler{
		timeout: 5 * time.Millisecond,
		host:    "http://example.com",
	}

	restoreDefaultClient := setDefaultClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			time.Sleep(20 * time.Millisecond)

			receivedBytes, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			assert.DeepEqual(t, dataBytes, receivedBytes)

			return jsonHTTPResponse(req, genesisJSON), nil
		}),
	})
	defer restoreDefaultClient()

	responseJSON := &beacon.GetGenesisResponse{}
	_, err := jsonRestHandler.PostRestJson(
		ctx,
		"/example/rest/api/endpoint",
		map[string]string{},
		bytes.NewBuffer(dataBytes),
		responseJSON,
	)
	require.NoError(t, err)
	assert.DeepEqual(t, genesisJSON, responseJSON)
}

func httpErrorJsonHandler(statusCode int, errorMessage string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		errorJson := &apimiddleware.DefaultErrorJson{
			Code:    statusCode,
			Message: errorMessage,
		}

		marshalledError, err := json.Marshal(errorJson)
		if err != nil {
			panic(err)
		}

		w.WriteHeader(statusCode)
		_, err = w.Write(marshalledError)
		if err != nil {
			panic(err)
		}
	}
}

func invalidJsonErrHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, err := w.Write([]byte("foo"))
	if err != nil {
		panic(err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func setDefaultClient(client *http.Client) func() {
	oldClient := http.DefaultClient
	http.DefaultClient = client
	return func() {
		http.DefaultClient = oldClient
	}
}

func jsonHTTPResponse(req *http.Request, responseJSON any) *http.Response {
	bodyBytes, err := json.Marshal(responseJSON)
	if err != nil {
		panic(err)
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		Header:     make(http.Header),
		Request:    req,
	}
}

func jsonHTTPErrorResponse(req *http.Request, statusCode int, message string) *http.Response {
	bodyBytes, err := json.Marshal(&apimiddleware.DefaultErrorJson{
		Code:    statusCode,
		Message: message,
	})
	if err != nil {
		panic(err)
	}

	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		Header:     make(http.Header),
		Request:    req,
	}
}

func invalidJsonResponseHandler(w http.ResponseWriter, _ *http.Request) {
	_, err := w.Write([]byte("foo"))
	if err != nil {
		panic(err)
	}
}
