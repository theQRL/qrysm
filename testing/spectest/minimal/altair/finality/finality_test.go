package finality

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/altair/finality"
)

func TestMinimal_Altair_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "minimal")
}
