package rewards

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/altair/rewards"
)

func TestMainnet_Altair_Rewards(t *testing.T) {
	rewards.RunPrecomputeRewardsAndPenaltiesTests(t, "mainnet")
}
