package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/capella/operations"
)

func TestMinimal_Capella_Operations_SyncCommittee(t *testing.T) {
	operations.RunProposerSlashingTest(t, "minimal")
}
