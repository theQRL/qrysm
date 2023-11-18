package operations

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/deneb/operations"
)

func TestMinimal_Deneb_Operations_DilithiumToExecutionChange(t *testing.T) {
	operations.RunDilithiumToExecutionChangeTest(t, "minimal")
}
