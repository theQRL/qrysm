package endtoend

import (
	"testing"

	"github.com/theQRL/qrysm/v4/config/params"
	"github.com/theQRL/qrysm/v4/runtime/version"
	"github.com/theQRL/qrysm/v4/testing/endtoend/types"
)

// Run mainnet e2e config with the current release validator against latest beacon node.
func TestEndToEnd_MainnetConfig_ValidatorAtCurrentRelease(t *testing.T) {
	e2eMainnet(t, true, types.StartAt(version.Capella, params.E2EMainnetTestConfig())).run()
}

func TestEndToEnd_MainnetConfig(t *testing.T) {
	e2eMainnet(t, false, types.StartAt(version.Capella, params.E2EMainnetTestConfig())).run()
}
