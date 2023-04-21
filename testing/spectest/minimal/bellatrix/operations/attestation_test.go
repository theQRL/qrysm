package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/bellatrix/operations"
)

func TestMinimal_Bellatrix_Operations_Attestation(t *testing.T) {
	operations.RunAttestationTest(t, "minimal")
}
