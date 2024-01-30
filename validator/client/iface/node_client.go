package iface

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	zondpb "github.com/theQRL/qrysm/v4/proto/qrysm/v1alpha1"
)

type NodeClient interface {
	GetSyncStatus(ctx context.Context, in *empty.Empty) (*zondpb.SyncStatus, error)
	GetGenesis(ctx context.Context, in *empty.Empty) (*zondpb.Genesis, error)
	GetVersion(ctx context.Context, in *empty.Empty) (*zondpb.Version, error)
	ListPeers(ctx context.Context, in *empty.Empty) (*zondpb.Peers, error)
}
