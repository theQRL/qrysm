package blockchain

import (
	"context"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/consensus-types/primitives"
	"github.com/theQRL/qrysm/testing/assert"
	"github.com/theQRL/qrysm/testing/require"
	"github.com/theQRL/qrysm/testing/util"
)

func TestReportEpochMetrics_BadHeadState(t *testing.T) {
	s, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	h, err := util.NewBeaconStateZond()
	require.NoError(t, err)
	require.NoError(t, h.SetValidators(nil))
	err = reportEpochMetrics(context.Background(), s, h)
	require.ErrorContains(t, "could not read every validator: state has nil validator slice", err)
}

func TestReportEpochMetrics_EffectiveBalances(t *testing.T) {
	st, _ := util.DeterministicGenesisStateZond(t, 4)
	require.NoError(t, st.SetSlot(2*params.BeaconConfig().SlotsPerEpoch))
	increment := params.BeaconConfig().EffectiveBalanceIncrement
	for i := primitives.ValidatorIndex(0); i < 4; i++ {
		v, err := st.ValidatorAtIndex(i)
		require.NoError(t, err)
		v.EffectiveBalance = uint64(i+1) * increment
		switch i {
		case 1:
			v.ExitEpoch = 3 // Still active, with a scheduled exit.
		case 2:
			v.Slashed = true
			v.ExitEpoch = 3 // Still active, but slashed.
		case 3:
			v.ExitEpoch = 2 // Already exited; exclude its balance.
		}
		require.NoError(t, st.UpdateValidatorAtIndex(i, v))
	}
	require.NoError(t, reportEpochMetrics(context.Background(), st, st))
	for _, tc := range []struct {
		label string
		want  uint64
	}{
		{label: "Active", want: 6 * increment},
		{label: "Exiting", want: 2 * increment},
		{label: "Slashing", want: 3 * increment},
	} {
		m := &dto.Metric{}
		require.NoError(t, validatorsEffectiveBalance.WithLabelValues(tc.label).Write(m))
		assert.Equal(t, float64(tc.want), m.GetGauge().GetValue(), tc.label)
	}
}

func TestReportEpochMetrics_ProcessedDeposits(t *testing.T) {
	st, _ := util.DeterministicGenesisStateZond(t, 4)
	for _, tc := range []struct {
		name  string
		count uint64
	}{
		{name: "no deposits", count: 0},
		{name: "one deposit", count: 1},
		{name: "all deposits", count: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The index points to the next unprocessed deposit: zero means
			// none processed, and reaching DepositCount means all processed.
			require.NoError(t, st.SetExecutionDepositIndex(tc.count))
			require.NoError(t, reportEpochMetrics(context.Background(), st, st))
			m := &dto.Metric{}
			require.NoError(t, processedDepositsCount.Write(m))
			assert.Equal(t, float64(tc.count), m.GetGauge().GetValue())
		})
	}
}
