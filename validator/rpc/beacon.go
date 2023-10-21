package rpc

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpcretry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	grpcopentracing "github.com/grpc-ecosystem/go-grpc-middleware/tracing/opentracing"
	grpcprometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/pkg/errors"
	grpcutil "github.com/theQRL/qrysm/v4/api/grpc"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	validatorpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/v4/validator/client"
	beaconChainClientFactory "github.com/theQRL/qrysm/v4/validator/client/beacon-chain-client-factory"
	nodeClientFactory "github.com/theQRL/qrysm/v4/validator/client/node-client-factory"
	validatorClientFactory "github.com/theQRL/qrysm/v4/validator/client/validator-client-factory"
	validatorHelpers "github.com/theQRL/qrysm/v4/validator/helpers"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Initialize a client connect to a beacon node gRPC endpoint.
func (s *Server) registerBeaconClient() error {
	streamInterceptor := grpc.WithStreamInterceptor(middleware.ChainStreamClient(
		grpcopentracing.StreamClientInterceptor(),
		grpcprometheus.StreamClientInterceptor,
		grpcretry.StreamClientInterceptor(),
	))
	dialOpts := client.ConstructDialOptions(
		s.clientMaxCallRecvMsgSize,
		s.clientWithCert,
		s.clientGrpcRetries,
		s.clientGrpcRetryDelay,
		streamInterceptor,
	)
	if dialOpts == nil {
		return errors.New("no dial options for beacon chain gRPC client")
	}

	s.ctx = grpcutil.AppendHeaders(s.ctx, s.clientGrpcHeaders)

	grpcConn, err := grpc.DialContext(s.ctx, s.beaconClientEndpoint, dialOpts...)
	if err != nil {
		return errors.Wrapf(err, "could not dial endpoint: %s", s.beaconClientEndpoint)
	}
	if s.clientWithCert != "" {
		log.Info("Established secure gRPC connection")
	}
	s.beaconNodeHealthClient = zondpb.NewHealthClient(grpcConn)

	conn := validatorHelpers.NewNodeConnection(
		grpcConn,
		s.beaconApiEndpoint,
		s.beaconApiTimeout,
	)

	s.beaconChainClient = beaconChainClientFactory.NewBeaconChainClient(conn)
	s.beaconNodeClient = nodeClientFactory.NewNodeClient(conn)
	s.beaconNodeValidatorClient = validatorClientFactory.NewValidatorClient(conn)
	return nil
}

// GetBeaconStatus retrieves information about the beacon node gRPC connection
// and certain chain metadata, such as the genesis time, the chain head, and the
// deposit contract address.
// DEPRECATED: Prysm Web UI and associated endpoints will be fully removed in a future hard fork.
func (s *Server) GetBeaconStatus(ctx context.Context, _ *empty.Empty) (*validatorpb.BeaconStatusResponse, error) {
	syncStatus, err := s.beaconNodeClient.GetSyncStatus(ctx, &emptypb.Empty{})
	if err != nil {
		//nolint:nilerr
		return &validatorpb.BeaconStatusResponse{
			BeaconNodeEndpoint: s.nodeGatewayEndpoint,
			Connected:          false,
			Syncing:            false,
		}, nil
	}
	genesis, err := s.beaconNodeClient.GetGenesis(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	genesisTime := uint64(time.Unix(genesis.GenesisTime.Seconds, 0).Unix())
	address := genesis.DepositContractAddress
	chainHead, err := s.beaconChainClient.GetChainHead(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	return &validatorpb.BeaconStatusResponse{
		BeaconNodeEndpoint:     s.beaconClientEndpoint,
		Connected:              true,
		Syncing:                syncStatus.Syncing,
		GenesisTime:            genesisTime,
		DepositContractAddress: address,
		ChainHead:              chainHead,
	}, nil
}

// GetValidatorParticipation is a wrapper around the /zond/v1alpha1 endpoint of the same name.
// DEPRECATED: Prysm Web UI and associated endpoints will be fully removed in a future hard fork.
func (s *Server) GetValidatorParticipation(
	ctx context.Context, req *zondpb.GetValidatorParticipationRequest,
) (*zondpb.ValidatorParticipationResponse, error) {
	return s.beaconChainClient.GetValidatorParticipation(ctx, req)
}

// GetValidatorPerformance is a wrapper around the /zond/v1alpha1 endpoint of the same name.
// DEPRECATED: Prysm Web UI and associated endpoints will be fully removed in a future hard fork.
func (s *Server) GetValidatorPerformance(
	ctx context.Context, req *zondpb.ValidatorPerformanceRequest,
) (*zondpb.ValidatorPerformanceResponse, error) {
	return s.beaconChainClient.GetValidatorPerformance(ctx, req)
}

// GetValidatorBalances is a wrapper around the /zond/v1alpha1 endpoint of the same name.
// DEPRECATED: Prysm Web UI and associated endpoints will be fully removed in a future hard fork.
func (s *Server) GetValidatorBalances(
	ctx context.Context, req *zondpb.ListValidatorBalancesRequest,
) (*zondpb.ValidatorBalances, error) {
	return s.beaconChainClient.ListValidatorBalances(ctx, req)
}

// GetValidators is a wrapper around the /zond/v1alpha1 endpoint of the same name.
// DEPRECATED: Prysm Web UI and associated endpoints will be fully removed in a future hard fork.
func (s *Server) GetValidators(
	ctx context.Context, req *zondpb.ListValidatorsRequest,
) (*zondpb.Validators, error) {
	return s.beaconChainClient.ListValidators(ctx, req)
}

// GetValidatorQueue is a wrapper around the /zond/v1alpha1 endpoint of the same name.
// DEPRECATED: Prysm Web UI and associated endpoints will be fully removed in a future hard fork.
func (s *Server) GetValidatorQueue(
	ctx context.Context, _ *empty.Empty,
) (*zondpb.ValidatorQueue, error) {
	return s.beaconChainClient.GetValidatorQueue(ctx, &emptypb.Empty{})
}

// GetPeers is a wrapper around the /zond/v1alpha1 endpoint of the same name.
// DEPRECATED: Prysm Web UI and associated endpoints will be fully removed in a future hard fork.
func (s *Server) GetPeers(
	ctx context.Context, _ *empty.Empty,
) (*zondpb.Peers, error) {
	return s.beaconNodeClient.ListPeers(ctx, &emptypb.Empty{})
}
