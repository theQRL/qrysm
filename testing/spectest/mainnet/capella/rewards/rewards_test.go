package rewards

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/capella/rewards"
)

func TestMainnet_Capella_Rewards(t *testing.T) {
	rewards.RunPrecomputeRewardsAndPenaltiesTests(t, "mainnet")
}
