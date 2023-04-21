package rewards

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/bellatrix/rewards"
)

func TestMinimal_Bellatrix_Rewards(t *testing.T) {
	rewards.RunPrecomputeRewardsAndPenaltiesTests(t, "minimal")
}
