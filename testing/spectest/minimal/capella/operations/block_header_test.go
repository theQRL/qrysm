package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/capella/operations"
)

func TestMinimal_Capella_Operations_BlockHeader(t *testing.T) {
	operations.RunBlockHeaderTest(t, "minimal")
}
