package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/bellatrix/operations"
)

func TestMinimal_Bellatrix_Operations_ProposerSlashing(t *testing.T) {
	operations.RunProposerSlashingTest(t, "minimal")
}
