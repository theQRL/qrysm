package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/altair/operations"
)

func TestMainnet_Altair_Operations_BlockHeader(t *testing.T) {
	operations.RunBlockHeaderTest(t, "mainnet")
}
