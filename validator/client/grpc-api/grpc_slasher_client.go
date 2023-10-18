package grpc_api

import (
	"context"

	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
	"github.com/theQRL/qrysm/v4/validator/client/iface"
	"google.golang.org/grpc"
)

type grpcSlasherClient struct {
	slasherClient zondpb.SlasherClient
}

func (c *grpcSlasherClient) IsSlashableAttestation(ctx context.Context, in *zondpb.IndexedAttestation) (*zondpb.AttesterSlashingResponse, error) {
	return c.slasherClient.IsSlashableAttestation(ctx, in)
}

func (c *grpcSlasherClient) IsSlashableBlock(ctx context.Context, in *zondpb.SignedBeaconBlockHeader) (*zondpb.ProposerSlashingResponse, error) {
	return c.slasherClient.IsSlashableBlock(ctx, in)
}

func NewSlasherClient(cc grpc.ClientConnInterface) iface.SlasherClient {
	return &grpcSlasherClient{zondpb.NewSlasherClient(cc)}
}
