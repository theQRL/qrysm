package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/altair/operations"
)

func TestMainnet_Altair_Operations_Attestation(t *testing.T) {
	operations.RunAttestationTest(t, "mainnet")
}
