package iface

import (
	"context"

	ethpb "github.com/cyyber/qrysm/v4/proto/prysm/v1alpha1"
)

type SlasherClient interface {
	IsSlashableAttestation(ctx context.Context, in *ethpb.IndexedAttestation) (*ethpb.AttesterSlashingResponse, error)
	IsSlashableBlock(ctx context.Context, in *ethpb.SignedBeaconBlockHeader) (*ethpb.ProposerSlashingResponse, error)
}
