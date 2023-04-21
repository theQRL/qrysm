package fork

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/altair/fork"
)

func TestMinimal_Altair_UpgradeToAltair(t *testing.T) {
	fork.RunUpgradeToAltair(t, "minimal")
}
