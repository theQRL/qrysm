package forkchoice

import (
	"testing"

	"github.com/cyyber/qrysm/v4/runtime/version"
	"github.com/cyyber/qrysm/v4/testing/spectest/shared/common/forkchoice"
)

func TestMainnet_Bellatrix_Forkchoice(t *testing.T) {
	forkchoice.Run(t, "mainnet", version.Bellatrix)
}
