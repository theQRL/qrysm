package client

import (
	"context"

	"github.com/theQRL/qrysm/consensus-types/primitives"
)

type dutyDependentRootProvider interface {
	GetDutyDependentRoots(ctx context.Context, epoch primitives.Epoch) ([]byte, []byte, error)
}
