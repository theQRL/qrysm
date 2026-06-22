package rpc

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// JWTInterceptor is kept as a wrapper for existing callers.
func (s *Server) JWTInterceptor() grpc.UnaryServerInterceptor {
	return s.AuthTokenInterceptor()
}

// AuthTokenInterceptor is a gRPC unary interceptor to authorize incoming requests.
func (s *Server) AuthTokenInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if err := s.authorize(ctx); err != nil {
			return nil, err
		}
		h, err := handler(ctx, req)
		log.WithError(err).WithFields(logrus.Fields{
			"FullMethod": info.FullMethod,
			"Server":     info.Server,
		}).Debug("Request handled")
		return h, err
	}
}

// Authorize the token received is valid.
func (s *Server) authorize(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "Retrieving metadata failed")
	}

	authHeader, ok := md["authorization"]
	if !ok {
		return status.Errorf(codes.Unauthenticated, "Authorization token could not be found")
	}
	if len(authHeader) < 1 || !strings.HasPrefix(authHeader[0], "Bearer ") {
		return status.Error(codes.Unauthenticated, "Invalid auth header, needs Bearer {token}")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader[0], "Bearer "))
	if token == authHeader[0] {
		return status.Error(codes.Unauthenticated, "Invalid auth header, needs Bearer {token}")
	}
	if token == "" || strings.Contains(token, " ") {
		return status.Error(codes.Unauthenticated, "Invalid token format")
	}
	if s.authToken == "" {
		return status.Error(codes.Internal, "Authorization token is not initialized")
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(s.authToken)) != 1 {
		return status.Error(codes.Unauthenticated, "Invalid auth token")
	}
	return nil
}
