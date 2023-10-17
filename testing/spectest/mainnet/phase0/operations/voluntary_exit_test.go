package operations

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/phase0/operations"
)

func TestMainnet_Phase0_Operations_VoluntaryExit(t *testing.T) {
	operations.RunVoluntaryExitTest(t, "mainnet")
}
