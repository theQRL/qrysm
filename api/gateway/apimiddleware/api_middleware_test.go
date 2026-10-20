package apimiddleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
)

// getOnlyEndpointFactory serves a single endpoint that only declares a GET
// response, like most of the beacon API paths proxied by the middleware.
type getOnlyEndpointFactory struct {
	path string
}

func (f *getOnlyEndpointFactory) Create(path string) (*Endpoint, error) {
	endpoint := DefaultEndpoint()
	endpoint.Path = path
	endpoint.GetResponse = &testResponseContainer{}
	return &endpoint, nil
}

func (f *getOnlyEndpointFactory) Paths() []string { return []string{f.path} }
func (*getOnlyEndpointFactory) IsNil() bool       { return false }

// TestWithMiddleware_CraftedPostToGetOnlyEndpoint is a regression test for a
// crafted `POST {}` to an endpoint without a POST request container: the
// middleware ran its request-container pipeline on a nil container and
// panicked in processField (reflect.TypeOf(nil).Kind()). The request must
// instead be proxied untouched so grpc-gateway can answer 405.
func TestWithMiddleware_CraftedPostToGetOnlyEndpoint(t *testing.T) {
	const path = "/qrl/v1/node/version"

	var gatewayMethod string
	var gatewayBody []byte
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayMethod = r.Method
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gatewayBody = body
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, err = w.Write([]byte(`{"code":405,"message":"Method Not Allowed"}`))
		require.NoError(t, err)
	}))
	defer gateway.Close()

	m := &ApiProxyMiddleware{
		GatewayAddress:  strings.TrimPrefix(gateway.URL, "http://"),
		EndpointCreator: &getOnlyEndpointFactory{path: path},
	}
	router := mux.NewRouter()
	m.Run(router)

	for _, tc := range []struct {
		name   string
		method string
		body   string
	}{
		{name: "POST empty object", method: http.MethodPost, body: "{}"},
		{name: "POST array", method: http.MethodPost, body: "[null]"},
		{name: "DELETE with body", method: http.MethodDelete, body: "{}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			// Must not panic.
			m.ServeHTTP(rec, req)

			assert.Equal(t, tc.method, gatewayMethod, "request was not proxied")
			assert.Equal(t, tc.body, string(gatewayBody), "request body was altered")
			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}

type nestedContainer struct {
	Value string `json:"value" hex:"true"`
}

type sliceContainer struct {
	Items  []*nestedContainer `json:"items"`
	Nested *nestedContainer   `json:"nested"`
	Value  string             `json:"value" hex:"true"`
}

// TestProcessField_NilPointers covers the nil cases that used to panic in the
// recursive field processor: a `null` element inside a struct-pointer slice
// (e.g. `[null]` posted to a container endpoint), a nil struct-pointer field,
// a typed nil container and an untyped nil container.
func TestProcessField_NilPointers(t *testing.T) {
	processors := []fieldProcessor{{tag: "hex", f: hexToBase64Processor}}

	c := &sliceContainer{
		Items:  []*nestedContainer{nil, {Value: "0x01"}, nil},
		Nested: nil,
		Value:  "0x02",
	}
	require.NoError(t, processField(c, processors))
	// Non-nil elements are still processed.
	assert.Equal(t, "AQ==", c.Items[1].Value)
	assert.Equal(t, "Ag==", c.Value)

	var typedNil *sliceContainer
	require.NoError(t, processField(typedNil, processors))

	err := processField(nil, processors)
	require.ErrorContains(t, "nil container", err)
}

func TestDeserializeRequestBodyIntoContainer_NilContainer(t *testing.T) {
	errJson := DeserializeRequestBodyIntoContainer(bytes.NewBufferString("{}"), nil)
	require.NotNil(t, errJson)
	assert.Equal(t, http.StatusInternalServerError, errJson.StatusCode())
}
