package operations

import (
	"testing"

	"github.com/cyyber/qrysm/v4/testing/spectest/shared/altair/operations"
)

func TestMainnet_Altair_Operations_ProposerSlashing(t *testing.T) {
	operations.RunProposerSlashingTest(t, "mainnet")
}
