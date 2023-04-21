package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/bellatrix/operations"
)

func TestMinimal_Bellatrix_Operations_VoluntaryExit(t *testing.T) {
	operations.RunVoluntaryExitTest(t, "minimal")
}
