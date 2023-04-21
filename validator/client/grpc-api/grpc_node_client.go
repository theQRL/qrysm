package grpc_api

import (
	"context"

	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/cyyber/qrysm/v4/validator/client/iface"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
)

type grpcNodeClient struct {
	nodeClient ethpb.NodeClient
}

func (c *grpcNodeClient) GetSyncStatus(ctx context.Context, in *empty.Empty) (*ethpb.SyncStatus, error) {
	return c.nodeClient.GetSyncStatus(ctx, in)
}

func (c *grpcNodeClient) GetGenesis(ctx context.Context, in *empty.Empty) (*ethpb.Genesis, error) {
	return c.nodeClient.GetGenesis(ctx, in)
}

func (c *grpcNodeClient) GetVersion(ctx context.Context, in *empty.Empty) (*ethpb.Version, error) {
	return c.nodeClient.GetVersion(ctx, in)
}

func (c *grpcNodeClient) ListPeers(ctx context.Context, in *empty.Empty) (*ethpb.Peers, error) {
	return c.nodeClient.ListPeers(ctx, in)
}

func NewNodeClient(cc grpc.ClientConnInterface) iface.NodeClient {
	return &grpcNodeClient{ethpb.NewNodeClient(cc)}
}
