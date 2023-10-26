package operations

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/capella/operations"
)

func TestMainnet_Capella_Operations_DilithiumToExecutionChange(t *testing.T) {
	operations.RunDilithiumToExecutionChangeTest(t, "mainnet")
}
