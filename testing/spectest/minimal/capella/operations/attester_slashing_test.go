package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/capella/operations"
)

func TestMinimal_Capella_Operations_AttesterSlashing(t *testing.T) {
	operations.RunAttesterSlashingTest(t, "minimal")
}
