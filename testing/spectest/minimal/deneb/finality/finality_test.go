package finality

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/deneb/finality"
)

func TestMinimal_Deneb_Finality(t *testing.T) {
	finality.RunFinalityTest(t, "minimal")
}
