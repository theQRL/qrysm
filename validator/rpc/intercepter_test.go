package rpc

import (
	"context"
	"strings"
	"testing"

	"github.com/theQRL/qrysm/testing/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestServer_AuthTokenInterceptor_Verify(t *testing.T) {
	s := Server{
		authToken: testAuthToken(),
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}
	ctxMD := map[string][]string{
		"authorization": {"Bearer " + s.authToken},
	}
	ctx := context.Background()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)
	_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	require.NoError(t, err)
}

func TestServer_AuthTokenInterceptor_BadToken(t *testing.T) {
	s := Server{
		authToken: testAuthToken(),
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}

	ctxMD := map[string][]string{
		"authorization": {"Bearer " + badAuthToken()},
	}
	ctx := context.Background()
	ctx = metadata.NewIncomingContext(ctx, ctxMD)
	_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	require.ErrorContains(t, "Invalid auth token", err)
}

func TestServer_AuthTokenInterceptor_NotInitialized(t *testing.T) {
	s := Server{}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{
		"authorization": {"Bearer " + testAuthToken()},
	})
	_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
	require.ErrorContains(t, "Authorization token is not initialized", err)
}

func TestServer_AuthTokenInterceptor_InvalidTokenFormat(t *testing.T) {
	s := Server{
		authToken: testAuthToken(),
	}
	interceptor := s.AuthTokenInterceptor()

	unaryInfo := &grpc.UnaryServerInfo{
		FullMethod: "Proto.CreateWallet",
	}
	unaryHandler := func(ctx context.Context, req any) (any, error) {
		return nil, nil
	}
	token := s.authToken

	tests := []struct {
		name      string
		authValue string
		wantErr   string
	}{
		{
			name:      "no space after Bearer",
			authValue: "Bearer" + token,
			wantErr:   "Invalid auth header",
		},
		{
			name:      "no Bearer prefix",
			authValue: token,
			wantErr:   "Invalid auth header",
		},
		{
			name:      "multiple Bearer splits",
			authValue: "Bearer " + token[:2] + " Bearer " + token[2:],
			wantErr:   "Invalid token format",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctxMD := map[string][]string{
				"authorization": {tt.authValue},
			}
			ctx := metadata.NewIncomingContext(context.Background(), ctxMD)
			_, err := interceptor(ctx, "xyz", unaryInfo, unaryHandler)
			require.ErrorContains(t, tt.wantErr, err)
		})
	}
}

func testAuthToken() string {
	return "0x" + strings.Repeat("11", 32)
}

func badAuthToken() string {
	return "0x" + strings.Repeat("22", 32)
}
