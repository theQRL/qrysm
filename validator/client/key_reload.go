package client

import (
	"context"

	"github.com/pkg/errors"
	field_params "github.com/theQRL/qrysm/config/fieldparams"
	zond "github.com/theQRL/qrysm/proto/qrysm/v1alpha1"
	"go.opencensus.io/trace"
)

// HandleKeyReload makes sure the validator keeps operating correctly after a change to the underlying keys.
// It is also responsible for logging out information about the new state of keys.
func (v *validator) HandleKeyReload(ctx context.Context, currentKeys [][field_params.DilithiumPubkeyLength]byte) (anyActive bool, err error) {
	ctx, span := trace.StartSpan(ctx, "validator.HandleKeyReload")
	defer span.End()

	statusRequestKeys := make([][]byte, len(currentKeys))
	for i := range currentKeys {
		statusRequestKeys[i] = currentKeys[i][:]
	}
	resp, err := v.validatorClient.MultipleValidatorStatus(ctx, &zond.MultipleValidatorStatusRequest{
		PublicKeys: statusRequestKeys,
	})
	if err != nil {
		return false, err
	}
	statuses := make([]*validatorStatus, len(resp.Statuses))
	for i, s := range resp.Statuses {
		statuses[i] = &validatorStatus{
			publicKey: resp.PublicKeys[i],
			status:    s,
			index:     resp.Indices[i],
		}
	}
	vals, err := v.beaconClient.ListValidators(ctx, &zond.ListValidatorsRequest{Active: true, PageSize: 0})
	if err != nil {
		return false, errors.Wrap(err, "could not get active validator count")
	}
	anyActive = v.checkAndLogValidatorStatus(statuses, uint64(vals.TotalSize))
	if anyActive {
		logActiveValidatorStatus(statuses)
	}

	return anyActive, nil
}
