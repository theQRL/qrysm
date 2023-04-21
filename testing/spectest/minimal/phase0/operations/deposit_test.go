package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/phase0/operations"
)

func TestMinimal_Phase0_Operations_Deposit(t *testing.T) {
	operations.RunDepositTest(t, "minimal")
}
