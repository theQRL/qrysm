package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	pb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1/validator-client"
	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/validator/client"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type mockSyncChecker struct {
	syncing bool
}

func (m *mockSyncChecker) Syncing(_ context.Context) (bool, error) {
	return m.syncing, nil
}

type mockGenesisFetcher struct{}

func (_ *mockGenesisFetcher) GenesisInfo(_ context.Context) (*zondpb.Genesis, error) {
	genesis := timestamppb.New(time.Unix(0, 0))
	return &zondpb.Genesis{
		GenesisTime: genesis,
	}, nil
}

func TestServer_GetBeaconNodeConnection(t *testing.T) {
	ctx := context.Background()
	endpoint := "localhost:90210"
	vs, err := client.NewValidatorService(ctx, &client.Config{})
	require.NoError(t, err)
	s := &Server{
		walletInitialized:   true,
		validatorService:    vs,
		syncChecker:         &mockSyncChecker{syncing: false},
		genesisFetcher:      &mockGenesisFetcher{},
		nodeGatewayEndpoint: endpoint,
	}
	got, err := s.GetBeaconNodeConnection(ctx, &empty.Empty{})
	require.NoError(t, err)
	want := &pb.NodeConnectionResponse{
		BeaconNodeEndpoint: endpoint,
		Connected:          true,
		Syncing:            false,
		GenesisTime:        uint64(time.Unix(0, 0).Unix()),
	}
	require.DeepEqual(t, want, got)
}
