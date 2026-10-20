package iface

import (
	"context"

	"github.com/pkg/errors"
	qrysmpb "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
)

// ErrNotSupported is returned when an endpoint is not available on the connected
// beacon node, for example a qrysm-specific endpoint when talking to a non-qrysm node.
var ErrNotSupported = errors.New("endpoint not supported")

// QrysmChainClient groups the qrysm-specific custom endpoints that are not part
// of the standard beacon API.
type QrysmChainClient interface {
	GetValidatorPerformance(context.Context, *qrysmpb.ValidatorPerformanceRequest) (*qrysmpb.ValidatorPerformanceResponse, error)
}
