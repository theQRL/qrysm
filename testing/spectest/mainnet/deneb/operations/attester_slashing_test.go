package operations

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/deneb/operations"
)

func TestMainnet_Deneb_Operations_AttesterSlashing(t *testing.T) {
	operations.RunAttesterSlashingTest(t, "mainnet")
}
