package grpc_api

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/validator/client/iface"
	"google.golang.org/grpc"
)

type grpcNodeClient struct {
	nodeClient zondpb.NodeClient
}

func (c *grpcNodeClient) GetSyncStatus(ctx context.Context, in *empty.Empty) (*zondpb.SyncStatus, error) {
	return c.nodeClient.GetSyncStatus(ctx, in)
}

func (c *grpcNodeClient) GetGenesis(ctx context.Context, in *empty.Empty) (*zondpb.Genesis, error) {
	return c.nodeClient.GetGenesis(ctx, in)
}

func (c *grpcNodeClient) GetVersion(ctx context.Context, in *empty.Empty) (*zondpb.Version, error) {
	return c.nodeClient.GetVersion(ctx, in)
}

func (c *grpcNodeClient) ListPeers(ctx context.Context, in *empty.Empty) (*zondpb.Peers, error) {
	return c.nodeClient.ListPeers(ctx, in)
}

func NewNodeClient(cc grpc.ClientConnInterface) iface.NodeClient {
	return &grpcNodeClient{zondpb.NewNodeClient(cc)}
}
