package fork_helper

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/altair/fork"
)

func TestMainnet_Altair_UpgradeToAltair(t *testing.T) {
	fork.RunUpgradeToAltair(t, "mainnet")
}
