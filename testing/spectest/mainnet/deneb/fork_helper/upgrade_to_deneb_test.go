package fork_helper

import (
	"testing"

	"github.com/theQRL/qrysm/v4/testing/spectest/shared/deneb/fork"
)

func TestMainnet_UpgradeToDeneb(t *testing.T) {
	fork.RunUpgradeToDeneb(t, "mainnet")
}
