package iface

import (
	"context"

	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type NodeClient interface {
	GetSyncStatus(ctx context.Context, in *emptypb.Empty) (*qrysmpb.SyncStatus, error)
	GetGenesis(ctx context.Context, in *emptypb.Empty) (*qrysmpb.Genesis, error)
	GetVersion(ctx context.Context, in *emptypb.Empty) (*qrysmpb.Version, error)
	ListPeers(ctx context.Context, in *emptypb.Empty) (*qrysmpb.Peers, error)
}
