package forkchoice

import (
	"testing"

	"github.com/cyyber/qrysm/v4/runtime/version"
	"github.com/cyyber/qrysm/v4/testing/spectest/shared/common/forkchoice"
)

func TestMainnet_Capella_Forkchoice(t *testing.T) {
	forkchoice.Run(t, "mainnet", version.Capella)
}
