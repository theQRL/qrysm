package gateway

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/mux"
	gwruntime "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	logTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/theQRL/qrysm/api/gateway/apimiddleware"
	"github.com/theQRL/qrysm/cmd/beacon-chain/flags"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/urfave/cli/v2"
)

type mockEndpointFactory struct {
}

func (*mockEndpointFactory) Paths() []string {
	return []string{}
}

func (*mockEndpointFactory) Create(_ string) (*apimiddleware.Endpoint, error) {
	return nil, nil
}

func (*mockEndpointFactory) IsNil() bool {
	return false
}

func TestGateway_Customized(t *testing.T) {
	r := mux.NewRouter()
	cert := "cert"
	origins := []string{"origin"}
	size := uint64(100)
	endpointFactory := &mockEndpointFactory{}

	opts := []Option{
		WithRouter(r),
		WithRemoteCert(cert),
		WithAllowedOrigins(origins),
		WithMaxCallRecvMsgSize(size),
		WithApiMiddleware(endpointFactory),
		WithMuxHandler(func(
			_ *apimiddleware.ApiProxyMiddleware,
			_ http.HandlerFunc,
			_ http.ResponseWriter,
			_ *http.Request,
		) {
		}),
	}

	g, err := New(context.Background(), opts...)
	require.NoError(t, err)

	assert.Equal(t, r, g.cfg.router)
	assert.Equal(t, cert, g.cfg.remoteCert)
	require.Equal(t, 1, len(g.cfg.allowedOrigins))
	assert.Equal(t, origins[0], g.cfg.allowedOrigins[0])
	assert.Equal(t, size, g.cfg.maxCallRecvMsgSize)
	assert.Equal(t, endpointFactory, g.cfg.apiMiddlewareEndpointFactory)
}

func TestGateway_StartStop(t *testing.T) {
	hook := logTest.NewGlobal()

	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	ctx := cli.NewContext(&app, set, nil)

	gatewayPort := ctx.Int(flags.GRPCGatewayPort.Name)
	gatewayHost := ctx.String(flags.GRPCGatewayHost.Name)
	rpcHost := ctx.String(flags.RPCHost.Name)
	selfAddress := fmt.Sprintf("%s:%d", rpcHost, ctx.Int(flags.RPCPort.Name))
	gatewayAddress := fmt.Sprintf("%s:%d", gatewayHost, gatewayPort)

	opts := []Option{
		WithGatewayAddr(gatewayAddress),
		WithRemoteAddr(selfAddress),
		WithMuxHandler(func(
			_ *apimiddleware.ApiProxyMiddleware,
			_ http.HandlerFunc,
			_ http.ResponseWriter,
			_ *http.Request,
		) {
		}),
	}

	g, err := New(context.Background(), opts...)
	require.NoError(t, err)

	g.Start()
	go func() {
		require.LogsContain(t, hook, "Starting gRPC gateway")
		require.LogsDoNotContain(t, hook, "Starting API middleware")
	}()
	err = g.Stop()
	require.NoError(t, err)
}

func TestGateway_NilHandler_NotFoundHandlerRegistered(t *testing.T) {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	ctx := cli.NewContext(&app, set, nil)

	gatewayPort := ctx.Int(flags.GRPCGatewayPort.Name)
	gatewayHost := ctx.String(flags.GRPCGatewayHost.Name)
	rpcHost := ctx.String(flags.RPCHost.Name)
	selfAddress := fmt.Sprintf("%s:%d", rpcHost, ctx.Int(flags.RPCPort.Name))
	gatewayAddress := fmt.Sprintf("%s:%d", gatewayHost, gatewayPort)

	opts := []Option{
		WithGatewayAddr(gatewayAddress),
		WithRemoteAddr(selfAddress),
	}

	g, err := New(context.Background(), opts...)
	require.NoError(t, err)

	writer := httptest.NewRecorder()
	g.cfg.router.ServeHTTP(writer, &http.Request{Method: "GET", Host: "localhost", URL: &url.URL{Path: "/foo"}})
	assert.Equal(t, http.StatusNotFound, writer.Code)
}

func TestWithTimeout_DisablesGlobalGrpcGatewayTimeout(t *testing.T) {
	originalTimeout := gwruntime.DefaultContextTimeout
	defer func() {
		gwruntime.DefaultContextTimeout = originalTimeout
	}()
	gwruntime.DefaultContextTimeout = time.Minute

	g := &Gateway{cfg: &config{}}
	require.NoError(t, WithTimeout(5)(g))
	assert.Equal(t, 5*time.Second, g.cfg.timeout)
	assert.Equal(t, time.Duration(0), gwruntime.DefaultContextTimeout)
}

func TestGateway_WithRequestTimeout(t *testing.T) {
	t.Run("applies timeout to non-event routes", func(t *testing.T) {
		g := &Gateway{cfg: &config{timeout: 5 * time.Second}}
		handler := g.withRequestTimeout(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			deadline, ok := req.Context().Deadline()
			require.Equal(t, true, ok)
			remaining := time.Until(deadline)
			assert.Equal(t, true, remaining > 0)
			assert.Equal(t, true, remaining <= 5*time.Second)
		}))

		req := httptest.NewRequest(http.MethodGet, "http://example.com/qrl/v1/beacon/states/head", nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	})

	t.Run("skips timeout for event routes", func(t *testing.T) {
		g := &Gateway{cfg: &config{timeout: 5 * time.Second}}
		handler := g.withRequestTimeout(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			_, ok := req.Context().Deadline()
			assert.Equal(t, false, ok)
		}))

		for _, path := range []string{"/qrl/v1/events", "/internal/qrl/v1/events", "/api/qrl/v1/events"} {
			req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}
