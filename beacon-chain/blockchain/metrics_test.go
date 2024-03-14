package blockchain

import (
	"context"
	"testing"

	"github.com/theQRL/qrysm/v4/testing/require"
	"github.com/theQRL/qrysm/v4/testing/util"
)

func TestReportEpochMetrics_BadHeadState(t *testing.T) {
	s, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	h, err := util.NewBeaconStateCapella()
	require.NoError(t, err)
	require.NoError(t, h.SetValidators(nil))
	err = reportEpochMetrics(context.Background(), s, h)
	require.ErrorContains(t, "could not read every validator: state has nil validator slice", err)
}
