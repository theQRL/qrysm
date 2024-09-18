package endtoend

import (
	"testing"

	"github.com/theQRL/qrysm/config/params"
	"github.com/theQRL/qrysm/runtime/version"
	"github.com/theQRL/qrysm/testing/endtoend/types"
)

// Run mainnet e2e config with the current release validator against latest beacon node.
func TestEndToEnd_MainnetConfig_ValidatorAtCurrentRelease(t *testing.T) {
	e2eMainnet(t, true, types.StartAt(version.Capella, params.E2EMainnetTestConfig())).run()
}

func TestEndToEnd_MainnetConfig(t *testing.T) {
	e2eMainnet(t, false, types.StartAt(version.Capella, params.E2EMainnetTestConfig())).run()
}
