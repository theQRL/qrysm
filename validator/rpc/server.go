package rpc

import (
	"context"
	"fmt"
	"net"
	"time"

	middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpcopentracing "github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/sirupsen/logrus"
	"github.com/theQRL/qrysm/async/event"
	"github.com/theQRL/qrysm/io/logs"
	"github.com/theQRL/qrysm/monitoring/tracing"
	zondpbservice "github.com/theQRL/qrysm/proto/zond/service"
	"github.com/theQRL/qrysm/validator/accounts/wallet"
	"github.com/theQRL/qrysm/validator/client"
	iface "github.com/theQRL/qrysm/validator/client/iface"
	"github.com/theQRL/qrysm/validator/db"
	"go.opencensus.io/plugin/ocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

// Config options for the gRPC server.
type Config struct {
	ValidatorGatewayHost     string
	ValidatorGatewayPort     int
	ValidatorMonitoringHost  string
	ValidatorMonitoringPort  int
	BeaconClientEndpoint     string
	ClientMaxCallRecvMsgSize int
	ClientGrpcRetries        uint
	ClientGrpcRetryDelay     time.Duration
	ClientGrpcHeaders        []string
	ClientWithCert           string
	Host                     string
	Port                     string
	CertFlag                 string
	KeyFlag                  string
	ValDB                    db.Database
	WalletDir                string
	ValidatorService         *client.ValidatorService
	SyncChecker              client.SyncChecker
	GenesisFetcher           client.GenesisFetcher
	WalletInitializedFeed    *event.Feed
	NodeGatewayEndpoint      string
	Wallet                   *wallet.Wallet
}

// Server defining a gRPC server for the remote signer API.
type Server struct {
	logsStreamer              logs.Streamer
	beaconNodeClient          iface.NodeClient
	beaconNodeValidatorClient iface.ValidatorClient
	valDB                     db.Database
	ctx                       context.Context
	cancel                    context.CancelFunc
	beaconClientEndpoint      string
	clientMaxCallRecvMsgSize  int
	clientGrpcRetries         uint
	clientGrpcRetryDelay      time.Duration
	clientGrpcHeaders         []string
	clientWithCert            string
	host                      string
	port                      string
	listener                  net.Listener
	withCert                  string
	withKey                   string
	credentialError           error
	grpcServer                *grpc.Server
	jwtSecret                 []byte
	validatorService          *client.ValidatorService
	syncChecker               client.SyncChecker
	genesisFetcher            client.GenesisFetcher
	walletDir                 string
	wallet                    *wallet.Wallet
	walletInitializedFeed     *event.Feed
	walletInitialized         bool
	nodeGatewayEndpoint       string
	validatorMonitoringHost   string
	validatorMonitoringPort   int
	validatorGatewayHost      string
	validatorGatewayPort      int
}

// NewServer instantiates a new gRPC server.
func NewServer(ctx context.Context, cfg *Config) *Server {
	ctx, cancel := context.WithCancel(ctx)
	return &Server{
		ctx:                      ctx,
		cancel:                   cancel,
		logsStreamer:             logs.NewStreamServer(),
		host:                     cfg.Host,
		port:                     cfg.Port,
		withCert:                 cfg.CertFlag,
		withKey:                  cfg.KeyFlag,
		beaconClientEndpoint:     cfg.BeaconClientEndpoint,
		clientMaxCallRecvMsgSize: cfg.ClientMaxCallRecvMsgSize,
		clientGrpcRetries:        cfg.ClientGrpcRetries,
		clientGrpcRetryDelay:     cfg.ClientGrpcRetryDelay,
		clientGrpcHeaders:        cfg.ClientGrpcHeaders,
		clientWithCert:           cfg.ClientWithCert,
		valDB:                    cfg.ValDB,
		validatorService:         cfg.ValidatorService,
		syncChecker:              cfg.SyncChecker,
		genesisFetcher:           cfg.GenesisFetcher,
		walletDir:                cfg.WalletDir,
		walletInitializedFeed:    cfg.WalletInitializedFeed,
		walletInitialized:        cfg.Wallet != nil,
		wallet:                   cfg.Wallet,
		nodeGatewayEndpoint:      cfg.NodeGatewayEndpoint,
		validatorMonitoringHost:  cfg.ValidatorMonitoringHost,
		validatorMonitoringPort:  cfg.ValidatorMonitoringPort,
		validatorGatewayHost:     cfg.ValidatorGatewayHost,
		validatorGatewayPort:     cfg.ValidatorGatewayPort,
	}
}

// Start the gRPC server.
func (s *Server) Start() {
	// Setup the gRPC server options and TLS configuration.
	address := fmt.Sprintf("%s:%s", s.host, s.port)
	lis, err := net.Listen("tcp", address)
	if err != nil {
		log.WithError(err).Errorf("Could not listen to port in Start() %s", address)
	}
	s.listener = lis

	// Register interceptors for metrics gathering as well as our
	// own, custom JWT unary interceptor.
	opts := []grpc.ServerOption{
		grpc.StatsHandler(&ocgrpc.ServerHandler{}),
		grpc.UnaryInterceptor(middleware.ChainUnaryServer(
			recovery.UnaryServerInterceptor(
				recovery.WithRecoveryHandlerContext(tracing.RecoveryHandlerFunc),
			),
			grpcprometheus.UnaryServerInterceptor,
			grpcopentracing.UnaryServerInterceptor(),
			s.JWTInterceptor(),
		)),
	}
	grpcprometheus.EnableHandlingTimeHistogram()

	if s.withCert != "" && s.withKey != "" {
		creds, err := credentials.NewServerTLSFromFile(s.withCert, s.withKey)
		if err != nil {
			log.WithError(err).Fatal("Could not load TLS keys")
		}
		opts = append(opts, grpc.Creds(creds))
		log.WithFields(logrus.Fields{
			"crt-path": s.withCert,
			"key-path": s.withKey,
		}).Info("Loaded TLS certificates")
	}
	s.grpcServer = grpc.NewServer(opts...)

	// Register services available for the gRPC server.
	reflection.Register(s.grpcServer)
	zondpbservice.RegisterKeyManagementServer(s.grpcServer, s)

	go func() {
		if s.listener != nil {
			if err := s.grpcServer.Serve(s.listener); err != nil {
				log.WithError(err).Error("Could not serve")
			}
		}
	}()
	log.WithField("address", address).Info("gRPC server listening on address")
}

// Stop the gRPC server.
func (s *Server) Stop() error {
	s.cancel()
	if s.listener != nil {
		s.grpcServer.GracefulStop()
		log.Debug("Initiated graceful stop of server")
	}
	return nil
}

// Status returns nil or credentialError.
func (s *Server) Status() error {
	return s.credentialError
}
