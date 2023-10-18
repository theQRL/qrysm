package iface

import (
	"context"

	zondpb "github.com/theQRL/qrysm/v4/proto/prysm/v1alpha1"
)

type SlasherClient interface {
	IsSlashableAttestation(ctx context.Context, in *zondpb.IndexedAttestation) (*zondpb.AttesterSlashingResponse, error)
	IsSlashableBlock(ctx context.Context, in *zondpb.SignedBeaconBlockHeader) (*zondpb.ProposerSlashingResponse, error)
}
