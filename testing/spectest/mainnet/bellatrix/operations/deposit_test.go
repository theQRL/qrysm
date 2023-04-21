package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/bellatrix/operations"
)

func TestMainnet_Bellatrix_Operations_Deposit(t *testing.T) {
	operations.RunDepositTest(t, "mainnet")
}
