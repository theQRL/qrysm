package rewards

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/capella/rewards"
)

func TestMinimal_Capella_Rewards(t *testing.T) {
	rewards.RunPrecomputeRewardsAndPenaltiesTests(t, "minimal")
}
